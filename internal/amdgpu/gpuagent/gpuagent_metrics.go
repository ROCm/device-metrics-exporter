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
	"math"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/ROCm/device-metrics-exporter/internal/k8s"
	"github.com/ROCm/device-metrics-exporter/internal/slurm"

	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/gen/gpumetrics"
	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/logger"
	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/parserutil"
	"github.com/gofrs/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// local variables
var (
	mandatoryLables = []string{
		gpumetrics.GPUMetricLabel_GPU_UUID.String(),
		gpumetrics.GPUMetricLabel_SERIAL_NUMBER.String(),
		gpumetrics.GPUMetricLabel_CARD_MODEL.String(),
	}
	exportLables    map[string]bool
	exportFieldMap  map[string]bool
	fieldMetricsMap []prometheus.Collector
	gpuSelectorMap  map[int]bool
)

type metrics struct {
	gpuNodesTotal              prometheus.Gauge
	gpuPackagePower            prometheus.GaugeVec
	gpuAvgPkgPower             prometheus.GaugeVec
	gpuEdgeTemp                prometheus.GaugeVec
	gpuJunctionTemp            prometheus.GaugeVec
	gpuMemoryTemp              prometheus.GaugeVec
	gpuHBMTemp                 prometheus.GaugeVec
	gpuGFXActivity             prometheus.GaugeVec
	gpuUMCActivity             prometheus.GaugeVec
	gpuMMAActivity             prometheus.GaugeVec
	gpuVCNActivity             prometheus.GaugeVec
	gpuJPEGActivity            prometheus.GaugeVec
	gpuVoltage                 prometheus.GaugeVec
	gpuGFXVoltage              prometheus.GaugeVec
	gpuMemVoltage              prometheus.GaugeVec
	gpuPCIeSpeed               prometheus.GaugeVec
	gpuPCIeMaxSpeed            prometheus.GaugeVec
	gpuPCIeBandwidth           prometheus.GaugeVec
	gpuEnergyConsumed          prometheus.GaugeVec
	gpuPCIeReplayCount         prometheus.GaugeVec
	gpuPCIeRecoveryCount       prometheus.GaugeVec
	gpuPCIeReplayRolloverCount prometheus.GaugeVec
	gpuPCIeNACKSentCount       prometheus.GaugeVec
	gpuPCIeNACKReceivedCount   prometheus.GaugeVec
	gpuClock                   prometheus.GaugeVec
	gpuPowerUsage              prometheus.GaugeVec

	gpuEccCorrectTotal      prometheus.GaugeVec
	gpuEccUncorrectTotal    prometheus.GaugeVec
	gpuEccCorrectSDMA       prometheus.GaugeVec
	gpuEccUncorrectSDMA     prometheus.GaugeVec
	gpuEccCorrectGFX        prometheus.GaugeVec
	gpuEccUncorrectGFX      prometheus.GaugeVec
	gpuEccCorrectMMHUB      prometheus.GaugeVec
	gpuEccUncorrectMMHUB    prometheus.GaugeVec
	gpuEccCorrectATHUB      prometheus.GaugeVec
	gpuEccUncorrectATHUB    prometheus.GaugeVec
	gpuEccCorrectBIF        prometheus.GaugeVec
	gpuEccUncorrectBIF      prometheus.GaugeVec
	gpuEccCorrectHDP        prometheus.GaugeVec
	gpuEccUncorrectHDP      prometheus.GaugeVec
	gpuEccCorrectXgmiWAFL   prometheus.GaugeVec
	gpuEccUncorrectXgmiWAFL prometheus.GaugeVec
	gpuEccCorrectDF         prometheus.GaugeVec
	gpuEccUncorrectDF       prometheus.GaugeVec
	gpuEccCorrectSMN        prometheus.GaugeVec
	gpuEccUncorrectSMN      prometheus.GaugeVec
	gpuEccCorrectSEM        prometheus.GaugeVec
	gpuEccUncorrectSEM      prometheus.GaugeVec
	gpuEccCorrectMP0        prometheus.GaugeVec
	gpuEccUncorrectMP0      prometheus.GaugeVec
	gpuEccCorrectMP1        prometheus.GaugeVec
	gpuEccUncorrectMP1      prometheus.GaugeVec
	gpuEccCorrectFUSE       prometheus.GaugeVec
	gpuEccUncorrectFUSE     prometheus.GaugeVec
	gpuEccCorrectUMC        prometheus.GaugeVec
	gpuEccUncorrectUMC      prometheus.GaugeVec
	xgmiNbrNopTx0           prometheus.GaugeVec
	xgmiNbrReqTx0           prometheus.GaugeVec
	xgmiNbrRespTx0          prometheus.GaugeVec
	xgmiNbrBeatsTx0         prometheus.GaugeVec
	xgmiNbrNopTx1           prometheus.GaugeVec
	xgmiNbrReqTx1           prometheus.GaugeVec
	xgmiNbrRespTx1          prometheus.GaugeVec
	xgmiNbrBeatsTx1         prometheus.GaugeVec
	xgmiNbrTxTput0          prometheus.GaugeVec
	xgmiNbrTxTput1          prometheus.GaugeVec
	xgmiNbrTxTput2          prometheus.GaugeVec
	xgmiNbrTxTput3          prometheus.GaugeVec
	xgmiNbrTxTput4          prometheus.GaugeVec
	xgmiNbrTxTput5          prometheus.GaugeVec

	gpuTotalVram prometheus.GaugeVec
	gpuUsedVram  prometheus.GaugeVec
	gpuFreeVram  prometheus.GaugeVec

	gpuTotalVisibleVram prometheus.GaugeVec
	gpuUsedVisibleVram  prometheus.GaugeVec
	gpuFreeVisibleVram  prometheus.GaugeVec

	gpuTotalGTT prometheus.GaugeVec
	gpuUsedGTT  prometheus.GaugeVec
	gpuFreeGTT  prometheus.GaugeVec

	gpuEccCorrectMCA   prometheus.GaugeVec
	gpuEccUncorrectMCA prometheus.GaugeVec

	gpuEccCorrectVCN   prometheus.GaugeVec
	gpuEccUncorrectVCN prometheus.GaugeVec

	gpuEccCorrectJPEG   prometheus.GaugeVec
	gpuEccUncorrectJPEG prometheus.GaugeVec

	gpuEccCorrectIH   prometheus.GaugeVec
	gpuEccUncorrectIH prometheus.GaugeVec

	gpuEccCorrectMPIO   prometheus.GaugeVec
	gpuEccUncorrectMPIO prometheus.GaugeVec
}

