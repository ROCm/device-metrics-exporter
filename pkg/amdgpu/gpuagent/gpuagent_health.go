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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

func (ga *GPUAgentGPUClient) getHealthThreshholds() *exportermetrics.GPUHealthThresholds {
	rConfig := ga.gpuHandler.mh.GetRunConfig()
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

// getCperHealthMaxAge returns how long a fatal CPER record remains actionable for health.
func (ga *GPUAgentGPUClient) getCperHealthMaxAge() time.Duration {
	return ga.gpuHandler.mh.GetRunConfig().GetGPUCperMaxAge()
}

func (ga *GPUAgentGPUClient) isFatalCPERActionable(record *amdgpu.CPEREntry, maxAge time.Duration) bool {
	if record.Severity != amdgpu.CPERSeverity_CPER_SEVERITY_FATAL {
		return false
	}
	if maxAge == 0 {
		return true
	}
	recordTS, ok := parseCPERRecordTimestamp(record)
	if !ok {
		// Without a timestamp we cannot prove the record is stale; treat as actionable.
		return true
	}
	age := time.Since(recordTS)
	if age > maxAge {
		logger.Debugf("ignoring stale fatal CPER RecordId=%v age=%v maxAge=%v TimeStamp=%v",
			record.RecordId, age.Round(time.Second), maxAge, record.GetTimestamp())
		return false
	}
	return true
}

func (ga *GPUAgentGPUClient) applyCPERHealthChecks(
	gpuUUIDMap map[string]string,
	newGPUState map[string]*metricssvc.GPUState,
	gpuCper *amdgpu.GPUCPERGetResponse,
	cperErr error,
) {
	if cperErr != nil {
		logger.Errorf("skipping CPER health checks: cache read failed: %v", cperErr)
		return
	}
	if gpuCper == nil {
		logger.Errorf("skipping CPER health checks: CPER cache not populated yet")
		return
	}
	if gpuCper.ApiStatus != 0 {
		logger.Errorf("skipping CPER health checks: CPER ApiStatus=%v", gpuCper.ApiStatus)
		return
	}
	maxAge := ga.getCperHealthMaxAge()
	unhealthy := strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String())

	for gpuuid, record := range latestCPERPerGPU(gpuCper) {
		if !ga.isFatalCPERActionable(record, maxAge) {
			continue
		}
		if gpuid, ok := gpuUUIDMap[gpuuid]; ok {
			newGPUState[gpuid].Health = unhealthy
		} else {
			logger.Errorf("ignoring latest fatal CPER RecordId=%v: unknown GPU UUID %v", record.RecordId, gpuuid)
		}
	}
}

// returns list of
// eccUncorrectBlock binds a per-engine uncorrectable-ECC counter to its health
// threshold, its typed GPUStats accessor (pull path), and its store field name
// (high-res path). The threshold check is identical regardless of source.
type eccUncorrectBlock struct {
	storeField string
	name       string
	threshold  func(*exportermetrics.GPUHealthThresholds) uint32
	stat       func(*amdgpu.GPUStats) uint64
}

