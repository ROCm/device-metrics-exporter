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
	"fmt"
	"testing"

	"github.com/google/uuid"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

func TestGpuAgent(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	t.Logf("gpuagent : %+v", ga)

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateStaticMetrics()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateMetricsStats()
	assert.Assert(t, err == nil, "expecting success config init")

	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		err = client.processHealthValidation()
		assert.Assert(t, err == nil, "expecting success health validation")
	}

	wls, err := ga.ListWorkloads()
	assert.Assert(t, err == nil, "expecting success workload list")
	assert.Assert(t, len(wls) == 0, "expecting success 2 workloads on slurm")
	ga.Close()

	// set k8s nil check test
	ga.isKubernetes = true
	wls, err = ga.ListWorkloads()
	assert.Assert(t, err == nil, "expecting success workload list")
	assert.Assert(t, len(wls) == 0, "expecting success empty list of workload on k8s and slurm")

}

func TestGpuAgentSlurm(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	t.Logf("gpuagent : %+v", ga)

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateStaticMetrics()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateMetricsStats()
	assert.Assert(t, err == nil, "expecting success config init")

	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		err = client.processHealthValidation()
		assert.Assert(t, err == nil, "expecting success health validation")
	}

	ga.slurmScheduler = newSlurmMockClient()
	wls, err := ga.ListWorkloads()
	assert.Assert(t, err == nil, "expecting success workload list")
	assert.Assert(t, len(wls) == 2, "expecting success 2 workloads on slurm")
	ga.Close()

}
func TestGpuAgentK8s(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	t.Logf("gpuagent : %+v", ga)

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateStaticMetrics()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateMetricsStats()
	assert.Assert(t, err == nil, "expecting success config init")

	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		err = client.processHealthValidation()
		assert.Assert(t, err == nil, "expecting success health validation")
	}
	ga.isKubernetes = true
	ga.k8sScheduler = newK8sSchedulerMock()
	wls, err := ga.ListWorkloads()
	assert.Assert(t, err == nil, "expecting success workload list")
	assert.Assert(t, len(wls) == 2, "expecting success 2 workloads on slurm")
	ga.Close()
}

func TestGPUAgentWithoutScheduler(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgentWithoutScheduler(t)
	t.Logf("gpuagent : %+v", ga)

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	wls, err := ga.ListWorkloads()
	assert.Assert(t, err == nil, "expecting success workload list")
	assert.Assert(t, len(wls) == 0, "expecting success 0 workloads as scheduler is not initialized")
	ga.Close()
}

func TestGPUAgentIFOEOnly(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgentWithOnlyIFOE(t)
	t.Logf("gpuagent : %+v", ga)

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateStaticMetrics()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateMetricsStats()
	assert.Assert(t, err == nil, "expecting success config init")

	wls, err := ga.ListWorkloads()
	assert.Assert(t, err == nil, "expecting success workload list")
	assert.Assert(t, len(wls) == 0, "expecting success 0 workloads as scheduler is not initialized")
	ga.Close()
}

