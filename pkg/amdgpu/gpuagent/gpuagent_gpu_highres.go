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
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
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
// it feeds. An empty name means subscribe but do not stream yet.
type highResWatchMetric struct {
	id   amdgpu.GPUWatchAttrId
	name string
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
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_CLOCK, ""},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MEM_CLOCK, ""},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_CURRENT, "gpu_clock"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_MIN, "gpu_min_clock"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_MAX, "gpu_max_clock"},

	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, "gpu_junction_temperature"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MEMORY_TEMP, "gpu_memory_temperature"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, "gpu_package_power"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_UTIL, "gpu_gfx_activity"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UMC_ACTIVITY, "gpu_umc_activity"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MM_ACTIVITY, "gpu_mma_activity"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_AVG_POWER, "gpu_average_package_power"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE, "gpu_gfx_voltage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_SOC_VOLTAGE, "gpu_voltage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MEM_VOLTAGE, "gpu_memory_voltage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_BANDWIDTH, "pcie_bidirectional_bandwidth"},
	// amdsmi reports VRAM usage in MB, matching the gauge's declared unit
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_MEMORY_USAGE, "gpu_used_vram"},

	// ECC: attribute suffix CE/UE maps to field infix CORRECT/UNCORRECT, and the
	// PCIE_BIF attribute shortens to BIF in the field name
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_CORRECT_TOTAL, "gpu_ecc_correct_total"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_UNCORRECT_TOTAL, "gpu_ecc_uncorrect_total"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SDMA_CE, "gpu_ecc_correct_sdma"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SDMA_UE, "gpu_ecc_uncorrect_sdma"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_GFX_CE, "gpu_ecc_correct_gfx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_GFX_UE, "gpu_ecc_uncorrect_gfx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MMHUB_CE, "gpu_ecc_correct_mmhub"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MMHUB_UE, "gpu_ecc_uncorrect_mmhub"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_ATHUB_CE, "gpu_ecc_correct_athub"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_ATHUB_UE, "gpu_ecc_uncorrect_athub"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_PCIE_BIF_CE, "gpu_ecc_correct_bif"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_PCIE_BIF_UE, "gpu_ecc_uncorrect_bif"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_HDP_CE, "gpu_ecc_correct_hdp"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_HDP_UE, "gpu_ecc_uncorrect_hdp"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_XGMI_WAFL_CE, "gpu_ecc_correct_xgmi_wafl"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_XGMI_WAFL_UE, "gpu_ecc_uncorrect_xgmi_wafl"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DF_CE, "gpu_ecc_correct_df"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DF_UE, "gpu_ecc_uncorrect_df"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SMN_CE, "gpu_ecc_correct_smn"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SMN_UE, "gpu_ecc_uncorrect_smn"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SEM_CE, "gpu_ecc_correct_sem"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SEM_UE, "gpu_ecc_uncorrect_sem"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP0_CE, "gpu_ecc_correct_mp0"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP0_UE, "gpu_ecc_uncorrect_mp0"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP1_CE, "gpu_ecc_correct_mp1"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MP1_UE, "gpu_ecc_uncorrect_mp1"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_FUSE_CE, "gpu_ecc_correct_fuse"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_FUSE_UE, "gpu_ecc_uncorrect_fuse"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_UMC_CE, "gpu_ecc_correct_umc"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_UMC_UE, "gpu_ecc_uncorrect_umc"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MCA_CE, "gpu_ecc_correct_mca"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MCA_UE, "gpu_ecc_uncorrect_mca"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_VCN_CE, "gpu_ecc_correct_vcn"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_VCN_UE, "gpu_ecc_uncorrect_vcn"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_JPEG_CE, "gpu_ecc_correct_jpeg"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_JPEG_UE, "gpu_ecc_uncorrect_jpeg"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_IH_CE, "gpu_ecc_correct_ih"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_IH_UE, "gpu_ecc_uncorrect_ih"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MPIO_CE, "gpu_ecc_correct_mpio"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_MPIO_UE, "gpu_ecc_uncorrect_mpio"},

	// ECC deferred counters (third category alongside correct/uncorrect)
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_TOTAL, "gpu_ecc_deferred_total"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_SDMA, "gpu_ecc_deferred_sdma"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_GFX, "gpu_ecc_deferred_gfx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MMHUB, "gpu_ecc_deferred_mmhub"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_ATHUB, "gpu_ecc_deferred_athub"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_PCIE_BIF, "gpu_ecc_deferred_bif"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_HDP, "gpu_ecc_deferred_hdp"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_XGMI_WAFL, "gpu_ecc_deferred_xgmi_wafl"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_DF, "gpu_ecc_deferred_df"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_SMN, "gpu_ecc_deferred_smn"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_SEM, "gpu_ecc_deferred_sem"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MP0, "gpu_ecc_deferred_mp0"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MP1, "gpu_ecc_deferred_mp1"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_FUSE, "gpu_ecc_deferred_fuse"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_UMC, "gpu_ecc_deferred_umc"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MCA, "gpu_ecc_deferred_mca"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_VCN, "gpu_ecc_deferred_vcn"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_JPEG, "gpu_ecc_deferred_jpeg"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_IH, "gpu_ecc_deferred_ih"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_DEF_MPIO, "gpu_ecc_deferred_mpio"},

	// PCIe accumulated counters + additional bulk scalars
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_REPLAY_COUNT, "pcie_replay_count"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_RECOVERY_COUNT, "pcie_recovery_count"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_REPLAY_ROLLOVER_COUNT, "pcie_replay_rollover_count"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_NACK_SENT_COUNT, "pcie_nack_sent_count"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_NACK_RECEIVED_COUNT, "pcie_nack_received_count"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_POWER_USAGE, "gpu_power_usage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VRAM_MAX_BANDWIDTH, "gpu_vram_max_bandwidth"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_EDGE_TEMP, "gpu_edge_temperature"},
	// energy accumulator (uJ): fast-tier from the bulk gpu_metrics read, so it
	// leaves the GPUGet non-watch carry set and updates every FastPeriodSecs
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ENERGY_CONSUMED, "gpu_energy_consumed"},

	// memory totals/used/free and PCIe link speed
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_TOTAL_VRAM, "gpu_total_vram"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_TOTAL_VISIBLE_VRAM, "gpu_total_visible_vram"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_TOTAL_GTT, "gpu_total_gtt"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_USED_VISIBLE_VRAM, "gpu_used_visible_vram"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_USED_GTT, "gpu_used_gtt"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FREE_VRAM, "gpu_free_vram"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FREE_VISIBLE_VRAM, "gpu_free_visible_vram"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FREE_GTT, "gpu_free_gtt"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_SPEED, "pcie_speed"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_PCIE_MAX_SPEED, "pcie_max_speed"},

	// XGMI: the attribute drops the NBR infix the field name carries, and the
	// per-link throughput attribute spells TX after the index
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_NOP_TX, "gpu_xgmi_nbr_0_nop_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_REQ_TX, "gpu_xgmi_nbr_0_req_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_RESP_TX, "gpu_xgmi_nbr_0_resp_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_BEATS_TX, "gpu_xgmi_nbr_0_beats_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_NOP_TX, "gpu_xgmi_nbr_1_nop_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_REQ_TX, "gpu_xgmi_nbr_1_req_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_RESP_TX, "gpu_xgmi_nbr_1_resp_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_BEATS_TX, "gpu_xgmi_nbr_1_beats_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_0_THRPUT, "gpu_xgmi_nbr_0_tx_thrput"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_1_THRPUT, "gpu_xgmi_nbr_1_tx_thrput"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_2_THRPUT, "gpu_xgmi_nbr_2_tx_thrput"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_3_THRPUT, "gpu_xgmi_nbr_3_tx_thrput"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_4_THRPUT, "gpu_xgmi_nbr_4_tx_thrput"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_5_THRPUT, "gpu_xgmi_nbr_5_tx_thrput"},

	// flat (non-indexed) violation counters: scalar path, like ECC/XGMI above
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_CURRENT_ACC_COUNTER, "gpu_violation_current_accumulated_counter"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PROC_HOT_RESIDENCY_ACC, "gpu_violation_processor_hot_residency_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PPT_RESIDENCY_ACC, "gpu_violation_ppt_residency_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACC, "gpu_violation_socket_thermal_residency_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_VR_THERMAL_RESIDENCY_ACC, "gpu_violation_vr_thermal_residency_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_HBM_THERMAL_RESIDENCY_ACC, "gpu_violation_hbm_thermal_residency_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PROC_HOT_RESIDENCY_PCT, "gpu_violation_processor_hot_residency_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PPT_RESIDENCY_PCT, "gpu_violation_ppt_residency_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_SOCKET_THERMAL_RESIDENCY_PCT, "gpu_violation_socket_thermal_residency_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_VR_THERMAL_RESIDENCY_PCT, "gpu_violation_vr_thermal_residency_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_HBM_THERMAL_RESIDENCY_PCT, "gpu_violation_hbm_thermal_residency_percentage"},

	// array-valued (indexed) attributes: the value rides GPUWatchAttrVal.IndexedVal
	// and applyWatchSnapshot fans out one sample per element, adding the family's
	// *_index label (see highResIndexedMeta). Names equal the pull-path GaugeVec
	// names so the series are identical whether scraped or streamed.
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_HBM_TEMP, "gpu_hbm_temperature"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VCN_ACTIVITY, "gpu_vcn_activity"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_ACTIVITY, "gpu_jpeg_activity"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_RX, "gpu_xgmi_link_rx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_TX, "gpu_xgmi_link_tx"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_POWER_ACC, "gpu_violation_gfx_clock_below_host_limit_power_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_THERMAL_ACC, "gpu_violation_gfx_clock_below_host_limit_thermal_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_LOW_UTIL_ACC, "gpu_violation_low_utilization_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_TOTAL_ACC, "gpu_violation_gfx_clock_below_host_limit_total_accumulated"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_POWER_PCT, "gpu_violation_gfx_clock_below_host_limit_power_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_THERMAL_PCT, "gpu_violation_gfx_clock_below_host_limit_thermal_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_LOW_UTIL_PCT, "gpu_violation_low_utilization_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_TOTAL_PCT, "gpu_violation_gfx_clock_below_host_limit_total_percentage"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_BUSY_INST, "gpu_gfx_busy_instantaneous"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VCN_BUSY_INST, "gpu_vcn_busy_instantaneous"},
	{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_BUSY_INST, "gpu_jpeg_busy_instantaneous"},
}

