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

// High-resolution GPU streaming: the exporter subscribes to gpuagent's
// GPUWatch service (ROCm/gpu-agent), which pushes a periodic stats snapshot
// per GPU. gpuagent samples once per cycle regardless of subscriber count, so
// this never multiplies hardware work. See
// docs-internal/knowledge/plans/2026-10-05-gpu-single-source-store-and-highres.md.
package gpuagent

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/events"
	exportermetrics "github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

// highResWatchID is a deterministic GPUWatch object id. The exporter is the
// sole GPUWatch client, so reusing a fixed id across reconnects/restarts keeps
// exactly one watch object in gpuagent rather than leaking a new one each time.
var highResWatchID = uuid.NewSHA1(uuid.NameSpaceOID,
	[]byte("amd-device-metrics-exporter/highres-gpu-watch"))

// highResWatchMetric pairs a GPUWatch attribute with the pull-path gauge name
// it feeds and how the frame-apply pipeline handles it. An empty name means
// subscribe but do not stream yet.
type highResWatchMetric struct {
	id   amdgpu.GPUWatchAttrId
	name string
	hrAttrMeta
}

// hrTier is the GPUWatch frame kind an attribute arrives in at sub-second
// periods.
type hrTier uint8

const (
	// hrTierFull attributes arrive in full frames only.
	hrTierFull hrTier = iota
	// hrTierPower arrives in partial frames at sub-second periods, in full
	// frames at 1000 ms and above.
	hrTierPower
)

// hrAttrMeta is the frame-apply pipeline's per-attribute metadata.
type hrAttrMeta struct {
	tier hrTier
	// indexLabel is the per-element label of an array-valued attribute;
	// empty for a scalar
	indexLabel string
	// hasSubType: the element's SubType feeds a clock_type label
	hasSubType bool
	// alias is the attribute whose metric this one also feeds at sub-second
	// periods (see packagePowerRule); INVALID for none
	alias amdgpu.GPUWatchAttrId
}

var (
	scalarAttr = hrAttrMeta{}
	powerAttr  = hrAttrMeta{tier: hrTierPower}
	// current_socket_power (126) also feeds gpu_package_power at sub-second
	// periods
	packagePowerAttr = hrAttrMeta{tier: hrTierPower,
		alias: amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE}
	// clocks carry both clock_index and a clock_type sub-label (from SubType)
	clockAttr = hrAttrMeta{indexLabel: "clock_index", hasSubType: true}
)

// indexedAttr is the metadata of an array-valued attribute whose elements
// carry indexLabel.
func indexedAttr(indexLabel string) hrAttrMeta {
	return hrAttrMeta{indexLabel: indexLabel}
}

