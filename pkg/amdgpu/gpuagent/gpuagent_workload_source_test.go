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

// GPUOP-1140 workload source unit tests (test plan cases 3-5, 7, 8, 19-21,
// 28-31, 33, 35-38, 42-48, 67, 69, 82-84).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

var errSlurmTest = errors.New("slurm list failed")

// legacyListWorkloads is base b2efd1b32's GPUAgentClient.ListWorkloads body,
// kept verbatim as the oracle for mergeWorkloads.
func legacyListWorkloads(isK8s bool, k8s, slurm scheduler.SchedulerClient) (wls map[string]scheduler.Workload, err error) {
	wls = make(map[string]scheduler.Workload)
	if isK8s && k8s != nil {
		var k8sWls map[string]scheduler.Workload
		k8sWls, err = k8s.ListWorkloads()
		if err != nil {
			return
		}
		for k, wl := range k8sWls {
			wls[k] = wl
		}
	}
	if slurm == nil {
		return wls, nil
	}
	var swls map[string]scheduler.Workload
	swls, err = slurm.ListWorkloads()
	if err != nil {
		return
	}
	for k, wl := range swls {
		wls[k] = wl
	}
	return
}

// TestMergeWorkloadsMatchesLegacy (cases 5, 84): truth table over k8sOn x k8s
// {nil, ok, err} x slurm {nil, ok, err} with overlapping and disjoint keys.
func TestMergeWorkloadsMatchesLegacy(t *testing.T) {
	mixed := fixtureByName("mixed")
	for _, k8sOn := range []bool{false, true} {
		for _, k8sMode := range []string{"nil", "ok", "err"} {
			for _, slurmMode := range []string{"nil", "ok", "err"} {
				name := fmt.Sprintf("k8sOn=%v/k8s=%s/slurm=%s", k8sOn, k8sMode, slurmMode)
				var k8s, slurm scheduler.SchedulerClient
				if k8sMode != "nil" {
					f := &oracleSched{wls: mixed.k8s}
					if k8sMode == "err" {
						f.err = errOracleKubelet
					}
					k8s = f
				}
				if slurmMode != "nil" {
					f := &oracleSched{wls: mixed.slurm}
					if slurmMode == "err" {
						f.err = errSlurmTest
					}
					slurm = f
				}
				want, wantErr := legacyListWorkloads(k8sOn, k8s, slurm)

				ga := &GPUAgentClient{isKubernetes: k8sOn, k8sScheduler: k8s, slurmScheduler: slurm}
				got, gotErr := ga.ListWorkloads()
				assert.Assert(t, got != nil, name)
				assert.Assert(t, reflect.DeepEqual(got, want), "%s: ListWorkloads %v want %v", name, got, want)
				assert.Equal(t, fmt.Sprint(gotErr), fmt.Sprint(wantErr), name)

				var kw, sw map[string]scheduler.Workload
				var ke, se error
				if k8s != nil {
					kw, ke = k8s.ListWorkloads()
				}
				if slurm != nil {
					sw, se = slurm.ListWorkloads()
				}
				got, gotErr = mergeWorkloads(k8sOn && k8s != nil, kw, ke, slurm != nil, sw, se)
				assert.Assert(t, got != nil, name)
				assert.Assert(t, reflect.DeepEqual(got, want), "%s: merge %v want %v", name, got, want)
				assert.Equal(t, fmt.Sprint(gotErr), fmt.Sprint(wantErr), name)
			}
		}
	}
	// merge order: on the shared key "3" slurm overwrites k8s
	ga := &GPUAgentClient{isKubernetes: true, k8sScheduler: &oracleSched{wls: mixed.k8s},
		slurmScheduler: &oracleSched{wls: mixed.slurm}}
	wls, err := ga.ListWorkloads()
	assert.NilError(t, err)
	assert.Equal(t, wls["3"].Type, scheduler.Slurm)
	assert.Equal(t, len(wls), 4)
}

// TestWorkloadSourceUnchangedSamePointer (case 3).
func TestWorkloadSourceUnchangedSamePointer(t *testing.T) {
	for _, name := range []string{"k8s", "slurm", "mixed"} {
		t.Run(name, func(t *testing.T) {
			e := newOracleEnv(t, oraclePullConfig, fixtureByName(name))
			src := manualSource(e)
			first := src.load()
			assert.Equal(t, first.gen, uint64(1))
			ch := src.Changed()
			for i := 0; i < 100; i++ {
				src.refresh(true)
			}
			assert.Assert(t, src.load() == first, "unchanged refresh published a new snapshot")
			select {
			case <-ch:
				t.Fatal("Changed closed without a change")
			default:
			}
		})
	}
}

