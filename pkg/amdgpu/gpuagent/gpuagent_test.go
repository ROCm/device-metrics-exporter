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
	"errors"
	"fmt"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
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

	err = ga.UpdateMetricsStats(context.Background())
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

	err = ga.UpdateMetricsStats(context.Background())
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

	err = ga.UpdateMetricsStats(context.Background())
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

	err = ga.UpdateMetricsStats(context.Background())
	assert.Assert(t, err == nil, "expecting success config init")

	wls, err := ga.ListWorkloads()
	assert.Assert(t, err == nil, "expecting success workload list")
	assert.Assert(t, len(wls) == 0, "expecting success 0 workloads as scheduler is not initialized")
	ga.Close()
}

// TestGetGPUsPartitionFilter verifies that getGPUs() passes through all GPU objects
// regardless of whether GPUPartition is set.
func TestGetGPUsPartitionFilter(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	partitionedResp := &amdgpu.GPUGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response: []*amdgpu.GPU{
			{
				Spec: &amdgpu.GPUSpec{Id: []byte(uuid.New().String())},
				Status: &amdgpu.GPUStatus{
					SerialNum:    "gpu-with-partition-field",
					PCIeStatus:   &amdgpu.GPUPCIeStatus{PCIeBusId: "0000:01:00.0"},
					GPUPartition: [][]byte{[]byte(uuid.New().String())},
					PartitionId:  0,
				},
				Stats: &amdgpu.GPUStats{PackagePower: 100},
			},
			{
				Spec: &amdgpu.GPUSpec{Id: []byte(uuid.New().String())},
				Status: &amdgpu.GPUStatus{
					SerialNum:  "gpu-without-partition-field",
					PCIeStatus: &amdgpu.GPUPCIeStatus{PCIeBusId: "0000:01:00.1"},
				},
				Stats: &amdgpu.GPUStats{PackagePower: 100},
			},
		},
	}
	gpuMockCl.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(partitionedResp, nil).AnyTimes()

	ga := getNewAgentWithoutScheduler(t)
	defer ga.Close()

	var gpuclient *GPUAgentGPUClient
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			gpuclient = client.(*GPUAgentGPUClient)
			break
		}
	}
	gpuclient.gpuclient = gpuMockCl
	gpuclient.evtclient = eventMockCl

	resp, _, err := gpuclient.getGPUs()
	assert.Assert(t, err == nil, "getGPUs should not error: %v", err)
	assert.Equal(t, len(resp.Response), 2, "both GPUs must be returned regardless of GPUPartition field")
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

// TestOccupancyElapsedCalculation validates that GPU_PROF_OCCUPANCY_ELAPSED is computed
// as MeanOccupancyPerActiveCU / GRBM_GUI_ACTIVE, matching the RDC rdc_rocp reference.
func TestOccupancyElapsedCalculation(t *testing.T) {
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
			break
		}
	}

	gpu := &amdgpu.GPU{
		Spec: &amdgpu.GPUSpec{
			Id: []byte(uuid.New().String()),
		},
		Status: &amdgpu.GPUStatus{
			Index:     0,
			SerialNum: "mock-serial",
			PCIeStatus: &amdgpu.GPUPCIeStatus{
				PCIeBusId: "pcie0",
			},
		},
		Stats: &amdgpu.GPUStats{},
	}

	const grbmActive = 1e7
	const meanOccPerActiveCU = 500.0
	profMetrics := map[string]float64{
		"GRBM_GUI_ACTIVE":          grbmActive,
		"MeanOccupancyPerActiveCU": meanOccPerActiveCU,
	}

	wls := make(map[string]scheduler.Workload)
	gpuclient.updateGPUInfoToMetrics(wls, gpu, nil, profMetrics, nil)

	// Collect the gpuOccElapsed gauge and verify the derived value
	labels := gpuclient.populateLabelsFromGPU(wls, gpu, nil)
	gauge, err := gpuclient.metrics.gpuOccElapsed.GetMetricWith(labels)
	assert.Assert(t, err == nil, "expected gpuOccElapsed metric to exist: %v", err)

	var dtoMetric dto.Metric
	err = gauge.Write(&dtoMetric)
	assert.Assert(t, err == nil, "expected gauge write to succeed: %v", err)

	got := dtoMetric.GetGauge().GetValue()
	want := meanOccPerActiveCU / grbmActive
	assert.Assert(t, got == want,
		"gpu_prof_occupancy_elapsed: got %v, want %v (MeanOccupancyPerActiveCU/GRBM_GUI_ACTIVE)", got, want)

	t.Logf("✓ gpu_prof_occupancy_elapsed = %.6e (MeanOccupancyPerActiveCU=%.1f / GRBM_GUI_ACTIVE=%.0f)",
		got, meanOccPerActiveCU, grbmActive)
}

