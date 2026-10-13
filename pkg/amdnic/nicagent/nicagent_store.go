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

// NIC single-source-store path (plan task T7, Approach 1). Additive: this
// mirrors the client's existing NIC GaugeVecs into the shared snapshot store and
// exposes descriptors for the generic StoreCollector, without changing the live
// GaugeVec population or wiring the StoreCollector into the live registry. The
// GaugeVecs are the source of truth for metric name/help/label derivation, so
// the store path cannot diverge from the pull path.
package nicagent

import (
	"context"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	nicFetchKey = "nicctl-fetch"
	// nicNodesTotalHelp mirrors the nicNodesTotal GaugeVec help string.
	nicNodesTotalHelp = "Number of NICs in the node"
	// nicNodeKeyPrefix reserves the NIC node-total store slot so a NIC telemetry
	// row with the node label set cannot collide with it.
	nicNodeKeyPrefix = "nic-node:"
)

// nicNodesTotalField is the exposed name of the node-level NIC-count metric.
var nicNodesTotalField = strings.ToLower(exportermetrics.NICMetricField_NIC_TOTAL.String())

// storeDeviceKey is a stable per-row store key built from a label set. Rows with
// identical labels (the same port/lif/qp) group into one device snapshot; rows
// that differ in any label get distinct keys. The key is internal and never
// exported — only the sample labels are.
func storeDeviceKey(labels prometheus.Labels) string {
	return snapshotstore.DeviceKey(labels, nil)
}

// gatherNICFamilies returns the current exposition of this client's enabled NIC
// GaugeVecs. The enable gate mirrors initFieldRegistration so the set matches the
// live /metrics surface; only populated families appear.
func (na *NICAgentClient) gatherNICFamilies() ([]*dto.MetricFamily, error) {
	reg := prometheus.NewRegistry()
	// RLock so a config reload rebuilding these maps (initFieldConfig /
	// initFieldMetricsMap take the write lock) cannot race this iteration.
	nicFieldMapMu.RLock()
	for field, enabled := range exportFieldMap {
		if !enabled && !strings.HasPrefix(field, "QP_") {
			continue
		}
		fm, ok := fieldMetricsMap[field]
		if !ok {
			continue
		}
		// Aliases share a collector; ignore the resulting duplicate-register error.
		_ = reg.Register(fm.Metric)
	}
	nicFieldMapMu.RUnlock()
	return reg.Gather()
}

// nicStoreFieldDescs builds StoreCollector descriptors mirroring the enabled NIC
// GaugeVecs: metric name, help, gauge type, and the gauge's variable label names.
// The GaugeVec path registers via RegisterMetric (prefix-wrapped); the store
// collector registers raw, so the prefix is applied here to keep names identical.
func (na *NICAgentClient) nicStoreFieldDescs() ([]snapshotstore.FieldDesc, error) {
	prefix := na.mh.GetPrefix()
	fams, err := na.gatherNICFamilies()
	if err != nil {
		return nil, err
	}
	descs := make([]snapshotstore.FieldDesc, 0, len(fams))
	for _, mf := range fams {
		if len(mf.GetMetric()) == 0 || mf.GetName() == nicNodesTotalField {
			// nic_total is handled explicitly below so it is always present.
			continue
		}
		// All metrics in a family share one label-name set; read it off any row.
		labelPairs := mf.GetMetric()[0].GetLabel()
		labelNames := make([]string, 0, len(labelPairs))
		for _, lp := range labelPairs {
			labelNames = append(labelNames, lp.GetName())
		}
		descs = append(descs, snapshotstore.FieldDesc{
			Field:      mf.GetName(),
			Name:       prefix + mf.GetName(),
			Help:       mf.GetHelp(),
			ValueType:  prometheus.GaugeValue,
			LabelNames: labelNames,
		})
	}
	// Node-level NIC count: descriptor is always emitted so the StoreCollector
	// renders it even at zero NICs / before the first fetch.
	descs = append(descs, snapshotstore.FieldDesc{
		Field:      nicNodesTotalField,
		Name:       prefix + nicNodesTotalField,
		Help:       nicNodesTotalHelp,
		ValueType:  prometheus.GaugeValue,
		LabelNames: na.GetExporterNonNICLabels(),
	})
	return descs, nil
}

// nicStoreCollector serves NIC /metrics from the snapshot store in place of the
// GaugeVecs, mirroring gpuStoreCollector. Descriptors are rebuilt from the live
// (populated) GaugeVec structure each scrape, so the store-served field/label set
// tracks the pull path exactly. Unchecked collector (Describe is a no-op) because
// the field set is only known post-population.
type nicStoreCollector struct {
	na *NICAgentClient
}

