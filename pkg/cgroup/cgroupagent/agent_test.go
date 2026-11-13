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

package cgroupagent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

const (
	pullConfig    = `{"CommonConfig": {"MetricsFieldPrefix": "amd_"}}`
	highResConfig = `{"CommonConfig": {"MetricsFieldPrefix": "amd_", "HighResolution": {"Enable": true}}}`

	memCurrent = "amd_cgroup_memory_current_bytes"
	memLimit   = "amd_cgroup_memory_limit_bytes"
	memVirtual = "amd_cgroup_memory_virtual_bytes"
	cpuSeconds = "amd_cgroup_cpu_usage_seconds_total"
)

func writeFile(t testing.TB, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// addJob writes a job cgroup with the given memory.current and memory.max.
func addJob(t testing.TB, root, path string, memCurrent int, memMax string) {
	t.Helper()
	dir := filepath.Join(root, path)
	for name, body := range map[string]string{
		"cpu.stat":       "usage_usec 3000000\nnr_periods 0\nnr_throttled 0\nthrottled_usec 0\n",
		"memory.current": fmt.Sprintf("%d\n", memCurrent),
		"memory.max":     memMax + "\n",
		"memory.stat":    "anon 1\ninactive_file 0\n",
		"io.stat":        "8:0 rbytes=1 wbytes=2 rios=3 wios=4\n",
		"cgroup.procs":   "",
	} {
		writeFile(t, filepath.Join(dir, name), body)
	}
}

type harness struct {
	t       testing.TB
	cfgPath string
	root    string
	proc    string
	mh      *metricsutil.MetricsHandler
	agent   *Agent
}

func newHarness(t testing.TB, cfgJSON string, opts ...Option) *harness {
	t.Helper()
	logger.Init(false)
	h := &harness{t: t, cfgPath: filepath.Join(t.TempDir(), "config.json"), root: t.TempDir(), proc: t.TempDir()}
	writeFile(t, filepath.Join(h.root, "cgroup.controllers"), "cpu memory io\n")
	h.setConfig(cfgJSON)
	mh, err := metricsutil.NewMetrics(config.NewConfigHandler(h.cfgPath, config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort}))
	if err != nil {
		t.Fatal(err)
	}
	h.mh = mh
	h.agent = NewAgent(mh, append([]Option{WithHostPaths(h.root, h.proc)}, opts...)...)
	h.agent.hostname = "node1"
	if err := h.agent.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.agent.Close)
	return h
}

func (h *harness) setConfig(cfgJSON string) {
	h.t.Helper()
	writeFile(h.t, h.cfgPath, cfgJSON)
}

func (h *harness) reload() { h.mh.InitConfig(context.Background()) }

type series struct {
	labels map[string]string
	value  float64
}

// scrape does what a /metrics request does for store-serving clients.
func (h *harness) scrape() map[string][]series {
	h.t.Helper()
	if err := h.mh.RefreshStoreServingClients(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	return h.gather()
}

func (h *harness) gather() map[string][]series {
	h.t.Helper()
	fams, err := h.mh.GetRegistry().Gather()
	if err != nil {
		h.t.Fatal(err)
	}
	out := map[string][]series{}
	for _, mf := range fams {
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			v := m.GetGauge().GetValue()
			if m.GetCounter() != nil {
				v = m.GetCounter().GetValue()
			}
			out[mf.GetName()] = append(out[mf.GetName()], series{labels, v})
		}
	}
	for _, s := range out {
		sort.Slice(s, func(i, j int) bool { return s[i].labels["job_id"] < s[j].labels["job_id"] })
	}
	return out
}

func TestExportsJobCgroupsOnly(t *testing.T) {
	h := newHarness(t, pullConfig)
	addJob(t, h.root, "spur/job_42_0", 100, "1073741824")
	addJob(t, h.root, "system.slice/slurmstepd.scope/job_7", 200, "max")
	addJob(t, h.root, "system.slice/slurmstepd.scope/job_7/step_0", 50, "max")
	addJob(t, h.root, "system.slice/sshd.service", 999, "max")
	h.reload()

	got := h.scrape()
	for name := range got {
		if !strings.HasPrefix(name, "amd_cgroup_") {
			t.Errorf("family %q lacks the configured prefix", name)
		}
	}
	mem := got[memCurrent]
	if len(mem) != 2 {
		t.Fatalf("memory series = %v; want the two job cgroups only", mem)
	}
	if mem[0].labels["job_id"] != "42" || mem[0].value != 100 || mem[0].labels["hostname"] != "node1" {
		t.Errorf("spur job series = %+v", mem[0])
	}
	if mem[1].labels["job_id"] != "7" || mem[1].value != 200 {
		t.Errorf("slurm job series = %+v", mem[1])
	}
	if limits := got[memLimit]; len(limits) != 1 || limits[0].labels["job_id"] != "42" {
		t.Errorf("limit series = %v; want only the limited job", limits)
	}
	if cpu := got[cpuSeconds]; len(cpu) != 2 || cpu[0].value != 3 {
		t.Errorf("cpu series = %v; want 3 seconds per job", cpu)
	}
	if v, ok := got[memVirtual]; ok {
		t.Errorf("virtual memory exported with no resolvable process: %v", v)
	}
}

