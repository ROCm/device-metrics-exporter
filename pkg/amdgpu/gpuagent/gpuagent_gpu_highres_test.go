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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
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
	assert.Equal(t, gotSpec.GetFastPeriodMs(), uint32(5000))
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
	want := []string{"pod", "namespace", "container", "job_id", "job_user", "job_partition", "job_account"}
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

// TestHighResFrameApply: below 1000 ms a partial frame (no Spec.Attribute)
// updates hot cells; a full frame, and any frame at 1000 ms, takes the full
// write and registers no hot cell.
func TestHighResFrameApply(t *testing.T) {
	temp := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP
	for _, tc := range []struct {
		name     string
		fastMs   uint32
		full     bool
		attr     amdgpu.GPUWatchAttrId
		field    string
		wantCell bool
	}{
		{"full frame at 10 ms", 10, true, temp, "gpu_junction_temperature", false},
		{"partial frame at 10 ms", 10, false, hrAttrGPUPowerUsage, "gpu_power_usage", true},
		{"partial-shaped frame at 1000 ms", 1000, false, temp, "gpu_junction_temperature", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ga, gpus := newHRFastClient(t, 1, tc.fastMs)
			w := hrPartial(gpus, hrAttr(tc.attr, 50))
			if tc.full {
				w = hrFull(gpus, hrAttr(tc.attr, 50))
			}
			ctx, cancel := context.WithCancel(context.Background())
			s := &blockingStream{ctx: ctx, frames: []*amdgpu.GPUWatch{w}, drained: make(chan struct{})}
			done := make(chan struct{})
			go func() {
				defer close(done)
				ga.consumeWatchStream(ctx, s)
			}()
			<-s.drained // every queued frame has been applied
			cancel()
			<-done
			assert.Equal(t, hrMustGet(t, ga, gpus[0], tc.field), 50.0)
			assert.Equal(t, hrGPU(ga, gpus[0]).fields[hrAttrGPUPowerUsage].cell != nil, tc.wantCell)
		})
	}
}

// TestHighResNAHandling: N/A is never stored and never written as 0, and is
// recorded unsupported once. On a partial frame a valid value that later goes
// N/A keeps the prior value; on a full frame UBB valid -> N/A -> valid stays
// suppressed, like gpu_package_power.
func TestHighResNAHandling(t *testing.T) {
	t.Run("partial frame", func(t *testing.T) {
		ga, gpus := newHRFastClient(t, 1, 10)
		volt := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE
		ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, hrNA), hrAttr(volt, 800)))
		_, ok := hrGet(ga, gpus[0], "gpu_power_usage")
		assert.Assert(t, !ok, "N/A must not be stored")
		_, ok = hrGet(ga, gpus[0], "gpu_package_power")
		assert.Assert(t, !ok, "N/A must not feed gpu_package_power")
		assert.Assert(t, ga.fl.checkUnsupportedFields("0", "GPU_POWER_USAGE"))
		f := hrGPU(ga, gpus[0]).fields[hrAttrGPUPowerUsage]
		assert.Assert(t, f.naSeen && f.skip && f.aliasSkip)
		assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_gfx_voltage"), 800.0)

		// valid then N/A after the filter pass: prior value kept, nothing written
		ga.fl.SetFilterDone()
		ga.applyPowerFrame(hrPartial(gpus, hrAttr(volt, hrNA)))
		assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_gfx_voltage"), 800.0)
		ga.applyPowerFrame(hrPartial(gpus, hrAttr(volt, 810)))
		assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_gfx_voltage"), 810.0)
	})
	t.Run("full frame UBB", func(t *testing.T) {
		tcase := setupTest(t)
		defer tcase(t)

		gc, gpuBytes := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
		ubb := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER
		pkg := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE
		tick := func(u, p uint64) map[string]bool {
			return hrDeltaFieldSet(gc.applyWatchSnapshot(hrSnapshot(gpuBytes, hrLongAttr(ubb, u), hrLongAttr(pkg, p))))
		}
		f := tick(1500, 1500)
		assert.Assert(t, f["gpu_ubb_power"] && f["gpu_package_power"])
		f = tick(hrNA, 100)
		assert.DeepEqual(t, f, map[string]bool{"gpu_package_power": true})
		assert.Equal(t, gc.fl.checkUnsupportedFields(hrGPUID, ubbField), true)
		f = tick(1600, hrNA)
		assert.Equal(t, f["gpu_ubb_power"], false, "UBB must stay suppressed after N/A")
		assert.Equal(t, f["gpu_package_power"], false)
	})
}

