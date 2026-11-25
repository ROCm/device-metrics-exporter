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

// GPUOP-1140 label caches: rebuild only on change, per-GPU map reuse, hot
// cache, label-input generation, StartHighRes ordering and seed (test plan
// cases 9-13, 22-27, 34, 53, 68, 70, 71, 77).

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/types"
)

func mapPtr(m prometheus.Labels) uintptr { return reflect.ValueOf(m).Pointer() }

// TestRebuildLabelMapReuse (case 10).
func TestRebuildLabelMapReuse(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("slurm"))
	_, _ = e.startHRSource(t, 0)
	snap := e.gpu.hrSnapshot.Load()
	wls := fixtureByName("slurm").slurm
	for _, key := range []func(*amdgpu.GPU) string{getGPUUUID, getGPURenderId} {
		prev := e.gpu.rebuildLabelMap(nil, snap.gpus, snap.partitionMap, wls, key)
		next := e.gpu.rebuildLabelMap(prev, snap.gpus, snap.partitionMap, maps.Clone(wls), key)
		assert.Assert(t, next == prev, "equal content must return prev")
		for k, l := range *prev {
			assert.Equal(t, mapPtr((*next)[k]), mapPtr(l))
		}
		m := maps.Clone(wls)
		m["2"] = oracleJob("999", "bob")
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

// TestLabelCacheNoRebuildWhenUnchanged (case 11): N unchanged refreshes keep
// the pointer, rebuilds stays 0 and a no-op refresh allocates nothing.
func TestLabelCacheNoRebuildWhenUnchanged(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("mixed"))
	hc, src := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	pc.refreshOnce()
	hp, pp := hc.labels.Load(), pc.labels.Load()
	hr, pr := hc.rebuilds.Load(), pc.rebuilds.Load()
	for i := 0; i < 100; i++ {
		src.refresh(true)
		hc.refreshOnce(context.Background())
		pc.refreshOnce()
	}
	assert.Assert(t, hc.labels.Load() == hp && pc.labels.Load() == pp)
	assert.Equal(t, hc.rebuilds.Load(), hr)
	assert.Equal(t, pc.rebuilds.Load(), pr)
	allocs := testing.AllocsPerRun(100, func() {
		hc.refreshOnce(context.Background())
		pc.refreshOnce()
	})
	assert.Equal(t, allocs, float64(0))
}

// TestHotCacheNoRebuildUnchangedLabels (case 12): partial power frames over N
// unchanged source + cache refreshes rebuild the GPUOP-1137 hot cache 0 times.
func TestHotCacheNoRebuildUnchangedLabels(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("slurm"))
	e.ga.slurmScheduler = newNotifySched(fixtureByName("slurm").slurm)
	hc, src := e.startHRSource(t, 0)
	e.gpu.hrFastMs.Store(10) // sub-second: attr 126 feeds gpu_package_power hot cells
	frame := hrPartial(oracleGPUIDs(), hrPowerTick(100)...)
	e.gpu.applyPowerFrame(frame)
	base := e.gpu.hrHotRebuilds.Load()
	for i := 0; i < 100; i++ {
		src.refresh(true)
		hc.refreshOnce(context.Background())
		e.gpu.applyPowerFrame(frame)
	}
	assert.Equal(t, e.gpu.hrHotRebuilds.Load(), base, "hot cache rebuilt with unchanged labels")
}

// TestLabelCacheOneGPUChange (case 13): a job change on one GPU replaces only
// that GPU's map; the hot cache rebuilds once and keeps the other cells.
func TestLabelCacheOneGPUChange(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("slurm"))
	slurm := newNotifySched(fixtureByName("slurm").slurm)
	e.ga.slurmScheduler = slurm
	hc, src := e.startHRSource(t, 0)
	e.gpu.hrFastMs.Store(10) // sub-second: attr 126 feeds gpu_package_power hot cells
	frame := hrPartial(oracleGPUIDs(), hrPowerTick(100)...)
	e.gpu.applyPowerFrame(frame)
	before := maps.Clone(*hc.labels.Load())
	oldHot := e.gpu.hrHot
	base := e.gpu.hrHotRebuilds.Load()
	m := maps.Clone(fixtureByName("slurm").slurm)
	m["3"] = oracleJob("333", "zoe")
	slurm.set(m)
	src.refresh(false)
	hc.refreshOnce(context.Background())
	after := *hc.labels.Load()
	changed := []string{}
	for k, l := range after {
		if mapPtr(before[k]) != mapPtr(l) {
			changed = append(changed, k)
		}
	}
	assert.Equal(t, len(changed), 1)
	assert.Equal(t, after[changed[0]]["job_id"], "333")
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
	assert.Equal(t, kept, oracleGPUs-1, "unaffected GPUs must keep their hot cells")
}

