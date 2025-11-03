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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

// local variables
var (
	ifoeMandatoryLables = []string{
		exportermetrics.MetricLabel_HOSTNAME.String(),
		exportermetrics.IFOEMetricLabel_IFOE_UUID.String(),
	}
)

// model for IFOE metrics
// Device->Station->NetworkPort
type IFOEMetrics struct {
	// IFOE network port stats
	totalNetworkPorts    prometheus.GaugeVec
	numFailedoverStreams prometheus.GaugeVec
	numPausedStreams     prometheus.GaugeVec

	// IFOE device stats
	totalDevices prometheus.GaugeVec

	// IFOE station stats
	totalStations prometheus.GaugeVec
}

func GetIFOEMandatoryLabels() []string {
	return ifoeMandatoryLables
}

// initCustomLabels initializes custom label configuration for the IFOE client.
func (ga *GPUAgentIFOEClient) initCustomLabels(config *exportermetrics.IFOEMetricConfig) {
	ga.customLabelMap = make(map[string]string)
	disallowedLabels := []string{}
	if config != nil && config.GetCustomLabels() != nil {
		for _, name := range exportermetrics.GPUMetricLabel_name {
			found := false
			for _, cname := range ga.allowedCustomLabels {
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
			ga.customLabelMap[label] = value
			labelCount++
		}
	}
	logger.Log.Printf("custom labels being exported: %v", ga.customLabelMap)
}

func (ga *GPUAgentIFOEClient) initLabelConfigs(config *exportermetrics.IFOEMetricConfig) {
	// list of mandatory labels
	ga.exportLabels = make(map[string]bool)

	// common labels
	for _, name := range exportermetrics.MetricLabel_name {
		ga.exportLabels[name] = false
	}

	for _, name := range exportermetrics.IFOEMetricLabel_name {
		ga.exportLabels[name] = false
	}
	// only mandatory labels are set for default
	for _, name := range ifoeMandatoryLables {
		ga.exportLabels[name] = true
	}

	if config != nil {
		for _, name := range config.GetLabels() {
			name = strings.ToUpper(name)
			if _, ok := ga.exportLabels[name]; ok {
				logger.Log.Printf("label %v enabled", name)
				ga.exportLabels[name] = true
			}
		}
	}
	logger.Log.Printf("export-labels updated to %v", ga.exportLabels)
}

func (ga *GPUAgentIFOEClient) initFieldConfig(config *exportermetrics.IFOEMetricConfig) {
	ga.exportFieldMap = make(map[string]bool)
	// setup metric fields in map to be monitored
	// init the map with all supported strings from enum
	enable_default := true
	if config != nil && len(config.GetFields()) != 0 {
		enable_default = false
	}
	for _, name := range exportermetrics.IFOEMetricField_name {
		ga.exportFieldMap[name] = enable_default
	}
	if config == nil || len(config.GetFields()) == 0 {
		return
	}
	for _, fieldName := range config.GetFields() {
		fieldName = strings.ToUpper(fieldName)
		if _, ok := ga.exportFieldMap[fieldName]; ok {
			ga.exportFieldMap[fieldName] = true
		}
	}
	// print disabled short list
	for k, v := range ga.exportFieldMap {
		if !v {
			logger.Log.Printf("%v field is disabled", k)
		}
	}
}

func (ga *GPUAgentIFOEClient) initPrometheusMetrics() {
	labels := ga.GetExportLabels()
	nonIfoeLabels := ga.GetExporterNonIFOELabels()
	logger.Log.Printf("initPrometheusMetrics with labels: %v", labels)
	logger.Log.Printf("initPrometheusMetrics with non-IFOE labels: %v", nonIfoeLabels)
	ga.metrics = &IFOEMetrics{
		totalDevices: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_total_devices",
				Help: "Total number of IFOE devices",
			},
			nonIfoeLabels,
		),
		totalStations: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_total_stations",
				Help: "Total number of IFOE stations",
			},
			nonIfoeLabels,
		),
		totalNetworkPorts: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_total_ports",
				Help: "Total number of IFOE network ports",
			},
			nonIfoeLabels,
		),
		numFailedoverStreams: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_num_failedover_streams",
				Help: "Number of failed over IFOE streams",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		numPausedStreams: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_num_paused_streams",
				Help: "Number of paused IFOE streams",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
	}
	ga.initFieldMetricsMap()
}