// TestWorkloadSourceGenBump (cases 4, 36, 37): +1 per content change, published
// maps never mutated, empty transitions.
func TestWorkloadSourceGenBump(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("slurm"))
	slurm := newNotifySched(fixtureByName("slurm").slurm)
	e.ga.slurmScheduler = slurm
	src := manualSource(e)
	assert.Equal(t, src.load().gen, uint64(1))

	step := func(wls map[string]scheduler.Workload, wantGen uint64) {
		t.Helper()
		prev := src.load()
		prevCopy := maps.Clone(prev.wls)
		ch := src.Changed()
		slurm.set(wls)
		src.refresh(false)
		cur := src.load()
		assert.Equal(t, cur.gen, wantGen)
		assert.Assert(t, maps.Equal(prev.wls, prevCopy), "published map mutated")
		if wantGen != prev.gen {
			<-ch
			assert.Assert(t, maps.Equal(cur.wls, wls) && cur.wls != nil)
		}
	}
	m := maps.Clone(fixtureByName("slurm").slurm)
	m["5"] = oracleJob("105", "eve")
	step(m, 2) // added
	delete(m, "5")
	step(maps.Clone(m), 3) // removed
	m["0"] = oracleJob("100", "alice2")
	step(maps.Clone(m), 4) // field change
	step(maps.Clone(m), 4) // same content, new version: no bump
	step(map[string]scheduler.Workload{}, 5)
	step(map[string]scheduler.Workload{}, 5) // empty -> empty

	// random sequences: gen strictly +1 per merged-content change
	rng := rand.New(rand.NewSource(1))
	gen := src.load().gen
	cur := map[string]scheduler.Workload{}
	for i := 0; i < 10000; i++ {
		next := maps.Clone(cur)
		k := fmt.Sprint(rng.Intn(4))
		if rng.Intn(2) == 0 {
			delete(next, k)
		} else {
			next[k] = oracleJob(fmt.Sprint(rng.Intn(3)), "u")
		}
		if !maps.Equal(next, cur) {
			gen++
		}
		cur = next
		slurm.set(maps.Clone(next))
		src.refresh(false)
		assert.Equal(t, src.load().gen, gen)
	}
}

// TestWorkloadSourceK8sNoChangeChange (case 7): real kubelet client over the
// fake PodResources server.
func TestWorkloadSourceK8sNoChangeChange(t *testing.T) {
	fk := newFakeKubelet(t, kubeResp("0000:01:00.0"))
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("k8s"))
	e.ga.k8sScheduler = fk.client(t)
	src := manualSource(e)
	first := src.load()
	calls := src.k8sCalls.Load()
	for i := 0; i < 5; i++ {
		src.refresh(true)
	}
	assert.Assert(t, src.load() == first)
	assert.Equal(t, src.k8sCalls.Load(), calls+5)
	assert.Equal(t, fk.calls.Load(), src.k8sCalls.Load())
	ch := src.Changed()
	fk.set(kubeResp("0000:02:00.0"), nil) // pod moved
	src.refresh(true)
	<-ch
	assert.Equal(t, src.load().gen, first.gen+1)
	_, ok := src.load().wls["0000:02:00.0"]
	assert.Assert(t, ok)
}

// TestWorkloadSourceK8sErrorDropsLikeToday (case 28): mixed fixture; the failed
// snapshot equals legacy ListWorkloads (empty, slurm not merged); hr and
// profiler labels drop exactly as a fresh build with the empty map.
func TestWorkloadSourceK8sErrorDropsLikeToday(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newOracleEnv(t, oracleHRConfig, fx)
	hc, src := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	pc.refreshOnce()
	gen := src.load().gen
	ch := src.Changed()
	e.k8s.set(fx.k8s, errOracleKubelet)
	src.refresh(true)
	<-ch
	ws := src.load()
	legacy, legacyErr := e.ga.ListWorkloads()
	assert.Assert(t, ws.err != nil)
	assert.Equal(t, ws.err.Error(), legacyErr.Error())
	assert.Assert(t, ws.wls != nil && len(ws.wls) == 0 && reflect.DeepEqual(ws.wls, legacy))
	assert.Equal(t, ws.gen, gen+1)
	hc.refreshOnce()
	pc.refreshOnce()
	assert.Assert(t, reflect.DeepEqual(*hc.labels.Load(), e.freshLabels(map[string]scheduler.Workload{}, getGPUUUID)))
	assert.Assert(t, reflect.DeepEqual(*pc.labels.Load(), e.freshLabels(map[string]scheduler.Workload{}, getGPURenderId)))
	for _, l := range *hc.labels.Load() {
		assert.Equal(t, l["job_id"], "")
		assert.Equal(t, l["pod"], "")
	}
}

