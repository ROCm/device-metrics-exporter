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

// GPU non-high-res pull path onto the single-source store (plan task T11,
// Approach 1). Additive: mirrors the client's populated GPU GaugeVecs into the
// shared snapshot store via Prometheus introspection and exposes descriptors for
// the generic StoreCollector, without changing the live GaugeVec population or
// wiring the StoreCollector into the live registry. The GaugeVecs are the source
// of truth for metric name/help/label derivation, so the store path cannot
// diverge from the pull path. Same structure as the NIC store writer
// (nicagent_store.go); the node-total slot follows the IFOE pattern.
package gpuagent

import (
	"context"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

const (
	gpuFetchKey = "gpu-pull"
	// gpuNodesTotalHelp mirrors the gpuNodesTotal GaugeVec help string.
	gpuNodesTotalHelp = "Number of GPUs in the node"
)

// useSnapshotStoreServing is true for every non-high-res GPU client: the snapshot
// store is the uniform storage+retrieval path, so /metrics always serves from it
// (per-scrape GPUGet populates the GaugeVecs and mirrors them into the store).
func (ga *GPUAgentGPUClient) useSnapshotStoreServing() bool {
	return !ga.gpuHandler.mh.IsHighResolution()
}

// useHighResStoreServing is true whenever high-res mode is on. High-res drives the
// GPUWatch feed into the store and serves /metrics from it — the single data path.
// Store-serving is unconditional; non-high-res serves from the same store via the
// per-scrape pull path.
func (ga *GPUAgentGPUClient) useHighResStoreServing() bool {
	return ga.gpuHandler.mh.IsHighResolution()
}

// gpuStoreCollector serves GPU /metrics from the snapshot store in place of the
// GaugeVecs. Descriptors are rebuilt from the live (populated) GaugeVec structure
// each scrape, so the store-served field/label set tracks the pull path exactly;
// the GaugeVecs remain internal mirror intermediates. Unchecked collector
// (Describe is a no-op) because the field set is only known post-population.
type gpuStoreCollector struct {
	ga *GPUAgentGPUClient
}

func (c *gpuStoreCollector) Describe(ch chan<- *prometheus.Desc) {}

func (c *gpuStoreCollector) Collect(ch chan<- prometheus.Metric) {
	var descs []snapshotstore.FieldDesc
	var err error
	if c.ga.useHighResStoreServing() {
		// High-res: the GaugeVecs are not populated (no per-scrape pull), so
		// descriptors are derived from the stream-fed store contents instead.
		descs = c.ga.gpuHighResStoreFieldDescs()
	} else {
		descs, err = c.ga.gpuStoreFieldDescs()
	}
	if err != nil {
		logger.Log.Printf("gpu store collector: field descs err: %v", err)
		return
	}
	snapshotstore.NewStoreCollector(c.ga.gpuHandler.mh.GetStore(), descs).Collect(ch)
}

// gpuHighResStoreFieldDescs builds StoreCollector descriptors by scanning the
// stream-fed store: one descriptor per field, with the label-name set being the
// union of label keys that field carries across device slots (indexed families
// add hbm_index/clock_type/etc.).
//
// The store is single-source — shared with the NIC/IFOE collectors in mixed
// deployments — so descriptors are restricted to GPU-owned fields; otherwise the
// GPU collector would emit NIC/IFOE series their own collectors already serve,
// duplicate-collecting and failing /metrics. The owned set is the stream-fed watch
// and profiler fields (whose GaugeVecs can be unpopulated here) unioned with the
// pull-path GPU GaugeVec families and the node total. gatherGPUFamilies also
// carries each field's established HELP, recovered here so the high-res exposition
// matches the flag-off GaugeVec surface (watch/profiler fields fall back to no HELP
// when their GaugeVec has not yet been populated by an initial/slow GPUGet).
func (ga *GPUAgentGPUClient) gpuHighResStoreFieldDescs() []snapshotstore.FieldDesc {
	prefix := ga.gpuHandler.mh.GetPrefix()
	store := ga.gpuHandler.mh.GetStore()
	if store == nil {
		return nil
	}
	// gpu_health is always owned: in high-res the store-based health poller writes it
	// directly to the store (not via a GaugeVec), so it won't appear in gatherGPUFamilies.
	ownedHelp := map[string]string{
		gpuHealthField: "Health of the GPU (0 = Unhealthy | 1 = Healthy)",
	}
	if ga.nodeTotalEnabled() {
		ownedHelp[gpuNodesTotalField] = gpuNodesTotalHelp
	}
	for f := range highResWatchFieldSet {
		ownedHelp[f] = ""
	}
	for f := range highResProfilerFieldSet {
		ownedHelp[f] = ""
	}
	if fams, ferr := ga.gatherGPUFamilies(); ferr == nil {
		for _, mf := range fams {
			ownedHelp[mf.GetName()] = mf.GetHelp()
		}
	} else {
		logger.Log.Printf("gpu high-res descs: gatherGPUFamilies err: %v", ferr)
	}
	labelKeys := map[string]map[string]struct{}{}
	for _, snap := range store.Snapshots() {
		snap.Range(func(field string, s snapshotstore.Sample) bool {
			if labelKeys[field] == nil {
				labelKeys[field] = map[string]struct{}{}
			}
			for k := range s.Labels {
				labelKeys[field][k] = struct{}{}
			}
			return true
		})
	}
	descs := make([]snapshotstore.FieldDesc, 0, len(labelKeys))
	for field, keys := range labelKeys {
		help, isGPU := ownedHelp[field]
		if !isGPU {
			// NIC/IFOE field in the shared store: served by their own collectors.
			continue
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		descs = append(descs, snapshotstore.FieldDesc{
			Field:      field,
			Name:       prefix + field,
			Help:       help,
			ValueType:  prometheus.GaugeValue,
			LabelNames: names,
		})
	}
	return descs
}

// gpuNodesTotalField is the store/gauge name of the node-level GPU-count metric.
var gpuNodesTotalField = strings.ToLower(exportermetrics.GPUMetricField_GPU_NODES_TOTAL.String())

// gpuHealthField is the store/gauge name of the per-GPU health metric. Together
// with gpuNodesTotalField these are the signal fields retained on a fetch
// failure (dropStaleGPUTelemetry keeps them, drops everything else); gpu_health
// is always present, gpu_nodes_total only when GPU_NODES_TOTAL is enabled.
var gpuHealthField = strings.ToLower(exportermetrics.GPUMetricField_GPU_HEALTH.String())

// dropStaleGPUTelemetry evicts the GPU telemetry fields from the store on a
// GPUGet failure so /metrics serves only the signal fields (the node
// gpu_nodes_total slot, when enabled, and per-GPU gpu_health) instead of
// last-known clocks, temps, etc. The drop set is the current GPU GaugeVec
// family names minus the two signal fields — scoped to GPU field names, so
// NIC/IFOE slots are untouched. The next successful getMetricsAll + publish
// merges the full set back (reconnect).
func (ga *GPUAgentGPUClient) dropStaleGPUTelemetry() {
	store := ga.gpuHandler.mh.GetStore()
	if store == nil {
		return
	}
	fams, err := ga.gatherGPUFamilies()
	if err != nil {
		return
	}
	drop := make(map[string]struct{}, len(fams))
	for _, mf := range fams {
		name := mf.GetName()
		if name == gpuHealthField || name == gpuNodesTotalField {
			continue
		}
		drop[name] = struct{}{}
	}
	store.DropFields(drop)
}

// gpuPullDeduper collapses concurrent pull-path scrapes onto one getMetricsAll
// fetch. Package-scoped (not a struct field) because GPUAgentGPUClient is defined
// in a sibling-owned file; one GPU client runs per process, so a shared deduper
// is equivalent to a per-client one.
var gpuPullDeduper = snapshotstore.NewDeduper()

// gpuStoreDeviceKey is a stable per-row store key built from a label set, omitting
// the volatile workload labels in skip. The key identifies the physical row (GPU
// identity + structural index labels), so a workload reassignment (pod/job change)
// updates the same slot instead of writing a new one and orphaning the old — the
// full label set still rides in Sample.Labels for rendering. Rows that differ only
// in a skipped label collapse onto one slot (a GPU carries one workload at a time).
// skip may be nil (no omission).
func gpuStoreDeviceKey(labels prometheus.Labels, skip map[string]struct{}) string {
	return snapshotstore.DeviceKey(labels, skip)
}

// workloadLabelKeys returns the set of label names whose values track the GPU's
// current workload (pod/job/process) and so change during steady-state operation
// without a config reload. They are excluded from the store device key so a
// workload reassignment reuses the GPU's slot (see gpuStoreDeviceKey); the values
// still appear in Sample.Labels. The static enum set is unioned with the
// config-defined extra pod labels, read under labelMapMu with the other label maps.
func (ga *GPUAgentGPUClient) workloadLabelKeys() map[string]struct{} {
	m := map[string]struct{}{
		strings.ToLower(exportermetrics.MetricLabel_POD.String()):               {},
		strings.ToLower(exportermetrics.MetricLabel_NAMESPACE.String()):         {},
		strings.ToLower(exportermetrics.MetricLabel_CONTAINER.String()):         {},
		strings.ToLower(exportermetrics.MetricLabel_POD_UUID.String()):          {},
		strings.ToLower(exportermetrics.MetricLabel_JOB_ID.String()):            {},
		strings.ToLower(exportermetrics.MetricLabel_JOB_USER.String()):          {},
		strings.ToLower(exportermetrics.MetricLabel_JOB_PARTITION.String()):     {},
		strings.ToLower(exportermetrics.MetricLabel_CLUSTER_NAME.String()):      {},
		strings.ToLower(exportermetrics.GPUMetricLabel_KFD_PROCESS_ID.String()): {},
	}
	ga.labelMapMu.RLock()
	for prometheusPodlabel := range ga.extraPodLabelsMap {
		m[strings.ToLower(prometheusPodlabel)] = struct{}{}
	}
	ga.labelMapMu.RUnlock()
	return m
}

// nodeStoreKey is the reserved, stable store key for the node-level gpu_nodes_total
// slot. Derived from the sorted node label-name set (never a device key), so the
// same slot is seeded and updated in place across scrapes.
func (ga *GPUAgentGPUClient) nodeStoreKey() string {
	names := append([]string(nil), ga.GetExporterNonGPULabels()...)
	sort.Strings(names)
	return "gpu-node:" + strings.Join(names, ",")
}

// gatherGPUFamilies returns the current exposition of this client's enabled GPU
// GaugeVecs. The enable gate mirrors initFieldRegistration so the set matches the
// live /metrics surface; only populated families appear.
func (ga *GPUAgentGPUClient) gatherGPUFamilies() ([]*dto.MetricFamily, error) {
	reg := prometheus.NewRegistry()
	// The background slow writer calls this during a config reload, when
	// initFieldConfig/initFieldMetricsMap rebuild exportFieldMap and fieldMetricsMap.
	// Read both under exportFieldMapMu (the lock those writers hold) so the
	// iteration cannot race or panic on a concurrent map replace.
	ga.exportFieldMapMu.RLock()
	for field, enabled := range ga.exportFieldMap {
		if !enabled {
			continue
		}
		fm, ok := ga.fieldMetricsMap[field]
		if !ok {
			continue
		}
		// Aliases share a collector; ignore the resulting duplicate-register error.
		_ = reg.Register(fm.Metric)
	}
	ga.exportFieldMapMu.RUnlock()
	return reg.Gather()
}

// gpuStoreFieldDescs builds StoreCollector descriptors mirroring the enabled GPU
// GaugeVecs: prefixed metric name, help, gauge type, and the gauge's variable
// label names. gpu_nodes_total is appended explicitly, gated on nodeTotalEnabled.
func (ga *GPUAgentGPUClient) gpuStoreFieldDescs() ([]snapshotstore.FieldDesc, error) {
	prefix := ga.gpuHandler.mh.GetPrefix()
	fams, err := ga.gatherGPUFamilies()
	if err != nil {
		return nil, err
	}
	descs := make([]snapshotstore.FieldDesc, 0, len(fams)+1)
	for _, mf := range fams {
		if len(mf.GetMetric()) == 0 || mf.GetName() == gpuNodesTotalField {
			// gpu_nodes_total is handled explicitly below, gated on nodeTotalEnabled.
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
	// Node-level GPU count: descriptor is emitted, when enabled, so the
	// StoreCollector renders it even at zero GPUs / before the first fetch.
	if ga.nodeTotalEnabled() {
		descs = append(descs, snapshotstore.FieldDesc{
			Field:      gpuNodesTotalField,
			Name:       prefix + gpuNodesTotalField,
			Help:       gpuNodesTotalHelp,
			ValueType:  prometheus.GaugeValue,
			LabelNames: ga.GetExporterNonGPULabels(),
		})
	}
	return descs, nil
}

// nodeTotalEnabled reports whether GPU_NODES_TOTAL is enabled in the config.
func (ga *GPUAgentGPUClient) nodeTotalEnabled() bool {
	return ga.getExporterFieldState(exportermetrics.GPUMetricField_GPU_NODES_TOTAL.String())
}

// gpuNodeTotalSample reads the current gpu_nodes_total value and node labels
// straight off the gpuNodesTotal GaugeVec. getMetricsAll Sets this gauge
// unconditionally (independent of the GPU_NODES_TOTAL config gate); callers
// gate on nodeTotalEnabled() before writing it into the store.
func (ga *GPUAgentGPUClient) gpuNodeTotalSample() (snapshotstore.Sample, bool) {
	reg := prometheus.NewRegistry()
	if err := reg.Register(ga.metrics.gpuNodesTotal); err != nil {
		return snapshotstore.Sample{}, false
	}
	fams, err := reg.Gather()
	if err != nil || len(fams) == 0 || len(fams[0].GetMetric()) == 0 {
		return snapshotstore.Sample{}, false
	}
	m := fams[0].GetMetric()[0]
	labels := make(prometheus.Labels, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	return snapshotstore.Sample{Value: m.GetGauge().GetValue(), Labels: labels}, true
}

// seedGPUNodeTotal publishes gpu_nodes_total=0 with node labels before any fetch,
// when GPU_NODES_TOTAL is enabled, so it is present on scrape #1 and persists
// (store COW) across zero/failed fetches.
func (ga *GPUAgentGPUClient) seedGPUNodeTotal() {
	store := ga.gpuHandler.mh.GetStore()
	if store == nil {
		return
	}
	if !ga.nodeTotalEnabled() {
		// Config reload may have disabled the field: evict any prior slot.
		store.DropFields(map[string]struct{}{gpuNodesTotalField: {}})
		return
	}
	labels := ga.populateLabelsFromGPU(nil, nil, nil)
	store.Update(ga.nodeStoreKey(), map[string]snapshotstore.Sample{
		gpuNodesTotalField: {Value: 0, Labels: labels},
	})
}

// writeGPUNodeSlot writes the node-level gpu_nodes_total metric into the store
// under the reserved node key, when GPU_NODES_TOTAL is enabled. Written
// independently of GaugeVec introspection; store COW keeps the last value
// across a zero/failed fetch.
func (ga *GPUAgentGPUClient) writeGPUNodeSlot(store snapshotstore.StoreWriter) {
	if store == nil || !ga.nodeTotalEnabled() {
		return
	}
	s, ok := ga.gpuNodeTotalSample()
	if !ok {
		// No gauge row yet: keep the seeded/last value (never cleared).
		return
	}
	// gpu_nodes_total is a boot/reconfiguration-time constant (the device count),
	// not telemetry. UpdateIfChanged stores it only when the value changes, so
	// steady-state scrapes and the slow-writer tick never re-store it.
	// seedGPUNodeTotal() resets the slot (to 0) on config (re)load via InitConfigs,
	// which re-triggers a single write here once the real count is next known.
	store.UpdateIfChanged(ga.nodeStoreKey(), map[string]snapshotstore.Sample{
		gpuNodesTotalField: s,
	})
}

// highResWatchFieldSet is the set of base field names fed by the fast GPUWatch
// stream writer (applyWatchSnapshot). Derived from highResWatchMetrics so it
// cannot drift from the watch set. The high-res slow writer skips these so it
// never clobbers the stream's fresh values; Store.Update merges, so the watch
// fields already in each device slot survive a non-watch-only update.
var highResWatchFieldSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(highResWatchMetrics))
	for _, wm := range highResWatchMetrics {
		if wm.name != "" {
			m[wm.name] = struct{}{}
		}
	}
	return m
}()

// --- P7 high-res profiler stream field set (keep as one delimited block) ---
// highResProfilerFieldSet is the set of gpu_prof_* store field names owned by the
// profiler stream writer in high-res mode. Derived from the profiler enum range so
// it cannot drift from the fields the stream can produce. Used both as the
// slow-writer skip-mask (so the per-scrape mirror never clobbers the stream's fresh
// values) and as the DropFields set when the stream is stopped/disabled. Scoped to
// profiler field names, so a drop leaves the node *_total slot, gpu_health, and all
// non-profiler fields untouched.
var highResProfilerFieldSet = func() map[string]struct{} {
	m := map[string]struct{}{}
	for i := profilerStarIndex; i <= profilerEndIndex; i++ {
		if name, ok := exportermetrics.GPUMetricField_name[i]; ok {
			m[strings.ToLower(name)] = struct{}{}
		}
	}
	return m
}()

// --- end P7 high-res profiler stream field set ---

// publishGPUMetricsToStore mirrors the client's currently-populated GPU GaugeVecs
// into store: one Sample per (row, field), field keyed by metric name. Additive —
// the GaugeVecs are left untouched.
func (ga *GPUAgentGPUClient) publishGPUMetricsToStore(store snapshotstore.StoreWriter) error {
	return ga.mirrorGPUFamiliesToStore(store, nil)
}

// mirrorGPUFamiliesToStore mirrors the populated GPU GaugeVec families into the
// store, one Sample per (row, field). When skip is non-nil, families whose base
// name it returns true for are excluded (the high-res slow writer passes the watch
// set so those stay owned by the stream writer). The node slot is always written.
func (ga *GPUAgentGPUClient) mirrorGPUFamiliesToStore(store snapshotstore.StoreWriter, skip func(field string) bool) error {
	if store == nil {
		return nil
	}
	// Write the node slot first (if enabled), independent of introspection.
	ga.writeGPUNodeSlot(store)

	fams, err := ga.gatherGPUFamilies()
	if err != nil {
		return err
	}
	workloadSkip := ga.workloadLabelKeys()
	byDevice := map[string]map[string]snapshotstore.Sample{}
	for _, mf := range fams {
		field := mf.GetName()
		if field == gpuNodesTotalField {
			// Handled by writeGPUNodeSlot; never mirror it from the GaugeVec.
			continue
		}
		if skip != nil && skip(field) {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := make(prometheus.Labels, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			key := gpuStoreDeviceKey(labels, workloadSkip)
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

// NeedsPerScrapePull reports whether this GPU client serves /metrics via the
// reset+refill pull path. False in either store-serving mode: non-high-res (store
// refreshed via singleflight on scrape) or high-res (store fed continuously by the
// GPUWatch stream writer). In both, the scrape mutex and in-place reset are not
// needed.
func (ga *GPUAgentGPUClient) NeedsPerScrapePull() bool {
	return !ga.useSnapshotStoreServing() && !ga.useHighResStoreServing()
}

// NeedsPerScrapePull on the composite agent is the OR over its sub-clients: the
// scrape mutex is still required if any sub-client (GPU in legacy/high-res mode,
// or IFOE) serves via reset+refill. False only when every sub-client is
// store-serving (GPU-only, store mode). Defined here (not in gpuagent.go) because
// the store-serving opt-out originates with the GPU store path.
func (ga *GPUAgentClient) NeedsPerScrapePull() bool {
	for _, client := range ga.clients {
		if client.NeedsPerScrapePull() {
			return true
		}
	}
	return len(ga.clients) == 0
}

// fetchMetricsOnce dedupes concurrent scrapes onto a single getMetricsAll fetch
// (Approach 1 singleflight), then mirrors the populated GaugeVecs into the store.
func (ga *GPUAgentGPUClient) fetchMetricsOnce(ctx context.Context) error {
	return gpuPullDeduper.FetchOnce(gpuFetchKey, func() error {
		if err := ga.getMetricsAll(ctx); err != nil {
			// GPUGet failed: drop stale device telemetry so /metrics serves only
			// gpu_nodes_total + gpu_health until the agent reconnects and the next
			// successful fetch repopulates the full field set.
			ga.dropStaleGPUTelemetry()
			return err
		}
		return ga.publishGPUMetricsToStore(ga.gpuHandler.mh.GetStore())
	})
}
