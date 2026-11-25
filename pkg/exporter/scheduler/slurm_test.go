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

package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

func TestReadJobFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("42", `{"SLURM_JOB_ID": "42", "SLURM_JOB_USER": "alice", "SLURM_JOB_PARTITION": "gpu",
		"SLURM_JOB_ACCOUNT": "research", "SLURM_CLUSTER_NAME": "c1"}`)
	write("43", `{"SLURM_JOB_ID": "99"}`)
	write("44", `not json`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "secret"), []byte(`{"SLURM_JOB_ID": "../secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := ReadJobFile(dir, "42")
	want := JobInfo{Id: "42", User: "alice", Partition: "gpu", Account: "research", Cluster: "c1"}
	if !ok || got != want {
		t.Errorf("ReadJobFile(42) = %+v, %v; want %+v", got, ok, want)
	}
	for _, id := range []string{"43", "44", "45", "", "..", "../secret"} {
		if got, ok := ReadJobFile(dir, id); ok {
			t.Errorf("ReadJobFile(%q) = %+v; want not found", id, got)
		}
	}
}

// runProlog runs the bundled prolog with env, writing under a temp dir instead
// of /var/run/exporter, and returns that dir.
func runProlog(t *testing.T, env []string) string {
	t.Helper()
	src, err := os.ReadFile("../../../debian/usr/local/etc/metrics/slurm/slurm-prolog.sh")
	if err != nil {
		t.Fatal(err)
	}
	const exportDir = `EXPORT_DIR="/var/run/exporter/"`
	if !bytes.Contains(src, []byte(exportDir)) {
		t.Fatalf("prolog no longer sets %s", exportDir)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "prolog.sh")
	src = bytes.Replace(src, []byte(exportDir), []byte(`EXPORT_DIR="`+dir+`/"`), 1)
	if err := os.WriteFile(script, src, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script)
	cmd.Env = append([]string{"PATH=/usr/bin:/bin"}, env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prolog: %v\n%s", err, out)
	}
	return dir
}

func TestPrologEscapesJobFields(t *testing.T) {
	for _, account := range []string{`x","SLURM_JOB_USER":"alice`, `back\slash`, "plain"} {
		dir := runProlog(t, []string{
			"SLURM_JOB_ID=7", "SLURM_JOB_USER=bob", "SLURM_JOB_PARTITION=gpu",
			"SLURM_JOB_ACCOUNT=" + account, "SLURM_JOB_GPUS=1",
		})
		want := JobInfo{Id: "7", User: "bob", Partition: "gpu", Account: account}
		if got, ok := ReadJobFile(filepath.Join(dir, "jobs"), "7"); !ok || got != want {
			t.Errorf("account %q: job file = %+v, %v; want %+v", account, got, ok, want)
		}
		buf, err := os.ReadFile(filepath.Join(dir, "1"))
		if err != nil {
			t.Fatal(err)
		}
		var env map[string]string
		if err := json.Unmarshal(buf, &env); err != nil {
			t.Fatalf("account %q: GPU file is not JSON: %v\n%s", account, err, buf)
		}
		if got := jobInfoFromEnv(env); got != want {
			t.Errorf("account %q: GPU file = %+v; want %+v", account, got, want)
		}
	}
}

// slurmSettle is how long a negative check waits for an event that must not
// change anything.
const slurmSettle = 200 * time.Millisecond

// newTestSlurm starts a slurm client watching a fresh temp dir.
func newTestSlurm(t *testing.T) (*client, string) {
	t.Helper()
	logger.Init(true)
	dir := filepath.Join(t.TempDir(), "slurm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cl, err := newSlurmClient(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cl.Close() })
	return cl, dir
}

func jobJSON(id, gpus string) string {
	return jobJSONUser(id, "u"+id, gpus)
}

func jobJSONUser(id, user, gpus string) string {
	return fmt.Sprintf(`{"SLURM_JOB_ID":"%s","SLURM_JOB_USER":"%s","SLURM_JOB_PARTITION":"gpu",`+
		`"SLURM_CLUSTER_NAME":"c1","SLURM_JOB_ACCOUNT":"a%s","CUDA_VISIBLE_DEVICES":"%s"}`, id, user, id, gpus)
}

func jobOf(id string) JobInfo {
	return JobInfo{Id: id, User: "u" + id, Partition: "gpu", Cluster: "c1", Account: "a" + id}
}

func writeJobFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func slurmJobs(cl *client) map[string]JobInfo {
	cl.Lock()
	defer cl.Unlock()
	out := make(map[string]JobInfo, len(cl.GpuJobs))
	for k, v := range cl.GpuJobs {
		out[k] = v
	}
	return out
}

// waitSlurm polls until the client has version v and jobs want, or fails.
func waitSlurm(t *testing.T, cl *client, v uint64, want map[string]JobInfo) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		gotV, got := cl.Version(), slurmJobs(cl)
		if gotV == v && reflect.DeepEqual(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slurm state = version %d jobs %v; want version %d jobs %v", gotV, got, v, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// expectSlurm waits slurmSettle, then requires version v and jobs want (for
// events that must not change anything).
func expectSlurm(t *testing.T, cl *client, v uint64, want map[string]JobInfo) {
	t.Helper()
	time.Sleep(slurmSettle)
	if gotV, got := cl.Version(), slurmJobs(cl); gotV != v || !reflect.DeepEqual(got, want) {
		t.Fatalf("slurm state = version %d jobs %v; want version %d jobs %v", gotV, got, v, want)
	}
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// TestSlurmVersionBump: Version starts at 0 and bumps once per
// content change: a create, a two-GPU file (one bump), a remove.
// ListWorkloads still returns a copy.
func TestSlurmVersionBump(t *testing.T) {
	cl, dir := newTestSlurm(t)
	var _ ChangeNotifier = cl
	if v := cl.Version(); v != 0 {
		t.Fatalf("initial Version = %d; want 0", v)
	}
	writeJobFile(t, dir, "0", jobJSON("10", "0"))
	waitSlurm(t, cl, 1, map[string]JobInfo{"0": jobOf("10")})
	writeJobFile(t, dir, "1", jobJSON("11", "1,2"))
	waitSlurm(t, cl, 2, map[string]JobInfo{"0": jobOf("10"), "1": jobOf("11"), "2": jobOf("11")})
	if err := os.Remove(filepath.Join(dir, "0")); err != nil {
		t.Fatal(err)
	}
	waitSlurm(t, cl, 3, map[string]JobInfo{"1": jobOf("11"), "2": jobOf("11")})

	wls, err := cl.ListWorkloads()
	if err != nil || len(wls) != 2 || wls["1"] != (Workload{Type: Slurm, Info: jobOf("11")}) {
		t.Fatalf("ListWorkloads = %v, %v", wls, err)
	}
	delete(wls, "1")
	if len(slurmJobs(cl)) != 2 {
		t.Error("ListWorkloads did not return a copy")
	}
	expectSlurm(t, cl, 3, map[string]JobInfo{"1": jobOf("11"), "2": jobOf("11")})
}

// TestSlurmChangedBroadcast: Changed closes on the first change after the
// call, wakes every waiter and is replaced after each bump. Two latencies: bump-to-wake (< 100 ms) and
// job-file-write-to-wake through inotify (< 1 s, well under the 3 s label
// tick; the file path includes inotify delivery and -race overhead). Readers
// run concurrently for -race.
func TestSlurmChangedBroadcast(t *testing.T) {
	cl, dir := newTestSlurm(t)
	ch := cl.Changed()
	if isClosed(ch) {
		t.Fatal("Changed closed before any change")
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = cl.Version()
					_ = cl.Changed()
					_, _ = cl.ListWorkloads()
					runtime.Gosched()
				}
			}
		}()
	}
	defer func() { close(stop); readers.Wait() }()

	// waitAll starts waiters on ch, runs trigger and requires all of them to
	// wake within bound of the trigger.
	waitAll := func(ch <-chan struct{}, bound time.Duration, trigger func()) {
		t.Helper()
		const waiters = 10
		woke := make(chan time.Time, waiters)
		for i := 0; i < waiters; i++ {
			go func() {
				<-ch
				woke <- time.Now()
			}()
		}
		start := time.Now()
		trigger()
		for i := 0; i < waiters; i++ {
			select {
			case at := <-woke:
				if d := at.Sub(start); d > bound {
					t.Errorf("waiter %d woke after %v; want < %v", i, d, bound)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("only %d of %d waiters woke", i, waiters)
			}
		}
	}

	waitAll(ch, time.Second, func() { writeJobFile(t, dir, "0", jobJSON("10", "0")) })
	waitSlurm(t, cl, 1, map[string]JobInfo{"0": jobOf("10")})

	ch2 := cl.Changed()
	if ch2 == ch || isClosed(ch2) {
		t.Fatal("Changed was not replaced by a fresh open channel after the bump")
	}
	writeJobFile(t, dir, "0", jobJSON("12", "0"))
	select {
	case <-ch2:
	case <-time.After(2 * time.Second):
		t.Fatal("Changed did not close on a content change")
	}
	waitSlurm(t, cl, 2, map[string]JobInfo{"0": jobOf("12")})

	// bump-to-wake: processSlurm is what the watcher runs per event. Use a
	// client with no watcher goroutine so the direct call has no writer to
	// race (a single file write can deliver several inotify events).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bare := &client{GpuJobs: map[string]JobInfo{}, ctx: ctx, cancel: cancel, changed: make(chan struct{})}
	waitAll(bare.Changed(), 100*time.Millisecond, func() {
		bare.processSlurm(fsnotify.Write, "0", []byte(jobJSON("13", "0")))
	})
	if v := bare.Version(); v != 1 {
		t.Errorf("bare client Version = %d; want 1", v)
	}
}

// TestSlurmNoBumpOnNoop: an empty or truncated job file, an identical
// rewrite, a parse failure and the remove of a key never stored do not bump
// and leave the watcher alive; the full content then lands with one bump, and
// a changed field bumps.
func TestSlurmNoBumpOnNoop(t *testing.T) {
	cl, dir := newTestSlurm(t)
	writeJobFile(t, dir, "5", "")
	writeJobFile(t, dir, "6", `{"SLURM_JOB_ID":"6","CUDA_VIS`)
	expectSlurm(t, cl, 0, map[string]JobInfo{})
	writeJobFile(t, dir, "6", jobJSON("6", "1"))
	waitSlurm(t, cl, 1, map[string]JobInfo{"1": jobOf("6")})

	writeJobFile(t, dir, "3", jobJSON("30", "3"))
	want := map[string]JobInfo{"1": jobOf("6"), "3": jobOf("30")}
	waitSlurm(t, cl, 2, want)
	ch := cl.Changed()

	writeJobFile(t, dir, "3", jobJSON("30", "3"))
	expectSlurm(t, cl, 2, want)
	writeJobFile(t, dir, "4", "{garbage")
	writeJobFile(t, dir, "42", "not json")
	if err := os.Remove(filepath.Join(dir, "42")); err != nil { // key "42" never stored
		t.Fatal(err)
	}
	expectSlurm(t, cl, 2, want)
	if isClosed(ch) {
		t.Fatal("Changed closed on a no-op")
	}

	writeJobFile(t, dir, "3", jobJSONUser("30", "other", "3"))
	other := jobOf("30")
	other.User = "other"
	waitSlurm(t, cl, 3, map[string]JobInfo{"1": jobOf("6"), "3": other})
	if !isClosed(ch) {
		t.Error("Changed not closed on a changed field")
	}
}

// TestSlurmWatcherRename: a job file renamed into place (new, and
// over an existing file) is read once per content change; the temp name is
// skipped.
func TestSlurmWatcherRename(t *testing.T) {
	cl, dir := newTestSlurm(t)
	tmp := filepath.Join(dir, ".job.tmp")
	mv := func(body string) {
		t.Helper()
		if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, filepath.Join(dir, "4")); err != nil {
			t.Fatal(err)
		}
	}
	mv(jobJSON("40", "4"))
	waitSlurm(t, cl, 1, map[string]JobInfo{"4": jobOf("40")})
	mv(jobJSON("41", "4"))
	waitSlurm(t, cl, 2, map[string]JobInfo{"4": jobOf("41")})
	mv(jobJSON("41", "4"))
	expectSlurm(t, cl, 2, map[string]JobInfo{"4": jobOf("41")})
}

