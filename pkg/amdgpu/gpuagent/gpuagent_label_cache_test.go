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

// Workload label cache (one cache, keyed by labelCacheKey): rebuild only on
// change, per-GPU map reuse, hot cache, label-input generation, start loop,
// StartHighRes seed, failure handling and concurrency.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/types"
)

// labelCacheKeys is every labelCacheKey, for tests parameterised by key.
var labelCacheKeys = []labelCacheKey{gpuUUIDKey{}, renderIDKey{}}

func newLabelCache(ga *GPUAgentGPUClient, key labelCacheKey) *workloadLabelCache {
	if _, ok := key.(renderIDKey); ok {
		return newProfilerLabelCache(ga)
	}
	return newHRWorkloadLabelCache(ga)
}

func mapPtr(m prometheus.Labels) uintptr { return reflect.ValueOf(m).Pointer() }

// TestRebuildLabelMapReuse: equal content returns prev with every per-GPU map
// reused; a change on one GPU replaces only that GPU's map.
func TestRebuildLabelMapReuse(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newWorkloadEnv(t, hrTestConfig, fx)
	_, _ = e.startHRSource(t, 0)
	snap := e.gpu.hrSnapshot.Load()
	for _, key := range labelCacheKeys {
		prev := e.gpu.rebuildLabelMap(nil, snap.gpus, snap.partitionMap, fx.k8s, key)
		next := e.gpu.rebuildLabelMap(prev, snap.gpus, snap.partitionMap, maps.Clone(fx.k8s), key)
		assert.Assert(t, next == prev, "equal content must return prev")
		for k, l := range *prev {
			assert.Equal(t, mapPtr((*next)[k]), mapPtr(l))
		}
		m := maps.Clone(fx.k8s)
		m["0000:03:00.0"] = testPod(2)
		next = e.gpu.rebuildLabelMap(prev, snap.gpus, snap.partitionMap, m, key)
		assert.Assert(t, next != prev)
		changed := 0
		for k, l := range *next {
			if mapPtr((*prev)[k]) != mapPtr(l) {
				changed++
			}
		}
		assert.Equal(t, changed, 1, "only GPU 2's map may be replaced")
	}
}

// TestLabelCacheNoRebuildWhenUnchanged: get misses before the first refresh;
// unchanged refreshes keep the pointer with 0 rebuilds and 0 allocs, and
// partial power frames rebuild the hot cache 0 times.
func TestLabelCacheNoRebuildWhenUnchanged(t *testing.T) {
	for _, key := range labelCacheKeys {
		t.Run(fmt.Sprintf("%T", key), func(t *testing.T) {
			e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
			hc, src := e.startHRSource(t, 0)
			c := hc
			if _, ok := key.(renderIDKey); ok {
				c = newProfilerLabelCache(e.gpu)
				_, ok := c.get("128")
				assert.Assert(t, !ok, "get must miss before the first refresh")
				c.refreshOnce()
			}
			_, ok := c.get("does-not-exist")
			assert.Assert(t, !ok, "get must miss for an unknown key")
			p, r := c.labels.Load(), c.rebuilds.Load()
			e.gpu.hrFastMs.Store(10) // sub-second: attr 126 feeds gpu_package_power hot cells
			frame := hrPartial(testGPUIDs(), hrPowerTick(100)...)
			e.gpu.applyPowerFrame(frame)
			hot := e.gpu.hrHotRebuilds.Load()
			for i := 0; i < 100; i++ {
				src.refresh(true)
				c.refreshOnce()
				hc.refreshOnce()
				e.gpu.applyPowerFrame(frame)
			}
			assert.Assert(t, c.labels.Load() == p)
			assert.Equal(t, c.rebuilds.Load(), r)
			assert.Equal(t, e.gpu.hrHotRebuilds.Load(), hot, "hot cache rebuilt with unchanged labels")
			assert.Equal(t, testing.AllocsPerRun(100, c.refreshOnce), float64(0))
		})
	}
}

