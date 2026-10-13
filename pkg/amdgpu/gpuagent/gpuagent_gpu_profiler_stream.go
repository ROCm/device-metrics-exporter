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

// High-resolution GPU profiler streaming (P7): in high-res mode the exporter
// runs a long-lived `rocpctl --stream` child (ProfilerStreamManager) whose
// decoded samples feed the snapshot store continuously, replacing today's
// per-scrape `rocpctl` exec for the gpu_prof_* fields. Additive: the per-scrape
// profiler path is untouched and still owns gpu_prof_* whenever the stream is
// gated off (non-high-res, profiler disabled, or PTL platform).
package gpuagent

import (
	"context"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/rocprofiler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

const (
	// profilerStreamBackoffBase is the supervisor restart backoff base (design: 10s).
	profilerStreamBackoffBase = 10 * time.Second
	// profilerStreamStopTimeout bounds the lifecycle Stop wait. It must exceed the
	// manager's internal SIGTERM→SIGKILL escalation window (Task 1 stopTimeout, 5s)
	// so a slow child is fully reaped rather than abandoned mid-shutdown.
	profilerStreamStopTimeout = 10 * time.Second
)

// resolveRocpctlPath resolves the rocpctl binary via PATH, matching the one-shot
// path's PATH-resolved `rocpctl` so the stream and per-scrape entry points cannot
// diverge. Falls back to the bare name (exec surfaces a clear not-found error).
func resolveRocpctlPath() string {
	if p, err := exec.LookPath("rocpctl"); err == nil {
		return p
	}
	return "rocpctl"
}

// profilerAliasField binds a rocprofiler output alias (the MetricValue.Field in a
// decoded sample) to the GPU_PROF_* enum field it feeds and an optional value
// transform. It is the authoritative alias→field mapping for the stream writer and
// mirrors the pull path's updateGPUInfoToMetrics profiler switch exactly, so a
// streamed sample lands under the same gpu_prof_* store key the per-scrape path
// writes for the same alias.
type profilerAliasField struct {
	field     exportermetrics.GPUMetricField
	transform func(float64) float64
}

// profilerAliasFields mirrors the updateGPUInfoToMetrics profiler switch: each
// alias maps to its GPU_PROF_* field (the lowercased enum name is the gauge name =
// store field key) and the same clamp the pull path applies. GPU_PROF_OCCUPANCY_ELAPSED
// is not here: it is derived (see profilerStoreFields), not a direct alias.
var profilerAliasFields = map[string]profilerAliasField{
	"GRBM_GUI_ACTIVE":                    {exportermetrics.GPUMetricField_GPU_PROF_GRBM_GUI_ACTIVE, nil},
	"SQ_WAVES":                           {exportermetrics.GPUMetricField_GPU_PROF_SQ_WAVES, nil},
	"GRBM_COUNT":                         {exportermetrics.GPUMetricField_GPU_PROF_GRBM_COUNT, nil},
	"GPU_UTIL":                           {exportermetrics.GPUMetricField_GPU_PROF_GUI_UTIL_PERCENT, utils.NormalizePercent},
	"FETCH_SIZE":                         {exportermetrics.GPUMetricField_GPU_PROF_FETCH_SIZE, nil},
	"WRITE_SIZE":                         {exportermetrics.GPUMetricField_GPU_PROF_WRITE_SIZE, nil},
	"TOTAL_16_OPS":                       {exportermetrics.GPUMetricField_GPU_PROF_TOTAL_16_OPS, nil},
	"TOTAL_32_OPS":                       {exportermetrics.GPUMetricField_GPU_PROF_TOTAL_32_OPS, nil},
	"TOTAL_64_OPS":                       {exportermetrics.GPUMetricField_GPU_PROF_TOTAL_64_OPS, nil},
	"CPC_CPC_STAT_BUSY":                  {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_STAT_BUSY, nil},
	"CPC_CPC_STAT_IDLE":                  {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_STAT_IDLE, nil},
	"CPC_CPC_STAT_STALL":                 {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_STAT_STALL, nil},
	"CPC_CPC_TCIU_BUSY":                  {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_TCIU_BUSY, nil},
	"CPC_CPC_TCIU_IDLE":                  {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_TCIU_IDLE, nil},
	"CPC_CPC_UTCL2IU_BUSY":               {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_UTCL2IU_BUSY, nil},
	"CPC_CPC_UTCL2IU_IDLE":               {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_UTCL2IU_IDLE, nil},
	"CPC_CPC_UTCL2IU_STALL":              {exportermetrics.GPUMetricField_GPU_PROF_CPC_CPC_UTCL2IU_STALL, nil},
	"CPC_ME1_BUSY_FOR_PACKET_DECODE":     {exportermetrics.GPUMetricField_GPU_PROF_CPC_ME1_BUSY_FOR_PACKET_DECODE, nil},
	"CPC_ME1_DC0_SPI_BUSY":               {exportermetrics.GPUMetricField_GPU_PROF_CPC_ME1_DC0_SPI_BUSY, nil},
	"CPC_UTCL1_STALL_ON_TRANSLATION":     {exportermetrics.GPUMetricField_GPU_PROF_CPC_UTCL1_STALL_ON_TRANSLATION, nil},
	"CPC_ALWAYS_COUNT":                   {exportermetrics.GPUMetricField_GPU_PROF_CPC_ALWAYS_COUNT, nil},
	"CPC_ADC_VALID_CHUNK_NOT_AVAIL":      {exportermetrics.GPUMetricField_GPU_PROF_CPC_ADC_VALID_CHUNK_NOT_AVAIL, nil},
	"CPC_ADC_DISPATCH_ALLOC_DONE":        {exportermetrics.GPUMetricField_GPU_PROF_CPC_ADC_DISPATCH_ALLOC_DONE, nil},
	"CPC_ADC_VALID_CHUNK_END":            {exportermetrics.GPUMetricField_GPU_PROF_CPC_ADC_VALID_CHUNK_END, nil},
	"CPC_SYNC_FIFO_FULL_LEVEL":           {exportermetrics.GPUMetricField_GPU_PROF_CPC_SYNC_FIFO_FULL_LEVEL, nil},
	"CPC_SYNC_FIFO_FULL":                 {exportermetrics.GPUMetricField_GPU_PROF_CPC_SYNC_FIFO_FULL, nil},
	"CPC_GD_BUSY":                        {exportermetrics.GPUMetricField_GPU_PROF_CPC_GD_BUSY, nil},
	"CPC_TG_SEND":                        {exportermetrics.GPUMetricField_GPU_PROF_CPC_TG_SEND, nil},
	"CPC_WALK_NEXT_CHUNK":                {exportermetrics.GPUMetricField_GPU_PROF_CPC_WALK_NEXT_CHUNK, nil},
	"CPC_STALLED_BY_SE0_SPI":             {exportermetrics.GPUMetricField_GPU_PROF_CPC_STALLED_BY_SE0_SPI, nil},
	"CPC_STALLED_BY_SE1_SPI":             {exportermetrics.GPUMetricField_GPU_PROF_CPC_STALLED_BY_SE1_SPI, nil},
	"CPC_STALLED_BY_SE2_SPI":             {exportermetrics.GPUMetricField_GPU_PROF_CPC_STALLED_BY_SE2_SPI, nil},
	"CPC_STALLED_BY_SE3_SPI":             {exportermetrics.GPUMetricField_GPU_PROF_CPC_STALLED_BY_SE3_SPI, nil},
	"CPC_LTE_ALL":                        {exportermetrics.GPUMetricField_GPU_PROF_CPC_LTE_ALL, nil},
	"CPC_SYNC_WRREQ_FIFO_BUSY":           {exportermetrics.GPUMetricField_GPU_PROF_CPC_SYNC_WRREQ_FIFO_BUSY, nil},
	"CPC_CANE_BUSY":                      {exportermetrics.GPUMetricField_GPU_PROF_CPC_CANE_BUSY, nil},
	"CPC_CANE_STALL":                     {exportermetrics.GPUMetricField_GPU_PROF_CPC_CANE_STALL, nil},
	"CPF_CMP_UTCL1_STALL_ON_TRANSLATION": {exportermetrics.GPUMetricField_GPU_PROF_CPF_CMP_UTCL1_STALL_ON_TRANSLATION, nil},
	"CPF_CPF_STAT_BUSY":                  {exportermetrics.GPUMetricField_GPU_PROF_CPF_CPF_STAT_BUSY, nil},
	"CPF_CPF_STAT_IDLE":                  {exportermetrics.GPUMetricField_GPU_PROF_CPF_CPF_STAT_IDLE, nil},
	"CPF_CPF_STAT_STALL":                 {exportermetrics.GPUMetricField_GPU_PROF_CPF_CPF_STAT_STALL, nil},
	"CPF_CPF_TCIU_BUSY":                  {exportermetrics.GPUMetricField_GPU_PROF_CPF_CPF_TCIU_BUSY, nil},
	"CPF_CPF_TCIU_IDLE":                  {exportermetrics.GPUMetricField_GPU_PROF_CPF_CPF_TCIU_IDLE, nil},
	"CPF_CPF_TCIU_STALL":                 {exportermetrics.GPUMetricField_GPU_PROF_CPF_CPF_TCIU_STALL, nil},
	"OccupancyPercent":                   {exportermetrics.GPUMetricField_GPU_PROF_OCCUPANCY_PERCENT, utils.NormalizePercent},
	"MfmaUtil":                           {exportermetrics.GPUMetricField_GPU_PROF_TENSOR_ACTIVE_PERCENT, utils.NormalizePercent},
	"ValuPipeIssueUtil":                  {exportermetrics.GPUMetricField_GPU_PROF_VALU_PIPE_ISSUE_UTIL, utils.NormalizePercent},
	"VALUBusy":                           {exportermetrics.GPUMetricField_GPU_PROF_SM_ACTIVE, utils.NormalizePercent},
	"MeanOccupancyPerActiveCU":           {exportermetrics.GPUMetricField_GPU_PROF_OCCUPANCY_PER_ACTIVE_CU, nil},
	"MeanOccupancyPerCU":                 {exportermetrics.GPUMetricField_GPU_PROF_OCCUPANCY_PER_CU, nil},
	"SIMD_UTILIZATION":                   {exportermetrics.GPUMetricField_GPU_PROF_SIMD_UTILIZATION, utils.NormalizeFraction},
}

// profilerStoreFields maps a decoded sample's alias→value map to store field
// key→value (keys are the lowercased GPU_PROF_* names = gauge names). It is the
// single source of the alias→field+transform+derived mapping; the stream writer's
// store keys are therefore byte-identical to the pull path's for the same aliases.
// GPU_PROF_OCCUPANCY_ELAPSED is derived (MeanOccupancyPerActiveCU / GRBM_GUI_ACTIVE),
// mirroring updateGPUInfoToMetrics.
func profilerStoreFields(aliasVals map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(aliasVals)+1)
	for alias, v := range aliasVals {
		af, ok := profilerAliasFields[alias]
		if !ok {
			continue
		}
		val := v
		if af.transform != nil {
			val = af.transform(v)
		}
		out[strings.ToLower(af.field.String())] = val
	}
	// Derived occupancy-elapsed: occupancy / active cycles, only when active
	// cycles are non-zero (matches the pull path's division guard).
	grbmActive, hasActive := aliasVals["GRBM_GUI_ACTIVE"]
	occPerActiveCU, hasOcc := aliasVals["MeanOccupancyPerActiveCU"]
	if hasActive && hasOcc && grbmActive != 0 {
		out[strings.ToLower(exportermetrics.GPUMetricField_GPU_PROF_OCCUPANCY_ELAPSED.String())] = occPerActiveCU / grbmActive
	}
	return out
}