var eccUncorrectBlocks = []eccUncorrectBlock{
	{"gpu_ecc_uncorrect_sdma", "GPU_ECC_UNCORRECT_SDMA", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_SDMA }, func(s *amdgpu.GPUStats) uint64 { return s.SDMAUncorrectableErrors }},
	{"gpu_ecc_uncorrect_gfx", "GPU_ECC_UNCORRECT_GFX", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_GFX }, func(s *amdgpu.GPUStats) uint64 { return s.GFXUncorrectableErrors }},
	{"gpu_ecc_uncorrect_mmhub", "GPU_ECC_UNCORRECT_MMHUB", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_MMHUB }, func(s *amdgpu.GPUStats) uint64 { return s.MMHUBUncorrectableErrors }},
	{"gpu_ecc_uncorrect_athub", "GPU_ECC_UNCORRECT_ATHUB", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_ATHUB }, func(s *amdgpu.GPUStats) uint64 { return s.ATHUBUncorrectableErrors }},
	{"gpu_ecc_uncorrect_bif", "GPU_ECC_UNCORRECT_BIF", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_BIF }, func(s *amdgpu.GPUStats) uint64 { return s.BIFUncorrectableErrors }},
	{"gpu_ecc_uncorrect_hdp", "GPU_ECC_UNCORRECT_HDP", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_HDP }, func(s *amdgpu.GPUStats) uint64 { return s.HDPUncorrectableErrors }},
	{"gpu_ecc_uncorrect_xgmi_wafl", "GPU_ECC_UNCORRECT_XGMI_WAFL", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_XGMI_WAFL }, func(s *amdgpu.GPUStats) uint64 { return s.XGMIWAFLUncorrectableErrors }},
	{"gpu_ecc_uncorrect_df", "GPU_ECC_UNCORRECT_DF", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_DF }, func(s *amdgpu.GPUStats) uint64 { return s.DFUncorrectableErrors }},
	{"gpu_ecc_uncorrect_smn", "GPU_ECC_UNCORRECT_SMN", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_SMN }, func(s *amdgpu.GPUStats) uint64 { return s.SMNUncorrectableErrors }},
	{"gpu_ecc_uncorrect_sem", "GPU_ECC_UNCORRECT_SEM", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_SEM }, func(s *amdgpu.GPUStats) uint64 { return s.SEMUncorrectableErrors }},
	{"gpu_ecc_uncorrect_mp0", "GPU_ECC_UNCORRECT_MP0", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_MP0 }, func(s *amdgpu.GPUStats) uint64 { return s.MP0UncorrectableErrors }},
	{"gpu_ecc_uncorrect_mp1", "GPU_ECC_UNCORRECT_MP1", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_MP1 }, func(s *amdgpu.GPUStats) uint64 { return s.MP1UncorrectableErrors }},
	{"gpu_ecc_uncorrect_fuse", "GPU_ECC_UNCORRECT_FUSE", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_FUSE }, func(s *amdgpu.GPUStats) uint64 { return s.FUSEUncorrectableErrors }},
	{"gpu_ecc_uncorrect_umc", "GPU_ECC_UNCORRECT_UMC", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_UMC }, func(s *amdgpu.GPUStats) uint64 { return s.UMCUncorrectableErrors }},
	{"gpu_ecc_uncorrect_mca", "GPU_ECC_UNCORRECT_MCA", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_MCA }, func(s *amdgpu.GPUStats) uint64 { return s.MCAUncorrectableErrors }},
	{"gpu_ecc_uncorrect_vcn", "GPU_ECC_UNCORRECT_VCN", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_VCN }, func(s *amdgpu.GPUStats) uint64 { return s.VCNUncorrectableErrors }},
	{"gpu_ecc_uncorrect_jpeg", "GPU_ECC_UNCORRECT_JPEG", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_JPEG }, func(s *amdgpu.GPUStats) uint64 { return s.JPEGUncorrectableErrors }},
	{"gpu_ecc_uncorrect_ih", "GPU_ECC_UNCORRECT_IH", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_IH }, func(s *amdgpu.GPUStats) uint64 { return s.IHUncorrectableErrors }},
	{"gpu_ecc_uncorrect_mpio", "GPU_ECC_UNCORRECT_MPIO", func(t *exportermetrics.GPUHealthThresholds) uint32 { return t.GPU_ECC_UNCORRECT_MPIO }, func(s *amdgpu.GPUStats) uint64 { return s.MPIOUncorrectableErrors }},
}

// markUnhealthyIfOverThreshold flips state to unhealthy when count (or an injected
// mock value) exceeds the per-block threshold. Shared by the pull and high-res paths.
func (ga *GPUAgentGPUClient) markUnhealthyIfOverThreshold(state *metricssvc.GPUState, gpuid, fieldName string, threshold uint32, count float64) {
	if mockVal := ga.getMockError(gpuid, fieldName); mockVal > 0 {
		count = float64(mockVal)
	}
	if count > float64(threshold) {
		state.Health = strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String())
		logger.Log.Printf("gpuid[%v] is set to unhealthy for ecc field [%v] error crossing threshold %v, current value %v", gpuid, fieldName, threshold, count)
	}
}

