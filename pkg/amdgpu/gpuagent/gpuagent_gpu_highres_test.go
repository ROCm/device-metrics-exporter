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
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

func TestWatchAttrFloatFiltersNASentinel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		val    *amdgpu.GPUWatchAttrVal
		expVal float64
		expOk  bool
	}{
		{"nil", nil, 0, false},
		{"unset oneof", &amdgpu.GPUWatchAttrVal{}, 0, false},
		{
			"NA sentinel",
			&amdgpu.GPUWatchAttrVal{WatchAttrVal: &amdgpu.GPUWatchAttrVal_LongVal{LongVal: math.MaxUint64}},
			0, false,
		},
		{
			"long value",
			&amdgpu.GPUWatchAttrVal{WatchAttrVal: &amdgpu.GPUWatchAttrVal_LongVal{LongVal: 42}},
			42, true,
		},
		{
			// a lower-width max in a uint64 is a real reading, not an NA
			"uint32 max in uint64",
			&amdgpu.GPUWatchAttrVal{WatchAttrVal: &amdgpu.GPUWatchAttrVal_LongVal{LongVal: math.MaxUint32}},
			math.MaxUint32, true,
		},
		{
			"float value",
			&amdgpu.GPUWatchAttrVal{WatchAttrVal: &amdgpu.GPUWatchAttrVal_FloatVal{FloatVal: 1.5}},
			1.5, true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			val, ok := watchAttrFloat(tc.val)
			assert.Equal(t, ok, tc.expOk)
			assert.Equal(t, val, tc.expVal)
		})
	}
}

// newHighResMetricsHandler builds a MetricsHandler over a throwaway config
// file so a test can set the high-res periods without touching the shared
// package-level handler.
func newHighResMetricsHandler(t *testing.T, body string) *metricsutil.MetricsHandler {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	assert.NilError(t, os.WriteFile(cfgPath, []byte(body), 0644))
	cfg := config.NewConfigHandler(cfgPath, config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort})
	hrmh, err := metricsutil.NewMetrics(cfg)
	assert.NilError(t, err)
	hrmh.InitConfig(context.Background())
	return hrmh
}

func TestGetHighResPeriods(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	for _, tc := range []struct {
		name    string
		body    string
		expFast uint32
		expSlow uint32
	}{
		{
			"configured",
			`{"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":5,"SlowPeriodSecs":30}}}`,
			5, 30,
		},
		{
			// unset periods fall back to the high-res defaults (fast=1s, slow=5s),
			// applied in the config handler so every caller (the GPUWatch feed and
			// the profiler stream gate) gets a non-zero period; a zero fast period
			// would make rocpctl stream at max rate.
			"unset",
			`{"CommonConfig":{"HighResolution":{"Enable":true}}}`,
			1, 5,
		},
		{"no common config", `{}`, 1, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fast, slow := newHighResMetricsHandler(t, tc.body).GetHighResPeriods()
			assert.Equal(t, fast, tc.expFast)
			assert.Equal(t, slow, tc.expSlow)
		})
	}
}

func TestHighResSpecCarriesConfiguredPeriods(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	hrmh := newHighResMetricsHandler(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":5,"SlowPeriodSecs":30}}}`)

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

	watchMockCl := mock_gen.NewMockGPUWatchSvcClient(mockCtl)
	gpuclient.gpuclient = gpuMockCl
	gpuclient.evtclient = eventMockCl
	gpuclient.watchclient = watchMockCl
	// builds the prometheus registry and the GPUGet filter StartHighRes relies on;
	// the agent does this at startup before streaming
	assert.NilError(t, gpuclient.InitConfigs())

	var gotSpec *amdgpu.GPUWatchSpec
	watchMockCl.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).
		Return(&amdgpu.GPUWatchGetResponse{}, nil).AnyTimes()
	watchMockCl.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *amdgpu.GPUWatchRequest,
			_ ...grpc.CallOption) (*amdgpu.GPUWatchResponse, error) {
			gotSpec = in.GetSpec()[0]
			return &amdgpu.GPUWatchResponse{}, nil
		}).Times(1)
	// returning an error here stops StartHighRes right after create, so the test
	// needs no stream mock and spawns no consumer goroutine
	errStop := errors.New("stop after create")
	watchMockCl.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).
		Return(nil, errStop).Times(1)

	// Cancelable ctx: high-res store-serving StartHighRes launches the async
	// workload-label cache goroutine before subscribing; cancel it on return so it
	// does not outlive the test (and the mock controller).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := gpuclient.StartHighRes(ctx)
	assert.Equal(t, err, errStop)
	assert.Assert(t, gotSpec != nil)
	assert.Equal(t, gotSpec.GetFastPeriodSecs(), uint32(5))
	assert.Equal(t, gotSpec.GetSlowPeriodSecs(), uint32(30))
}

