/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package gpuagent

import (
	"testing"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// newFilterTestClient builds a client whose enabled-field map reflects the given
// field list (nil/empty = all fields enabled, matching initFieldConfig default).
func newFilterTestClient(fields []string, labels ...string) *GPUAgentGPUClient {
	logger.Init(true)
	ga := &GPUAgentGPUClient{
		exportLabels: make(map[string]bool),
	}
	cfg := &exportermetrics.GPUMetricConfig{Fields: fields}
	ga.initFieldConfig(cfg)
	for _, l := range labels {
		ga.exportLabels[l] = true
	}
	return ga
}

func TestBuildGPUGetFilterDefaultKeepsEverything(t *testing.T) {
	// empty Fields => all fields enabled => nothing skipped
	ga := newFilterTestClient(nil)
	f := ga.buildGPUGetFilter()
	assert.Equal(t, false, f.SkipClockStatus)
	assert.Equal(t, false, f.SkipXGMIStatus)
	assert.Equal(t, false, f.SkipProcessStatus)
	assert.Equal(t, false, f.SkipVRAMUsageStats)
	assert.Equal(t, false, f.SkipViolationStats)
	assert.Equal(t, false, f.SkipPCIeStats)
	assert.Equal(t, false, f.SkipXGMIStats)
	assert.Equal(t, false, f.SkipActivityStats)
}

func TestBuildGPUGetFilterMinimalConfigSkipsUnneeded(t *testing.T) {
	// only a health/temperature field enabled => every skippable group is skipped
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_EDGE_TEMPERATURE.String(),
	})
	f := ga.buildGPUGetFilter()
	assert.Equal(t, true, f.SkipClockStatus)
	assert.Equal(t, true, f.SkipXGMIStatus)
	assert.Equal(t, true, f.SkipProcessStatus)
	assert.Equal(t, true, f.SkipVRAMUsageStats)
	assert.Equal(t, true, f.SkipViolationStats)
	assert.Equal(t, true, f.SkipPCIeStats)
	assert.Equal(t, true, f.SkipXGMIStats)
	assert.Equal(t, true, f.SkipActivityStats)
	// ECC and PCIe status are never expressed as skip flags (health needs them)
	assert.Equal(t, false, f.SkipECCStats)
	assert.Equal(t, false, f.SkipPCIeStatus)
}

func TestBuildGPUGetFilterProcessGatedByField(t *testing.T) {
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_PROCESS_CU_OCCUPANCY.String(),
	})
	assert.Equal(t, false, ga.buildGPUGetFilter().SkipProcessStatus)
}

func TestBuildGPUGetFilterProcessGatedByKFDLabel(t *testing.T) {
	// no process field, but KFD_PROCESS_ID label => process walk still needed
	ga := newFilterTestClient(
		[]string{exportermetrics.GPUMetricField_GPU_EDGE_TEMPERATURE.String()},
		exportermetrics.GPUMetricLabel_KFD_PROCESS_ID.String(),
	)
	assert.Equal(t, false, ga.buildGPUGetFilter().SkipProcessStatus)
}

func TestBuildGPUGetFilterPCIeStatsGatedByField(t *testing.T) {
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_PCIE_RX.String(),
	})
	assert.Equal(t, false, ga.buildGPUGetFilter().SkipPCIeStats)
}

func TestBuildGPUGetFilterViolationGatedByPrefix(t *testing.T) {
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_VIOLATION_PPT_RESIDENCY_ACCUMULATED.String(),
	})
	assert.Equal(t, false, ga.buildGPUGetFilter().SkipViolationStats)
}

func TestBuildGPUGetFilterXGMIGatesBothStatusAndStats(t *testing.T) {
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_XGMI_NBR_0_REQ_TX.String(),
	})
	f := ga.buildGPUGetFilter()
	assert.Equal(t, false, f.SkipXGMIStatus)
	assert.Equal(t, false, f.SkipXGMIStats)
}

func TestBuildGPUGetFilterActivityAndClock(t *testing.T) {
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_GFX_ACTIVITY.String(),
		exportermetrics.GPUMetricField_GPU_CLOCK.String(),
	})
	f := ga.buildGPUGetFilter()
	assert.Equal(t, false, f.SkipActivityStats)
	assert.Equal(t, false, f.SkipClockStatus)
	// unrelated groups still skipped
	assert.Equal(t, true, f.SkipViolationStats)
	assert.Equal(t, true, f.SkipPCIeStats)
}

// Health validation reads ECC counters and PCIeStatus regardless of which
// metric fields are exported; the shared filter must never skip those groups.
func TestBuildGPUGetFilterNeverSkipsHealthGroups(t *testing.T) {
	// most restrictive config: a single non-ECC, non-PCIe field
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_GFX_ACTIVITY.String(),
	})
	f := ga.buildGPUGetFilter()
	assert.Equal(t, false, f.SkipECCStats)
	assert.Equal(t, false, f.SkipPCIeStatus)
}

func TestBuildGPUGetFilterVRAMUsageVsStatus(t *testing.T) {
	// GPU_TOTAL_VRAM is sourced from VRAMStatus (ungated), so enabling only it
	// must NOT keep the vram-usage collector alive.
	ga := newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_TOTAL_VRAM.String(),
	})
	assert.Equal(t, true, ga.buildGPUGetFilter().SkipVRAMUsageStats)

	// GPU_USED_VRAM is sourced from vram-usage, so it must keep the collector.
	ga = newFilterTestClient([]string{
		exportermetrics.GPUMetricField_GPU_USED_VRAM.String(),
	})
	assert.Equal(t, false, ga.buildGPUGetFilter().SkipVRAMUsageStats)
}

var _ = amdgpu.GPUGetFilter{}