func (ga *GPUAgentClient) ResetMetrics() error {
	// reset all label based fields
	ga.m.gpuPackagePower.Reset()
	ga.m.gpuAvgPkgPower.Reset()
	ga.m.gpuEdgeTemp.Reset()
	ga.m.gpuJunctionTemp.Reset()
	ga.m.gpuMemoryTemp.Reset()
	ga.m.gpuHBMTemp.Reset()
	ga.m.gpuGFXActivity.Reset()
	ga.m.gpuUMCActivity.Reset()
	ga.m.gpuMMAActivity.Reset()
	ga.m.gpuVCNActivity.Reset()
	ga.m.gpuJPEGActivity.Reset()
	ga.m.gpuVoltage.Reset()
	ga.m.gpuGFXVoltage.Reset()
	ga.m.gpuMemVoltage.Reset()
	ga.m.gpuPCIeSpeed.Reset()
	ga.m.gpuPCIeMaxSpeed.Reset()
	ga.m.gpuPCIeBandwidth.Reset()
	ga.m.gpuEnergyConsumed.Reset()
	ga.m.gpuPCIeReplayCount.Reset()
	ga.m.gpuPCIeRecoveryCount.Reset()
	ga.m.gpuPCIeReplayRolloverCount.Reset()
	ga.m.gpuPCIeNACKSentCount.Reset()
	ga.m.gpuPCIeNACKReceivedCount.Reset()
	ga.m.gpuClock.Reset()
	ga.m.gpuPowerUsage.Reset()
	ga.m.gpuTotalVram.Reset()
	ga.m.gpuUsedVram.Reset()
	ga.m.gpuFreeVram.Reset()
	ga.m.gpuTotalVisibleVram.Reset()
	ga.m.gpuUsedVisibleVram.Reset()
	ga.m.gpuFreeVisibleVram.Reset()
	ga.m.gpuTotalGTT.Reset()
	ga.m.gpuUsedGTT.Reset()
	ga.m.gpuFreeGTT.Reset()
	ga.m.gpuEccCorrectTotal.Reset()
	ga.m.gpuEccUncorrectTotal.Reset()
	ga.m.gpuEccCorrectSDMA.Reset()
	ga.m.gpuEccUncorrectSDMA.Reset()
	ga.m.gpuEccCorrectGFX.Reset()
	ga.m.gpuEccUncorrectGFX.Reset()
	ga.m.gpuEccCorrectMMHUB.Reset()
	ga.m.gpuEccUncorrectMMHUB.Reset()
	ga.m.gpuEccCorrectATHUB.Reset()
	ga.m.gpuEccUncorrectATHUB.Reset()
	ga.m.gpuEccCorrectBIF.Reset()
	ga.m.gpuEccUncorrectBIF.Reset()
	ga.m.gpuEccCorrectHDP.Reset()
	ga.m.gpuEccUncorrectHDP.Reset()
	ga.m.gpuEccCorrectXgmiWAFL.Reset()
	ga.m.gpuEccUncorrectXgmiWAFL.Reset()
	ga.m.gpuEccCorrectDF.Reset()
	ga.m.gpuEccUncorrectDF.Reset()
	ga.m.gpuEccCorrectSMN.Reset()
	ga.m.gpuEccUncorrectSMN.Reset()
	ga.m.gpuEccCorrectSEM.Reset()
	ga.m.gpuEccUncorrectSEM.Reset()
	ga.m.gpuEccCorrectMP0.Reset()
	ga.m.gpuEccUncorrectMP0.Reset()
	ga.m.gpuEccCorrectMP1.Reset()
	ga.m.gpuEccUncorrectMP1.Reset()
	ga.m.gpuEccCorrectFUSE.Reset()
	ga.m.gpuEccUncorrectFUSE.Reset()
	ga.m.gpuEccCorrectUMC.Reset()
	ga.m.gpuEccUncorrectUMC.Reset()
	ga.m.xgmiNbrNopTx0.Reset()
	ga.m.xgmiNbrReqTx0.Reset()
	ga.m.xgmiNbrRespTx0.Reset()
	ga.m.xgmiNbrBeatsTx0.Reset()
	ga.m.xgmiNbrNopTx1.Reset()
	ga.m.xgmiNbrReqTx1.Reset()
	ga.m.xgmiNbrRespTx1.Reset()
	ga.m.xgmiNbrBeatsTx1.Reset()
	ga.m.xgmiNbrTxTput0.Reset()
	ga.m.xgmiNbrTxTput1.Reset()
	ga.m.xgmiNbrTxTput2.Reset()
	ga.m.xgmiNbrTxTput3.Reset()
	ga.m.xgmiNbrTxTput4.Reset()
	ga.m.xgmiNbrTxTput5.Reset()
	ga.m.gpuEccCorrectMCA.Reset()
	ga.m.gpuEccUncorrectMCA.Reset()
	ga.m.gpuEccCorrectVCN.Reset()
	ga.m.gpuEccUncorrectVCN.Reset()
	ga.m.gpuEccCorrectJPEG.Reset()
	ga.m.gpuEccUncorrectJPEG.Reset()
	ga.m.gpuEccCorrectIH.Reset()
	ga.m.gpuEccUncorrectIH.Reset()
	ga.m.gpuEccCorrectMPIO.Reset()
	ga.m.gpuEccUncorrectMPIO.Reset()
	return nil
}

