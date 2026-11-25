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
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

// hrNA is gpuagent's N/A sentinel for a uint64 attribute.
const hrNA = math.MaxUint64

var hrLoggerOnce sync.Once

// newHRFastClient builds a minimal high-res GPU client (metrics handler with a
// store, field allowlist, fieldLogger, label caches for n synthetic GPUs) so
// applyPowerFrame/applyWatchSnapshot can be driven directly. It needs no gRPC
// mocks, so benchmarks use it too. fastMs is the accepted watch period.
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

// TestHighResFrameDispatch: a frame whose Spec has no Attribute takes the
// fast path (hot cells, cache built); a full frame takes applyWatchSnapshot
// and builds no cache.
func TestHighResFrameDispatch(t *testing.T) {
	// run consumes frames until the stream blocks, then stops the consumer;
	// the frames are applied before Recv blocks, so all of them are in.
	run := func(ga *GPUAgentGPUClient, frames ...*amdgpu.GPUWatch) {
		ctx, cancel := context.WithCancel(context.Background())
		s := &blockingStream{ctx: ctx, frames: frames}
		done := make(chan struct{})
		go func() {
			defer close(done)
			ga.consumeWatchStream(ctx, s)
		}()
		time.Sleep(50 * time.Millisecond)
		cancel()
		<-done
	}

	ga, gpus := newHRFastClient(t, 1, 10)
	run(ga, hrFull(gpus, hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 50)))
	assert.Assert(t, ga.hrHot == nil, "full frame must not use the fast path")
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_junction_temperature"), 50.0)

	run(ga, hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 250)))
	assert.Assert(t, ga.hrHot != nil, "partial frame must use the fast path")
	assert.Assert(t, hrGPU(ga, gpus[0]).fields[hrAttrGPUPowerUsage].cell != nil)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 250.0)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_junction_temperature"), 50.0)
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

// TestHighResPartialFrameNANeverWritten: N/A is never stored and never 0; the
// first N/A is recorded once in ga.fl; a valid value that later goes N/A keeps
// the prior value.
func TestHighResPartialFrameNANeverWritten(t *testing.T) {
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
}

// TestHighResFullFrameSubSecondSkipsPowerNames: at sub-second periods a full
// frame writes no power-family name; at 1000 ms it writes them as before.
func TestHighResFullFrameSubSecondSkipsPowerNames(t *testing.T) {
	attrs := append(hrPowerTick(7), hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 50))
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
}

// TestHighResPackagePowerRule: per GPU, a valid 126 drives gpu_package_power
// and suppresses the full frame's POWER_USAGE (5); an N/A 126 hands it back to
// 5 (written through the alias cell, no epoch bump); a valid 126 takes over
// again.
func TestHighResPackagePowerRule(t *testing.T) {
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

	// 126 goes N/A on GPU a: the next full frame's 5 feeds it, through the cell
	ga.applyPowerFrame(hrPartial([][]byte{a}, hrAttr(hrAttrGPUPowerUsage, hrNA)))
	assert.Assert(t, !hrGPU(ga, a).pkgFrom126)
	assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 500.0, "N/A keeps the prior value")
	epoch := st.Epoch()
	ga.applyWatchSnapshot(hrFull([][]byte{a}, hrAttr(hrAttrPowerUsage, 440)))
	assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 440.0)
	assert.Equal(t, st.Epoch(), epoch, "write-through must not orphan the alias cell")

	// valid again: 126 takes over (state spans frames)
	ga.applyPowerFrame(hrPartial([][]byte{a}, hrAttr(hrAttrGPUPowerUsage, 520)))
	ga.applyWatchSnapshot(hrFull([][]byte{a}, hrAttr(hrAttrPowerUsage, 430)))
	assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 520.0)
	assert.Equal(t, hrMustGet(t, ga, a, "gpu_power_usage"), 520.0)
}

