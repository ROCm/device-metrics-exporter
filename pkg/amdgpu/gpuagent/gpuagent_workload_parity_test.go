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

// GPUOP-1140 /metrics parity against the base goldens (testdata/golden-base,
// captured from b2efd1b32; never regenerated here), kubelet call count, and
// reconnect behaviour (test plan cases 14-16, 18, 39, 40, 76, 78, 80, 81, 87,
// 88).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

const goldenDir = "testdata/golden-base"

func golden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(goldenDir, name))
	assert.NilError(t, err)
	return string(b)
}

// assertGolden compares got byte for byte with the base golden.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	want := golden(t, name)
	if got != want {
		t.Fatalf("%s: /metrics differs from base golden\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func fixtureByName(name string) oracleFixture {
	for _, fx := range oracleFixtures() {
		if fx.name == name {
			return fx
		}
	}
	panic("no fixture " + name)
}

// checkParity renders the three modes on the post-change code and compares
// them with the base goldens: pull without cache (per-scrape ListWorkloads),
// pull with the gate over the shared source, and store-served high-res with
// the source-backed seed and label cache.
func checkParity(t *testing.T, name string) {
	fx := fixtureByName(name)
	t.Run("pull-sync", func(t *testing.T) {
		e := newOracleEnv(t, oraclePullConfig, fx)
		assertGolden(t, name+".pull-sync.metrics", e.renderPull(t))
	})
	t.Run("pull-cache", func(t *testing.T) {
		e := newOracleEnv(t, oraclePullConfig, fx)
		src := runningSource(t, e, time.Hour)
		e.gpu.pullWLCache.Store(newPullWorkloadCache(src))
		assertGolden(t, name+".pull-cache.metrics", e.renderPull(t))
	})
	t.Run("hr", func(t *testing.T) {
		e := newOracleEnv(t, oracleHRConfig, fx)
		hc, _ := e.startHRSource(t, 0)
		assertGolden(t, name+".hr.metrics", e.renderHR(t))
		assertGolden(t, name+".hr.labels.json", labelDump(t, hc.labels.Load()))
	})
}

func TestMetricsParityK8s(t *testing.T)              { checkParity(t, "k8s") }
func TestMetricsParitySlurm(t *testing.T)            { checkParity(t, "slurm") }
func TestMetricsParityMixed(t *testing.T)            { checkParity(t, "mixed") }
func TestMetricsParityHostileJobFields(t *testing.T) { checkParity(t, "hostile") }

// TestMetricsParityK8sFailure (case 80) and TestK8sRecoveryRestoresLabels (case
// 81): with the kubelet failing, /metrics equals the base failure goldens in all
// three modes; on recovery the hr output equals the base recovered golden.
func TestMetricsParityK8sFailure(t *testing.T) {
	for _, name := range []string{"k8s", "mixed"} {
		fx := fixtureByName(name)
		t.Run(name+"/pull-sync", func(t *testing.T) {
			e := newOracleEnv(t, oraclePullConfig, fx)
			e.k8s.set(fx.k8s, errOracleKubelet)
			assertGolden(t, name+"-k8sfail.pull-sync.metrics", e.renderPull(t))
			wls, err := e.ga.ListWorkloads()
			assertGolden(t, name+"-k8sfail.listworkloads.txt",
				fmt.Sprintf("len(wls)=%d nil=%v err=%v\n", len(wls), wls == nil, err))
		})
		t.Run(name+"/pull-cache", func(t *testing.T) {
			e := newOracleEnv(t, oraclePullConfig, fx)
			src := manualSource(e)
			e.gpu.pullWLCache.Store(newPullWorkloadCache(src))
			e.k8s.set(fx.k8s, errOracleKubelet)
			src.refresh(true)
			assert.Assert(t, src.load().err != nil, "failed snapshot not published")
			assertGolden(t, name+"-k8sfail.pull-cache.metrics", e.renderPull(t))
			// the failed snapshot is today's ListWorkloads result for the failure
			wls, err := e.gpu.currentWorkloads()
			assertGolden(t, name+"-k8sfail.listworkloads.txt",
				fmt.Sprintf("len(wls)=%d nil=%v err=%v\n", len(wls), wls == nil, err))
		})
		t.Run(name+"/hr", func(t *testing.T) {
			e := newOracleEnv(t, oracleHRConfig, fx)
			hc, src := e.startHRSource(t, 0)
			e.k8s.set(fx.k8s, errOracleKubelet)
			src.refresh(true)
			hc.refreshOnce(context.Background())
			assertGolden(t, name+"-k8sfail.hr.metrics", e.renderHR(t))
			assertGolden(t, name+"-k8sfail.hr.labels.json", labelDump(t, hc.labels.Load()))
		})
	}
}

func TestK8sRecoveryRestoresLabels(t *testing.T) {
	for _, name := range []string{"k8s", "mixed"} {
		fx := fixtureByName(name)
		t.Run(name, func(t *testing.T) {
			e := newOracleEnv(t, oracleHRConfig, fx)
			hc, src := e.startHRSource(t, 0)
			frame := hrPartial(oracleGPUIDs(), hrPowerTick(100)...)
			e.gpu.applyPowerFrame(frame) // warm the hot cache
			hot := e.gpu.hrHotRebuilds.Load()

			e.k8s.set(fx.k8s, errOracleKubelet)
			src.refresh(true)
			gen := src.load().gen
			hc.refreshOnce(context.Background())
			e.gpu.applyPowerFrame(frame)
			assert.Equal(t, e.gpu.hrHotRebuilds.Load(), hot+1, "hot cache must rebuild once for the drop")

			e.k8s.set(fx.k8s, nil)
			src.refresh(true)
			assert.Equal(t, src.load().gen, gen+1, "recovery must bump gen")
			hc.refreshOnce(context.Background())
			e.gpu.applyPowerFrame(frame)
			e.gpu.applyPowerFrame(frame)
			assert.Equal(t, e.gpu.hrHotRebuilds.Load(), hot+2, "hot cache must rebuild once for the restore")
			assertGolden(t, name+"-k8sfail-recovered.hr.metrics", e.renderHR(t))
			assertGolden(t, name+".hr.labels.json", labelDump(t, hc.labels.Load()))
		})
	}
}

// oracleGPUIDs returns the oracle GPUs' 16-byte ids.
func oracleGPUIDs() [][]byte {
	var ids [][]byte
	for _, g := range oracleGPUResp(oracleGPUs).Response {
		ids = append(ids, g.Spec.Id)
	}
	return ids
}

// TestKubeletListCountOneSource (case 18): with the hr label cache, the
// profiler label cache, the pull gate and health all active over a real
// kubelet client and a fake PodResources server, every List comes from the
// source: exactly one per interval (plus the seed); slurm events add none.
func TestKubeletListCountOneSource(t *testing.T) {
	fk := newFakeKubelet(t, kubeResp("0000:01:00.0", "0000:02:00.0"))
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("k8s"))
	e.ga.k8sScheduler = fk.client(t)
	slurm := newNotifySched(map[string]scheduler.Workload{"3": oracleJob("103", "dave")})
	e.ga.slurmScheduler = slurm
	const interval = 50 * time.Millisecond
	e.gpu.wlInterval = interval
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	hc, src := e.startHRSource(t, interval)
	go hc.start(ctx, hrWorkloadLabelRefreshInterval)
	pc := newProfilerLabelCache(e.gpu)
	go pc.start(ctx, hrWorkloadLabelRefreshInterval)
	e.gpu.pullWLCache.Store(newPullWorkloadCache(src))

	base := fk.calls.Load()
	srcBase := src.k8sCalls.Load()
	assert.Equal(t, base, srcBase, "List calls before the window must all be the seed")
	start := time.Now()
	for i := 0; i < 20; i++ {
		// health (both sites read currentWorkloads), pull gate, slurm event
		_, _ = e.gpu.currentWorkloads()
		e.gpu.updateAllGPUsHealthState("healthy")
		_, _ = e.gpu.pullWLCache.Load().get()
		slurm.set(map[string]scheduler.Workload{"3": oracleJob(fmt.Sprint(200+i), "dave")})
		time.Sleep(interval)
	}
	elapsed := time.Since(start)
	calls := fk.calls.Load() - base
	polls := src.k8sCalls.Load() - srcBase
	t.Logf("kubelet List calls in %v (%d intervals of %v): %d; source polls: %d; slurm event refreshes: %d",
		elapsed.Round(time.Millisecond), 20, interval, calls, polls, src.evRefreshes.Load())
	assert.Equal(t, calls, polls, "every kubelet List must come from the source")
	maxPolls := int64(elapsed/interval) + 1
	// lower bound loose: a slow refresh under -race drops ticks
	assert.Assert(t, polls <= maxPolls && polls >= int64(elapsed/interval)/2,
		"want one List per interval: %d polls in %v (max %d)", polls, elapsed, maxPolls)
	assert.Assert(t, src.evRefreshes.Load() > 0, "slurm events did not refresh the source")
}

