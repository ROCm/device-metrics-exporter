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
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

// GPUOP-1126: gpu_ubb_power on the pull (GPUGet) and high-res (GPUWatch) paths.

const ubbField = "GPU_UBB_POWER"

// newUbbPullClient builds a pull-mode (non high-res) GPU client from a config
// body, so the GPUGet path writes the registry gauges directly.
func newUbbPullClient(t *testing.T, body string) *GPUAgentGPUClient {
	t.Helper()
	mh := newHighResMetricsHandler(t, body)
	ga := NewAgent(mh, WithK8sClient(nil), WithK8sSchedulerClient(nil), WithGPUMonitoring(true))
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
	return gpuclient
}

func ubbGPU(index, partitionID uint32, ubb, pkg uint64) *amdgpu.GPU {
	id := make([]byte, 16)
	for i := range id {
		id[i] = byte(0x40 + int(index)*16 + i)
	}
	return &amdgpu.GPU{
		Spec: &amdgpu.GPUSpec{Id: id},
		Status: &amdgpu.GPUStatus{
			Index:       index,
			PartitionId: partitionID,
			SerialNum:   "ubb-serial",
			PCIeStatus:  &amdgpu.GPUPCIeStatus{PCIeBusId: "0000:01:00.0"},
		},
		Stats: &amdgpu.GPUStats{UbbPower: ubb, PackagePower: pkg},
	}
}

// pullScrape runs one GPUGet-path update for the given entries.
func pullScrape(gc *GPUAgentGPUClient, gpus ...*amdgpu.GPU) {
	for _, gpu := range gpus {
		gc.updateGPUInfoToMetrics(nil, gpu, nil, nil, nil)
	}
}

// gaugeSeries collects every series of a GaugeVec.
func gaugeSeries(t *testing.T, gv *prometheus.GaugeVec) []*dto.Metric {
	t.Helper()
	ch := make(chan prometheus.Metric, 64)
	go func() {
		gv.Collect(ch)
		close(ch)
	}()
	var out []*dto.Metric
	for m := range ch {
		d := &dto.Metric{}
		assert.NilError(t, m.Write(d))
		out = append(out, d)
	}
	return out
}

func dtoLabel(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}

// pullServedNames mirrors the pull-path gauges into the snapshot store (the
// per-scrape publish step) and returns the prefixed names /metrics serves.
func pullServedNames(t *testing.T, gc *GPUAgentGPUClient) map[string]bool {
	t.Helper()
	assert.NilError(t, gc.publishGPUMetricsToStore(gc.gpuHandler.mh.GetStore()))
	return hrStoreMetricNames(t, gc)
}

// TC1: pull, non-partitioned entry exports the value with the GPU labels.
func TestUbbPowerPullExported(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc := newUbbPullClient(t, `{"GPUConfig":{"Labels":["GPU_UUID"]}}`)
	gpu := ubbGPU(0, 0, 1500, 100)
	pullScrape(gc, gpu)

	series := gaugeSeries(t, &gc.metrics.gpuUbbPower)
	assert.Equal(t, len(series), 1)
	assert.Equal(t, series[0].GetGauge().GetValue(), float64(1500))
	assert.Equal(t, dtoLabel(series[0], "gpu_uuid"), utils.UUIDToString(gpu.Spec.Id))
	assert.Equal(t, dtoLabel(series[0], "gpu_id"), "0")
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), false)

	prefix := gc.gpuHandler.mh.GetPrefix()
	assert.Assert(t, pullServedNames(t, gc)[prefix+"gpu_ubb_power"], "gpu_ubb_power not served")
}

// TC2: partition 0 carries the value, children carry N/A; only partition 0
// exports, with no exporter-side partition check (D1).
func TestUbbPowerPullPartition0Only(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc := newUbbPullClient(t, `{}`)
	pullScrape(gc,
		ubbGPU(0, 0, 1500, 100),
		ubbGPU(1, 1, math.MaxUint64, 100),
		ubbGPU(2, 2, math.MaxUint64, 100),
	)

	series := gaugeSeries(t, &gc.metrics.gpuUbbPower)
	assert.Equal(t, len(series), 1)
	assert.Equal(t, dtoLabel(series[0], "gpu_partition_id"), "0")
	assert.Equal(t, series[0].GetGauge().GetValue(), float64(1500))
	// package power is unaffected on every partition
	assert.Equal(t, len(gaugeSeries(t, &gc.metrics.gpuPackagePower)), 3)
}

