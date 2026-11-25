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

// High-resolution power fast path: at sub-second watch periods gpuagent pushes
// a partial GPUWatch frame (Spec carries only Id) on every power tick, carrying
// only the power attributes refreshed that tick. applyPowerFrame writes them
// through per-(GPU, field) snapshot-store hot cells, so a frame costs no lock,
// no allocation and no logging in steady state. Feed goroutine only.
package gpuagent

import (
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	exportermetrics "github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

// hrMaxAttrID bounds the GPUWatchAttrId values the fast path indexes; the
// enum stays below it.
const hrMaxAttrID = 256

const (
	hrAttrPowerUsage    = amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_POWER_USAGE
	hrAttrGPUPowerUsage = amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_POWER_USAGE
)

// hrPackagePowerField is the ga.fl / allowlist key of gpu_package_power, which
// attr 126 (current_socket_power) feeds at sub-second periods.
var hrPackagePowerField = exportermetrics.GPUMetricField_GPU_PACKAGE_POWER.String()

// hrPowerAttrIDs is the power family gpuagent refreshes every power tick at
// sub-second periods and streams in partial frames. Full frames omit it at
// those periods; attrs without a highResWatchNames entry (141, 142) are not
// exported.
var hrPowerAttrIDs = []amdgpu.GPUWatchAttrId{
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_POWER_USAGE,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_ENERGY_CONSUMED,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_SOC_VOLTAGE,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_MEM_VOLTAGE,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_CURRENT_ACC_COUNTER,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PROC_HOT_RESIDENCY_ACC,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_PPT_RESIDENCY_ACC,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACC,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_VR_THERMAL_RESIDENCY_ACC,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_VIOLATION_HBM_THERMAL_RESIDENCY_ACC,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FIRMWARE_TIMESTAMP,
	amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_SYSTEM_CLOCK_COUNTER,
}

// hrPowerAttr is hrPowerAttrIDs indexed by attr id.
var hrPowerAttr = func() (m [hrMaxAttrID]bool) {
	for _, id := range hrPowerAttrIDs {
		m[id] = true
	}
	return m
}()

// hrHotField is the fast-path state of one (GPU, attr).
type hrHotField struct {
	cell      *snapshotstore.HotCell // existing metric name for this attr
	alias     *snapshotstore.HotCell // gpu_package_power (attr 126 only)
	skip      bool                   // not in allowlist, or unsupported (cached)
	aliasSkip bool                   // GPU_PACKAGE_POWER not fed from this attr
	naSeen    bool                   // first N/A already logged
}

// hrHotGPU is the fast-path state of one GPU (or partition).
type hrHotGPU struct {
	// labels is shared, not cloned: hrLabelsByUUID and the workload-label
	// cache maps are never mutated once published
	labels    prometheus.Labels
	deviceKey string
	gpuID     string // ga.fl key
	fields    [hrMaxAttrID]hrHotField
	// pkgFrom126 is true while attr 126 last arrived valid at a sub-second
	// period: gpu_package_power then follows it and full frames' POWER_USAGE
	// (5) is not written. An N/A 126 hands gpu_package_power back to 5.
	pkgFrom126 bool
}

// hotCell returns the registered hot cell a full frame must write attr id
// through (nil if none), so a full frame never replaces a hot sample.
func (g *hrHotGPU) hotCell(id amdgpu.GPUWatchAttrId) *snapshotstore.HotCell {
	switch {
	case id == hrAttrPowerUsage:
		return g.fields[hrAttrGPUPowerUsage].alias
	case int(id) < hrMaxAttrID && hrPowerAttr[id]:
		return g.fields[id].cell
	}
	return nil
}

// hrHotCache is the per-GPU fast-path cache with the inputs it was built from;
// a change in any of them triggers a rebuild on the next partial frame.
type hrHotCache struct {
	epoch    uint64                        // store.Epoch() read before (re)building
	wlLabels *map[string]prometheus.Labels // hrWorkloadLabelCache map seen
	fieldGen uint64                        // ga.exportFieldGen seen
	byGPU    map[[16]byte]*hrHotGPU
}

// hrSubSecond reports whether the accepted watch period is below 1000 ms. Zero
// (no watch created yet) is not sub-second.
func (ga *GPUAgentGPUClient) hrSubSecond() bool {
	ms := ga.hrFastMs.Load()
	return ms != 0 && ms < highResFallbackFastPeriodMs
}

// hrWorkloadLabels returns the workload-label cache's current map pointer, nil
// when no cache runs.
func (ga *GPUAgentGPUClient) hrWorkloadLabels() *map[string]prometheus.Labels {
	if ga.hrWLCache == nil {
		return nil
	}
	return ga.hrWLCache.labels.Load()
}

// applyPowerFrame applies one partial (power) frame through the hot cells. Only
// the attributes present are written; N/A is never written (and never 0). The
// first N/A per (GPU, attr) is logged once through ga.fl, as on the full path.
func (ga *GPUAgentGPUClient) applyPowerFrame(w *amdgpu.GPUWatch) {
	st := ga.gpuHandler.mh.GetStore()
	if st == nil || w == nil {
		return
	}
	c := ga.hrHot
	if wl := ga.hrWorkloadLabels(); c == nil || st.Epoch() != c.epoch ||
		wl != c.wlLabels || ga.exportFieldGen.Load() != c.fieldGen {
		c = ga.rebuildHot(st)
	}
	now := time.Now().UnixMicro()
	for _, perGPU := range w.GetStats().GetGPUWatchAttr() {
		b := perGPU.GetGPU()
		if len(b) != 16 {
			continue
		}
		g := c.byGPU[[16]byte(b)]
		if g == nil {
			continue
		}
		for _, attr := range perGPU.GetAttr() {
			id := attr.GetId()
			if id < 0 || int(id) >= hrMaxAttrID {
				continue
			}
			f := &g.fields[id]
			if f.skip && f.aliasSkip {
				continue
			}
			v, ok := watchAttrFloat(attr.GetValue())
			if !ok {
				if id == hrAttrGPUPowerUsage {
					g.pkgFrom126 = false
				}
				if !f.naSeen {
					f.naSeen = true
					field := strings.ToUpper(highResWatchNames[id])
					ga.fl.logUnsupportedField(g.gpuID, field)
					if ga.fl.checkUnsupportedFields(g.gpuID, field) {
						f.skip, f.aliasSkip = true, true
					}
				}
				continue
			}
			if !f.skip {
				if f.cell == nil {
					f.cell = st.RegisterHot(g.deviceKey, highResWatchNames[id],
						g.labels, v, now)
				} else {
					f.cell.Set(v, now)
				}
			}
			if !f.aliasSkip {
				if f.alias == nil {
					f.alias = st.RegisterHot(g.deviceKey,
						highResWatchNames[hrAttrPowerUsage], g.labels, v, now)
				} else {
					f.alias.Set(v, now)
				}
				g.pkgFrom126 = true
			}
		}
	}
}

// rebuildHot rebuilds the fast-path cache (slow path: feed start, store epoch
// bump, workload-label swap, field-config reload). The store epoch is read
// first, so a store change racing the rebuild is seen on the next frame. Cells
// are kept when the epoch and a GPU's labels are unchanged; otherwise they are
// re-registered lazily on the GPU's next valid value.
func (ga *GPUAgentGPUClient) rebuildHot(st snapshotstore.StoreWriter) *hrHotCache {
	ga.hrHotRebuilds.Add(1)
	epoch := st.Epoch()
	wl := ga.hrWorkloadLabels()
	old := ga.hrHot
	c := &hrHotCache{
		epoch:    epoch,
		wlLabels: wl,
		fieldGen: ga.exportFieldGen.Load(),
		byGPU:    make(map[[16]byte]*hrHotGPU, len(ga.hrLabelsByUUID)),
	}
	keepCells := old != nil && old.epoch == epoch
	sameGen := old != nil && old.fieldGen == c.fieldGen
	sub := ga.hrSubSecond()
	skipKeys := ga.workloadLabelKeys()
	// allowlist state is per field, not per GPU
	var enabled [hrMaxAttrID]bool
	for _, id := range hrPowerAttrIDs {
		if name := highResWatchNames[id]; name != "" {
			enabled[id] = ga.getExporterFieldState(name)
		}
	}
	pkgEnabled := ga.getExporterFieldState(hrPackagePowerField)
	for uuidStr, labels := range ga.hrLabelsByUUID {
		if wl != nil {
			if l, ok := (*wl)[uuidStr]; ok && l != nil {
				labels = l
			}
		}
		if labels == nil {
			continue
		}
		gu, err := uuid.Parse(uuidStr)
		if err != nil {
			continue
		}
		gpuID := ga.hrGPUIDByUUID[uuidStr]
		if gpuID == "" {
			gpuID = uuidStr
		}
		g := &hrHotGPU{
			labels:    labels,
			deviceKey: gpuStoreDeviceKey(labels, skipKeys),
			gpuID:     gpuID,
		}
		var prev *hrHotGPU
		if old != nil {
			prev = old.byGPU[gu]
		}
		reuse := keepCells && prev != nil && prev.deviceKey == g.deviceKey &&
			maps.Equal(prev.labels, labels)
		for i := range g.fields {
			g.fields[i].skip, g.fields[i].aliasSkip = true, true
		}
		for _, id := range hrPowerAttrIDs {
			name := highResWatchNames[id]
			if name == "" {
				continue
			}
			field := strings.ToUpper(name)
			unsupported := ga.fl.checkUnsupportedFields(gpuID, field)
			f := &g.fields[id]
			f.skip = !enabled[id] || unsupported
			// GPU_PACKAGE_POWER's own unsupported mark comes from attr 5
			// (average power), so it must not stop 126 feeding the metric
			if id == hrAttrGPUPowerUsage {
				f.aliasSkip = !sub || !pkgEnabled || unsupported
			}
			// after a field-config reload (fl reset) the first N/A logs again
			if prev != nil && sameGen {
				f.naSeen = prev.fields[id].naSeen
			}
			if reuse {
				f.cell, f.alias = prev.fields[id].cell, prev.fields[id].alias
			}
		}
		if prev != nil && !g.fields[hrAttrGPUPowerUsage].aliasSkip {
			g.pkgFrom126 = prev.pkgFrom126
		}
		c.byGPU[gu] = g
	}
	ga.hrHot = c
	return c
}
