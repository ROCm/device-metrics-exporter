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
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"github.com/ROCm/device-metrics-exporter/pkg/types"
)

// ifoeStoreFieldSpec mirrors one IFOE GaugeVec for the store path: store field
// key (== gauge metric name), enum field name (for enable gating), help text,
// whether it carries per-port labels (vs node-level non-IFOE labels), and
// whether it is a node-level always-on total. Single source for
// buildStoreFieldDescs; values/labels are written in updateMetrics.
type ifoeStoreFieldSpec struct {
	field     string
	enum      string
	help      string
	perPort   bool
	nodeLevel bool
}

var ifoeStoreFieldSpecs = []ifoeStoreFieldSpec{
	{"ifoe_total_devices", exportermetrics.IFOEMetricField_IFOE_TOTAL_DEVICES.String(), "Total number of IFOE devices", false, true},
	{"ifoe_total_stations", exportermetrics.IFOEMetricField_IFOE_TOTAL_STATIONS.String(), "Total number of IFOE stations", false, true},
	{"ifoe_total_ports", exportermetrics.IFOEMetricField_IFOE_TOTAL_PORTS.String(), "Total number of IFOE network ports", false, true},
	{"ifoe_num_failedover_streams", exportermetrics.IFOEMetricField_IFOE_NUMBER_FAILEDOVER_STREAMS.String(), "Number of failed over IFOE streams", true, false},
	{"ifoe_num_paused_streams", exportermetrics.IFOEMetricField_IFOE_NUMBER_PAUSED_STREAMS.String(), "Number of paused IFOE streams", true, false},
	{"ifoe_bit_error_rate", exportermetrics.IFOEMetricField_IFOE_BIT_ERROR_RATE.String(), "Bit Error Rate (BER) reported by the network port expressed as errors per 10^12 bits", true, false},
	{"ifoe_fec_codeword_symbol_errors0", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS0.String(), "Total number of FEC codewords with 0 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors1", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS1.String(), "Total number of FEC codewords with 1 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors2", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS2.String(), "Total number of FEC codewords with 2 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors3", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS3.String(), "Total number of FEC codewords with 3 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors4", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS4.String(), "Total number of FEC codewords with 4 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors5", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS5.String(), "Total number of FEC codewords with 5 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors6", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS6.String(), "Total number of FEC codewords with 6 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors7", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS7.String(), "Total number of FEC codewords with 7 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors8", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS8.String(), "Total number of FEC codewords with 8 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors9", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS9.String(), "Total number of FEC codewords with 9 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors10", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS10.String(), "Total number of FEC codewords with 10 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors11", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS11.String(), "Total number of FEC codewords with 11 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors12", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS12.String(), "Total number of FEC codewords with 12 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors13", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS13.String(), "Total number of FEC codewords with 13 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors14", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS14.String(), "Total number of FEC codewords with 14 symbol errors", true, false},
	{"ifoe_fec_codeword_symbol_errors15", exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS15.String(), "Total number of FEC codewords with 15 symbol errors", true, false},
}

// nodeStoreKey is the reserved, stable store key for the node-level IFOE totals
// slot. Derived from the sorted node label-name set (never a device UUID), it is
// identical at seed time and on every scrape so the same slot is updated in place.
func (ga *GPUAgentIFOEClient) nodeStoreKey() string {
	names := append([]string(nil), ga.GetExporterNonIFOELabels()...)
	sort.Strings(names)
	return "ifoe-node:" + strings.Join(names, ",")
}

// seedNodeTotals publishes the three node-level IFOE totals at 0 before any
// fetch. They are always-on health signals, so they must be present on scrape #1
// and persist (store COW) across zero/failed fetches.
func (ga *GPUAgentIFOEClient) seedNodeTotals() {
	store := ga.gpuHandler.mh.GetStore()
	if store == nil {
		return
	}
	labels := ga.populateLabelsFromObject(nil, nil, nil)
	store.Update(ga.nodeStoreKey(), map[string]snapshotstore.Sample{
		"ifoe_total_ports":    {Value: 0, Labels: labels},
		"ifoe_total_devices":  {Value: 0, Labels: labels},
		"ifoe_total_stations": {Value: 0, Labels: labels},
	})
}