// TC10: the N/A sentinel is never exported (not as 0 either) and is recorded
// as unsupported once.
func TestUbbPowerPullNA(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc := newUbbPullClient(t, `{}`)
	gpu := ubbGPU(0, 0, math.MaxUint64, 100)
	pullScrape(gc, gpu)
	gc.fl.SetFilterDone()
	pullScrape(gc, gpu)

	assert.Equal(t, len(gaugeSeries(t, &gc.metrics.gpuUbbPower)), 0)
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), true)
	assert.Equal(t, len(gaugeSeries(t, &gc.metrics.gpuPackagePower)), 1)
}

// TC11 / TC24 / TC30: 0 in the filter pass marks the field unsupported for
// that gpuid; it stays absent after the value turns non-zero, exactly like
// GPU_PACKAGE_POWER, and the two fields gate independently.
func TestUbbPowerPullZeroFilterPass(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc := newUbbPullClient(t, `{}`)
	// filter pass: UBB 0, package power valid
	pullScrape(gc, ubbGPU(0, 0, 0, 100))
	gc.fl.SetFilterDone()
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), true)
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", "GPU_PACKAGE_POWER"), false)

	// later scrape: UBB now non-zero but still suppressed
	pullScrape(gc, ubbGPU(0, 0, 1500, 120))
	assert.Equal(t, len(gaugeSeries(t, &gc.metrics.gpuUbbPower)), 0)
	pkg := gaugeSeries(t, &gc.metrics.gpuPackagePower)
	assert.Equal(t, len(pkg), 1)
	assert.Equal(t, pkg[0].GetGauge().GetValue(), float64(120))
}

// TC24 / TC30: the reverse - zero package power at the filter pass does not
// affect gpu_ubb_power, and both fields share the same startup-zero rule.
func TestUbbPowerCoexistPackagePower(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc := newUbbPullClient(t, `{}`)
	pullScrape(gc, ubbGPU(0, 0, 1500, 0))
	gc.fl.SetFilterDone()
	pullScrape(gc, ubbGPU(0, 0, 1600, 100))

	assert.Equal(t, gc.fl.checkUnsupportedFields("0", "GPU_PACKAGE_POWER"), true)
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), false)
	assert.Equal(t, len(gaugeSeries(t, &gc.metrics.gpuPackagePower)), 0)
	ubb := gaugeSeries(t, &gc.metrics.gpuUbbPower)
	assert.Equal(t, len(ubb), 1)
	assert.Equal(t, ubb[0].GetGauge().GetValue(), float64(1600))
}

// TC25: a config re-init (InitConfigs, the auto-reload path) clears the
// unsupported state so a now-valid value is exported again.
func TestUbbPowerReloadReenable(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc := newUbbPullClient(t, `{}`)
	pullScrape(gc, ubbGPU(0, 0, 0, 100))
	gc.fl.SetFilterDone()
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), true)

	assert.NilError(t, gc.InitConfigs())
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), false)
	pullScrape(gc, ubbGPU(0, 0, 1500, 100))
	ubb := gaugeSeries(t, &gc.metrics.gpuUbbPower)
	assert.Equal(t, len(ubb), 1)
	assert.Equal(t, ubb[0].GetGauge().GetValue(), float64(1500))
}

