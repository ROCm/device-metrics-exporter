/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package gpuagent

// Workload-source test helpers: fixtures, scheduler and kubelet fakes, and
// source setup over a mocked GPU client.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"gotest.tools/assert"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

const testGPUCount = 4

// testLabels enables every workload label plus the GPU_UUID key.
const testLabels = `"Labels":["GPU_UUID","POD","NAMESPACE","CONTAINER",` +
	`"JOB_ID","JOB_USER","JOB_PARTITION","JOB_ACCOUNT","CLUSTER_NAME"]`

const pullTestConfig = `{"GPUConfig":{` + testLabels + `,"Fields":["GPU_PACKAGE_POWER"]}}`

const hrTestConfig = `{"CommonConfig":{"HighResolution":{"Enable":true}},` +
	`"GPUConfig":{` + testLabels + `,"Fields":["GPU_PACKAGE_POWER"]}}`

var errKubeletUnavailable = errors.New("rpc error: code = Unavailable desc = connection error: " +
	"desc = \"transport: Error while dialing: dial unix /var/lib/kubelet/pod-resources/kubelet.sock: " +
	"connect: no such file or directory\"")

// fakeSched is a SchedulerClient fake with a settable map and error; it does
// not implement scheduler.ChangeNotifier.
type fakeSched struct {
	mu     sync.Mutex
	typ    scheduler.SchedulerType
	wls    map[string]scheduler.Workload
	err    error
	lists  int
	closes int
}

func (f *fakeSched) set(wls map[string]scheduler.Workload, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wls, f.err = wls, err
}

func (f *fakeSched) ListWorkloads() (map[string]scheduler.Workload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]scheduler.Workload, len(f.wls))
	for k, v := range f.wls {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSched) CheckExportLabels(map[string]bool) bool { return true }

func (f *fakeSched) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}

func (f *fakeSched) Type() scheduler.SchedulerType { return f.typ }

func (f *fakeSched) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists, f.closes
}

// testGPUResp is a deterministic GPU list: fixed UUIDs, index i, PCIe
// 0000:0<i+1>:00.0, DRM card 1+i, render 128+i.
func testGPUResp(n int) *amdgpu.GPUGetResponse {
	resp := &amdgpu.GPUGetResponse{ApiStatus: amdgpu.ApiStatus_API_STATUS_OK}
	for i := 0; i < n; i++ {
		id := make([]byte, 16)
		for j := range id {
			id[j] = byte(16*i + j + 1)
		}
		resp.Response = append(resp.Response, &amdgpu.GPU{
			Spec: &amdgpu.GPUSpec{Id: id},
			Status: &amdgpu.GPUStatus{
				Index:       uint32(i),
				SerialNum:   fmt.Sprintf("SN%d", i),
				DRMCardId:   uint32(1 + i),
				DRMRenderId: uint32(128 + i),
				PCIeStatus:  &amdgpu.GPUPCIeStatus{PCIeBusId: fmt.Sprintf("0000:0%d:00.0", i+1)},
			},
			Stats: &amdgpu.GPUStats{PackagePower: uint64(100 + i)},
		})
	}
	return resp
}

// testGPUIDs returns the test GPUs' 16-byte ids.
func testGPUIDs() [][]byte {
	var ids [][]byte
	for _, g := range testGPUResp(testGPUCount).Response {
		ids = append(ids, g.Spec.Id)
	}
	return ids
}

func testPod(i int) scheduler.Workload {
	return scheduler.Workload{Type: scheduler.Kubernetes, Info: scheduler.PodResourceInfo{
		Pod: fmt.Sprintf("pod%d", i), Namespace: fmt.Sprintf("ns%d", i), Container: fmt.Sprintf("ctr%d", i)}}
}

func testJob(id, user string) scheduler.Workload {
	return scheduler.Workload{Type: scheduler.Slurm, Info: scheduler.JobInfo{
		Id: id, User: user, Partition: "gpu", Cluster: "c1", Account: "acct-" + user}}
}

// workloadFixture: k8s and slurm parts (nil = no such scheduler).
type workloadFixture struct {
	name  string
	k8s   map[string]scheduler.Workload
	slurm map[string]scheduler.Workload
}