func (ga *GPUAgentClient) GetExportLabels() []string {
	labelList := []string{}
	for key, enabled := range exportLables {
		if !enabled {
			continue
		}
		labelList = append(labelList, strings.ToLower(key))
	}
	return labelList
}

func (ga *GPUAgentClient) initLabelConfigs(config *gpumetrics.GPUMetricConfig) {

	// list of mandatory labels
	exportLables = make(map[string]bool)
	for _, name := range gpumetrics.GPUMetricLabel_name {
		exportLables[name] = false
	}
	// only mandatory labels are set for default
	for _, name := range mandatoryLables {
		exportLables[name] = true
	}

	if config != nil {
		for _, name := range config.GetLabels() {
			name = strings.ToUpper(name)
			if _, ok := exportLables[name]; ok {
				if _, ok := k8s.ExportLabels[name]; ok && !ga.isKubernetes {
					continue
				}
				logger.Log.Printf("label %v enabled", name)
				exportLables[name] = true
			}
		}
	}
	logger.Log.Printf("export-labels updated to %v", exportLables)
}

func initGPUSelectorConfig(config *gpumetrics.GPUMetricConfig) {
	if config != nil && config.GetSelector() != "" {
		selector := config.GetSelector()
		indices, err := parserutil.RangeStrToIntIndices(selector)
		if err != nil {
			logger.Log.Printf("GPUConfig.Selector parsing err :%v", err)
			logger.Log.Printf("monitoring all gpu instances")
			return
		}
		for _, ins := range indices {
			gpuSelectorMap[ins] = true
		}
	}
}

func initFieldConfig(config *gpumetrics.GPUMetricConfig) {
	exportFieldMap = make(map[string]bool)
	// setup metric fields in map to be monitored
	// init the map with all supported strings from enum
	enable_default := true
	if config != nil && len(config.GetFields()) != 0 {
		enable_default = false
	}
	for _, name := range gpumetrics.GPUMetricField_name {
		logger.Log.Printf("%v set to %v", name, enable_default)
		exportFieldMap[name] = enable_default
	}
	if config == nil || len(config.GetFields()) == 0 {
		return
	}
	for _, fieldName := range config.GetFields() {
		fieldName = strings.ToUpper(fieldName)
		if _, ok := exportFieldMap[fieldName]; ok {
			logger.Log.Printf("%v enabled", fieldName)
			exportFieldMap[fieldName] = true
		}
	}
	return
}

func (ga *GPUAgentClient) initFieldMetricsMap() {
	// must follow index mapping to fields.proto (GPUMetricField)
	fieldMetricsMap = []prometheus.Collector{
		ga.m.gpuNodesTotal,
		ga.m.gpuPackagePower,
		ga.m.gpuAvgPkgPower,
		ga.m.gpuEdgeTemp,
		ga.m.gpuJunctionTemp,
		ga.m.gpuMemoryTemp,
		ga.m.gpuHBMTemp,
		ga.m.gpuGFXActivity,
		ga.m.gpuUMCActivity,
		ga.m.gpuMMAActivity,
		ga.m.gpuVCNActivity,
		ga.m.gpuJPEGActivity,
		ga.m.gpuVoltage,
		ga.m.gpuGFXVoltage,
		ga.m.gpuMemVoltage,
		ga.m.gpuPCIeSpeed,
		ga.m.gpuPCIeMaxSpeed,
		ga.m.gpuPCIeBandwidth,
		ga.m.gpuEnergyConsumed,
		ga.m.gpuPCIeReplayCount,
		ga.m.gpuPCIeRecoveryCount,
		ga.m.gpuPCIeReplayRolloverCount,
		ga.m.gpuPCIeNACKSentCount,
		ga.m.gpuPCIeNACKReceivedCount,
		ga.m.gpuClock,
		ga.m.gpuPowerUsage,
		ga.m.gpuTotalVram,
		ga.m.gpuEccCorrectTotal,
		ga.m.gpuEccUncorrectTotal,
		ga.m.gpuEccCorrectSDMA,
		ga.m.gpuEccUncorrectSDMA,
		ga.m.gpuEccCorrectGFX,
		ga.m.gpuEccUncorrectGFX,
		ga.m.gpuEccCorrectMMHUB,
		ga.m.gpuEccUncorrectMMHUB,
		ga.m.gpuEccCorrectATHUB,
		ga.m.gpuEccUncorrectATHUB,
		ga.m.gpuEccCorrectBIF,
		ga.m.gpuEccUncorrectBIF,
		ga.m.gpuEccCorrectHDP,
		ga.m.gpuEccUncorrectHDP,
		ga.m.gpuEccCorrectXgmiWAFL,
		ga.m.gpuEccUncorrectXgmiWAFL,
		ga.m.gpuEccCorrectDF,
		ga.m.gpuEccUncorrectDF,
		ga.m.gpuEccCorrectSMN,
		ga.m.gpuEccUncorrectSMN,
		ga.m.gpuEccCorrectSEM,
		ga.m.gpuEccUncorrectSEM,
		ga.m.gpuEccCorrectMP0,
		ga.m.gpuEccUncorrectMP0,
		ga.m.gpuEccCorrectMP1,
		ga.m.gpuEccUncorrectMP1,
		ga.m.gpuEccCorrectFUSE,
		ga.m.gpuEccUncorrectFUSE,
		ga.m.gpuEccCorrectUMC,
		ga.m.gpuEccUncorrectUMC,
		ga.m.xgmiNbrNopTx0,
		ga.m.xgmiNbrReqTx0,
		ga.m.xgmiNbrRespTx0,
		ga.m.xgmiNbrBeatsTx0,
		ga.m.xgmiNbrNopTx1,
		ga.m.xgmiNbrReqTx1,
		ga.m.xgmiNbrRespTx1,
		ga.m.xgmiNbrBeatsTx1,
		ga.m.xgmiNbrTxTput0,
		ga.m.xgmiNbrTxTput1,
		ga.m.xgmiNbrTxTput2,
		ga.m.xgmiNbrTxTput3,
		ga.m.xgmiNbrTxTput4,
		ga.m.xgmiNbrTxTput5,
		ga.m.gpuUsedVram,
		ga.m.gpuFreeVram,
		ga.m.gpuTotalVisibleVram,
		ga.m.gpuUsedVisibleVram,
		ga.m.gpuFreeVisibleVram,
		ga.m.gpuTotalGTT,
		ga.m.gpuUsedGTT,
		ga.m.gpuFreeGTT,
		ga.m.gpuEccCorrectMCA,
		ga.m.gpuEccUncorrectMCA,
		ga.m.gpuEccCorrectVCN,
		ga.m.gpuEccUncorrectVCN,
		ga.m.gpuEccCorrectJPEG,
		ga.m.gpuEccUncorrectJPEG,
		ga.m.gpuEccCorrectIH,
		ga.m.gpuEccUncorrectIH,
		ga.m.gpuEccCorrectMPIO,
		ga.m.gpuEccUncorrectMPIO,
	}

}