// TestWorkloadSourceSlurmErrorMatchesLegacy (case 29).
func TestWorkloadSourceSlurmErrorMatchesLegacy(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newOracleEnv(t, oraclePullConfig, fx)
	src := manualSource(e)
	gen := src.load().gen
	e.slurm.set(fx.slurm, errSlurmTest)
	src.refresh(false)
	ws := src.load()
	legacy, legacyErr := e.ga.ListWorkloads()
	assert.Equal(t, ws.gen, gen+1)
	assert.Equal(t, fmt.Sprint(ws.err), fmt.Sprint(legacyErr))
	assert.Assert(t, reflect.DeepEqual(ws.wls, legacy) && len(ws.wls) == 3, "want k8s-only map, got %v", ws.wls)
	src.refresh(false) // failed -> failed: no bump
	assert.Equal(t, src.load().gen, gen+1)
	e.slurm.set(fx.slurm, nil)
	src.refresh(false)
	assert.Equal(t, src.load().gen, gen+2)
	assert.Assert(t, src.load().err == nil)
}

// TestWorkloadSourceSeedFailure (case 30).
func TestWorkloadSourceSeedFailure(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newOracleEnv(t, oraclePullConfig, fx)
	e.k8s.set(fx.k8s, errOracleKubelet)
	src := manualSource(e) // the seed refresh ensureWorkloadSource runs
	ws := src.load()
	assert.Assert(t, ws != nil && ws.err != nil && len(ws.wls) == 0 && ws.gen == 1)
	gate := src
	_, ok := gate.lastGood()
	assert.Assert(t, !ok, "pull gate must miss before a success")
	wls, err := e.gpu.currentWorkloads()
	assert.Assert(t, err != nil && len(wls) == 0)
	e.k8s.set(fx.k8s, nil)
	src.refresh(true)
	assert.Equal(t, src.load().gen, uint64(2))
	assert.Assert(t, src.load().err == nil)
	got, ok := gate.lastGood()
	assert.Assert(t, ok && maps.Equal(got, fx.k8s))
}

// TestWorkloadSourceNonNotifier (case 31): oracleSched does not implement
// ChangeNotifier: fetched every refresh, compared, and the run loop is tick-only.
func TestWorkloadSourceNonNotifier(t *testing.T) {
	fx := fixtureByName("slurm")
	e := newOracleEnv(t, oraclePullConfig, fx)
	src := manualSource(e)
	_, ev := src.notifier()
	assert.Assert(t, ev == nil)
	lists, _ := e.slurm.counts()
	src.refresh(false)
	src.refresh(false)
	after, _ := e.slurm.counts()
	assert.Equal(t, after, lists+2)
	assert.Equal(t, src.load().gen, uint64(1))
	m := maps.Clone(fx.slurm)
	delete(m, "7")
	e.slurm.set(m, nil)
	src.refresh(false)
	assert.Equal(t, src.load().gen, uint64(2))
}

// TestWorkloadSourceUnchangedPartsSkipMerge (cases 33, 56): steady state on a
// notifier slurm node is 0 allocs; on k8s only the List itself allocates.
func TestWorkloadSourceUnchangedPartsSkipMerge(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("slurm"))
	slurm := newNotifySched(fixtureByName("slurm").slurm)
	e.ga.slurmScheduler = slurm
	src := manualSource(e)
	first := src.load()
	allocs := testing.AllocsPerRun(100, func() { src.refresh(true) })
	assert.Equal(t, allocs, float64(0), "slurm-only unchanged refresh allocates")
	assert.Equal(t, slurm.lists.Load(), int64(1), "unchanged version must not copy")
	assert.Assert(t, src.load() == first)

	k := newOracleEnv(t, oraclePullConfig, fixtureByName("k8s"))
	ks := manualSource(k)
	listAllocs := testing.AllocsPerRun(100, func() { _, _ = k.k8s.ListWorkloads() })
	allocs = testing.AllocsPerRun(100, func() { ks.refresh(true) })
	t.Logf("k8s unchanged refresh: %v allocs (fake List alone: %v)", allocs, listAllocs)
	assert.Assert(t, allocs <= listAllocs, "source adds allocs beyond the List: %v > %v", allocs, listAllocs)
}

