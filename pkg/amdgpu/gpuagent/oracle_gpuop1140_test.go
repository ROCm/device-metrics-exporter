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

// GPUOP-1140 oracle fixtures: the fixture and rendering helpers of the base
// oracle capture harness (GPUOP-1141 Step 0), copied unchanged. The goldens in
// testdata/golden-base were captured from base b2efd1b32 with these helpers (the
// only addition is a t.Cleanup that stops the client's workload source) and
// must never be regenerated from this tree, so the base-only capture tests
// (TestGoldenCapture, TestCharacterizeBase) are not carried here; they live in
// test-plans/oracles/GPUOP-1140/harness. Parity tests that use these helpers
// are in gpuagent_workload_parity_test.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"go.uber.org/mock/gomock"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

const oracleGPUs = 4

// oracleLabels enables every workload label plus the GPU_UUID key.
const oracleLabels = `"Labels":["GPU_UUID","POD","NAMESPACE","CONTAINER",` +
	`"JOB_ID","JOB_USER","JOB_PARTITION","JOB_ACCOUNT","CLUSTER_NAME"]`

const oraclePullConfig = `{"GPUConfig":{` + oracleLabels + `,"Fields":["GPU_PACKAGE_POWER"]}}`

const oracleHRConfig = `{"CommonConfig":{"HighResolution":{"Enable":true}},` +
	`"GPUConfig":{` + oracleLabels + `,"Fields":["GPU_PACKAGE_POWER"]}}`

// oracleSched is a hand-written SchedulerClient fake (gomock's Close().Times(1)
// does not fit reconnect scenarios). Settable map and error; counts calls.
type oracleSched struct {
	mu     sync.Mutex
	typ    scheduler.SchedulerType
	wls    map[string]scheduler.Workload
	err    error
	lists  int
	closes int
}

func (f *oracleSched) set(wls map[string]scheduler.Workload, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wls, f.err = wls, err
}

func (f *oracleSched) ListWorkloads() (map[string]scheduler.Workload, error) {
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

func (f *oracleSched) CheckExportLabels(map[string]bool) bool { return true }

func (f *oracleSched) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}

func (f *oracleSched) Type() scheduler.SchedulerType { return f.typ }

func (f *oracleSched) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists, f.closes
}

