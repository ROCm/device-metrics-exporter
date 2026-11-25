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
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

// workloadSourceInterval is the k8s poll / safety tick of the workload source.
const workloadSourceInterval = hrWorkloadLabelRefreshInterval

// workloadSourceCoalesce is how long a slurm job event waits before the source
// reads, so a burst of job-file writes becomes one refresh.
const workloadSourceCoalesce = time.Millisecond

// workloadSnap is one published workload state. It is immutable once published:
// wls is never mutated. err != nil marks a "fetch failed" snapshot whose wls is
// exactly what GPUAgentClient.ListWorkloads returns for that failure.
type workloadSnap struct {
	wls map[string]scheduler.Workload
	err error
	gen uint64 // bumps only on a content change or an ok<->failed change
}

// workloadSource is the single owner of scheduler calls for a GPU client. It
// polls k8s on a ticker, refreshes slurm on job events, and publishes a
// snapshot plus a close-and-replace change broadcast. It lives for the process
// (GPUAgentClient.Close also runs on every gpuagent reconnect, so it must not
// stop the source).
type workloadSource struct {
	ga       *GPUAgentClient
	snap     atomic.Pointer[workloadSnap]
	lastOK   atomic.Pointer[workloadSnap] // last snapshot with err == nil
	mu       sync.Mutex                   // guards changed
	changed  chan struct{}                // closed and replaced on each gen bump
	interval time.Duration
	coalesce time.Duration
	cancel   context.CancelFunc
	// refresher-owned state (the seed refresh runs before run starts)
	k8sPart   map[string]scheduler.Workload
	k8sErr    error
	slurmPart map[string]scheduler.Workload
	slurmErr  error
	slurmCl   scheduler.SchedulerClient // client slurmVer belongs to
	slurmVer  uint64
	// test observability
	k8sCalls     atomic.Int64
	evRefreshes  atomic.Int64
	slurmListing atomic.Int64
}

func newWorkloadSource(ga *GPUAgentClient, interval time.Duration) *workloadSource {
	if interval <= 0 {
		interval = workloadSourceInterval
	}
	return &workloadSource{
		ga:       ga,
		changed:  make(chan struct{}),
		interval: interval,
		coalesce: workloadSourceCoalesce,
	}
}

// load returns the current snapshot; nil only before the seed refresh.
func (s *workloadSource) load() *workloadSnap {
	return s.snap.Load()
}

// Changed returns a channel closed on the next gen bump after the call. Obtain
// it before reading the snapshot it guards, so a bump in between is not lost.
func (s *workloadSource) Changed() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}

// notifier returns the current slurm client and its change channel (nil when
// the client is absent or does not implement scheduler.ChangeNotifier).
func (s *workloadSource) notifier() (scheduler.SchedulerClient, <-chan struct{}) {
	_, _, slurm := s.ga.schedulers()
	if n, ok := slurm.(scheduler.ChangeNotifier); ok {
		return slurm, n.Changed()
	}
	return slurm, nil
}

// run refreshes on slurm events and on the ticker until ctx is done. Each wake
// re-arms the slurm channel before it refreshes (obtain-then-read), so a change
// racing the refresh closes the new channel and is not lost.
func (s *workloadSource) run(ctx context.Context, evCl scheduler.SchedulerClient, ev <-chan struct{}) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ev:
			evCl, ev = s.notifier()
			if s.coalesce > 0 {
				t := time.NewTimer(s.coalesce)
				select {
				case <-ctx.Done():
					t.Stop()
					return
				case <-t.C:
				}
			}
			s.evRefreshes.Add(1)
			s.refresh(false)
		case <-ticker.C:
			if _, _, cl := s.ga.schedulers(); cl != evCl {
				evCl, ev = s.notifier()
			}
			s.refresh(true)
		}
	}
}

// refresh reads the schedulers (k8s only when poll) and publishes a new
// snapshot when the merged result or its ok/failed state changed.
func (s *workloadSource) refresh(poll bool) {
	k8sOn, k8s, slurm := s.ga.schedulers()
	k8sUse := k8sOn && k8s != nil
	dirty := false

	if !k8sUse {
		if s.k8sPart != nil || s.k8sErr != nil {
			s.k8sPart, s.k8sErr = nil, nil
			dirty = true
		}
	} else if poll {
		s.k8sCalls.Add(1)
		m, err := k8s.ListWorkloads()
		switch {
		case err != nil:
			// k8sPart keeps its value but is unused while k8sErr is set.
			if s.k8sErr == nil {
				dirty = true
			}
			s.k8sErr = err
		case s.k8sErr != nil || !maps.Equal(m, s.k8sPart):
			s.k8sPart, s.k8sErr = m, nil
			dirty = true
		}
	}

	if slurm == nil {
		if s.slurmPart != nil || s.slurmErr != nil || s.slurmCl != nil {
			s.slurmPart, s.slurmErr, s.slurmCl, s.slurmVer = nil, nil, nil, 0
			dirty = true
		}
	} else if s.slurmStale(slurm) {
		var v uint64
		if n, ok := slurm.(scheduler.ChangeNotifier); ok {
			// Read the version before the copy: a bump racing the copy is
			// caught on the next wake.
			v = n.Version()
		}
		s.slurmListing.Add(1)
		m, err := slurm.ListWorkloads()
		s.slurmCl, s.slurmVer = slurm, v
		switch {
		case err != nil:
			if s.slurmErr == nil {
				dirty = true
			}
			s.slurmErr = err
		case s.slurmErr != nil || !maps.Equal(m, s.slurmPart):
			s.slurmPart, s.slurmErr = m, nil
			dirty = true
		}
	}

	cur := s.snap.Load()
	if cur != nil && !dirty {
		return
	}
	var wls map[string]scheduler.Workload
	var err error
	switch {
	// A single successful part is published as is (it is never mutated and
	// equals the merge result), saving the merge copy on a change.
	case k8sUse && slurm == nil && s.k8sErr == nil && s.k8sPart != nil:
		wls = s.k8sPart
	case !k8sUse && slurm != nil && s.slurmErr == nil && s.slurmPart != nil:
		wls = s.slurmPart
	default:
		wls, err = mergeWorkloads(k8sUse, s.k8sPart, s.k8sErr, slurm != nil, s.slurmPart, s.slurmErr)
	}
	if cur != nil && (err == nil) == (cur.err == nil) && maps.Equal(wls, cur.wls) {
		return
	}
	next := &workloadSnap{wls: wls, err: err, gen: 1}
	if cur != nil {
		next.gen = cur.gen + 1
	}
	s.snap.Store(next)
	if err == nil {
		s.lastOK.Store(next)
	}
	s.mu.Lock()
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
}