// profilerStreamSink is the store-backed StreamSink: it maps each decoded profiler
// sample to per-device gpu_prof_* store deltas and merges them via StoreWriter.Update.
// It holds its own label cache (not ga's watch-feed caches) so reads are race-free
// across the manager goroutine.
type profilerStreamSink struct {
	ga     *GPUAgentGPUClient
	labels *profilerLabelCache
}

func (s *profilerStreamSink) Publish(p *amdgpu.GpuProfiler) {
	s.ga.publishProfilerSample(p, s.labels)
}

// publishProfilerSample merges one decoded profiler sample into the store: one
// per-device delta per GPU, keyed by the stable label set (the same device key the
// pull/watch writers use), with gpu_prof_* field keys. Honors the config field-gate
// so a disabled profiler field is absent from the store, matching the pull surface.
// GPUs whose labels are not yet cached are skipped (transient, self-heals), as the
// watch writer does.
func (ga *GPUAgentGPUClient) publishProfilerSample(p *amdgpu.GpuProfiler, lc *profilerLabelCache) {
	if p == nil || lc == nil {
		return
	}
	st := ga.gpuHandler.mh.GetStore()
	if st == nil {
		return
	}
	var store snapshotstore.StoreWriter = st
	now := time.Now().UnixMicro()
	workloadSkip := ga.workloadLabelKeys()
	for _, gm := range p.GetGpuMetrics() {
		labels, ok := lc.get(gm.GetDrmRenderId())
		if !ok || labels == nil {
			continue
		}
		aliasVals := make(map[string]float64, len(gm.GetMetrics()))
		for _, m := range gm.GetMetrics() {
			aliasVals[m.GetField()] = strDoubleToFloat(m.GetValue())
		}
		fields := profilerStoreFields(aliasVals)
		if len(fields) == 0 {
			continue
		}
		deltas := make(map[string]snapshotstore.Sample, len(fields))
		for field, val := range fields {
			if !ga.getExporterFieldState(field) {
				continue
			}
			deltas[field] = snapshotstore.Sample{Value: val, Labels: labels, TsUs: now}
		}
		if len(deltas) == 0 {
			continue
		}
		store.Update(gpuStoreDeviceKey(labels, workloadSkip), deltas)
	}
}

