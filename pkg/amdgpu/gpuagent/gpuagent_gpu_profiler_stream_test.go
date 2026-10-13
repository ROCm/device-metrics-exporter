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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

// profilerSample builds a one-GPU decoded profiler sample for the given render id
// and alias→value pairs (value as the rocpctl string double).
func profilerSample(renderID string, aliasVals map[string]string) *amdgpu.GpuProfiler {
	metrics := make([]*amdgpu.MetricValue, 0, len(aliasVals))
	for field, val := range aliasVals {
		metrics = append(metrics, &amdgpu.MetricValue{Field: field, Value: val})
	}
	return &amdgpu.GpuProfiler{
		GpuMetrics: []*amdgpu.GpuMetric{{DrmRenderId: renderID, Metrics: metrics}},
	}
}

// seedProfilerLabelCache builds a renderID→labels cache for the synthetic GPU.
func seedProfilerLabelCache(ga *GPUAgentGPUClient, renderID string, gpuBytes []byte) (*profilerLabelCache, prometheus.Labels) {
	gpu := &amdgpu.GPU{Spec: &amdgpu.GPUSpec{Id: gpuBytes}, Status: &amdgpu.GPUStatus{Index: 0}}
	labels := prometheus.Labels(ga.populateLabelsFromGPU(nil, gpu, nil))
	lc := newProfilerLabelCache(ga)
	m := map[string]prometheus.Labels{renderID: labels}
	lc.labels.Store(&m)
	return lc, labels
}

// profilerStoreEnableBody is a high-res config with the profiler enabled so the
// stream writer's fields pass the config field-gate (matching the pull surface).
const profilerStoreEnableBody = `{"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":1}},` +
	`"GPUConfig":{"Labels":["GPU_UUID"],"ProfilerMetrics":{"all":true}}}`

// TestProfilerStreamSinkStoreKeyParity (gate a) proves the stream writer maps a
// decoded GpuProfiler to the exact gpu_prof_* store keys the pull path writes for
// the same aliases, applying the same NormalizePercent clamp and the derived
// occupancy-elapsed. Every produced key is a real profiler gauge field, so a stream
// series is byte-identical to the per-scrape one.
func TestProfilerStreamSinkStoreKeyParity(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, gpuBytes := newHighResStoreClient(t, profilerStoreEnableBody)
	lc, labels := seedProfilerLabelCache(gpuclient, "99", gpuBytes)

	gpuclient.publishProfilerSample(profilerSample("99", map[string]string{
		"GPU_UTIL":                 "50",
		"SQ_WAVES":                 "1234",
		"GRBM_GUI_ACTIVE":          "10",
		"MeanOccupancyPerActiveCU": "40",
		"NOT_A_PROFILER_ALIAS":     "7", // unknown alias: dropped, not a store key
	}), lc)

	snap := gpuclient.gpuHandler.mh.GetStore().Snapshot(gpuStoreDeviceKey(labels, gpuclient.workloadLabelKeys()))
	assert.Assert(t, snap != nil, "profiler sample produced no store slot")
	got := map[string]float64{}
	snap.Range(func(field string, s snapshotstore.Sample) bool {
		got[field] = s.Value
		return true
	})

	want := map[string]float64{
		"gpu_prof_gui_util_percent":        50, // NormalizePercent(50)
		"gpu_prof_sq_waves":                1234,
		"gpu_prof_grbm_gui_active":         10,
		"gpu_prof_occupancy_per_active_cu": 40,
		"gpu_prof_occupancy_elapsed":       4, // 40 / 10 (derived)
	}
	assert.DeepEqual(t, got, want)

	// Key parity: every store key the stream writes is a registered profiler gauge
	// field (the pull path's store key for the same metric is that gauge name).
	for field := range got {
		_, ok := gpuclient.fieldMetricsMap[strings.ToUpper(field)]
		assert.Assert(t, ok, "stream store key %q is not a pull-path profiler field", field)
	}
}