func (ga *GPUAgentGPUClient) processEccErrorMetrics(gpus []*amdgpu.GPU, wls map[string]scheduler.Workload) map[string]*metricssvc.GPUState {

	gpuHealthMap := make(map[string]*metricssvc.GPUState)
	// this will fetch the latest threshold as the config refresh is done
	// through metrics handler in the main thread
	thresholds := ga.getHealthThreshholds()

	for _, gpu := range gpus {
		uuid, _ := uuid.FromBytes(gpu.Spec.Id)
		gpuid := getGPUInstanceIDString(gpu)
		gpuuid := uuid.String()
		stats := gpu.Stats
		deviceid := ""
		if gpu.Status.PCIeStatus != nil {
			deviceid = strings.ToLower(gpu.Status.PCIeStatus.PCIeBusId)
		}
		workloadInfo := ga.getWorkloadsListString(wls, gpuid)
		// default is healthy
		gpuHealthMap[gpuid] = &metricssvc.GPUState{
			ID:                 gpuid,
			UUID:               gpuuid,
			Health:             strings.ToLower(metricssvc.GPUHealth_HEALTHY.String()),
			Device:             deviceid,
			AssociatedWorkload: workloadInfo,
		}

		// business logic for health detection
		for _, b := range eccUncorrectBlocks {
			ga.markUnhealthyIfOverThreshold(gpuHealthMap[gpuid], gpuid, b.name, b.threshold(thresholds), utils.NormalizeUint64(b.stat(stats)))
		}
	}

	return gpuHealthMap

}

// processEccHealthFromStore computes per-GPU ECC health in high-res mode by reading
// the watcher-fed uncorrectable ECC counters from the snapshot store instead of
// issuing a getGPUs. It preserves the sole-amd-smi-caller invariant (no device call)
// and mirrors the resulting gpu_health gauge back into the store so /metrics stays
// consistent. Metadata (uuid, pcie, index) comes from the subscribe-time snapshot.
func (ga *GPUAgentGPUClient) processEccHealthFromStore(wls map[string]scheduler.Workload) map[string]*metricssvc.GPUState {
	store := ga.gpuHandler.mh.GetStore()
	snap := ga.hrSnapshot.Load()
	if store == nil || snap == nil || len(snap.gpus) == 0 {
		return nil
	}
	thresholds := ga.getHealthThreshholds()
	skip := ga.workloadLabelKeys()
	healthyStr := strings.ToLower(metricssvc.GPUHealth_HEALTHY.String())
	out := make(map[string]*metricssvc.GPUState, len(snap.gpus))
	for _, gpu := range snap.gpus {
		// snap.gpus is the unfiltered getGPUs result; skip Spec-less entries (the
		// subscribe-list filter drops them too) so gpu.Spec.Id can't nil-panic.
		if gpu.GetSpec() == nil || len(gpu.GetSpec().GetId()) == 0 {
			continue
		}
		gpuid := getGPUInstanceIDString(gpu)
		guuid, _ := uuid.FromBytes(gpu.GetSpec().GetId())
		deviceid := ""
		if gpu.Status != nil && gpu.Status.PCIeStatus != nil {
			deviceid = strings.ToLower(gpu.Status.PCIeStatus.PCIeBusId)
		}
		// Key must match the stream writer's key: both are gpuStoreDeviceKey over
		// the per-GPU label set with the workload skip, and the keyed labels are
		// static per GPU so they agree. If they ever diverged, the ECC fields would
		// read absent and the hold-prior branch below keeps the last verdict rather
		// than flipping to healthy.
		labels := ga.populateLabelsFromGPU(wls, gpu, snap.partitionMap)
		key := gpuStoreDeviceKey(labels, skip)
		ds := store.Snapshot(key)
		state := &metricssvc.GPUState{
			ID:                 gpuid,
			UUID:               guuid.String(),
			Health:             healthyStr,
			Device:             deviceid,
			AssociatedWorkload: ga.getWorkloadsListString(wls, gpuid),
		}
		eccPresent := false
		for _, b := range eccUncorrectBlocks {
			s, ok := ds.Get(b.storeField)
			if !ok {
				// Absent != zero: a stream gap (DropFields drops gpu_ecc_uncorrect_*)
				// or a Fields list that omits them must not read as "0 errors" and
				// flip a bad GPU healthy. Skip the block instead of defaulting to 0.
				continue
			}
			eccPresent = true
			ga.markUnhealthyIfOverThreshold(state, gpuid, b.name, b.threshold(thresholds), s.Value)
		}
		if !eccPresent {
			// No ECC telemetry in the store for this GPU (stream dropped or ECC
			// fields disabled): hold the prior verdict rather than declaring healthy
			// on missing data.
			if prev := ga.priorHealthState(gpuid); prev != nil {
				state.Health = prev.Health
			}
		}
		out[gpuid] = state
		healthVal := 1.0
		if state.Health != healthyStr {
			healthVal = 0.0
		}
		store.Update(key, map[string]snapshotstore.Sample{
			gpuHealthField: {Value: healthVal, Labels: labels},
		})
	}
	return out
}