func workloadFixtures() []workloadFixture {
	return []workloadFixture{
		{name: "k8s", k8s: map[string]scheduler.Workload{
			"0000:01:00.0": testPod(0), "0000:02:00.0": testPod(1), "0000:09:00.0": testPod(9)}},
		{name: "slurm", slurm: map[string]scheduler.Workload{
			"0": testJob("100", "alice"), "2": testJob("102", "bob"), "7": testJob("107", "zed")}},
		// mixed: k8s on GPUs 0,1 (pcie key) and key "3"; slurm on "1" (same GPU
		// as pod1, different key) and "3" (same key: slurm overwrites k8s).
		{name: "mixed",
			k8s: map[string]scheduler.Workload{
				"0000:01:00.0": testPod(0), "0000:02:00.0": testPod(1), "3": testPod(3)},
			slurm: map[string]scheduler.Workload{
				"1": testJob("101", "carol"), "3": testJob("103", "dave")}},
	}
}

func fixtureByName(name string) workloadFixture {
	for _, fx := range workloadFixtures() {
		if fx.name == name {
			return fx
		}
	}
	panic("no fixture " + name)
}

type workloadEnv struct {
	ga    *GPUAgentClient
	gpu   *GPUAgentGPUClient
	k8s   *fakeSched
	slurm *fakeSched
}

// newWorkloadEnv builds a GPU client over deterministic mocks with fake k8s
// and slurm schedulers.
func newWorkloadEnv(t testing.TB, cfg string, fx workloadFixture) *workloadEnv {
	t.Helper()
	logger.Init(true)
	t.Setenv("DS_NODE_NAME", "test-node")
	ctl := gomock.NewController(t)
	gpuMock := mock_gen.NewMockGPUSvcClient(ctl)
	evtMock := mock_gen.NewMockEventSvcClient(ctl)
	gpuMock.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(testGPUResp(testGPUCount), nil).AnyTimes()
	gpuMock.EXPECT().GPUCPERGet(gomock.Any(), gomock.Any()).
		Return(&amdgpu.GPUCPERGetResponse{ApiStatus: amdgpu.ApiStatus_API_STATUS_OK}, nil).AnyTimes()
	evtMock.EXPECT().EventGet(gomock.Any(), gomock.Any()).
		Return(&amdgpu.EventResponse{ApiStatus: amdgpu.ApiStatus_API_STATUS_OK}, nil).AnyTimes()

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	assert.NilError(t, os.WriteFile(cfgPath, []byte(cfg), 0o644))
	mhh, err := metricsutil.NewMetrics(config.NewConfigHandler(cfgPath,
		config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort}))
	assert.NilError(t, err)
	mhh.InitConfig(context.Background())
	ga := NewAgent(mhh, WithK8sClient(nil), WithK8sSchedulerClient(nil), WithSlurmClient(false),
		WithGPUMonitoring(true))
	assert.NilError(t, ga.Init())
	env := &workloadEnv{ga: ga}
	ga.isKubernetes = false
	if fx.k8s != nil {
		env.k8s = &fakeSched{typ: scheduler.Kubernetes, wls: fx.k8s}
		ga.isKubernetes = true
		ga.k8sScheduler = env.k8s
	}
	if fx.slurm != nil {
		env.slurm = &fakeSched{typ: scheduler.Slurm, wls: fx.slurm}
		ga.slurmScheduler = env.slurm
	}
	for _, c := range ga.clients {
		if c.GetDeviceType() == globals.GPUDevice {
			env.gpu = c.(*GPUAgentGPUClient)
		}
	}
	assert.Assert(t, env.gpu != nil)
	t.Cleanup(env.gpu.stopWorkloadSource) // sources otherwise live for the process
	env.gpu.gpuclient = gpuMock
	env.gpu.evtclient = evtMock
	assert.NilError(t, env.gpu.InitConfigs())
	getResp, _, gerr := env.gpu.getGPUs(env.gpu.gCache.gpuGetFilter, &env.gpu.gCache.metricsCache)
	assert.NilError(t, gerr)
	env.gpu.initGPUMetadata(getResp.GetResponse())
	return env
}

// render gathers the client's /metrics exposition: metric families with the
// exporter prefix, text format, lines sorted.
func (e *workloadEnv) render(t *testing.T) string {
	t.Helper()
	mfs, err := e.gpu.gpuHandler.mh.GetRegistry().Gather()
	assert.NilError(t, err)
	return e.encode(t, mfs)
}

