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
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
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

// gpuClientOf returns the GPU device client of an agent.
func gpuClientOf(t *testing.T, ga *GPUAgentClient) *GPUAgentGPUClient {
	t.Helper()
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			return client.(*GPUAgentGPUClient)
		}
	}
	t.Fatal("expecting an agent with a gpu client")
	return nil
}

// A device held by more than one workload is reported for each of them. This is
// the shared DRA claim case: the claim is referenced by two pods and holds every
// device of the pool, so naming one consumer leaves the other with no series at
// all for a device it is using.
func TestASharedDeviceIsReportedForEveryConsumer(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	defer ga.Close()

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")
	ga.fl.Reset()
	defer ga.fl.Reset()

	const draKey = "gpu-0-0"
	gpu := &amdgpu.GPU{
		Spec: &amdgpu.GPUSpec{Id: []byte(uuid.New().String())},
		Status: &amdgpu.GPUStatus{
			Index:      0,
			SerialNum:  "test-serial",
			CardModel:  "test-model",
			CardSeries: "test-series",
			CardVendor: "AMD",
			PCIeStatus: &amdgpu.GPUPCIeStatus{PCIeBusId: "0000:01:00.0"},
		},
		Stats: &amdgpu.GPUStats{PackagePower: 100},
	}
	wls := map[string]scheduler.Workloads{
		draKey: {
			{Type: scheduler.Kubernetes, Info: scheduler.PodResourceInfo{Pod: "llm-a", Namespace: "ai", Container: "server"}},
			{Type: scheduler.Kubernetes, Info: scheduler.PodResourceInfo{Pod: "llm-b", Namespace: "ai", Container: "server"}},
		},
	}

	gpuclient := gpuClientOf(t, ga)
	gpuclient.gpuIDMap[fmt.Sprintf("%v", gpu.Status.Index)] = GPUIDMeta{DRAKey: draKey}
	gpuclient.exportLabels[exportermetrics.MetricLabel_POD.String()] = true
	gpuclient.exportLabels[exportermetrics.MetricLabel_NAMESPACE.String()] = true

	gpuclient.updateGPUInfoToMetrics(wls, gpu, nil, nil, nil)

	mfs, err := mh.GetRegistry().Gather()
	assert.Assert(t, err == nil, "expecting metrics gather to succeed: %v", err)

	pods := []string{}
	for _, mf := range mfs {
		if mf.GetName() != "gpu_package_power" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == strings.ToLower(exportermetrics.MetricLabel_POD.String()) {
					pods = append(pods, label.GetValue())
				}
			}
		}
	}
	sort.Strings(pods)
	assert.DeepEqual(t, pods, []string{"llm-a", "llm-b"})
}
