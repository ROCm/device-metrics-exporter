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

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

// pullWorkloadCache keeps the non-high-res pull path's scheduler workload map off
// the /metrics scrape hot path. ListWorkloads() can block on the K8s/slurm
// scheduler, so refreshing it on a ticker lets getMetricsAll read the last
// snapshot lock-free. Only built/started in flag-on non-high-res store-serving
// mode, so flag-off /metrics keeps the per-scrape ListWorkloads call and stays
// byte-identical. Shares the 3s refresh cadence with hrWorkloadLabelCache.
type pullWorkloadCache struct {
	ga       *GPUAgentGPUClient
	wls      atomic.Pointer[map[string]scheduler.Workload]
	interval time.Duration
}

func newPullWorkloadCache(ga *GPUAgentGPUClient) *pullWorkloadCache {
	return &pullWorkloadCache{ga: ga, interval: hrWorkloadLabelRefreshInterval}
}

// refreshOnce fetches the workload map and atomically swaps it in. The stored map
// is never mutated after the Store, so get() is lock-free and never observes a
// torn map. A fetch error leaves the previous snapshot in place.
func (c *pullWorkloadCache) refreshOnce(_ context.Context) {
	wls, err := c.ga.gpuHandler.ListWorkloads()
	if err != nil {
		return
	}
	c.wls.Store(&wls)
}

// start seeds the cache synchronously, then refreshes on a ticker until ctx is
// done. Run in its own goroutine.
func (c *pullWorkloadCache) start(ctx context.Context, interval time.Duration) {
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

// get returns the latest cached workload map without locking. ok is false until
// the first successful refresh, so the caller falls back to a synchronous fetch.
func (c *pullWorkloadCache) get() (map[string]scheduler.Workload, bool) {
	m := c.wls.Load()
	if m == nil {
		return nil, false
	}
	return *m, true
}

// startPullWorkloadCache builds and starts the async workload cache once, gated to
// flag-on non-high-res store-serving mode (useSnapshotStoreServing already encodes
// enabled && !high-res). No-op in flag-off/legacy and high-res, where getMetricsAll
// keeps the synchronous per-scrape ListWorkloads. Idempotent: a second call while
// a cache is already published is a no-op.
func (ga *GPUAgentGPUClient) startPullWorkloadCache(ctx context.Context) {
	if !ga.useSnapshotStoreServing() {
		return
	}
	if ga.pullWLCache.Load() != nil {
		return
	}
	c := newPullWorkloadCache(ga)
	ga.pullWLCache.Store(c)
	go c.start(ctx, hrWorkloadLabelRefreshInterval)
}