// TestLabelCacheOneGPUChange: a pod change on one GPU replaces only that
// GPU's map; the hot cache rebuilds once and the other GPUs keep their cells.
func TestLabelCacheOneGPUChange(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newWorkloadEnv(t, hrTestConfig, fx)
	hc, src := e.startHRSource(t, 0)
	e.gpu.hrFastMs.Store(10) // sub-second: attr 126 feeds gpu_package_power hot cells
	frame := hrPartial(testGPUIDs(), hrPowerTick(100)...)
	e.gpu.applyPowerFrame(frame)
	before := maps.Clone(*hc.labels.Load())
	oldHot := e.gpu.hrHot
	base := e.gpu.hrHotRebuilds.Load()
	m := maps.Clone(fx.k8s)
	m["0000:04:00.0"] = testPod(3)
	e.k8s.set(m, nil)
	src.refresh(true)
	hc.refreshOnce()
	after := *hc.labels.Load()
	var changed string
	for k, l := range after {
		if mapPtr(before[k]) != mapPtr(l) {
			changed = k
		}
	}
	assert.Equal(t, after[changed]["pod"], "pod3")
	e.gpu.applyPowerFrame(frame)
	assert.Equal(t, e.gpu.hrHotRebuilds.Load(), base+1)
	// a GPU keeps its hot cells when every registered cell/alias is reused
	kept := 0
	for id, g := range e.gpu.hrHot.byGPU {
		og := oldHot.byGPU[id]
		if og == nil {
			continue
		}
		same, any := true, false
		for i := range og.fields {
			for _, pair := range [][2]interface{}{{og.fields[i].cell, g.fields[i].cell},
				{og.fields[i].alias, g.fields[i].alias}} {
				if !reflect.ValueOf(pair[0]).IsNil() {
					any = true
					same = same && pair[0] == pair[1]
				}
			}
		}
		if any && same {
			kept++
		}
	}
	assert.Equal(t, kept, testGPUCount-1, "unaffected GPUs must keep their hot cells")
}

// TestLabelCacheNoLostWakeup: with 1h cache tickers only the source's Changed
// wakes the caches; changes at random offsets around the refresh are never
// lost.
func TestLabelCacheNoLostWakeup(t *testing.T) {
	const iters = 200
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewSource(seed))
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	hc, _ := e.startHRSource(t, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hc.start(ctx, time.Hour)
	pc := newProfilerLabelCache(e.gpu)
	go pc.start(ctx, time.Hour)
	uuid0 := getGPUUUID(testGPUResp(testGPUCount).Response[0])
	has := func(pod string) bool {
		h, ok1 := hc.get(uuid0)
		p, ok2 := pc.get("128")
		return ok1 && ok2 && h["pod"] == pod && p["pod"] == pod
	}
	pod := func(name string) map[string]scheduler.Workload {
		return map[string]scheduler.Workload{"0000:01:00.0": {Type: scheduler.Kubernetes,
			Info: scheduler.PodResourceInfo{Pod: name}}}
	}
	for i := 1; i <= iters; i++ {
		e.k8s.set(pod(fmt.Sprint("a", i)), nil)
		time.Sleep(time.Duration(rng.Intn(1500)) * time.Microsecond)
		want := fmt.Sprint("b", i)
		e.k8s.set(pod(want), nil)
		if !waitWithin(wakeBound, func() bool { return has(want) }) {
			t.Fatalf("iteration %d: cache missed a change", i)
		}
	}
}

// TestLabelInputGenConfigReload: every config label reload path bumps
// labelInputGen, and a reload with unchanged workloads reaches both caches.
func TestLabelInputGenConfigReload(t *testing.T) {
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	hc, _ := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	pc.refreshOnce()
	cfg := &exportermetrics.GPUMetricConfig{CustomLabels: map[string]string{"CLUSTER_NAME": "reloaded"}}
	for name, fn := range map[string]func(){
		"initCustomLabels": func() { e.gpu.initCustomLabels(cfg) },
		"initLabelConfigs": func() {
			e.gpu.initLabelConfigs(&exportermetrics.GPUMetricConfig{Labels: []string{"GPU_UUID", "POD", "CLUSTER_NAME"}})
		},
		"InitPodExtraLabels":       func() { e.gpu.InitPodExtraLabels(nil) },
		"PopulateStaticHostLabels": func() { _ = e.gpu.PopulateStaticHostLabels() },
	} {
		g := e.gpu.labelInputGen.Load()
		fn()
		assert.Equal(t, e.gpu.labelInputGen.Load(), g+1, name)
	}
	e.gpu.initCustomLabels(cfg)
	for _, c := range []*workloadLabelCache{hc, pc} {
		c.refreshOnce()
		for _, l := range *c.labels.Load() {
			assert.Equal(t, l["cluster_name"], "reloaded")
		}
	}
}

