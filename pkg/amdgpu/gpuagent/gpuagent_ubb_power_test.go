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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

// gpu_ubb_power on the pull (GPUGet) and high-res (GPUWatch) paths.

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

// TestUbbPowerPullExported: a non-partitioned entry exports the value (also
// above uint32) with the GPU labels, served as gpu_ubb_power.
func TestUbbPowerPullExported(t *testing.T) {
	for _, val := range []uint64{1500, 4294967297} {
		t.Run(fmt.Sprint(val), func(t *testing.T) {
			tcase := setupTest(t)
			defer tcase(t)

			gc := newUbbPullClient(t, `{"GPUConfig":{"Labels":["GPU_UUID"]}}`)
			gpu := ubbGPU(0, 0, val, 100)
			pullScrape(gc, gpu)

			series := gaugeSeries(t, &gc.metrics.gpuUbbPower)
			assert.Equal(t, len(series), 1)
			assert.Equal(t, series[0].GetGauge().GetValue(), float64(val))
			assert.Equal(t, dtoLabel(series[0], "gpu_uuid"), utils.UUIDToString(gpu.Spec.Id))
			assert.Equal(t, dtoLabel(series[0], "gpu_id"), "0")
			assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), false)

			prefix := gc.gpuHandler.mh.GetPrefix()
			assert.Assert(t, pullServedNames(t, gc)[prefix+"gpu_ubb_power"], "gpu_ubb_power not served")
		})
	}
}

// TestUbbPowerPullPartition0Only: partition 0 carries the value, children carry N/A; only partition 0
// exports, with no exporter-side partition check.
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

// TestUbbPowerPullNA: the N/A sentinel is never exported (not as 0 either) and is recorded
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

// TestUbbPowerPullZeroFilterPass: 0 in the filter pass marks the field
// unsupported for that gpuid; it stays absent after the value turns non-zero,
// exactly like GPU_PACKAGE_POWER, and the two fields gate independently in
// both directions. A config re-init (InitConfigs) clears the state.
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

	// the reverse: zero package power at the filter pass does not gate UBB
	rc := newUbbPullClient(t, `{}`)
	pullScrape(rc, ubbGPU(0, 0, 1500, 0))
	rc.fl.SetFilterDone()
	pullScrape(rc, ubbGPU(0, 0, 1600, 100))
	assert.Equal(t, rc.fl.checkUnsupportedFields("0", "GPU_PACKAGE_POWER"), true)
	assert.Equal(t, len(gaugeSeries(t, &rc.metrics.gpuPackagePower)), 0)
	ubb := gaugeSeries(t, &rc.metrics.gpuUbbPower)
	assert.Equal(t, len(ubb), 1)
	assert.Equal(t, ubb[0].GetGauge().GetValue(), float64(1600))

	// a config re-init clears the unsupported state
	assert.NilError(t, gc.InitConfigs())
	assert.Equal(t, gc.fl.checkUnsupportedFields("0", ubbField), false)
	pullScrape(gc, ubbGPU(0, 0, 1500, 100))
	ubb = gaugeSeries(t, &gc.metrics.gpuUbbPower)
	assert.Equal(t, len(ubb), 1)
	assert.Equal(t, ubb[0].GetGauge().GetValue(), float64(1500))
}

// TestUbbPowerHighResSample: high-res subscribes attr 140 as a scalar and
// writes gpu_ubb_power (also above uint32) to the store with the GPU labels;
// the proto ids are pinned (attr 140, field 142).
func TestUbbPowerHighResSample(t *testing.T) {
	assert.Equal(t, int32(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER), int32(140))
	assert.Equal(t, int32(exportermetrics.GPUMetricField_GPU_UBB_POWER), int32(142))
	label, hasSub := watchAttrIndexLabel(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER)
	assert.Assert(t, label == "" && !hasSub, "attr 140 must be a scalar")
	for _, val := range []uint64{1500, 4294967297} {
		t.Run(fmt.Sprint(val), func(t *testing.T) {
			tcase := setupTest(t)
			defer tcase(t)

			gc, gpuBytes := newHighResStoreClient(t,
				`{"CommonConfig":{"HighResolution":{"Enable":true}},"GPUConfig":{"Labels":["GPU_UUID"]}}`)
			deltas := gc.applyWatchSnapshot(hrSnapshot(gpuBytes,
				hrLongAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_UBB_POWER, val)))
			s := hrFindSample(deltas, "gpu_ubb_power", "", "")
			assert.Assert(t, s != nil)
			assert.Equal(t, s.Value, float64(val))
			assert.Equal(t, s.Labels["gpu_uuid"], utils.UUIDToString(gpuBytes))

			prefix := gc.gpuHandler.mh.GetPrefix()
			assert.Assert(t, hrStoreMetricNames(t, gc)[prefix+"gpu_ubb_power"], "gpu_ubb_power not served from store")
		})
	}
}

// TestUbbPowerHighResChildNA: partition 0 publishes, the child partition (N/A) does not.
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

// TestUbbPowerHighResZero pins a design decision: the high-res path has no
// zero gate, so a raw 0 publishes. The gpuagent producer never sends 0.
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

// TestUbbPowerAllowlist: the Fields allowlist toggles the field on the pull
// path (the high-res rows are in TestHighResAllowlist).
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
		})
	}
}