func TestEndedJobAndDroppedFieldDisappear(t *testing.T) {
	h := newHarness(t, pullConfig)
	addJob(t, h.root, "spur/job_1_0", 1, "1073741824")
	addJob(t, h.root, "spur/job_2_0", 2, "1073741824")
	h.reload()
	if n := len(h.scrape()[memCurrent]); n != 2 {
		t.Fatalf("series before = %d; want 2", n)
	}

	if err := os.RemoveAll(filepath.Join(h.root, "spur/job_1_0")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.root, "spur/job_2_0/memory.max"), "max\n")
	got := h.scrape()
	if mem := got[memCurrent]; len(mem) != 1 || mem[0].labels["job_id"] != "2" {
		t.Errorf("series after = %v; want only job 2", mem)
	}
	if limits := got[memLimit]; len(limits) != 0 {
		t.Errorf("limit served stale after it became unlimited: %v", limits)
	}
}

func TestRequeuedJobKeepsHighestAttempt(t *testing.T) {
	h := newHarness(t, pullConfig)
	addJob(t, h.root, "spur/job_5_0", 10, "max")
	addJob(t, h.root, "spur/job_5_1", 20, "max")
	h.reload()
	if mem := h.scrape()[memCurrent]; len(mem) != 1 || mem[0].value != 20 {
		t.Errorf("series = %v; want one series from attempt 1", mem)
	}
}

func TestVirtualMemoryOnlyWhenProcessesResolve(t *testing.T) {
	h := newHarness(t, pullConfig)
	addJob(t, h.root, "spur/job_1_0", 1, "max")
	addJob(t, h.root, "spur/job_2_0", 1, "max")
	writeFile(t, filepath.Join(h.root, "spur/job_1_0/cgroup.procs"), "5\n")
	writeFile(t, filepath.Join(h.proc, "5/status"), "VmSize:\t 4 kB\n")
	// Seen from outside the host PID namespace.
	writeFile(t, filepath.Join(h.root, "spur/job_2_0/cgroup.procs"), "0\n")
	h.reload()

	if v := h.scrape()[memVirtual]; len(v) != 1 || v[0].labels["job_id"] != "1" || v[0].value != 4096 {
		t.Errorf("virtual memory series = %v; want 4096 for job 1 only", v)
	}
}

func TestJobLabels(t *testing.T) {
	jobs := map[string]scheduler.JobInfo{"42": {Id: "42", User: "alice", Partition: "gpu", Account: "research"}}
	h := newHarness(t, pullConfig, WithJobLookup(func(id string) (scheduler.JobInfo, bool) {
		j, ok := jobs[id]
		return j, ok
	}))
	addJob(t, h.root, "spur/job_42_0", 1, "max")
	addJob(t, h.root, "spur/job_43_0", 2, "max")
	h.reload()

	mem := h.scrape()[memCurrent]
	if len(mem) != 2 {
		t.Fatalf("series = %v; want 2", mem)
	}
	want := map[string]string{"hostname": "node1", "job_id": "42", "job_user": "alice", "job_partition": "gpu", "job_account": "research"}
	if !reflect.DeepEqual(mem[0].labels, want) {
		t.Errorf("labels = %v; want %v", mem[0].labels, want)
	}
	want = map[string]string{"hostname": "node1", "job_id": "43", "job_user": "", "job_partition": "", "job_account": ""}
	if !reflect.DeepEqual(mem[1].labels, want) {
		t.Errorf("job without a file = %v; want %v", mem[1].labels, want)
	}
}

func TestCgroupV1HostExportsNothing(t *testing.T) {
	h := newHarness(t, pullConfig)
	if err := os.Remove(filepath.Join(h.root, "cgroup.controllers")); err != nil {
		t.Fatal(err)
	}
	addJob(t, h.root, "spur/job_1_0", 1, "max")
	h.reload()
	if got := h.scrape(); len(got) != 0 {
		t.Errorf("exported %v on a cgroup v1 host", got)
	}
}