// TestHighResPackagePowerRule: the gpu_package_power rule across frame kinds
// and periods.
func TestHighResPackagePowerRule(t *testing.T) {
	temp := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP
	for _, tc := range []struct {
		name string
		run  func(t *testing.T)
	}{
		// per GPU, a valid 126 drives gpu_package_power and suppresses the full
		// frame's 5; an N/A 126 hands it back to 5 through the alias cell with
		// no epoch bump; a valid 126 takes over again.
		{"126 drives, N/A hands back to 5", func(t *testing.T) {
			ga, gpus := newHRFastClient(t, 2, 10)
			st := ga.gpuHandler.mh.GetStore()
			a, b := gpus[0], gpus[1]
			ga.fl.SetFilterDone() // a transient N/A must not mark 126 unsupported here
			frame := &amdgpu.GPUWatch{
				Spec: &amdgpu.GPUWatchSpec{},
				Stats: &amdgpu.GPUWatchStats{GPUWatchAttr: []*amdgpu.GPUWatchAttrs{
					{GPU: a, Attr: []*amdgpu.GPUWatchAttr{hrAttr(hrAttrGPUPowerUsage, 500)}},
					{GPU: b, Attr: []*amdgpu.GPUWatchAttr{hrAttr(hrAttrGPUPowerUsage, hrNA)}},
				}},
			}
			ga.applyPowerFrame(frame)
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 500.0)
			assert.Assert(t, hrGPU(ga, a).pkgFrom126)
			assert.Assert(t, !hrGPU(ga, b).pkgFrom126)
			_, ok := hrGet(ga, b, "gpu_package_power")
			assert.Assert(t, !ok)

			ga.applyWatchSnapshot(hrFull(gpus, hrAttr(hrAttrPowerUsage, 450)))
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 500.0, "5 must be skipped while 126 is valid")
			assert.Equal(t, hrMustGet(t, ga, b, "gpu_package_power"), 450.0, "126 N/A: 5 feeds gpu_package_power")

			ga.applyPowerFrame(hrPartial([][]byte{a}, hrAttr(hrAttrGPUPowerUsage, hrNA)))
			assert.Assert(t, !hrGPU(ga, a).pkgFrom126)
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 500.0, "N/A keeps the prior value")
			epoch := st.Epoch()
			ga.applyWatchSnapshot(hrFull([][]byte{a}, hrAttr(hrAttrPowerUsage, 440)))
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 440.0)
			assert.Equal(t, st.Epoch(), epoch, "write-through must not orphan the alias cell")

			ga.applyPowerFrame(hrPartial([][]byte{a}, hrAttr(hrAttrGPUPowerUsage, 520)))
			ga.applyWatchSnapshot(hrFull([][]byte{a}, hrAttr(hrAttrPowerUsage, 430)))
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 520.0)
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_power_usage"), 520.0)
		}},
		// at 1000 ms 126 never feeds gpu_package_power and 5 does.
		{"inert at 1000 ms", func(t *testing.T) {
			ga, gpus := newHRFastClient(t, 1, 1000)
			ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500)))
			assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 500.0)
			_, ok := hrGet(ga, gpus[0], "gpu_package_power")
			assert.Assert(t, !ok)
			assert.Assert(t, !hrGPU(ga, gpus[0]).pkgFrom126)
			ga.applyWatchSnapshot(hrFull(gpus, hrAttr(hrAttrPowerUsage, 450)))
			assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_package_power"), 450.0)
		}},
		// at sub-second periods a full frame writes no power-family name; at
		// 1000 ms it writes them.
		{"full frame power names by period", func(t *testing.T) {
			attrs := append(hrPowerTick(7), hrAttr(temp, 50))
			ga, gpus := newHRFastClient(t, 1, 10)
			got := hrDeltaFieldSet(ga.applyWatchSnapshot(hrFull(gpus, attrs...)))
			assert.DeepEqual(t, got, map[string]bool{"gpu_junction_temperature": true})

			ga, gpus = newHRFastClient(t, 1, 1000)
			got = hrDeltaFieldSet(ga.applyWatchSnapshot(hrFull(gpus, attrs...)))
			for _, id := range hrPowerAttrIDs {
				if name := highResWatchNames[id]; name != "" {
					assert.Assert(t, got[name], "%s not written at 1000 ms", name)
				}
			}
		}},
		// 126 keeps feeding gpu_package_power when 5 is N/A or already marked
		// unsupported, across cache rebuilds; an N/A 5 does not mark
		// GPU_PACKAGE_POWER unsupported once the 126 alias is live.
		{"126 when 5 is N/A or unsupported", func(t *testing.T) {
			ga, gpus := newHRFastClient(t, 2, 10)
			a, b := gpus[0], gpus[1]
			ga.fl.markUnsupportedFields("0", hrPackagePowerField)
			ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500)))
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 500.0)
			ga.applyWatchSnapshot(hrFull([][]byte{b}, hrAttr(hrAttrPowerUsage, hrNA)))
			assert.Assert(t, !ga.fl.checkUnsupportedFields("1", hrPackagePowerField))
			ga.exportFieldGen.Add(1) // a rebuild keeps 126 as the source
			ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 510)))
			assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 510.0)
			assert.Equal(t, hrMustGet(t, ga, b, "gpu_package_power"), 510.0)
		}},
		// alternating partial and full frames: the rule holds through the
		// sequence, and after round 1 the store epoch and the alias cell are
		// unchanged (full frames never replace hot samples).
		{"alternating frames do not churn", func(t *testing.T) {
			ga, gpus := newHRFastClient(t, 1, 1)
			st := ga.gpuHandler.mh.GetStore()
			ga.fl.SetFilterDone()
			steps := []struct {
				w    *amdgpu.GPUWatch
				want float64
			}{
				{hrPartial(gpus, append(hrPowerTick(9), hrAttr(hrAttrGPUPowerUsage, 350))...), 350},
				{hrFull(gpus, hrAttr(hrAttrPowerUsage, 300), hrAttr(temp, 40)), 350},
				{hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, hrNA)), 350},
				{hrFull(gpus, hrAttr(hrAttrPowerUsage, 310), hrAttr(temp, 41)), 310},
				{hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 360)), 360},
				{hrFull(gpus, hrAttr(hrAttrPowerUsage, 320), hrAttr(temp, 42)), 360},
			}
			var epoch uint64
			var cell *snapshotstore.HotCell
			for round := 0; round < 3; round++ {
				for i, s := range steps {
					if s.w.Spec.Attribute == nil {
						ga.applyPowerFrame(s.w)
					} else {
						ga.applyWatchSnapshot(s.w)
					}
					assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_package_power"), s.want, "round %d step %d", round, i)
				}
				if round == 0 {
					epoch, cell = st.Epoch(), hrGPU(ga, gpus[0]).fields[hrAttrGPUPowerUsage].alias
					continue
				}
				assert.Equal(t, st.Epoch(), epoch)
				assert.Assert(t, hrGPU(ga, gpus[0]).fields[hrAttrGPUPowerUsage].alias == cell)
			}
		}},
	} {
		t.Run(tc.name, tc.run)
	}
}