// encode renders the prefixed families as sorted exposition lines.
func (e *workloadEnv) encode(t *testing.T, mfs []*dto.MetricFamily) string {
	t.Helper()
	prefix := e.gpu.gpuHandler.mh.GetPrefix()
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if strings.HasPrefix(mf.GetName(), prefix) {
			assert.NilError(t, enc.Encode(mf))
		}
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// renderPull runs one store-serving pull scrape (fetchMetricsOnce) and renders
// what /metrics serves from the store.
func (e *workloadEnv) renderPull(t *testing.T) string {
	t.Helper()
	assert.NilError(t, e.gpu.fetchMetricsOnce(context.Background()))
	return e.renderStore(t)
}

// renderStore renders what /metrics serves from the store, without a fetch.
func (e *workloadEnv) renderStore(t *testing.T) string {
	t.Helper()
	descs, err := e.gpu.gpuStoreFieldDescs()
	assert.NilError(t, err)
	reg := prometheus.NewRegistry()
	assert.NilError(t, reg.Register(snapshotstore.NewStoreCollector(e.gpu.gpuHandler.mh.GetStore(), descs)))
	mfs, err := reg.Gather()
	assert.NilError(t, err)
	return e.encode(t, mfs)
}

// notifySched is a slurm-like ChangeNotifier; hook runs at "changed",
// "version" and "list" so tests can inject a bump there.
type notifySched struct {
	mu      sync.Mutex
	wls     map[string]scheduler.Workload
	version uint64
	changed chan struct{}
	lists   atomic.Int64
	closes  atomic.Int64
	hook    atomic.Pointer[func(point string)]
}

var _ scheduler.ChangeNotifier = (*notifySched)(nil)

func newNotifySched(wls map[string]scheduler.Workload) *notifySched {
	return &notifySched{wls: wls, changed: make(chan struct{})}
}

// set replaces the content and bumps the version (like processSlurm on a real
// content change).
func (f *notifySched) set(wls map[string]scheduler.Workload) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wls = wls
	f.version++
	close(f.changed)
	f.changed = make(chan struct{})
}

func (f *notifySched) fire(point string) {
	if h := f.hook.Load(); h != nil {
		(*h)(point)
	}
}

func (f *notifySched) Version() uint64 {
	f.mu.Lock()
	v := f.version
	f.mu.Unlock()
	f.fire("version")
	return v
}

func (f *notifySched) Changed() <-chan struct{} {
	f.mu.Lock()
	ch := f.changed
	f.mu.Unlock()
	f.fire("changed")
	return ch
}

func (f *notifySched) ListWorkloads() (map[string]scheduler.Workload, error) {
	f.lists.Add(1)
	f.mu.Lock()
	out := make(map[string]scheduler.Workload, len(f.wls))
	for k, v := range f.wls {
		out[k] = v
	}
	f.mu.Unlock()
	f.fire("list")
	return out, nil
}

func (f *notifySched) CheckExportLabels(map[string]bool) bool { return true }
func (f *notifySched) Close() error                           { f.closes.Add(1); return nil }
func (f *notifySched) Type() scheduler.SchedulerType          { return scheduler.Slurm }

// fakeKubelet serves a PodResourcesLister on a temp unix socket with a call
// counter and a settable response / error.
type fakeKubelet struct {
	kube.UnimplementedPodResourcesListerServer
	sock  string
	calls atomic.Int64
	mu    sync.Mutex
	resp  *kube.ListPodResourcesResponse
	err   error
	srv   *grpc.Server
}

func newFakeKubelet(t testing.TB, resp *kube.ListPodResourcesResponse) *fakeKubelet {
	t.Helper()
	// short path: unix socket paths are limited to ~108 bytes
	dir, err := os.MkdirTemp("", "kl")
	assert.NilError(t, err)
	k := &fakeKubelet{sock: filepath.Join(dir, "k.sock"), resp: resp}
	lis, err := net.Listen("unix", k.sock)
	assert.NilError(t, err)
	k.srv = grpc.NewServer()
	kube.RegisterPodResourcesListerServer(k.srv, k)
	go func() { _ = k.srv.Serve(lis) }()
	t.Cleanup(func() {
		k.srv.Stop()
		_ = os.RemoveAll(dir)
	})
	return k
}

func (k *fakeKubelet) List(context.Context, *kube.ListPodResourcesRequest) (*kube.ListPodResourcesResponse, error) {
	k.calls.Add(1)
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.resp, k.err
}

func (k *fakeKubelet) set(resp *kube.ListPodResourcesResponse, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.resp, k.err = resp, err
}

// client returns a real kubelet scheduler client dialing the fake.
func (k *fakeKubelet) client(t testing.TB) scheduler.SchedulerClient {
	t.Helper()
	cl, err := scheduler.NewKubernetesClientForSocket(context.Background(), k.sock)
	assert.NilError(t, err)
	return cl
}