// buildStoreFieldDescs returns the IFOE StoreCollector descriptors mirroring the
// GaugeVec definitions: prefixed name, help, gauge type, and the matching label
// set (per-port vs node-level). Config-disabled per-device fields are omitted,
// matching initFieldRegistration; node-level totals are always included.
func (ga *GPUAgentIFOEClient) buildStoreFieldDescs() []snapshotstore.FieldDesc {
	prefix := ga.gpuHandler.mh.GetPrefix()
	perPortLabels := append([]string{"station_uuid", "port_name", "device_uuid"}, ga.GetExportLabels()...)
	nonIfoeLabels := ga.GetExporterNonIFOELabels()
	descs := make([]snapshotstore.FieldDesc, 0, len(ifoeStoreFieldSpecs))
	for _, s := range ifoeStoreFieldSpecs {
		if !s.nodeLevel {
			if enabled, ok := ga.exportFieldMap[s.enum]; ok && !enabled {
				continue
			}
		}
		labelNames := nonIfoeLabels
		if s.perPort {
			labelNames = perPortLabels
		}
		descs = append(descs, snapshotstore.FieldDesc{
			Field:      s.field,
			Name:       prefix + s.field,
			Help:       s.help,
			ValueType:  prometheus.GaugeValue,
			LabelNames: labelNames,
		})
	}
	return descs
}

// ifoeStoreCollector serves IFOE /metrics from the snapshot store in place of the
// GaugeVecs, mirroring gpuStoreCollector. updateMetrics feeds the store (port
// samples via Update, node totals via UpdateIfChanged); descriptors come from
// buildStoreFieldDescs. Unchecked collector (Describe is a no-op).
type ifoeStoreCollector struct {
	ga *GPUAgentIFOEClient
}

func (c *ifoeStoreCollector) Describe(ch chan<- *prometheus.Desc) {}

func (c *ifoeStoreCollector) Collect(ch chan<- prometheus.Metric) {
	snapshotstore.NewStoreCollector(c.ga.gpuHandler.mh.GetStore(), c.ga.buildStoreFieldDescs()).Collect(ch)
}

// local variables
var (
	ifoeMandatoryLables = []string{
		exportermetrics.MetricLabel_HOSTNAME.String(),
		exportermetrics.GPUMetricLabel_GPU_UUID.String(),
	}
)

