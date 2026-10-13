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
	"testing"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
)

// NeedsPerScrapePull drives the middleware's mutex-vs-lock-free choice. Every
// device client now serves /metrics from the single-source store: GPU in either
// mode (non-high-res refreshed via singleflight, high-res fed by the GPUWatch
// stream writer), NIC, and IFOE all return false. The composite ORs over its
// sub-clients, so it is false whenever at least one client is registered and true
// only for the degenerate zero-client agent (legacy mutex path).
func TestNeedsPerScrapePull(t *testing.T) {
	t.Run("GPU-only, non-high-res -> lock-free", func(t *testing.T) {
		teardown := setupTest(t)
		defer teardown(t)

		ga := getNewAgent(t)
		defer ga.Close()
		assert.Equal(t, ga.NeedsPerScrapePull(), false)
	})

	t.Run("GPU-only, high-res -> lock-free (store fed by stream)", func(t *testing.T) {
		teardown := setupTest(t)
		defer teardown(t)

		hrmh := newHighResMetricsHandler(t,
			`{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
		// No ga.Init()/Close(): NeedsPerScrapePull reads only the store-serving
		// predicate, and Close() would deref the Init-time cancel.
		ga := NewAgent(hrmh, WithK8sClient(nil), WithK8sSchedulerClient(nil),
			WithGPUMonitoring(true))
		assert.Equal(t, ga.NeedsPerScrapePull(), false)
	})

	t.Run("GPU+IFOE -> lock-free (both store-served)", func(t *testing.T) {
		teardown := setupTest(t)
		defer teardown(t)

		// No ga.Init()/Close(): the predicate needs only the sub-client list.
		ga := NewAgent(mh, WithK8sClient(nil), WithK8sSchedulerClient(nil),
			WithGPUMonitoring(true), WithIFOEMonitoring(true))

		// Both sub-clients are present, and each is store-served -> false, so the
		// OR cannot be rescued by a non-store-serving sibling.
		var gpuSeen, ifoeSeen bool
		for _, c := range ga.clients {
			switch c.GetDeviceType() {
			case globals.GPUDevice:
				gpuSeen = true
			case globals.IFOEDevice:
				ifoeSeen = true
			}
			assert.Equal(t, c.NeedsPerScrapePull(), false)
		}
		assert.Assert(t, gpuSeen && ifoeSeen, "expected GPU+IFOE sub-clients")
		assert.Equal(t, ga.NeedsPerScrapePull(), false)
	})

	t.Run("zero clients -> needs pull (legacy mutex path)", func(t *testing.T) {
		// A composite with no registered clients keeps the mutex path.
		ga := &GPUAgentClient{}
		assert.Equal(t, ga.NeedsPerScrapePull(), true)
	})
}