// TestProfilerStreamFieldGateMasks proves a config Fields allowlist that excludes a
// profiler field masks it on the stream write surface too, matching the pull path.
func TestProfilerStreamFieldGateMasks(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	// Enable only GPU_PROF_SQ_WAVES; GPU_PROF_GUI_UTIL_PERCENT is thus disabled.
	gpuclient, gpuBytes := newHighResStoreClient(t,
		`{"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":1}},`+
			`"GPUConfig":{"Labels":["GPU_UUID"],"ProfilerMetrics":{"all":true},"Fields":["GPU_PROF_SQ_WAVES"]}}`)
	lc, labels := seedProfilerLabelCache(gpuclient, "99", gpuBytes)

	gpuclient.publishProfilerSample(profilerSample("99", map[string]string{
		"SQ_WAVES": "1234",
		"GPU_UTIL": "50",
	}), lc)

	snap := gpuclient.gpuHandler.mh.GetStore().Snapshot(gpuStoreDeviceKey(labels, gpuclient.workloadLabelKeys()))
	assert.Assert(t, snap != nil)
	got := map[string]bool{}
	snap.Range(func(field string, _ snapshotstore.Sample) bool {
		got[field] = true
		return true
	})
	assert.Assert(t, got["gpu_prof_sq_waves"], "enabled profiler field missing from store")
	assert.Assert(t, !got["gpu_prof_gui_util_percent"], "disabled profiler field leaked into store")
}

// TestProfilerDropFieldsClearsProfilerOnly (gate b) proves DropFields with the
// profiler field set evicts gpu_prof_* but leaves the node *_total slot, gpu_health,
// and non-profiler device fields intact — the failure/disable clearing behavior.
func TestProfilerDropFieldsClearsProfilerOnly(t *testing.T) {
	store := snapshotstore.New()
	store.Update("dev1", map[string]snapshotstore.Sample{
		"gpu_prof_sq_waves": {Value: 1},
		"gpu_package_power": {Value: 2},
		"gpu_health":        {Value: 3},
	})
	store.Update("gpu-node", map[string]snapshotstore.Sample{
		"gpu_nodes_total": {Value: 8},
	})

	store.DropFields(highResProfilerFieldSet)

	dev := store.Snapshot("dev1")
	assert.Assert(t, dev != nil)
	fields := map[string]bool{}
	dev.Range(func(f string, _ snapshotstore.Sample) bool { fields[f] = true; return true })
	assert.Assert(t, !fields["gpu_prof_sq_waves"], "profiler field not cleared")
	assert.Assert(t, fields["gpu_package_power"], "non-profiler telemetry wrongly cleared")
	assert.Assert(t, fields["gpu_health"], "gpu_health wrongly cleared")

	node := store.Snapshot("gpu-node")
	assert.Assert(t, node != nil, "node total slot wrongly dropped")
	nodeFields := map[string]bool{}
	node.Range(func(f string, _ snapshotstore.Sample) bool { nodeFields[f] = true; return true })
	assert.Assert(t, nodeFields["gpu_nodes_total"], "gpu_nodes_total wrongly cleared")
}

// TestProfilerStreamGate (gates c, d) proves the stream runs only when high-res AND
// profiler-enabled AND non-PTL, and that the interval derives from the fast period.
func TestProfilerStreamGate(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	for _, tc := range []struct {
		name         string
		body         string
		wantRun      bool
		wantInterval uint64
	}{
		{
			"high-res profiler non-PTL runs, interval from fast period",
			`{"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":2}},` +
				`"GPUConfig":{"ProfilerMetrics":{"all":true}}}`,
			true, 2000,
		},
		{
			"non-high-res does not run",
			`{"CommonConfig":{"HighResolution":{"Enable":false}},` +
				`"GPUConfig":{"ProfilerMetrics":{"all":true}}}`,
			false, 0,
		},
		{
			"profiler disabled does not run",
			`{"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":1}},` +
				`"GPUConfig":{"ProfilerMetrics":{"all":false}}}`,
			false, 0,
		},
		{
			"PTL platform does not run",
			`{"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":1}},` +
				`"GPUConfig":{"ProfilerMetrics":{"all":true},"ProfilerConfig":{"PtlDelay":5,"SamplingInterval":1000000}}}`,
			false, 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gpuclient, _ := newHighResStoreClient(t, tc.body)
			run, interval := gpuclient.profilerStreamGate()
			assert.Equal(t, run, tc.wantRun)
			assert.Equal(t, interval, tc.wantInterval)
		})
	}
}