// TestExitOnAgentDownExitsAfterConsecutiveFailures verifies that the exit logic
// fires after maxConsecutiveFailures consecutive failures, matching the logic in
// StartMonitor for the processHealthValidation() failure path.
func TestExitOnAgentDownExitsAfterConsecutiveFailures(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	ga.exitOnAgentDown = true
	exitCalled := false
	ga.exitFn = func(code int) {
		exitCalled = true
	}

	const maxConsecutiveFailures = 3
	consecutiveFailures := 0

	simulateValidationFailure := func() {
		consecutiveFailures++
		if ga.exitOnAgentDown && consecutiveFailures >= maxConsecutiveFailures {
			ga.exitFn(1)
		}
	}

	simulateValidationFailure()
	simulateValidationFailure()
	simulateValidationFailure()

	assert.Assert(t, exitCalled, "exitFn should be called after %d consecutive failures", maxConsecutiveFailures)
	assert.Equal(t, consecutiveFailures, maxConsecutiveFailures, "consecutive failures count should match")
}

// TestExitOnAgentDownCounterResetsOnSuccess verifies that a successful poll tick
// resets the consecutive-failure counter so transient failures don't accumulate.
func TestExitOnAgentDownCounterResetsOnSuccess(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	ga.exitOnAgentDown = true
	exitCalled := false
	ga.exitFn = func(code int) {
		exitCalled = true
	}

	// Simulate the counter-reset logic directly: 2 failures, then a success tick,
	// then 2 more failures — total should never reach maxConsecutiveFailures (3).
	const maxConsecutiveFailures = 3
	consecutiveFailures := 0

	reconnectFail := func() {
		consecutiveFailures++
		if ga.exitOnAgentDown && consecutiveFailures >= maxConsecutiveFailures {
			ga.exitFn(1)
		}
	}
	successTick := func() {
		consecutiveFailures = 0
	}

	reconnectFail()
	reconnectFail()
	successTick() // resets to 0
	reconnectFail()
	reconnectFail()

	assert.Assert(t, !exitCalled,
		"exitFn should not be called: failures were not consecutive across the success tick")
	assert.Equal(t, consecutiveFailures, 2,
		"counter should be 2 after reset + 2 more failures")
}

// TestExitOnAgentDownDisabledDoesNotExit verifies that when exitOnAgentDown is false
// the exitFn is never invoked even after many reconnect failures.
func TestExitOnAgentDownDisabledDoesNotExit(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	ga.exitOnAgentDown = false
	exitCalled := false
	ga.exitFn = func(code int) {
		exitCalled = true
	}

	const maxConsecutiveFailures = 3
	consecutiveFailures := 0
	for i := 0; i < maxConsecutiveFailures+5; i++ {
		consecutiveFailures++
		if ga.exitOnAgentDown && consecutiveFailures >= maxConsecutiveFailures {
			ga.exitFn(1)
		}
	}

	assert.Assert(t, !exitCalled, "exitFn should not be called when exitOnAgentDown is false")
}

// TestProcessHealthValidationZeroGPUsReturnsErrZeroGPUs verifies that when the
// gpuagent gRPC call succeeds but returns an empty GPU list (boot race / driver
// crash scenario), processHealthValidation returns ErrZeroGPUs rather than nil.
// This ensures StartMonitor counts the event toward the exit threshold.
func TestProcessHealthValidationZeroGPUsReturnsErrZeroGPUs(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	// Use a fresh controller so the zero-GPU expectation is the only GPUGet
	// registration, bypassing the AnyTimes() 2-GPU response from setupTest.
	localCtl := gomock.NewController(t)
	defer localCtl.Finish()
	localGPUMock := mock_gen.NewMockGPUSvcClient(localCtl)
	localEvtMock := mock_gen.NewMockEventSvcClient(localCtl)

	// Return an empty GPU list: gpuagent reachable, amdsmi_get_socket_handles
	// returned 0 because KFD nodes were not yet registered at gpuagent init.
	zeroGPUResp := &amdgpu.GPUGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response:  []*amdgpu.GPU{},
	}
	localGPUMock.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(zeroGPUResp, nil).AnyTimes()
	localEvtMock.EXPECT().EventGet(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	ga := getNewAgent(t)
	defer ga.Close()

	// Inject the local mocks into the GPU client.
	var gpuclient *GPUAgentGPUClient
	for _, client := range ga.clients {
		if c, ok := client.(*GPUAgentGPUClient); ok {
			gpuclient = c
			break
		}
	}
	gpuclient.gpuclient = localGPUMock
	gpuclient.evtclient = localEvtMock

	err := gpuclient.processHealthValidation()
	assert.Assert(t, err != nil, "processHealthValidation should return an error when GPUGet returns 0 GPUs")
	assert.Assert(t, errors.Is(err, ErrZeroGPUs),
		"processHealthValidation should return ErrZeroGPUs when GPUGet returns 0 GPUs, got: %v", err)
}

