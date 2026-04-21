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
	"fmt"
	"strings"
	"testing"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
)

// TestGpuMetricsPrefixNamingConvention verifies that none of the registered
// prometheus metrics in GpuMetrics use the disallowed "amd_" vendor prefix.
// All GPU metrics must use standard prefixes (e.g. "gpu_", "pcie_", "xgmi_").
func TestGpuMetricsPrefixNamingConvention(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	defer ga.Close()

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	var gpuclient *GPUAgentGPUClient
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			gpuclient = client.(*GPUAgentGPUClient)
			gpuclient.enableProfileMetrics = true
			assert.Assert(t, err == nil, "expecting success gpu metrics init: %v", err)
			break
		}
	}

	assert.Assert(t, gpuclient != nil, "expecting GPU client to be present")
	assert.Assert(t, gpuclient.metrics != nil, "expecting metrics to be initialized")

	// Trigger one collection cycle so GaugeVecs have observed series; an empty
	// GaugeVec is omitted from Gather() output.
	err = ga.UpdateMetricsStats(context.Background())
	assert.Assert(t, err == nil, "expecting UpdateMetricsStats to succeed: %v", err)

	// Query the same Prometheus registry that initFieldRegistration registered
	// the enabled GaugeVecs into, instead of reflecting over the (unexported)
	// GpuMetrics struct fields.
	mfs, err := mh.GetRegistry().Gather()
	assert.Assert(t, err == nil, "expecting metrics gather to succeed: %v", err)
	assert.Assert(t, len(mfs) > 0, "expecting at least one metric family to be gathered")

	var violations []string
	for _, mf := range mfs {
		name := mf.GetName()
		if strings.HasPrefix(name, "amd_") {
			violations = append(violations, name)
		}
	}

	assert.Assert(t, len(violations) == 0,
		fmt.Sprintf("the following metrics use the disallowed 'amd_' prefix: %v", violations))
}