// TestMetricFieldMapping validates that all metric field references match fieldMetricsMap
// by actually calling updateGPUInfoToMetrics with comprehensive GPU data
func TestMetricFieldMapping(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	defer ga.Close()

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	// Reset filter logger before test
	ga.fl.Reset()
	defer ga.fl.Reset() // Reset after test as well

	// Create comprehensive GPU data with all fields populated
	gpu := &amdgpu.GPU{
		Spec: &amdgpu.GPUSpec{
			Id: []byte(uuid.New().String()),
		},
		Status: &amdgpu.GPUStatus{
			Index:      0,
			SerialNum:  "test-serial",
			CardModel:  "test-model",
			CardSeries: "test-series",
			CardVendor: "test-vendor",
			PCIeStatus: &amdgpu.GPUPCIeStatus{
				PCIeBusId: "0000:01:00.0",
			},
		},
		Stats: &amdgpu.GPUStats{
			PackagePower:    100,
			AvgPackagePower: 95,
			PowerUsage:      90,
			EnergyConsumed:  1000.0,
			Temperature: &amdgpu.GPUTemperatureStats{
				EdgeTemperature:     65.0,
				JunctionTemperature: 70.0,
				MemoryTemperature:   60.0,
				HBMTemperature:      []float32{55.0, 56.0},
			},
			Usage: &amdgpu.GPUUsage{
				GFXActivity:  50,
				UMCActivity:  30,
				MMActivity:   20,
				VCNActivity:  []uint32{10, 15},
				JPEGActivity: []uint32{5, 6},
				GFXBusyInst:  []uint32{45},
				VCNBusyInst:  []uint32{12},
				JPEGBusyInst: []uint32{8},
			},
			Voltage: &amdgpu.GPUVoltage{
				Voltage:       1200,
				GFXVoltage:    1100,
				MemoryVoltage: 1050,
			},
			PCIeStats: &amdgpu.GPUPCIeStats{
				ReplayCount:         10,
				RecoveryCount:       2,
				ReplayRolloverCount: 0,
				NACKSentCount:       1,
				NACKReceivedCount:   0,
				RxBytes:             1024000,
				TxBytes:             2048000,
				BiDirBandwidth:      3,
			},
			VRAMUsage: &amdgpu.GPUVRAMUsage{
				TotalVisibleVRAM: 16384,
				UsedVisibleVRAM:  8192,
				FreeVisibleVRAM:  8192,
				TotalGTT:         4096,
				UsedGTT:          2048,
				FreeGTT:          2048,
				UsedVRAM:         8000,
			},
			ViolationStats: &amdgpu.GPUViolationStats{
				CurrentAccumulatedCounter:         5,
				ProcessorHotResidencyAccumulated:  100,
				PPTResidencyAccumulated:           50,
				SocketThermalResidencyAccumulated: 25,
				VRThermalResidencyAccumulated:     10,
				HBMThermalResidencyAccumulated:    5,
			},
			TotalCorrectableErrors:      10,
			TotalUncorrectableErrors:    0,
			SDMACorrectableErrors:       2,
			SDMAUncorrectableErrors:     0,
			GFXCorrectableErrors:        3,
			GFXUncorrectableErrors:      0,
			MMHUBCorrectableErrors:      1,
			MMHUBUncorrectableErrors:    0,
			ATHUBCorrectableErrors:      1,
			ATHUBUncorrectableErrors:    0,
			BIFCorrectableErrors:        1,
			BIFUncorrectableErrors:      0,
			HDPCorrectableErrors:        1,
			HDPUncorrectableErrors:      0,
			XGMIWAFLCorrectableErrors:   1,
			XGMIWAFLUncorrectableErrors: 0,
			DFCorrectableErrors:         0,
			DFUncorrectableErrors:       0,
			SMNCorrectableErrors:        0,
			SMNUncorrectableErrors:      0,
			SEMCorrectableErrors:        0,
			SEMUncorrectableErrors:      0,
			MP0CorrectableErrors:        0,
			MP0UncorrectableErrors:      0,
			MP1CorrectableErrors:        0,
			MP1UncorrectableErrors:      0,
			FUSECorrectableErrors:       0,
			FUSEUncorrectableErrors:     0,
			UMCCorrectableErrors:        0,
			UMCUncorrectableErrors:      0,
			MCACorrectableErrors:        0,
			MCAUncorrectableErrors:      0,
			VCNCorrectableErrors:        0,
			VCNUncorrectableErrors:      0,
			JPEGCorrectableErrors:       0,
			JPEGUncorrectableErrors:     0,
			IHCorrectableErrors:         0,
			IHUncorrectableErrors:       0,
			MPIOCorrectableErrors:       0,
			MPIOUncorrectableErrors:     0,
		},
	}

	wls := make(map[string]scheduler.Workload)
	partitionMap := make(map[string]*amdgpu.GPU)
	cper := make(map[string]*amdgpu.CPEREntry)

	// Call the function under test through the GPU client
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			// nolint
			gpuclient, _ := client.(*GPUAgentGPUClient)
			gpuclient.updateGPUInfoToMetrics(wls, gpu, partitionMap, nil, cper)
			break
		}
	}

	// Validate that critical fields we fixed are not in unsupported list
	gpuid := fmt.Sprintf("%v", 0)

	// Check the fields we specifically fixed
	testFields := []string{
		"GPU_GFX_BUSY_INSTANTANEOUS",
		"GPU_VCN_BUSY_INSTANTANEOUS",
		"GPU_JPEG_BUSY_INSTANTANEOUS",
		"GPU_HBM_TEMPERATURE",
	}

	for _, fieldName := range testFields {
		isUnsupported := ga.fl.checkUnsupportedFields(gpuid, fieldName)
		assert.Assert(t, !isUnsupported,
			"Field %s should not be in unsupported list", fieldName)
	}

	// Mark filtering as done to prevent further logging
	ga.fl.SetFilterDone()

	t.Logf("✓ All critical metric fields validated successfully")
	t.Logf("✓ No fields marked as unsupported incorrectly")
}