// TestLabelInputGenPodInfo: identical pod info on every scrape does not bump
// labelInputGen; a changed pod label bumps it once, both caches rebuild once,
// the hot cache rebuilds once and the hot sample carries the new label.
func TestLabelInputGenPodInfo(t *testing.T) {
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	e.gpu.InitPodExtraLabels(&exportermetrics.GPUMetricConfig{ExtraPodLabels: map[string]string{"APP": "app"}})
	hc, _ := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	pc.refreshOnce()
	e.gpu.hrFastMs.Store(10)
	gpus := testGPUIDs()
	frame := hrPartial(gpus, hrPowerTick(100)...)
	e.gpu.applyPowerFrame(frame)

	key := (&types.PodUniqueKey{PodName: "pod0", Namespace: "ns0"}).String()
	info := func(app string) map[string]types.K8sPodInfo {
		return map[string]types.K8sPodInfo{key: {Name: "pod0", Namespace: "ns0", Labels: map[string]string{"app": app}}}
	}
	e.gpu.setK8PodInfoMap(info("v0"))
	hc.refreshOnce()
	pc.refreshOnce()
	g, r := e.gpu.labelInputGen.Load(), hc.rebuilds.Load()
	for i := 0; i < 100; i++ {
		e.gpu.setK8PodInfoMap(info("v0"))
		hc.refreshOnce()
	}
	assert.Equal(t, e.gpu.labelInputGen.Load(), g)
	assert.Equal(t, hc.rebuilds.Load(), r)
	e.gpu.applyPowerFrame(frame)

	for _, v := range []string{"v1", "v2"} {
		g := e.gpu.labelInputGen.Load()
		hot := e.gpu.hrHotRebuilds.Load()
		e.gpu.setK8PodInfoMap(info(v))
		assert.Equal(t, e.gpu.labelInputGen.Load(), g+1)
		hr, pr := hc.rebuilds.Load(), pc.rebuilds.Load()
		hc.refreshOnce()
		pc.refreshOnce()
		assert.Equal(t, hc.rebuilds.Load(), hr+1, "hr cache did not rebuild for %s", v)
		assert.Equal(t, pc.rebuilds.Load(), pr+1, "profiler cache did not rebuild for %s", v)
		e.gpu.applyPowerFrame(frame)
		assert.Equal(t, e.gpu.hrHotRebuilds.Load(), hot+1, "hot cache did not pick the new labels")
		s, ok := hrGet(e.gpu, gpus[0], "gpu_package_power")
		assert.Assert(t, ok)
		assert.Equal(t, s.Labels["app"], v, "hot sample lacks app=%s", v)
	}
}

// tickHookKey counts a key's tick upkeep calls.
type tickHookKey struct {
	labelCacheKey
	ticks *atomic.Int64
}

func (k tickHookKey) tick(ga *GPUAgentGPUClient) {
	k.ticks.Add(1)
	k.labelCacheKey.tick(ga)
}

// TestLabelCacheStartLoop: start seeds synchronously, wakes on a source change
// with a 1h ticker, runs the key's tick upkeep on its ticker and stops on
// cancel.
func TestLabelCacheStartLoop(t *testing.T) {
	for _, key := range labelCacheKeys {
		t.Run(fmt.Sprintf("%T", key), func(t *testing.T) {
			e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
			_, _ = e.startHRSource(t, 5*time.Millisecond)
			gpu2 := testGPUResp(testGPUCount).Response[2]
			for _, interval := range []time.Duration{time.Hour, 5 * time.Millisecond} {
				c := newLabelCache(e.gpu, key)
				ticks := &atomic.Int64{}
				c.keys = tickHookKey{labelCacheKey: key, ticks: ticks}
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() { c.start(ctx, interval); close(done) }()
				assert.Assert(t, waitWithin(time.Second, func() bool { return c.labels.Load() != nil }),
					"start did not seed")
				if interval == time.Hour {
					want := fmt.Sprint("p", rand.Int())
					e.k8s.set(map[string]scheduler.Workload{"0000:03:00.0": {Type: scheduler.Kubernetes,
						Info: scheduler.PodResourceInfo{Pod: want}}}, nil)
					assert.Assert(t, waitWithin(time.Second, func() bool {
						l, _ := c.get(key.key(gpu2))
						return l["pod"] == want
					}), "a source change did not wake the cache")
					assert.Equal(t, ticks.Load(), int64(0))
				} else {
					assert.Assert(t, waitWithin(time.Second, func() bool { return ticks.Load() > 0 }),
						"the ticker must run the key's upkeep")
					if _, ok := key.(renderIDKey); ok {
						assertTickDropsDisabledProfilerFields(t, e)
					}
				}
				cancel()
				<-done
			}
		})
	}
}

