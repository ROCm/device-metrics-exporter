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

// hrWorkloadLabelCache keeps the high-res stream's per-GPU label set 1-1 with
// the pull path, including the dynamic pod/slurm workload labels. The stream
// caches a static label set once at subscribe (wls=nil), so streamed samples
// lose workload labels and go stale; this cache rebuilds the full set off the
// stream's hot path from the shared workload source, and only when an input
// changed.
type hrWorkloadLabelCache struct {
	ga       *GPUAgentGPUClient
	labels   atomic.Pointer[map[string]prometheus.Labels]
	interval time.Duration
	built    labelInputs  // inputs labels was built from (cache goroutine)
	rebuilds atomic.Int64 // published rebuilds (test observability)
}

// hrWorkloadLabelRefreshInterval mirrors the 3s runtime config reload cadence:
// workload labels cannot be fresher than the scheduler data they derive from.
const hrWorkloadLabelRefreshInterval = 3 * time.Second

func newHRWorkloadLabelCache(ga *GPUAgentGPUClient) *hrWorkloadLabelCache {
	return &hrWorkloadLabelCache{ga: ga, interval: hrWorkloadLabelRefreshInterval}
}

// labelInputs are the inputs a label cache was built from. Equal inputs mean
// the labels are current.
type labelInputs struct {
	wlGen    uint64     // workloadSnap.gen (0 = no snapshot yet)
	snap     *hrGPUSnap // ga.hrSnapshot.Load()
	labelGen uint64     // ga.labelInputGen.Load()
}

// labelSourceInputs reads a label cache's inputs: labelInputGen first, then the
// workload snapshot, so a concurrent config write is seen on the next wake
// rather than lost.
func (ga *GPUAgentGPUClient) labelSourceInputs(snap *hrGPUSnap) (labelInputs, *workloadSnap) {
	in := labelInputs{snap: snap, labelGen: ga.labelInputGen.Load()}
	ws := ga.ensureWorkloadSource().load()
	in.wlGen = ws.gen
	return in, ws
}

// rebuildLabelMap builds key -> labels for gpus and reuses each prev entry
// that is maps.Equal to the new one. It returns prev itself when no entry
// changed and the key set is the same, so readers comparing pointers see no
// change.
func (ga *GPUAgentGPUClient) rebuildLabelMap(prev *map[string]prometheus.Labels,
	gpus []*amdgpu.GPU, pm map[string]*amdgpu.GPU, wls map[string]scheduler.Workload,
	key func(*amdgpu.GPU) string,
) *map[string]prometheus.Labels {
	next := make(map[string]prometheus.Labels, len(gpus))
	same := prev != nil
	for _, gpu := range gpus {
		k := key(gpu)
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

// refreshOnce rebuilds the per-UUID label map (same inputs as the pull path)
// when an input changed, and swaps it in only if some GPU's labels changed.
// Published maps are never mutated, so get() is lock-free and never observes
// a torn map.
func (c *hrWorkloadLabelCache) refreshOnce(_ context.Context) {
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
	next := c.ga.rebuildLabelMap(prev, snap.gpus, snap.partitionMap, ws.wls, getGPUUUID)
	if next != prev {
		c.labels.Store(next)
		c.rebuilds.Add(1)
	}
	c.built = in
}

// start seeds the cache synchronously, then refreshes on a source change or on
// the ticker (config / snapshot changes) until ctx is done. The change channel
// is obtained before each refresh, so a change racing it is not lost. The
// synchronous seed makes the first streamed sample workload-aware; run start
// in its own goroutine.
func (c *hrWorkloadLabelCache) start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = c.interval
	}
	src := c.ga.ensureWorkloadSource()
	ch := src.Changed()
	c.refreshOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			ch = src.Changed()
			c.refreshOnce(ctx)
		case <-ticker.C:
			c.refreshOnce(ctx)
		}
	}
}

// get returns the latest cached labels for a GPU UUID without locking.
func (c *hrWorkloadLabelCache) get(uuid string) (prometheus.Labels, bool) {
	m := c.labels.Load()
	if m == nil {
		return nil, false
	}
	l, ok := (*m)[uuid]
	return l, ok
}