// setUnhealthyGPU : reset the health status to unhealthy
// to make all gpu unavailable through
// device plugin - populate the old pcie bus entries with updated workload
// list
func (ga *GPUAgentGPUClient) setUnhealthyGPU(wls map[string]scheduler.Workload) error {
	// valid only for k8s case
	ga.Lock()
	defer ga.Unlock()

	// lookup based on device id is limited to k8s case we'll have only k8s job info
	// this is good enough for reporting the GPU as unhealthy for slinky case as well
	for _, gpustate := range ga.healthState {
		workloadInfo := ga.getWorkloadsListString(wls, gpustate.ID)
		gpustate.Health = strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String())
		gpustate.AssociatedWorkload = workloadInfo
	}

	return nil
}

func (ga *GPUAgentGPUClient) updateNewHealthState(newGPUState map[string]*metricssvc.GPUState) error {
	ga.Lock()
	defer ga.Unlock()
	ga.healthState = make(map[string]*metricssvc.GPUState)
	for gpuid, hstate := range newGPUState {
		ga.healthState[gpuid] = hstate
	}
	return nil
}

// priorHealthState returns the last computed health state for gpuid, or nil if
// none. Held under ga.Lock since updateNewHealthState rewrites healthState. Used
// by processEccHealthFromStore to carry a verdict forward when ECC telemetry is
// momentarily absent from the store.
func (ga *GPUAgentGPUClient) priorHealthState(gpuid string) *metricssvc.GPUState {
	ga.Lock()
	defer ga.Unlock()
	return ga.healthState[gpuid]
}

// ErrAgentUnreachable is returned by processHealthValidation when the gRPC
// data pull fails, indicating the gpuagent process is unreachable. It is
// distinct from non-connectivity errors (e.g. compute node unhealthy) so that
// StartMonitor can count only genuine unreachability towards the exit threshold.
var ErrAgentUnreachable = errors.New("gpuagent unreachable")

// ErrZeroGPUs is returned by processHealthValidation when gpuagent is reachable
// but reports 0 GPUs. Known causes:
//   - Boot race: amdgpu loaded late so KFD nodes are not yet registered when
//     amdsmi_get_socket_handles is called at gpuagent init time.
//   - Driver crash: amdgpu crashes mid-run and gpuagent loses all GPU handles.
//   - vfio-pci passthrough: GPU is bound to vfio-pci (passed through to a VM)
//     so amdgpu/KFD never claims it and amdsmi sees 0 devices.
//
// In all cases gpuagent never re-discovers GPUs after init, so the only recovery
// is a restart. Counting this towards the exit threshold allows --exit-on-agent-down
// to trigger a container restart so the exporter can re-init once the GPU is
// accessible again.
var ErrZeroGPUs = errors.New("gpuagent reports zero GPUs")

