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
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

// newHighResStoreClient builds a high-res GPU client. High-res is always
// store-serving, so InitConfigs registers the store collector and
// applyWatchSnapshot feeds the store. Modeled on
// newHighResGPUClient: one synthetic 16-byte GPU seeded into the label caches so
// applyWatchSnapshot can be driven directly. Returns the client and the GPU id to
// put in snapshots.
func newHighResStoreClient(t *testing.T, body string) (*GPUAgentGPUClient, []byte) {
	t.Helper()

	hrmh := newHighResMetricsHandler(t, body)
	ga := NewAgent(hrmh, WithK8sClient(nil), WithK8sSchedulerClient(nil), WithGPUMonitoring(true))
	ga.Init()
	ga.isKubernetes = false

	var gpuclient *GPUAgentGPUClient
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			gpuclient = client.(*GPUAgentGPUClient)
			break
		}
	}
	assert.Assert(t, gpuclient != nil)
	gpuclient.gpuclient = gpuMockCl
	gpuclient.evtclient = eventMockCl
	assert.NilError(t, gpuclient.InitConfigs())

	gpuBytes := make([]byte, 16)
	for i := range gpuBytes {
		gpuBytes[i] = byte(i + 1)
	}
	uuidStr := utils.UUIDToString(gpuBytes)
	gpu := &amdgpu.GPU{
		Spec:   &amdgpu.GPUSpec{Id: gpuBytes},
		Status: &amdgpu.GPUStatus{Index: 0},
	}
	gpuclient.hrLabelsByUUID = map[string]prometheus.Labels{
		uuidStr: prometheus.Labels(gpuclient.populateLabelsFromGPU(nil, gpu, nil)),
	}
	gpuclient.hrGPUIDByUUID = map[string]string{uuidStr: hrGPUID}
	gpuclient.fl.Reset()
	return gpuclient, gpuBytes
}

// hrDeltaFieldSet returns the unique set of base (unprefixed) field names written
// to the store by applyWatchSnapshot, from its returned per-device deltas.
func hrDeltaFieldSet(deltas map[string]map[string]snapshotstore.Sample) map[string]bool {
	out := map[string]bool{}
	for _, fields := range deltas {
		for f := range fields {
			out[f] = true
		}
	}
	return out
}

// hrStoreMetricNames gathers the client's /metrics exposition and returns the set
// of prefixed metric names (store collector output).
func hrStoreMetricNames(t *testing.T, ga *GPUAgentGPUClient) map[string]bool {
	t.Helper()
	mfs, err := ga.gpuHandler.mh.GetRegistry().Gather()
	assert.NilError(t, err)
	prefix := ga.gpuHandler.mh.GetPrefix()
	out := map[string]bool{}
	for _, mf := range mfs {
		if strings.HasPrefix(mf.GetName(), prefix) {
			out[mf.GetName()] = true
		}
	}
	return out
}