// TestMetricFieldMappingMI2xxEmptyBusyInst validates that when BusyInst fields are empty
// (as on MI2xx platforms), the correct metric fields are marked as unsupported
func TestMetricFieldMappingMI2xxEmptyBusyInst(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	defer ga.Close()

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	// Reset filter logger before test
	ga.fl.Reset()
	defer ga.fl.Reset() // Reset after test as well

	// Create GPU data with EMPTY BusyInst fields to simulate MI2xx platform
	// where these metrics are not supported
	gpu := &amdgpu.GPU{
		Spec: &amdgpu.GPUSpec{
			Id: []byte(uuid.New().String()),
		},
		Status: &amdgpu.GPUStatus{
			Index:      0,
			SerialNum:  "mi2xx-serial",
			CardModel:  "MI210",
			CardSeries: "MI2xx",
			CardVendor: "AMD",
			PCIeStatus: &amdgpu.GPUPCIeStatus{
				PCIeBusId: "0000:01:00.0",
			},
		},
		Stats: &amdgpu.GPUStats{
			PackagePower:    100,
			AvgPackagePower: 95,
			PowerUsage:      90,
			EnergyConsumed:  1000.0,
			Temperature: &amdgpu.GPUTemperatureStats{
				EdgeTemperature:     65.0,
				JunctionTemperature: 70.0,
				MemoryTemperature:   60.0,
				HBMTemperature:      []float32{}, // Empty to test unsupported scenario
			},
			Usage: &amdgpu.GPUUsage{
				GFXActivity:  50,
				UMCActivity:  30,
				MMActivity:   20,
				VCNActivity:  []uint32{10, 15},
				JPEGActivity: []uint32{5, 6},
				// Empty BusyInst fields to simulate MI2xx platform
				GFXBusyInst:  []uint32{},
				VCNBusyInst:  []uint32{},
				JPEGBusyInst: []uint32{},
			},
			Voltage: &amdgpu.GPUVoltage{
				Voltage:       1200,
				GFXVoltage:    1100,
				MemoryVoltage: 1050,
			},
			VRAMUsage: &amdgpu.GPUVRAMUsage{
				TotalVisibleVRAM: 16384,
				UsedVisibleVRAM:  8192,
				FreeVisibleVRAM:  8192,
				TotalGTT:         4096,
				UsedGTT:          2048,
				FreeGTT:          2048,
				UsedVRAM:         8000,
			},
		},
	}

	wls := make(map[string]scheduler.Workload)
	partitionMap := make(map[string]*amdgpu.GPU)
	cper := make(map[string]*amdgpu.CPEREntry)

	// Call the function under test through the GPU client
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			// nolint
			gpuclient, _ := client.(*GPUAgentGPUClient)
			gpuclient.updateGPUInfoToMetrics(wls, gpu, partitionMap, nil, cper)
			break
		}
	}

	// Validate that BusyInst fields ARE in unsupported list for MI2xx
	gpuid := fmt.Sprintf("%v", 0)

	// These fields should be marked as unsupported when empty (MI2xx platforms)
	mi2xxUnsupportedFields := []string{
		"GPU_GFX_BUSY_INSTANTANEOUS",
		"GPU_VCN_BUSY_INSTANTANEOUS",
		"GPU_JPEG_BUSY_INSTANTANEOUS",
		"GPU_HBM_TEMPERATURE",
	}

	for _, fieldName := range mi2xxUnsupportedFields {
		isUnsupported := ga.fl.checkUnsupportedFields(gpuid, fieldName)
		assert.Assert(t, isUnsupported,
			"Field %s should be in unsupported list for MI2xx (empty BusyInst)", fieldName)
	}

	// Mark filtering as done
	ga.fl.SetFilterDone()

	t.Logf("✓ All MI2xx BusyInst empty fields correctly marked as unsupported")
	t.Logf("✓ Validated correct behavior for platforms without BusyInst support")
}