// hrGPUID is the ga.fl key for the synthetic GPU below: the pull path keys on
// the GPU instance index as a string, not the UUID.
const hrGPUID = "0"

// newHighResGPUClient builds a GPU client with its registry initialised and one
// synthetic GPU seeded into the high-res label caches, so applyWatchSnapshot can
// be driven directly without a live watch stream. Returns the client and the
// 16-byte GPU id the caller must put in the snapshot.
func newHighResGPUClient(t *testing.T) (*GPUAgentGPUClient, []byte) {
	t.Helper()
	// GPU_UUID is not in gpuMandatoryLables, so it has to be enabled explicitly
	// for the label set to carry the key the samples are keyed on
	hrmh := newHighResMetricsHandler(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)
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
	// build the label set through the production path, as StartHighRes does:
	// a hand-rolled subset has the wrong cardinality for the registered GaugeVecs,
	// which silently turns every registry assertion below into a tautology
	gpu := &amdgpu.GPU{
		Spec:   &amdgpu.GPUSpec{Id: gpuBytes},
		Status: &amdgpu.GPUStatus{Index: 0},
	}
	gpuclient.hrLabelsByUUID = map[string]prometheus.Labels{
		uuidStr: prometheus.Labels(gpuclient.populateLabelsFromGPU(nil, gpu, nil)),
	}
	gpuclient.hrGPUIDByUUID = map[string]string{uuidStr: hrGPUID}
	// fl is shared with the agent handler; clear filterDone so the first NA tick
	// actually logs and records rather than being suppressed by leftover state
	gpuclient.fl.Reset()
	return gpuclient, gpuBytes
}

func hrSnapshot(gpuBytes []byte, attrs ...*amdgpu.GPUWatchAttr) *amdgpu.GPUWatch {
	return &amdgpu.GPUWatch{Stats: &amdgpu.GPUWatchStats{
		GPUWatchAttr: []*amdgpu.GPUWatchAttrs{{GPU: gpuBytes, Attr: attrs}},
	}}
}

func hrLongAttr(id amdgpu.GPUWatchAttrId, val uint64) *amdgpu.GPUWatchAttr {
	return &amdgpu.GPUWatchAttr{
		Id:    id,
		Value: &amdgpu.GPUWatchAttrVal{WatchAttrVal: &amdgpu.GPUWatchAttrVal_LongVal{LongVal: val}},
	}
}

// hrDelta is one flattened store write: the base (unprefixed) field name plus its
// Sample. applyWatchSnapshot now returns the per-device store deltas it wrote
// (deviceKey -> field -> Sample) instead of a push-stream frame, so the per-sample
// assertions flatten that map into (field, Sample) pairs.
type hrDelta struct {
	field  string
	sample snapshotstore.Sample
}

// hrFlatten flattens the per-device store deltas into a (field, Sample) list. Each
// indexed element lands in its own device slot (its *_index label makes the device
// key unique), so a flat view recovers one entry per emitted sample.
func hrFlatten(deltas map[string]map[string]snapshotstore.Sample) []hrDelta {
	var out []hrDelta
	for _, fields := range deltas {
		for f, s := range fields {
			out = append(out, hrDelta{field: f, sample: s})
		}
	}
	return out
}

func hrSampleNames(deltas map[string]map[string]snapshotstore.Sample) []string {
	names := []string{}
	for _, d := range hrFlatten(deltas) {
		names = append(names, d.field)
	}
	return names
}

func hrIndexedLong(index uint32, subType string, val uint64) *amdgpu.GPUWatchAttrIndexedVal {
	return &amdgpu.GPUWatchAttrIndexedVal{
		Index:          index,
		SubType:        subType,
		IndexedAttrVal: &amdgpu.GPUWatchAttrIndexedVal_LongVal{LongVal: val},
	}
}