// TestLabelCacheSlurmEventLatency (case 9): with cache and source intervals at
// 1h, a real slurm job create/remove reaches both caches within 100ms.
func TestLabelCacheSlurmEventLatency(t *testing.T) {
	dir := t.TempDir()
	cl, err := scheduler.NewSlurmClientForDir(context.Background(), dir)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	e := newOracleEnv(t, oracleHRConfig, oracleFixture{name: "none"})
	e.ga.slurmScheduler = cl
	hc, _ := e.startHRSource(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hc.start(ctx, time.Hour)
	pc := newProfilerLabelCache(e.gpu)
	go pc.start(ctx, time.Hour)
	uuid2 := getGPUUUID(oracleGPUResp(oracleGPUs).Response[2])
	has := func(id string) bool {
		h, ok1 := hc.get(uuid2)
		p, ok2 := pc.get("130")
		return ok1 && ok2 && h["job_id"] == id && p["job_id"] == id
	}
	time.Sleep(10 * time.Millisecond)
	for i := 0; i < 3; i++ {
		id := fmt.Sprint(800 + i)
		writeSlurmJob(t, dir, "2", id, "2")
		assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { return has(id) }), "create %d", i)
		assert.NilError(t, os.Remove(dir+"/2"))
		assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { return has("") }), "remove %d", i)
	}
	h, _ := hc.get(uuid2)
	assert.Assert(t, maps.Equal(h, e.freshLabels(map[string]scheduler.Workload{}, getGPUUUID)[uuid2]))
}

// TestLabelCacheNoLostWakeup (case 68): only Changed() wakes the caches (1h
// ticker); bumps at random offsets around the refresh are never lost.
func TestLabelCacheNoLostWakeup(t *testing.T) {
	iters := 1000
	if testing.Short() {
		iters = 100
	}
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewSource(seed))
	e := newOracleEnv(t, oracleHRConfig, oracleFixture{name: "none"})
	f := newNotifySched(map[string]scheduler.Workload{})
	e.ga.slurmScheduler = f
	hc, _ := e.startHRSource(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hc.start(ctx, time.Hour)
	pc := newProfilerLabelCache(e.gpu)
	go pc.start(ctx, time.Hour)
	uuid0 := getGPUUUID(oracleGPUResp(oracleGPUs).Response[0])
	has := func(id string) bool {
		h, ok1 := hc.get(uuid0)
		p, ok2 := pc.get("128")
		return ok1 && ok2 && h["job_id"] == id && p["job_id"] == id
	}
	for i := 1; i <= iters; i++ {
		f.set(map[string]scheduler.Workload{"0": oracleJob(fmt.Sprint(i), "u")})
		time.Sleep(time.Duration(rng.Intn(1500)) * time.Microsecond)
		id := fmt.Sprint(-i)
		f.set(map[string]scheduler.Workload{"0": oracleJob(id, "u")})
		if !waitWithin(wakeBound, func() bool { return has(id) }) {
			t.Fatalf("iteration %d: cache missed a change", i)
		}
	}
}

// TestLabelInputGenConfigReload (case 24): a config label reload with unchanged
// workloads bumps labelInputGen and the caches pick up the new label.
func TestLabelInputGenConfigReload(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("slurm"))
	hc, _ := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	pc.refreshOnce()
	cfg := &exportermetrics.GPUMetricConfig{CustomLabels: map[string]string{"CLUSTER_NAME": "reloaded"}}
	for name, fn := range map[string]func(){
		"initCustomLabels": func() { e.gpu.initCustomLabels(cfg) },
		"initLabelConfigs": func() {
			e.gpu.initLabelConfigs(&exportermetrics.GPUMetricConfig{Labels: []string{"GPU_UUID", "JOB_ID", "CLUSTER_NAME"}})
		},
		"InitPodExtraLabels":       func() { e.gpu.InitPodExtraLabels(nil) },
		"PopulateStaticHostLabels": func() { _ = e.gpu.PopulateStaticHostLabels() },
	} {
		g := e.gpu.labelInputGen.Load()
		fn()
		assert.Equal(t, e.gpu.labelInputGen.Load(), g+1, name)
	}
	e.gpu.initCustomLabels(cfg)
	hc.refreshOnce(context.Background())
	pc.refreshOnce()
	for _, l := range *hc.labels.Load() {
		assert.Equal(t, l["cluster_name"], "reloaded")
	}
	for _, l := range *pc.labels.Load() {
		assert.Equal(t, l["cluster_name"], "reloaded")
	}
}