func (ga *GPUAgentClient) initPrometheusMetrics() {
	labels := ga.GetExportLabels()
	ga.m = &metrics{
		gpuNodesTotal: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "gpu_nodes_total",
				Help: "Number of GPUs in the node",
			},
		),
		gpuPackagePower: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_package_power",
			Help: "package power in Watts",
		},
			labels),
		gpuAvgPkgPower: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_average_package_power",
			Help: "Average package power in Watts",
		},
			labels),
		gpuEdgeTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_edge_temperature",
			Help: "Current edge temperature in celsius",
		},
			labels),
		gpuJunctionTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_junction_temperature",
			Help: "Current junction/hotspot temperature in celsius",
		},
			labels),
		gpuMemoryTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_memory_temperature",
			Help: "Current memory temperature in celsius",
		},
			labels),
		gpuHBMTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_hbm_temperature",
			Help: "Current HBM temperature in celsius",
		},
			append([]string{"hbm_index"}, labels...)),
		gpuGFXActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_gfx_activity",
		},
			labels),
		gpuUMCActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_umc_activity",
		},
			labels),
		gpuMMAActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_mma_activity",
			Help: "usage of MultiMedia (MM) engine as a percentage",
		},
			labels),
		gpuVCNActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_vcn_activity",
			Help: "usage of Video Core Next (VCN) activity as a percentage",
		},
			append([]string{"vcn_index"}, labels...)),
		gpuJPEGActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_jpeg_activity",
		},
			append([]string{"jpeg_index"}, labels...)),
		gpuVoltage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_voltage",
			Help: "Current voltage draw in mV",
		},
			labels),
		gpuGFXVoltage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_gfx_voltage",
			Help: "Current graphics voltage in mV",
		},
			labels),
		gpuMemVoltage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_memory_voltage",
			Help: "current memory voltage in mV",
		},
			labels),
		gpuPCIeSpeed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_speed",
			Help: "current PCIe speed in GT/s",
		},
			labels),
		gpuPCIeMaxSpeed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_max_speed",
			Help: "maximum PCIe speed in GT/s",
		},
			labels),
		gpuPCIeBandwidth: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_bandwidth",
			Help: "current PCIe bandwidth in Mb/s",
		},
			labels),
		gpuEnergyConsumed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_energy_consumed",
			Help: "accumulated energy consumed in uJ",
		},
			labels),
		gpuPCIeReplayCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_replay_count",
		},
			labels),
		gpuPCIeRecoveryCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_recovery_count",
		},
			labels),
		gpuPCIeReplayRolloverCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_replay_rollover_count",
		},
			labels),
		gpuPCIeNACKSentCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_nack_sent_count",
		},
			labels),
		gpuPCIeNACKReceivedCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_nack_received_count",
		},
			labels),
		gpuClock: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_clock",
			Help: "current GPU clock frequency in MHz",
		},
			append([]string{"clock_index", "clock_type"}, labels...)),
		gpuPowerUsage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_power_usage",
			Help: "power usage in Watts",
		},
			labels),
		gpuTotalVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_total_vram",
			Help: "total VRAM of the GPU (in MB)",
		},
			labels),
		gpuUsedVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_used_vram",
			Help: "used VRAM of the GPU (in MB)",
		},
			labels),
		gpuFreeVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_free_vram",
			Help: "free VRAM memory of the GPU (in MB)",
		},
			labels),
		gpuTotalVisibleVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_total_visible_vram",
			Help: "total visible VRAM of the GPU (in MB)",
		},
			labels),
		gpuUsedVisibleVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_used_visible_vram",
			Help: "used visible VRAM of the GPU (in MB)",
		},
			labels),
		gpuFreeVisibleVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_free_visible_vram",
			Help: "free visible VRAM of the GPU (in MB)",
		},
			labels),
		gpuTotalGTT: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_total_gtt",
			Help: "total graphics translation table of the GPU (in MB)",
		},
			labels),
		gpuUsedGTT: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_used_gtt",
			Help: "used graphics translation table of the GPU (in MB)",
		},
			labels),
		gpuFreeGTT: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_free_gtt",
			Help: "total graphics translation table of the GPU (in MB)",
		},
			labels),
		gpuEccCorrectTotal: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_total",
		},
			labels),
		gpuEccUncorrectTotal: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_total",
		},
			labels),
		gpuEccCorrectSDMA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_sdma",
		},
			labels),
		gpuEccUncorrectSDMA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_sdma",
		},
			labels),
		gpuEccCorrectGFX: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_gfx",
		},
			labels),
		gpuEccUncorrectGFX: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_gfx",
		},
			labels),
		gpuEccCorrectMMHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mmhub",
		},
			labels),
		gpuEccUncorrectMMHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mmhub",
		},
			labels),
		gpuEccCorrectATHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_athub",
		},
			labels),
		gpuEccUncorrectATHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_athub",
		},
			labels),
		gpuEccCorrectBIF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_bif",
		},
			labels),
		gpuEccUncorrectBIF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_bif",
		},
			labels),
		gpuEccCorrectHDP: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_hdp",
		},
			labels),
		gpuEccUncorrectHDP: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_hdp",
		},
			labels),
		gpuEccCorrectXgmiWAFL: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_xgmi_wafl",
		},
			labels),
		gpuEccUncorrectXgmiWAFL: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_xgmi_wafl",
		},
			labels),
		gpuEccCorrectDF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_df",
		},
			labels),
		gpuEccUncorrectDF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_df",
		},
			labels),
		gpuEccCorrectSMN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_smn",
		},
			labels),
		gpuEccUncorrectSMN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_smn",
		},
			labels),
		gpuEccCorrectSEM: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_sem",
		},
			labels),
		gpuEccUncorrectSEM: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_sem",
		},
			labels),
		gpuEccCorrectMP0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mp0",
		},
			labels),
		gpuEccUncorrectMP0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mp0",
		},
			labels),
		gpuEccCorrectMP1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mp1",
		},
			labels),
		gpuEccUncorrectMP1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mp1",
		},
			labels),
		gpuEccCorrectFUSE: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_fuse",
		},
			labels),
		gpuEccUncorrectFUSE: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_fuse",
		},
			labels),
		gpuEccCorrectUMC: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_umc",
		},
			labels),
		gpuEccUncorrectUMC: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_umc",
		},
			labels),
		xgmiNbrNopTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_nop_tx",
		},
			labels),
		xgmiNbrNopTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_nop_tx",
		},
			labels),
		xgmiNbrReqTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_request_tx",
		},
			labels),
		xgmiNbrReqTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_request_tx",
		},
			labels),
		xgmiNbrRespTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_response_tx",
		},
			labels),
		xgmiNbrRespTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_response_tx",
		},
			labels),
		xgmiNbrBeatsTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_beats_tx",
		},
			labels),
		xgmiNbrBeatsTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_beats_tx",
		},
			labels),
		xgmiNbrTxTput0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_tx_throughput",
		},
			labels),
		xgmiNbrTxTput1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_tx_throughput",
		},
			labels),
		xgmiNbrTxTput2: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_2_tx_throughput",
		},
			labels),
		xgmiNbrTxTput3: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_3_tx_throughput",
		},
			labels),
		xgmiNbrTxTput4: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_4_tx_throughput",
		},
			labels),
		xgmiNbrTxTput5: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_5_tx_throughput",
		},
			labels),
		gpuEccCorrectMCA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mca",
		},
			labels),
		gpuEccUncorrectMCA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mca",
		},
			labels),
		gpuEccCorrectVCN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_vcn",
		},
			labels),
		gpuEccUncorrectVCN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_vcn",
		},
			labels),
		gpuEccCorrectJPEG: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_jpeg",
		},
			labels),
		gpuEccUncorrectJPEG: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_jpeg",
		},
			labels),
		gpuEccCorrectIH: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_ih",
		},
			labels),
		gpuEccUncorrectIH: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_ih",
		},
			labels),
		gpuEccCorrectMPIO: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mpio",
		},
			labels),
		gpuEccUncorrectMPIO: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mpio",
		},
			labels),
	}
	ga.initFieldMetricsMap()

}