// TestHighResPackagePowerRuleInertAt1000: at 1000 ms 126 never feeds
// gpu_package_power and 5 does, as before.
func TestHighResPackagePowerRuleInertAt1000(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 1000)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500)))
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 500.0)
	_, ok := hrGet(ga, gpus[0], "gpu_package_power")
	assert.Assert(t, !ok)
	assert.Assert(t, !hrGPU(ga, gpus[0]).pkgFrom126)
	ga.applyWatchSnapshot(hrFull(gpus, hrAttr(hrAttrPowerUsage, 450)))
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_package_power"), 450.0)
}

// TestHighResPackagePowerAllowlist: the GPUConfig.Fields allowlist is honoured
// on either name.
func TestHighResPackagePowerAllowlist(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 10, "GPU_POWER_USAGE")
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500)))
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_power_usage"), 500.0)
	_, ok := hrGet(ga, gpus[0], "gpu_package_power")
	assert.Assert(t, !ok, "GPU_PACKAGE_POWER is not in the allowlist")

	ga, gpus = newHRFastClient(t, 1, 10, "GPU_PACKAGE_POWER")
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500)))
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_package_power"), 500.0)
	_, ok = hrGet(ga, gpus[0], "gpu_power_usage")
	assert.Assert(t, !ok, "GPU_POWER_USAGE is not in the allowlist")

	ga, gpus = newHRFastClient(t, 1, 10, "GPU_GFX_VOLTAGE")
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500),
		hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE, 800)))
	assert.Equal(t, len(hrStoreFields(ga)), 1)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_gfx_voltage"), 800.0)
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

// TestHighResScrapeNotBlockedDuringFrameFlow: a scrape (StoreCollector.Collect)
// is never blocked while 8-GPU power frames arrive every 1 ms, nor while the
// feed applies them at full speed: every Collect returns and the p99 Collect
// latency stays within 2x of idle (plus a scheduling allowance under -race).
func TestHighResScrapeNotBlockedDuringFrameFlow(t *testing.T) {
	ga, gpus := newHRFastClient(t, 8, 1)
	st := ga.gpuHandler.mh.GetStore()
	var descs []snapshotstore.FieldDesc
	for _, m := range highResWatchMetrics {
		if m.name != "" {
			descs = append(descs, snapshotstore.FieldDesc{Field: m.name, Name: "amd_" + m.name,
				ValueType:  prometheus.GaugeValue,
				LabelNames: []string{"gpu_id", "gpu_uuid", "serial_number", "card_model", "hostname", "gpu_partition_id"}})
		}
	}
	coll := snapshotstore.NewStoreCollector(st, descs)
	ga.applyWatchSnapshot(hrFull(gpus, hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 50)))
	ga.applyPowerFrame(hrPartial(gpus, hrPowerTick(1)...))

	collect := func() time.Duration {
		ch := make(chan prometheus.Metric, 256)
		start := time.Now()
		go func() {
			coll.Collect(ch)
			close(ch)
		}()
		n := 0
		for range ch {
			n++
		}
		d := time.Since(start)
		if n == 0 {
			t.Error("empty scrape")
		}
		return d
	}
	p99 := func(scrapers, per int) time.Duration {
		var mu sync.Mutex
		var lat []time.Duration
		var wg sync.WaitGroup
		for i := 0; i < scrapers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				local := make([]time.Duration, 0, per)
				for j := 0; j < per; j++ {
					local = append(local, collect())
				}
				mu.Lock()
				lat = append(lat, local...)
				mu.Unlock()
			}()
		}
		wg.Wait()
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		return lat[len(lat)*99/100]
	}
	const scrapers, per = 4, 300
	// underFeed returns the p99 Collect latency while frames are applied
	// every 1 ms or at full speed, and how many frames were applied
	underFeed := func(mode string) (time.Duration, uint64) {
		stop := make(chan struct{})
		var frames atomic.Uint64
		var fed sync.WaitGroup
		fed.Add(1)
		go func() {
			defer fed.Done()
			var tick <-chan time.Time
			if mode == "1ms" {
				tk := time.NewTicker(time.Millisecond)
				defer tk.Stop()
				tick = tk.C
			}
			w := hrPartial(gpus, hrPowerTick(0)...)
			for v := uint64(2); ; v++ {
				if tick != nil {
					select {
					case <-stop:
						return
					case <-tick:
					}
				} else {
					select {
					case <-stop:
						return
					default:
					}
				}
				for _, g := range w.Stats.GPUWatchAttr {
					for _, a := range g.Attr {
						a.Value.WatchAttrVal.(*amdgpu.GPUWatchAttrVal_LongVal).LongVal = v
					}
				}
				ga.applyPowerFrame(w)
				frames.Add(1)
			}
		}()
		busy := p99(scrapers, per)
		close(stop)
		fed.Wait()
		return busy, frames.Load()
	}
	// A Collect takes ~2 ms here, so on a shared host one p99 sample is
	// dominated by scheduling noise. Real blocking fails every attempt;
	// noise does not, so the bound must hold in one of three attempts, each
	// with a fresh idle baseline.
	for _, mode := range []string{"1ms", "full-speed"} {
		var ok bool
		for attempt := 1; attempt <= 3 && !ok; attempt++ {
			idle := p99(scrapers, per)
			busy, frames := underFeed(mode)
			limit := 2*idle + 2*time.Millisecond
			t.Logf("%s attempt %d: idle p99 %v, under feed p99 %v (limit %v), %d frames applied",
				mode, attempt, idle, busy, limit, frames)
			assert.Assert(t, frames > 0)
			ok = busy <= limit
		}
		assert.Assert(t, ok, "%s: p99 Collect above 2x idle + 2 ms in all attempts", mode)
	}
	// the last applied value is what a scrape serves
	s, ok := hrGet(ga, gpus[7], "gpu_power_usage")
	assert.Assert(t, ok && s.Value >= 2)
}

