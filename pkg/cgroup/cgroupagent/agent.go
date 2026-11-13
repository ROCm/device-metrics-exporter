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

// Package cgroupagent exports per-job cgroup v2 metrics for Spur and Slurm
// through the shared snapshot store.
package cgroupagent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/cgroup"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	storeKeyPrefix = "cgroup:"
	fetchKey       = "cgroup-fetch"
	// maxWorkloads caps series cardinality if cgroups pile up.
	maxWorkloads = 1024
)

func labelName(l exportermetrics.MetricLabel) string { return strings.ToLower(l.String()) }

var (
	hostnameLabel  = labelName(exportermetrics.MetricLabel_HOSTNAME)
	jobIDLabel     = labelName(exportermetrics.MetricLabel_JOB_ID)
	jobUserLabel   = labelName(exportermetrics.MetricLabel_JOB_USER)
	partitionLabel = labelName(exportermetrics.MetricLabel_JOB_PARTITION)
	accountLabel   = labelName(exportermetrics.MetricLabel_JOB_ACCOUNT)
	labelNames     = []string{hostnameLabel, jobIDLabel, jobUserLabel, partitionLabel, accountLabel}
)

var _ metricsutil.MetricsInterface = (*Agent)(nil)

// JobLookup returns the scheduler metadata of a job, false when unknown.
type JobLookup func(jobID string) (scheduler.JobInfo, bool)

// Ticker returns a tick channel and its stop function.
type Ticker func(time.Duration) (<-chan time.Time, func())

type Option func(*Agent)

func WithJobLookup(f JobLookup) Option { return func(a *Agent) { a.lookupJob = f } }

// WithHostPaths overrides where the cgroup v2 hierarchy and procfs are read.
func WithHostPaths(cgroupRoot, procRoot string) Option {
	return func(a *Agent) { a.cgroupRoot, a.procRoot = cgroupRoot, procRoot }
}

func WithTicker(t Ticker) Option { return func(a *Agent) { a.newTicker = t } }

func realTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

type Agent struct {
	mh         *metricsutil.MetricsHandler
	lookupJob  JobLookup
	newTicker  Ticker
	deduper    *snapshotstore.Deduper
	hostname   string
	cgroupRoot string
	procRoot   string

	// Read by publish and scrapes without a lock, so a long publish never
	// blocks them.
	enabled atomic.Bool
	highRes atomic.Bool

	// mu guards the feed lifecycle.
	mu       sync.Mutex
	stopFeed chan struct{}
	feedDone chan struct{}

	warnings warnOnce
}

// warnOnce logs a warning when it first appears and stays quiet while it keeps
// recurring, so the 1 Hz high-res feed does not repeat it every sample.
type warnOnce struct {
	mu     sync.Mutex
	active map[string]bool
}

func (w *warnOnce) flush(msgs []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	next := make(map[string]bool, len(msgs))
	for _, m := range msgs {
		if !w.active[m] {
			logger.Log.Print(m)
		}
		next[m] = true
	}
	w.active = next
}

func NewAgent(mh *metricsutil.MetricsHandler, opts ...Option) *Agent {
	a := &Agent{
		mh:         mh,
		newTicker:  realTicker,
		deduper:    snapshotstore.NewDeduper(),
		cgroupRoot: "/sys/fs/cgroup",
		procRoot:   "/proc",
	}
	for _, o := range opts {
		o(a)
	}
	if h, err := utils.GetHostName(); err == nil {
		a.hostname = h
	}
	return a
}

func (a *Agent) Init() error {
	a.mh.RegisterMetricsClient(a)
	return nil
}

func (a *Agent) InitConfigs() error {
	a.highRes.Store(a.mh.IsHighResolution())
	if !cgroup.IsV2(a.cgroupRoot) {
		a.enabled.Store(false)
		logger.Log.Printf("cgroup metrics disabled: %s is not a cgroup v2 mount; cgroup v2 is required", a.cgroupRoot)
		return nil
	}
	a.enabled.Store(true)
	return a.mh.GetRegistry().Register(snapshotstore.NewStoreCollector(a.mh.GetStore(), fieldDescs(a.mh.GetPrefix())))
}

func fieldDescs(prefix string) []snapshotstore.FieldDesc {
	descs := make([]snapshotstore.FieldDesc, 0, len(cgroup.AllFields))
	for _, f := range cgroup.AllFields {
		descs = append(descs, snapshotstore.FieldDesc{
			Field:      string(f.Field),
			Name:       prefix + string(f.Field),
			Help:       f.Help,
			ValueType:  f.ValueType,
			LabelNames: labelNames,
		})
	}
	return descs
}