// model for IFOE metrics
// Device->Station->NetworkPort
type IFOEMetrics struct {
	// IFOE network port stats
	totalNetworkPorts         prometheus.GaugeVec
	numFailedoverStreams      prometheus.GaugeVec
	numPausedStreams          prometheus.GaugeVec
	bitErrorRate              prometheus.GaugeVec
	fecCodeWordSymbolErrors0  prometheus.GaugeVec
	fecCodeWordSymbolErrors1  prometheus.GaugeVec
	fecCodeWordSymbolErrors2  prometheus.GaugeVec
	fecCodeWordSymbolErrors3  prometheus.GaugeVec
	fecCodeWordSymbolErrors4  prometheus.GaugeVec
	fecCodeWordSymbolErrors5  prometheus.GaugeVec
	fecCodeWordSymbolErrors6  prometheus.GaugeVec
	fecCodeWordSymbolErrors7  prometheus.GaugeVec
	fecCodeWordSymbolErrors8  prometheus.GaugeVec
	fecCodeWordSymbolErrors9  prometheus.GaugeVec
	fecCodeWordSymbolErrors10 prometheus.GaugeVec
	fecCodeWordSymbolErrors11 prometheus.GaugeVec
	fecCodeWordSymbolErrors12 prometheus.GaugeVec
	fecCodeWordSymbolErrors13 prometheus.GaugeVec
	fecCodeWordSymbolErrors14 prometheus.GaugeVec
	fecCodeWordSymbolErrors15 prometheus.GaugeVec

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
	// list of supported labels
	ga.exportLabels = make(map[string]bool)

	for _, name := range exportermetrics.MetricLabel_name {
		ga.exportLabels[name] = false
	}
	for _, name := range exportermetrics.GPUMetricLabel_name {
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
		bitErrorRate: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_bit_error_rate",
				Help: "Bit Error Rate (BER) reported by the network port expressed as errors per 10^12 bits",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors0: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors0",
				Help: "Total number of FEC codewords with 0 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors1: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors1",
				Help: "Total number of FEC codewords with 1 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors2: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors2",
				Help: "Total number of FEC codewords with 2 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors3: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors3",
				Help: "Total number of FEC codewords with 3 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors4: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors4",
				Help: "Total number of FEC codewords with 4 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors5: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors5",
				Help: "Total number of FEC codewords with 5 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors6: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors6",
				Help: "Total number of FEC codewords with 6 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors7: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors7",
				Help: "Total number of FEC codewords with 7 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors8: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors8",
				Help: "Total number of FEC codewords with 8 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors9: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors9",
				Help: "Total number of FEC codewords with 9 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors10: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors10",
				Help: "Total number of FEC codewords with 10 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors11: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors11",
				Help: "Total number of FEC codewords with 11 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors12: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors12",
				Help: "Total number of FEC codewords with 12 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors13: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors13",
				Help: "Total number of FEC codewords with 13 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors14: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors14",
				Help: "Total number of FEC codewords with 14 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
		fecCodeWordSymbolErrors15: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ifoe_fec_codeword_symbol_errors15",
				Help: "Total number of FEC codewords with 15 symbol errors",
			},
			append([]string{"station_uuid", "port_name", "device_uuid"}, labels...)),
	}
	ga.initFieldMetricsMap()
}

func (ga *GPUAgentIFOEClient) initFieldMetricsMap() {
	// nolint
	ga.fieldMetricsMap = map[string]FieldMeta{
		exportermetrics.IFOEMetricField_IFOE_TOTAL_DEVICES.String():                FieldMeta{Metric: ga.metrics.totalDevices},
		exportermetrics.IFOEMetricField_IFOE_TOTAL_STATIONS.String():               FieldMeta{Metric: ga.metrics.totalStations},
		exportermetrics.IFOEMetricField_IFOE_TOTAL_PORTS.String():                  FieldMeta{Metric: ga.metrics.totalNetworkPorts},
		exportermetrics.IFOEMetricField_IFOE_NUMBER_FAILEDOVER_STREAMS.String():    FieldMeta{Metric: ga.metrics.numFailedoverStreams},
		exportermetrics.IFOEMetricField_IFOE_NUMBER_PAUSED_STREAMS.String():        FieldMeta{Metric: ga.metrics.numPausedStreams},
		exportermetrics.IFOEMetricField_IFOE_BIT_ERROR_RATE.String():               FieldMeta{Metric: ga.metrics.bitErrorRate},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS0.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors0},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS1.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors1},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS2.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors2},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS3.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors3},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS4.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors4},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS5.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors5},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS6.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors6},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS7.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors7},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS8.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors8},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS9.String():  FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors9},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS10.String(): FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors10},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS11.String(): FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors11},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS12.String(): FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors12},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS13.String(): FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors13},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS14.String(): FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors14},
		exportermetrics.IFOEMetricField_IFOE_FEC_CODEWORD_SYMBOL_ERRORS15.String(): FieldMeta{Metric: ga.metrics.fecCodeWordSymbolErrors15},
	}
}

func (ga *GPUAgentIFOEClient) initFieldRegistration() error {
	// Serve IFOE /metrics from the snapshot store instead of the GaugeVecs:
	// updateMetrics feeds the store per scrape (singleflight) and the GaugeVecs are
	// left unregistered so output comes solely from the store collector. Registered
	// raw (not via RegisterMetric) because the collector's descriptors already carry
	// the configured metric prefix. Mirrors the GPU path.
	return ga.gpuHandler.mh.GetRegistry().Register(&ifoeStoreCollector{ga: ga})
}