// hrBenchFrame is one 8-GPU power tick with every power attr valid.
func hrBenchFrame(b *testing.B, fastMs uint32) (*GPUAgentGPUClient, *amdgpu.GPUWatch) {
	ga, gpus := newHRFastClient(b, 8, fastMs)
	w := hrPartial(gpus, hrPowerTick(100)...)
	ga.applyPowerFrame(w)
	return ga, w
}

// BenchmarkHighResApplyPowerFrame8GPU: steady-state apply of one 8-GPU power
// frame. Target: 0 allocs/op, < 10 us/op.
func BenchmarkHighResApplyPowerFrame8GPU(b *testing.B) {
	ga, w := hrBenchFrame(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ga.applyPowerFrame(w)
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}

// BenchmarkHighResRecvApply8GPU: proto unmarshal into a fresh GPUWatch (as the
// gRPC receive does) plus apply.
func BenchmarkHighResRecvApply8GPU(b *testing.B) {
	ga, w := hrBenchFrame(b, 1)
	buf, err := proto.Marshal(w)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := &amdgpu.GPUWatch{}
		if err := proto.Unmarshal(buf, m); err != nil {
			b.Fatal(err)
		}
		ga.applyPowerFrame(m)
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}

// BenchmarkHighResHotCacheRebuild8GPU: a frame right after an unrelated store
// epoch bump (the cgroup agent's ReplaceOwned, once per poll): cache rebuild
// plus lazy re-registration of every cell.
func BenchmarkHighResHotCacheRebuild8GPU(b *testing.B) {
	ga, w := hrBenchFrame(b, 1)
	st := ga.gpuHandler.mh.GetStore()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st.ReplaceOwned("cgroup:", nil)
		ga.applyPowerFrame(w)
	}
}