// TestHighResAllowlist: the GPUConfig.Fields allowlist is honoured on either
// power name (partial frames) and on gpu_ubb_power (full frames).
func TestHighResAllowlist(t *testing.T) {
	volt := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE
	for _, tc := range []struct {
		field string
		want  map[string]bool
	}{
		{"GPU_POWER_USAGE", map[string]bool{"gpu_power_usage": true}},
		{"GPU_PACKAGE_POWER", map[string]bool{"gpu_package_power": true}},
		{"GPU_GFX_VOLTAGE", map[string]bool{"gpu_gfx_voltage": true}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			ga, gpus := newHRFastClient(t, 1, 10, tc.field)
			ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500), hrAttr(volt, 800)))
			got := map[string]bool{}
			for _, f := range hrStoreFields(ga) {
				got[f] = true
			}
			assert.DeepEqual(t, got, tc.want)
		})
	}
	for _, tc := range []struct {
		name    string
		fields  string
		present bool
	}{
		{"UBB excluded", `["GPU_PACKAGE_POWER"]`, false},
		{"UBB included", `["GPU_PACKAGE_POWER","GPU_UBB_POWER"]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tcase := setupTest(t)
			defer tcase(t)

			hc, gpuBytes := newHighResStoreClient(t,
				`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Fields":`+tc.fields+`}}`)
			deltas := hc.applyWatchSnapshot(hrSnapshot(gpuBytes,
				hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, 1500),
				hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100)))
			fnames := hrDeltaFieldSet(deltas)
			assert.Equal(t, fnames["gpu_ubb_power"], tc.present)
			assert.Assert(t, fnames["gpu_package_power"])
			prefix := hc.gpuHandler.mh.GetPrefix()
			assert.Equal(t, hrStoreMetricNames(t, hc)[prefix+"gpu_ubb_power"], tc.present)
		})
	}
}