// TestSlurmWatcherFsnotifyError: a watcher error is logged without a
// bump and the loop keeps serving events; a closed Events or Errors channel
// ends the loop (no spin); the last state stays readable.
func TestSlurmWatcherFsnotifyError(t *testing.T) {
	logger.Init(true)
	dir := t.TempDir()
	for _, closeEvents := range []bool{true, false} {
		t.Run(fmt.Sprintf("closeEvents=%v", closeEvents), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cl := &client{GpuJobs: map[string]JobInfo{}, ctx: ctx, cancel: cancel, changed: make(chan struct{})}
			events, errs := make(chan fsnotify.Event), make(chan error)
			done := make(chan struct{})
			go func() {
				cl.watch(events, errs)
				close(done)
			}()
			errs <- errors.New("injected fsnotify error")
			if v := cl.Version(); v != 0 {
				t.Fatalf("Version = %d after an error; want 0", v)
			}
			writeJobFile(t, dir, "1", jobJSON("1", "1"))
			events <- fsnotify.Event{Name: filepath.Join(dir, "1"), Op: fsnotify.Write}
			waitSlurm(t, cl, 1, map[string]JobInfo{"1": jobOf("1")})
			ch := cl.Changed()
			if closeEvents {
				close(events)
			} else {
				close(errs)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("watch loop did not return after its channel closed")
			}
			if v := cl.Version(); v != 1 || isClosed(ch) {
				t.Errorf("after close: Version = %d, Changed closed = %v; want 1, false", v, isClosed(ch))
			}
			if wls, err := cl.ListWorkloads(); err != nil || len(wls) != 1 {
				t.Errorf("ListWorkloads after close = %v, %v", wls, err)
			}
		})
	}
}