// profilerLabelCache holds the per-GPU full label set keyed by DRM render id (the
// key a decoded profiler sample carries). It is refreshed off the stream hot path
// on the same cadence as the workload-label cache, since ListWorkloads can block on
// the scheduler. atomic.Pointer makes get() lock-free and race-free across the
// manager goroutine.
type profilerLabelCache struct {
	ga     *GPUAgentGPUClient
	labels atomic.Pointer[map[string]prometheus.Labels]
}

func newProfilerLabelCache(ga *GPUAgentGPUClient) *profilerLabelCache {
	return &profilerLabelCache{ga: ga}
}

// refreshOnce rebuilds the renderID→label map from the same inputs as the pull
// path (workloads + GPUGet), so streamed profiler series carry labels 1-1 with the
// per-scrape profiler series. The new map is never mutated after Store.
func (c *profilerLabelCache) refreshOnce() {
	// Reuse the subscribe-time snapshot (sole-amd-smi-caller invariant); only the
	// scheduler-sourced workload labels change at runtime.
	snap := c.ga.hrSnapshot.Load()
	if snap == nil || len(snap.gpus) == 0 {
		return
	}
	gpus := snap.gpus
	partitionMap := snap.partitionMap
	wls, _ := c.ga.gpuHandler.ListWorkloads()
	next := make(map[string]prometheus.Labels, len(gpus))
	for _, gpu := range gpus {
		rid := getGPURenderId(gpu)
		if rid == "" {
			continue
		}
		next[rid] = prometheus.Labels(c.ga.populateLabelsFromGPU(wls, gpu, partitionMap))
	}
	c.labels.Store(&next)
}