// TestWorkloadSourceNoSchedulers (case 35).
func TestWorkloadSourceNoSchedulers(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
	src := manualSource(e)
	ws := src.load()
	assert.Assert(t, ws.err == nil && ws.wls != nil && len(ws.wls) == 0)
	_, ev := src.notifier()
	assert.Assert(t, ev == nil)
	e.ga.isKubernetes = false
	e.ga.k8sScheduler = &oracleSched{wls: fixtureByName("k8s").k8s}
	src.refresh(true)
	assert.Equal(t, src.k8sCalls.Load(), int64(0))
	assert.Equal(t, src.load(), ws)
}

// TestWorkloadEqualComparable (case 38).
func TestWorkloadEqualComparable(t *testing.T) {
	a := map[string]scheduler.Workload{"0": oraclePod(0), "1": oracleJob("1", "u")}
	b := map[string]scheduler.Workload{"0": oraclePod(0), "1": oracleJob("1", "u")}
	assert.Assert(t, maps.Equal(a, b))
	b["1"] = scheduler.Workload{Type: scheduler.Slurm, Info: scheduler.PodResourceInfo{Pod: "1"}}
	assert.Assert(t, !maps.Equal(a, b))
}

// TestWorkloadSourceSchedulerRecreated (case 42): a new client whose version
// restarts at the old memoised value still forces a full re-read.
func TestWorkloadSourceSchedulerRecreated(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("slurm"))
	old := newNotifySched(fixtureByName("slurm").slurm)
	e.ga.slurmScheduler = old
	src := manualSource(e)
	gen := src.load().gen
	same := newNotifySched(fixtureByName("slurm").slurm) // version 0 == old's
	e.ga.slurmScheduler = same
	src.refresh(false)
	assert.Equal(t, same.lists.Load(), int64(1), "new client not re-read")
	assert.Equal(t, src.load().gen, gen)
	diff := newNotifySched(map[string]scheduler.Workload{"0": oracleJob("9", "x")})
	e.ga.slurmScheduler = diff
	src.refresh(false)
	assert.Equal(t, src.load().gen, gen+1)
}

// TestWorkloadSourceErrorRecovery (case 43): one bump at the first failure,
// none while failing (slurm events cause no List and no republish), one at
// recovery, also when the recovered map is empty.
func TestWorkloadSourceErrorRecovery(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newOracleEnv(t, oraclePullConfig, fx)
	slurm := newNotifySched(fx.slurm)
	e.ga.slurmScheduler = slurm
	src := manualSource(e)
	lastOK := src.lastOK.Load()
	gen := src.load().gen
	e.k8s.set(fx.k8s, errOracleKubelet)
	for i := 0; i < 5; i++ {
		src.refresh(true)
		assert.Equal(t, src.load().gen, gen+1, "interval %d", i)
		calls := src.k8sCalls.Load()
		slurm.set(map[string]scheduler.Workload{"1": oracleJob(fmt.Sprint(300+i), "c")})
		src.refresh(false)
		assert.Equal(t, src.k8sCalls.Load(), calls, "event path called the kubelet")
		assert.Equal(t, src.load().gen, gen+1, "event during outage republished")
	}
	assert.Assert(t, src.lastOK.Load() == lastOK)
	e.k8s.set(fx.k8s, nil)
	src.refresh(true)
	ws := src.load()
	assert.Equal(t, ws.gen, gen+2)
	assert.Assert(t, ws.err == nil && src.lastOK.Load() == ws)
	assert.Equal(t, jobID(ws.wls, "1"), "304")

	// empty-ok != empty-failed
	z := newOracleEnv(t, oraclePullConfig, fixtureByName("k8s"))
	zs := manualSource(z)
	z.k8s.set(map[string]scheduler.Workload{}, nil)
	zs.refresh(true)
	g := zs.load().gen
	z.k8s.set(nil, errOracleKubelet)
	zs.refresh(true)
	z.k8s.set(map[string]scheduler.Workload{}, nil)
	zs.refresh(true)
	assert.Equal(t, zs.load().gen, g+2)
	assert.Assert(t, zs.load().err == nil && len(zs.load().wls) == 0)
}

// case82Base is characterization/case82-transitions-per-caller.json from base
// b2efd1b32: hr, profiler and health each made 12 label transitions over this
// pattern (pull cache 0) with 4 kubelet calls per interval.
var case82Pattern = []int{1, 0, 0, 1, 0, 1, 1, 0, 1, 1, 1, 0, 0, 0, 1, 0, 1, 1, 0, 1}