func (ga *GPUAgentClient) initFieldRegistration() error {
	for field, enabled := range exportFieldMap {
		if !enabled {
			continue
		}
		fieldIndex, ok := gpumetrics.GPUMetricField_value[field]
		if !ok {
			logger.Log.Printf("Invalid field %v, ignored", field)
			continue
		}
		ga.mh.GetRegistry().MustRegister(fieldMetricsMap[fieldIndex])
	}

	return nil
}

func (ga *GPUAgentClient) InitConfigs() error {
	filedConfigs := ga.mh.GetMetricsConfig()

	ga.initLabelConfigs(filedConfigs)
	initFieldConfig(filedConfigs)
	initGPUSelectorConfig(filedConfigs)
	ga.initPrometheusMetrics()
	return ga.initFieldRegistration()
}

func getGPUInstanceID(gpu *amdgpu.GPU) int {
	return int(gpu.Status.Index)
}

func (ga *GPUAgentClient) UpdateStaticMetrics() error {
	// send the req to gpuclient
	resp, err := ga.getMetrics()
	if err != nil {
		// crash to let service restart
		logger.Log.Fatalf("err :%v", err)
		return err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return fmt.Errorf("%v", resp.ApiStatus)
	}
	/* disable multiple request to gpuagent, getting wrong responses
	ga.Lock()
	ga.cacheGpuids = make(map[string][]byte)
	for _, gpu := range resp.Response {
		uuid, _ := uuid.FromBytes(gpu.Spec.Id)
		ga.cacheGpuids[uuid.String()] = gpu.Spec.Id
	}
	ga.Unlock()
	*/
	ga.m.gpuNodesTotal.Set(float64(len(resp.Response)))
	for _, gpu := range resp.Response {
		ga.updateGPUInfoToMetrics(gpu)
	}
	return nil
}

func (ga *GPUAgentClient) UpdateMetricsStats() error {
	return ga.getMetricsAll()
}