// TestProfilerOneShotSkippedWhenStreaming (BLOCKER fix) proves getMetricsAll does
// NOT issue the one-shot rocpctl fetch while the stream owns gpu_prof_* (profStreamOn),
// and DOES issue it otherwise — so two rocpctl processes never sample the same GPU.
func TestProfilerOneShotSkippedWhenStreaming(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, _ := newHighResStoreClient(t, profilerStoreEnableBody)
	// Empty the field list so the one-shot path never execs a real rocpctl; the
	// fetch counter still records that the one-shot path was entered.
	gpuclient.rocpclient.SetFields(nil)

	// Not streaming: the per-scrape one-shot fetch is issued once.
	gpuclient.profStreamOn.Store(false)
	gpuclient.profilerOneShotFetches.Store(0)
	assert.NilError(t, gpuclient.getMetricsAll(context.Background()))
	assert.Equal(t, gpuclient.profilerOneShotFetches.Load(), int64(1),
		"one-shot profiler fetch must run when not streaming")

	// Streaming: the one-shot fetch is skipped entirely (no HW/failure contention).
	gpuclient.profStreamOn.Store(true)
	gpuclient.profilerOneShotFetches.Store(0)
	assert.NilError(t, gpuclient.getMetricsAll(context.Background()))
	assert.Equal(t, gpuclient.profilerOneShotFetches.Load(), int64(0),
		"one-shot profiler fetch must be skipped while streaming")
}

// TestProfilerStreamGateDisabledOnFailure proves the gate declines to (re)start a
// stream once the shared rocpclient is already failure-disabled.
func TestProfilerStreamGateDisabledOnFailure(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, _ := newHighResStoreClient(t, profilerStoreEnableBody)
	run, _ := gpuclient.profilerStreamGate()
	assert.Assert(t, run, "gate should be on before failure-disable")

	// Three consecutive failures cross the disable threshold.
	for i := 0; i < 3; i++ {
		gpuclient.rocpclient.IncFailureCount(context.Background())
	}
	assert.Assert(t, gpuclient.rocpclient.IsDisabledOnFailure())

	run, interval := gpuclient.profilerStreamGate()
	assert.Assert(t, !run, "gate should be off once failure-disabled")
	assert.Equal(t, interval, uint64(0))
}

// mockStreamScript writes a stand-in rocpctl that handles SIGTERM and emits one
// profiler NDJSON line periodically, so start/stop lifecycle can be exercised
// without the real binary.
func mockStreamScript(t *testing.T) string {
	t.Helper()
	body := `#!/bin/bash
trap 'exit 0' TERM INT
while true; do
  echo '{"GpuMetrics":[{"DrmRenderId":"99","Metrics":[{"Field":"SQ_WAVES","Value":"1"}]}]}'
  sleep 0.05
done
`
	path := filepath.Join(t.TempDir(), "rocpctl-mock")
	assert.NilError(t, os.WriteFile(path, []byte(body), 0o755))
	return path
}

// TestProfilerStreamStartStopLifecycle proves reconcile start sets the manager and
// the on-flag, and stop clears both and evicts profiler fields from the store.
func TestProfilerStreamStartStopLifecycle(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	gpuclient, _ := newHighResStoreClient(t, profilerStoreEnableBody)
	gpuclient.profStreamBinPath = mockStreamScript(t)

	gpuclient.hrMu.Lock()
	gpuclient.startProfilerStreamLocked(1000)
	gpuclient.hrMu.Unlock()
	assert.Assert(t, gpuclient.profStreamMgr != nil, "manager not started")
	assert.Assert(t, gpuclient.profStreamOn.Load(), "profStreamOn not set")

	// Pre-seed a profiler field so stop's DropFields has something to clear.
	gpuclient.gpuHandler.mh.GetStore().Update("dev1",
		map[string]snapshotstore.Sample{"gpu_prof_sq_waves": {Value: 1}})

	time.Sleep(50 * time.Millisecond) // let the child spawn before stop

	gpuclient.hrMu.Lock()
	gpuclient.stopProfilerStreamLocked()
	gpuclient.hrMu.Unlock()
	assert.Assert(t, gpuclient.profStreamMgr == nil, "manager not cleared on stop")
	assert.Assert(t, !gpuclient.profStreamOn.Load(), "profStreamOn still set after stop")

	if dev := gpuclient.gpuHandler.mh.GetStore().Snapshot("dev1"); dev != nil {
		dev.Range(func(f string, _ snapshotstore.Sample) bool {
			assert.Assert(t, f != "gpu_prof_sq_waves", "profiler field not cleared on stop")
			return true
		})
	}
}