// TestK8sLabelsSurviveReconnect (case 40, fixed behaviour; base: labels lost
// after the first reconnect, see characterization/case40): the exporter-owned
// k8s client is neither closed nor dropped, polls keep listing, gen is
// unchanged and hr labels and pull /metrics keep the pod labels.
func TestK8sLabelsSurviveReconnect(t *testing.T) {
	e := newOracleEnv(t, oracleHRConfig, fixtureByName("k8s"))
	hc, src := e.startHRSource(t, 0)
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
		src.refresh(true) // the next poll
		lists, _ := e.k8s.counts()
		assert.Equal(t, lists, before+1, "cycle %d: poll did not list", i)
		assert.Assert(t, src.load().err == nil, "cycle %d: failed snapshot", i)
		assert.Equal(t, src.load().gen, gen, "cycle %d: gen changed with unchanged content", i)
		hc.refreshOnce(context.Background())
		assert.Assert(t, hasPod(), "cycle %d: hr labels lost the pod", i)
		assert.Assert(t, strings.Contains(e.renderPull(t), `pod="pod0"`), "cycle %d: pull /metrics lost the pod", i)
	}
}

// TestSharedK8sClientSurvivesGPUReconnect (case 87, fixed behaviour; base: the
// second borrower fails with "client connection is closing"): a second borrower
// of the same real kubelet client keeps listing across gpuagent reconnects.
func TestSharedK8sClientSurvivesGPUReconnect(t *testing.T) {
	fk := newFakeKubelet(t, kubeResp("0000:01:00.0"))
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("k8s"))
	shared := fk.client(t)
	e.ga.k8sScheduler = shared // what exporter.go injects into gpuagent and nicagent
	borrower := shared
	for i := 1; i <= 3; i++ {
		assert.NilError(t, e.ga.reconnect())
		wls, err := borrower.ListWorkloads()
		assert.NilError(t, err, "cycle %d: second borrower broken by gpuagent reconnect", i)
		assert.Equal(t, len(wls), 1)
		wls, err = e.ga.ListWorkloads()
		assert.NilError(t, err)
		assert.Equal(t, len(wls), 1)
	}
}