func (ga *GPUAgentClient) populateLabelsFromGPU(gpu *amdgpu.GPU) map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	var podInfo k8s.PodResourceInfo
	var jobInfo slurm.JobInfo

	if ga.isKubernetes {
		if ga.kubeClient.CheckExportLabels(exportLables) {
			if pods, err := ga.kubeClient.ListPods(ctx); err == nil {
				if gpu.Status.PCIeStatus != nil {
					podInfo = pods[strings.ToLower(gpu.Status.PCIeStatus.PCIeBusId)]
				}
			} else {
				logger.Log.Printf("failed to list pod resources, %v", err)
				// continue
			}
		}
	} else {
		if ga.slurmClient.CheckExportLabels(exportLables) {
			jobs := ga.slurmClient.ListJobs()
			jobInfo = jobs[fmt.Sprintf("%v", gpu.Status.Index)]
		}
	}

	labels := make(map[string]string)

	for ckey, enabled := range exportLables {
		if !enabled {
			continue
		}
		key := strings.ToLower(ckey)
		switch ckey {
		case gpumetrics.GPUMetricLabel_GPU_UUID.String():
			uuid, _ := uuid.FromBytes(gpu.Spec.Id)
			labels[key] = uuid.String()
		case gpumetrics.GPUMetricLabel_GPU_ID.String():
			labels[key] = fmt.Sprintf("%v", gpu.Status.Index)
		case gpumetrics.GPUMetricLabel_POD.String():
			labels[key] = podInfo.Pod
		case gpumetrics.GPUMetricLabel_NAMESPACE.String():
			labels[key] = podInfo.Namespace
		case gpumetrics.GPUMetricLabel_CONTAINER.String():
			labels[key] = podInfo.Container
		case gpumetrics.GPUMetricLabel_JOB_ID.String():
			labels[key] = jobInfo.Id
		case gpumetrics.GPUMetricLabel_JOB_USER.String():
			labels[key] = jobInfo.User
		case gpumetrics.GPUMetricLabel_JOB_PARTITION.String():
			labels[key] = jobInfo.Partition
		case gpumetrics.GPUMetricLabel_CLUSTER_NAME.String():
			labels[key] = jobInfo.Cluster
		case gpumetrics.GPUMetricLabel_SERIAL_NUMBER.String():
			labels[key] = gpu.Status.SerialNum
		case gpumetrics.GPUMetricLabel_CARD_SERIES.String():
			labels[key] = gpu.Status.CardSeries
		case gpumetrics.GPUMetricLabel_CARD_MODEL.String():
			labels[key] = gpu.Status.CardModel
		case gpumetrics.GPUMetricLabel_CARD_VENDOR.String():
			labels[key] = gpu.Status.CardVendor
		case gpumetrics.GPUMetricLabel_DRIVER_VERSION.String():
			labels[key] = gpu.Status.DriverVersion
		case gpumetrics.GPUMetricLabel_VBIOS_VERSION.String():
			labels[key] = gpu.Status.VBIOSVersion
		case gpumetrics.GPUMetricLabel_HOSTNAME.String():
			labels[key] = ga.staticHostLabels[gpumetrics.GPUMetricLabel_HOSTNAME.String()]
		default:
			logger.Log.Printf("Invalid label is ignored %v", key)
		}
	}
	return labels
}

func (ga *GPUAgentClient) exporterEnabledGPU(instance int) bool {
	if gpuSelectorMap == nil {
		return true
	}
	_, enabled := gpuSelectorMap[instance]
	return enabled

}

func normalizeUint64(x interface{}) float64 {
	if v, ok := x.(uint64); ok {
		if v == math.MaxUint64 || v == math.MaxUint32 || v == math.MaxUint16 {
			return 0
		} else {
			return float64(v)
		}
	}
	if v, ok := x.(uint32); ok {
		// special case
		if v == math.MaxUint16 || v == math.MaxUint32 {
			return 0
		} else {
			return float64(v)
		}
	}
	logger.Log.Fatalf("only uint64 and uint32 are expected but got %v", reflect.TypeOf(x))
	return 0
}

