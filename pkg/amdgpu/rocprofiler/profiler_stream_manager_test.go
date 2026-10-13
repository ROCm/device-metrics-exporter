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

package rocprofiler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
)

// mockScriptBody is a scripted stand-in for rocpctl --stream, driven entirely by env
// vars so one script covers all test scenarios (avoids compiling the C++ mock).
const mockScriptBody = `#!/bin/bash
set -u
echo "$$" > "$PIDFILE"
echo "$*" >> "$ARGSLOG"
if [ "${ABORT:-0}" = "1" ]; then
  kill -ABRT $$
fi
if [ "${NO_EMIT:-0}" = "1" ]; then
  exit "${EXIT_CODE:-0}"
fi
if [ "${SLOW_DEATH:-0}" = "1" ]; then
  trap 'sleep 0.3; exit 0' TERM
fi
while true; do
  echo '{"GpuMetrics":[{"GpuId":"0","Metrics":[{"Field":"gpu_prof_test","Value":"42"}]}]}'
  sleep "${EMIT_SLEEP:-0.02}"
  if [ "${ONE_SHOT:-0}" = "1" ]; then
    break
  fi
done
exit "${EXIT_CODE:-0}"
`

func writeMockScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rocpctl-mock")
	if err := os.WriteFile(path, []byte(mockScriptBody), 0o755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}
	return path
}

type fakeSink struct {
	mu      sync.Mutex
	samples []*amdgpu.GpuProfiler
}

func (f *fakeSink) Publish(p *amdgpu.GpuProfiler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.samples = append(f.samples, p)
}

func (f *fakeSink) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.samples)
}

func (f *fakeSink) first() *amdgpu.GpuProfiler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.samples) == 0 {
		return nil
	}
	return f.samples[0]
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

// fileLines reads path, returning nil if it doesn't exist yet or is empty.
func fileLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func newTestManager(t *testing.T, fields []string, backoff time.Duration) (*ProfilerStreamManager, *fakeSink, *ROCProfilerClient, string, string) {
	t.Helper()
	bin := writeMockScript(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	argsLog := filepath.Join(t.TempDir(), "args")
	t.Setenv("PIDFILE", pidFile)
	t.Setenv("ARGSLOG", argsLog)

	sink := &fakeSink{}
	client := NewRocProfilerClient("test")
	mgr := NewProfilerStreamManager(bin, fields, 100, 1000, sink, client, backoff)
	return mgr, sink, client, pidFile, argsLog
}

func TestProfilerStreamManager_DecodeAndPublish(t *testing.T) {
	t.Setenv("EMIT_SLEEP", "0.01")
	mgr, sink, _, _, _ := newTestManager(t, []string{"gpu_prof_test"}, 20*time.Millisecond)

	if err := mgr.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop(context.Background())

	waitUntil(t, 2*time.Second, func() bool { return sink.count() > 0 })

	sample := sink.first()
	assert.Assert(t, sample != nil)
	assert.Equal(t, len(sample.GpuMetrics), 1)
	assert.Equal(t, sample.GpuMetrics[0].GpuId, "0")
}

func TestProfilerStreamManager_RestartsOnNonFatalExit(t *testing.T) {
	// NO_EMIT: a crash-loop that never decodes a sample must still accumulate to
	// disable -- ResetFailureCount only fires on a successfully decoded sample.
	t.Setenv("NO_EMIT", "1")
	t.Setenv("EXIT_CODE", "1")
	mgr, _, client, _, argsLog := newTestManager(t, []string{"gpu_prof_test"}, 5*time.Millisecond)

	if err := mgr.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop(context.Background())

	// failThreshold (3) consecutive non-fatal exits disables the profiler; reaching that
	// state proves the supervisor restarted the child multiple times within backoff.
	waitUntil(t, 3*time.Second, func() bool { return client.IsDisabledOnFailure() })
	assert.Assert(t, strings.Contains(client.GetDisabledReason(), "consecutive"))
	assert.Assert(t, len(fileLines(argsLog)) >= failThreshold)
}

func TestProfilerStreamManager_HealthyRestartsResetFailureCount(t *testing.T) {
	// Each run decodes one sample then exits non-zero: the successful decode must reset
	// the failure count every cycle, so repeated restarts never reach the 3-strike disable.
	t.Setenv("ONE_SHOT", "1")
	t.Setenv("EXIT_CODE", "1")
	t.Setenv("EMIT_SLEEP", "0.001")
	mgr, sink, client, _, _ := newTestManager(t, []string{"gpu_prof_test"}, 5*time.Millisecond)

	if err := mgr.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop(context.Background())

	// More restart cycles than failThreshold, each of which emitted a sample first.
	waitUntil(t, 3*time.Second, func() bool { return sink.count() > failThreshold+2 })
	assert.Assert(t, !client.IsDisabledOnFailure())
}

func TestProfilerStreamManager_FatalAbortDisablesNoRestart(t *testing.T) {
	t.Setenv("ABORT", "1")
	mgr, _, client, _, argsLog := newTestManager(t, []string{"gpu_prof_test"}, 20*time.Millisecond)

	if err := mgr.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop(context.Background())

	waitUntil(t, 2*time.Second, func() bool { return client.IsDisabledOnFailure() })
	assert.Equal(t, client.GetDisabledReason(), disabledReasonCrash)

	// Give the supervisor a chance to (incorrectly) restart, then confirm it didn't.
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, len(fileLines(argsLog)), 1)
}