// TestWorkloadSourceSurvivesReconnect (case 39): the source outlives the
// handler context cancelled by reconnect and keeps publishing changes.
func TestWorkloadSourceSurvivesReconnect(t *testing.T) {
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("k8s"))
	src := runningSource(t, e, 20*time.Millisecond)
	e.gpu.pullWLCache.Store(newPullWorkloadCache(src))
	gen := src.load().gen
	assert.NilError(t, e.ga.reconnect())
	e.k8s.set(map[string]scheduler.Workload{"0000:01:00.0": oraclePod(7)}, nil)
	assert.Assert(t, waitWithin(2*time.Second, func() bool { return src.load().gen == gen+1 }),
		"source stopped after reconnect")
	wls, ok := e.gpu.pullWLCache.Load().get()
	assert.Assert(t, ok)
	assert.Equal(t, wls["0000:01:00.0"].Info.(scheduler.PodResourceInfo).Pod, "pod7")
}

// slurmFactory replaces newSlurmScheduler with real slurm clients on dir and
// tracks how many are live (created and not closed).
type slurmFactory struct {
	created atomic.Int64
	closed  atomic.Int64
}

type trackedSlurm struct {
	scheduler.SchedulerClient
	f *slurmFactory
}

func (c *trackedSlurm) Close() error { c.f.closed.Add(1); return c.SchedulerClient.Close() }

