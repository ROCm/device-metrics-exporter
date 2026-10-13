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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

const (
	// stopTimeout bounds how long Stop/Reconfigure waits after SIGTERM before escalating
	// to SIGKILL on the child process group.
	stopTimeout = 5 * time.Second
	// backoffCapMultiplier bounds supervisor restart backoff growth relative to the
	// injected base (production base is 10s per design; tests inject a short base).
	backoffCapMultiplier = 16
)

// errRestartRequested signals a runOnce exit caused by Reconfigure, distinct from a
// child crash: the supervisor must restart immediately without backoff or failure accounting.
var errRestartRequested = errors.New("rocpctl stream restart requested")

// StreamSink receives decoded profiler samples from a running stream. The
// store-backed implementation feeds the snapshot store; tests use a fake
// recording sink.
type StreamSink interface {
	Publish(sample *amdgpu.GpuProfiler)
}

// ProfilerStreamManager supervises a long-lived `rocpctl --stream` child, decoding its
// NDJSON stdout into amdgpu.GpuProfiler samples published to a StreamSink. It is pure
// stream+supervise+decode+publish: no config/store/gpuagent dependencies.
type ProfilerStreamManager struct {
	binPath     string
	sink        StreamSink
	client      *ROCProfilerClient
	backoffBase time.Duration

	mu         sync.Mutex
	fields     []string
	intervalMs uint64
	durationUs uint64
	cmd        *exec.Cmd
	killCh     chan struct{} // closed by Reconfigure to kill+respawn the current child
	cancel     context.CancelFunc

	wg sync.WaitGroup // supervisor loop goroutine
}

// NewProfilerStreamManager builds a manager for a `rocpctl --stream` child.
// client supplies failure-policy accounting only; fields/interval/duration are owned
// here so tests can drive them without going through the one-shot client.
func NewProfilerStreamManager(binPath string, fields []string, intervalMs, durationUs uint64, sink StreamSink, client *ROCProfilerClient, backoffBase time.Duration) *ProfilerStreamManager {
	return &ProfilerStreamManager{
		binPath:     binPath,
		fields:      fields,
		intervalMs:  intervalMs,
		durationUs:  durationUs,
		sink:        sink,
		client:      client,
		backoffBase: backoffBase,
	}
}

// Start spawns the supervisor goroutine, which keeps the rocpctl stream child running
// until Stop is called or the failure policy disables it.
func (m *ProfilerStreamManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return fmt.Errorf("profiler stream manager already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	m.wg.Add(1)
	go m.superviseLoop(runCtx)
	return nil
}

// Reconfigure updates the stream args and kills the running child (if any) so the
// supervisor respawns it immediately with the new fields/interval/duration.
func (m *ProfilerStreamManager) Reconfigure(fields []string, intervalMs, durationUs uint64) {
	m.mu.Lock()
	m.fields = fields
	m.intervalMs = intervalMs
	m.durationUs = durationUs
	killCh := m.killCh
	// Nil under the lock so a second Reconfigure landing before runOnce's teardown
	// finishes (up to stopTimeout) sees nil instead of the same already-closed channel.
	m.killCh = nil
	m.mu.Unlock()
	if killCh != nil {
		close(killCh)
	}
}

// Stop cancels the lifecycle context (terminating any running child via SIGTERM then
// SIGKILL) and waits for the supervisor goroutine to exit, bounded by ctx.
func (m *ProfilerStreamManager) Stop(ctx context.Context) {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()
	if cancel == nil {
		return // never started, or already stopped
	}
	cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		logger.Log.Printf("rocpctl stream: Stop context expired before shutdown completed")
	}
}