func TestProfilerStreamManager_StopDrainsNoLeakedProcess(t *testing.T) {
	t.Setenv("EMIT_SLEEP", "0.02")
	mgr, sink, _, pidFile, _ := newTestManager(t, []string{"gpu_prof_test"}, 20*time.Millisecond)

	if err := mgr.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool { return sink.count() > 0 })

	var pid int
	waitUntil(t, 2*time.Second, func() bool {
		lines := fileLines(pidFile)
		if len(lines) == 0 {
			return false
		}
		p, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			return false
		}
		pid = p
		return true
	})

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	mgr.Stop(stopCtx)

	waitUntil(t, 2*time.Second, func() bool {
		err := syscall.Kill(pid, 0)
		return errors.Is(err, syscall.ESRCH)
	})
}

func TestProfilerStreamManager_ReconfigureRespawnsWithNewArgs(t *testing.T) {
	t.Setenv("EMIT_SLEEP", "0.02")
	mgr, sink, _, _, argsLog := newTestManager(t, []string{"field_a"}, 50*time.Millisecond)

	if err := mgr.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop(context.Background())

	waitUntil(t, 2*time.Second, func() bool { return sink.count() > 0 })

	mgr.Reconfigure([]string{"field_b"}, 200, 2000)

	waitUntil(t, 2*time.Second, func() bool { return len(fileLines(argsLog)) >= 2 })

	lines := fileLines(argsLog)
	assert.Assert(t, strings.Contains(lines[0], "field_a"))
	last := lines[len(lines)-1]
	assert.Assert(t, strings.Contains(last, "field_b"))
	assert.Assert(t, strings.Contains(last, "200"))
	assert.Assert(t, strings.Contains(last, "2000"))
}

// TestProfilerStreamManager_ReconfigureTwiceDuringTeardownDoesNotPanic is a regression
// test for a double-close-of-killCh panic: Reconfigure used to read+close m.killCh
// without nil-ing it, so a second Reconfigure landing before the first child's teardown
// (SIGTERM -> exit, up to stopTimeout) finished its own cleanup would close the same
// already-closed channel again. SLOW_DEATH makes the first child ignore SIGTERM briefly
// (trap + short sleep) to widen that teardown window deterministically.
func TestProfilerStreamManager_ReconfigureTwiceDuringTeardownDoesNotPanic(t *testing.T) {
	t.Setenv("EMIT_SLEEP", "0.02")
	t.Setenv("SLOW_DEATH", "1")
	mgr, sink, _, _, argsLog := newTestManager(t, []string{"field_a"}, 50*time.Millisecond)

	if err := mgr.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop(context.Background())

	waitUntil(t, 2*time.Second, func() bool { return sink.count() > 0 })

	// First call starts the (slow-dying) child's teardown; its runOnce stays blocked in
	// waitForExit for ~300ms before reaching its own m.killCh = nil cleanup. Fire a
	// second Reconfigure while still inside that window -- pre-fix this double-closed
	// the same channel and panicked the whole process.
	mgr.Reconfigure([]string{"field_b"}, 150, 1500)
	time.Sleep(50 * time.Millisecond)
	mgr.Reconfigure([]string{"field_c"}, 250, 2500)

	// Respawn must reflect the latest config once the slow-dying child actually exits.
	waitUntil(t, 3*time.Second, func() bool { return len(fileLines(argsLog)) >= 2 })

	lines := fileLines(argsLog)
	assert.Assert(t, strings.Contains(lines[0], "field_a"))
	last := lines[len(lines)-1]
	assert.Assert(t, strings.Contains(last, "field_c"))
	assert.Assert(t, strings.Contains(last, "250"))
	assert.Assert(t, strings.Contains(last, "2500"))
}