func (c *profilerLabelCache) get(renderID string) (prometheus.Labels, bool) {
	m := c.labels.Load()
	if m == nil {
		return nil, false
	}
	l, ok := (*m)[renderID]
	return l, ok
}

// start seeds the cache synchronously, then refreshes on a ticker until ctx is
// done. Each tick also clears stale profiler fields if the stream has hit the
// failure-disable threshold, so a terminally-disabled profiler stops serving
// last-known gpu_prof_* values instead of leaving them frozen.
func (c *profilerLabelCache) start(ctx context.Context, interval time.Duration) {
	c.refreshOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refreshOnce()
			c.ga.dropProfilerFieldsIfDisabled()
		}
	}
}

// dropProfilerFieldsIfDisabled evicts the profiler store fields once the stream is
// disabled on failure (3-strike or crash), so /metrics serves only the
// always-present signals until a pod restart re-enables the profiler.
func (ga *GPUAgentGPUClient) dropProfilerFieldsIfDisabled() {
	if ga.rocpclient == nil || !ga.rocpclient.IsDisabledOnFailure() {
		return
	}
	if store := ga.gpuHandler.mh.GetStore(); store != nil {
		store.DropFields(highResProfilerFieldSet)
	}
}

// profilerStreamGate decides whether the high-res profiler stream should run and,
// if so, the --interval (ms) derived from the high-res fast period. It runs only
// when high-res AND profiler is enabled AND the platform is non-PTL (PtlDelay == 0);
// PTL high-res profiler is unsupported by design and keeps the per-scrape path.
func (ga *GPUAgentGPUClient) profilerStreamGate() (bool, uint64) {
	if !ga.gpuHandler.mh.IsHighResolution() {
		return false, 0
	}
	if !ga.enableProfileMetrics || !ga.gpuHandler.enableGPUMonitoring {
		return false, 0
	}
	// Already failure-disabled (3-strike or crash): don't spawn a manager that will
	// immediately exit, and don't flip the slow-writer skip-mask for a dead stream.
	if ga.rocpclient == nil || ga.rocpclient.IsDisabledOnFailure() {
		return false, 0
	}
	if ga.gpuHandler.mh.GetProfilerConfig().GetPtlDelay() != 0 {
		return false, 0
	}
	// GetHighResPeriods already applies the fast-period default, so fastSecs is
	// never 0 here; converting to ms cannot yield rocpctl's interval-0 max-rate.
	fastSecs, _ := ga.gpuHandler.mh.GetHighResPeriods()
	return true, uint64(fastSecs) * 1000
}