// TC20 / TC21 / TC22: boundary values on both paths.
func TestUbbPowerValueBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  uint64
	}{
		{"MinValue", 1},
		{"LargeValueAboveUint16", 70000},
		{"LargeValueAboveUint32", 4294967297},
		{"SentinelAdjacent", math.MaxUint64 - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tcase := setupTest(t)
			defer tcase(t)

			gc := newUbbPullClient(t, `{}`)
			pullScrape(gc, ubbGPU(0, 0, tc.val, 100))
			series := gaugeSeries(t, &gc.metrics.gpuUbbPower)
			assert.Equal(t, len(series), 1)
			assert.Equal(t, series[0].GetGauge().GetValue(), float64(tc.val))

			hc, gpuBytes := newHighResStoreClient(t,
				`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)
			deltas := hc.applyWatchSnapshot(hrSnapshot(gpuBytes,
				hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, tc.val)))
			s := hrFindSample(deltas, "gpu_ubb_power", "", "")
			assert.Assert(t, s != nil)
			assert.Equal(t, s.Value, float64(tc.val))
		})
	}
}

// TC4 / TC5: one shared series name; the watch table subscribes attr 140
// as a scalar and the store's watch field set includes it.
func TestUbbPowerNameParity(t *testing.T) {
	name := strings.ToLower(exportermetrics.GPUMetricField_GPU_UBB_POWER.String())
	assert.Equal(t, name, "gpu_ubb_power")
	assert.Equal(t, watchAttrMetricName(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER), name)
	assert.Equal(t, int32(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER), int32(140))
	assert.Equal(t, int32(exportermetrics.GPUMetricField_GPU_UBB_POWER), int32(142))

	found := false
	for _, id := range highResWatchAttrs {
		if id == amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER {
			found = true
		}
	}
	assert.Assert(t, found, "attr 140 not subscribed")
	_, inStore := highResWatchFieldSet[name]
	assert.Assert(t, inStore, "gpu_ubb_power missing from highResWatchFieldSet")
	label, hasSub := watchAttrIndexLabel(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER)
	assert.Equal(t, label, "")
	assert.Equal(t, hasSub, false)

	// pull gauge registered under the same base name
	desc := make(chan *prometheus.Desc, 1)
	gc := &GpuMetrics{}
	gc.gpuUbbPower = *prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name}, []string{"x"})
	gc.gpuUbbPower.Describe(desc)
	assert.Assert(t, strings.Contains((<-desc).String(), `"`+name+`"`))
}

// TC4 (pull side): the gauge built by initPrometheusMetrics is named
// gpu_ubb_power and is the FieldMeta for GPU_UBB_POWER.
func TestUbbPowerPullGaugeName(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc := newUbbPullClient(t, `{}`)
	ch := make(chan *prometheus.Desc, 1)
	gc.metrics.gpuUbbPower.Describe(ch)
	assert.Assert(t, strings.Contains((<-ch).String(), `fqName: "gpu_ubb_power"`))
	gc.exportFieldMapMu.Lock()
	_, ok := gc.fieldMetricsMap[ubbField]
	gc.exportFieldMapMu.Unlock()
	assert.Assert(t, ok, "GPU_UBB_POWER missing from fieldMetricsMap")
}

// TC3: high-res writes gpu_ubb_power to the store with the GPU labels.
func TestUbbPowerHighResSample(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)
	deltas := gc.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, 1500)))
	s := hrFindSample(deltas, "gpu_ubb_power", "", "")
	assert.Assert(t, s != nil)
	assert.Equal(t, s.Value, float64(1500))
	assert.Equal(t, s.Labels["gpu_uuid"], utils.UUIDToString(gpuBytes))

	prefix := gc.gpuHandler.mh.GetPrefix()
	assert.Assert(t, hrStoreMetricNames(t, gc)[prefix+"gpu_ubb_power"], "gpu_ubb_power not served from store")
}

// TC12: an N/A LongVal writes no sample and marks the field unsupported.
func TestUbbPowerHighResNA(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc, gpuBytes := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
	deltas := gc.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, math.MaxUint64),
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100)))
	assert.Assert(t, hrFindSample(deltas, "gpu_ubb_power", "", "") == nil)
	assert.DeepEqual(t, hrSampleNames(deltas), []string{"gpu_package_power"})
	assert.Equal(t, gc.fl.checkUnsupportedFields(hrGPUID, ubbField), true)
}

// TC13: partition 0 publishes, the child partition (N/A) does not.
func TestUbbPowerHighResChildNA(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc, gpu0 := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
	child := ubbGPU(1, 1, 0, 0)
	childUUID := utils.UUIDToString(child.Spec.Id)
	gc.hrLabelsByUUID[childUUID] = prometheus.Labels(gc.populateLabelsFromGPU(nil, child, nil))
	gc.hrGPUIDByUUID[childUUID] = "1"

	snap := &amdgpu.GPUWatch{Stats: &amdgpu.GPUWatchStats{GPUWatchAttr: []*amdgpu.GPUWatchAttrs{
		{GPU: gpu0, Attr: []*amdgpu.GPUWatchAttr{
			hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, 1500)}},
		{GPU: child.Spec.Id, Attr: []*amdgpu.GPUWatchAttr{
			hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, math.MaxUint64)}},
	}}}
	deltas := gc.applyWatchSnapshot(snap)
	n := 0
	for _, d := range hrFlatten(deltas) {
		if d.field == "gpu_ubb_power" {
			n++
			assert.Equal(t, d.sample.Value, float64(1500))
			assert.Equal(t, d.sample.Labels["gpu_partition_id"], "0")
		}
	}
	assert.Equal(t, n, 1)
}

// TC15 (pinned to design D2/OQ2): the high-res path has no zero gate, so a
// raw 0 publishes. The producer contract (GPUOP-1108) never sends 0.
func TestUbbPowerHighResZero(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc, gpuBytes := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
	deltas := gc.applyWatchSnapshot(hrSnapshot(gpuBytes,
		hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, 0)))
	s := hrFindSample(deltas, "gpu_ubb_power", "", "")
	assert.Assert(t, s != nil)
	assert.Equal(t, s.Value, float64(0))
}

// TC16: attr absent or nil value - no panic, no UBB sample, other attrs
// still publish.
func TestUbbPowerHighResNilAttr(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc, gpuBytes := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
	pkg := hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100)

	deltas := gc.applyWatchSnapshot(hrSnapshot(gpuBytes, pkg))
	assert.DeepEqual(t, hrSampleNames(deltas), []string{"gpu_package_power"})

	nilVal := &amdgpu.GPUWatchAttr{Id: amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER}
	deltas = gc.applyWatchSnapshot(hrSnapshot(gpuBytes, nilVal, pkg))
	assert.DeepEqual(t, hrSampleNames(deltas), []string{"gpu_package_power"})
}

// TC23: valid -> N/A -> valid. Pinned to existing high-res behaviour: the N/A
// tick marks the field unsupported for that GPU (no filter pass ends in
// high-res mode), so later valid values stay suppressed until a config reload,
// exactly as for gpu_package_power. Never a 0 sample.
func TestUbbPowerHighResTransitions(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc, gpuBytes := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
	ubb := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER
	pkg := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE
	tick := func(v uint64) map[string]bool {
		return hrDeltaFieldSet(gc.applyWatchSnapshot(hrSnapshot(gpuBytes, hrLongAttr(ubb, v), hrLongAttr(pkg, v))))
	}

	d := gc.applyWatchSnapshot(hrSnapshot(gpuBytes, hrLongAttr(ubb, 1500), hrLongAttr(pkg, 1500)))
	s := hrFindSample(d, "gpu_ubb_power", "", "")
	assert.Assert(t, s != nil)
	assert.Equal(t, s.Value, float64(1500))

	f := tick(math.MaxUint64)
	assert.Equal(t, f["gpu_ubb_power"], false)
	assert.Equal(t, f["gpu_package_power"], false)
	assert.Equal(t, gc.fl.checkUnsupportedFields(hrGPUID, ubbField), true)

	f = tick(1600)
	assert.Equal(t, f["gpu_ubb_power"], f["gpu_package_power"], "UBB must match package power stale handling")
	assert.Equal(t, f["gpu_ubb_power"], false)
}

// TC14: the Fields allowlist toggles the field on both paths.
func TestUbbPowerAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fields  string
		present bool
	}{
		{"excluded", `["GPU_PACKAGE_POWER"]`, false},
		{"included", `["GPU_PACKAGE_POWER","GPU_UBB_POWER"]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tcase := setupTest(t)
			defer tcase(t)

			pc := newUbbPullClient(t, `{"GPUConfig":{"Fields":`+tc.fields+`}}`)
			pullScrape(pc, ubbGPU(0, 0, 1500, 100))
			prefix := pc.gpuHandler.mh.GetPrefix()
			names := pullServedNames(t, pc)
			assert.Equal(t, names[prefix+"gpu_ubb_power"], tc.present)
			assert.Assert(t, names[prefix+"gpu_package_power"])

			hc, gpuBytes := newHighResStoreClient(t,
				`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Fields":`+tc.fields+`}}`)
			deltas := hc.applyWatchSnapshot(hrSnapshot(gpuBytes,
				hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, 1500),
				hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE, 100)))
			fnames := hrDeltaFieldSet(deltas)
			assert.Equal(t, fnames["gpu_ubb_power"], tc.present)
			assert.Assert(t, fnames["gpu_package_power"])
			assert.Equal(t, hrStoreMetricNames(t, hc)[prefix+"gpu_ubb_power"], tc.present)
		})
	}
}

// TC27: concurrent /metrics gathers while the stream writer publishes UBB.
// Run with -race.
func TestUbbPowerRaceHighRes(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gc, gpuBytes := newHighResStoreClient(t, `{"CommonConfig":{"HighResolution":{"Enable":true}}}`)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			gc.applyWatchSnapshot(hrSnapshot(gpuBytes,
				hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, uint64(1000+i))))
		}
	}()
	for i := 0; i < 200; i++ {
		_, err := gc.gpuHandler.mh.GetRegistry().Gather()
		assert.NilError(t, err)
	}
	wg.Wait()
	prefix := gc.gpuHandler.mh.GetPrefix()
	assert.Assert(t, hrStoreMetricNames(t, gc)[prefix+"gpu_ubb_power"])
}