// isRestartableFailure reports whether err should count toward the consecutive-failure
// exit threshold in StartMonitor. Only genuine unreachability and zero-GPU conditions
// are restartable; other errors (e.g. compute node unhealthy) are transient and should
// not trigger an exit.
func isRestartableFailure(err error) bool {
	return errors.Is(err, ErrAgentUnreachable) || errors.Is(err, ErrZeroGPUs)
}

func (ga *GPUAgentGPUClient) processHealthValidation() error {
	wls, err := ga.gpuHandler.ListWorkloads()
	if err != nil {
		logger.Log.Printf("Error listing workloads: %v", err)
	}

	ga.Lock()
	if !ga.gpuHandler.computeNodeHealthState { // unhealthy
		ga.Unlock()
		_ = ga.setUnhealthyGPU(wls)
		err := fmt.Errorf("compute node unhealthy, cannot process metrics")
		logger.Log.Printf("err: %+v", err)
		return err
	}
	ga.Unlock()

	// High-res: the GPUWatch stream is the sole amd-smi caller, so health is computed
	// from the watcher-fed ECC counters in the store instead of a getGPUs. CPER and
	// event checks are skipped (in-band RAS is disabled in high-res). The snapshot is
	// nil until the first subscribe completes; skip the tick until then.
	if ga.useHighResStoreServing() {
		snap := ga.hrSnapshot.Load()
		if snap == nil {
			// First subscribe hasn't completed; nothing to assess yet.
			return nil
		}
		if len(snap.gpus) == 0 {
			// Snapshot present but zero GPUs: boot race (amdgpu loaded after
			// gpuagent) or a driver crash. Mark existing GPUs unhealthy and return
			// ErrZeroGPUs so StartMonitor counts it toward --exit-on-agent-down,
			// matching the pull path's zero-GPU handling.
			logger.Log.Printf("high-res: gpuagent snapshot reports 0 GPUs; marking existing GPUs unhealthy")
			_ = ga.setUnhealthyGPU(wls)
			return fmt.Errorf("gpuagent returned 0 GPUs: %w", ErrZeroGPUs)
		}
		newGPUState := ga.processEccHealthFromStore(wls)
		if newGPUState == nil {
			return nil
		}
		return ga.updateNewHealthState(newGPUState)
	}

	var gpumetrics *amdgpu.GPUGetResponse
	var evtData *amdgpu.EventResponse
	var gpuCper *amdgpu.GPUCPERGetResponse
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
	gpumetrics, _, err = ga.getGPUs(ga.gCache.gpuGetFilter, &ga.gCache.metricsCache)
	if err != nil || (gpumetrics != nil && gpumetrics.ApiStatus != 0) {
		errOccured = true
		logger.Log.Printf("gpuagent get metrics failed %v", err)
		goto ret
	} else if len(gpumetrics.Response) == 0 {
		// gpuagent returned 0 GPUs despite a successful RPC. This happens on a
		// boot race (amdgpu loaded late, KFD nodes not registered yet at gpuagent
		// init) or a driver crash. Mark existing GPUs unhealthy and return
		// ErrZeroGPUs so StartMonitor can count this towards the exit threshold
		// and restart the container for recovery.
		logger.Log.Printf("gpuagent returned 0 GPUs; marking existing GPUs unhealthy")
		_ = ga.setUnhealthyGPU(wls)
		return fmt.Errorf("gpuagent returned 0 GPUs: %w", ErrZeroGPUs)
	} else {
		newGPUState = ga.processEccErrorMetrics(gpumetrics.Response, wls)
	}

	for _, gpu := range gpumetrics.Response {
		uuid, _ := uuid.FromBytes(gpu.Spec.Id)
		gpuid := fmt.Sprintf("%v", gpu.Status.Index)
		gpuuid := uuid.String()
		gpuUUIDMap[gpuuid] = gpuid
	}

	// events have no notification stream under SR-IOV/GIM or sim; baremetal-only
	if !(ga.gpuHandler.enableSriov || utils.IsSimEnabled()) {
		if !utils.IsEventsDisabled() {
			evtData, err = ga.getEvents(amdgpu.EventSeverity_EVENT_SEVERITY_CRITICAL)
			if err != nil || (evtData != nil && evtData.ApiStatus != 0) {
				// ignore event errors log only
				logger.Log.Printf("gpuagent get events failed %v", err)
			} else {
				// business logic for health detection
				for _, evt := range evtData.Event {
					eventErrCheck(evt)
				}
			}
		}
	}

	// CPER runs on baremetal and SR-IOV/GIM alike, but only when enabled; when
	// disabled the refresh goroutine never starts, so the cache stays nil and
	// running the checks would log a spurious error on every health poll.
	if utils.IsCperEnabled() {
		gpuCper, err = ga.cacheCperRead()
		ga.applyCPERHealthChecks(gpuUUIDMap, newGPUState, gpuCper, err)
	}

ret:
	// disconnect on error
	if errOccured {
		ga.Close()
		// set state to unhealthy with updated workload list
		_ = ga.setUnhealthyGPU(wls)
		return fmt.Errorf("data pull error occured: %w", ErrAgentUnreachable)
	}

	return ga.updateNewHealthState(newGPUState)
}

