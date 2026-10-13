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
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// hrWorkloadLabelCache keeps the high-res stream's per-GPU label set 1-1 with
// the pull path, including the dynamic pod/slurm workload labels. The stream
// caches a static label set once at subscribe (wls=nil), so streamed samples
// lose workload labels and go stale; this cache rebuilds the full set off the
// stream's hot path. ListWorkloads() can block on the scheduler, so it runs
// here rather than in applyWatchSnapshot.
type hrWorkloadLabelCache struct {
	ga       *GPUAgentGPUClient
	labels   atomic.Pointer[map[string]prometheus.Labels]
	interval time.Duration
}

// hrWorkloadLabelRefreshInterval mirrors the 3s runtime config reload cadence:
// workload labels cannot be fresher than the scheduler data they derive from.
const hrWorkloadLabelRefreshInterval = 3 * time.Second

func newHRWorkloadLabelCache(ga *GPUAgentGPUClient) *hrWorkloadLabelCache {
	return &hrWorkloadLabelCache{ga: ga, interval: hrWorkloadLabelRefreshInterval}
}

// refreshOnce rebuilds the full per-UUID label map (same inputs as the pull
// path) and atomically swaps it in. The new map is never mutated after the
// Store, so get() is lock-free and never observes a torn map.
func (c *hrWorkloadLabelCache) refreshOnce(_ context.Context) {
	// Reuse the subscribe-time snapshot (sole-amd-smi-caller invariant); only the
	// scheduler-sourced workload labels change at runtime.
	snap := c.ga.hrSnapshot.Load()
	if snap == nil || len(snap.gpus) == 0 {
		return
	}
	gpus := snap.gpus
	partitionMap := snap.partitionMap
	wls, _ := c.ga.gpuHandler.ListWorkloads()
	next := make(map[string]prometheus.Labels, len(gpus))
	for _, gpu := range gpus {
		uuidStr := getGPUUUID(gpu)
		if uuidStr == "" {
			continue
		}
		next[uuidStr] = prometheus.Labels(c.ga.populateLabelsFromGPU(wls, gpu, partitionMap))
	}
	c.labels.Store(&next)
}

// start seeds the cache synchronously, then refreshes on a ticker until ctx is
// done. The synchronous seed makes the first streamed sample workload-aware
// rather than waiting a full interval; run start in its own goroutine.
func (c *hrWorkloadLabelCache) start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = c.interval
	}
	c.refreshOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
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