func (ga *GPUAgentIFOEClient) InitConfigs() error {
	logger.Log.Printf("Initializing GPU Agent IFOE Client configs")
	filedConfigs := ga.gpuHandler.mh.GetIFOEMetricsConfig()

	ga.initCustomLabels(filedConfigs)
	ga.initLabelConfigs(filedConfigs)
	ga.initFieldConfig(filedConfigs)
	ga.InitPodExtraLabels(filedConfigs)
	ga.initPrometheusMetrics()
	err := ga.initFieldRegistration()
	// Seed the always-on node-level totals so they are present on scrape #1,
	// before the first fetch.
	ga.seedNodeTotals()
	return err
}

func (ga *GPUAgentIFOEClient) InitPodExtraLabels(config *exportermetrics.IFOEMetricConfig) {
	// initialize pod labels maps
	ga.k8PodInfoMap = make(map[string]types.K8sPodInfo)
	if config != nil {
		ga.extraPodLabelsMap = utils.NormalizeExtraPodLabels(config.GetExtraPodLabels())
		if len(ga.extraPodLabelsMap) > 0 {
			ga.podInfoEnabled = true
		}
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
		case exportermetrics.GPUMetricLabel_GPU_UUID.String():
			if ualPort != nil {
				ifoeUUID := utils.UUIDToString(ualPort.Spec.Id)
				labels[key] = ifoeUUID
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
		podLabels := utils.GetPodLabels(&podInfo, ga.k8PodInfoMap)
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

func (ga *GPUAgentIFOEClient) UpdateStaticMetrics(ctx context.Context) error {
	return ga.updateMetrics(ctx)
}

func (ga *GPUAgentIFOEClient) UpdateMetricsStats(ctx context.Context) error {
	// Approach 1: dedupe concurrent fetch-on-scrape into one gpuagent fetch.
	return ga.deduper.FetchOnce("ifoe", func() error {
		if err := ga.updateMetrics(ctx); err != nil {
			// Fetch failed: drop stale IFOE telemetry so /metrics serves only the
			// always-present ifoe_total_* signals until the next successful fetch.
			ga.dropStaleIFOETelemetry()
			return err
		}
		return nil
	})
}

// dropStaleIFOETelemetry evicts the IFOE telemetry fields from the store on a
// fetch failure, keeping only the always-present node-level ifoe_total_* signals
// (nodeLevel=true in ifoeStoreFieldSpecs). Scoped to IFOE field names, so GPU/NIC
// slots are untouched.
func (ga *GPUAgentIFOEClient) dropStaleIFOETelemetry() {
	store := ga.gpuHandler.mh.GetStore()
	if store == nil {
		return
	}
	drop := make(map[string]struct{}, len(ifoeStoreFieldSpecs))
	for _, s := range ifoeStoreFieldSpecs {
		if s.nodeLevel {
			continue
		}
		drop[s.field] = struct{}{}
	}
	store.DropFields(drop)
}

// NeedsPerScrapePull reports whether this IFOE client serves /metrics via the
// reset+refill pull path. False: IFOE is store-served (updateMetrics feeds the
// store via singleflight on scrape and the store collector reads lock-free),
// mirroring GPU.
func (ga *GPUAgentIFOEClient) NeedsPerScrapePull() bool {
	return false
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

// FetchPodInfoForNode fetches pod labels for all pods running on this node
func (ga *GPUAgentIFOEClient) FetchPodInfoForNode() (map[string]types.K8sPodInfo, error) {
	if !ga.gpuHandler.enabledK8sApi {
		return nil, nil
	}
	k8sSchedClient := ga.gpuHandler.GetK8sApiClient()
	if k8sSchedClient == nil {
		return nil, fmt.Errorf("k8s scheduler client is nil")
	}
	listMap := make(map[string]types.K8sPodInfo)
	if ga.gpuHandler.enabledK8sApi && ga.podInfoEnabled {
		return k8sSchedClient.GetAllPods()
	}
	return listMap, nil
}