func hrIndexedAttr(id amdgpu.GPUWatchAttrId, vals ...*amdgpu.GPUWatchAttrIndexedVal) *amdgpu.GPUWatchAttr {
	return &amdgpu.GPUWatchAttr{Id: id, Value: &amdgpu.GPUWatchAttrVal{IndexedVal: vals}}
}

// hrFindSample returns the store delta sample for the given base field name whose
// indexLabel equals idxVal, or nil. For scalars pass indexLabel="" idxVal="" (every
// label set matches the empty key). The returned pointer aliases the delta's Labels
// map, so a mutation through it is observable on the stored sample.
func hrFindSample(deltas map[string]map[string]snapshotstore.Sample, name, indexLabel, idxVal string) *snapshotstore.Sample {
	for _, fields := range deltas {
		if s, ok := fields[name]; ok && s.Labels[indexLabel] == idxVal {
			sc := s
			return &sc
		}
	}
	return nil
}

func TestApplyWatchSnapshotFansOutIndexed(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResGPUClient(t)
	uuidStr := utils.UUIDToString(gpuBytes)

	snap := hrSnapshot(gpuBytes,
		// three HBM stacks; the middle one is NA and must be dropped
		hrIndexedAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_HBM_TEMP,
			hrIndexedLong(0, "", 60),
			hrIndexedLong(1, "", math.MaxUint64),
			hrIndexedLong(2, "", 62),
		),
		// a second indexed family on a different index label (per-XCC)
		hrIndexedAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_BUSY_INST,
			hrIndexedLong(0, "", 10),
			hrIndexedLong(1, "", 20),
		),
	)

	deltas := gpuclient.applyWatchSnapshot(snap)
	assert.Assert(t, deltas != nil)
	// 2 HBM (NA dropped) + 2 gfx_busy
	assert.Equal(t, len(hrFlatten(deltas)), 4)

	hbm0 := hrFindSample(deltas, "gpu_hbm_temperature", "hbm_index", "0")
	assert.Assert(t, hbm0 != nil)
	assert.Equal(t, hbm0.Value, float64(60))
	assert.Equal(t, hbm0.Labels["gpu_uuid"], uuidStr)
	assert.Assert(t, hrFindSample(deltas, "gpu_hbm_temperature", "hbm_index", "1") == nil)
	hbm2 := hrFindSample(deltas, "gpu_hbm_temperature", "hbm_index", "2")
	assert.Assert(t, hbm2 != nil)
	assert.Equal(t, hbm2.Value, float64(62))

	busy0 := hrFindSample(deltas, "gpu_gfx_busy_instantaneous", "xcc_index", "0")
	assert.Assert(t, busy0 != nil)
	assert.Equal(t, busy0.Value, float64(10))
	busy1 := hrFindSample(deltas, "gpu_gfx_busy_instantaneous", "xcc_index", "1")
	assert.Assert(t, busy1 != nil)
	assert.Equal(t, busy1.Value, float64(20))
	// the hbm_index key must not leak onto the xcc-indexed sample
	_, leaked := busy0.Labels["hbm_index"]
	assert.Equal(t, leaked, false)

	// Zero memory retention / isolation: the index key never leaks into the cache
	_, cached := gpuclient.hrLabelsByUUID[uuidStr]["hbm_index"]
	assert.Equal(t, cached, false)
}