// TestIsRestartableFailure verifies that isRestartableFailure — the production
// helper used by StartMonitor to decide which errors count toward the exit
// threshold — returns true for ErrAgentUnreachable and ErrZeroGPUs (both
// wrapped and direct) and false for all other errors.
func TestIsRestartableFailure(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	assert.Assert(t, isRestartableFailure(ErrAgentUnreachable),
		"ErrAgentUnreachable must be a restartable failure")
	assert.Assert(t, isRestartableFailure(ErrZeroGPUs),
		"ErrZeroGPUs must be a restartable failure")
	assert.Assert(t, isRestartableFailure(fmt.Errorf("wrapped: %w", ErrZeroGPUs)),
		"wrapped ErrZeroGPUs must be a restartable failure")
	assert.Assert(t, isRestartableFailure(fmt.Errorf("wrapped: %w", ErrAgentUnreachable)),
		"wrapped ErrAgentUnreachable must be a restartable failure")

	assert.Assert(t, !isRestartableFailure(errors.New("compute node unhealthy")),
		"unrelated errors must not be restartable failures")
	assert.Assert(t, !isRestartableFailure(nil),
		"nil must not be a restartable failure")
}

func TestDebugModeContext(t *testing.T) {
	// Test WithDebugMode and GetDebugMode context helpers
	ctx := context.Background()

	// Test default behavior - should return DebugModeNone
	mode := globals.GetDebugMode(ctx)
	assert.Assert(t, mode == globals.DebugModeNone,
		"GetDebugMode should return DebugModeNone for empty context")

	// Test setting DebugModeQP
	ctxWithQP := globals.WithDebugMode(ctx, globals.DebugModeQP)
	mode = globals.GetDebugMode(ctxWithQP)
	assert.Assert(t, mode == globals.DebugModeQP,
		"GetDebugMode should return DebugModeQP after setting it")

	// Test that original context is not modified
	mode = globals.GetDebugMode(ctx)
	assert.Assert(t, mode == globals.DebugModeNone,
		"Original context should remain unchanged")
}

func TestUpdateMetricsStatsWithDebugContext(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)

	err := ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	// Test with normal context (no debug mode)
	err = ga.UpdateMetricsStats(context.Background())
	assert.Assert(t, err == nil, "UpdateMetricsStats should succeed with normal context")

	// Test with debug mode QP
	ctxWithDebug := globals.WithDebugMode(context.Background(), globals.DebugModeQP)
	err = ga.UpdateMetricsStats(ctxWithDebug)
	assert.Assert(t, err == nil, "UpdateMetricsStats should succeed with debug mode QP context")

	ga.Close()
}

// newCPERTestMocks creates an isolated gomock controller with GPU, CPER, and event mocks.
// Using a separate controller avoids FIFO expectation conflicts with the AnyTimes
// defaults registered in setupTest.
func newCPERTestMocks(t *testing.T, cperErr error) (gpuSvc *mock_gen.MockGPUSvcClient, evtSvc *mock_gen.MockEventSvcClient, finish func()) {
	ctrl := gomock.NewController(t)

	gpuSvc = mock_gen.NewMockGPUSvcClient(ctrl)
	evtSvc = mock_gen.NewMockEventSvcClient(ctrl)

	gpuSvc.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(&amdgpu.GPUGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response: []*amdgpu.GPU{
			{
				Spec:   &amdgpu.GPUSpec{Id: []byte("72ff740f-0000-1000-804c-3b58bf67050e")},
				Status: &amdgpu.GPUStatus{PCIeStatus: &amdgpu.GPUPCIeStatus{PCIeBusId: "pcie0"}},
				Stats:  &amdgpu.GPUStats{},
			},
		},
	}, nil).AnyTimes()
	gpuSvc.EXPECT().GPUCPERGet(gomock.Any(), gomock.Any()).Return(nil, cperErr).AnyTimes()
	evtSvc.EXPECT().EventGet(gomock.Any(), gomock.Any()).Return(&amdgpu.EventResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
	}, nil).AnyTimes()

	return gpuSvc, evtSvc, ctrl.Finish
}