const case82BaseTransitions = 12

// TestWorkloadSourceNoFlapping (case 82).
func TestWorkloadSourceNoFlapping(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newOracleEnv(t, oracleHRConfig, fx)
	hc, src := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	pc.refreshOnce()
	gate := src
	edges := 0
	for i := 1; i < len(case82Pattern); i++ {
		if case82Pattern[i] != case82Pattern[i-1] {
			edges++
		}
	}
	gen0 := src.load().gen
	calls0 := src.k8sCalls.Load()
	hrPrev, pcPrev := hc.labels.Load(), pc.labels.Load()
	pullPrev, _ := gate.lastGood()
	hrTrans, pcTrans, healthTrans, pullTrans := 0, 0, 0, 0
	healthPrev, _ := e.gpu.currentWorkloads()
	for _, ok := range case82Pattern {
		if ok == 1 {
			e.k8s.set(fx.k8s, nil)
		} else {
			e.k8s.set(fx.k8s, errOracleKubelet)
		}
		src.refresh(true)
		hc.refreshOnce()
		pc.refreshOnce()
		if p := hc.labels.Load(); p != hrPrev {
			hrTrans++
			hrPrev = p
		}
		if p := pc.labels.Load(); p != pcPrev {
			pcTrans++
			pcPrev = p
		}
		if m, _ := e.gpu.currentWorkloads(); !reflect.DeepEqual(m, healthPrev) {
			healthTrans++
			healthPrev = m
		}
		if m, _ := gate.lastGood(); !reflect.DeepEqual(m, pullPrev) {
			pullTrans++
			pullPrev = m
		}
	}
	gens := int(src.load().gen - gen0)
	t.Logf("edges=%d gen bumps=%d transitions hr=%d profiler=%d health=%d pull=%d (base: 12/12/12/0); kubelet calls/interval=%.1f (base 4)",
		edges, gens, hrTrans, pcTrans, healthTrans, pullTrans,
		float64(src.k8sCalls.Load()-calls0)/float64(len(case82Pattern)))
	assert.Equal(t, gens, edges)
	assert.Equal(t, hrTrans, edges)
	assert.Equal(t, pcTrans, edges)
	assert.Assert(t, hrTrans <= case82BaseTransitions && pcTrans <= case82BaseTransitions &&
		healthTrans <= case82BaseTransitions && pullTrans == 0)
	assert.Equal(t, src.k8sCalls.Load()-calls0, int64(len(case82Pattern)))
}

// TestPullGateKeepsLastGoodOnError (cases 19, 83).
func TestPullGateKeepsLastGoodOnError(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newOracleEnv(t, oraclePullConfig, fx)
	src := manualSource(e) // startPullWorkloadCache reuses it
	e.gpu.startPullWorkloadCache(context.Background())
	gate := e.gpu.pullWLSource.Load()
	assert.Assert(t, gate != nil)
	e.gpu.startPullWorkloadCache(context.Background()) // idempotent
	assert.Assert(t, e.gpu.pullWLSource.Load() == gate)
	good, ok := gate.lastGood()
	assert.Assert(t, ok && maps.Equal(good, fx.k8s))
	e.k8s.set(fx.k8s, errOracleKubelet)
	for i := 0; i < 3; i++ {
		src.refresh(true)
		got, ok := gate.lastGood()
		assert.Assert(t, ok && maps.Equal(got, good), "interval %d: lost the last good map", i)
	}
	assertGolden(t, "k8s-k8sfail.pull-cache.metrics", e.renderPull(t))
	next := map[string]scheduler.Workload{"0000:01:00.0": oraclePod(5)}
	e.k8s.set(next, nil)
	src.refresh(true)
	got, _ := gate.lastGood()
	assert.Assert(t, maps.Equal(got, next))
}

// TestCurrentWorkloadsFallback (case 21).
func TestCurrentWorkloadsFallback(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newOracleEnv(t, oraclePullConfig, fx)
	e.slurm.set(fx.slurm, errSlurmTest)
	want, wantErr := e.ga.ListWorkloads()
	got, err := e.gpu.currentWorkloads()
	assert.Assert(t, reflect.DeepEqual(got, want))
	assert.Equal(t, fmt.Sprint(err), fmt.Sprint(wantErr))
	assert.Assert(t, e.gpu.wlSource.Load() == nil, "currentWorkloads must not start a source")
	src := manualSource(e)
	got, err = e.gpu.currentWorkloads()
	assert.Assert(t, reflect.DeepEqual(got, src.load().wls) && err == src.load().err)
	e.slurm.set(fx.slurm, nil)
	src.refresh(false)
	got, err = e.gpu.currentWorkloads()
	assert.NilError(t, err)
	assert.Equal(t, len(got), 4)
}