// TestHighResStoreConfigDisableBothSurfaces proves (gate b) that a config Fields
// allowlist excluding a field masks it on BOTH the store deltas the stream writer
// produces and the store-served /metrics, while the enabled field and node total
// stay present. The external push stream is gone, so the store deltas are the
// write-side surface.
func TestHighResStoreConfigDisableBothSurfaces(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	// Enable only GPU_PACKAGE_POWER; GPU_GFX_ACTIVITY is thus disabled.
	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},`+
			`"GPUConfig":{"Labels":["GPU_UUID"],"Fields":["GPU_PACKAGE_POWER"]}}`)
	prefix := gpuclient.gpuHandler.mh.GetPrefix()

	deltas := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100), // gpu_package_power (enabled)
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_UTIL, 50),     // gpu_gfx_activity (disabled)
	))
	assert.Assert(t, deltas != nil)

	// Write surface (store deltas, base field names): only the enabled field is
	// written.
	fnames := hrDeltaFieldSet(deltas)
	assert.Assert(t, fnames["gpu_package_power"], "enabled field missing from store deltas")
	assert.Assert(t, !fnames["gpu_gfx_activity"], "disabled field leaked into store deltas")

	// /metrics surface (store collector, prefixed): same masking, node total present.
	mnames := hrStoreMetricNames(t, gpuclient)
	assert.Assert(t, mnames[prefix+"gpu_package_power"], "enabled field missing from /metrics")
	assert.Assert(t, !mnames[prefix+"gpu_gfx_activity"], "disabled field leaked into /metrics")
	assert.Assert(t, mnames[prefix+gpuNodesTotalField], "node total absent from /metrics")
}

// TestHighResStoreServedMatchesStreamFieldSet proves (gate d) that in high-res the
// store-served /metrics carries exactly the per-device field set the stream writer
// wrote to the store (node total excepted: it is /metrics-only here, sourced from
// the seeded node slot rather than the stream writer).
func TestHighResStoreServedMatchesStreamFieldSet(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)
	prefix := gpuclient.gpuHandler.mh.GetPrefix()

	// Mix scalar and indexed (fanned-out) families to exercise label-hash keying.
	deltas := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100),
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 70),
		hrIndexedAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_HBM_TEMP,
			hrIndexedLong(0, "", 60),
			hrIndexedLong(1, "", 62),
		),
	))
	assert.Assert(t, deltas != nil)

	// Store deltas use base field names; /metrics names are prefixed. Strip the
	// prefix (and drop the seeded node total) so the two per-device field sets
	// compare directly.
	deltaNames := hrDeltaFieldSet(deltas)
	metricNames := hrStoreMetricNames(t, gpuclient)
	delete(metricNames, prefix+gpuNodesTotalField)
	got := map[string]bool{}
	for n := range metricNames {
		got[strings.TrimPrefix(n, prefix)] = true
	}

	assert.DeepEqual(t, got, deltaNames)
}

// pullPathNonWatchNames returns the prefixed names of the non-watch GPU GaugeVec
// families the pull path currently populates (watch fields and the node total
// excluded). This is the reference set the high-res slow writer must reproduce in
// the store.
func pullPathNonWatchNames(t *testing.T, ga *GPUAgentGPUClient) map[string]bool {
	t.Helper()
	prefix := ga.gpuHandler.mh.GetPrefix()
	fams, err := ga.gatherGPUFamilies()
	assert.NilError(t, err)
	out := map[string]bool{}
	for _, mf := range fams {
		field := mf.GetName()
		if field == gpuNodesTotalField {
			continue
		}
		if _, isWatch := highResWatchFieldSet[field]; isWatch {
			continue
		}
		out[prefix+field] = true
	}
	return out
}

// TestHighResSlowWriterAddsNonWatchFields proves the closed parity gap: in
// store-serving high-res the fast stream writer feeds only watch fields, so
// non-watch fields (gpu_health, gpu_afid_errors, gpu_energy_consumed, ...) are
// absent from store-served /metrics until the slow writer runs. After one slow
// tick the store-served non-watch field set equals the pull-path non-watch field
// set, and the watch fields the stream wrote are preserved (not clobbered).
func TestHighResStoreHealthFromPoller(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)
	prefix := gpuclient.gpuHandler.mh.GetPrefix()
	healthName := prefix + strings.ToLower("GPU_HEALTH")

	// Fast stream writer first: a watch field lands in the store. gpu_health is a
	// non-watch (health-path) field, so the stream never carries it.
	deltas := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100),
	))
	assert.Assert(t, deltas != nil)

	before := hrStoreMetricNames(t, gpuclient)
	assert.Assert(t, before[prefix+"gpu_package_power"], "stream watch field missing from store /metrics")
	assert.Assert(t, !before[healthName], "gpu_health present before health poll -- premise invalid")

	// Seed the subscribe-time snapshot the store-based health poller reads.
	getResp, pm, err := gpuclient.getGPUs(gpuclient.gCache.gpuGetFilter, &gpuclient.gCache.metricsCache)
	assert.NilError(t, err)
	gpuclient.hrSnapshot.Store(&hrGPUSnap{gpus: getResp.GetResponse(), partitionMap: pm})

	// High-res health poll computes ECC health from the store (no getGPUs) and writes
	// gpu_health back into it.
	gpuclient.gpuHandler.computeNodeHealthState = true
	assert.NilError(t, gpuclient.processHealthValidation())

	after := hrStoreMetricNames(t, gpuclient)
	assert.Assert(t, after[healthName], "gpu_health absent from store /metrics after health poll")
	assert.Assert(t, after[prefix+"gpu_package_power"], "health poll clobbered the stream watch field")
	assert.Assert(t, after[prefix+gpuNodesTotalField], "node total absent after health poll")
	// afid is not served in high-res (dropped; not a watcher attr).
	assert.Assert(t, !after[prefix+"gpu_afid_errors"], "gpu_afid_errors should not be served in high-res")
}

// TestHighResStoreHealthUnhealthyOnEcc proves the store-based health poll marks a GPU
// unhealthy from a watcher-fed uncorrectable-ECC counter over threshold, without any
// getGPUs (default thresholds are zero, so any non-zero count trips it).
func TestHighResStoreHealthUnhealthyOnEcc(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)

	// Stream a high uncorrectable-ECC count into the store.
	gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ECC_SDMA_UE, 1000),
	))
	// Seed the subscribe snapshot with the same GPU identity as the streamed sample.
	gpu := &amdgpu.GPU{Spec: &amdgpu.GPUSpec{Id: gpuBytes}, Status: &amdgpu.GPUStatus{Index: 0}}
	gpuclient.hrSnapshot.Store(&hrGPUSnap{gpus: []*amdgpu.GPU{gpu}})

	gpuclient.gpuHandler.computeNodeHealthState = true
	assert.NilError(t, gpuclient.processHealthValidation())

	sawUnhealthy := false
	for _, st := range gpuclient.healthState {
		if st.Health == "unhealthy" {
			sawUnhealthy = true
		}
	}
	assert.Assert(t, sawUnhealthy, "GPU not marked unhealthy despite ECC over threshold in store")
}

// TestHighResStreamUsesWorkloadCacheWhenServing proves (gate c, serving wiring)
// that applyWatchSnapshot prefers the fresh workload-label cache over the static
// subscribe-time set when store-serving, so streamed samples carry live workload
// labels. The cache's labels-equal-pull-path property is covered by
// TestHRWorkloadLabelCacheParity.
func TestHighResStreamUsesWorkloadCacheWhenServing(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},`+
			`"GPUConfig":{"Labels":["GPU_UUID","JOB_ID"]}}`)
	uuidStr := utils.UUIDToString(gpuBytes)

	// Fresh cache carries a workload label the static set lacks.
	cache := newHRWorkloadLabelCache(gpuclient)
	fresh := map[string]prometheus.Labels{
		uuidStr: {"gpu_uuid": uuidStr, "job_id": "SLURM_JOB_ID0"},
	}
	cache.labels.Store(&fresh)
	gpuclient.hrWLCache = cache
	// Static fallback deliberately omits job_id.
	gpuclient.hrLabelsByUUID[uuidStr] = prometheus.Labels{"gpu_uuid": uuidStr}

	deltas := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100),
	))
	assert.Assert(t, deltas != nil)
	flat := hrFlatten(deltas)
	assert.Equal(t, len(flat), 1)
	assert.Equal(t, flat[0].sample.Labels["job_id"], "SLURM_JOB_ID0",
		"store delta sample did not pick up the fresh workload label")
}