// reconcileProfilerStreamLocked starts/stops the profiler stream to match the gate,
// symmetric with the GPUWatch feed reconciliation. Caller must hold hrMu.
func (ga *GPUAgentGPUClient) reconcileProfilerStreamLocked() {
	run, intervalMs := ga.profilerStreamGate()
	running := ga.profStreamMgr != nil
	switch {
	case run && !running:
		ga.startProfilerStreamLocked(intervalMs)
	case !run && running:
		ga.stopProfilerStreamLocked()
	case run && running:
		// Still gated on across a config reload: keep the child alive and only
		// retune if the cadence changed (Reconfigure kills+respawns the child).
		if intervalMs != ga.profStreamIntervalMs {
			ga.profStreamIntervalMs = intervalMs
			ga.profStreamMgr.Reconfigure(ga.rocpclient.MetricFields, intervalMs, ga.rocpclient.SamplingInterval)
		}
	}
}

// startProfilerStreamLocked spawns the stream manager and its label cache. Caller
// must hold hrMu.
func (ga *GPUAgentGPUClient) startProfilerStreamLocked(intervalMs uint64) {
	ctx := ga.gpuHandler.ctx
	lc := newProfilerLabelCache(ga)
	lcCtx, cancel := context.WithCancel(ctx)
	go lc.start(lcCtx, hrWorkloadLabelRefreshInterval)

	sink := &profilerStreamSink{ga: ga, labels: lc}
	mgr := rocprofiler.NewProfilerStreamManager(ga.profStreamBinPath, ga.rocpclient.MetricFields,
		intervalMs, ga.rocpclient.SamplingInterval, sink, ga.rocpclient, profilerStreamBackoffBase)
	if err := mgr.Start(ctx); err != nil {
		logger.Log.Printf("profiler high-res stream: start failed: %v", err)
		cancel()
		return
	}
	ga.profStreamMgr = mgr
	ga.profLabelCache = lc
	ga.profLabelCancel = cancel
	ga.profStreamIntervalMs = intervalMs
	ga.profStreamOn.Store(true)
	logger.Log.Printf("profiler high-res stream ENABLED (interval=%dms)", intervalMs)
}

// stopProfilerStreamLocked stops the stream manager and label cache and clears the
// profiler fields it owns from the store. Caller must hold hrMu.
func (ga *GPUAgentGPUClient) stopProfilerStreamLocked() {
	ga.profStreamOn.Store(false)
	if ga.profLabelCancel != nil {
		ga.profLabelCancel()
		ga.profLabelCancel = nil
	}
	ga.profLabelCache = nil
	if ga.profStreamMgr != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), profilerStreamStopTimeout)
		ga.profStreamMgr.Stop(stopCtx)
		cancel()
		ga.profStreamMgr = nil
	}
	ga.profStreamIntervalMs = 0
	// Clear stream-accrued consecutive failures so a high-res→non-PTL toggle hands
	// the per-scrape path a clean counter. Preserve a fatal (crash) auto-disable —
	// a config reload must NOT re-enable a core-dumping rocpctl; only a pod restart
	// (Close) clears that.
	if ga.rocpclient != nil {
		ga.rocpclient.ResetConsecutiveFailures()
	}
	if store := ga.gpuHandler.mh.GetStore(); store != nil {
		store.DropFields(highResProfilerFieldSet)
	}
	logger.Log.Printf("profiler high-res stream DISABLED (profiler fields cleared)")
}

// teardownProfilerStream stops the profiler stream on shutdown. Safe when not
// running (no-op path inside the locked stop).
func (ga *GPUAgentGPUClient) teardownProfilerStream() {
	ga.hrMu.Lock()
	defer ga.hrMu.Unlock()
	if ga.profStreamMgr == nil && ga.profLabelCancel == nil {
		return
	}
	ga.stopProfilerStreamLocked()
}
