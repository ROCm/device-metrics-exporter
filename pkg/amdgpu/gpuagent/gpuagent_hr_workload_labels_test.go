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
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
)

// hrWorkloadLabelConfig enables the GPU_UUID key (what samples are keyed on) and
// the slurm workload labels, so the resolved label set is non-trivially
// workload-dependent and parity with the pull path is meaningful.
const hrWorkloadLabelConfig = `{"CommonConfig":{"HighResolution":{"Enable":true}},` +
	`"GPUConfig":{"Labels":["GPU_UUID","JOB_ID","JOB_USER","JOB_PARTITION","CLUSTER_NAME"]}}`

// newHRWorkloadTestClient builds a GPU client wired to the shared gpu mock and a
// slurm scheduler mock, with workload labels enabled, and seeds gpuIDMap the way
// StartHighRes does (initGPUMetadata) so workload keys resolve. The slurm
// mock requires Close() exactly once, so callers must defer ga.Close().
func newHRWorkloadTestClient(t *testing.T) (*GPUAgentClient, *GPUAgentGPUClient) {
	t.Helper()
	hrmh := newHighResMetricsHandler(t, hrWorkloadLabelConfig)
	ga := NewAgent(hrmh, WithK8sClient(nil), WithK8sSchedulerClient(nil), WithGPUMonitoring(true))
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
	// this in StartHighRes before the cache starts).
	getResp, pm, err := gpu.getGPUs(gpu.gCache.gpuGetFilter, &gpu.gCache.metricsCache)
	assert.NilError(t, err)
	gpu.initGPUMetadata(getResp.GetResponse())
	// Seed the subscribe-time snapshot the label cache reads (production: StartHighRes).
	gpu.hrSnapshot.Store(&hrGPUSnap{gpus: getResp.GetResponse(), partitionMap: pm})
	return ga, gpu
}

// TestHRWorkloadLabelCacheParity asserts the cache's per-UUID labels are
// identical to a direct pull-path populateLabelsFromGPU(wls, ...) call, and that
// the slurm workload labels are actually present (i.e. wls was not nil).
func TestHRWorkloadLabelCacheParity(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga, gpu := newHRWorkloadTestClient(t)
	defer ga.Close()

	cache := newHRWorkloadLabelCache(gpu)
	cache.refreshOnce(context.Background())

	// Independently rebuild the expected label set exactly as the pull path does.
	wls, err := ga.ListWorkloads()
	assert.NilError(t, err)
	getResp, pm, err := gpu.getGPUs(gpu.gCache.gpuGetFilter, &gpu.gCache.metricsCache)
	assert.NilError(t, err)
	assert.Assert(t, len(getResp.GetResponse()) > 0, "fixture produced no GPUs")

	jobIDKey := strings.ToLower(exportermetrics.MetricLabel_JOB_ID.String())
	sawWorkloadLabel := false
	for _, g := range getResp.GetResponse() {
		uuidStr := getGPUUUID(g)
		want := prometheus.Labels(gpu.populateLabelsFromGPU(wls, g, pm))
		got, ok := cache.get(uuidStr)
		assert.Assert(t, ok, "cache missing labels for %s", uuidStr)
		assert.Assert(t, reflect.DeepEqual(map[string]string(got), map[string]string(want)),
			"cache labels != pull-path labels for %s:\n got=%v\nwant=%v", uuidStr, got, want)
		// Both mock GPUs have index 0 -> slurm workload "0" -> SLURM_JOB_ID0.
		if got[jobIDKey] == "SLURM_JOB_ID0" {
			sawWorkloadLabel = true
		}
	}
	assert.Assert(t, sawWorkloadLabel,
		"no GPU carried the slurm job_id label; workload labels were dropped (wls=nil)")
}

// TestHRWorkloadLabelCacheAtomicSwap asserts each refresh publishes a brand-new
// map (copy-on-write, never mutated in place) and get() returns the latest.
func TestHRWorkloadLabelCacheAtomicSwap(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga, gpu := newHRWorkloadTestClient(t)
	defer ga.Close()

	cache := newHRWorkloadLabelCache(gpu)

	// Empty before the first refresh.
	_, ok := cache.get("does-not-exist")
	assert.Assert(t, !ok, "get must miss before first refresh")

	cache.refreshOnce(context.Background())
	p1 := cache.labels.Load()
	assert.Assert(t, p1 != nil, "labels nil after first refresh")

	cache.refreshOnce(context.Background())
	p2 := cache.labels.Load()
	assert.Assert(t, p2 != nil && p1 != p2, "refresh must swap in a new map pointer")

	// Unknown UUID always misses; known UUID always hits.
	_, ok = cache.get("does-not-exist")
	assert.Assert(t, !ok, "get must miss for unknown uuid")
	for uuidStr := range *p2 {
		_, ok := cache.get(uuidStr)
		assert.Assert(t, ok, "get must hit for known uuid %s", uuidStr)
	}
}

// TestHRWorkloadLabelCacheConcurrentGet drives many lock-free get() readers
// against a single refresher. Under -race this gates that the atomic swap never
// exposes a torn or partially-built map.
func TestHRWorkloadLabelCacheConcurrentGet(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga, gpu := newHRWorkloadTestClient(t)
	defer ga.Close()

	cache := newHRWorkloadLabelCache(gpu)
	cache.refreshOnce(context.Background())
	// Grab a stable UUID to read throughout.
	var uuidStr string
	for u := range *cache.labels.Load() {
		uuidStr = u
		break
	}
	assert.Assert(t, uuidStr != "", "no seeded uuid")

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			cache.refreshOnce(ctx)
		}
	}()

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				if l, ok := cache.get(uuidStr); ok {
					// A torn map would surface here as a missing/empty entry.
					_ = l[strings.ToLower(exportermetrics.GPUMetricLabel_GPU_UUID.String())]
				}
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()

	// After the storm, the UUID is still resolvable with the gpu_uuid label set.
	l, ok := cache.get(uuidStr)
	assert.Assert(t, ok, "uuid vanished after concurrent refresh")
	assert.Equal(t, l[strings.ToLower(exportermetrics.GPUMetricLabel_GPU_UUID.String())], uuidStr)
}

// TestHRWorkloadLabelCacheStartSeeds asserts start() seeds synchronously (the
// first streamed sample is workload-aware without waiting an interval) and stops
// on ctx cancel.
func TestHRWorkloadLabelCacheStartSeeds(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga, gpu := newHRWorkloadTestClient(t)
	defer ga.Close()

	cache := newHRWorkloadLabelCache(gpu)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		cache.start(ctx, time.Millisecond)
		close(done)
	}()

	// start() seeds before entering its ticker loop; poll briefly for it.
	populated := false
	for i := 0; i < 200 && !populated; i++ {
		if m := cache.labels.Load(); m != nil && len(*m) > 0 {
			populated = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	assert.Assert(t, populated, "start() did not seed the label cache")
}