// TestHighResFrameMalformed: unknown GPU bytes, GPU bytes of length != 16, an
// attr id >= 256, a GPU entry with no Attr, an empty frame, a nil Value and an
// attr outside the power family write nothing on a partial frame; on a full
// frame an absent or nil UBB attr writes no UBB sample and does not panic.
func TestHighResFrameMalformed(t *testing.T) {
	t.Run("partial frame", func(t *testing.T) {
		ga, gpus := newHRFastClient(t, 1, 10)
		unknown := make([]byte, 16)
		ga.applyPowerFrame(&amdgpu.GPUWatch{})
		ga.applyPowerFrame(hrPartial(nil))
		ga.applyPowerFrame(hrPartial([][]byte{unknown}, hrAttr(hrAttrGPUPowerUsage, 1)))
		ga.applyPowerFrame(hrPartial([][]byte{gpus[0][:8]}, hrAttr(hrAttrGPUPowerUsage, 1)))
		ga.applyPowerFrame(hrPartial(gpus, hrAttr(amdgpu.GPUWatchAttrId(300), 1),
			&amdgpu.GPUWatchAttr{Id: amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE}))
		ga.applyPowerFrame(hrPartial(gpus))
		ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrPowerUsage, 1),
			hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 1)))
		assert.Equal(t, len(hrStoreFields(ga)), 0)
	})
	t.Run("full frame UBB", func(t *testing.T) {
		tcase := setupTest(t)
		defer tcase(t)

		gc, gpuBytes := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
		pkg := hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100)
		nilVal := &amdgpu.GPUWatchAttr{Id: amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER}
		for _, attrs := range [][]*amdgpu.GPUWatchAttr{{pkg}, {nilVal, pkg}} {
			deltas := gc.applyWatchSnapshot(hrSnapshot(gpuBytes, attrs...))
			assert.DeepEqual(t, hrSampleNames(deltas), []string{"gpu_package_power"})
		}
	})
}

// TestHighResPartialFrameUpdatesOnlyPresent: a partial frame writes only the
// attrs it carries, in place: other fields, the field count and the store
// epoch are unchanged once the cells are registered.
func TestHighResPartialFrameUpdatesOnlyPresent(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 10)
	st := ga.gpuHandler.mh.GetStore()
	ga.applyWatchSnapshot(hrFull(gpus, hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 50)))
	ga.applyPowerFrame(hrPartial(gpus,
		hrAttr(hrAttrGPUPowerUsage, 200),
		hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ENERGY_CONSUMED, 1000)))
	epoch := st.Epoch()
	key := hrGPU(ga, gpus[0]).deviceKey
	n := st.Snapshot(key).Len()

	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 300)))
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 300.0)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_energy_consumed"), 1000.0)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_junction_temperature"), 50.0)
	_, volt := hrGet(ga, gpus[0], "gpu_gfx_voltage")
	assert.Assert(t, !volt, "absent attr must not be written")
	assert.Equal(t, st.Snapshot(key).Len(), n)
	assert.Equal(t, st.Epoch(), epoch)
	// the store timestamp advances with the frames
	s1, _ := hrGet(ga, gpus[0], "gpu_power_usage")
	time.Sleep(2 * time.Millisecond)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 301)))
	s2, _ := hrGet(ga, gpus[0], "gpu_power_usage")
	assert.Assert(t, s2.TsUs > s1.TsUs)
}