func (ga *GPUAgentIFOEClient) initFieldMetricsMap() {
	// nolint
	ga.fieldMetricsMap = map[string]FieldMeta{
		exportermetrics.IFOEMetricField_IFOE_TOTAL_DEVICES.String():             FieldMeta{Metric: ga.metrics.totalDevices},
		exportermetrics.IFOEMetricField_IFOE_TOTAL_STATIONS.String():            FieldMeta{Metric: ga.metrics.totalStations},
		exportermetrics.IFOEMetricField_IFOE_TOTAL_PORTS.String():               FieldMeta{Metric: ga.metrics.totalNetworkPorts},
		exportermetrics.IFOEMetricField_IFOE_NUMBER_FAILEDOVER_STREAMS.String(): FieldMeta{Metric: ga.metrics.numFailedoverStreams},
		exportermetrics.IFOEMetricField_IFOE_NUMBER_PAUSED_STREAMS.String():     FieldMeta{Metric: ga.metrics.numPausedStreams},
	}
}

func (ga *GPUAgentIFOEClient) initFieldRegistration() error {
	for field, enabled := range ga.exportFieldMap {
		if !enabled {
			continue
		}
		prommetric, ok := ga.fieldMetricsMap[field]
		if !ok {
			logger.Log.Printf("invalid field found ignore %v", field)
			continue
		}
		if err := ga.gpuHandler.mh.RegisterMetric(prommetric.Metric); err != nil {
			logger.Log.Printf("Field %v registration failed with err : %v", field, err)
		}
	}

	return nil
}

func (ga *GPUAgentIFOEClient) InitConfigs() error {
	logger.Log.Printf("Initializing GPU Agent IFOE Client configs")
	filedConfigs := ga.gpuHandler.mh.GetIFOEMetricsConfig()

	ga.initCustomLabels(filedConfigs)
	ga.initLabelConfigs(filedConfigs)
	ga.initFieldConfig(filedConfigs)
	ga.InitPodExtraLabels(filedConfigs)
	ga.initPrometheusMetrics()
	return ga.initFieldRegistration()
}

func (ga *GPUAgentIFOEClient) InitPodExtraLabels(config *exportermetrics.IFOEMetricConfig) {
	// initialize pod labels maps
	ga.k8PodLabelsMap = make(map[string]map[string]string)
	if config != nil {
		ga.extraPodLabelsMap = utils.NormalizeExtraPodLabels(config.GetExtraPodLabels())
	}
	logger.Log.Printf("export-labels updated to %v", ga.extraPodLabelsMap)
}

func (ga *GPUAgentIFOEClient) PopulateStaticHostLabels() error {
	ga.staticHostLabels = map[string]string{}
	hostname, err := utils.GetHostName()
	if err != nil {
		return err
	}
	logger.Log.Printf("hostame %v", hostname)
	ga.staticHostLabels[exportermetrics.MetricLabel_HOSTNAME.String()] = hostname
	return nil
}

