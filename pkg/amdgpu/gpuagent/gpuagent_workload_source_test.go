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

// Workload source unit tests: merge and failure semantics, change detection,
// last-good snapshot, consumers, event wake-ups, lifecycle and reconnect.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

var errSlurmTest = errors.New("slurm list failed")

// referenceListWorkloads is the ListWorkloads body before the workload
// source existed, kept verbatim as the reference for mergeWorkloads.
func referenceListWorkloads(isK8s bool, k8s, slurm scheduler.SchedulerClient) (wls map[string]scheduler.Workload, err error) {
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

// TestMergeWorkloadsMatchesReference: truth table over k8sOn x k8s {nil, ok,
// err} x slurm {nil, ok, empty, err} with overlapping and disjoint keys.
func TestMergeWorkloadsMatchesReference(t *testing.T) {
	mixed := fixtureByName("mixed")
	for _, k8sOn := range []bool{false, true} {
		for _, k8sMode := range []string{"nil", "ok", "err"} {
			for _, slurmMode := range []string{"nil", "ok", "empty", "err"} {
				name := fmt.Sprintf("k8sOn=%v/k8s=%s/slurm=%s", k8sOn, k8sMode, slurmMode)
				var k8s, slurm scheduler.SchedulerClient
				if k8sMode != "nil" {
					f := &fakeSched{wls: mixed.k8s}
					if k8sMode == "err" {
						f.err = errKubeletUnavailable
					}
					k8s = f
				}
				if slurmMode != "nil" {
					f := &fakeSched{wls: mixed.slurm}
					switch slurmMode {
					case "err":
						f.err = errSlurmTest
					case "empty":
						f.wls = map[string]scheduler.Workload{}
					}
					slurm = f
				}
				want, wantErr := referenceListWorkloads(k8sOn, k8s, slurm)

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
	ga := &GPUAgentClient{isKubernetes: true, k8sScheduler: &fakeSched{wls: mixed.k8s},
		slurmScheduler: &fakeSched{wls: mixed.slurm}}
	wls, err := ga.ListWorkloads()
	assert.NilError(t, err)
	assert.Equal(t, wls["3"].Type, scheduler.Slurm)
	assert.Equal(t, len(wls), 4)
}

// TestWorkloadSourceUnchangedSamePointer: unchanged refreshes keep the
// published snapshot and never close Changed; with a real kubelet client every
// List comes from the source and a pod move bumps gen once.
func TestWorkloadSourceUnchangedSamePointer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kubelet bool
	}{
		{"k8s fake", false},
		{"k8s real kubelet client", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newWorkloadEnv(t, pullTestConfig, fixtureByName("k8s"))
			var fk *fakeKubelet
			if tc.kubelet {
				fk = newFakeKubelet(t, kubeResp("0000:01:00.0"))
				e.ga.k8sScheduler = fk.client(t)
			}
			src := manualSource(e)
			first := src.load()
			assert.Equal(t, first.gen, uint64(1))
			ch := src.Changed()
			calls := src.k8sCalls.Load()
			for i := 0; i < 100; i++ {
				src.refresh(true)
			}
			assert.Assert(t, src.load() == first, "unchanged refresh published a new snapshot")
			assert.Equal(t, src.k8sCalls.Load(), calls+100)
			select {
			case <-ch:
				t.Fatal("Changed closed without a change")
			default:
			}
			if fk == nil {
				return
			}
			assert.Equal(t, fk.calls.Load(), src.k8sCalls.Load(), "a kubelet List did not come from the source")
			fk.set(kubeResp("0000:02:00.0"), nil) // pod moved
			src.refresh(true)
			<-ch
			assert.Equal(t, src.load().gen, first.gen+1)
			_, ok := src.load().wls["0000:02:00.0"]
			assert.Assert(t, ok)
		})
	}
}

// TestWorkloadSourceGenBump: gen +1 per content change, published maps never
// mutated, empty transitions, and a seeded random walk.
func TestWorkloadSourceGenBump(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, fixtureByName("slurm"))
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
	m["5"] = testJob("105", "eve")
	step(m, 2) // added
	delete(m, "5")
	step(maps.Clone(m), 3) // removed
	m["0"] = testJob("100", "alice2")
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
			next[k] = testJob(fmt.Sprint(rng.Intn(3)), "u")
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

// TestWorkloadSourceK8sErrorDropsWorkloadLabels: a kubelet failure publishes
// the reference ListWorkloads result (empty, non-nil, same error) and both
// label caches drop the pod labels exactly as a fresh build with no workloads.
func TestWorkloadSourceK8sErrorDropsWorkloadLabels(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newWorkloadEnv(t, hrTestConfig, fx)
	hc, src := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	pc.refreshOnce()
	gen := src.load().gen
	ch := src.Changed()
	e.k8s.set(fx.k8s, errKubeletUnavailable)
	src.refresh(true)
	<-ch
	ws := src.load()
	ref, refErr := e.ga.ListWorkloads()
	assert.Assert(t, ws.err != nil)
	assert.Equal(t, ws.err.Error(), refErr.Error())
	assert.Assert(t, ws.wls != nil && len(ws.wls) == 0 && reflect.DeepEqual(ws.wls, ref))
	assert.Equal(t, ws.gen, gen+1)
	for _, c := range []*workloadLabelCache{hc, pc} {
		c.refreshOnce()
		assert.Assert(t, reflect.DeepEqual(*c.labels.Load(),
			e.freshLabels(map[string]scheduler.Workload{}, c.keys.key)), "%T", c.keys)
		for _, l := range *c.labels.Load() {
			assert.Equal(t, l["pod"], "")
		}
	}
}

// TestWorkloadSourceSlurmErrorMatchesReference: a slurm failure publishes the
// k8s-only map with the reference error; failed -> failed does not bump and
// recovery does.
func TestWorkloadSourceSlurmErrorMatchesReference(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newWorkloadEnv(t, pullTestConfig, fx)
	src := manualSource(e)
	gen := src.load().gen
	e.slurm.set(fx.slurm, errSlurmTest)
	src.refresh(false)
	ws := src.load()
	ref, refErr := e.ga.ListWorkloads()
	assert.Equal(t, ws.gen, gen+1)
	assert.Equal(t, fmt.Sprint(ws.err), fmt.Sprint(refErr))
	assert.Assert(t, reflect.DeepEqual(ws.wls, ref) && len(ws.wls) == 3, "want k8s-only map, got %v", ws.wls)
	src.refresh(false) // failed -> failed: no bump
	assert.Equal(t, src.load().gen, gen+1)
	e.slurm.set(fx.slurm, nil)
	src.refresh(false)
	assert.Equal(t, src.load().gen, gen+2)
	assert.Assert(t, src.load().err == nil)
}

// TestWorkloadSourceSeedFailure: a failed seed publishes gen 1 with the error;
// the first success publishes gen 2.
func TestWorkloadSourceSeedFailure(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newWorkloadEnv(t, pullTestConfig, fx)
	e.k8s.set(fx.k8s, errKubeletUnavailable)
	src := manualSource(e) // the seed refresh ensureWorkloadSource runs
	ws := src.load()
	assert.Assert(t, ws != nil && ws.err != nil && len(ws.wls) == 0 && ws.gen == 1)
	wls, err := e.gpu.currentWorkloads()
	assert.Assert(t, err != nil && len(wls) == 0)
	e.k8s.set(fx.k8s, nil)
	src.refresh(true)
	assert.Equal(t, src.load().gen, uint64(2))
	assert.Assert(t, src.load().err == nil)
}

// TestWorkloadSourceNonNotifier: a scheduler that does not implement
// ChangeNotifier is listed on every refresh and compared; the run loop is
// tick-only.
func TestWorkloadSourceNonNotifier(t *testing.T) {
	fx := fixtureByName("slurm")
	e := newWorkloadEnv(t, pullTestConfig, fx)
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

// TestWorkloadSourceUnchangedPartsSkipMerge: steady state on a notifier node
// is 0 allocs with no copy; on k8s only the List itself allocates.
func TestWorkloadSourceUnchangedPartsSkipMerge(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, fixtureByName("slurm"))
	slurm := newNotifySched(fixtureByName("slurm").slurm)
	e.ga.slurmScheduler = slurm
	src := manualSource(e)
	first := src.load()
	allocs := testing.AllocsPerRun(100, func() { src.refresh(true) })
	assert.Equal(t, allocs, float64(0), "notifier-only unchanged refresh allocates")
	assert.Equal(t, slurm.lists.Load(), int64(1), "unchanged version must not copy")
	assert.Assert(t, src.load() == first)

	k := newWorkloadEnv(t, pullTestConfig, fixtureByName("k8s"))
	ks := manualSource(k)
	listAllocs := testing.AllocsPerRun(100, func() { _, _ = k.k8s.ListWorkloads() })
	allocs = testing.AllocsPerRun(100, func() { ks.refresh(true) })
	assert.Assert(t, allocs <= listAllocs, "source adds allocs beyond the List: %v > %v", allocs, listAllocs)
}

// TestWorkloadSourceNoSchedulers: no schedulers publish an empty ok snapshot,
// and a k8s client is not called when the node is not k8s.
func TestWorkloadSourceNoSchedulers(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, workloadFixture{name: "none"})
	src := manualSource(e)
	ws := src.load()
	assert.Assert(t, ws.err == nil && ws.wls != nil && len(ws.wls) == 0)
	_, ev := src.notifier()
	assert.Assert(t, ev == nil)
	e.ga.isKubernetes = false
	e.ga.k8sScheduler = &fakeSched{wls: fixtureByName("k8s").k8s}
	src.refresh(true)
	assert.Equal(t, src.k8sCalls.Load(), int64(0))
	assert.Equal(t, src.load(), ws)
}

