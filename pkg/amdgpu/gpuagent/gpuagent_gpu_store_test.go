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
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

func gpuClientFrom(t *testing.T, ga *GPUAgentClient) *GPUAgentGPUClient {
	for _, c := range ga.clients {
		if c.GetDeviceType() == globals.GPUDevice {
			return c.(*GPUAgentGPUClient)
		}
	}
	t.Fatal("no GPU client found")
	return nil
}

// TestGPUStoreParity gates the store writer: for the shared GPU mock fixture, the
// store -> StoreCollector exposition must be byte-identical to the existing
// GaugeVec registry exposition. UpdateMetricsStats populates both paths; we then
// diff the StoreCollector against the live registry text.
func TestGPUStoreParity(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga := getNewAgent(t)
	defer ga.Close()
	assert.NilError(t, ga.InitConfigs())
	assert.NilError(t, ga.UpdateMetricsStats(context.Background()))

	gpu := gpuClientFrom(t, ga)

	// Encode the old GaugeVec registry (prefix-wrapped) as the expected text.
	mfs, err := mh.GetRegistry().Gather()
	assert.NilError(t, err)
	assert.Assert(t, len(mfs) > 0, "no populated GPU gauge families; fixture produced nothing to compare")
	var regBuf bytes.Buffer
	enc := expfmt.NewEncoder(&regBuf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		assert.NilError(t, enc.Encode(mf))
	}

	descs, err := gpu.gpuStoreFieldDescs()
	assert.NilError(t, err)

	// The store -> StoreCollector output must reproduce the registry exactly.
	coll := snapshotstore.NewStoreCollector(mh.GetStore(), descs)
	if err := testutil.CollectAndCompare(coll, bytes.NewReader(regBuf.Bytes())); err != nil {
		t.Fatalf("store/gauge parity mismatch:\n%v", err)
	}
}

// TestGPUStoreNodeTotalSeeded asserts gpu_nodes_total is present at value 0 with
// node labels after InitConfigs alone (startup seed): zero GPUs, before any fetch.
func TestGPUStoreNodeTotalSeeded(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga := getNewAgent(t)
	defer ga.Close()
	assert.NilError(t, ga.InitConfigs())
	// Deliberately no UpdateMetricsStats: only the startup seed has run.

	gpu := gpuClientFrom(t, ga)
	snap := mh.GetStore().Snapshot(gpu.nodeStoreKey())
	assert.Assert(t, snap != nil, "node total slot must exist after seed")

	s, ok := snap.Get(gpuNodesTotalField)
	assert.Assert(t, ok, "%s must be seeded", gpuNodesTotalField)
	assert.Equal(t, s.Value, float64(0), "%s must be 0 at seed", gpuNodesTotalField)
	hostLabel := strings.ToLower(exportermetrics.MetricLabel_HOSTNAME.String())
	_, hasHost := s.Labels[hostLabel]
	assert.Assert(t, hasHost, "%s must carry node label %q", gpuNodesTotalField, hostLabel)

	// Descriptor must always include gpu_nodes_total, independent of fetch state.
	descs, err := gpu.gpuStoreFieldDescs()
	assert.NilError(t, err)
	found := false
	for _, d := range descs {
		if d.Field == gpuNodesTotalField {
			found = true
		}
	}
	assert.Assert(t, found, "descriptor for %s must always be present", gpuNodesTotalField)
}