func (ga *GPUAgentIFOEClient) populateLabelsFromObject(
	wls map[string]scheduler.Workload,
	ualStationMap map[string]*amdgpu.UALStation,
	ualPort *amdgpu.UALNetworkPort) map[string]string {

	var podInfo scheduler.PodResourceInfo

	labels := make(map[string]string)

	for ckey, enabled := range ga.exportLabels {
		if !enabled {
			continue
		}
		key := strings.ToLower(ckey)
		switch ckey {
		case exportermetrics.IFOEMetricLabel_IFOE_UUID.String():
			if ualPort != nil {
				// ifoeUUID := utils.UUIDToString(ualPort.Spec.Id)
				labels[key] = "ifoe_uuid_placeholder"
			}
		case exportermetrics.MetricLabel_CARD_SERIES.String():
			if ualPort != nil {
				labels[key] = "card_series_placeholder"
			}
		case exportermetrics.MetricLabel_CARD_MODEL.String():
			if ualPort != nil {
				labels[key] = "card_model_placeholder"
			}
		case exportermetrics.MetricLabel_CARD_VENDOR.String():
			if ualPort != nil {
				labels[key] = "AMD"
			}
		case exportermetrics.MetricLabel_DRIVER_VERSION.String():
			if ualPort != nil {
				labels[key] = "driver_version_placeholder"
			}
		case exportermetrics.MetricLabel_VBIOS_VERSION.String():
			if ualPort != nil {
				labels[key] = "vbios_version_placeholder"
			}
		case exportermetrics.MetricLabel_POD.String():
			if ualPort != nil {
				labels[key] = "pod_placeholder"
			}
		case exportermetrics.MetricLabel_NAMESPACE.String():
			if ualPort != nil {
				labels[key] = "namespace_placeholder"
			}
		case exportermetrics.MetricLabel_CONTAINER.String():
			if ualPort != nil {
				labels[key] = "container_placeholder"
			}
		case exportermetrics.MetricLabel_JOB_ID.String():
			if ualPort != nil {
				labels[key] = "job_id_placeholder"
			}
		case exportermetrics.MetricLabel_JOB_USER.String():
			if ualPort != nil {
				labels[key] = "job_user_placeholder"
			}
		case exportermetrics.MetricLabel_JOB_PARTITION.String():
			if ualPort != nil {
				labels[key] = "job_partition_placeholder"
			}
		case exportermetrics.MetricLabel_CLUSTER_NAME.String():
			if ualPort != nil {
				labels[key] = "cluster_name_placeholder"
			}
		case exportermetrics.MetricLabel_SERIAL_NUMBER.String():
			if ualPort != nil {
				labels[key] = "serial_number_placeholder"
			}
		case exportermetrics.MetricLabel_HOSTNAME.String():
			labels[key] = ga.staticHostLabels[exportermetrics.MetricLabel_HOSTNAME.String()]
		default:
			logger.Log.Printf("Invalid label is ignored %v", key)
		}
	}

	// Add extra pod labels only if config has mapped any
	if ualPort != nil && len(ga.extraPodLabelsMap) > 0 {
		podLabels := utils.GetPodLabels(&podInfo, ga.k8PodLabelsMap)
		for prometheusPodlabel, k8Podlabel := range ga.extraPodLabelsMap {
			label := strings.ToLower(prometheusPodlabel)
			labels[label] = podLabels[k8Podlabel]
		}
	}

	// Add custom labels
	for label, value := range ga.customLabelMap {
		labels[label] = value
	}
	return labels
}

func (ga *GPUAgentIFOEClient) ResetMetrics() error {
	// reset all label based fields
	for _, prommetric := range ga.fieldMetricsMap {
		prommetric.Metric.Reset()
	}
	return nil
}

func (ga *GPUAgentIFOEClient) UpdateStaticMetrics() error {
	if utils.IsSimEnabled() {
		return ga.updateMockMetrics()
	}
	return ga.updateMetrics()
}

func (ga *GPUAgentIFOEClient) UpdateMetricsStats() error {
	if utils.IsSimEnabled() {
		return ga.updateMockMetrics()
	}
	return ga.updateMetrics()
}

func (ga *GPUAgentIFOEClient) QueryMetrics() (interface{}, error) {
	// No op for now
	return nil, nil
}

func (ga *GPUAgentIFOEClient) QueryInbandRASErrors(severity string) (interface{}, error) {
	// No op for now
	return nil, nil
}

// SetComputeNodeHealthState sets the compute node health state
func (ga *GPUAgentIFOEClient) SetComputeNodeHealthState(state bool) {
	ga.Lock()

	if ga.computeNodeHealthState == state {
		return
	}
	logger.Log.Printf("updating compute node health from: %v, to: %v", ga.computeNodeHealthState, state)
	ga.computeNodeHealthState = state
	ga.Unlock()

	// for now no metrics to update or health states to send
}

// FetchPodLabelsForNode fetches pod labels for all pods running on this node
func (ga *GPUAgentIFOEClient) FetchPodLabelsForNode() (map[string]map[string]string, error) {
	if !ga.gpuHandler.enabledK8sApi {
		return nil, nil
	}
	k8sSchedClient := ga.gpuHandler.GetK8sApiClient()
	if k8sSchedClient == nil {
		return nil, fmt.Errorf("k8s scheduler client is nil")
	}
	listMap := make(map[string]map[string]string)
	if ga.gpuHandler.enabledK8sApi && len(ga.extraPodLabelsMap) > 0 {
		return k8sSchedClient.GetAllPods()
	}
	return listMap, nil
}