// TestHighResHotCacheInvalidation: a store epoch bump, a workload-label swap
// and a field-config reload rebuild the cache; cells are re-registered when
// the epoch or the labels changed, kept otherwise; values continue.
func TestHighResHotCacheInvalidation(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 10)
	st := ga.gpuHandler.mh.GetStore()
	pw := hrAttrGPUPowerUsage
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(pw, 100)))
	c0 := ga.hrHot
	cell0 := hrGPU(ga, gpus[0]).fields[pw].cell

	// steady state: no rebuild
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(pw, 101)))
	assert.Assert(t, ga.hrHot == c0)

	// unrelated epoch bump (cgroup agent ReplaceOwned): rebuild, re-register
	st.ReplaceOwned("cgroup:", nil)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(pw, 102)))
	assert.Assert(t, ga.hrHot != c0)
	cell1 := hrGPU(ga, gpus[0]).fields[pw].cell
	assert.Assert(t, cell1 != nil && cell1 != cell0)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 102.0)

	// stream failure drops the fields (epoch bump): re-registered, value back
	st.DropFields(highResWatchFieldSet)
	_, ok := hrGet(ga, gpus[0], "gpu_power_usage")
	assert.Assert(t, !ok)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(pw, 103)))
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 103.0)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_package_power"), 103.0)

	// workload-label swap with equal labels: rebuild, cells kept
	u := utils.UUIDToString(gpus[0])
	cache := newHRWorkloadLabelCache(ga)
	same := map[string]prometheus.Labels{u: ga.hrLabelsByUUID[u]}
	cache.labels.Store(&same)
	ga.hrWLCache = cache
	cell2 := hrGPU(ga, gpus[0]).fields[pw].cell
	c2 := ga.hrHot
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(pw, 104)))
	assert.Assert(t, ga.hrHot != c2)
	assert.Assert(t, hrGPU(ga, gpus[0]).fields[pw].cell == cell2)

	// workload-label swap with a new pod: same slot, new labels on /metrics
	changed := map[string]prometheus.Labels{u: {}}
	for k, v := range ga.hrLabelsByUUID[u] {
		changed[u][k] = v
	}
	changed[u]["pod"] = "podB"
	cache.labels.Store(&changed)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(pw, 105)))
	assert.Assert(t, hrGPU(ga, gpus[0]).fields[pw].cell != cell2)
	s, ok := hrGet(ga, gpus[0], "gpu_power_usage")
	assert.Assert(t, ok)
	assert.Equal(t, s.Value, 105.0)
	assert.Equal(t, s.Labels["pod"], "podB")
	assert.Equal(t, len(st.Snapshots()), 1, "workload labels must not split the device slot")

	// field-config reload drops GPU_POWER_USAGE from the allowlist
	ga.initFieldConfig(&exportermetrics.GPUMetricConfig{Fields: []string{"GPU_PACKAGE_POWER"}})
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(pw, 106)))
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 105.0)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_package_power"), 106.0)
}

// TestHighResFeedRestartResetsHotCache: a new session drops the fast-path
// cache, so the gpu_package_power state starts over.
func TestHighResFeedRestartResetsHotCache(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 10)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 100)))
	assert.Assert(t, ga.hrHot != nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	ga.runHighResFeed(ctx, done)
	<-done
	assert.Assert(t, ga.hrHot == nil)
}

// TestApplyPowerFrameNoAlloc: steady state is allocation-free.
func TestApplyPowerFrameNoAlloc(t *testing.T) {
	ga, gpus := newHRFastClient(t, 8, 1)
	w := hrPartial(gpus, hrPowerTick(100)...)
	ga.applyPowerFrame(w) // registers the cells
	allocs := testing.AllocsPerRun(1000, func() { ga.applyPowerFrame(w) })
	assert.Equal(t, allocs, 0.0)
}

// TestHighResPowerFrameNoHotPathLogging: 10 000 partial frames with 126 N/A
// log exactly one unsupported-field line (never for GPU_PACKAGE_POWER);
// 10 000 valid frames log nothing.
func TestHighResPowerFrameNoHotPathLogging(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 10)
	buf := captureExporterLog(t, logrus.DebugLevel)
	na := hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, hrNA))
	for i := 0; i < 10000; i++ {
		ga.applyPowerFrame(na)
	}
	assert.Equal(t, strings.Count(buf.String(), "doesn't support field"), 1, buf.String())
	assert.Assert(t, strings.Contains(buf.String(), "GPU_POWER_USAGE"))
	assert.Assert(t, !strings.Contains(buf.String(), hrPackagePowerField))

	ga2, gpus2 := newHRFastClient(t, 8, 1)
	ok := hrPartial(gpus2, hrPowerTick(5)...)
	ga2.applyPowerFrame(ok)
	n := len(buf.String())
	for i := 0; i < 10000; i++ {
		ga2.applyPowerFrame(ok)
	}
	assert.Equal(t, len(buf.String()), n, buf.String()[n:])
}

// hrNA is gpuagent's N/A sentinel for a uint64 attribute.
const hrNA = math.MaxUint64

var hrLoggerOnce sync.Once