// highResIndexedMeta maps an array-valued attribute to the per-element index
// label its pull-path GaugeVec carries, and whether SubType feeds a clock_type
// label. Scalars are absent and resolve to ("", false), so applyWatchSnapshot
// treats them on the scalar path.
var highResIndexedMeta = map[amdgpu.GPUWatchAttrId]struct {
	indexLabel string
	hasSubType bool
}{
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_HBM_TEMP:                                       {"hbm_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VCN_ACTIVITY:                                   {"vcn_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_ACTIVITY:                                  {"jpeg_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_RX:                                   {"link_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_TX:                                   {"link_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_POWER_ACC:   {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_THERMAL_ACC: {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_LOW_UTIL_ACC:                         {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_TOTAL_ACC:   {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_POWER_PCT:   {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_THERMAL_PCT: {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_LOW_UTIL_PCT:                         {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_GFX_CLK_BELOW_HOST_LIMIT_TOTAL_PCT:   {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_BUSY_INST:                                  {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VCN_BUSY_INST:                                  {"xcc_index", false},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_BUSY_INST:                                 {"xcc_index", false},
	// clocks carry both clock_index and a clock_type sub-label (from SubType)
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_CURRENT: {"clock_index", true},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_MIN:     {"clock_index", true},
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_MAX:     {"clock_index", true},
}

// watchAttrIndexLabel returns the per-element index label for an array-valued
// attribute and whether it also carries a clock_type sub-label. Scalars return
// ("", false).
func watchAttrIndexLabel(id amdgpu.GPUWatchAttrId) (string, bool) {
	m, ok := highResIndexedMeta[id]
	if !ok {
		return "", false
	}
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
	desired := ga.gpuHandler.mh.IsHighResolution()
	running := ga.hrCancel != nil
	switch {
	case desired && !running:
		hrCtx, cancel := context.WithCancel(ga.gpuHandler.ctx)
		done := make(chan struct{})
		ga.hrCancel = cancel
		ga.hrDone = done
		go ga.runHighResFeed(hrCtx, done)
		logger.Log.Printf("high-resolution ENABLED: GPUWatch feed started (gpuagent SMI persistent)")
	case !desired && running:
		ga.teardownHighResLocked()
		logger.Log.Printf("high-resolution DISABLED: GPUWatch feed stopped, watch deleted (gpuagent returns to lazy SMI)")
	}
	// Reconcile the profiler stream under the same lock, symmetric with the watch
	// feed: its gate additionally requires profiler enabled and a non-PTL platform.
	ga.reconcileProfilerStreamLocked()
}

// highResReconnectBackoff is the wait between a GPUWatch stream failure and the
// next reconnect attempt, bounding the churn when gpuagent is down.
const highResReconnectBackoff = 5 * time.Second

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
	ga.hrSnapshot.Store(&hrGPUSnap{gpus: getResp.GetResponse(), partitionMap: partitionMap})
	ga.initGPUMetadata(getResp.GetResponse())
	ga.hrLabelsByUUID = make(map[string]prometheus.Labels)
	ga.hrGPUIDByUUID = make(map[string]string)
	// Store-serving only: seed the subscribe-time label set with real workloads so
	// the first streamed frame is workload-aware; flag-off keeps nil (static set,
	// unchanged stream labels).
	storeServing := ga.useHighResStoreServing()
	var seedWls map[string]scheduler.Workload
	if storeServing {
		seedWls, _ = ga.gpuHandler.ListWorkloads()
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
	fastSecs, slowSecs := ga.gpuHandler.mh.GetHighResPeriods()
	spec := &amdgpu.GPUWatchSpec{
		Id:             watchID[:],
		GPU:            gpuIDs,
		Attribute:      highResWatchAttrs,
		FastPeriodSecs: fastSecs,
		SlowPeriodSecs: slowSecs,
	}
	if _, err := ga.watchclient.GPUWatchCreate(ctx,
		&amdgpu.GPUWatchRequest{Spec: []*amdgpu.GPUWatchSpec{spec}}); err != nil {
		return err
	}
	stream, err := ga.watchclient.GPUWatchSubscribe(ctx,
		&amdgpu.GPUWatchSubscribeRequest{Id: [][]byte{watchID[:]}})
	if err != nil {
		return err
	}
	logger.Log.Printf("high-res: GPUWatch subscribed for %d GPU(s), %d attrs",
		len(gpuIDs), len(highResWatchAttrs))

	// Consume inline: blocks until the stream fails or ctx is cancelled, returning
	// control to runHighResFeed to drop stale telemetry and reconnect.
	ga.consumeWatchStream(ctx, stream)
	return nil
}

// consumeWatchStream reads GPUWatch snapshots until the stream errors or ctx is
// cancelled, applying each to the store, and logging the observed inter-sample
// cadence so the achievable high-resolution rate can be measured on hardware.
func (ga *GPUAgentGPUClient) consumeWatchStream(ctx context.Context,
	stream amdgpu.GPUWatchSvc_GPUWatchSubscribeClient) {
	var last time.Time
	var count uint64
	for ctx.Err() == nil {
		w, err := stream.Recv()
		if err != nil {
			logger.Log.Printf("high-res: GPUWatch stream ended: %v", err)
			return
		}
		now := time.Now()
		if !last.IsZero() {
			// Cadence trace at debug only — at 1s it floods info logs otherwise.
			if count%10 == 0 {
				logger.Log.Debugf("high-res: GPUWatch sample interval %v (sample #%d)",
					now.Sub(last), count)
			}
		}
		last = now
		count++
		ga.applyWatchSnapshot(w)
	}
}

// applyWatchSnapshot applies one GPUWatch snapshot to the single-source store:
// one Sample per (GPU/partition, attribute), keyed by the full label set. The
// store feeds /metrics, which is the only data path out of the exporter (there
// is no push stream). Returns the per-device deltas written (nil if nothing was
// applied) so tests can assert what was stored.
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
	var store snapshotstore.StoreWriter = st
	now := time.Now().UnixMicro()
	workloadSkip := ga.workloadLabelKeys()
	storeByDevice := map[string]map[string]snapshotstore.Sample{}
	for _, perGPU := range w.GetStats().GetGPUWatchAttr() {
		guid, err := uuid.FromBytes(perGPU.GetGPU())
		if err != nil {
			continue
		}
		uuidStr := guid.String()
		// Prefer the async workload-label cache (live pod/slurm labels), falling
		// back to the subscribe-time static set.
		var labels prometheus.Labels
		var ok bool
		if ga.hrWLCache != nil {
			labels, ok = ga.hrWLCache.get(uuidStr)
		}
		if !ok || labels == nil {
			labels, ok = ga.hrLabelsByUUID[uuidStr]
		}
		if !ok || labels == nil {
			continue
		}
		gpuid := ga.hrGPUIDByUUID[uuidStr]
		if gpuid == "" {
			// fall back to the UUID so the unsupported-field log/dedup key is
			// distinct per GPU instead of an empty string shared across all
			gpuid = uuidStr
		}
		for _, attr := range perGPU.GetAttr() {
			base := watchAttrMetricName(attr.GetId())
			if base == "" {
				continue
			}
			// Gauge base names are the lowercased GPUMetricField enum names, so
			// upcasing yields the same ga.fl key the pull path uses: a field
			// unsupported in one path is then unsupported in both, logged once.
			field := strings.ToUpper(base)
			if ga.fl.checkUnsupportedFields(gpuid, field) {
				continue
			}
			// Honor the config field-gate so a disabled field is absent from
			// /metrics (the single output), matching the pull path.
			if !ga.getExporterFieldState(field) {
				continue
			}
			// array-valued attribute: fan out one sample per element, adding the
			// family's *_index label (and clock_type for clocks). Branch before
			// watchAttrFloat, whose scalar oneof would otherwise ignore IndexedVal.
			if indexLabel, hasSubType := watchAttrIndexLabel(attr.GetId()); indexLabel != "" {
				applicable := false
				for _, e := range attr.GetValue().GetIndexedVal() {
					v, ok := indexedValFloat(e)
					if !ok {
						continue
					}
					applicable = true
					// Clone per element: labels aliases the shared cache entry, and
					// the index (and clock_type) key must not leak across samples.
					lbl := maps.Clone(labels)
					lbl[indexLabel] = strconv.FormatUint(uint64(e.GetIndex()), 10)
					if hasSubType {
						if st := e.GetSubType(); st != "" {
							lbl["clock_type"] = st
						}
					}
					addHRStoreSample(storeByDevice, base, lbl, v, now, workloadSkip)
				}
				// no applicable element means the field is unsupported on this GPU;
				// log once, exactly as the scalar NA path does
				if !applicable {
					ga.fl.logUnsupportedField(gpuid, field)
				}
				continue
			}
			val, ok := watchAttrFloat(attr.GetValue())
			if !ok {
				ga.fl.logUnsupportedField(gpuid, field)
				continue
			}
			lbl := maps.Clone(labels)
			addHRStoreSample(storeByDevice, base, lbl, val, now, workloadSkip)
		}
	}
	if len(storeByDevice) == 0 {
		return nil
	}
	// Apply per-device deltas, keyed by the stable label set (same as the pull-path
	// store writer) so indexed families stay distinct and a workload change reuses
	// the slot.
	for key, fields := range storeByDevice {
		store.Update(key, fields)
	}
	return storeByDevice
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