func TestApplyWatchSnapshotFansOutClocks(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResGPUClient(t)
	uuidStr := utils.UUIDToString(gpuBytes)

	// clocks carry a sparse global clock_index plus a clock_type sub-label; the
	// NA element must drop, and clock_type must not leak across samples/cache.
	snap := hrSnapshot(gpuBytes,
		hrIndexedAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_CURRENT,
			hrIndexedLong(0, "system", 800),
			hrIndexedLong(8, "memory", 1090),
			hrIndexedLong(9, "video", math.MaxUint64),
		),
		hrIndexedAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_CLOCK_FREQ_MAX,
			hrIndexedLong(0, "system", 875),
		),
	)

	deltas := gpuclient.applyWatchSnapshot(snap)
	assert.Assert(t, deltas != nil)
	// 2 current (NA video dropped) + 1 max
	assert.Equal(t, len(hrFlatten(deltas)), 3)

	sys := hrFindSample(deltas, "gpu_clock", "clock_index", "0")
	assert.Assert(t, sys != nil)
	assert.Equal(t, sys.Value, float64(800))
	assert.Equal(t, sys.Labels["clock_type"], "system")
	assert.Equal(t, sys.Labels["gpu_uuid"], uuidStr)

	mem := hrFindSample(deltas, "gpu_clock", "clock_index", "8")
	assert.Assert(t, mem != nil)
	assert.Equal(t, mem.Labels["clock_type"], "memory")

	assert.Assert(t, hrFindSample(deltas, "gpu_clock", "clock_index", "9") == nil)

	mx := hrFindSample(deltas, "gpu_max_clock", "clock_index", "0")
	assert.Assert(t, mx != nil)
	assert.Equal(t, mx.Value, float64(875))
	assert.Equal(t, mx.Labels["clock_type"], "system")

	// clock_type / clock_index must not leak into the shared label cache
	_, cachedType := gpuclient.hrLabelsByUUID[uuidStr]["clock_type"]
	assert.Equal(t, cachedType, false)
	_, cachedIdx := gpuclient.hrLabelsByUUID[uuidStr]["clock_index"]
	assert.Equal(t, cachedIdx, false)
}

func TestFlatViolationScalars(t *testing.T) {
	// the flat (non-xcc) violation gauges ride the scalar path: named, but no
	// index label
	for id, name := range map[amdgpu.GPUWatchAttrId]string{
		amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_CURRENT_ACC_COUNTER:          "gpu_violation_current_accumulated_counter",
		amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PROC_HOT_RESIDENCY_ACC:       "gpu_violation_processor_hot_residency_accumulated",
		amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_HBM_THERMAL_RESIDENCY_PCT:    "gpu_violation_hbm_thermal_residency_percentage",
		amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACC: "gpu_violation_socket_thermal_residency_accumulated",
	} {
		assert.Equal(t, watchAttrMetricName(id), name)
		label, hasSub := watchAttrIndexLabel(id)
		assert.Equal(t, label, "")
		assert.Equal(t, hasSub, false)
	}
}

func TestApplyWatchSnapshotIndexedAllNALoggedOnce(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResGPUClient(t)
	assert.Equal(t, gpuclient.fl.checkUnsupportedFields(hrGPUID, "GPU_HBM_TEMPERATURE"), false)

	snap := hrSnapshot(gpuBytes,
		hrIndexedAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_HBM_TEMP,
			hrIndexedLong(0, "", math.MaxUint64),
			hrIndexedLong(1, "", math.MaxUint64),
		),
	)

	deltas := gpuclient.applyWatchSnapshot(snap)
	// every element NA -> no samples, and the field is marked unsupported once
	assert.Assert(t, deltas == nil)
	assert.Equal(t, gpuclient.fl.checkUnsupportedFields(hrGPUID, "GPU_HBM_TEMPERATURE"), true)
}

func TestApplyWatchSnapshotMarksNAFieldUnsupportedOnce(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResGPUClient(t)
	assert.Equal(t, gpuclient.fl.checkUnsupportedFields(hrGPUID, "GPU_JUNCTION_TEMPERATURE"), false)

	snap := hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, math.MaxUint64),
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100),
	)

	deltas := gpuclient.applyWatchSnapshot(snap)
	assert.Assert(t, deltas != nil)
	// the NA attr is dropped, the supported one still writes the store
	assert.DeepEqual(t, hrSampleNames(deltas), []string{"gpu_package_power"})
	// recorded on the first tick, so the second tick is short-circuited before
	// logUnsupportedField is reached again
	assert.Equal(t, gpuclient.fl.checkUnsupportedFields(hrGPUID, "GPU_JUNCTION_TEMPERATURE"), true)

	deltas = gpuclient.applyWatchSnapshot(snap)
	assert.Assert(t, deltas != nil)
	assert.DeepEqual(t, hrSampleNames(deltas), []string{"gpu_package_power"})
}

