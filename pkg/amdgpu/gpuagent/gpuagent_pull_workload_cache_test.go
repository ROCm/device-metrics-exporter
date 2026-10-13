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

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

// pullWorkloadConfig enables the GPU_UUID + JOB_ID labels and the package-power
// field, so a populated gpu_package_power gauge carries a workload-derived job_id
// label that reveals which workload source the pull path consumed. Non-high-res
// (no HighResolution block), so the pull path runs getMetricsAll per scrape.
const pullWorkloadConfig = `{"GPUConfig":{"Labels":["GPU_UUID","JOB_ID"],"Fields":["GPU_PACKAGE_POWER"]}}`

// newPullWorkloadTestClient builds a non-high-res GPU client wired to the shared
// gpu mock and a slurm scheduler mock, with workload labels enabled. The slurm
// mock requires Close() exactly once, so callers must defer ga.Close().
func newPullWorkloadTestClient(t *testing.T) (*GPUAgentClient, *GPUAgentGPUClient, *metricsutil.MetricsHandler) {
	t.Helper()
	pmh := newHighResMetricsHandler(t, pullWorkloadConfig)
	ga := NewAgent(pmh, WithK8sClient(nil), WithK8sSchedulerClient(nil), WithGPUMonitoring(true))
	ga.Init()
	ga.isKubernetes = false
	ga.slurmScheduler = newSlurmMockClient()

	var gpu *GPUAgentGPUClient
	for _, c := range ga.clients {
		if c.GetDeviceType() == globals.GPUDevice {
			gpu = c.(*GPUAgentGPUClient)
			break
		}
	}
	assert.Assert(t, gpu != nil, "no GPU client found")
	gpu.gpuclient = gpuMockCl
	gpu.evtclient = eventMockCl
	assert.NilError(t, gpu.InitConfigs())

	// Seed gpuIDMap so getWorkloadInfo -> GetGPUMeta resolves (production seeds
	// this in UpdateStaticMetrics before scrapes). Seeding alone applies no
	// workload labels, so the only job_id in the pull output is the cache's.
	getResp, _, err := gpu.getGPUs(gpu.gCache.gpuGetFilter, &gpu.gCache.metricsCache)
	assert.NilError(t, err)
	gpu.initGPUMetadata(getResp.GetResponse())
	return ga, gpu, pmh
}

// TestPullWorkloadCacheServesAfterRefresh asserts get() misses before the first
// refresh and, after refreshOnce, returns exactly the scheduler's workload map
// (non-empty, so the slurm labels are really present).
func TestPullWorkloadCacheServesAfterRefresh(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga, gpu, _ := newPullWorkloadTestClient(t)
	defer ga.Close()

	cache := newPullWorkloadCache(gpu)

	_, ok := cache.get()
	assert.Assert(t, !ok, "get must miss before first refresh")

	cache.refreshOnce(context.Background())
	got, ok := cache.get()
	assert.Assert(t, ok, "get must hit after refresh")

	want, err := ga.ListWorkloads()
	assert.NilError(t, err)
	assert.Assert(t, len(want) > 0, "slurm mock produced no workloads; test is vacuous")
	assert.Assert(t, reflect.DeepEqual(got, want),
		"cache map != scheduler map:\n got=%v\nwant=%v", got, want)
}

// TestGetMetricsAllUsesPullCache proves the pull path reads workloads from the
// cache, not the synchronous scheduler. The cache is seeded with a sentinel job_id
// that differs from the slurm mock's; after getMetricsAll the emitted
// gpu_package_power gauge must carry the sentinel, never the scheduler value.
func TestGetMetricsAllUsesPullCache(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga, gpu, pmh := newPullWorkloadTestClient(t)
	defer ga.Close()

	// Both mock GPUs have index 0 -> workload key "0". Seed a sentinel the
	// scheduler never returns so the source is unambiguous.
	const sentinel = "CACHED_JOB_ID"
	sentinelWls := map[string]scheduler.Workload{
		"0": {Type: scheduler.Slurm, Info: scheduler.JobInfo{Id: sentinel}},
	}
	c := newPullWorkloadCache(gpu)
	c.wls.Store(&sentinelWls)
	gpu.pullWLCache.Store(c)

	// Non-high-res /metrics is served from the snapshot store (the single serving
	// path after the flag removal), so populate the GaugeVecs via the pull path and
	// mirror them into the store the registry's collector reads.
	assert.NilError(t, gpu.getMetricsAll(context.Background()))
	assert.NilError(t, gpu.publishGPUMetricsToStore(pmh.GetStore()))

	mfs, err := pmh.GetRegistry().Gather()
	assert.NilError(t, err)
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		assert.NilError(t, enc.Encode(mf))
	}
	out := buf.String()
	assert.Assert(t, strings.Contains(out, sentinel),
		"pull path did not use cached workloads; %q absent from /metrics", sentinel)
	assert.Assert(t, !strings.Contains(out, "SLURM_JOB_ID0"),
		"pull path used the synchronous scheduler instead of the cache")
}