// TestHealthUsesCurrentWorkloads (case 20): with a source, health polls make no
// scheduler calls; without one, health does not start a source.
func TestHealthUsesCurrentWorkloads(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newOracleEnv(t, oraclePullConfig, fx)
	before, _ := e.k8s.counts()
	_ = e.gpu.processHealthValidation()
	e.gpu.updateAllGPUsHealthState("healthy")
	after, _ := e.k8s.counts()
	assert.Assert(t, after > before, "without a source health fetches synchronously")
	assert.Assert(t, e.gpu.wlSource.Load() == nil, "health must not start the source")
	src := manualSource(e)
	before, _ = e.k8s.counts()
	for i := 0; i < 10; i++ {
		_ = e.gpu.processHealthValidation()
		e.gpu.updateAllGPUsHealthState("healthy")
	}
	after, _ = e.k8s.counts()
	assert.Equal(t, after, before, "health called the kubelet with a source running")
	assert.Assert(t, src.load().err == nil)
}

// TestWorkloadSourceSlurmEventLatency (case 8): real slurm client on a temp
// dir, interval 1h: a job create / remove shows within 100ms, no kubelet call.
func TestWorkloadSourceSlurmEventLatency(t *testing.T) {
	dir := t.TempDir()
	cl, err := scheduler.NewSlurmClientForDir(context.Background(), dir)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("k8s"))
	e.ga.slurmScheduler = cl
	src := runningSource(t, e, time.Hour)
	calls := src.k8sCalls.Load()
	for i := 0; i < 5; i++ {
		id := fmt.Sprint(700 + i)
		start := time.Now()
		writeSlurmJob(t, dir, "3", id, "3")
		assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { return jobID(src.load().wls, "3") == id }),
			"create %d not seen within 100ms", i)
		created := time.Since(start)
		start = time.Now()
		assert.NilError(t, os.Remove(dir+"/3"))
		assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { return jobID(src.load().wls, "3") == "" }),
			"remove %d not seen within 100ms", i)
		t.Logf("event latency create=%v remove=%v", created, time.Since(start))
	}
	assert.Equal(t, src.k8sCalls.Load(), calls, "slurm events called the kubelet")
}

// TestWorkloadSourceEventCoalescing (case 47): 100 job-file writes in a burst
// coalesce into a few refreshes; the final snapshot equals the directory.
func TestWorkloadSourceEventCoalescing(t *testing.T) {
	dir := t.TempDir()
	cl, err := scheduler.NewSlurmClientForDir(context.Background(), dir)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("slurm"))
	e.ga.slurmScheduler = cl
	src := runningSource(t, e, time.Hour)
	calls := src.k8sCalls.Load()
	start := time.Now()
	for i := 0; i < 100; i++ {
		writeSlurmJob(t, dir, "1", fmt.Sprint(1000+i), "1")
	}
	burst := time.Since(start)
	assert.Assert(t, waitWithin(time.Second, func() bool { return jobID(src.load().wls, "1") == "1099" }))
	time.Sleep(20 * time.Millisecond)
	t.Logf("100 writes in %v -> %d event refreshes, gen %d", burst, src.evRefreshes.Load(), src.load().gen)
	assert.Assert(t, src.evRefreshes.Load() <= 50, "no coalescing: %d refreshes", src.evRefreshes.Load())
	assert.Equal(t, src.k8sCalls.Load(), calls)
}

