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
	"math"
	"os"
	"reflect"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/parserutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"github.com/gofrs/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// local variables
var (
	mandatoryLables = []string{
		exportermetrics.GPUMetricLabel_GPU_ID.String(),
		exportermetrics.GPUMetricLabel_SERIAL_NUMBER.String(),
		exportermetrics.GPUMetricLabel_CARD_MODEL.String(),
		exportermetrics.GPUMetricLabel_HOSTNAME.String(),
		exportermetrics.GPUMetricLabel_GPU_PARTITION_ID.String(),
		exportermetrics.GPUMetricLabel_GPU_COMPUTE_PARTITION_TYPE.String(),
	}
	// List of suppported labels that can be customized
	allowedCustomLabels = []string{
		exportermetrics.GPUMetricLabel_CLUSTER_NAME.String(),
	}
	exportLables    map[string]bool
	exportFieldMap  map[string]bool
	fieldMetricsMap []prometheus.Collector
	gpuSelectorMap  map[int]bool
	customLabelMap  map[string]string
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

	gpuHealth prometheus.GaugeVec

	gpuXgmiLinkStatsRx prometheus.GaugeVec
	gpuXgmiLinkStatsTx prometheus.GaugeVec
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
	ga.m.gpuHealth.Reset()
	ga.m.gpuXgmiLinkStatsRx.Reset()
	ga.m.gpuXgmiLinkStatsTx.Reset()
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

	for key := range customLabelMap {
		exists := false
		for _, label := range labelList {
			if key == label {
				exists = true
				break
			}
		}

		// Add only unique labels to export labels
		if !exists {
			labelList = append(labelList, key)
		}
	}

	return labelList
}

func (ga *GPUAgentClient) initLabelConfigs(config *exportermetrics.GPUMetricConfig) {

	// list of mandatory labels
	exportLables = make(map[string]bool)
	for _, name := range exportermetrics.GPUMetricLabel_name {
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
				logger.Log.Printf("label %v enabled", name)
				exportLables[name] = true
			}
		}
	}
	logger.Log.Printf("export-labels updated to %v", exportLables)
}

func initCustomLabels(config *exportermetrics.GPUMetricConfig) {
	customLabelMap = make(map[string]string)
	disallowedLabels := []string{}
	if config != nil && config.GetCustomLabels() != nil {
		for _, name := range exportermetrics.GPUMetricLabel_name {
			found := false
			for _, cname := range allowedCustomLabels {
				if name == cname {
					found = true
					break
				}
			}
			if !found {
				disallowedLabels = append(disallowedLabels, strings.ToLower(name))
			}
		}
		cl := config.GetCustomLabels()
		labelCount := 0

		for l, value := range cl {
			if labelCount >= globals.MaxSupportedCustomLabels {
				logger.Log.Printf("Max custom labels supported: %v, ignoring extra labels.", globals.MaxSupportedCustomLabels)
				break
			}
			label := strings.ToLower(l)

			// Check if custom label is a mandatory label, ignore if true
			found := false
			for _, dlabel := range disallowedLabels {
				if dlabel == label {
					logger.Log.Printf("Label %s cannot be customized, ignoring...", dlabel)
					found = true
					break
				}
			}
			if found {
				continue
			}

			// Store all custom labels
			customLabelMap[label] = value
			labelCount++
		}
	}
	logger.Log.Printf("custom labels being exported: %v", customLabelMap)
}

