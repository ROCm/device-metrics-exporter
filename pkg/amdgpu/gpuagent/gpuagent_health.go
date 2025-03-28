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
	"strings"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/gofrs/uuid"
)

func (ga *GPUAgentClient) getHealthThreshholds() *exportermetrics.GPUHealthThresholds {
	rConfig := ga.mh.GetRunConfig()
	// config is never nil as the handler preserves default config
	if rConfig != nil && rConfig.GetConfig() != nil {
		gpuConfig := rConfig.GetConfig()
		if gpuConfig.GPUConfig != nil && gpuConfig.GPUConfig.HealthThresholds != nil {
			return gpuConfig.GPUConfig.HealthThresholds
		}
	}
	// default is all zero
	return &exportermetrics.GPUHealthThresholds{}
}

// returns list of
func (ga *GPUAgentClient) processEccErrorMetrics(gpus []*amdgpu.GPU, wls map[string]scheduler.Workload) map[string]*metricssvc.GPUState {

	gpuHealthMap := make(map[string]*metricssvc.GPUState)
	metricErrCheck := func(gpuid string, fieldName string, threshold uint32, count float64) {

		mockVal := ga.getMockError(gpuid, fieldName)
		if mockVal > 0 {
			count = float64(mockVal)
		}

		if count > float64(threshold) {
			// set health to unhealthy
			gpuHealthMap[gpuid].Health = strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String())
			logger.Log.Printf("gpuid[%v] is set to unhealthy for ecc field [%v] error crossing threshold %v, current value %v", gpuid, fieldName, threshold, count)
		}
	}
	// this will fetch the latest threshold as the config refresh is done
	// through metrics handler in the main thread
	thresholds := ga.getHealthThreshholds()

	for _, gpu := range gpus {
		uuid, _ := uuid.FromBytes(gpu.Spec.Id)
		gpuid := fmt.Sprintf("%v", gpu.Status.Index)
		gpuuid := uuid.String()
		stats := gpu.Stats
		deviceid := ""
		if gpu.Status.PCIeStatus != nil {
			deviceid = strings.ToLower(gpu.Status.PCIeStatus.PCIeBusId)
		}
		workloadInfo := []string{} // only one per gpu

		if wl := ga.getWorkloadInfo(wls, gpu, false); wl != nil {
			if wl.Type == scheduler.Kubernetes {
				podInfo := wl.Info.(scheduler.PodResourceInfo)
				workloadInfo = append(workloadInfo, fmt.Sprintf("pod : %v, namespace : %v, container: %v",
					podInfo.Pod, podInfo.Namespace, podInfo.Container))
			} else {
				jobInfo := wl.Info.(scheduler.JobInfo)
				workloadInfo = append(workloadInfo, fmt.Sprintf("id: %v, user : %v, partition: %v, cluster: %v",
					jobInfo.Id, jobInfo.User, jobInfo.Partition, jobInfo.Cluster))
			}
		}
		// default is healthy
		gpuHealthMap[gpuid] = &metricssvc.GPUState{
			ID:                 gpuid,
			UUID:               gpuuid,
			Health:             strings.ToLower(metricssvc.GPUHealth_HEALTHY.String()),
			Device:             deviceid,
			AssociatedWorkload: workloadInfo,
		}

		// business logic for health detection
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_SDMA", thresholds.GPU_ECC_UNCORRECT_SDMA, normalizeUint64(stats.SDMAUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_GFX", thresholds.GPU_ECC_UNCORRECT_GFX, normalizeUint64(stats.GFXUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_MMHUB", thresholds.GPU_ECC_UNCORRECT_MMHUB, normalizeUint64(stats.MMHUBUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_ATHUB", thresholds.GPU_ECC_UNCORRECT_ATHUB, normalizeUint64(stats.ATHUBUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_BIF", thresholds.GPU_ECC_UNCORRECT_BIF, normalizeUint64(stats.BIFUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_HDP", thresholds.GPU_ECC_UNCORRECT_HDP, normalizeUint64(stats.HDPUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_XGMI_WAFL", thresholds.GPU_ECC_UNCORRECT_XGMI_WAFL, normalizeUint64(stats.XGMIWAFLUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_DF", thresholds.GPU_ECC_UNCORRECT_DF, normalizeUint64(stats.DFUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_SMN", thresholds.GPU_ECC_UNCORRECT_SMN, normalizeUint64(stats.SMNUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_SEM", thresholds.GPU_ECC_UNCORRECT_SEM, normalizeUint64(stats.SEMUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_MP0", thresholds.GPU_ECC_UNCORRECT_MP0, normalizeUint64(stats.MP0UncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_MP1", thresholds.GPU_ECC_UNCORRECT_MP1, normalizeUint64(stats.MP1UncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_FUSE", thresholds.GPU_ECC_UNCORRECT_FUSE, normalizeUint64(stats.FUSEUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_UMC", thresholds.GPU_ECC_UNCORRECT_UMC, normalizeUint64(stats.UMCUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_MCA", thresholds.GPU_ECC_UNCORRECT_MCA, normalizeUint64(stats.MCAUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_VCN", thresholds.GPU_ECC_UNCORRECT_VCN, normalizeUint64(stats.VCNUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_JPEG", thresholds.GPU_ECC_UNCORRECT_JPEG, normalizeUint64(stats.JPEGUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_IH", thresholds.GPU_ECC_UNCORRECT_IH, normalizeUint64(stats.IHUncorrectableErrors))
		metricErrCheck(gpuid, "GPU_ECC_UNCORRECT_MPIO", thresholds.GPU_ECC_UNCORRECT_MPIO, normalizeUint64(stats.MPIOUncorrectableErrors))
	}

	return gpuHealthMap

}

// setUnhealthyGPU : reset the health status to unhealthy
// to make all gpu unavailable through
// device plugin - populate the old pcie bus entries with updated workload
// list
func (ga *GPUAgentClient) setUnhealthyGPU(wls map[string]scheduler.Workload) error {
	if !ga.isKubernetes {
		return nil
	}
	// valid only for k8s case
	ga.Lock()
	defer ga.Unlock()

	for _, gpustate := range ga.healthState {
		workloadInfo := []string{} // one per gpu
		if wl, ok := wls[gpustate.Device]; ok {
			if wl.Type == scheduler.Kubernetes {
				if podInfo, ok := wl.Info.(scheduler.PodResourceInfo); ok {
					workloadInfo = append(workloadInfo, fmt.Sprintf("pod : %v, namespace : %v, container: %v",
						podInfo.Pod, podInfo.Namespace, podInfo.Container))
				}
			}

		}
		gpustate.Health = strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String())
		gpustate.AssociatedWorkload = workloadInfo
	}

	return nil
}

func (ga *GPUAgentClient) updateNewHealthState(newGPUState map[string]*metricssvc.GPUState) error {
	ga.Lock()
	defer ga.Unlock()
	ga.healthState = make(map[string]*metricssvc.GPUState)
	for gpuid, hstate := range newGPUState {
		ga.healthState[gpuid] = hstate
	}
	return nil
}

func (ga *GPUAgentClient) processHealthValidation() error {
	wls, err := ga.ListWorkloads()
	if err != nil {
		logger.Log.Printf("Error listing workloads: %v", err)
	}

	ga.Lock()
	if !ga.computeNodeHealthState { // unhealthy
		ga.Unlock()
		_ = ga.setUnhealthyGPU(wls)
		err := fmt.Errorf("compute node unhealthy, cannot process metrics")
		logger.Log.Printf("err: %+v", err)
		return err
	}
	ga.Unlock()

	var gpumetrics *amdgpu.GPUGetResponse
	var evtData *amdgpu.EventResponse
	var newGPUState map[string]*metricssvc.GPUState

	errOccured := false

	gpuUUIDMap := make(map[string]string)

	eventErrCheck := func(e *amdgpu.Event) {
		uuid, _ := uuid.FromBytes(e.GPU)
		gpuuid := uuid.String()
		ts := e.Time.AsTime().Format(time.RFC3339)
		logger.Log.Printf("evt id=%v gpuid=%v severity=%v TimeStamp=%v Description=%v",
			e.Id, gpuuid, e.Severity, ts, e.Description)
		if e.Severity == amdgpu.EventSeverity_EVENT_SEVERITY_CRITICAL {
			if gpuid, ok := gpuUUIDMap[gpuuid]; ok {
				newGPUState[gpuid].Health = strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String())
				logger.Log.Printf("gpuid[%v] is set to unhealthy for evt[%+v]", gpuid, e)
			} else {
				logger.Log.Printf("ignoring invalid gpuid[%v] is set to unhealthy for evt[%+v]", gpuuid, e)
			}
		}
	}

	gpumetrics, err = ga.getGPUs()
	if err != nil || (gpumetrics != nil && gpumetrics.ApiStatus != 0) {
		errOccured = true
		logger.Log.Printf("gpuagent get metrics failed %v", err)
		goto ret
	} else if len(gpumetrics.Response) == 0 {
		// on driver crash gpuagent will return 0 gpus, handle such cases
		// if we have old state, mark all of the gpu as unhealthy
		return ga.setUnhealthyGPU(wls)
	} else {
		newGPUState = ga.processEccErrorMetrics(gpumetrics.Response, wls)
	}

	for _, gpu := range gpumetrics.Response {
		uuid, _ := uuid.FromBytes(gpu.Spec.Id)
		gpuid := fmt.Sprintf("%v", gpu.Status.Index)
		gpuuid := uuid.String()
		gpuUUIDMap[gpuuid] = gpuid
	}

	evtData, err = ga.getEvents(amdgpu.EventSeverity_EVENT_SEVERITY_CRITICAL)
	if err != nil || (evtData != nil && evtData.ApiStatus != 0) {
		errOccured = true
		logger.Log.Printf("gpuagent get events failed %v", err)
	} else {
		// business logic for health detection
		for _, evt := range evtData.Event {
			eventErrCheck(evt)
		}
	}

ret:
	// disconnect on error
	if errOccured {
		ga.Close()
		// set state to unhealthy with updated workload list
		_ = ga.setUnhealthyGPU(wls)
		return fmt.Errorf("data pull error occured")
	}

	return ga.updateNewHealthState(newGPUState)
}

func (ga *GPUAgentClient) SetError(gpuid string, fields []string, values []uint32) error {
	ga.Lock()
	defer ga.Unlock()
	if _, ok := ga.mockEccField[gpuid]; !ok {
		ga.mockEccField[gpuid] = make(map[string]uint32)
	}
	for i, field := range fields {
		if values[i] == 0 {
			delete(ga.mockEccField[gpuid], field)
			if len(ga.mockEccField[gpuid]) == 0 {
				delete(ga.mockEccField, gpuid)
			}
			continue
		}
		ga.mockEccField[gpuid][field] = values[i]
	}
	return nil
}

func (ga *GPUAgentClient) getMockError(gpuid, field string) uint32 {
	ga.Lock()
	defer ga.Unlock()
	if _, ok := ga.mockEccField[gpuid]; !ok {
		return 0
	}
	mv, ok := ga.mockEccField[gpuid][field]
	if !ok {
		return 0
	}
	return mv
}

func (ga *GPUAgentClient) GetGPUHealthStates() (map[string]interface{}, error) {
	ga.Lock()
	defer ga.Unlock()
	if len(ga.healthState) == 0 {
		return nil, fmt.Errorf("health status not available")
	}
	healthMap := make(map[string]interface{})
	for id, gstate := range ga.healthState {
		healthMap[id] = gstate
	}

	return healthMap, nil
}

// SetComputeNodeHealthState sets the compute node health state
func (ga *GPUAgentClient) SetComputeNodeHealthState(state bool) {
	ga.Lock()

	// If the state is unchanged, no action is needed.
	if ga.computeNodeHealthState == state {
		ga.Unlock()
		return
	}

	logger.Log.Printf("updating compute node health from: %v, to: %v", ga.computeNodeHealthState, state)
	ga.computeNodeHealthState = state
	ga.Unlock()

	if !state { // Mark GPUs as unavailable only if the state is unhealthy (false).
		ga.updateAllGPUsHealthState(strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String()))
	} else {
		ga.updateAllGPUsHealthState(strings.ToLower(metricssvc.GPUHealth_HEALTHY.String()))
	}
}

func (ga *GPUAgentClient) updateAllGPUsHealthState(healthStr string) {
	// If health state is already set, mark all GPUs as unhealthy
	if len(ga.healthState) > 0 {
		logger.Log.Printf("GPUs are already fetched, setting health state")
		for gpuid := range ga.healthState {
			ga.healthState[gpuid].Health = healthStr
		}
		return
	}

	logger.Log.Printf("fetch GPUs and set health state")
	// If health state is not set, fetch GPUs and mark them as unhealthy
	wls, _ := ga.ListWorkloads()
	gpus, err := ga.getGPUs()
	if err != nil || (gpus != nil && gpus.ApiStatus != 0) {
		logger.Log.Printf("gpuagent get GPUs failed %v", err)
		return
	}

	for _, gpu := range gpus.Response {
		uuid, _ := uuid.FromBytes(gpu.Spec.Id)
		gpuid := fmt.Sprintf("%v", gpu.Status.Index)
		gpuuid := uuid.String()
		deviceid := ""
		if gpu.Status.PCIeStatus != nil {
			deviceid = strings.ToLower(gpu.Status.PCIeStatus.PCIeBusId)
		}

		workloadInfo := []string{} // only one per gpu
		if wl := ga.getWorkloadInfo(wls, gpu, false); wl != nil {
			if wl.Type == scheduler.Kubernetes {
				podInfo := wl.Info.(scheduler.PodResourceInfo)
				workloadInfo = append(workloadInfo, fmt.Sprintf("pod : %v, namespace : %v, container: %v",
					podInfo.Pod, podInfo.Namespace, podInfo.Container))
			} else {
				jobInfo := wl.Info.(scheduler.JobInfo)
				workloadInfo = append(workloadInfo, fmt.Sprintf("id: %v, user : %v, partition: %v, cluster: %v",
					jobInfo.Id, jobInfo.User, jobInfo.Partition, jobInfo.Cluster))
			}
		}
		ga.healthState[gpuid] = &metricssvc.GPUState{
			ID:                 gpuid,
			UUID:               gpuuid,
			Health:             healthStr,
			Device:             deviceid,
			AssociatedWorkload: workloadInfo,
		}
	}
}