// kubeResp builds a device-plugin response: one pod per entry, pod<i> on pcie.
func kubeResp(pcie ...string) *kube.ListPodResourcesResponse {
	r := &kube.ListPodResourcesResponse{}
	for i, p := range pcie {
		r.PodResources = append(r.PodResources, &kube.PodResources{
			Name: fmt.Sprintf("pod%d", i), Namespace: fmt.Sprintf("ns%d", i),
			Containers: []*kube.ContainerResources{{Name: fmt.Sprintf("ctr%d", i),
				Devices: []*kube.ContainerDevices{{ResourceName: "amd.com/gpu", DeviceIds: []string{p}}}}}})
	}
	return r
}

// manualSource installs a seeded source with no goroutine on the GPU client,
// so a test drives refresh itself (single goroutine, deterministic).
func manualSource(e *workloadEnv) *workloadSource {
	s := newWorkloadSource(e.ga, time.Hour)
	s.refresh(true)
	e.gpu.wlSource.Store(s)
	return s
}

// runningSource starts the real source with the given interval and stops it
// at test end (the source otherwise lives for the process).
func runningSource(t testing.TB, e *workloadEnv, interval time.Duration) *workloadSource {
	t.Helper()
	e.gpu.wlInterval = interval
	s := e.gpu.ensureWorkloadSource()
	t.Cleanup(s.stop)
	return s
}

// startHRSource mirrors StartHighRes' label setup; interval <= 0 installs a
// manual source the test refreshes itself (never concurrently).
func (e *workloadEnv) startHRSource(t testing.TB, interval time.Duration) (*workloadLabelCache, *workloadSource) {
	t.Helper()
	ga := e.gpu
	getResp, pm, err := ga.getGPUs(ga.gCache.gpuGetFilter, &ga.gCache.metricsCache)
	assert.NilError(t, err)
	ga.initGPUMetadata(getResp.GetResponse())
	ga.hrSnapshot.Store(&hrGPUSnap{gpus: getResp.GetResponse(), partitionMap: pm})
	ga.hrLabelsByUUID = make(map[string]prometheus.Labels)
	ga.hrGPUIDByUUID = make(map[string]string)
	var src *workloadSource
	if interval <= 0 {
		src = manualSource(e)
	} else {
		src = runningSource(t, e, interval)
	}
	seedWls, _ := ga.currentWorkloads()
	for _, g := range getResp.GetResponse() {
		guid := getGPUUUID(g)
		ga.hrLabelsByUUID[guid] = prometheus.Labels(ga.populateLabelsFromGPU(seedWls, g, pm))
		ga.hrGPUIDByUUID[guid] = getGPUInstanceIDString(g)
	}
	c := newHRWorkloadLabelCache(ga)
	ga.hrWLCache = c
	c.refreshOnce()
	return c, src
}

// freshLabels builds the hr label map from scratch for wls (the expected
// content of a cache).
func (e *workloadEnv) freshLabels(wls map[string]scheduler.Workload, key func(*amdgpu.GPU) string) map[string]prometheus.Labels {
	snap := e.gpu.hrSnapshot.Load()
	out := map[string]prometheus.Labels{}
	for _, g := range snap.gpus {
		out[key(g)] = prometheus.Labels(e.gpu.populateLabelsFromGPU(wls, g, snap.partitionMap))
	}
	return out
}

// useSlurmDir makes the agent's slurm client constructor watch dir instead of
// the system job directory.
func useSlurmDir(t *testing.T, dir string) {
	t.Helper()
	orig := newSlurmScheduler
	newSlurmScheduler = func(ctx context.Context) (scheduler.SchedulerClient, error) {
		return scheduler.NewSlurmClientForDir(ctx, dir)
	}
	t.Cleanup(func() { newSlurmScheduler = orig })
}

// sourceGoroutines counts running workloadSource.run goroutines (immune to
// unrelated goroutines starting or exiting).
func sourceGoroutines() int {
	buf := make([]byte, 8<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "(*workloadSource).run(")
}

// sourceGoroutinesOf counts running run goroutines of s.
func sourceGoroutinesOf(s *workloadSource) int {
	buf := make([]byte, 8<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), fmt.Sprintf("(*workloadSource).run(%p", s))
}

// waitWithin polls cond every 200us until it holds or d elapses.
func waitWithin(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Microsecond)
	}
}

// jobID returns the slurm job id of key in wls ("" when absent).
func jobID(wls map[string]scheduler.Workload, key string) string {
	if j, ok := wls[key].Info.(scheduler.JobInfo); ok {
		return j.Id
	}
	return ""
}
