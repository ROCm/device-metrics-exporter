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

// GPUOP-1140 test helpers: a ChangeNotifier slurm fake with injection hooks, a
// fake kubelet PodResources server, and source/high-res setup over the oracle
// fixtures (oracle_gpuop1140_test.go).

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"gotest.tools/assert"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

// notifySched is a slurm-like SchedulerClient that implements
// scheduler.ChangeNotifier. hook (if set) runs at "changed" (after Changed()
// picked its channel), "version" (after Version() read the value) and "list"
// (after ListWorkloads copied), so tests can inject a bump at those points.
type notifySched struct {
	mu      sync.Mutex
	wls     map[string]scheduler.Workload
	err     error
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

func (f *notifySched) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
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
	err := f.err
	f.mu.Unlock()
	f.fire("list")
	if err != nil {
		return nil, err
	}
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
func manualSource(e *oracleEnv) *workloadSource {
	s := newWorkloadSource(e.ga, time.Hour)
	s.refresh(true)
	e.gpu.wlSource.Store(s)
	return s
}

// runningSource starts the real source with the given interval and stops it
// at test end (the source otherwise lives for the process).
func runningSource(t testing.TB, e *oracleEnv, interval time.Duration) *workloadSource {
	t.Helper()
	e.gpu.wlInterval = interval
	s := e.gpu.ensureWorkloadSource()
	t.Cleanup(s.stop)
	return s
}

// startHRSource mirrors StartHighRes' store-serving label setup on the
// post-change code: metadata, snapshot, shared source, seed from
// currentWorkloads, then the hr label cache (not started; tests drive it).
// interval <= 0 installs a manual source (no goroutine) for tests that call
// refresh themselves; refresh is refresher-owned and must not run twice at once.
func (e *oracleEnv) startHRSource(t testing.TB, interval time.Duration) (*hrWorkloadLabelCache, *workloadSource) {
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
	c.refreshOnce(context.Background())
	return c, src
}

// freshLabels builds the hr label map from scratch for wls (oracle of a cache).
func (e *oracleEnv) freshLabels(wls map[string]scheduler.Workload, key func(*amdgpu.GPU) string) map[string]prometheus.Labels {
	snap := e.gpu.hrSnapshot.Load()
	out := map[string]prometheus.Labels{}
	for _, g := range snap.gpus {
		out[key(g)] = prometheus.Labels(e.gpu.populateLabelsFromGPU(wls, g, snap.partitionMap))
	}
	return out
}

// writeSlurmJob writes a job file named name for gpus (comma list).
func writeSlurmJob(t testing.TB, dir, name, id, gpus string) {
	t.Helper()
	b, err := json.Marshal(map[string]string{"SLURM_JOB_ID": id, "SLURM_JOB_USER": "u" + id,
		"SLURM_JOB_PARTITION": "gpu", "SLURM_CLUSTER_NAME": "c1", "SLURM_JOB_ACCOUNT": "a" + id,
		"CUDA_VISIBLE_DEVICES": gpus})
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, name), b, 0o644))
}

// sourceGoroutines counts running workloadSource.run goroutines (immune to
// unrelated goroutines starting or exiting).
func sourceGoroutines() int {
	buf := make([]byte, 8<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "(*workloadSource).run(")
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