func initGPUSelectorConfig(config *exportermetrics.GPUMetricConfig) {
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

func initFieldConfig(config *exportermetrics.GPUMetricConfig) {
	exportFieldMap = make(map[string]bool)
	// setup metric fields in map to be monitored
	// init the map with all supported strings from enum
	enable_default := true
	if config != nil && len(config.GetFields()) != 0 {
		enable_default = false
	}
	for _, name := range exportermetrics.GPUMetricField_name {
		exportFieldMap[name] = enable_default
	}
	if config == nil || len(config.GetFields()) == 0 {
		return
	}
	for _, fieldName := range config.GetFields() {
		fieldName = strings.ToUpper(fieldName)
		if _, ok := exportFieldMap[fieldName]; ok {
			exportFieldMap[fieldName] = true
		}
	}
	// print disabled short list
	for k, v := range exportFieldMap {
		if !v {
			logger.Log.Printf("%v field is disabled", k)
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
		ga.m.gpuHealth,
		ga.m.gpuXgmiLinkStatsRx,
		ga.m.gpuXgmiLinkStatsTx,
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
			Help: "Current socker power in Watts",
		},
			labels),
		gpuAvgPkgPower: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_average_package_power",
			Help: "Average socket power in Watts",
		},
			labels),
		gpuEdgeTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_edge_temperature",
			Help: "Current edge temperature in Celsius",
		},
			labels),
		gpuJunctionTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_junction_temperature",
			Help: "Current junction/hotspot temperature in Celsius",
		},
			labels),
		gpuMemoryTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_memory_temperature",
			Help: "Current memory temperature in Celsius",
		},
			labels),
		gpuHBMTemp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_hbm_temperature",
			Help: "List of current HBM temperatures in Celsius",
		},
			append([]string{"hbm_index"}, labels...)),
		gpuGFXActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_gfx_activity",
			Help: "Graphics engine usage in Percentage (0-100)",
		},
			labels),
		gpuUMCActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_umc_activity",
			Help: "Memory engine usage in Percentage (0-100)",
		},
			labels),
		gpuMMAActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_mma_activity",
			Help: "Average MultiMedia (MM) engine usage in Percentage (0-100)",
		},
			labels),
		gpuVCNActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_vcn_activity",
			Help: "List of Video Core Next (VCN) encoe/decode usage in percentage",
		},
			append([]string{"vcn_index"}, labels...)),
		gpuJPEGActivity: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_jpeg_activity",
			Help: "List of JPEG engine usage in Percentage (0-100)",
		},
			append([]string{"jpeg_index"}, labels...)),
		gpuVoltage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_voltage",
			Help: "Current SoC voltage in mV",
		},
			labels),
		gpuGFXVoltage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_gfx_voltage",
			Help: "Current gfx voltage in mV",
		},
			labels),
		gpuMemVoltage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_memory_voltage",
			Help: "Current memory voltage in mV",
		},
			labels),
		gpuPCIeSpeed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_speed",
			Help: "Current PCIe speed in GT/s",
		},
			labels),
		gpuPCIeMaxSpeed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_max_speed",
			Help: "Maximum PCIe speed in GT/s",
		},
			labels),
		gpuPCIeBandwidth: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_bandwidth",
			Help: "Current PCIe bandwidth in Mb/s",
		},
			labels),
		gpuEnergyConsumed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_energy_consumed",
			Help: "Accumulated energy consumed by the GPU in uJ",
		},
			labels),
		gpuPCIeReplayCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_replay_count",
			Help: "Total number of PCIe replays",
		},
			labels),
		gpuPCIeRecoveryCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_recovery_count",
			Help: "Total number of PCIe recoveries",
		},
			labels),
		gpuPCIeReplayRolloverCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_replay_rollover_count",
			Help: "PCIe replay accumulated count",
		},
			labels),
		gpuPCIeNACKSentCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_nack_sent_count",
			Help: "PCIe NAK sent accumulated count",
		},
			labels),
		gpuPCIeNACKReceivedCount: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pcie_nack_received_count",
			Help: "PCIe NAK received accumulated count",
		},
			labels),
		gpuClock: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_clock",
			Help: "List of current GPU clock frequencies in MHz",
		},
			append([]string{"clock_index", "clock_type"}, labels...)),
		gpuPowerUsage: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_power_usage",
			Help: "GPU Power usage in Watts",
		},
			labels),
		gpuTotalVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_total_vram",
			Help: "Total VRAM memory of the GPU (in MB)",
		},
			labels),
		gpuUsedVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_used_vram",
			Help: "Used VRAM memory of the GPU (in MB)",
		},
			labels),
		gpuFreeVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_free_vram",
			Help: "Free VRAM memory of the GPU (in MB)",
		},
			labels),
		gpuTotalVisibleVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_total_visible_vram",
			Help: "Total visible VRAM memory of the GPU (in MB)",
		},
			labels),
		gpuUsedVisibleVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_used_visible_vram",
			Help: "Used visible VRAM memory of the GPU (in MB)",
		},
			labels),
		gpuFreeVisibleVram: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_free_visible_vram",
			Help: "Free visible VRAM memory of the GPU (in MB)",
		},
			labels),
		gpuTotalGTT: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_total_gtt",
			Help: "Total graphics translation table memory of the GPU (in MB)",
		},
			labels),
		gpuUsedGTT: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_used_gtt",
			Help: "Used graphics translation table memory of the GPU (in MB)",
		},
			labels),
		gpuFreeGTT: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_free_gtt",
			Help: "Free graphics translation table memory of the GPU (in MB)",
		},
			labels),
		gpuEccCorrectTotal: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_total",
			Help: "Total Correctable error count",
		},
			labels),
		gpuEccUncorrectTotal: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_total",
			Help: "Total Uncorrectable error count",
		},
			labels),
		gpuEccCorrectSDMA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_sdma",
			Help: "Correctable error count in SDMA block",
		},
			labels),
		gpuEccUncorrectSDMA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_sdma",
			Help: "Uncorrectable error count in SDMA block",
		},
			labels),
		gpuEccCorrectGFX: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_gfx",
			Help: "Correctable error count in GFX block",
		},
			labels),
		gpuEccUncorrectGFX: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_gfx",
			Help: "Uncorrectable error count in GFX block",
		},
			labels),
		gpuEccCorrectMMHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mmhub",
			Help: "Correctable error count in MMHUB block",
		},
			labels),
		gpuEccUncorrectMMHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mmhub",
			Help: "Uncorrectable error count in MMHUB block",
		},
			labels),
		gpuEccCorrectATHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_athub",
			Help: "Correctable error count in ATHUB block",
		},
			labels),
		gpuEccUncorrectATHUB: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_athub",
			Help: "Uncorrectable error count in ATHUB block",
		},
			labels),
		gpuEccCorrectBIF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_bif",
			Help: "Correctable error count in BIF block",
		},
			labels),
		gpuEccUncorrectBIF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_bif",
			Help: "Uncorrectable error count in BIF block",
		},
			labels),
		gpuEccCorrectHDP: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_hdp",
			Help: "Correctable error count in HDP block",
		},
			labels),
		gpuEccUncorrectHDP: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_hdp",
			Help: "Uncorrectable error count in HDP block",
		},
			labels),
		gpuEccCorrectXgmiWAFL: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_xgmi_wafl",
			Help: "Correctable error count in WAFL block",
		},
			labels),
		gpuEccUncorrectXgmiWAFL: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_xgmi_wafl",
			Help: "Uncorrectable error count in WAFL block",
		},
			labels),
		gpuEccCorrectDF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_df",
			Help: "Correctable error count in DF block",
		},
			labels),
		gpuEccUncorrectDF: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_df",
			Help: "Uncorrectable error count in DF block",
		},
			labels),
		gpuEccCorrectSMN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_smn",
			Help: "Correctable error count in SMN block",
		},
			labels),
		gpuEccUncorrectSMN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_smn",
			Help: "Uncorrectable error count in SMN block",
		},
			labels),
		gpuEccCorrectSEM: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_sem",
			Help: "Correctable error count in SEM block",
		},
			labels),
		gpuEccUncorrectSEM: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_sem",
			Help: "Uncorrectable error count in SEM block",
		},
			labels),
		gpuEccCorrectMP0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mp0",
			Help: "Correctable error count in MP0 block",
		},
			labels),
		gpuEccUncorrectMP0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mp0",
			Help: "Uncorrectable error count in MP0 block",
		},
			labels),
		gpuEccCorrectMP1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mp1",
			Help: "Correctable error count in MP1 block",
		},
			labels),
		gpuEccUncorrectMP1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mp1",
			Help: "Uncorrectable error count in MP1 block",
		},
			labels),
		gpuEccCorrectFUSE: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_fuse",
			Help: "Correctable error count in Fuse block",
		},
			labels),
		gpuEccUncorrectFUSE: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_fuse",
			Help: "Uncorrectable error count in Fuse block",
		},
			labels),
		gpuEccCorrectUMC: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_umc",
			Help: "Correctable error count in UMC block",
		},
			labels),
		gpuEccUncorrectUMC: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_umc",
			Help: "Uncorrectable error count in UMC block",
		},
			labels),
		xgmiNbrNopTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_nop_tx",
			Help: "NOPs sent to neighbor 0",
		},
			labels),
		xgmiNbrNopTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_nop_tx",
			Help: "NOPs sent to neighbor 1",
		},
			labels),
		xgmiNbrReqTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_request_tx",
			Help: "Outgoing requests to neighbor 0",
		},
			labels),
		xgmiNbrReqTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_request_tx",
			Help: "Outgoing requests to neighbor 1",
		},
			labels),
		xgmiNbrRespTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_response_tx",
			Help: "Outgoing responses to neighbor 0",
		},
			labels),
		xgmiNbrRespTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_response_tx",
			Help: "Outgoing responses to neighbor 1",
		},
			labels),
		xgmiNbrBeatsTx0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_beats_tx",
			Help: "Data beats sent to neighbor 0; Each beat represents 32 bytes",
		},
			labels),
		xgmiNbrBeatsTx1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_beats_tx",
			Help: "Data beats sent to neighbor 1; Each beat represents 32 bytes",
		},
			labels),
		xgmiNbrTxTput0: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_0_tx_throughput",
			Help: "Represents the number of outbound beats (each representing 32 bytes) on link 0; Throughput = BEATS/time_running * 10^9  bytes/sec",
		},
			labels),
		xgmiNbrTxTput1: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_1_tx_throughput",
			Help: "Represents the number of outbound beats (each representing 32 bytes) on link 1; Throughput = BEATS/time_running * 10^9  bytes/sec",
		},
			labels),
		xgmiNbrTxTput2: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_2_tx_throughput",
			Help: "Represents the number of outbound beats (each representing 32 bytes) on link 2; Throughput = BEATS/time_running * 10^9  bytes/sec",
		},
			labels),
		xgmiNbrTxTput3: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_3_tx_throughput",
			Help: "Represents the number of outbound beats (each representing 32 bytes) on link 3; Throughput = BEATS/time_running * 10^9  bytes/sec",
		},
			labels),
		xgmiNbrTxTput4: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_4_tx_throughput",
			Help: "Represents the number of outbound beats (each representing 32 bytes) on link 4; Throughput = BEATS/time_running * 10^9  bytes/sec",
		},
			labels),
		xgmiNbrTxTput5: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "xgmi_neighbor_5_tx_throughput",
			Help: "Represents the number of outbound beats (each representing 32 bytes) on link 5; Throughput = BEATS/time_running * 10^9  bytes/sec",
		},
			labels),
		gpuEccCorrectMCA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mca",
			Help: "Correctable error count in MCA block",
		},
			labels),
		gpuEccUncorrectMCA: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mca",
			Help: "Uncorrectable error count in MCA block",
		},
			labels),
		gpuEccCorrectVCN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_vcn",
			Help: "Correctable error count in VCN block",
		},
			labels),
		gpuEccUncorrectVCN: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_vcn",
			Help: "Uncorrectable error count in VCN block",
		},
			labels),
		gpuEccCorrectJPEG: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_jpeg",
			Help: "Correctable error count in JPEG block",
		},
			labels),
		gpuEccUncorrectJPEG: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_jpeg",
			Help: "Uncorrectable error count in JPEG block",
		},
			labels),
		gpuEccCorrectIH: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_ih",
			Help: "Correctable error count in IH block",
		},
			labels),
		gpuEccUncorrectIH: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_ih",
			Help: "Uncorrectable error count in IH block",
		},
			labels),
		gpuEccCorrectMPIO: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_correct_mpio",
			Help: "Correctable error count in MPIO block",
		},
			labels),
		gpuEccUncorrectMPIO: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_ecc_uncorrect_mpio",
			Help: "Uncorrectable error count in MPIO block",
		},
			labels),
		gpuHealth: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_health",
			Help: "Health of the GPU (0 = Unhealthy | 1 = Healthy)",
		},
			labels),
		gpuXgmiLinkStatsRx: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_xgmi_link_rx",
			Help: "XGMI Link Data Read in KB",
		},
			append([]string{"link_index"}, labels...)),
		gpuXgmiLinkStatsTx: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_xgmi_link_tx",
			Help: "XGMI Link Data Write in KB",
		},
			append([]string{"link_index"}, labels...)),
	}
	ga.initFieldMetricsMap()

}