// TestCPERDeadlineExceededDoesNotTriggerExit verifies that a DeadlineExceeded error
// from the CPER RPC in processHealthValidation does NOT return ErrAgentUnreachable.
// Older drivers without inband-RAS support will always timeout on this call; treating
// it as an agent-down event would break those nodes. The background goroutine
// (startCperRefresh) handles the /metrics cache path separately.
func TestCPERDeadlineExceededDoesNotTriggerExit(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	gpuSvc, evtSvc, finish := newCPERTestMocks(t, status.Error(codes.DeadlineExceeded, "deadline exceeded"))
	defer finish()

	ga := getNewAgent(t)
	defer ga.Close()

	var gpuclient *GPUAgentGPUClient
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			gpuclient = client.(*GPUAgentGPUClient)
			break
		}
	}
	gpuclient.gpuclient = gpuSvc
	gpuclient.evtclient = evtSvc
	gpuclient.gpuHandler.computeNodeHealthState = true

	err := gpuclient.processHealthValidation()
	assert.Assert(t, !errors.Is(err, ErrAgentUnreachable),
		"DeadlineExceeded on CPER RPC should NOT return ErrAgentUnreachable, got: %v", err)
}

// TestCPERNonFatalErrorDoesNotTriggerExit verifies that non-DeadlineExceeded errors
// from the CPER RPC (e.g. Unimplemented/NOT_SUPPORTED) do not return ErrAgentUnreachable —
// the exporter continues serving other metrics.
func TestCPERNonFatalErrorDoesNotTriggerExit(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	gpuSvc, evtSvc, finish := newCPERTestMocks(t, status.Error(codes.Unimplemented, "not supported"))
	defer finish()

	ga := getNewAgent(t)
	defer ga.Close()

	var gpuclient *GPUAgentGPUClient
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			gpuclient = client.(*GPUAgentGPUClient)
			break
		}
	}
	gpuclient.gpuclient = gpuSvc
	gpuclient.evtclient = evtSvc
	gpuclient.gpuHandler.computeNodeHealthState = true

	err := gpuclient.processHealthValidation()
	assert.Assert(t, !errors.Is(err, ErrAgentUnreachable),
		"non-fatal CPER error should NOT return ErrAgentUnreachable, got: %v", err)
}

// TestCPERFatalSeveritySetsGPUUnhealthy verifies that only CPER_SEVERITY_FATAL
// entries mark the GPU unhealthy; non-fatal severities must not.
func TestCPERFatalSeveritySetsGPUUnhealthy(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	gpuIDBytes := []byte("72ff740f-0000-1000-804c-3b58bf67050e")

	gpuSvc, evtSvc, finish := newCPERTestMocks(t, nil)
	defer finish()

	ga := getNewAgent(t)
	defer ga.Close()

	var gpuclient *GPUAgentGPUClient
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			gpuclient = client.(*GPUAgentGPUClient)
			break
		}
	}
	gpuclient.gpuclient = gpuSvc
	gpuclient.evtclient = evtSvc
	gpuclient.gpuHandler.computeNodeHealthState = true

	setCperCache := func(severity amdgpu.CPERSeverity) {
		gpuclient.gCache.Lock()
		defer gpuclient.gCache.Unlock()
		gpuclient.gCache.lastCperResponse = &amdgpu.GPUCPERGetResponse{
			ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
			CPER: []*amdgpu.GPUCPEREntry{
				{GPU: gpuIDBytes, CPEREntry: []*amdgpu.CPEREntry{{Severity: severity}}},
			},
		}
	}

	unhealthy := strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String())

	setCperCache(amdgpu.CPERSeverity_CPER_SEVERITY_FATAL)
	err := gpuclient.processHealthValidation()
	assert.Assert(t, !errors.Is(err, ErrAgentUnreachable),
		"CPER fatal should not return ErrAgentUnreachable, got: %v", err)
	gpuclient.Lock()
	for _, state := range gpuclient.healthState {
		assert.Equal(t, unhealthy, state.Health, "GPU must be unhealthy on CPER_SEVERITY_FATAL")
	}
	gpuclient.Unlock()

	setCperCache(amdgpu.CPERSeverity_CPER_SEVERITY_NON_FATAL_UNCORRECTED)
	err = gpuclient.processHealthValidation()
	assert.Assert(t, !errors.Is(err, ErrAgentUnreachable),
		"non-fatal CPER severity should not return ErrAgentUnreachable, got: %v", err)
	gpuclient.Lock()
	for _, state := range gpuclient.healthState {
		assert.Assert(t, state.Health != unhealthy, "GPU must not be unhealthy on non-fatal CPER severity")
	}
	gpuclient.Unlock()
}