// BenchmarkHighResApplyWatchSnapshot8GPU: the full-frame path (rev 2 baseline)
// for the same 8-GPU power payload at 1000 ms.
func BenchmarkHighResApplyWatchSnapshot8GPU(b *testing.B) {
	ga, gpus := newHRFastClient(b, 8, 1000)
	w := hrFull(gpus, hrPowerTick(100)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ga.applyWatchSnapshot(w)
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}

// TestHighResPackagePowerFrom126WhenAverageNA: 126 keeps feeding
// gpu_package_power when 5 (average power) is N/A or already marked
// unsupported, across cache rebuilds; an N/A 5 at sub-second does not mark
// GPU_PACKAGE_POWER unsupported once the GPU's 126 alias is live.
func TestHighResPackagePowerFrom126WhenAverageNA(t *testing.T) {
	ga, gpus := newHRFastClient(t, 2, 10)
	a, b := gpus[0], gpus[1]
	// GPU a: the pull path marked GPU_PACKAGE_POWER unsupported (average N/A)
	ga.fl.markUnsupportedFields("0", hrPackagePowerField)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 500)))
	assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 500.0)
	// GPU b: a full frame's N/A 5 must not mark it
	ga.applyWatchSnapshot(hrFull([][]byte{b}, hrAttr(hrAttrPowerUsage, hrNA)))
	assert.Assert(t, !ga.fl.checkUnsupportedFields("1", hrPackagePowerField))
	// a rebuild (field-config generation) keeps 126 as the source
	ga.exportFieldGen.Add(1)
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrGPUPowerUsage, 510)))
	assert.Equal(t, hrMustGet(t, ga, a, "gpu_package_power"), 510.0)
	assert.Equal(t, hrMustGet(t, ga, b, "gpu_package_power"), 510.0)
}

// TestHighResDispatchFullPathAt1000: at 1000 ms and above a frame without
// Spec.Attribute keeps today's path; partial frames exist only below 1000 ms.
func TestHighResDispatchFullPathAt1000(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	s := &blockingStream{ctx: ctx, frames: []*amdgpu.GPUWatch{
		hrPartial(gpus, hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 50)),
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ga.consumeWatchStream(ctx, s)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	assert.Assert(t, ga.hrHot == nil)
	assert.Equal(t, hrMustGet(t, ga, gpus[0], "gpu_junction_temperature"), 50.0)
}

// TestHighResPowerFrameMalformed: unknown GPU bytes, GPU bytes of length != 16,
// an attr id >= 256, a GPU entry with no Attr, an empty frame and a nil Value
// are skipped without writing anything.
func TestHighResPowerFrameMalformed(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 10)
	unknown := make([]byte, 16)
	ga.applyPowerFrame(&amdgpu.GPUWatch{})
	ga.applyPowerFrame(hrPartial(nil))
	ga.applyPowerFrame(hrPartial([][]byte{unknown}, hrAttr(hrAttrGPUPowerUsage, 1)))
	ga.applyPowerFrame(hrPartial([][]byte{gpus[0][:8]}, hrAttr(hrAttrGPUPowerUsage, 1)))
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(amdgpu.GPUWatchAttrId(300), 1),
		&amdgpu.GPUWatchAttr{Id: amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GFX_VOLTAGE}))
	ga.applyPowerFrame(hrPartial(gpus))
	assert.Equal(t, len(hrStoreFields(ga)), 0)
	// an attr outside the power family (5, temperature) is ignored too
	ga.applyPowerFrame(hrPartial(gpus, hrAttr(hrAttrPowerUsage, 1),
		hrAttr(amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP, 1)))
	assert.Equal(t, len(hrStoreFields(ga)), 0)
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

// TestHighResFullFramesDoNotChurnHotCells: at sub-second periods, alternating
// partial and full frames leave the store epoch and the cells unchanged after
// warm-up (full frames never replace hot samples), and the package rule holds
// through the sequence 126=350, 5=300, 126 N/A, 5=310, 126=360, 5=320.
func TestHighResFullFramesDoNotChurnHotCells(t *testing.T) {
	ga, gpus := newHRFastClient(t, 1, 1)
	st := ga.gpuHandler.mh.GetStore()
	ga.fl.SetFilterDone()
	temp := amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_GPU_TEMP
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
	for round := 0; round < 10; round++ {
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
}