func (ga *GPUAgentGPUClient) SetError(gpuid string, fields []string, values []uint32) error {
	ga.Lock()
	defer ga.Unlock()
	if ga.mockEccField == nil {
		ga.mockEccField = make(map[string]map[string]uint32)
	}
	if _, ok := ga.mockEccField[gpuid]; !ok {
		ga.mockEccField[gpuid] = make(map[string]uint32)
	}
	for i, field := range fields {
		ga.mockEccField[gpuid][field] = values[i]
	}
	return nil
}

func (ga *GPUAgentGPUClient) getMockError(gpuid, field string) uint32 {
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

func (ga *GPUAgentGPUClient) GetHealthStates() (map[string]interface{}, error) {
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
func (ga *GPUAgentGPUClient) SetComputeNodeHealthState(state bool) {
	ga.Lock()

	// If the state is unchanged, no action is needed.
	if ga.gpuHandler.computeNodeHealthState == state {
		ga.Unlock()
		return
	}

	logger.Log.Printf("updating compute node health from: %v, to: %v", ga.gpuHandler.computeNodeHealthState, state)
	ga.gpuHandler.computeNodeHealthState = state
	ga.Unlock()

	if !state { // Mark GPUs as unavailable only if the state is unhealthy (false).
		ga.updateAllGPUsHealthState(strings.ToLower(metricssvc.GPUHealth_UNHEALTHY.String()))
	} else {
		ga.updateAllGPUsHealthState(strings.ToLower(metricssvc.GPUHealth_HEALTHY.String()))
	}
}

func (ga *GPUAgentGPUClient) updateAllGPUsHealthState(healthStr string) {
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
	wls, _ := ga.gpuHandler.ListWorkloads()
	ga.gpuIDMapMu.RLock()
	gpuIDMapCopy := make(map[string]GPUIDMeta, len(ga.gpuIDMap))
	for k, v := range ga.gpuIDMap {
		gpuIDMapCopy[k] = v
	}
	ga.gpuIDMapMu.RUnlock()
	for gpuid, gpuIdMeta := range gpuIDMapCopy {
		workloadInfo := ga.getWorkloadsListString(wls, gpuid)
		ga.healthState[gpuid] = &metricssvc.GPUState{
			ID:                 gpuIdMeta.GPUID,
			UUID:               gpuIdMeta.UUID,
			Health:             healthStr,
			Device:             gpuIdMeta.PCIeBusId,
			AssociatedWorkload: workloadInfo,
		}
	}
}