// oracleGPUResp is a deterministic GPU list: fixed UUIDs, index i, PCIe
// 0000:0<i+1>:00.0, DRM card 1+i, render 128+i.
func oracleGPUResp(n int) *amdgpu.GPUGetResponse {
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

func oraclePod(i int) scheduler.Workload {
	return scheduler.Workload{Type: scheduler.Kubernetes, Info: scheduler.PodResourceInfo{
		Pod: fmt.Sprintf("pod%d", i), Namespace: fmt.Sprintf("ns%d", i), Container: fmt.Sprintf("ctr%d", i)}}
}

func oracleJob(id, user string) scheduler.Workload {
	return scheduler.Workload{Type: scheduler.Slurm, Info: scheduler.JobInfo{
		Id: id, User: user, Partition: "gpu", Cluster: "c1", Account: "acct-" + user}}
}

// oracleFixture: k8s and slurm parts (nil = no such scheduler).
type oracleFixture struct {
	name  string
	k8s   map[string]scheduler.Workload
	slurm map[string]scheduler.Workload
}

func oracleFixtures() []oracleFixture {
	hostile := strings.Repeat("x", 4096)
	return []oracleFixture{
		{name: "k8s", k8s: map[string]scheduler.Workload{
			"0000:01:00.0": oraclePod(0), "0000:02:00.0": oraclePod(1), "0000:09:00.0": oraclePod(9)}},
		{name: "slurm", slurm: map[string]scheduler.Workload{
			"0": oracleJob("100", "alice"), "2": oracleJob("102", "bob"), "7": oracleJob("107", "zed")}},
		// mixed: k8s on GPUs 0,1 (pcie key) and key "3"; slurm on "1" (same GPU
		// as pod1, different key) and "3" (same key: slurm overwrites k8s).
		{name: "mixed",
			k8s: map[string]scheduler.Workload{
				"0000:01:00.0": oraclePod(0), "0000:02:00.0": oraclePod(1), "3": oraclePod(3)},
			slurm: map[string]scheduler.Workload{
				"1": oracleJob("101", "carol"), "3": oracleJob("103", "dave")}},
		{name: "hostile", slurm: map[string]scheduler.Workload{
			"0": {Type: scheduler.Slurm, Info: scheduler.JobInfo{
				Id: `9"9`, User: `back\slash`, Partition: "new\nline", Cluster: "tab\there", Account: "unicodé✓"}},
			"1": {Type: scheduler.Slurm, Info: scheduler.JobInfo{
				Id: "1", User: hostile, Partition: `x","job_user":"alice`, Cluster: "", Account: "\x00nul"}},
		}},
	}
}

type oracleEnv struct {
	ga    *GPUAgentClient
	gpu   *GPUAgentGPUClient
	k8s   *oracleSched
	slurm *oracleSched
}

// newOracleEnv builds a GPU client over deterministic mocks with the given
// schedulers wired the way the exporter does (k8s injected, slurm created by
// Init; here both are fakes set on the base fields).
func newOracleEnv(t testing.TB, cfg string, fx oracleFixture) *oracleEnv {
	t.Helper()
	logger.Init(true)
	t.Setenv("DS_NODE_NAME", "oracle-node")
	ctl := gomock.NewController(t)
	gpuMock := mock_gen.NewMockGPUSvcClient(ctl)
	evtMock := mock_gen.NewMockEventSvcClient(ctl)
	gpuMock.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(oracleGPUResp(oracleGPUs), nil).AnyTimes()
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
	env := &oracleEnv{ga: ga}
	ga.isKubernetes = false
	if fx.k8s != nil {
		env.k8s = &oracleSched{typ: scheduler.Kubernetes, wls: fx.k8s}
		ga.isKubernetes = true
		ga.k8sScheduler = env.k8s
	}
	if fx.slurm != nil {
		env.slurm = &oracleSched{typ: scheduler.Slurm, wls: fx.slurm}
		ga.slurmScheduler = env.slurm
	}
	for _, c := range ga.clients {
		if c.GetDeviceType() == globals.GPUDevice {
			env.gpu = c.(*GPUAgentGPUClient)
		}
	}
	assert.Assert(t, env.gpu != nil)
	t.Cleanup(env.gpu.stopWorkloadSource) // GPUOP-1142: sources live for the process
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
func (e *oracleEnv) render(t *testing.T) string {
	t.Helper()
	mfs, err := e.gpu.gpuHandler.mh.GetRegistry().Gather()
	assert.NilError(t, err)
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

// renderPull runs one pull-path scrape (getMetricsAll + store publish).
func (e *oracleEnv) renderPull(t *testing.T) string {
	t.Helper()
	assert.NilError(t, e.gpu.getMetricsAll(context.Background()))
	assert.NilError(t, e.gpu.publishGPUMetricsToStore(e.gpu.gpuHandler.mh.GetStore()))
	return e.render(t)
}

// startHR mirrors StartHighRes' store-serving label setup (subscribe-time seed
// from ListWorkloads + hr workload-label cache) without the gpuagent watch.
func (e *oracleEnv) startHR(t *testing.T) *workloadLabelCache {
	t.Helper()
	ga := e.gpu
	getResp, pm, err := ga.getGPUs(ga.gCache.gpuGetFilter, &ga.gCache.metricsCache)
	assert.NilError(t, err)
	ga.hrSnapshot.Store(&hrGPUSnap{gpus: getResp.GetResponse(), partitionMap: pm})
	ga.initGPUMetadata(getResp.GetResponse())
	ga.hrLabelsByUUID = make(map[string]prometheus.Labels)
	ga.hrGPUIDByUUID = make(map[string]string)
	seedWls, _ := ga.gpuHandler.ListWorkloads()
	for _, g := range getResp.GetResponse() {
		guid := getGPUUUID(g)
		ga.hrLabelsByUUID[guid] = prometheus.Labels(ga.populateLabelsFromGPU(seedWls, g, pm))
		ga.hrGPUIDByUUID[guid] = getGPUInstanceIDString(g)
	}
	c := newHRWorkloadLabelCache(ga)
	ga.hrWLCache = c
	c.refreshOnce()
	return c
}

// renderHR applies one full high-res frame (power attr for every GPU) and
// gathers the store-served /metrics.
func (e *oracleEnv) renderHR(t *testing.T) string {
	t.Helper()
	var gpus [][]byte
	for _, g := range oracleGPUResp(oracleGPUs).Response {
		gpus = append(gpus, g.Spec.Id)
	}
	id := highResWatchID
	w := &amdgpu.GPUWatch{Spec: &amdgpu.GPUWatchSpec{Id: id[:], Attribute: highResWatchAttrs},
		Stats: &amdgpu.GPUWatchStats{}}
	for _, g := range gpus {
		w.Stats.GPUWatchAttr = append(w.Stats.GPUWatchAttr, &amdgpu.GPUWatchAttrs{GPU: g,
			Attr: []*amdgpu.GPUWatchAttr{hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 200)}})
	}
	e.gpu.applyWatchSnapshot(w)
	return e.render(t)
}

// labelDump is a deterministic JSON of the hr cache's per-UUID labels.
func labelDump(t *testing.T, m *map[string]prometheus.Labels) string {
	t.Helper()
	if m == nil {
		return "null\n"
	}
	b, err := json.MarshalIndent(*m, "", "  ") // encoding/json sorts map keys
	assert.NilError(t, err)
	return string(b) + "\n"
}

var errOracleKubelet = errors.New("rpc error: code = Unavailable desc = connection error: " +
	"desc = \"transport: Error while dialing: dial unix /var/lib/kubelet/pod-resources/kubelet.sock: " +
	"connect: no such file or directory\"")