// highResWatchMetrics is the single source of truth for high-resolution mode:
// both the subscription set and the stream metric names derive from it, so the
// two cannot drift. Keeping them as separate lists previously let attributes be
// subscribed but unnamed, which silently dropped every sample for them.
//
// Gauge names here must equal strings.ToLower of the GPUMetricField enum name,
// which is what the pull path registers; any divergence splits the series.
// Fields absent from this table need gpuagent attribute additions first and are
// marked unsupported in this mode until then.
var highResWatchMetrics = []highResWatchMetric{
	// clocks are array-valued: one element per (clock_type, instance), carrying
	// clock_index + clock_type labels matching the pull path. The legacy scalar
	// GPU_CLOCK/MEM_CLOCK stay subscribed (no emit) so gpuagent keeps sampling.
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_CLOCK, "", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MEM_CLOCK, "", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_CURRENT, "gpu_clock", clockAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_MIN, "gpu_min_clock", clockAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_MAX, "gpu_max_clock", clockAttr},

	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, "gpu_junction_temperature", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MEMORY_TEMP, "gpu_memory_temperature", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, "gpu_package_power", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_UTIL, "gpu_gfx_activity", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UMC_ACTIVITY, "gpu_umc_activity", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MM_ACTIVITY, "gpu_mma_activity", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_AVG_POWER, "gpu_average_package_power", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE, "gpu_gfx_voltage", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_SOC_VOLTAGE, "gpu_voltage", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MEM_VOLTAGE, "gpu_memory_voltage", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_BANDWIDTH, "pcie_bidirectional_bandwidth", scalarAttr},
	// amdsmi reports VRAM usage in MB, matching the gauge's declared unit
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_MEMORY_USAGE, "gpu_used_vram", scalarAttr},

	// ECC: attribute suffix CE/UE maps to field infix CORRECT/UNCORRECT, and the
	// PCIE_BIF attribute shortens to BIF in the field name
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_CORRECT_TOTAL, "gpu_ecc_correct_total", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_UNCORRECT_TOTAL, "gpu_ecc_uncorrect_total", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SDMA_CE, "gpu_ecc_correct_sdma", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SDMA_UE, "gpu_ecc_uncorrect_sdma", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_GFX_CE, "gpu_ecc_correct_gfx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_GFX_UE, "gpu_ecc_uncorrect_gfx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MMHUB_CE, "gpu_ecc_correct_mmhub", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MMHUB_UE, "gpu_ecc_uncorrect_mmhub", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_ATHUB_CE, "gpu_ecc_correct_athub", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_ATHUB_UE, "gpu_ecc_uncorrect_athub", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_PCIE_BIF_CE, "gpu_ecc_correct_bif", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_PCIE_BIF_UE, "gpu_ecc_uncorrect_bif", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_HDP_CE, "gpu_ecc_correct_hdp", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_HDP_UE, "gpu_ecc_uncorrect_hdp", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_XGMI_WAFL_CE, "gpu_ecc_correct_xgmi_wafl", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_XGMI_WAFL_UE, "gpu_ecc_uncorrect_xgmi_wafl", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DF_CE, "gpu_ecc_correct_df", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DF_UE, "gpu_ecc_uncorrect_df", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SMN_CE, "gpu_ecc_correct_smn", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SMN_UE, "gpu_ecc_uncorrect_smn", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SEM_CE, "gpu_ecc_correct_sem", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SEM_UE, "gpu_ecc_uncorrect_sem", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP0_CE, "gpu_ecc_correct_mp0", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP0_UE, "gpu_ecc_uncorrect_mp0", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP1_CE, "gpu_ecc_correct_mp1", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP1_UE, "gpu_ecc_uncorrect_mp1", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_FUSE_CE, "gpu_ecc_correct_fuse", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_FUSE_UE, "gpu_ecc_uncorrect_fuse", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_UMC_CE, "gpu_ecc_correct_umc", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_UMC_UE, "gpu_ecc_uncorrect_umc", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MCA_CE, "gpu_ecc_correct_mca", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MCA_UE, "gpu_ecc_uncorrect_mca", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_VCN_CE, "gpu_ecc_correct_vcn", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_VCN_UE, "gpu_ecc_uncorrect_vcn", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_JPEG_CE, "gpu_ecc_correct_jpeg", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_JPEG_UE, "gpu_ecc_uncorrect_jpeg", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_IH_CE, "gpu_ecc_correct_ih", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_IH_UE, "gpu_ecc_uncorrect_ih", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MPIO_CE, "gpu_ecc_correct_mpio", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MPIO_UE, "gpu_ecc_uncorrect_mpio", scalarAttr},

	// ECC deferred counters (third category alongside correct/uncorrect)
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_TOTAL, "gpu_ecc_deferred_total", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_SDMA, "gpu_ecc_deferred_sdma", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_GFX, "gpu_ecc_deferred_gfx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MMHUB, "gpu_ecc_deferred_mmhub", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_ATHUB, "gpu_ecc_deferred_athub", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_PCIE_BIF, "gpu_ecc_deferred_bif", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_HDP, "gpu_ecc_deferred_hdp", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_XGMI_WAFL, "gpu_ecc_deferred_xgmi_wafl", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_DF, "gpu_ecc_deferred_df", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_SMN, "gpu_ecc_deferred_smn", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_SEM, "gpu_ecc_deferred_sem", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MP0, "gpu_ecc_deferred_mp0", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MP1, "gpu_ecc_deferred_mp1", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_FUSE, "gpu_ecc_deferred_fuse", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_UMC, "gpu_ecc_deferred_umc", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MCA, "gpu_ecc_deferred_mca", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_VCN, "gpu_ecc_deferred_vcn", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_JPEG, "gpu_ecc_deferred_jpeg", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_IH, "gpu_ecc_deferred_ih", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MPIO, "gpu_ecc_deferred_mpio", scalarAttr},

	// PCIe accumulated counters + additional bulk scalars
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_REPLAY_COUNT, "pcie_replay_count", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_RECOVERY_COUNT, "pcie_recovery_count", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_REPLAY_ROLLOVER_COUNT, "pcie_replay_rollover_count", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_NACK_SENT_COUNT, "pcie_nack_sent_count", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_NACK_RECEIVED_COUNT, "pcie_nack_received_count", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_POWER_USAGE, "gpu_power_usage", packagePowerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VRAM_MAX_BANDWIDTH, "gpu_vram_max_bandwidth", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_EDGE_TEMP, "gpu_edge_temperature", scalarAttr},
	// energy accumulator (uJ): fast-tier from the bulk gpu_metrics read, so it
	// leaves the GPUGet non-watch carry set and updates every FastPeriodSecs
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ENERGY_CONSUMED, "gpu_energy_consumed", powerAttr},
	// UBB node power (W): node-wide, partition 0 only, N/A elsewhere
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, "gpu_ubb_power", scalarAttr},

	// memory totals/used/free and PCIe link speed
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_TOTAL_VRAM, "gpu_total_vram", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_TOTAL_VISIBLE_VRAM, "gpu_total_visible_vram", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_TOTAL_GTT, "gpu_total_gtt", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_USED_VISIBLE_VRAM, "gpu_used_visible_vram", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_USED_GTT, "gpu_used_gtt", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FREE_VRAM, "gpu_free_vram", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FREE_VISIBLE_VRAM, "gpu_free_visible_vram", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FREE_GTT, "gpu_free_gtt", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_SPEED, "pcie_speed", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_MAX_SPEED, "pcie_max_speed", scalarAttr},

	// XGMI: the attribute drops the NBR infix the field name carries, and the
	// per-link throughput attribute spells TX after the index
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_NOP_TX, "gpu_xgmi_nbr_0_nop_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_REQ_TX, "gpu_xgmi_nbr_0_req_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_RESP_TX, "gpu_xgmi_nbr_0_resp_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_BEATS_TX, "gpu_xgmi_nbr_0_beats_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_NOP_TX, "gpu_xgmi_nbr_1_nop_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_REQ_TX, "gpu_xgmi_nbr_1_req_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_RESP_TX, "gpu_xgmi_nbr_1_resp_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_BEATS_TX, "gpu_xgmi_nbr_1_beats_tx", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_THRPUT, "gpu_xgmi_nbr_0_tx_thrput", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_THRPUT, "gpu_xgmi_nbr_1_tx_thrput", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_2_THRPUT, "gpu_xgmi_nbr_2_tx_thrput", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_3_THRPUT, "gpu_xgmi_nbr_3_tx_thrput", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_4_THRPUT, "gpu_xgmi_nbr_4_tx_thrput", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_5_THRPUT, "gpu_xgmi_nbr_5_tx_thrput", scalarAttr},

	// flat (non-indexed) violation counters: scalar path, like ECC/XGMI above
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_CURRENT_ACC_COUNTER, "gpu_violation_current_accumulated_counter", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PROC_HOT_RESIDENCY_ACC, "gpu_violation_processor_hot_residency_accumulated", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PPT_RESIDENCY_ACC, "gpu_violation_ppt_residency_accumulated", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACC, "gpu_violation_socket_thermal_residency_accumulated", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_VR_THERMAL_RESIDENCY_ACC, "gpu_violation_vr_thermal_residency_accumulated", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_HBM_THERMAL_RESIDENCY_ACC, "gpu_violation_hbm_thermal_residency_accumulated", powerAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PROC_HOT_RESIDENCY_PCT, "gpu_violation_processor_hot_residency_percentage", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PPT_RESIDENCY_PCT, "gpu_violation_ppt_residency_percentage", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_SOCKET_THERMAL_RESIDENCY_PCT, "gpu_violation_socket_thermal_residency_percentage", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_VR_THERMAL_RESIDENCY_PCT, "gpu_violation_vr_thermal_residency_percentage", scalarAttr},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_HBM_THERMAL_RESIDENCY_PCT, "gpu_violation_hbm_thermal_residency_percentage", scalarAttr},

	// array-valued (indexed) attributes: the value rides GPUWatchAttrVal.IndexedVal
	// and fans out one sample per element; names match the pull path.
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_HBM_TEMP, "gpu_hbm_temperature", indexedAttr("hbm_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VCN_ACTIVITY, "gpu_vcn_activity", indexedAttr("vcn_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_ACTIVITY, "gpu_jpeg_activity", indexedAttr("jpeg_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_RX, "gpu_xgmi_link_rx", indexedAttr("link_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_TX, "gpu_xgmi_link_tx", indexedAttr("link_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_POWER_ACC, "gpu_violation_gfx_clock_below_host_limit_power_accumulated", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_THERMAL_ACC, "gpu_violation_gfx_clock_below_host_limit_thermal_accumulated", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_LOW_UTIL_ACC, "gpu_violation_low_utilization_accumulated", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_TOTAL_ACC, "gpu_violation_gfx_clock_below_host_limit_total_accumulated", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_POWER_PCT, "gpu_violation_gfx_clock_below_host_limit_power_percentage", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_THERMAL_PCT, "gpu_violation_gfx_clock_below_host_limit_thermal_percentage", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_LOW_UTIL_PCT, "gpu_violation_low_utilization_percentage", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_TOTAL_PCT, "gpu_violation_gfx_clock_below_host_limit_total_percentage", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_BUSY_INST, "gpu_gfx_busy_instantaneous", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VCN_BUSY_INST, "gpu_vcn_busy_instantaneous", indexedAttr("xcc_index")},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_BUSY_INST, "gpu_jpeg_busy_instantaneous", indexedAttr("xcc_index")},
}

// hrMaxAttrID bounds the GPUWatchAttrId values the pipeline indexes; the enum
// stays below it.
const hrMaxAttrID = 256

const (
	hrAttrPowerUsage    = amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE
	hrAttrGPUPowerUsage = amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_POWER_USAGE
)

// hrPackagePowerField is the ga.fl / allowlist key of gpu_package_power, which
// attr 126 (current_socket_power) feeds at sub-second periods.
var hrPackagePowerField = exportermetrics.GPUMetricField_GPU_PACKAGE_POWER.String()

// hrAttrInfo is one highResWatchMetrics row plus what the pipeline derives
// from it.
type hrAttrInfo struct {
	highResWatchMetric
	// field is the upper-cased name, the ga.fl and allowlist key
	field string
	// aliasedBy is the attribute whose alias feeds this one's metric
	aliasedBy amdgpu.GPUWatchAttrId
}

// hrAttrTable is highResWatchMetrics indexed by attribute id; an id absent
// from it has an empty name.
var hrAttrTable = func() (t [hrMaxAttrID]hrAttrInfo) {
	for _, m := range highResWatchMetrics {
		t[m.id] = hrAttrInfo{highResWatchMetric: m, field: strings.ToUpper(m.name)}
	}
	for _, m := range highResWatchMetrics {
		if m.alias != amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_INVALID {
			t[m.alias].aliasedBy = m.id
		}
	}
	return t
}()

// hrPowerAttrIDs is the power tier of highResWatchMetrics.
var hrPowerAttrIDs = func() []amdgpu.GPUWatchAttrId {
	var ids []amdgpu.GPUWatchAttrId
	for _, m := range highResWatchMetrics {
		if m.tier == hrTierPower {
			ids = append(ids, m.id)
		}
	}
	return ids
}()

// watchAttrIndexLabel returns the per-element index label for an array-valued
// attribute and whether it also carries a clock_type sub-label. Scalars return
// ("", false).
func watchAttrIndexLabel(id amdgpu.GPUWatchAttrId) (string, bool) {
	if id < 0 || int(id) >= hrMaxAttrID {
		return "", false
	}
	m := &hrAttrTable[id]
	return m.indexLabel, m.hasSubType
}

// highResWatchAttrs is the subscription set sent to gpuagent, derived from
// highResWatchMetrics so it always covers exactly the attributes handled below.
var highResWatchAttrs = func() []amdgpu.GPUWatchAttrId {
	attrs := make([]amdgpu.GPUWatchAttrId, 0, len(highResWatchMetrics))
	for _, m := range highResWatchMetrics {
		attrs = append(attrs, m.id)
	}
	return attrs
}()

// highResWatchNames indexes highResWatchMetrics for per-sample lookup.
var highResWatchNames = func() map[amdgpu.GPUWatchAttrId]string {
	names := make(map[amdgpu.GPUWatchAttrId]string, len(highResWatchMetrics))
	for _, m := range highResWatchMetrics {
		names[m.id] = m.name
	}
	return names
}()

// deleteHighResWatch removes the deterministic GPUWatch object if present,
// releasing the gpuagent watcher's persistent SMI/KFD hold. Idempotent — a no-op
// when no watch exists. Called before (re)creating the watch and on high-res
// teardown, so a high-res→non-high-res transition (or shutdown) stops gpuagent
// sampling and returns it to lazy SMI for the per-scrape GPUGet path.
func (ga *GPUAgentGPUClient) deleteHighResWatch(ctx context.Context) {
	if ga.watchclient == nil {
		return
	}
	id := highResWatchID
	getResp, gerr := ga.watchclient.GPUWatchGet(ctx,
		&amdgpu.GPUWatchGetRequest{Id: [][]byte{id[:]}})
	if gerr != nil || len(getResp.GetResponse()) == 0 {
		return
	}
	if _, derr := ga.watchclient.GPUWatchDelete(ctx,
		&amdgpu.GPUWatchDeleteRequest{Id: [][]byte{id[:]}}); derr != nil {
		logger.Log.Printf("high-res: GPUWatch delete failed: %v", derr)
		return
	}
	logger.Log.Printf("high-res: removed GPUWatch object %s (released SMI/KFD hold)", id)
}

// reconcileHighRes starts or stops the high-resolution GPUWatch feed to match the
// live config (HighResolution.Enable). It is called from InitConfigs, which the
// MetricsHandler dispatches to this client at startup and on every config reload,
// so a runtime high-res toggle is handled without any lifecycle logic in the
// exporter:
//   - enable  → start the consumer; gpuagent holds SMI persistently while watching.
//   - disable → cancel the consumer and delete the GPUWatch object, releasing
//     gpuagent's SMI/KFD hold so it returns to lazy SMI for the per-scrape GPUGet.
//
// Idempotent. The consumer runs on a context derived from the agent's lifetime
// context (gpuHandler.ctx), cancelled here on a →non-high-res transition and in
// Close() on shutdown.
func (ga *GPUAgentGPUClient) reconcileHighRes() {
	// The consumer context is derived from the agent's lifetime context, set in
	// Init() before the MetricsHandler dispatches the first InitConfigs. If it is
	// nil the agent is not initialized yet (e.g. a unit test that drives applyWatch
	// directly), so there is nothing to reconcile.
	if ga.gpuHandler == nil || ga.gpuHandler.ctx == nil {
		return
	}
	ga.hrMu.Lock()
	defer ga.hrMu.Unlock()
	var periods [2]uint32
	desired := ga.gpuHandler.mh.IsHighResolution()
	running := ga.hrCancel != nil
	if desired {
		periods[0], periods[1] = ga.gpuHandler.mh.GetHighResWatchPeriods()
	}
	// watch periods are create-time only, so a change restarts the feed
	if desired && running && periods != ga.hrPeriodsRunning {
		logger.Log.Printf("high-res: watch periods changed %dms/%ds -> %dms/%ds, restarting GPUWatch feed",
			ga.hrPeriodsRunning[0], ga.hrPeriodsRunning[1], periods[0], periods[1])
		ga.teardownHighResLocked()
		running = false
	}
	switch {
	case desired && !running:
		hrCtx, cancel := context.WithCancel(ga.gpuHandler.ctx)
		done := make(chan struct{})
		ga.hrCancel = cancel
		ga.hrDone = done
		ga.hrPeriodsRunning = periods
		go ga.runHighResFeed(hrCtx, done)
		logger.Log.Printf("high-resolution ENABLED: GPUWatch feed started (gpuagent SMI persistent)")
	case !desired && running:
		ga.teardownHighResLocked()
		logger.Log.Printf("high-resolution DISABLED: GPUWatch feed stopped, watch deleted (gpuagent returns to lazy SMI)")
		// the feed goroutine has exited, so hrTierMs is ours to update
		ga.signalHighResTier(ga.gpuHandler.ctx, 0)
	}
	// Reconcile the profiler stream under the same lock, symmetric with the watch
	// feed: its gate additionally requires profiler enabled and a non-PTL platform.
	ga.reconcileProfilerStreamLocked()
}

// highResReconnectBackoff is the wait between a GPUWatch stream failure and the
// next reconnect attempt, bounding the churn when gpuagent is down.
const highResReconnectBackoff = 5 * time.Second

// highResFallbackFastPeriodMs is the GPUWatch fast period used when gpuagent
// rejects a sub-second FastPeriodMs (a gpuagent without millisecond support).
const highResFallbackFastPeriodMs uint32 = 1000

// highResPeriodRejected reports whether gpuagent rejected a sub-second period
// (unsupported platform or an older gpuagent).
func highResPeriodRejected(st amdgpu.ApiStatus) bool {
	return st == amdgpu.ApiStatus_API_STATUS_OPERATION_NOT_SUPPORTED ||
		st == amdgpu.ApiStatus_API_STATUS_INVALID_ARG
}

// highResMissedTicksLogInterval rate-limits the GPUWatchStatus.MissedTicks
// debug line.
const highResMissedTicksLogInterval = time.Minute

// highResCadenceLogInterval rate-limits the frames/s debug line.
const highResCadenceLogInterval = 10 * time.Second

// highResDiagnosticPeriodMs: accepted periods below it are diagnostic tiers.
const highResDiagnosticPeriodMs uint32 = 100

// highResDiagnosticTierMsg is the log and K8s event text for an accepted
// diagnostic fast period.
func highResDiagnosticTierMsg(ms uint32) string {
	msg := fmt.Sprintf("high-resolution fast period %d ms is a diagnostic tier (below %d ms): "+
		"exporter and gpuagent CPU cost is much higher than at 100 ms or 1 s; use it for "+
		"short diagnostic runs only", ms, highResDiagnosticPeriodMs)
	if ms == 1 {
		msg += "; 1 ms is experimental"
	}
	return msg
}

// signalHighResTier warns (log and K8s event) when the accepted period
// enters a diagnostic tier and logs when it leaves; silent if unchanged.
func (ga *GPUAgentGPUClient) signalHighResTier(ctx context.Context, acceptedMs uint32) {
	prev := ga.hrTierMs
	ga.hrTierMs = acceptedMs
	if acceptedMs == prev {
		return
	}
	isDiag := func(ms uint32) bool { return ms != 0 && ms < highResDiagnosticPeriodMs }
	switch {
	case isDiag(acceptedMs):
		msg := highResDiagnosticTierMsg(acceptedMs)
		// EnhancedLogger.Warnf drops warnings at the default Info level
		logger.Log.Logger.Warnf("high-res: %s", msg)
		if ga.hrEvents != nil {
			ga.hrEvents.EmitWarning(ctx, events.HighResDiagnosticTier, msg)
		} else {
			events.EmitWarning(ctx, events.HighResDiagnosticTier, msg)
		}
	case isDiag(prev):
		now := "high-resolution disabled"
		if acceptedMs != 0 {
			now = fmt.Sprintf("fast period now %d ms", acceptedMs)
		}
		logger.Log.Printf("high-res: diagnostic tier %d ms ended (%s)", prev, now)
	}
}

// runHighResFeed owns the high-resolution GPUWatch feed for one enable session.
// It (re)establishes the watch and consumes it until the stream fails, then evicts
// the now-stale watch telemetry from the store — so /metrics serves only the
// always-present signals (gpu_nodes_total and gpu_health, kept fresh by the slow
// writer) until the feed recovers — and reconnects after a short backoff, at which
// point applyWatchSnapshot repopulates the telemetry. It is the sole owner of the
// high-res connection state; teardown cancels ctx and waits on done before any new
// session starts, so two feeds never run concurrently. Exits on ctx cancellation.
func (ga *GPUAgentGPUClient) runHighResFeed(ctx context.Context, done chan struct{}) {
	defer close(done)
	// the per-GPU state cache belongs to this goroutine; drop it on exit
	defer func() { ga.hrHot = nil }()
	for ctx.Err() == nil {
		err := ga.StartHighRes(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Log.Printf("high-resolution feed error: %+v (reconnecting)", err)
		}
		// Stream ended / setup failed while high-res is still desired: drop stale
		// telemetry, then back off before reconnecting.
		ga.dropStaleHighResTelemetry()
		select {
		case <-ctx.Done():
			return
		case <-time.After(highResReconnectBackoff):
		}
	}
}

// dropStaleHighResTelemetry evicts the GPUWatch-fed fields from the store on a
// stream failure so /metrics stops serving last-known clocks/temps and shows only
// the always-present signals (gpu_nodes_total + gpu_health) until the feed
// reconnects and applyWatchSnapshot repopulates them.
func (ga *GPUAgentGPUClient) dropStaleHighResTelemetry() {
	if store := ga.gpuHandler.mh.GetStore(); store != nil {
		store.DropFields(highResWatchFieldSet)
	}
}

// teardownHighRes cancels the consumer and deletes the GPUWatch object, releasing
// gpuagent's SMI/KFD hold. Safe to call when not running (no-op). Used by Close()
// on shutdown; reconcileHighRes uses the locked variant.
func (ga *GPUAgentGPUClient) teardownHighRes() {
	ga.hrMu.Lock()
	defer ga.hrMu.Unlock()
	ga.teardownHighResLocked()
}

// teardownHighResLocked is the body of teardownHighRes; caller must hold hrMu.
func (ga *GPUAgentGPUClient) teardownHighResLocked() {
	if ga.hrCancel == nil {
		return
	}
	ga.hrCancel()
	// Wait for the feed goroutine (sole owner of the high-res connection state) to
	// fully exit before clearing state, so a later enable never races it.
	if ga.hrDone != nil {
		<-ga.hrDone
		ga.hrDone = nil
	}
	ga.hrCancel = nil
	// Stop the workload-label refresher so it stops ticking (and calling the
	// scheduler) while high-res is off; StartHighRes starts a fresh one on re-subscribe.
	if ga.hrWLCancel != nil {
		ga.hrWLCancel()
		ga.hrWLCancel = nil
	}
	// Fresh ctx: the feed's ctx is now cancelled, but the delete RPC must run.
	// Bound it with queryTimeout so a wedged gpuagent can't hang teardown (and the
	// /metrics rebind that waits on it).
	delCtx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	ga.deleteHighResWatch(delCtx)
}

// StartHighRes enumerates GPUs, creates a GPUWatch over the supported attributes
// for all of them, subscribes, and consumes the feed in a background goroutine,
// applying each received snapshot to the single-source store.
func (ga *GPUAgentGPUClient) StartHighRes(ctx context.Context) error {
	if ga.gpuclient == nil || ga.watchclient == nil {
		if err := ga.InitClients(); err != nil {
			return err
		}
	}

	// Enumerate GPUs via the same path as the pull model (getGPUs) so
	// partitioned GPUs (CPX/NPS1) are handled identically: each logical
	// partition is its own GPU entry (own UUID + PartitionId) and is subscribed
	// individually, while the returned partitionMap lets child partitions
	// resolve their parent's card series/model/vendor labels. Passing a nil map
	// (as a raw GPUGet would) leaves those labels blank for non-parent
	// partitions.
	getResp, partitionMap, err := ga.getGPUs(ga.gCache.gpuGetFilter, &ga.gCache.metricsCache)
	if err != nil {
		return err
	}
	// Cache the GPU snapshot from this subscribe-time getGPUs (the only getGPUs run
	// while high-res is active — it happens here with the watcher not yet started).
	// The label caches reuse it so they never issue a concurrent getGPUs that would
	// race the watcher's amd-smi access.
	// metadata before the snapshot, so a label cache never pairs the new
	// snapshot with the old gpuIDMap
	ga.initGPUMetadata(getResp.GetResponse())
	ga.hrSnapshot.Store(&hrGPUSnap{gpus: getResp.GetResponse(), partitionMap: partitionMap})
	ga.hrLabelsByUUID = make(map[string]prometheus.Labels)
	ga.hrGPUIDByUUID = make(map[string]string)
	// Store-serving only: seed the subscribe-time label set with real workloads so
	// the first streamed frame is workload-aware; flag-off keeps nil (static set,
	// unchanged stream labels).
	storeServing := ga.useHighResStoreServing()
	var seedWls map[string]scheduler.Workload
	if storeServing {
		ga.ensureWorkloadSource()
		seedWls, _ = ga.currentWorkloads()
	}
	var gpuIDs [][]byte
	for _, gpu := range getResp.GetResponse() {
		if gpu.GetSpec() == nil || len(gpu.GetSpec().GetId()) == 0 {
			continue
		}
		gpuIDs = append(gpuIDs, gpu.GetSpec().GetId())
		// Cache the static label set per logical GPU/partition UUID so streamed
		// snapshots (which carry only GPU-id + attr values) can be labelled.
		guid := getGPUUUID(gpu)
		ga.hrLabelsByUUID[guid] = prometheus.Labels(ga.populateLabelsFromGPU(seedWls, gpu, partitionMap))
		// ga.fl keys on the instance index, so cache it too: sharing the key
		// with the pull path is what makes a field unsupported in one path
		// unsupported in both, and logged once rather than once per path.
		ga.hrGPUIDByUUID[guid] = getGPUInstanceIDString(gpu)
	}
	if len(gpuIDs) == 0 {
		return ErrZeroGPUs
	}
	// Store-serving only: run the async full-label cache so streamed samples carry
	// live pod/slurm workload labels 1-1 with the pull path. Off the stream hot
	// path (ListWorkloads can block on the scheduler).
	if storeServing {
		// Replace any prior cache (this may be a reconnect): cancel the previous
		// refresher goroutine first so it does not leak, then start a fresh one.
		if ga.hrWLCancel != nil {
			ga.hrWLCancel()
		}
		wlCtx, wlCancel := context.WithCancel(ctx)
		ga.hrWLCancel = wlCancel
		ga.hrWLCache = newHRWorkloadLabelCache(ga)
		go ga.hrWLCache.start(wlCtx, hrWorkloadLabelRefreshInterval)
	}

	// Use a deterministic watch-object id so a reconnect or exporter restart
	// reuses the same object instead of leaking a new one on each attempt. The
	// exporter is the only GPUWatch client, so a fixed id is safe.
	watchID := highResWatchID
	// Remove any pre-existing watch object of the same id (stale from a prior
	// instance, or a reconnect) so create starts from a clean slate and never
	// duplicates.
	ga.deleteHighResWatch(ctx)
	// Periods are create-time only: gpuagent keeps one set for the single watch
	// object, so a retune must delete and recreate it.
	fastMs, slowSecs := ga.gpuHandler.mh.GetHighResWatchPeriods()
	spec := &amdgpu.GPUWatchSpec{
		Id:             watchID[:],
		GPU:            gpuIDs,
		Attribute:      highResWatchAttrs,
		FastPeriodMs:   fastMs,
		SlowPeriodSecs: slowSecs,
	}
	resp, err := ga.watchclient.GPUWatchCreate(ctx,
		&amdgpu.GPUWatchRequest{Spec: []*amdgpu.GPUWatchSpec{spec}})
	if err != nil {
		return err
	}
	if st := resp.GetApiStatus(); st != amdgpu.ApiStatus_API_STATUS_OK {
		if spec.FastPeriodMs >= highResFallbackFastPeriodMs || !highResPeriodRejected(st) {
			return fmt.Errorf("GPUWatchCreate: %v", st)
		}
		// fall back to 1000 ms for this session; reconnects retry the period
		logger.Log.Printf("high-res: gpuagent rejected FastPeriodMs=%d (%v); recreating GPUWatch at %d ms",
			spec.FastPeriodMs, st, highResFallbackFastPeriodMs)
		ga.deleteHighResWatch(ctx)
		spec.FastPeriodMs = highResFallbackFastPeriodMs
		resp, err = ga.watchclient.GPUWatchCreate(ctx,
			&amdgpu.GPUWatchRequest{Spec: []*amdgpu.GPUWatchSpec{spec}})
		if err != nil {
			return err
		}
		if st := resp.GetApiStatus(); st != amdgpu.ApiStatus_API_STATUS_OK {
			return fmt.Errorf("GPUWatchCreate: %v", st)
		}
	}
	ga.hrFastMs.Store(spec.FastPeriodMs)
	ga.signalHighResTier(ctx, spec.FastPeriodMs)
	// new session: the first frame rebuilds the per-GPU state cache
	ga.hrHot = nil
	stream, err := ga.watchclient.GPUWatchSubscribe(ctx,
		&amdgpu.GPUWatchSubscribeRequest{Id: [][]byte{watchID[:]}})
	if err != nil {
		return err
	}
	logger.Log.Printf("high-res: GPUWatch subscribed for %d GPU(s), %d attrs, fast %d ms, slow %d s",
		len(gpuIDs), len(highResWatchAttrs), spec.FastPeriodMs, spec.SlowPeriodSecs)

	// Consume inline: blocks until the stream fails or ctx is cancelled, returning
	// control to runHighResFeed to drop stale telemetry and reconnect.
	ga.consumeWatchStream(ctx, stream)
	return nil
}

// consumeWatchStream applies GPUWatch frames to the store until the stream
// ends. Partial frames (Spec without Attribute) take applyPowerFrame.
func (ga *GPUAgentGPUClient) consumeWatchStream(ctx context.Context,
	stream amdgpu.GPUWatchSvc_GPUWatchSubscribeClient) {
	var winStart, missedLogged time.Time
	var winFrames, missed uint64
	arrivals := newHRArrivals(ga.hrGPUIDByUUID)
	for ctx.Err() == nil {
		w, err := stream.Recv()
		if err != nil {
			logger.Log.Printf("high-res: GPUWatch stream ended: %v", err)
			return
		}
		now := time.Now()
		// MissedTicks counts power ticks, not frames; logged only
		if m := w.GetStatus().GetMissedTicks(); m > missed {
			if now.Sub(missedLogged) >= highResMissedTicksLogInterval {
				logger.Log.Debugf("high-res: GPUWatch missed ticks %d (+%d)", m, m-missed)
				missedLogged = now
			}
			missed = m
		}
		// time-based cadence trace; a per-frame trigger would flood at 1 ms
		winFrames++
		if winStart.IsZero() {
			winStart = now
		} else if d := now.Sub(winStart); d >= highResCadenceLogInterval {
			if logger.Log.IsLevelEnabled(logrus.DebugLevel) {
				logger.Log.Debugf("high-res: GPUWatch %.1f frames/s over %v, missed_ticks=%d%s",
					float64(winFrames)/d.Seconds(), d, missed, arrivals.summary(d))
			}
			arrivals.reset()
			winStart, winFrames = now, 0
		}
		// partial frames exist only at sub-second periods
		if ga.hrSubSecond() && len(w.GetSpec().GetAttribute()) == 0 {
			arrivals.record(w, now)
			ga.applyPowerFrame(w)
		} else {
			ga.applyWatchSnapshot(w)
		}
	}
}

// hrArrivalRing is the per-GPU arrival intervals kept for percentiles.
const hrArrivalRing = 4096

// hrArrivalGPU is one GPU's arrival intervals in the current window; ring
// holds only the most recent ones.
type hrArrivalGPU struct {
	name   string
	last   time.Time
	frames uint64
	count  uint64
	maxUs  uint32
	sum    float64
	sumSq  float64
	ring   [hrArrivalRing]uint32 // microseconds
}

// hrArrivals tracks per-GPU partial-frame cadence for the debug line; feed
// goroutine only.
type hrArrivals struct {
	names   map[string]string // GPU UUID -> gpu_id, for the summary
	byGPU   map[[16]byte]*hrArrivalGPU
	order   []*hrArrivalGPU
	scratch [hrArrivalRing]uint32
}

func newHRArrivals(names map[string]string) *hrArrivals {
	return &hrArrivals{names: names, byGPU: map[[16]byte]*hrArrivalGPU{}}
}

// record notes the arrival at now of a partial frame for each GPU it carries.
func (a *hrArrivals) record(w *amdgpu.GPUWatch, now time.Time) {
	for _, perGPU := range w.GetStats().GetGPUWatchAttr() {
		b := perGPU.GetGPU()
		if len(b) != 16 {
			continue
		}
		g := a.byGPU[[16]byte(b)]
		if g == nil {
			g = a.add(b)
		}
		g.frames++
		if !g.last.IsZero() {
			us := now.Sub(g.last).Microseconds()
			if us < 0 {
				us = 0
			}
			v := uint32(min(us, int64(^uint32(0))))
			g.ring[g.count%hrArrivalRing] = v
			g.count++
			g.maxUs = max(g.maxUs, v)
			f := float64(v)
			g.sum += f
			g.sumSq += f * f
		}
		g.last = now
	}
}

// add starts tracking GPU b.
func (a *hrArrivals) add(b []byte) *hrArrivalGPU {
	u := utils.UUIDToString(b)
	name := a.names[u]
	if name == "" {
		name = u
	}
	g := &hrArrivalGPU{name: name}
	a.byGPU[[16]byte(b)] = g
	a.order = append(a.order, g)
	return g
}

// reset starts a new window, keeping the last arrival.
func (a *hrArrivals) reset() {
	for _, g := range a.order {
		g.frames, g.count, g.maxUs, g.sum, g.sumSq = 0, 0, 0, 0, 0
	}
}

// summary formats per-GPU frames/s and interval p50/p99/max/stddev over d;
// empty when no partial frame arrived.
func (a *hrArrivals) summary(d time.Duration) string {
	var sb strings.Builder
	for _, g := range a.order {
		fmt.Fprintf(&sb, "; gpu %s: %.1f frames/s", g.name, float64(g.frames)/d.Seconds())
		if g.count == 0 {
			continue
		}
		n := int(min(g.count, hrArrivalRing))
		s := a.scratch[:n]
		copy(s, g.ring[:n])
		slices.Sort(s)
		mean := g.sum / float64(g.count)
		std := math.Sqrt(max(0, g.sumSq/float64(g.count)-mean*mean))
		fmt.Fprintf(&sb, " interval p50=%.3fms p99=%.3fms max=%.3fms jitter=%.3fms n=%d",
			float64(hrNearestRank(s, 0.50))/1e3, float64(hrNearestRank(s, 0.99))/1e3,
			float64(g.maxUs)/1e3, std/1e3, g.count)
	}
	return sb.String()
}

// hrNearestRank is the nearest-rank p-quantile of the sorted, non-empty s.
func hrNearestRank(s []uint32, p float64) uint32 {
	i := int(math.Ceil(p*float64(len(s)))) - 1
	return s[max(0, min(i, len(s)-1))]
}

// hrHotField is the per-(GPU, attribute) state of the frame-apply pipeline.
type hrHotField struct {
	cell      *snapshotstore.HotCell // existing metric name for this attr
	alias     *snapshotstore.HotCell // metric of the attr's alias (126 only)
	skip      bool                   // not in allowlist, or unsupported (cached)
	aliasSkip bool                   // the alias metric is not fed from this attr
	naSeen    bool                   // first N/A already logged
}

// hrHotGPU is the per-GPU (or partition) state of both frame kinds.
type hrHotGPU struct {
	// labels is shared, not cloned: published label maps are immutable
	labels    prometheus.Labels
	deviceKey string
	gpuID     string // ga.fl key
	fields    [hrMaxAttrID]hrHotField
	// pkgFrom126: attr 126 currently feeds gpu_package_power
	pkgFrom126 bool
}

// cell returns the hot cell attr id is written through (its alias's, if any).
func (g *hrHotGPU) cell(id amdgpu.GPUWatchAttrId) **snapshotstore.HotCell {
	if src := hrAttrTable[id].aliasedBy; src != amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_INVALID {
		return &g.fields[src].alias
	}
	return &g.fields[id].cell
}

// packagePowerRule: at sub-second periods a valid 126 (current_socket_power)
// feeds gpu_package_power instead of POWER_USAGE (5); an N/A 126 hands it
// back to 5. Reports whether to skip the write and whether an N/A must not
// mark the field unsupported.
func (g *hrHotGPU) packagePowerRule(id amdgpu.GPUWatchAttrId, valid bool) (drop, keepSupported bool) {
	src := &g.fields[hrAttrGPUPowerUsage]
	switch id {
	case hrAttrGPUPowerUsage:
		g.pkgFrom126 = valid && !src.aliasSkip
	case hrAttrPowerUsage:
		return g.pkgFrom126 && !src.aliasSkip, !src.aliasSkip
	}
	return false, false
}

// hrHotCache is the per-GPU state cache, rebuilt when an input changes.
type hrHotCache struct {
	epoch    uint64                        // store.Epoch() read before (re)building
	wlLabels *map[string]prometheus.Labels // hrWLCache map seen
	fieldGen uint64                        // ga.exportFieldGen seen
	byGPU    map[[16]byte]*hrHotGPU
	partial  hrPartialSink // reused by every partial frame (no allocation)
}

// hrSampleSink receives frame-apply output; the frame kinds differ only here.
type hrSampleSink interface {
	// carries reports whether the frame applies attributes of tier t.
	carries(t hrTier) bool
	// scalar writes v for g, through *cell where the sink uses hot cells.
	scalar(g *hrHotGPU, cell **snapshotstore.HotCell, name string, v float64)
	// element stages one array element; the sink owns labels.
	element(name string, labels prometheus.Labels, v float64)
}

// hrPartialSink writes the power tier through per-(GPU, field) hot cells: no
// lock or allocation in steady state.
type hrPartialSink struct {
	st   snapshotstore.StoreWriter
	tsUs int64
}

func (s *hrPartialSink) carries(t hrTier) bool {
	return t == hrTierPower
}

func (s *hrPartialSink) scalar(g *hrHotGPU, cell **snapshotstore.HotCell, name string, v float64) {
	if *cell == nil {
		*cell = s.st.RegisterHot(g.deviceKey, name, g.labels, v, s.tsUs)
		return
	}
	(*cell).Set(v, s.tsUs)
}

// element is never reached: no array-valued attribute is in the power tier.
func (s *hrPartialSink) element(string, prometheus.Labels, float64) {}

// hrFullSink stages full-frame samples as per-device deltas; fields with a
// hot cell go through it. Skips the power tier at sub-second periods.
type hrFullSink struct {
	sub    bool
	tsUs   int64
	skip   map[string]struct{} // workload label keys left out of the device key
	deltas map[string]map[string]snapshotstore.Sample
}

func (s *hrFullSink) carries(t hrTier) bool {
	return !s.sub || t != hrTierPower
}

func (s *hrFullSink) scalar(g *hrHotGPU, cell **snapshotstore.HotCell, name string, v float64) {
	if *cell != nil {
		(*cell).Set(v, s.tsUs)
		return
	}
	addHRStoreSample(s.deltas, name, maps.Clone(g.labels), v, s.tsUs, s.skip)
}

func (s *hrFullSink) element(name string, labels prometheus.Labels, v float64) {
	addHRStoreSample(s.deltas, name, labels, v, s.tsUs, s.skip)
}

// hrSubSecond reports an accepted watch period below 1000 ms (0 is not).
func (ga *GPUAgentGPUClient) hrSubSecond() bool {
	ms := ga.hrFastMs.Load()
	return ms != 0 && ms < highResFallbackFastPeriodMs
}

// hrWorkloadLabels returns the workload-label cache's map, nil if none.
func (ga *GPUAgentGPUClient) hrWorkloadLabels() *map[string]prometheus.Labels {
	if ga.hrWLCache == nil {
		return nil
	}
	return ga.hrWLCache.labels.Load()
}

// hrState returns the per-GPU state cache, rebuilt if stale.
func (ga *GPUAgentGPUClient) hrState(st snapshotstore.StoreWriter) *hrHotCache {
	c := ga.hrHot
	if wl := ga.hrWorkloadLabels(); c == nil || st.Epoch() != c.epoch ||
		wl != c.wlLabels || ga.exportFieldGen.Load() != c.fieldGen {
		c = ga.rebuildHot(st)
	}
	return c
}

// applyPowerFrame applies a partial frame; N/A is never written (nor as 0).
func (ga *GPUAgentGPUClient) applyPowerFrame(w *amdgpu.GPUWatch) {
	st := ga.gpuHandler.mh.GetStore()
	if st == nil || w == nil {
		return
	}
	c := ga.hrState(st)
	c.partial.st, c.partial.tsUs = st, time.Now().UnixMicro()
	ga.applyFrame(w, c, &c.partial)
}

// applyWatchSnapshot applies one full GPUWatch frame to the single-source
// store, keyed by the full label set. The store feeds /metrics, which is the
// only data path out of the exporter (there is no push stream). Returns the
// per-device deltas written (nil if none) so tests can assert what was stored.
//
// This must not touch the Prometheus registry: the legacy pull path resets and
// refills every gauge under prometheusMiddlewareMu, which this goroutine cannot
// take (it lives in pkg/exporter). The store is the single high-res writer target.
func (ga *GPUAgentGPUClient) applyWatchSnapshot(w *amdgpu.GPUWatch) map[string]map[string]snapshotstore.Sample {
	if w == nil || w.GetStats() == nil {
		return nil
	}
	st := ga.gpuHandler.mh.GetStore()
	if st == nil {
		return nil
	}
	sink := &hrFullSink{
		sub:    ga.hrSubSecond(),
		tsUs:   time.Now().UnixMicro(),
		skip:   ga.workloadLabelKeys(),
		deltas: map[string]map[string]snapshotstore.Sample{},
	}
	ga.applyFrame(w, ga.hrState(st), sink)
	if len(sink.deltas) == 0 {
		return nil
	}
	// key deltas by the stable label set, as the pull path does
	for key, fields := range sink.deltas {
		st.Update(key, fields)
	}
	return sink.deltas
}

// applyFrame is the pipeline shared by both frame kinds; the first N/A per
// (GPU, attr) is logged once via ga.fl, shared with the pull path.
func (ga *GPUAgentGPUClient) applyFrame(w *amdgpu.GPUWatch, c *hrHotCache, sink hrSampleSink) {
	carries := [...]bool{sink.carries(hrTierFull), sink.carries(hrTierPower)}
	for _, perGPU := range w.GetStats().GetGPUWatchAttr() {
		b := perGPU.GetGPU()
		if len(b) != 16 {
			continue
		}
		g := c.byGPU[[16]byte(b)]
		if g == nil {
			continue
		}
		for _, attr := range perGPU.GetAttr() {
			id := attr.GetId()
			if id < 0 || int(id) >= hrMaxAttrID {
				continue
			}
			m := &hrAttrTable[id]
			f := &g.fields[id]
			if m.name == "" || !carries[m.tier] || (f.skip && f.aliasSkip) {
				continue
			}
			if m.indexLabel != "" {
				ga.applyIndexed(g, f, m, attr.GetValue(), sink)
				continue
			}
			v, ok := watchAttrFloat(attr.GetValue())
			drop, keepSupported := g.packagePowerRule(id, ok)
			if !ok {
				if !keepSupported {
					ga.hrMarkNA(g, f, m.field)
				}
				continue
			}
			if drop {
				continue
			}
			if !f.skip {
				sink.scalar(g, g.cell(id), m.name, v)
			}
			if !f.aliasSkip {
				sink.scalar(g, &f.alias, hrAttrTable[m.alias].name, v)
			}
		}
	}
}

// applyIndexed fans an array-valued attribute out to one sample per element;
// no applicable element counts as N/A.
func (ga *GPUAgentGPUClient) applyIndexed(g *hrHotGPU, f *hrHotField, m *hrAttrInfo,
	val *amdgpu.GPUWatchAttrVal, sink hrSampleSink) {
	applicable := false
	for _, e := range val.GetIndexedVal() {
		v, ok := indexedValFloat(e)
		if !ok {
			continue
		}
		applicable = true
		// clone: labels is the shared cache entry
		lbl := maps.Clone(g.labels)
		lbl[m.indexLabel] = strconv.FormatUint(uint64(e.GetIndex()), 10)
		if m.hasSubType {
			if st := e.GetSubType(); st != "" {
				lbl["clock_type"] = st
			}
		}
		sink.element(m.name, lbl, v)
	}
	if !applicable {
		ga.hrMarkNA(g, f, m.field)
	}
}

// hrMarkNA records the first N/A of a (GPU, attr) in ga.fl, and stops applying
// the attr once ga.fl holds it unsupported.
func (ga *GPUAgentGPUClient) hrMarkNA(g *hrHotGPU, f *hrHotField, field string) {
	if f.naSeen {
		return
	}
	f.naSeen = true
	ga.fl.logUnsupportedField(g.gpuID, field)
	if ga.fl.checkUnsupportedFields(g.gpuID, field) {
		f.skip, f.aliasSkip = true, true
	}
}

// rebuildHot rebuilds the per-GPU state cache (slow path). The store epoch is
// read first so a racing store change is seen on the next frame.
func (ga *GPUAgentGPUClient) rebuildHot(st snapshotstore.StoreWriter) *hrHotCache {
	ga.hrHotRebuilds.Add(1)
	epoch := st.Epoch()
	wl := ga.hrWorkloadLabels()
	old := ga.hrHot
	c := &hrHotCache{
		epoch:    epoch,
		wlLabels: wl,
		fieldGen: ga.exportFieldGen.Load(),
		byGPU:    make(map[[16]byte]*hrHotGPU, len(ga.hrLabelsByUUID)),
	}
	keepCells := old != nil && old.epoch == epoch
	sameGen := old != nil && old.fieldGen == c.fieldGen
	sub := ga.hrSubSecond()
	skipKeys := ga.workloadLabelKeys()
	// allowlist state is per field, not per GPU
	var enabled [hrMaxAttrID]bool
	for _, m := range highResWatchMetrics {
		if m.name != "" {
			enabled[m.id] = ga.getExporterFieldState(m.name)
		}
	}
	for uuidStr, labels := range ga.hrLabelsByUUID {
		if wl != nil {
			if l, ok := (*wl)[uuidStr]; ok && l != nil {
				labels = l
			}
		}
		if labels == nil {
			continue
		}
		gu, err := uuid.Parse(uuidStr)
		if err != nil {
			continue
		}
		gpuID := ga.hrGPUIDByUUID[uuidStr]
		if gpuID == "" {
			// UUID keeps the unsupported-field dedup key distinct per GPU
			gpuID = uuidStr
		}
		g := &hrHotGPU{
			labels:    labels,
			deviceKey: gpuStoreDeviceKey(labels, skipKeys),
			gpuID:     gpuID,
		}
		var prev *hrHotGPU
		if old != nil {
			prev = old.byGPU[gu]
		}
		reuse := keepCells && prev != nil && prev.deviceKey == g.deviceKey &&
			maps.Equal(prev.labels, labels)
		for i := range g.fields {
			g.fields[i].skip, g.fields[i].aliasSkip = true, true
		}
		for _, m := range highResWatchMetrics {
			if m.name == "" {
				continue
			}
			unsupported := ga.fl.checkUnsupportedFields(gpuID, hrAttrTable[m.id].field)
			f := &g.fields[m.id]
			f.skip = !enabled[m.id] || unsupported
			// the alias metric's own N/A mark must not stop 126 feeding it
			if m.alias != amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_INVALID {
				f.aliasSkip = !sub || !enabled[m.alias] || unsupported
			}
			// after a field-config reload (fl reset) the first N/A logs again
			if prev != nil && sameGen {
				f.naSeen = prev.fields[m.id].naSeen
			}
			if reuse {
				f.cell, f.alias = prev.fields[m.id].cell, prev.fields[m.id].alias
			}
		}
		if prev != nil {
			g.pkgFrom126 = prev.pkgFrom126
		}
		c.byGPU[gu] = g
	}
	ga.hrHot = c
	return c
}

// addHRStoreSample accumulates one stream sample into the per-device store delta
// map, keyed by the stable label set (workload labels in skip omitted) so each GPU
// (and each indexed element, which adds an *_index label) lands in its own device
// slot, and a workload reassignment reuses that slot rather than orphaning it.
func addHRStoreSample(byDevice map[string]map[string]snapshotstore.Sample,
	base string, labels prometheus.Labels, val float64, tsUs int64, skip map[string]struct{}) {
	key := gpuStoreDeviceKey(labels, skip)
	if byDevice[key] == nil {
		byDevice[key] = map[string]snapshotstore.Sample{}
	}
	byDevice[key][base] = snapshotstore.Sample{Value: val, Labels: labels, TsUs: tsUs}
}

// watchAttrMetricName maps a GPUWatch attribute to its base metric name (no
// prefix). The configurable MetricsFieldPrefix is prepended by the caller.
// Empty means the attribute is not streamed.
func watchAttrMetricName(id amdgpu.GPUWatchAttrId) string {
	return highResWatchNames[id]
}

// watchAttrFloat extracts a float64 from a GPUWatch attribute value regardless
// of its oneof arm. Returns false when the attribute is absent or carries the
// NA sentinel, in which case the caller must publish nothing for it.
func watchAttrFloat(v *amdgpu.GPUWatchAttrVal) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.GetWatchAttrVal().(type) {
	case *amdgpu.GPUWatchAttrVal_LongVal:
		// gpuagent reports an unsupported field as the datatype-max NA
		// sentinel; publishing it would surface 1.8446744e+19 as a reading
		if !utils.IsValueApplicable(val.LongVal) {
			return 0, false
		}
		return float64(val.LongVal), true
	case *amdgpu.GPUWatchAttrVal_FloatVal:
		if !utils.IsValueApplicable(val.FloatVal) {
			return 0, false
		}
		return float64(val.FloatVal), true
	default:
		return 0, false
	}
}

// indexedValFloat extracts a float64 from one element of an array-valued
// attribute. Returns false for a nil element or the NA sentinel, so the caller
// emits nothing for that element.
func indexedValFloat(e *amdgpu.GPUWatchAttrIndexedVal) (float64, bool) {
	if e == nil {
		return 0, false
	}
	switch v := e.GetIndexedAttrVal().(type) {
	case *amdgpu.GPUWatchAttrIndexedVal_LongVal:
		if !utils.IsValueApplicable(v.LongVal) {
			return 0, false
		}
		return float64(v.LongVal), true
	case *amdgpu.GPUWatchAttrIndexedVal_FloatVal:
		if !utils.IsValueApplicable(v.FloatVal) {
			return 0, false
		}
		return float64(v.FloatVal), true
	default:
		return 0, false
	}
}