// TestWorkloadSourceNoLostWakeup (case 67): interval 1h, so only Changed() can
// wake the source. A bump is injected at each point of the obtain-then-read
// sequence; every one must be visible within 100ms.
func TestWorkloadSourceNoLostWakeup(t *testing.T) {
	iters := 1000
	if testing.Short() {
		iters = 100
	}
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewSource(seed))
	content := func(i int) map[string]scheduler.Workload {
		return map[string]scheduler.Workload{"0": oracleJob(fmt.Sprint(i), "u")}
	}
	// a: right after the run loop re-armed (inside Changed); c: between
	// Version() and ListWorkloads(); list: right after the copy.
	for _, point := range []string{"changed", "version", "list"} {
		t.Run(point, func(t *testing.T) {
			e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
			f := newNotifySched(content(0))
			e.ga.slurmScheduler = f
			src := runningSource(t, e, time.Hour)
			for i := 1; i <= iters; i++ {
				want := fmt.Sprint(-i)
				var once sync.Once
				h := func(p string) {
					if p == point {
						once.Do(func() { go f.set(content(-i)) })
					}
				}
				f.hook.Store(&h)
				f.set(content(i)) // wakes the source, which hits the hook
				if !waitWithin(wakeBound, func() bool { return jobID(src.load().wls, "0") == want }) {
					t.Fatalf("iteration %d: bump at %q lost (have %q)", i, point, jobID(src.load().wls, "0"))
				}
				f.hook.Store(nil)
			}
		})
	}
	// b: during the coalesce wait; d: just after a refresh returned.
	for _, point := range []string{"coalesce", "after"} {
		t.Run(point, func(t *testing.T) {
			e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
			f := newNotifySched(content(0))
			e.ga.slurmScheduler = f
			src := runningSource(t, e, time.Hour)
			for i := 1; i <= iters; i++ {
				f.set(content(i))
				d := time.Duration(rng.Intn(500)) * time.Microsecond
				if point == "after" {
					want := fmt.Sprint(i)
					waitWithin(100*time.Millisecond, func() bool { return jobID(src.load().wls, "0") == want })
				}
				time.Sleep(d)
				f.set(content(-i))
				want := fmt.Sprint(-i)
				if !waitWithin(wakeBound, func() bool { return jobID(src.load().wls, "0") == want }) {
					t.Fatalf("iteration %d: bump at %q (+%v) lost", i, point, d)
				}
			}
		})
	}
	// e/f: seed path: a bump right after ensure armed the channel, and one
	// between the seed refresh and run starting.
	for _, point := range []string{"changed", "list"} {
		t.Run("seed-"+point, func(t *testing.T) {
			for i := 1; i <= iters/10; i++ {
				e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
				f := newNotifySched(content(0))
				e.ga.slurmScheduler = f
				var once sync.Once
				h := func(p string) {
					if p == point {
						once.Do(func() { go f.set(content(-i)) })
					}
				}
				f.hook.Store(&h)
				src := runningSource(t, e, time.Hour)
				want := fmt.Sprint(-i)
				if !waitWithin(wakeBound, func() bool { return jobID(src.load().wls, "0") == want }) {
					t.Fatalf("iteration %d: seed bump at %q lost", i, point)
				}
				src.stop()
			}
		})
	}
	// g: tick path re-arm after the slurm client is re-created.
	t.Run("recreated", func(t *testing.T) {
		e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
		e.ga.slurmScheduler = newNotifySched(content(0))
		src := runningSource(t, e, 10*time.Millisecond)
		for i := 1; i <= 20; i++ {
			f := newNotifySched(content(i))
			e.ga.schedMu.Lock()
			e.ga.slurmScheduler = f
			e.ga.schedMu.Unlock()
			want := fmt.Sprint(i)
			assert.Assert(t, waitWithin(time.Second, func() bool { return jobID(src.load().wls, "0") == want }))
			time.Sleep(15 * time.Millisecond) // past the re-arm tick
			f.set(content(-i))
			want = fmt.Sprint(-i)
			if !waitWithin(wakeBound, func() bool { return jobID(src.load().wls, "0") == want }) {
				t.Fatalf("iteration %d: event on re-created client lost", i)
			}
		}
	})
}

// wakeBound bounds "a change is not lost" waits. A lost wakeup waits the 1h
// interval, so 1s separates it from a scheduler stall under -race; the 100ms
// latency bound is asserted in the dedicated latency tests.
const wakeBound = time.Second

// TestWorkloadSourceVersionBeforeList (case 69): a bump during the copy (the
// copy returns pre-bump content) is caught on the next wake.
func TestWorkloadSourceVersionBeforeList(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
	f := newNotifySched(map[string]scheduler.Workload{"0": oracleJob("1", "u")})
	e.ga.slurmScheduler = f
	src := runningSource(t, e, time.Hour)
	var once sync.Once
	h := func(p string) {
		if p == "list" {
			once.Do(func() { f.set(map[string]scheduler.Workload{"0": oracleJob("3", "u")}) })
		}
	}
	f.hook.Store(&h)
	f.set(map[string]scheduler.Workload{"0": oracleJob("2", "u")})
	assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { return jobID(src.load().wls, "0") == "3" }))
	assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { return f.lists.Load() >= 3 }))
}