// UpdateStaticMetrics seeds the store so the first scrape is complete, then
// starts the high-resolution feed when that mode is on.
func (a *Agent) UpdateStaticMetrics(_ context.Context) error {
	err := a.publish()
	if a.enabled.Load() && a.highRes.Load() {
		a.startFeed()
	}
	return err
}

// UpdateMetricsStats refreshes on scrape in pull mode. In high-resolution mode
// the feed owns the store and a scrape serves it as is.
func (a *Agent) UpdateMetricsStats(_ context.Context) error {
	if a.highRes.Load() {
		return nil
	}
	return a.deduper.FetchOnce(fetchKey, a.publish)
}

func (a *Agent) startFeed() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopFeed != nil {
		return
	}
	fast, _ := a.mh.GetHighResPeriods()
	stop, done := make(chan struct{}), make(chan struct{})
	a.stopFeed, a.feedDone = stop, done
	ticks, stopTicker := a.newTicker(time.Duration(fast) * time.Second)
	go func() {
		defer close(done)
		defer stopTicker()
		for {
			select {
			case <-stop:
				return
			case <-ticks:
				_ = a.publish()
			}
		}
	}()
}

func (a *Agent) StopServices() {
	a.mu.Lock()
	stop, done := a.stopFeed, a.feedDone
	a.stopFeed, a.feedDone = nil, nil
	a.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-done
}

func (a *Agent) Close() { a.StopServices() }

// publish reads every job cgroup and makes the agent's store slots exactly
// that set in one atomic swap, so a scrape never sees a half-updated set and a
// concurrent reload cannot leave stale slots behind.
func (a *Agent) publish() error {
	if !a.enabled.Load() {
		return nil
	}
	var warns []string
	defer func() { a.warnings.flush(warns) }()
	store := a.mh.GetStore()
	ws, shadowed, err := cgroup.Discover(a.cgroupRoot)
	if err != nil {
		warns = append(warns, fmt.Sprintf("list job cgroups: %v", err))
		store.ReplaceOwned(storeKeyPrefix, nil)
		return err
	}
	for _, w := range shadowed {
		warns = append(warns, fmt.Sprintf("job cgroup %s not exported: job %s is already exported from another layout", w.Path, w.JobID))
	}
	if len(ws) > maxWorkloads {
		warns = append(warns, fmt.Sprintf("job cgroups exceed the limit of %d; exporting the newest", maxWorkloads))
		ws = ws[:maxWorkloads]
	}
	tsUs := time.Now().UnixMicro()
	slots := make(map[string]map[string]snapshotstore.Sample, len(ws))
	for _, w := range ws {
		dir := filepath.Join(a.cgroupRoot, w.Path)
		values, err := cgroup.Read(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			warns = append(warns, fmt.Sprintf("read cgroup %s: %v", w.Path, err))
			continue
		}
		if vm, resolved, err := cgroup.VirtualMemoryBytes(dir, a.procRoot); err != nil {
			warns = append(warns, fmt.Sprintf("virtual memory of %s: %v", w.Path, err))
		} else if resolved > 0 {
			values[cgroup.MemoryVirtualBytes] = vm
		}
		labels := a.labelsFor(w.JobID)
		samples := make(map[string]snapshotstore.Sample, len(values))
		for f, v := range values {
			samples[string(f)] = snapshotstore.Sample{Value: v, Labels: labels, TsUs: tsUs}
		}
		slots[storeKeyPrefix+w.Path] = samples
	}
	store.ReplaceOwned(storeKeyPrefix, slots)
	return nil
}

func (a *Agent) labelsFor(jobID string) prometheus.Labels {
	labels := prometheus.Labels{
		hostnameLabel:  a.hostname,
		jobIDLabel:     jobID,
		jobUserLabel:   "",
		partitionLabel: "",
		accountLabel:   "",
	}
	if a.lookupJob == nil {
		return labels
	}
	if job, ok := a.lookupJob(jobID); ok {
		labels[jobUserLabel] = job.User
		labels[partitionLabel] = job.Partition
		labels[accountLabel] = job.Account
	}
	return labels
}

func (a *Agent) NeedsPerScrapePull() bool { return false }

func (a *Agent) ResetMetrics() error { return nil }

func (a *Agent) QueryMetrics() (interface{}, error) { return nil, nil }

func (a *Agent) QueryInbandRASErrors(string) (interface{}, error) { return nil, nil }

func (a *Agent) GetDeviceType() globals.DeviceType { return globals.CgroupDevice }
