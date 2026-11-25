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

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

// pullWorkloadCache keeps the non-high-res pull path's scheduler workload map off
// the /metrics scrape hot path. It is a thin gate over the shared workload
// source: get() returns the last successfully fetched map, lock-free. Only
// built in flag-on non-high-res store-serving mode, so flag-off /metrics keeps
// the per-scrape ListWorkloads call and stays byte-identical.
type pullWorkloadCache struct {
	src *workloadSource
}

func newPullWorkloadCache(src *workloadSource) *pullWorkloadCache {
	return &pullWorkloadCache{src: src}
}

// get returns the last successfully fetched workload map without locking. ok is
// false until a fetch has succeeded, so the caller falls back to a synchronous
// fetch; a later failed fetch keeps the last good map (as the cache's own
// refresh did before the shared source).
func (c *pullWorkloadCache) get() (map[string]scheduler.Workload, bool) {
	ws := c.src.lastOK.Load()
	if ws == nil {
		return nil, false
	}
	return ws.wls, true
}

// startPullWorkloadCache publishes the pull gate once, gated to flag-on
// non-high-res store-serving mode (useSnapshotStoreServing already encodes
// enabled && !high-res). No-op in flag-off/legacy and high-res, where
// getMetricsAll keeps the synchronous per-scrape ListWorkloads. Idempotent: a
// second call while a cache is already published is a no-op. The gate has no
// goroutine of its own; the shared source refreshes it and outlives a gpuagent
// reconnect.
func (ga *GPUAgentGPUClient) startPullWorkloadCache(_ context.Context) {
	if !ga.useSnapshotStoreServing() {
		return
	}
	if ga.pullWLCache.Load() != nil {
		return
	}
	ga.pullWLCache.Store(newPullWorkloadCache(ga.ensureWorkloadSource()))
}