// TestStopWorkloadSource (case 44).
func TestStopWorkloadSource(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
	f := newNotifySched(map[string]scheduler.Workload{})
	e.ga.slurmScheduler = f
	src := runningSource(t, e, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-src.Changed():
			case <-ctx.Done():
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	base := sourceGoroutines()
	e.gpu.stopWorkloadSource()
	assert.Assert(t, waitWithin(time.Second, func() bool { return sourceGoroutines() == base-1 }),
		"source goroutine did not exit")
	last := src.load()
	assert.Assert(t, e.gpu.ensureWorkloadSource() == src, "stopped source must not restart")
	for i := 0; i < 3; i++ {
		f.set(map[string]scheduler.Workload{"0": oracleJob(fmt.Sprint(i), "u")})
	}
	time.Sleep(30 * time.Millisecond)
	assert.Assert(t, src.load() == last, "stopped source advanced")
	e.gpu.stopWorkloadSource() // no-op
	cancel()
	wg.Wait()
}

// TestEnsureWorkloadSourceConcurrent (case 45).
func TestEnsureWorkloadSourceConcurrent(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("slurm"))
	e.gpu.wlInterval = time.Hour
	base := sourceGoroutines()
	var wg sync.WaitGroup
	got := make([]*workloadSource, 64)
	start := make(chan struct{})
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			got[i] = e.gpu.ensureWorkloadSource()
		}(i)
	}
	close(start)
	wg.Wait()
	t.Cleanup(e.gpu.stopWorkloadSource)
	for _, s := range got {
		assert.Assert(t, s == got[0])
	}
	assert.Equal(t, got[0].load().gen, uint64(1))
	// losers never start a goroutine: exactly one source goroutine was added
	assert.Equal(t, sourceGoroutines(), base+1)
}

// blockingSched blocks ListWorkloads until released.
type blockingSched struct {
	oracleSched
	release chan struct{}
}

func (b *blockingSched) ListWorkloads() (map[string]scheduler.Workload, error) {
	<-b.release
	return b.oracleSched.ListWorkloads()
}

// TestSchedMuRace (case 46): slurm pointer writes (Init/Close) race the
// readers; no scheduler call under schedMu (a blocked List does not block
// Close); Close never writes the k8s pointer.
func TestSchedMuRace(t *testing.T) {
	dir := t.TempDir()
	useSlurmDir(t, dir)
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("k8s"))
	e.ga.enableSlurmScl = true
	k8s := e.ga.k8sScheduler
	src := runningSource(t, e, time.Millisecond)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = e.ga.ListWorkloads()
				_, _, _ = e.ga.schedulers()
				_ = src.load()
			}
		}()
	}
	for i := 0; i < 20; i++ {
		e.ga.Lock()
		assert.NilError(t, e.ga.initalizeScheduler())
		e.ga.Unlock()
		e.ga.Close()
	}
	close(stop)
	wg.Wait()
	_, k, _ := e.ga.schedulers()
	assert.Assert(t, k == k8s)

	b := &blockingSched{oracleSched: oracleSched{wls: fixtureByName("slurm").slurm}, release: make(chan struct{})}
	e.ga.schedMu.Lock()
	e.ga.slurmScheduler = b
	e.ga.schedMu.Unlock()
	go func() { _, _ = e.ga.ListWorkloads() }()
	time.Sleep(10 * time.Millisecond)
	done := make(chan struct{})
	go func() { e.ga.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close blocked behind a scheduler call")
	}
	close(b.release)
}

// TestWorkloadSourceConcurrentReaders (case 48): readers vs an event storm and
// a stop mid-run; every snapshot is consistent.
func TestWorkloadSourceConcurrentReaders(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, oracleFixture{name: "none"})
	f := newNotifySched(map[string]scheduler.Workload{})
	e.ga.slurmScheduler = f
	src := runningSource(t, e, time.Millisecond)
	gate := src
	var bad atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				ws := src.load()
				// writer invariant: content id == gen - 1 (one change per gen)
				if id := jobID(ws.wls, "0"); id != "" && id != fmt.Sprint(ws.gen-1) {
					bad.Add(1)
				}
				_, _ = gate.lastGood()
			}
		}()
	}
	for i := 1; i <= 500; i++ {
		want := uint64(i + 1)
		f.set(map[string]scheduler.Workload{"0": oracleJob(fmt.Sprint(i), "u")})
		if !waitWithin(time.Second, func() bool { return src.load().gen == want }) {
			t.Fatalf("change %d not published", i)
		}
		if i == 400 {
			src.stop()
			break
		}
	}
	close(stop)
	wg.Wait()
	assert.Equal(t, bad.Load(), int64(0))
	b, _ := json.Marshal(src.load().gen)
	t.Logf("final gen %s", b)
}