// newHRFastClient builds a minimal high-res GPU client (metrics handler with a
// store, field allowlist, fieldLogger, label caches for n synthetic GPUs) so
// applyPowerFrame/applyWatchSnapshot can be driven directly without gRPC
// mocks. fastMs is the accepted watch period.
func newHRFastClient(tb testing.TB, n int, fastMs uint32, fields ...string) (*GPUAgentGPUClient, [][]byte) {
	tb.Helper()
	hrLoggerOnce.Do(func() { logger.Init(true) })
	cfgPath := filepath.Join(tb.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"CommonConfig":{"HighResolution":{"Enable":true}}}`), 0644); err != nil {
		tb.Fatal(err)
	}
	cfg := config.NewConfigHandler(cfgPath, config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort})
	mh, err := metricsutil.NewMetrics(cfg)
	if err != nil {
		tb.Fatal(err)
	}
	ga := &GPUAgentGPUClient{
		gpuHandler:     &GPUAgentClient{mh: mh},
		fl:             NewFieldLogger(),
		hrLabelsByUUID: map[string]prometheus.Labels{},
		hrGPUIDByUUID:  map[string]string{},
	}
	ga.initFieldConfig(&exportermetrics.GPUMetricConfig{Fields: fields})
	ga.hrFastMs.Store(fastMs)
	gpus := make([][]byte, n)
	for i := range gpus {
		b := make([]byte, 16)
		for j := range b {
			b[j] = byte(16*i + j + 1)
		}
		gpus[i] = b
		u := utils.UUIDToString(b)
		ga.hrLabelsByUUID[u] = prometheus.Labels{
			"gpu_id":           strconv.Itoa(i),
			"gpu_uuid":         u,
			"serial_number":    "SN" + strconv.Itoa(i),
			"card_model":       "MI300X",
			"hostname":         "node0",
			"gpu_partition_id": "0",
		}
		ga.hrGPUIDByUUID[u] = strconv.Itoa(i)
	}
	return ga, gpus
}

func hrAttr(id amdgpu.GPUWatchAttrId, v uint64) *amdgpu.GPUWatchAttr {
	return hrLongAttr(id, v)
}

// hrPartial builds a partial (power) frame: Spec carries only Id, and every
// listed GPU carries attrs.
func hrPartial(gpus [][]byte, attrs ...*amdgpu.GPUWatchAttr) *amdgpu.GPUWatch {
	id := highResWatchID
	w := &amdgpu.GPUWatch{Spec: &amdgpu.GPUWatchSpec{Id: id[:]}, Stats: &amdgpu.GPUWatchStats{}}
	for _, g := range gpus {
		w.Stats.GPUWatchAttr = append(w.Stats.GPUWatchAttr, &amdgpu.GPUWatchAttrs{GPU: g, Attr: attrs})
	}
	return w
}

// hrFull builds a full frame: Spec carries the attribute list.
func hrFull(gpus [][]byte, attrs ...*amdgpu.GPUWatchAttr) *amdgpu.GPUWatch {
	w := hrPartial(gpus, attrs...)
	w.Spec.Attribute = highResWatchAttrs
	return w
}

// hrPowerTick is one power tick's payload as gpuagent sends it.
func hrPowerTick(v uint64) []*amdgpu.GPUWatchAttr {
	attrs := make([]*amdgpu.GPUWatchAttr, 0, len(hrPowerAttrIDs))
	for _, id := range hrPowerAttrIDs {
		attrs = append(attrs, hrAttr(id, v))
	}
	return attrs
}

// hrGet returns the stored value of field for gpu.
func hrGet(ga *GPUAgentGPUClient, gpu []byte, field string) (snapshotstore.Sample, bool) {
	u := utils.UUIDToString(gpu)
	for _, snap := range ga.gpuHandler.mh.GetStore().Snapshots() {
		if s, ok := snap.Get(field); ok && s.Labels["gpu_uuid"] == u {
			return s, true
		}
	}
	return snapshotstore.Sample{}, false
}

func hrMustGet(t *testing.T, ga *GPUAgentGPUClient, gpu []byte, field string) float64 {
	t.Helper()
	s, ok := hrGet(ga, gpu, field)
	assert.Assert(t, ok, "%s missing from store", field)
	return s.Value
}

func hrGPU(ga *GPUAgentGPUClient, gpu []byte) *hrHotGPU {
	return ga.hrHot.byGPU[[16]byte(gpu)]
}

func hrStoreFields(ga *GPUAgentGPUClient) []string {
	var out []string
	for _, snap := range ga.gpuHandler.mh.GetStore().Snapshots() {
		snap.Range(func(f string, _ snapshotstore.Sample) bool {
			out = append(out, f)
			return true
		})
	}
	return out
}