// slurmStale reports whether the slurm part must be re-read: always for a client
// that does not track changes, otherwise on a new client, a new version or
// after a failed read.
func (s *workloadSource) slurmStale(slurm scheduler.SchedulerClient) bool {
	n, ok := slurm.(scheduler.ChangeNotifier)
	if !ok || slurm != s.slurmCl || s.slurmErr != nil {
		return true
	}
	return n.Version() != s.slurmVer
}

// lastGood returns the last successfully fetched workload map without
// locking. ok is false until a fetch has succeeded, so the caller falls back to
// a synchronous fetch; a later failed fetch keeps the last good map.
func (s *workloadSource) lastGood() (map[string]scheduler.Workload, bool) {
	ws := s.lastOK.Load()
	if ws == nil {
		return nil, false
	}
	return ws.wls, true
}

// stop cancels the source goroutine (tests only; the source lives for the
// process). Idempotent.
func (s *workloadSource) stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// mergeWorkloads is GPUAgentClient.ListWorkloads' merge with the fetches lifted
// out, and returns exactly what it returns: on a k8s error an empty map and the
// error (slurm is not merged); on a slurm error the k8s-only map and the error;
// otherwise k8s first and slurm overwriting the same key. Never returns nil.
func mergeWorkloads(k8sOn bool, k8s map[string]scheduler.Workload, k8sErr error,
	slurmOn bool, slurm map[string]scheduler.Workload, slurmErr error,
) (map[string]scheduler.Workload, error) {
	wls := make(map[string]scheduler.Workload)
	if k8sOn {
		if k8sErr != nil {
			return wls, k8sErr
		}
		for k, wl := range k8s {
			wls[k] = wl
		}
	}
	if !slurmOn {
		return wls, nil
	}
	if slurmErr != nil {
		return wls, slurmErr
	}
	for k, wl := range slurm {
		wls[k] = wl
	}
	return wls, nil
}

// ensureWorkloadSource returns the client's workload source, creating, seeding
// and starting it on first use. The seed refresh is synchronous, so the first
// read is workload-aware; it always publishes gen 1 (ok or failed). Its context
// derives from context.Background(): a gpuagent reconnect cancels the handler
// context and nothing would restart the source.
func (ga *GPUAgentGPUClient) ensureWorkloadSource() *workloadSource {
	if s := ga.wlSource.Load(); s != nil {
		return s
	}
	s := newWorkloadSource(ga.gpuHandler, ga.wlInterval)
	evCl, ev := s.notifier()
	s.refresh(true)
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	if !ga.wlSource.CompareAndSwap(nil, s) {
		cancel()
		return ga.wlSource.Load()
	}
	go s.run(ctx, evCl, ev)
	return s
}

// stopWorkloadSource stops the client's source goroutine. Tests only.
func (ga *GPUAgentGPUClient) stopWorkloadSource() {
	if s := ga.wlSource.Load(); s != nil {
		s.stop()
	}
}

// currentWorkloads returns the shared snapshot's (wls, err) once a source runs,
// otherwise today's synchronous ListWorkloads (including its error).
func (ga *GPUAgentGPUClient) currentWorkloads() (map[string]scheduler.Workload, error) {
	if s := ga.wlSource.Load(); s != nil {
		if ws := s.load(); ws != nil {
			return ws.wls, ws.err
		}
	}
	return ga.gpuHandler.ListWorkloads()
}

// startPullWorkloadCache publishes the shared source as the non-high-res pull
// path's workload cache once, gated to flag-on non-high-res store-serving mode
// (useSnapshotStoreServing already encodes enabled && !high-res), so the
// scheduler fetch stays off the /metrics scrape hot path. No-op in
// flag-off/legacy and high-res, where getMetricsAll keeps the synchronous
// per-scrape ListWorkloads, so flag-off /metrics stays byte-identical.
// Idempotent. The source outlives a gpuagent reconnect.
func (ga *GPUAgentGPUClient) startPullWorkloadCache(_ context.Context) {
	if !ga.useSnapshotStoreServing() {
		return
	}
	if ga.pullWLSource.Load() != nil {
		return
	}
	ga.pullWLSource.Store(ga.ensureWorkloadSource())
}
