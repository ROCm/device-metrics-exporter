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
	"maps"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

// workloadLabelCache keeps a stream's per-GPU labels, workload labels
// included, in sync with the pull path; it rebuilds off the stream's hot path
// and only when an input changed.
type workloadLabelCache struct {
	ga       *GPUAgentGPUClient
	keys     labelCacheKey
	labels   atomic.Pointer[map[string]prometheus.Labels]
	interval time.Duration
	built    labelInputs  // inputs labels was built from (cache goroutine)
	rebuilds atomic.Int64 // published rebuilds (test observability)
}

// labelCacheKey is a workloadLabelCache's key and per-tick upkeep.
type labelCacheKey interface {
	// key returns gpu's cache key; "" leaves the GPU out.
	key(gpu *amdgpu.GPU) string
	// tick runs after the refresh on every ticker wake.
	tick(ga *GPUAgentGPUClient)
}

// gpuUUIDKey keys the high-res GPUWatch stream's labels by GPU UUID.
type gpuUUIDKey struct{}

func (gpuUUIDKey) key(gpu *amdgpu.GPU) string {
	return getGPUUUID(gpu)
}

func (gpuUUIDKey) tick(*GPUAgentGPUClient) {}

// renderIDKey keys the profiler stream's labels by DRM render id.
type renderIDKey struct{}

func (renderIDKey) key(gpu *amdgpu.GPU) string {
	return getGPURenderId(gpu)
}

// tick clears stale gpu_prof_* values once the profiler is disabled.
func (renderIDKey) tick(ga *GPUAgentGPUClient) {
	ga.dropProfilerFieldsIfDisabled()
}

// hrWorkloadLabelRefreshInterval mirrors the 3s runtime config reload cadence:
// workload labels cannot be fresher than the scheduler data they derive from.
const hrWorkloadLabelRefreshInterval = 3 * time.Second

// newHRWorkloadLabelCache returns the high-res stream's cache.
func newHRWorkloadLabelCache(ga *GPUAgentGPUClient) *workloadLabelCache {
	return &workloadLabelCache{ga: ga, keys: gpuUUIDKey{}, interval: hrWorkloadLabelRefreshInterval}
}

// newProfilerLabelCache returns the profiler stream's cache.
func newProfilerLabelCache(ga *GPUAgentGPUClient) *workloadLabelCache {
	return &workloadLabelCache{ga: ga, keys: renderIDKey{}, interval: hrWorkloadLabelRefreshInterval}
}

// labelInputs are the inputs a label cache was built from.
type labelInputs struct {
	wlGen    uint64     // workloadSnap.gen (0 = no snapshot yet)
	snap     *hrGPUSnap // ga.hrSnapshot.Load()
	labelGen uint64     // ga.labelInputGen.Load()
}

// labelSourceInputs reads labelInputGen before the snapshot, so a concurrent
// config write is seen on the next wake.
func (ga *GPUAgentGPUClient) labelSourceInputs(snap *hrGPUSnap) (labelInputs, *workloadSnap) {
	in := labelInputs{snap: snap, labelGen: ga.labelInputGen.Load()}
	ws := ga.ensureWorkloadSource().load()
	in.wlGen = ws.gen
	return in, ws
}

// rebuildLabelMap builds key -> labels, returning prev itself when nothing
// changed so pointer comparisons see no change.
func (ga *GPUAgentGPUClient) rebuildLabelMap(prev *map[string]prometheus.Labels,
	gpus []*amdgpu.GPU, pm map[string]*amdgpu.GPU, wls map[string]scheduler.Workload,
	keys labelCacheKey,
) *map[string]prometheus.Labels {
	next := make(map[string]prometheus.Labels, len(gpus))
	same := prev != nil
	for _, gpu := range gpus {
		k := keys.key(gpu)
		if k == "" {
			continue
		}
		l := prometheus.Labels(ga.populateLabelsFromGPU(wls, gpu, pm))
		if prev != nil {
			if old, ok := (*prev)[k]; ok && maps.Equal(old, l) {
				l = old
			} else {
				same = false
			}
		}
		next[k] = l
	}
	if same && len(next) == len(*prev) {
		return prev
	}
	return &next
}

// refreshOnce rebuilds the label map when an input changed. Published maps are
// never mutated, so get() is lock free.
func (c *workloadLabelCache) refreshOnce() {
	// Reuse the subscribe-time snapshot (sole-amd-smi-caller invariant); only the
	// scheduler-sourced workload labels change at runtime.
	snap := c.ga.hrSnapshot.Load()
	if snap == nil || len(snap.gpus) == 0 {
		return
	}
	in, ws := c.ga.labelSourceInputs(snap)
	prev := c.labels.Load()
	if prev != nil && in == c.built {
		return
	}
	next := c.ga.rebuildLabelMap(prev, snap.gpus, snap.partitionMap, ws.wls, c.keys)
	if next != prev {
		c.labels.Store(next)
		c.rebuilds.Add(1)
	}
	c.built = in
}

// start seeds the cache synchronously, then refreshes on source changes and on
// the ticker until ctx is done. Run it in its own goroutine.
func (c *workloadLabelCache) start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = c.interval
	}
	src := c.ga.ensureWorkloadSource()
	ch := src.Changed()
	c.refreshOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			ch = src.Changed()
			c.refreshOnce()
		case <-ticker.C:
			c.refreshOnce()
			c.keys.tick(c.ga)
		}
	}
}

// get returns the latest cached labels for key without locking.
func (c *workloadLabelCache) get(key string) (prometheus.Labels, bool) {
	m := c.labels.Load()
	if m == nil {
		return nil, false
	}
	l, ok := (*m)[key]
	return l, ok
}