func TestPrefixChangeOnReload(t *testing.T) {
	h := newHarness(t, pullConfig)
	addJob(t, h.root, "spur/job_1_0", 1, "max")
	h.reload()
	h.setConfig(`{"CommonConfig": {"MetricsFieldPrefix": "site_"}}`)
	h.reload()
	got := h.scrape()
	if _, ok := got[memCurrent]; ok {
		t.Error("old prefix still served after reload")
	}
	if n := len(got["site_cgroup_memory_current_bytes"]); n != 1 {
		t.Errorf("series under the new prefix = %d; want 1", n)
	}
}

// manualTicker hands out one unbuffered channel: a second send can only
// complete once the feed has finished the publish the first send started.
type manualTicker chan time.Time

func (m manualTicker) ticker(time.Duration) (<-chan time.Time, func()) { return m, func() {} }

func TestHighResolutionFeedOwnsTheStore(t *testing.T) {
	tick := make(manualTicker)
	h := newHarness(t, highResConfig, WithTicker(tick.ticker))
	addJob(t, h.root, "spur/job_1_0", 1, "max")
	h.reload()
	if mem := h.gather()[memCurrent]; len(mem) != 1 || mem[0].value != 1 {
		t.Fatalf("seeded series = %v; want 1", mem)
	}

	writeFile(t, filepath.Join(h.root, "spur/job_1_0/memory.current"), "2\n")
	tick <- time.Time{}
	tick <- time.Time{}
	if mem := h.gather()[memCurrent]; len(mem) != 1 || mem[0].value != 2 {
		t.Fatalf("series after feed ticks = %v; want 2", mem)
	}

	h.agent.StopServices()
	writeFile(t, filepath.Join(h.root, "spur/job_1_0/memory.current"), "3\n")
	if mem := h.scrape()[memCurrent]; len(mem) != 1 || mem[0].value != 2 {
		t.Errorf("scrape in high-resolution mode = %v; want the feed's 2, not a fresh read", mem)
	}
}

// TestFeedScrapesAndReloadTogether is for the race detector: the feed, pull
// scrapes and reloads that toggle high-resolution all run at once.
func TestFeedScrapesAndReloadTogether(t *testing.T) {
	tick := make(manualTicker)
	h := newHarness(t, highResConfig, WithTicker(tick.ticker))
	addJob(t, h.root, "spur/job_1_0", 1, "max")
	h.reload()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for {
			select {
			case tick <- time.Time{}:
			case <-done:
				return
			}
		}
	}()
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if err := h.agent.UpdateMetricsStats(context.Background()); err != nil {
					t.Error(err)
				}
				h.mh.GetStore().Snapshots()
			}
		}()
	}
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			h.setConfig(pullConfig)
		} else {
			h.setConfig(highResConfig)
		}
		h.reload()
	}
	close(done)
	wg.Wait()

	mem := h.scrape()[memCurrent]
	if len(mem) != 1 || mem[0].labels["job_id"] != "1" {
		t.Errorf("series after concurrent reloads = %v; want exactly job 1", mem)
	}
}

func TestRepeatedWarningLoggedOnce(t *testing.T) {
	h := newHarness(t, pullConfig)
	addJob(t, h.root, "spur/job_1_1", 1, "max")
	addJob(t, h.root, "spur/job_2_1", 1, "max")
	addJob(t, h.root, "system.slice/slurmstepd.scope/job_2", 1, "max")
	var logs bytes.Buffer
	logger.Log.SetOutput(&logs)
	t.Cleanup(func() { logger.Init(false) })
	count := func(s string) int { return strings.Count(logs.String(), s) }
	breakJob := func(body string) { writeFile(t, filepath.Join(h.root, "spur/job_1_1/memory.current"), body) }

	breakJob("not-a-number\n")
	h.reload()
	for i := 0; i < 3; i++ {
		if err := h.agent.publish(); err != nil {
			t.Fatal(err)
		}
	}
	if n := count("read cgroup /spur/job_1_1"); n != 1 {
		t.Errorf("read warning logged %d times over 4 samples; want 1", n)
	}
	if n := count("job cgroup /system.slice/slurmstepd.scope/job_2 not exported"); n != 1 {
		t.Errorf("shadowed job warning logged %d times over 4 samples; want 1", n)
	}

	breakJob("1\n")
	if err := h.agent.publish(); err != nil {
		t.Fatal(err)
	}
	breakJob("not-a-number\n")
	if err := h.agent.publish(); err != nil {
		t.Fatal(err)
	}
	if n := count("read cgroup /spur/job_1_1"); n != 2 {
		t.Errorf("read warning logged %d times after it cleared and came back; want 2", n)
	}
}

func BenchmarkPublish(b *testing.B) {
	for _, jobs := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("jobs=%d", jobs), func(b *testing.B) {
			h := newHarness(b, pullConfig)
			for i := 0; i < jobs; i++ {
				addJob(b, h.root, fmt.Sprintf("spur/job_%d_0", i+1), i, "max")
			}
			h.reload()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := h.agent.publish(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