// Version/Changed forward the ChangeNotifier of the wrapped client.
func (c *trackedSlurm) Version() uint64 {
	return c.SchedulerClient.(scheduler.ChangeNotifier).Version()
}
func (c *trackedSlurm) Changed() <-chan struct{} {
	return c.SchedulerClient.(scheduler.ChangeNotifier).Changed()
}

func useSlurmDir(t *testing.T, dir string) *slurmFactory {
	t.Helper()
	f := &slurmFactory{}
	orig := newSlurmScheduler
	newSlurmScheduler = func(ctx context.Context) (scheduler.SchedulerClient, error) {
		cl, err := scheduler.NewSlurmClientForDir(ctx, dir)
		if err != nil {
			return nil, err
		}
		f.created.Add(1)
		return &trackedSlurm{SchedulerClient: cl, f: f}, nil
	}
	t.Cleanup(func() { newSlurmScheduler = orig })
	return f
}

// TestSlurmSingleWatcherAcrossReconnect (case 88): one live slurm client (one
// watcher) across reconnects; the source follows the re-created client and slurm
// labels match the directory; a job written after the reconnect shows up fast.
func TestSlurmSingleWatcherAcrossReconnect(t *testing.T) {
	dir := t.TempDir()
	f := useSlurmDir(t, dir)
	writeSlurmJob(t, dir, "1", "501", "1")
	e := newOracleEnv(t, oraclePullConfig, fixtureByName("slurm"))
	e.ga.slurmScheduler = nil
	e.ga.enableSlurmScl = true
	assert.NilError(t, e.ga.initalizeScheduler())
	assert.NilError(t, e.ga.initalizeScheduler()) // live client: no second one
	assert.Equal(t, f.created.Load(), int64(1))
	src := runningSource(t, e, 20*time.Millisecond)
	assert.Assert(t, waitWithin(time.Second, func() bool { return jobID(src.load().wls, "1") == "501" }))
	for i := 1; i <= 3; i++ {
		gen := src.load().gen
		assert.NilError(t, e.ga.reconnect())
		assert.Equal(t, f.created.Load()-f.closed.Load(), int64(1), "cycle %d: live slurm clients", i)
		// the old client's watcher is cancelled: a job is only seen through the
		// new client, which the source picks up on its next tick
		time.Sleep(60 * time.Millisecond)
		assert.Equal(t, jobID(src.load().wls, "1"), "501")
		assert.Equal(t, src.load().gen, gen, "cycle %d: same content must not bump", i)
		id := fmt.Sprint(600 + i)
		writeSlurmJob(t, dir, "2", id, "2")
		assert.Assert(t, waitWithin(100*time.Millisecond, func() bool { return jobID(src.load().wls, "2") == id }),
			"cycle %d: job after reconnect not seen within 100ms", i)
	}
	e.ga.Close()
	assert.Equal(t, f.created.Load(), f.closed.Load(), "slurm client leaked")
}