func (ga *GPUAgentClient) updateGPUInfoToMetrics(gpu *amdgpu.GPU) {
	if !ga.exporterEnabledGPU(getGPUInstanceID(gpu)) {
		return
	}

	labels := ga.populateLabelsFromGPU(gpu)
	labelsWithIndex := ga.populateLabelsFromGPU(gpu)
	status := gpu.Status
	stats := gpu.Stats
	ga.m.gpuPackagePower.With(labels).Set(normalizeUint64(stats.PackagePower))
	ga.m.gpuAvgPkgPower.With(labels).Set(normalizeUint64(stats.AvgPackagePower))

	// gpu temp stats
	tempStats := stats.Temperature
	if tempStats != nil {
		ga.m.gpuEdgeTemp.With(labels).Set(float64(tempStats.EdgeTemperature))
		ga.m.gpuJunctionTemp.With(labels).Set(float64(tempStats.JunctionTemperature))
		ga.m.gpuMemoryTemp.With(labels).Set(float64(tempStats.MemoryTemperature))
		for j, temp := range tempStats.HBMTemperature {
			labelsWithIndex["hbm_index"] = fmt.Sprintf("%v", j)
			ga.m.gpuHBMTemp.With(labelsWithIndex).Set(float64(temp))
		}
		delete(labelsWithIndex, "hbm_index")
	}

	// gpu usage
	gpuUsage := stats.Usage
	if gpuUsage != nil {
		ga.m.gpuGFXActivity.With(labels).Set(float64(gpuUsage.GFXActivity))
		ga.m.gpuUMCActivity.With(labels).Set(float64(gpuUsage.UMCActivity))
		ga.m.gpuMMAActivity.With(labels).Set(float64(gpuUsage.MMActivity))
		for j, act := range gpuUsage.VCNActivity {
			labelsWithIndex["vcn_index"] = fmt.Sprintf("%v", j)
			ga.m.gpuVCNActivity.With(labelsWithIndex).Set(normalizeUint64(act))
		}
		delete(labelsWithIndex, "vcn_index")
		for j, act := range gpuUsage.JPEGActivity {
			labelsWithIndex["jpeg_index"] = fmt.Sprintf("%v", j)
			ga.m.gpuJPEGActivity.With(labelsWithIndex).Set(normalizeUint64(act))
		}
		delete(labelsWithIndex, "jpeg_index")
	}

	volt := stats.Voltage
	if volt != nil {
		ga.m.gpuVoltage.With(labels).Set(normalizeUint64(volt.Voltage))
		ga.m.gpuGFXVoltage.With(labels).Set(normalizeUint64(volt.GFXVoltage))
		ga.m.gpuMemVoltage.With(labels).Set(normalizeUint64(volt.MemoryVoltage))
	}

	// pcie status
	pcieStatus := status.PCIeStatus
	if pcieStatus != nil {
		ga.m.gpuPCIeSpeed.With(labels).Set(normalizeUint64(pcieStatus.Speed))
		ga.m.gpuPCIeMaxSpeed.With(labels).Set(normalizeUint64(pcieStatus.MaxSpeed))
		ga.m.gpuPCIeBandwidth.With(labels).Set(normalizeUint64(pcieStatus.Bandwidth))
	}

	// pcie stats
	pcieStats := stats.PCIeStats
	if pcieStats != nil {
		ga.m.gpuPCIeReplayCount.With(labels).Set(normalizeUint64(pcieStats.ReplayCount))
		ga.m.gpuPCIeRecoveryCount.With(labels).Set(normalizeUint64(pcieStats.RecoveryCount))
		ga.m.gpuPCIeReplayRolloverCount.With(labels).Set(normalizeUint64(pcieStats.ReplayRolloverCount))
		ga.m.gpuPCIeNACKSentCount.With(labels).Set(normalizeUint64(pcieStats.NACKSentCount))
		ga.m.gpuPCIeNACKReceivedCount.With(labels).Set(normalizeUint64(pcieStats.NACKReceivedCount))
	}

	ga.m.gpuEnergyConsumed.With(labels).Set(stats.EnergyConsumed)

	// clock status
	clockStatus := status.ClockStatus
	if clockStatus != nil {
		for j, clock := range clockStatus {
			labelsWithIndex["clock_index"] = fmt.Sprintf("%v", j)
			labelsWithIndex["clock_type"] = fmt.Sprintf("%v", clock.Type.String())
			ga.m.gpuClock.With(labelsWithIndex).Set(normalizeUint64(clock.Frequency))
		}
		delete(labelsWithIndex, "clock_index")
		delete(labelsWithIndex, "clock_type")
	}

	ga.m.gpuPowerUsage.With(labels).Set(float64(stats.PowerUsage))

	ga.m.gpuEccCorrectTotal.With(labels).Set(normalizeUint64(stats.TotalCorrectableErrors))
	ga.m.gpuEccUncorrectTotal.With(labels).Set(normalizeUint64(stats.TotalUncorrectableErrors))
	ga.m.gpuEccCorrectSDMA.With(labels).Set(normalizeUint64(stats.SDMACorrectableErrors))
	ga.m.gpuEccUncorrectSDMA.With(labels).Set(normalizeUint64(stats.SDMAUncorrectableErrors))
	ga.m.gpuEccCorrectGFX.With(labels).Set(normalizeUint64(stats.GFXCorrectableErrors))
	ga.m.gpuEccUncorrectGFX.With(labels).Set(normalizeUint64(stats.GFXUncorrectableErrors))
	ga.m.gpuEccCorrectMMHUB.With(labels).Set(normalizeUint64(stats.MMHUBCorrectableErrors))
	ga.m.gpuEccUncorrectMMHUB.With(labels).Set(normalizeUint64(stats.MMHUBUncorrectableErrors))
	ga.m.gpuEccCorrectATHUB.With(labels).Set(normalizeUint64(stats.ATHUBCorrectableErrors))
	ga.m.gpuEccUncorrectATHUB.With(labels).Set(normalizeUint64(stats.ATHUBUncorrectableErrors))

	ga.m.gpuEccCorrectBIF.With(labels).Set(normalizeUint64(stats.BIFCorrectableErrors))
	ga.m.gpuEccUncorrectBIF.With(labels).Set(normalizeUint64(stats.BIFUncorrectableErrors))
	ga.m.gpuEccCorrectHDP.With(labels).Set(normalizeUint64(stats.HDPCorrectableErrors))
	ga.m.gpuEccUncorrectHDP.With(labels).Set(normalizeUint64(stats.HDPUncorrectableErrors))
	ga.m.gpuEccCorrectXgmiWAFL.With(labels).Set(normalizeUint64(stats.XGMIWAFLCorrectableErrors))
	ga.m.gpuEccUncorrectXgmiWAFL.With(labels).Set(normalizeUint64(stats.XGMIWAFLUncorrectableErrors))
	ga.m.gpuEccCorrectDF.With(labels).Set(normalizeUint64(stats.DFCorrectableErrors))
	ga.m.gpuEccUncorrectDF.With(labels).Set(normalizeUint64(stats.DFUncorrectableErrors))
	ga.m.gpuEccCorrectSMN.With(labels).Set(normalizeUint64(stats.SMNCorrectableErrors))
	ga.m.gpuEccUncorrectSMN.With(labels).Set(normalizeUint64(stats.SMNUncorrectableErrors))
	ga.m.gpuEccCorrectSEM.With(labels).Set(normalizeUint64(stats.SEMCorrectableErrors))
	ga.m.gpuEccUncorrectSEM.With(labels).Set(normalizeUint64(stats.SEMUncorrectableErrors))

	ga.m.gpuEccCorrectMP0.With(labels).Set(normalizeUint64(stats.MP0CorrectableErrors))
	ga.m.gpuEccUncorrectMP0.With(labels).Set(normalizeUint64(stats.MP0UncorrectableErrors))
	ga.m.gpuEccCorrectMP1.With(labels).Set(normalizeUint64(stats.MP1CorrectableErrors))
	ga.m.gpuEccUncorrectMP1.With(labels).Set(normalizeUint64(stats.MP1UncorrectableErrors))
	ga.m.gpuEccCorrectFUSE.With(labels).Set(normalizeUint64(stats.FUSECorrectableErrors))
	ga.m.gpuEccUncorrectFUSE.With(labels).Set(normalizeUint64(stats.FUSEUncorrectableErrors))
	ga.m.gpuEccCorrectUMC.With(labels).Set(normalizeUint64(stats.UMCCorrectableErrors))
	ga.m.gpuEccUncorrectUMC.With(labels).Set(normalizeUint64(stats.UMCUncorrectableErrors))

	ga.m.gpuEccCorrectMCA.With(labels).Set(normalizeUint64(stats.MCACorrectableErrors))
	ga.m.gpuEccUncorrectMCA.With(labels).Set(normalizeUint64(stats.MCAUncorrectableErrors))

	ga.m.gpuEccCorrectVCN.With(labels).Set(normalizeUint64(stats.VCNCorrectableErrors))
	ga.m.gpuEccUncorrectVCN.With(labels).Set(normalizeUint64(stats.VCNUncorrectableErrors))

	ga.m.gpuEccCorrectJPEG.With(labels).Set(normalizeUint64(stats.JPEGCorrectableErrors))
	ga.m.gpuEccUncorrectJPEG.With(labels).Set(normalizeUint64(stats.JPEGUncorrectableErrors))

	ga.m.gpuEccCorrectIH.With(labels).Set(normalizeUint64(stats.IHCorrectableErrors))
	ga.m.gpuEccUncorrectIH.With(labels).Set(normalizeUint64(stats.IHUncorrectableErrors))

	ga.m.gpuEccCorrectMPIO.With(labels).Set(normalizeUint64(stats.MPIOCorrectableErrors))
	ga.m.gpuEccUncorrectMPIO.With(labels).Set(normalizeUint64(stats.MPIOUncorrectableErrors))

	ga.m.xgmiNbrNopTx0.With(labels).Set(normalizeUint64(stats.XGMINeighbor0TxNOPs))
	ga.m.xgmiNbrReqTx0.With(labels).Set(normalizeUint64(stats.XGMINeighbor0TxRequests))
	ga.m.xgmiNbrRespTx0.With(labels).Set(normalizeUint64(stats.XGMINeighbor0TxResponses))
	ga.m.xgmiNbrBeatsTx0.With(labels).Set(normalizeUint64(stats.XGMINeighbor0TXBeats))

	ga.m.xgmiNbrNopTx1.With(labels).Set(normalizeUint64(stats.XGMINeighbor1TxNOPs))
	ga.m.xgmiNbrReqTx1.With(labels).Set(normalizeUint64(stats.XGMINeighbor1TxRequests))
	ga.m.xgmiNbrRespTx1.With(labels).Set(normalizeUint64(stats.XGMINeighbor1TxResponses))
	ga.m.xgmiNbrBeatsTx1.With(labels).Set(normalizeUint64(stats.XGMINeighbor1TXBeats))

	ga.m.xgmiNbrTxTput0.With(labels).Set(normalizeUint64(stats.XGMINeighbor0TxThroughput))
	ga.m.xgmiNbrTxTput1.With(labels).Set(normalizeUint64(stats.XGMINeighbor1TxThroughput))
	ga.m.xgmiNbrTxTput2.With(labels).Set(normalizeUint64(stats.XGMINeighbor2TxThroughput))
	ga.m.xgmiNbrTxTput3.With(labels).Set(normalizeUint64(stats.XGMINeighbor3TxThroughput))
	ga.m.xgmiNbrTxTput4.With(labels).Set(normalizeUint64(stats.XGMINeighbor4TxThroughput))
	ga.m.xgmiNbrTxTput5.With(labels).Set(normalizeUint64(stats.XGMINeighbor5TxThroughput))

	vramUsage := stats.VRAMUsage
	if vramUsage != nil {
		ga.m.gpuTotalVram.With(labels).Set(normalizeUint64(vramUsage.TotalVRAM))
		ga.m.gpuUsedVram.With(labels).Set(normalizeUint64(vramUsage.UsedVRAM))
		ga.m.gpuFreeVram.With(labels).Set(normalizeUint64(vramUsage.FreeVRAM))

		ga.m.gpuTotalVisibleVram.With(labels).Set(normalizeUint64(vramUsage.TotalVisibleVRAM))
		ga.m.gpuUsedVisibleVram.With(labels).Set(normalizeUint64(vramUsage.UsedVisibleVRAM))
		ga.m.gpuFreeVisibleVram.With(labels).Set(normalizeUint64(vramUsage.FreeVisibleVRAM))

		ga.m.gpuTotalGTT.With(labels).Set(normalizeUint64(vramUsage.TotalGTT))
		ga.m.gpuUsedGTT.With(labels).Set(normalizeUint64(vramUsage.UsedGTT))
		ga.m.gpuFreeGTT.With(labels).Set(normalizeUint64(vramUsage.FreeGTT))
	}
}

func (ga *GPUAgentClient) populateStaticHostLabels() error {
	ga.staticHostLabels = map[string]string{}
	hostname, err := ga.getHostName()
	if err != nil {
		return err
	}
	logger.Log.Printf("hostame %v", hostname)
	ga.staticHostLabels[gpumetrics.GPUMetricLabel_HOSTNAME.String()] = hostname
	return nil
}

func (ga *GPUAgentClient) getHostName() (string, error) {
	hostname := ""
	var err error
	if nodeName := os.Getenv("NODE_NAME"); nodeName != "" {
		hostname = nodeName
	} else {
		hostname, err = os.Hostname()
		if err != nil {
			return "", err
		}
	}
	return hostname, nil
}