func (c *nicStoreCollector) Describe(ch chan<- *prometheus.Desc) {}

func (c *nicStoreCollector) Collect(ch chan<- prometheus.Metric) {
	descs, err := c.na.nicStoreFieldDescs()
	if err != nil {
		return
	}
	snapshotstore.NewStoreCollector(c.na.mh.GetStore(), descs).Collect(ch)
}

// writeNodeSlot writes the node-level nic_total metric (count of NICs, node
// label set) into the store under a reserved node key. Written explicitly and
// independently of GaugeVec introspection so it is always present; store COW
// keeps the last value across a zero/failed fetch (never cleared).
func (na *NICAgentClient) writeNodeSlot(store snapshotstore.StoreWriter) {
	if store == nil {
		return
	}
	labels := na.populateNonNICLabels()
	// nic_total is a boot/reconfiguration-time constant (the device count), not
	// telemetry. UpdateIfChanged stores it only when the count changes, so
	// steady-state scrapes never re-store it. Reserve the slot with a "nic-node:"
	// prefix (mirroring the GPU "gpu-node:" slot) so a NIC telemetry row carrying
	// exactly the node label set can never collide with it.
	store.UpdateIfChanged(nicNodeKeyPrefix+storeDeviceKey(labels), map[string]snapshotstore.Sample{
		nicNodesTotalField: {Value: float64(len(na.nics)), Labels: labels},
	})
}

// publishNICMetricsToStore mirrors the client's currently-populated NIC
// GaugeVecs into store: one Sample per (row, field), field keyed by metric name.
// Additive — the GaugeVecs are left untouched.
func (na *NICAgentClient) publishNICMetricsToStore(store snapshotstore.StoreWriter) error {
	// Always write the node slot first, independent of introspection.
	na.writeNodeSlot(store)

	fams, err := na.gatherNICFamilies()
	if err != nil {
		return err
	}
	byDevice := map[string]map[string]snapshotstore.Sample{}
	for _, mf := range fams {
		field := mf.GetName()
		if field == nicNodesTotalField {
			// Handled by writeNodeSlot; never mirror it from the GaugeVec.
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := make(prometheus.Labels, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			key := storeDeviceKey(labels)
			if byDevice[key] == nil {
				byDevice[key] = map[string]snapshotstore.Sample{}
			}
			byDevice[key][field] = snapshotstore.Sample{
				Value:  m.GetGauge().GetValue(),
				Labels: labels,
			}
		}
	}
	for key, fields := range byDevice {
		store.Update(key, fields)
	}
	return nil
}

// fetchMetricsOnce dedupes concurrent scrapes onto a single getMetricsAll fetch
// (Approach 1 singleflight), then mirrors the populated GaugeVecs into the store.
func (na *NICAgentClient) fetchMetricsOnce(ctx context.Context) error {
	return na.fetchDeduper.FetchOnce(nicFetchKey, func() error {
		if err := na.getMetricsAll(ctx); err != nil {
			// Fetch failed: drop stale NIC telemetry so /metrics serves only the
			// always-present nic_total signal until the next successful fetch
			// repopulates the full field set.
			na.dropStaleNICTelemetry()
			return err
		}
		return na.publishNICMetricsToStore(na.mh.GetStore())
	})
}

// dropStaleNICTelemetry evicts the NIC telemetry fields from the store on a fetch
// failure, keeping only the always-present nic_total signal. Scoped to NIC field
// names (gatherNICFamilies minus nic_total), so GPU/IFOE slots are untouched.
func (na *NICAgentClient) dropStaleNICTelemetry() {
	store := na.mh.GetStore()
	if store == nil {
		return
	}
	fams, err := na.gatherNICFamilies()
	if err != nil {
		return
	}
	drop := make(map[string]struct{}, len(fams))
	for _, mf := range fams {
		if mf.GetName() == nicNodesTotalField {
			continue
		}
		drop[mf.GetName()] = struct{}{}
	}
	store.DropFields(drop)
}

// NeedsPerScrapePull reports whether this NIC client serves /metrics via the
// reset+refill pull path. False: NIC is store-served (the store is refreshed via
// singleflight on scrape and the store collector reads lock-free), mirroring GPU.
func (na *NICAgentClient) NeedsPerScrapePull() bool {
	return false
}