// superviseLoop owns the restart-with-backoff policy: a clean/crash exit increments the
// shared failure count (or sets fatal state on dumped/aborted) and retries after backoff;
// a Reconfigure-induced exit restarts immediately with a reset backoff.
func (m *ProfilerStreamManager) superviseLoop(ctx context.Context) {
	defer m.wg.Done()
	backoff := m.backoffBase
	maxBackoff := m.backoffBase * backoffCapMultiplier

	for {
		if ctx.Err() != nil {
			return
		}
		if m.client.IsDisabledOnFailure() {
			logger.Log.Printf("rocpctl stream: %v disabled, not starting", m.client.Name)
			return
		}

		err := m.runOnce(ctx)

		if errors.Is(err, errRestartRequested) {
			backoff = m.backoffBase
			continue
		}
		if ctx.Err() != nil {
			return // Stop requested shutdown
		}
		if err != nil && isFatalExit(err) {
			m.client.SetFatalFailureState(ctx)
			return
		}

		m.client.IncFailureCount(ctx)
		if m.client.IsDisabledOnFailure() {
			return
		}
		logger.Log.Printf("rocpctl stream exited (%v), restarting in %v", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// isFatalExit matches the same crash signature rocpclient.go's one-shot getMetrics uses:
// Go renders a SIGABRT exit as "signal: aborted (core dumped)" (or without the parenthetical
// if core dumps are disabled), hence checking both substrings.
func isFatalExit(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "dumped") || strings.Contains(msg, "aborted")
}

// runOnce spawns one child, decodes its stdout until it exits (naturally, via Reconfigure,
// or via ctx cancellation), and returns the exit reason. errRestartRequested means the exit
// was intentional (Reconfigure) and must not count against the failure policy.
func (m *ProfilerStreamManager) runOnce(ctx context.Context) error {
	cmd := exec.Command(m.binPath, m.buildArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	killCh := make(chan struct{})
	m.mu.Lock()
	m.cmd = cmd
	m.killCh = killCh
	m.mu.Unlock()

	var drainWg sync.WaitGroup
	drainWg.Add(2)
	go func() { defer drainWg.Done(); m.decodeStdout(stdout) }()
	go func() { defer drainWg.Done(); logStderr(stderr) }()

	// Wait() must only be called once stdout/stderr readers have seen EOF (which happens
	// as soon as the child exits, independent of Wait reaping it) -- calling it earlier
	// races the pipe close against in-flight reads.
	waitErr := make(chan error, 1)
	go func() {
		drainWg.Wait()
		waitErr <- cmd.Wait()
	}()

	var result error
	select {
	case result = <-waitErr:
	case <-killCh:
		m.signalGroup(cmd, syscall.SIGTERM)
		waitForExit(waitErr, cmd)
		result = errRestartRequested
	case <-ctx.Done():
		m.signalGroup(cmd, syscall.SIGTERM)
		waitForExit(waitErr, cmd)
		result = ctx.Err()
	}

	m.mu.Lock()
	m.cmd = nil
	m.killCh = nil
	m.mu.Unlock()
	return result
}

// waitForExit waits up to stopTimeout for the already-signaled child to be reaped,
// escalating to SIGKILL on the process group if it hasn't exited by then.
func waitForExit(waitErr <-chan error, cmd *exec.Cmd) {
	select {
	case <-waitErr:
		return
	case <-time.After(stopTimeout):
	}
	// Force-kill is intentionally not bound to the caller's ctx: it must run to
	// completion or the child (and its process group) leaks.
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	<-waitErr
}

func (m *ProfilerStreamManager) signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil {
		logger.Log.Printf("rocpctl stream: signal %v to pgid %d failed: %v", sig, cmd.Process.Pid, err)
	}
}

func (m *ProfilerStreamManager) buildArgs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := []string{
		"--stream",
		"--interval", strconv.FormatUint(m.intervalMs, 10),
		"-d", strconv.FormatUint(m.durationUs, 10),
		"-p", "0", // high-res stream path is PTL-gated off; caller guarantees PtlDelay == 0
	}
	return append(args, m.fields...)
}

func (m *ProfilerStreamManager) decodeStdout(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var sample amdgpu.GpuProfiler
		if err := json.Unmarshal(line, &sample); err != nil {
			logger.Log.Printf("rocpctl stream: failed to decode line: %v", err)
			continue
		}
		m.sink.Publish(&sample)
		// Mirrors getMetrics' reset-on-success: a run that ever produces data is healthy,
		// so only a crash-loop that never decodes a sample should accumulate to disable.
		// Preserve a fatal (crash) disable — that stays until the pod restarts.
		m.client.ResetConsecutiveFailures()
	}
}

func logStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		logger.Log.Printf("rocpctl stream stderr: %s", scanner.Text())
	}
}