// TestLabelInputGenPodInfo (cases 25, 26): identical pod info on every scrape
// does not bump; a change bumps once and the caches rebuild.
func TestLabelInputGenPodInfo(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("k8s"))
	hc, _ := e.startHRSource(t, 0)
	info := map[string]types.K8sPodInfo{"pod0": {}}
	e.gpu.setK8PodInfoMap(info)
	g := e.gpu.labelInputGen.Load()
	hc.refreshOnce(context.Background())
	r := hc.rebuilds.Load()
	for i := 0; i < 1000; i++ {
		e.gpu.setK8PodInfoMap(map[string]types.K8sPodInfo{"pod0": {}})
		hc.refreshOnce(context.Background())
	}
	assert.Equal(t, e.gpu.labelInputGen.Load(), g)
	assert.Equal(t, hc.rebuilds.Load(), r)
	e.gpu.setK8PodInfoMap(map[string]types.K8sPodInfo{"pod1": {}})
	assert.Equal(t, e.gpu.labelInputGen.Load(), g+1)
	hc.refreshOnce(context.Background())
	assert.Equal(t, hc.built.labelGen, g+1, "cache did not re-check after a pod-info change")
}

// TestProfilerCacheStartLoop (case 27): start seeds synchronously, wakes on a
// source change and keeps calling dropProfilerFieldsIfDisabled on its tick.
func TestProfilerCacheStartLoop(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, oracleFixture{name: "none"})
	f := newNotifySched(map[string]scheduler.Workload{})
	e.ga.slurmScheduler = f
	_, _ = e.startHRSource(t, time.Hour)
	pc := newProfilerLabelCache(e.gpu)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { pc.start(ctx, 5*time.Millisecond); close(done) }()
	assert.Assert(t, waitWithin(time.Second, func() bool { return pc.labels.Load() != nil }))
	f.set(map[string]scheduler.Workload{"1": oracleJob("41", "u")})
	assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { l, _ := pc.get("129"); return l["job_id"] == "41" }))
	cancel()
	<-done
	src, err := os.ReadFile("gpuagent_gpu_profiler_stream.go")
	assert.NilError(t, err)
	body := string(src)
	loop := body[strings.Index(body, "func (c *profilerLabelCache) start("):]
	loop = loop[:strings.Index(loop, "\n}\n")]
	assert.Assert(t, strings.Contains(loop, "case <-ticker.C:\n\t\t\tc.refreshOnce()\n\t\t\tc.ga.dropProfilerFieldsIfDisabled()"),
		"tick must still drop disabled profiler fields")
}

// TestStartHighResMetadataBeforeSnapshot (case 22): order assertion on the
// source, initGPUMetadata before hrSnapshot.Store in StartHighRes.
func TestStartHighResMetadataBeforeSnapshot(t *testing.T) {
	src, err := os.ReadFile("gpuagent_gpu_highres.go")
	assert.NilError(t, err)
	body := string(src)
	fn := body[strings.Index(body, "func (ga *GPUAgentGPUClient) StartHighRes("):]
	meta := strings.Index(fn, "ga.initGPUMetadata(getResp.GetResponse())")
	store := strings.Index(fn, "ga.hrSnapshot.Store(")
	assert.Assert(t, meta > 0 && store > 0 && meta < store, "initGPUMetadata must run before hrSnapshot.Store")
}

// TestStartHighResSeedsFromSource (case 23): StartHighRes ensures the source,
// seeds from it (no extra kubelet call) and the first labels are workload-aware.
func TestStartHighResSeedsFromSource(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("k8s"))
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