// Zero memory retention: the pull path owns the registry and resets it under
// prometheusMiddlewareMu, held across serialization. This goroutine cannot take
// that lock (pkg/exporter imports this package), so a gauge write from here
// could land after the reset and be served as a leftover series.
func TestApplyWatchSnapshotDoesNotWriteRegistry(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResGPUClient(t)
	before := testutil.CollectAndCount(gpuclient.metrics.gpuPackagePower)

	deltas := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100),
	))
	assert.Assert(t, deltas != nil)
	assert.DeepEqual(t, hrSampleNames(deltas), []string{"gpu_package_power"})
	assert.Equal(t, testutil.CollectAndCount(gpuclient.metrics.gpuPackagePower), before)
}

func TestApplyWatchSnapshotCopiesLabelsPerSample(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResGPUClient(t)
	uuidStr := utils.UUIDToString(gpuBytes)

	deltas := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100),
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_UTIL, 50),
	))
	assert.Assert(t, deltas != nil)
	assert.Equal(t, len(hrFlatten(deltas)), 2)

	// indexed attrs will add a per-sample index key, so no sample may share the
	// cached map with another sample or with the cache itself. Mutating one
	// sample's labels (via the aliasing pointer) must not touch the other sample
	// or the per-UUID cache.
	pow := hrFindSample(deltas, "gpu_package_power", "", "")
	act := hrFindSample(deltas, "gpu_gfx_activity", "", "")
	assert.Assert(t, pow != nil && act != nil)
	pow.Labels["gpu_uuid"] = "mutated"
	assert.Equal(t, act.Labels["gpu_uuid"], uuidStr)
	assert.Equal(t, gpuclient.hrLabelsByUUID[uuidStr]["gpu_uuid"], uuidStr)
}

func TestWatchAttrIndexLabel(t *testing.T) {
	for _, tc := range []struct {
		id         amdgpu.GPUWatchAttrId
		label      string
		hasSubType bool
		name       string
	}{
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_HBM_TEMP, "hbm_index", false, "gpu_hbm_temperature"},
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VCN_ACTIVITY, "vcn_index", false, "gpu_vcn_activity"},
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_ACTIVITY, "jpeg_index", false, "gpu_jpeg_activity"},
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_RX, "link_index", false, "gpu_xgmi_link_rx"},
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_XGMI_LINK_TX, "link_index", false, "gpu_xgmi_link_tx"},
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_LOW_UTIL_ACC, "xcc_index", false,
			"gpu_violation_low_utilization_accumulated"},
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_BUSY_INST, "xcc_index", false, "gpu_gfx_busy_instantaneous"},
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_JPEG_BUSY_INST, "xcc_index", false, "gpu_jpeg_busy_instantaneous"},
		// scalars resolve to no index label
		{amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, "", false, "gpu_junction_temperature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			label, hasSub := watchAttrIndexLabel(tc.id)
			assert.Equal(t, label, tc.label)
			assert.Equal(t, hasSub, tc.hasSubType)
			assert.Equal(t, watchAttrMetricName(tc.id), tc.name)
		})
	}
}

// TestHighResSeedCarriesWorkloadLabelKeys reproduces the stream-vs-pull label gap:
// the seed/cache label set must carry the mandatory workload keys (pod, namespace,
// container, job_*) as empty strings when no workload matches, so streamed samples
// are 1-1 with the pull path. Asserts populateLabelsFromGPU emits them.
func TestHighResSeedCarriesWorkloadLabelKeys(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)
	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)
	gpu := &amdgpu.GPU{Spec: &amdgpu.GPUSpec{Id: gpuBytes}, Status: &amdgpu.GPUStatus{Index: 0}}
	labels := gpuclient.populateLabelsFromGPU(nil, gpu, nil)
	t.Logf("seed label keys: %v", labels)
	want := []string{"pod", "namespace", "container", "job_id", "job_user", "job_partition"}
	for _, k := range want {
		if _, ok := labels[k]; !ok {
			t.Errorf("mandatory workload label key %q missing from populateLabelsFromGPU output", k)
		}
	}
	// End-to-end: the stored sample must carry the same keys (seed path, no cache).
	// This is the pull/store parity the user observed broken.
	deltas := gpuclient.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100),
	))
	flat := hrFlatten(deltas)
	if len(flat) == 0 {
		t.Fatal("expected a store delta sample")
	}
	got := flat[0].sample.Labels
	t.Logf("store delta label keys: %v", got)
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("store delta sample missing mandatory workload label key %q", k)
		}
	}
}