// assertTickDropsDisabledProfilerFields: once the profiler is failure-disabled,
// the render-id key's tick removes the cached gpu_prof_* fields from the store.
func assertTickDropsDisabledProfilerFields(t *testing.T, e *workloadEnv) {
	t.Helper()
	assert.Assert(t, e.gpu.rocpclient != nil)
	var field string
	for f := range highResProfilerFieldSet {
		field = f
		break
	}
	store := e.gpu.gpuHandler.mh.GetStore()
	store.Update("prof-gpu", map[string]snapshotstore.Sample{field: {Value: 1}})
	has := func() bool { _, ok := store.Snapshot("prof-gpu").Get(field); return ok }
	assert.Assert(t, has())
	for i := 0; i < 3; i++ {
		e.gpu.rocpclient.IncFailureCount(context.Background())
	}
	assert.Assert(t, e.gpu.rocpclient.IsDisabledOnFailure())
	assert.Assert(t, waitWithin(time.Second, func() bool { return !has() }),
		"tick did not drop the disabled profiler's %s", field)
}

// TestStartHighResSeedsFromSource: StartHighRes ensures the source, seeds from
// it with a single kubelet List, and the first labels are workload-aware.
func TestStartHighResSeedsFromSource(t *testing.T) {
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	e.gpu.wlInterval = time.Hour
	t.Cleanup(e.gpu.stopWorkloadSource)
	watch := mock_gen.NewMockGPUWatchSvcClient(gomock.NewController(t))
	errStop := errors.New("stop after create")
	watch.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).Return(&amdgpu.GPUWatchGetResponse{}, nil).AnyTimes()
	watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
		Return(&amdgpu.GPUWatchResponse{ApiStatus: amdgpu.ApiStatus_API_STATUS_OK}, nil).AnyTimes()
	watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).Return(nil, errStop).AnyTimes()
	e.gpu.watchclient = watch
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	before, _ := e.k8s.counts()
	assert.Equal(t, e.gpu.StartHighRes(ctx), errStop)
	after, _ := e.k8s.counts()
	src := e.gpu.wlSource.Load()
	assert.Assert(t, src != nil, "StartHighRes did not ensure the source")
	assert.Equal(t, after-before, 1, "seed must be the source's single List")
	pods := 0
	for _, l := range e.gpu.hrLabelsByUUID {
		if l["pod"] != "" {
			pods++
		}
	}
	assert.Equal(t, pods, 2)
}

// TestLabelCacheFailedThenOK: before a snapshot the cache stays nil; a failed
// snapshot builds the empty-map labels (one hot cache rebuild for the drop);
// the next ok snapshot restores labels equal to a fresh pull-path build (one
// hot cache rebuild for the restore).
func TestLabelCacheFailedThenOK(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newWorkloadEnv(t, hrTestConfig, fx)
	hc := newHRWorkloadLabelCache(e.gpu)
	hc.refreshOnce() // no hrSnapshot yet
	assert.Assert(t, hc.labels.Load() == nil)
	hc, src := e.startHRSource(t, 0)
	assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(fx.k8s, getGPUUUID)))
	e.gpu.hrFastMs.Store(10)
	frame := hrPartial(testGPUIDs(), hrPowerTick(100)...)
	e.gpu.applyPowerFrame(frame) // warm the hot cache
	hot := e.gpu.hrHotRebuilds.Load()

	e.k8s.set(fx.k8s, errKubeletUnavailable)
	src.refresh(true)
	assert.Assert(t, src.load().err != nil)
	hc.refreshOnce()
	assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(map[string]scheduler.Workload{}, getGPUUUID)))
	e.gpu.applyPowerFrame(frame)
	assert.Equal(t, e.gpu.hrHotRebuilds.Load(), hot+1, "hot cache must rebuild once for the drop")

	e.k8s.set(fx.k8s, nil)
	src.refresh(true)
	hc.refreshOnce()
	assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(fx.k8s, getGPUUUID)))
	e.gpu.applyPowerFrame(frame)
	e.gpu.applyPowerFrame(frame)
	assert.Equal(t, e.gpu.hrHotRebuilds.Load(), hot+2, "hot cache must rebuild once for the restore")
}