func (ga *GPUAgentClient) initFieldRegistration() error {
	for field, enabled := range exportFieldMap {
		if !enabled {
			continue
		}
		fieldIndex, ok := exportermetrics.GPUMetricField_value[field]
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

	initCustomLabels(filedConfigs)
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
	wls := make(map[string]scheduler.Workload)
	resp, err := ga.getGPUs()
	if err != nil {
		return err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return fmt.Errorf("%v", resp.ApiStatus)
	}
	wls, _ = ga.ListWorkloads()
	ga.m.gpuNodesTotal.Set(float64(len(resp.Response)))
	// do this only once as the health monitoring thread will
	// update periodically. this is required only for first state
	// of the metrics pull response from prometheus
	newGPUState := ga.processEccErrorMetrics(resp.Response, wls)
	_ = ga.updateNewHealthState(newGPUState)
	for _, gpu := range resp.Response {
		ga.updateGPUInfoToMetrics(wls, gpu)
	}
	return nil
}

func (ga *GPUAgentClient) UpdateMetricsStats() error {
	return ga.getMetricsAll()
}

func (ga *GPUAgentClient) getWorkloadInfo(wls map[string]scheduler.Workload, gpu *amdgpu.GPU, filter bool) *scheduler.Workload {
	if filter && !ga.checkExportLabels(exportLables) {
		// return empty if labels are not set to be exportered
		return nil
	}
	// populate with workload info
	if gpu.Status.PCIeStatus != nil {
		if workload, ok := wls[strings.ToLower(gpu.Status.PCIeStatus.PCIeBusId)]; ok {
			return &workload
		}
	}
	// ignore errors as we always expect slurm deployment as default
	if workload, ok := wls[fmt.Sprintf("%v", getGPUInstanceID(gpu))]; ok {
		return &workload
	}
	return nil
}

func (ga *GPUAgentClient) populateLabelsFromGPU(wls map[string]scheduler.Workload, gpu *amdgpu.GPU) map[string]string {
	var podInfo scheduler.PodResourceInfo
	var jobInfo scheduler.JobInfo

	if wl := ga.getWorkloadInfo(wls, gpu, true); wl != nil {
		if wl.Type == scheduler.Kubernetes {
			podInfo = wl.Info.(scheduler.PodResourceInfo)
		} else {
			jobInfo = wl.Info.(scheduler.JobInfo)
		}
	}

	labels := make(map[string]string)

	for ckey, enabled := range exportLables {
		if !enabled {
			continue
		}
		key := strings.ToLower(ckey)
		switch ckey {
		case exportermetrics.GPUMetricLabel_GPU_UUID.String():
			uuid, _ := uuid.FromBytes(gpu.Spec.Id)
			labels[key] = uuid.String()
		case exportermetrics.GPUMetricLabel_GPU_ID.String():
			labels[key] = fmt.Sprintf("%v", getGPUInstanceID(gpu))
		case exportermetrics.GPUMetricLabel_POD.String():
			labels[key] = podInfo.Pod
		case exportermetrics.GPUMetricLabel_NAMESPACE.String():
			labels[key] = podInfo.Namespace
		case exportermetrics.GPUMetricLabel_CONTAINER.String():
			labels[key] = podInfo.Container
		case exportermetrics.GPUMetricLabel_JOB_ID.String():
			labels[key] = jobInfo.Id
		case exportermetrics.GPUMetricLabel_JOB_USER.String():
			labels[key] = jobInfo.User
		case exportermetrics.GPUMetricLabel_JOB_PARTITION.String():
			labels[key] = jobInfo.Partition
		case exportermetrics.GPUMetricLabel_CLUSTER_NAME.String():
			labels[key] = jobInfo.Cluster
		case exportermetrics.GPUMetricLabel_SERIAL_NUMBER.String():
			labels[key] = gpu.Status.SerialNum
		case exportermetrics.GPUMetricLabel_CARD_SERIES.String():
			labels[key] = gpu.Status.CardSeries
		case exportermetrics.GPUMetricLabel_CARD_MODEL.String():
			labels[key] = gpu.Status.CardModel
		case exportermetrics.GPUMetricLabel_CARD_VENDOR.String():
			labels[key] = gpu.Status.CardVendor
		case exportermetrics.GPUMetricLabel_DRIVER_VERSION.String():
			labels[key] = gpu.Status.DriverVersion
		case exportermetrics.GPUMetricLabel_VBIOS_VERSION.String():
			labels[key] = gpu.Status.VBIOSVersion
		case exportermetrics.GPUMetricLabel_HOSTNAME.String():
			labels[key] = ga.staticHostLabels[exportermetrics.GPUMetricLabel_HOSTNAME.String()]
		case exportermetrics.GPUMetricLabel_GPU_PARTITION_ID.String():
			labels[key] = fmt.Sprintf("%v", gpu.Status.PartitionId)
		case exportermetrics.GPUMetricLabel_GPU_COMPUTE_PARTITION_TYPE.String():
			partitionType := gpu.Spec.ComputePartitionType
			trimmedValue := strings.TrimPrefix(partitionType.String(), "GPU_COMPUTE_PARTITION_TYPE_")
			labels[key] = strings.ToLower(trimmedValue)
		default:
			logger.Log.Printf("Invalid label is ignored %v", key)
		}
	}

	// Add custom labels
	for label, value := range customLabelMap {
		labels[label] = value
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

func (ga *GPUAgentClient) updateGPUInfoToMetrics(wls map[string]scheduler.Workload, gpu *amdgpu.GPU) {
	if !ga.exporterEnabledGPU(getGPUInstanceID(gpu)) {
		return
	}

	labels := ga.populateLabelsFromGPU(wls, gpu)
	labelsWithIndex := ga.populateLabelsFromGPU(wls, gpu)
	status := gpu.Status
	stats := gpu.Stats
	ga.m.gpuPackagePower.With(labels).Set(normalizeUint64(stats.PackagePower))
	ga.m.gpuAvgPkgPower.With(labels).Set(normalizeUint64(stats.AvgPackagePower))

	// export health state only if available
	gpuid := fmt.Sprintf("%v", getGPUInstanceID(gpu))
	if hstate, ok := ga.healthState[gpuid]; ok {
		if hstate.Health == strings.ToLower(metricssvc.GPUHealth_HEALTHY.String()) {
			ga.m.gpuHealth.With(labels).Set(1)
		} else {
			ga.m.gpuHealth.With(labels).Set(0)
		}
	}

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
		ga.m.gpuMMAActivity.With(labels).Set(normalizeUint64(gpuUsage.MMActivity))
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
	xgmiStats := stats.XGMILinkStats
	if xgmiStats != nil {
		for j, linkStat := range xgmiStats {
			labelsWithIndex["link_index"] = fmt.Sprintf("%v", j)
			ga.m.gpuXgmiLinkStatsRx.With(labelsWithIndex).Set(normalizeUint64(linkStat.DataRead))
			ga.m.gpuXgmiLinkStatsTx.With(labelsWithIndex).Set(normalizeUint64(linkStat.DataWrite))
		}
		delete(labelsWithIndex, "link_index")
	}
}

func (ga *GPUAgentClient) populateStaticHostLabels() error {
	ga.staticHostLabels = map[string]string{}
	hostname, err := ga.getHostName()
	if err != nil {
		return err
	}
	logger.Log.Printf("hostame %v", hostname)
	ga.staticHostLabels[exportermetrics.GPUMetricLabel_HOSTNAME.String()] = hostname
	return nil
}

func (ga *GPUAgentClient) getHostName() (string, error) {
	hostname := ""
	var err error
	if nodeName := utils.GetNodeName(); nodeName != "" {
		hostname = nodeName
	} else {
		hostname, err = os.Hostname()
		if err != nil {
			return "", err
		}
	}
	return hostname, nil
}

func GetGPUAgentMandatoryLabels() []string {
	return mandatoryLables
}