// TestWorkloadSourceSchedulerRecreated: a new notifier client whose version
// restarts at the old memoised value still forces a full re-read.
func TestWorkloadSourceSchedulerRecreated(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, fixtureByName("slurm"))
	old := newNotifySched(fixtureByName("slurm").slurm)
	e.ga.slurmScheduler = old
	src := manualSource(e)
	gen := src.load().gen
	same := newNotifySched(fixtureByName("slurm").slurm) // version 0 == old's
	e.ga.slurmScheduler = same
	src.refresh(false)
	assert.Equal(t, same.lists.Load(), int64(1), "new client not re-read")
	assert.Equal(t, src.load().gen, gen)
	diff := newNotifySched(map[string]scheduler.Workload{"0": testJob("9", "x")})
	e.ga.slurmScheduler = diff
	src.refresh(false)
	assert.Equal(t, src.load().gen, gen+1)
}

// TestWorkloadSourceErrorRecovery: one bump at the first failure, none while
// failing (notifier events cause no kubelet call and no republish), one at
// recovery, also when the recovered map is empty.
func TestWorkloadSourceErrorRecovery(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newWorkloadEnv(t, pullTestConfig, fx)
	slurm := newNotifySched(fx.slurm)
	e.ga.slurmScheduler = slurm
	src := manualSource(e)
	lastOK := src.lastOK.Load()
	gen := src.load().gen
	e.k8s.set(fx.k8s, errKubeletUnavailable)
	for i := 0; i < 5; i++ {
		src.refresh(true)
		assert.Equal(t, src.load().gen, gen+1, "interval %d", i)
		calls := src.k8sCalls.Load()
		slurm.set(map[string]scheduler.Workload{"1": testJob(fmt.Sprint(300+i), "c")})
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
	z := newWorkloadEnv(t, pullTestConfig, fixtureByName("k8s"))
	zs := manualSource(z)
	z.k8s.set(map[string]scheduler.Workload{}, nil)
	zs.refresh(true)
	g := zs.load().gen
	z.k8s.set(nil, errKubeletUnavailable)
	zs.refresh(true)
	z.k8s.set(map[string]scheduler.Workload{}, nil)
	zs.refresh(true)
	assert.Equal(t, zs.load().gen, g+2)
	assert.Assert(t, zs.load().err == nil && len(zs.load().wls) == 0)
}

// TestWorkloadSourceLastGood: the pull path's last-good map misses before the
// first success, keeps the last good map (and the pod labels on /metrics)
// across failures and updates on the next success. Starting the pull gate is
// idempotent and a no-op in high-res mode.
func TestWorkloadSourceLastGood(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newWorkloadEnv(t, pullTestConfig, fx)
	e.k8s.set(fx.k8s, errKubeletUnavailable)
	src := manualSource(e) // startPullWorkloadCache reuses it
	_, ok := src.lastGood()
	assert.Assert(t, !ok, "last good must miss before a success")
	e.k8s.set(fx.k8s, nil)
	src.refresh(true)

	e.gpu.startPullWorkloadCache(context.Background())
	gate := e.gpu.pullWLSource.Load()
	assert.Assert(t, gate == src)
	e.gpu.startPullWorkloadCache(context.Background()) // idempotent
	assert.Assert(t, e.gpu.pullWLSource.Load() == gate)
	good, ok := gate.lastGood()
	assert.Assert(t, ok && maps.Equal(good, fx.k8s))
	e.k8s.set(fx.k8s, errKubeletUnavailable)
	for i := 0; i < 3; i++ {
		src.refresh(true)
		got, ok := gate.lastGood()
		assert.Assert(t, ok && maps.Equal(got, good), "interval %d: lost the last good map", i)
	}
	assert.Assert(t, strings.Contains(e.renderPull(t), `pod="pod0"`), "pull /metrics lost the pod during the outage")
	next := map[string]scheduler.Workload{"0000:01:00.0": testPod(5)}
	e.k8s.set(next, nil)
	src.refresh(true)
	got, _ := gate.lastGood()
	assert.Assert(t, maps.Equal(got, next))

	h := newWorkloadEnv(t, hrTestConfig, fx)
	h.gpu.startPullWorkloadCache(context.Background())
	assert.Assert(t, h.gpu.pullWLSource.Load() == nil, "pull gate started in high-res mode")
}

// TestGetMetricsAllUsesPullCache: a pull scrape reads workloads from the
// source's last-good map and makes no scheduler call; through pod start,
// change and end the gated /metrics equals the per-scrape ListWorkloads path.
func TestGetMetricsAllUsesPullCache(t *testing.T) {
	t.Run("sentinel", func(t *testing.T) {
		e := newWorkloadEnv(t, pullTestConfig, fixtureByName("k8s"))
		src := newWorkloadSource(e.ga, 0)
		src.lastOK.Store(&workloadSnap{gen: 1, wls: map[string]scheduler.Workload{
			"0000:01:00.0": {Type: scheduler.Kubernetes, Info: scheduler.PodResourceInfo{Pod: "cached-pod"}}}})
		e.gpu.pullWLSource.Store(src)
		before, _ := e.k8s.counts()
		out := e.renderPull(t)
		after, _ := e.k8s.counts()
		assert.Equal(t, after, before, "pull scrape called the scheduler")
		assert.Assert(t, strings.Contains(out, `pod="cached-pod"`), "pull path did not use the last-good map")
		assert.Assert(t, !strings.Contains(out, `pod="pod0"`), "pull path used the scheduler")
	})
	t.Run("gated equals ungated", func(t *testing.T) {
		fx := fixtureByName("k8s")
		gated := newWorkloadEnv(t, pullTestConfig, fx)
		src := manualSource(gated)
		gated.gpu.pullWLSource.Store(src)
		ungated := newWorkloadEnv(t, pullTestConfig, fx)
		for _, st := range []struct {
			name string
			wls  map[string]scheduler.Workload
			want string
			gone string
		}{
			{"start", map[string]scheduler.Workload{"0000:03:00.0": testPod(2)}, `pod="pod2"`, ""},
			{"change", map[string]scheduler.Workload{"0000:03:00.0": testPod(4)}, `pod="pod4"`, `pod="pod2"`},
			{"end", map[string]scheduler.Workload{}, "", `pod="pod4"`},
		} {
			gated.k8s.set(st.wls, nil)
			ungated.k8s.set(st.wls, nil)
			src.refresh(true)
			out := gated.renderPull(t)
			assert.Equal(t, out, ungated.renderPull(t), "%s: gated /metrics differs from the per-scrape path", st.name)
			if st.want != "" {
				assert.Assert(t, strings.Contains(out, st.want), "%s: /metrics lacks %s", st.name, st.want)
			}
			if st.gone != "" {
				assert.Assert(t, !strings.Contains(out, st.gone), "%s: /metrics still serves %s", st.name, st.gone)
			}
		}
	})
}

// TestCurrentWorkloadsFallback: with no source, currentWorkloads is the
// synchronous reference ListWorkloads and starts no source; with a source it
// returns the source's snapshot.
func TestCurrentWorkloadsFallback(t *testing.T) {
	fx := fixtureByName("mixed")
	e := newWorkloadEnv(t, pullTestConfig, fx)
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

// TestWorkloadConsumersMakeNoSchedulerCalls: with a source installed, health,
// the hr and profiler label caches and the pull gate make no kubelet call;
// without one, health lists synchronously and does not start a source.
func TestWorkloadConsumersMakeNoSchedulerCalls(t *testing.T) {
	fx := fixtureByName("k8s")
	e := newWorkloadEnv(t, hrTestConfig, fx)
	before, _ := e.k8s.counts()
	_ = e.gpu.processHealthValidation()
	e.gpu.updateAllGPUsHealthState("healthy")
	after, _ := e.k8s.counts()
	assert.Assert(t, after > before, "without a source health fetches synchronously")
	assert.Assert(t, e.gpu.wlSource.Load() == nil, "health must not start the source")

	hc, src := e.startHRSource(t, 0)
	pc := newProfilerLabelCache(e.gpu)
	e.gpu.pullWLSource.Store(src)
	for _, tc := range []struct {
		name string
		use  func()
	}{
		{"health", func() {
			_ = e.gpu.processHealthValidation()
			e.gpu.updateAllGPUsHealthState("healthy")
		}},
		{"hr label cache", hc.refreshOnce},
		{"profiler label cache", pc.refreshOnce},
		{"pull gate", func() { _, _ = e.gpu.pullWLSource.Load().lastGood() }},
	} {
		before, _ := e.k8s.counts()
		for i := 0; i < 10; i++ {
			tc.use()
		}
		after, _ := e.k8s.counts()
		assert.Equal(t, after, before, "%s called the kubelet with a source installed", tc.name)
	}
	assert.Assert(t, src.load().err == nil)
}

// TestWorkloadSourceEventCoalescing: a burst of 100 notifier changes coalesces
// into a few refreshes; the final snapshot is the last change.
func TestWorkloadSourceEventCoalescing(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, fixtureByName("k8s"))
	f := newNotifySched(map[string]scheduler.Workload{})
	e.ga.slurmScheduler = f
	src := runningSource(t, e, time.Hour)
	calls := src.k8sCalls.Load()
	for i := 0; i < 100; i++ {
		f.set(map[string]scheduler.Workload{"1": testJob(fmt.Sprint(1000+i), "u")})
	}
	assert.Assert(t, waitWithin(time.Second, func() bool { return jobID(src.load().wls, "1") == "1099" }))
	time.Sleep(20 * time.Millisecond)
	assert.Assert(t, src.evRefreshes.Load() <= 50, "no coalescing: %d refreshes", src.evRefreshes.Load())
	assert.Equal(t, src.k8sCalls.Load(), calls, "notifier events called the kubelet")
}

// TestWorkloadSourceNoLostWakeup: interval 1h, so only Changed() can wake the
// source. A bump is injected at each point of the obtain-then-read sequence;
// every one must be visible within wakeBound.
func TestWorkloadSourceNoLostWakeup(t *testing.T) {
	const iters = 100
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewSource(seed))
	content := func(i int) map[string]scheduler.Workload {
		return map[string]scheduler.Workload{"0": testJob(fmt.Sprint(i), "u")}
	}
	// changed: right after the run loop re-armed (inside Changed); version:
	// between Version() and ListWorkloads(); list: right after the copy.
	for _, point := range []string{"changed", "version", "list"} {
		t.Run(point, func(t *testing.T) {
			e := newWorkloadEnv(t, pullTestConfig, workloadFixture{name: "none"})
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
	// coalesce: during the coalesce wait; after: just after a refresh returned.
	for _, point := range []string{"coalesce", "after"} {
		t.Run(point, func(t *testing.T) {
			e := newWorkloadEnv(t, pullTestConfig, workloadFixture{name: "none"})
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
	// seed path: a bump right after ensure armed the channel, and one between
	// the seed refresh and run starting.
	for _, point := range []string{"changed", "list"} {
		t.Run("seed-"+point, func(t *testing.T) {
			for i := 1; i <= iters/10; i++ {
				e := newWorkloadEnv(t, pullTestConfig, workloadFixture{name: "none"})
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
	// recreated: tick path re-arm after the notifier client is re-created.
	t.Run("recreated", func(t *testing.T) {
		e := newWorkloadEnv(t, pullTestConfig, workloadFixture{name: "none"})
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
// interval, so 1s separates it from a scheduler stall under -race.
const wakeBound = time.Second

// TestStopWorkloadSource: stop ends the goroutine, the source is not
// restarted and does not advance any more.
func TestStopWorkloadSource(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, workloadFixture{name: "none"})
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
		f.set(map[string]scheduler.Workload{"0": testJob(fmt.Sprint(i), "u")})
	}
	time.Sleep(30 * time.Millisecond)
	assert.Assert(t, src.load() == last, "stopped source advanced")
	e.gpu.stopWorkloadSource() // no-op
	cancel()
	wg.Wait()
}

// TestEnsureWorkloadSourceConcurrent: concurrent ensure calls return one
// source and start one goroutine.
func TestEnsureWorkloadSourceConcurrent(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, fixtureByName("k8s"))
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
	fakeSched
	release chan struct{}
}

func (b *blockingSched) ListWorkloads() (map[string]scheduler.Workload, error) {
	<-b.release
	return b.fakeSched.ListWorkloads()
}

// TestSchedMuRace: slurm pointer writes (Init/Close) race the readers; no
// scheduler call runs under schedMu (a blocked List does not block Close);
// Close never writes the k8s pointer.
func TestSchedMuRace(t *testing.T) {
	useSlurmDir(t, t.TempDir())
	e := newWorkloadEnv(t, pullTestConfig, fixtureByName("k8s"))
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

	b := &blockingSched{fakeSched: fakeSched{wls: fixtureByName("slurm").slurm}, release: make(chan struct{})}
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

// TestWorkloadSourceConcurrentReaders: readers of the snapshot and the
// last-good map run against a stream of changes and a stop mid-run; every
// snapshot is consistent.
func TestWorkloadSourceConcurrentReaders(t *testing.T) {
	e := newWorkloadEnv(t, pullTestConfig, workloadFixture{name: "none"})
	f := newNotifySched(map[string]scheduler.Workload{})
	e.ga.slurmScheduler = f
	src := runningSource(t, e, time.Millisecond)
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
				_, _ = src.lastGood()
			}
		}()
	}
	for i := 1; i <= 100; i++ {
		want := uint64(i + 1)
		f.set(map[string]scheduler.Workload{"0": testJob(fmt.Sprint(i), "u")})
		if !waitWithin(time.Second, func() bool { return src.load().gen == want }) {
			t.Fatalf("change %d not published", i)
		}
		if i == 80 {
			src.stop()
			break
		}
	}
	close(stop)
	wg.Wait()
	assert.Equal(t, bad.Load(), int64(0))
}

// TestWorkloadSourceSurvivesReconnect: across gpuagent reconnects the
// exporter-owned k8s client is neither closed nor replaced, exactly one source
// goroutine runs, polls keep listing with no gen bump, and the hr labels and
// pull /metrics keep the pod; after the reconnects the source still publishes
// a change.
func TestWorkloadSourceSurvivesReconnect(t *testing.T) {
	e := newWorkloadEnv(t, hrTestConfig, fixtureByName("k8s"))
	hc, src := e.startHRSource(t, 20*time.Millisecond)
	e.gpu.pullWLSource.Store(src)
	k8s := e.ga.k8sScheduler
	gen := src.load().gen
	hasPod := func() bool {
		for _, l := range *hc.labels.Load() {
			if l["pod"] != "" {
				return true
			}
		}
		return false
	}
	assert.Assert(t, hasPod())
	for i := 1; i <= 3; i++ {
		before, _ := e.k8s.counts()
		assert.NilError(t, e.ga.reconnect())
		_, closes := e.k8s.counts()
		assert.Assert(t, e.ga.k8sScheduler == k8s, "cycle %d: k8s client replaced", i)
		assert.Equal(t, closes, 0, "cycle %d: gpuagent closed the exporter-owned k8s client", i)
		assert.Assert(t, e.ga.isKubernetes)
		assert.Equal(t, sourceGoroutinesOf(src), 1, "cycle %d: source goroutines", i)
		assert.Assert(t, waitWithin(2*time.Second, func() bool { l, _ := e.k8s.counts(); return l > before }),
			"cycle %d: poll did not list", i)
		assert.Assert(t, src.load().err == nil, "cycle %d: failed snapshot", i)
		assert.Equal(t, src.load().gen, gen, "cycle %d: gen changed with unchanged content", i)
		hc.refreshOnce()
		assert.Assert(t, hasPod(), "cycle %d: hr labels lost the pod", i)
		assert.Assert(t, strings.Contains(e.renderPull(t), `pod="pod0"`), "cycle %d: pull /metrics lost the pod", i)
	}
	e.k8s.set(map[string]scheduler.Workload{"0000:01:00.0": testPod(7)}, nil)
	assert.Assert(t, waitWithin(2*time.Second, func() bool { return src.load().gen == gen+1 }),
		"source stopped after reconnect")
	wls, ok := e.gpu.pullWLSource.Load().lastGood()
	assert.Assert(t, ok)
	assert.Equal(t, wls["0000:01:00.0"].Info.(scheduler.PodResourceInfo).Pod, "pod7")
}