// TestLabelCacheFailedThenOK (case 34): before a snapshot the cache stays nil;
// a failed snapshot builds the empty-map labels, the next ok one restores.
func TestLabelCacheFailedThenOK(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newOracleEnv(t, oracleHRConfig, fx)
	hc := newHRWorkloadLabelCache(e.gpu)
	hc.refreshOnce(context.Background()) // no hrSnapshot yet
	assert.Assert(t, hc.labels.Load() == nil)
	e.k8s.set(fx.k8s, errOracleKubelet)
	hc, src := e.startHRSource(t, 0)
	assert.Assert(t, src.load().err != nil)
	assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(map[string]scheduler.Workload{}, getGPUUUID)))
	e.k8s.set(fx.k8s, nil)
	src.refresh(true)
	hc.refreshOnce(context.Background())
	assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(fx.k8s, getGPUUUID)))
}

// TestLabelInputGenWriteRacesRefresh (case 70) and TestPodInfoWriteRacesRefresh
// (case 71): concurrent label-input writes against refreshOnce; after quiesce
// one more refresh equals a fresh build.
func TestLabelInputGenWriteRacesRefresh(t *testing.T) {
	raceLabelInputs(t, func(e *oracleEnv, i int) {
		e.gpu.initCustomLabels(&exportermetrics.GPUMetricConfig{
			CustomLabels: map[string]string{"CLUSTER_NAME": fmt.Sprint("c", i)}})
	})
}

func TestPodInfoWriteRacesRefresh(t *testing.T) {
	raceLabelInputs(t, func(e *oracleEnv, i int) {
		e.gpu.setK8PodInfoMap(map[string]types.K8sPodInfo{fmt.Sprint("pod", i): {}})
	})
}

func raceLabelInputs(t *testing.T, write func(e *oracleEnv, i int)) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("mixed"))
	hc, _ := e.startHRSource(t, 0)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rng := rand.New(rand.NewSource(1))
		for i := 0; i < 1000; i++ {
			write(e, i)
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
				hc.refreshOnce(context.Background())
			}
		}
	}()
	wg.Wait()
	close(stop)
	<-done
	hc.refreshOnce(context.Background())
	ws := e.gpu.wlSource.Load().load()
	assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(ws.wls, getGPUUUID)),
		"cache labels stale after concurrent label-input writes")
}

// TestLabelCacheZeroOneGPU (case 77).
func TestLabelCacheZeroOneGPU(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("slurm"))
	runningSource(t, e, time.Hour)
	hc := newHRWorkloadLabelCache(e.gpu)
	pc := newProfilerLabelCache(e.gpu)
	e.gpu.hrSnapshot.Store(&hrGPUSnap{})
	hc.refreshOnce(context.Background())
	pc.refreshOnce()
	assert.Assert(t, hc.labels.Load() == nil && pc.labels.Load() == nil)
	assert.Equal(t, hc.built, labelInputs{})
	one := oracleGPUResp(1).Response
	e.gpu.hrSnapshot.Store(&hrGPUSnap{gpus: one, partitionMap: map[string]*amdgpu.GPU{}})
	hc.refreshOnce(context.Background())
	pc.refreshOnce()
	assert.Equal(t, len(*hc.labels.Load()), 1)
	assert.Equal(t, len(*pc.labels.Load()), 1)
	l, _ := hc.get(getGPUUUID(one[0]))
	assert.Equal(t, l["job_id"], "100")
}

// TestLabelInputGenGPUMetadata: re-running initGPUMetadata with the same GPUs
// does not bump labelInputGen; a changed GPU id mapping does (review finding:
// gpuIDMap feeds workload matching).
func TestLabelInputGenGPUMetadata(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("k8s"))
	gpus := oracleGPUResp(oracleGPUs).Response
	e.gpu.initGPUMetadata(gpus)
	g := e.gpu.labelInputGen.Load()
	e.gpu.initGPUMetadata(oracleGPUResp(oracleGPUs).Response)
	assert.Equal(t, e.gpu.labelInputGen.Load(), g)
	moved := oracleGPUResp(oracleGPUs).Response
	moved[0].Status.PCIeStatus.PCIeBusId = "0000:0a:00.0"
	e.gpu.initGPUMetadata(moved)
	assert.Equal(t, e.gpu.labelInputGen.Load(), g+1)
}