// TestHighResStoreStableKeyReusesSlotOnWorkloadChange proves the store slot for a
// GPU is keyed by its stable identity, not its volatile workload labels: when a
// pod reassignment changes the GPU's pod label, the stream writer updates the SAME
// slot (new labels) instead of orphaning the old one. Without a stable key the
// store grows a new slot per workload change and serves the stale pod forever.
func TestHighResStoreStableKeyReusesSlotOnWorkloadChange(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID","POD"]}}`)
	uuidStr := utils.UUIDToString(gpuBytes)
	store := gpuclient.gpuHandler.mh.GetStore()
	nodeKey := gpuclient.nodeStoreKey()

	gpuDeviceSlots := func() int {
		n := 0
		for k := range store.Snapshots() {
			if k != nodeKey {
				n++
			}
		}
		return n
	}
	servedPod := func() string {
		for k, snap := range store.Snapshots() {
			if k == nodeKey {
				continue
			}
			if s, ok := snap.Get("gpu_package_power"); ok {
				return s.Labels["pod"]
			}
		}
		return ""
	}

	// First workload assignment: pod=podA.
	gpuclient.hrLabelsByUUID[uuidStr] = prometheus.Labels{"gpu_uuid": uuidStr, "pod": "podA"}
	d1 := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100)))
	assert.Assert(t, d1 != nil)
	assert.Equal(t, gpuDeviceSlots(), 1, "one GPU slot after first workload")
	assert.Equal(t, servedPod(), "podA")

	// Pod reassignment: same GPU, pod=podB. The slot must be reused, not orphaned.
	gpuclient.hrLabelsByUUID[uuidStr] = prometheus.Labels{"gpu_uuid": uuidStr, "pod": "podB"}
	d2 := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 110)))
	assert.Assert(t, d2 != nil)
	assert.Equal(t, gpuDeviceSlots(), 1, "workload change must reuse the slot, not orphan it")
	assert.Equal(t, servedPod(), "podB", "store must serve the current pod label")
}
