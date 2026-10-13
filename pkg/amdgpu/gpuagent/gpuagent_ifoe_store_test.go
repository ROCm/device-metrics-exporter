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
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

func ifoeClientFrom(t *testing.T, ga *GPUAgentClient) *GPUAgentIFOEClient {
	for _, c := range ga.clients {
		if c.GetDeviceType() == globals.IFOEDevice {
			return c.(*GPUAgentIFOEClient)
		}
	}
	t.Fatal("no IFOE client found")
	return nil
}

// TestIFOEStoreParity gates the store writer: for the shared IFOE mock fixture,
// the store -> StoreCollector exposition must be identical to the existing
// GaugeVec registry exposition. UpdateMetricsStats populates both paths; we
// then diff the StoreCollector against the live registry text.
func TestIFOEStoreParity(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga := getNewAgentWithOnlyIFOE(t)
	assert.NilError(t, ga.InitConfigs())
	assert.NilError(t, ga.UpdateMetricsStats(ga.GetContext()))

	ifoe := ifoeClientFrom(t, ga)

	// Encode the old GaugeVec registry (prefix-wrapped) as the expected text.
	mfs, err := mh.GetRegistry().Gather()
	assert.NilError(t, err)
	var regBuf bytes.Buffer
	enc := expfmt.NewEncoder(&regBuf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		assert.NilError(t, enc.Encode(mf))
	}

	// The store -> StoreCollector output must reproduce it exactly.
	coll := snapshotstore.NewStoreCollector(mh.GetStore(), ifoe.buildStoreFieldDescs())
	if err := testutil.CollectAndCompare(coll, bytes.NewReader(regBuf.Bytes())); err != nil {
		t.Fatalf("store/gauge parity mismatch:\n%v", err)
	}
}

// TestIFOEStoreNodeTotalsSeeded asserts the three node-level totals are present
// at value 0 with node labels after InitConfigs alone (startup seed), i.e. with
// zero devices and before any fetch.
func TestIFOEStoreNodeTotalsSeeded(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga := getNewAgentWithOnlyIFOE(t)
	assert.NilError(t, ga.InitConfigs())
	// Deliberately no UpdateMetricsStats: only the startup seed has run.

	ifoe := ifoeClientFrom(t, ga)
	snap := mh.GetStore().Snapshot(ifoe.nodeStoreKey())
	assert.Assert(t, snap != nil, "node totals slot must exist after seed")

	hostLabel := strings.ToLower(exportermetrics.MetricLabel_HOSTNAME.String())
	for _, field := range []string{"ifoe_total_devices", "ifoe_total_stations", "ifoe_total_ports"} {
		s, ok := snap.Get(field)
		assert.Assert(t, ok, "%s must be seeded", field)
		assert.Equal(t, s.Value, float64(0), "%s must be 0 at seed", field)
		_, hasHost := s.Labels[hostLabel]
		assert.Assert(t, hasHost, "%s must carry node label %q", field, hostLabel)
	}

	// Descriptors must always include the three node-level totals, independent
	// of field config.
	names := map[string]bool{}
	for _, d := range ifoe.buildStoreFieldDescs() {
		names[d.Field] = true
	}
	for _, field := range []string{"ifoe_total_devices", "ifoe_total_stations", "ifoe_total_ports"} {
		assert.Assert(t, names[field], "descriptor for %s must always be present", field)
	}
}