// TestLabelInputGenWriteRacesRefresh: concurrent label-input writes race
// refreshOnce; after quiesce one more refresh equals a fresh build.
func TestLabelInputGenWriteRacesRefresh(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(e *workloadEnv, i int)
	}{
		{"custom labels", func(e *workloadEnv, i int) {
			e.gpu.initCustomLabels(&exportermetrics.GPUMetricConfig{
				CustomLabels: map[string]string{"CLUSTER_NAME": fmt.Sprint("c", i)}})
		}},
		{"pod info", func(e *workloadEnv, i int) {
			e.gpu.setK8PodInfoMap(map[string]types.K8sPodInfo{fmt.Sprint("pod", i): {}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
			hc, _ := e.startHRSource(t, 0)
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				rng := rand.New(rand.NewSource(1))
				for i := 0; i < 200; i++ {
					tc.write(e, i)
					time.Sleep(time.Duration(rng.Intn(50)) * time.Microsecond)
				}
			}()
			stop := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					select {
					case <-stop:
						return
					default:
						hc.refreshOnce()
					}
				}
			}()
			wg.Wait()
			close(stop)
			<-done
			hc.refreshOnce()
			ws := e.gpu.wlSource.Load().load()
			assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(ws.wls, getGPUUUID)),
				"cache labels stale after concurrent label-input writes")
		})
	}
}

// TestLabelCacheZeroOneGPU: 0 GPUs leave the labels nil; 1 GPU builds one
// workload-aware entry, for every key.
func TestLabelCacheZeroOneGPU(t *testing.T) {
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	runningSource(t, e, time.Hour)
	one := testGPUResp(1).Response
	for _, key := range labelCacheKeys {
		c := newLabelCache(e.gpu, key)
		e.gpu.hrSnapshot.Store(&hrGPUSnap{})
		c.refreshOnce()
		assert.Assert(t, c.labels.Load() == nil)
		assert.Equal(t, c.built, labelInputs{})
		e.gpu.hrSnapshot.Store(&hrGPUSnap{gpus: one, partitionMap: map[string]*amdgpu.GPU{}})
		c.refreshOnce()
		assert.Equal(t, len(*c.labels.Load()), 1)
		l, _ := c.get(key.key(one[0]))
		assert.Equal(t, l["pod"], "pod0")
	}
}

// TestLabelInputGenGPUMetadata: re-running initGPUMetadata with the same GPUs
// does not bump labelInputGen; a changed GPU id mapping does.
func TestLabelInputGenGPUMetadata(t *testing.T) {
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	e.gpu.initGPUMetadata(testGPUResp(testGPUCount).Response)
	g := e.gpu.labelInputGen.Load()
	e.gpu.initGPUMetadata(testGPUResp(testGPUCount).Response)
	assert.Equal(t, e.gpu.labelInputGen.Load(), g)
	moved := testGPUResp(testGPUCount).Response
	moved[0].Status.PCIeStatus.PCIeBusId = "0000:0a:00.0"
	e.gpu.initGPUMetadata(moved)
	assert.Equal(t, e.gpu.labelInputGen.Load(), g+1)
}

// TestLabelCacheConcurrentGet: lock-free get() readers against a refresher
// that swaps maps never observe a torn or partially built map.
func TestLabelCacheConcurrentGet(t *testing.T) {
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	hc, src := e.startHRSource(t, 0)
	uuid0 := getGPUUUID(testGPUResp(testGPUCount).Response[0])
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ctx.Err() == nil; i++ {
			e.k8s.set(map[string]scheduler.Workload{"0000:01:00.0": testPod(i % 2)}, nil)
			src.refresh(true)
			hc.refreshOnce()
		}
	}()
	var bad atomic.Int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				l, ok := hc.get(uuid0)
				if !ok || l["gpu_uuid"] != uuid0 || (l["pod"] != "pod0" && l["pod"] != "pod1") {
					bad.Add(1)
				}
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
	assert.Equal(t, bad.Load(), int64(0))
}
