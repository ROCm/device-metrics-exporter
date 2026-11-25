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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/fsnotify/fsnotify"
)

var SlurmLabels = map[string]bool{
	exportermetrics.MetricLabel_JOB_ID.String():        true,
	exportermetrics.MetricLabel_JOB_USER.String():      true,
	exportermetrics.MetricLabel_JOB_PARTITION.String(): true,
	exportermetrics.MetricLabel_JOB_ACCOUNT.String():   true,
}

// SlurmJobDir holds one file per running job, named by job id and written by
// the prolog. Unlike the per-GPU files it also covers CPU-only jobs.
var SlurmJobDir = filepath.Join(globals.SlurmDir, "jobs")

var jobIDRe = regexp.MustCompile(`^[A-Za-z0-9_.+-]+$`)

func jobInfoFromEnv(env map[string]string) JobInfo {
	return JobInfo{
		Id:        env["SLURM_JOB_ID"],
		User:      env["SLURM_JOB_USER"],
		Partition: env["SLURM_JOB_PARTITION"],
		Cluster:   env["SLURM_CLUSTER_NAME"],
		Account:   env["SLURM_JOB_ACCOUNT"],
	}
}

// ReadJobFile returns the metadata of jobID from its file in dir, or false
// when no valid file exists. It reads on every call: a handful of small reads
// per scrape is cheaper than keeping a cache coherent with requeues that reuse
// a job id.
func ReadJobFile(dir, jobID string) (JobInfo, bool) {
	if jobID == "." || jobID == ".." || !jobIDRe.MatchString(jobID) {
		return JobInfo{}, false
	}
	buf, err := os.ReadFile(filepath.Join(dir, jobID))
	if err != nil {
		return JobInfo{}, false
	}
	var env map[string]string
	if err := json.Unmarshal(buf, &env); err != nil {
		return JobInfo{}, false
	}
	info := jobInfoFromEnv(env)
	if info.Id != jobID {
		return JobInfo{}, false
	}
	return info, true
}

type client struct {
	sync.Mutex
	GpuJobs map[string]JobInfo
	ctx     context.Context
	cancel  context.CancelFunc
	// version and changed are guarded by the embedded mutex. version counts
	// GpuJobs content changes; changed is closed and replaced on each one.
	version uint64
	changed chan struct{}
}

var _ ChangeNotifier = (*client)(nil)

// NewSlurmClient creates a slurm scheduler client; watcher setup failures return an error.
func NewSlurmClient(ctx context.Context) (SchedulerClient, error) {
	cl, err := newSlurmClient(ctx, globals.SlurmDir)
	if err != nil {
		return nil, err
	}
	return cl, nil
}

// newSlurmClient watches dir for slurm job files (tests pass a temp dir).
func newSlurmClient(ctx context.Context, dir string) (*client, error) {
	ctx, cancel := context.WithCancel(ctx)

	cl := &client{
		GpuJobs: make(map[string]JobInfo),
		ctx:     ctx,
		cancel:  cancel,
		changed: make(chan struct{}),
	}

	if err := os.MkdirAll(path.Dir(dir), 0644); err != nil {
		logger.Log.Printf("error creating slurm dir %v err: %v", dir, err)
	}

	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("slurm fsnotify watcher init failed: %v. Likely cause: host-wide inotify exhaustion (fs.inotify.max_user_instances).", err)
	}

	// Add a path.
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		cancel()
		return nil, fmt.Errorf("slurm fsnotify watcher failed to watch %q: %v. Check directory permissions.", dir, err)
	}

	// Start listening for events.
	go func() {
		defer watcher.Close()
		cl.watch(watcher.Events, watcher.Errors)
	}()

	// read existing
	if fds, err := os.ReadDir(dir); err == nil {
		for _, f := range fds {
			watcher.Events <- fsnotify.Event{Name: dir + "/" + f.Name(), Op: fsnotify.Write}
		}
	}

	logger.Log.Printf("created slurm scheduler client")
	return cl, nil
}

// watch handles job file events until ctx is done or a channel is closed.
func (cl *client) watch(events <-chan fsnotify.Event, errs <-chan error) {
	for cl.ctx.Err() == nil {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}

			if _, err := strconv.Atoi(path.Base(event.Name)); err != nil {
				logger.Log.Printf("skip event: %+v", event)
				continue
			}
			logger.Log.Printf("event: %+v", event)

			if event.Has(fsnotify.Create | fsnotify.Write) {
				logger.Log.Printf("modified file: %v", event.Name)
				data, err := os.ReadFile(event.Name)
				if err != nil {
					logger.Log.Printf("failed to read %v, %v", event.Name, err)
					continue
				}
				cl.processSlurm(fsnotify.Write, path.Base(event.Name), data)

			} else if event.Has(fsnotify.Remove) {
				logger.Log.Printf("deleted file: %v", event.Name)
				cl.processSlurm(fsnotify.Remove, path.Base(event.Name), nil)
			}
		case err, ok := <-errs:
			if !ok {
				return
			}
			logger.Log.Printf("error: %v", err)
		case <-cl.ctx.Done():
			logger.Log.Printf("slurm job watcher stopped")
			return
		}
	}
}

func (cl *client) processSlurm(op fsnotify.Op, name string, buff []byte) {
	if op.Has(fsnotify.Write) {
		var jobEnv map[string]string
		if err := json.Unmarshal(buff, &jobEnv); err != nil {
			logger.Log.Printf("could not parse job env %v", err)
			logger.Log.Printf("job env %v ", string(buff))
			return
		}

		logger.Log.Printf("received job env %+v", jobEnv)
		if gpus, ok := jobEnv["CUDA_VISIBLE_DEVICES"]; ok {
			info := jobInfoFromEnv(jobEnv)
			dirty := false
			cl.Lock()
			for _, allocGPU := range strings.Split(gpus, ",") {
				if old, ok := cl.GpuJobs[allocGPU]; !ok || old != info {
					cl.GpuJobs[allocGPU] = info
					dirty = true
				}
			}
			if dirty {
				cl.bumpLocked()
			}
			cl.Unlock()
			logger.Log.Printf("updated %v", cl.GpuJobs)
		}
	} else {
		key := fmt.Sprintf("%v", name)
		cl.Lock()
		if _, ok := cl.GpuJobs[key]; ok {
			delete(cl.GpuJobs, key)
			cl.bumpLocked()
		}
		cl.Unlock()
		logger.Log.Printf("updated gpu %v jobs %v", name, cl.GpuJobs)
	}
}

// bumpLocked records a GpuJobs content change: it bumps the version and wakes
// every Changed() waiter (close-and-replace, never blocks). Caller holds the lock.
func (cl *client) bumpLocked() {
	cl.version++
	close(cl.changed)
	cl.changed = make(chan struct{})
}

// Version returns the GpuJobs content version; it increases on every change.
func (cl *client) Version() uint64 {
	cl.Lock()
	defer cl.Unlock()
	return cl.version
}

// Changed returns a channel closed on the next GpuJobs content change.
func (cl *client) Changed() <-chan struct{} {
	cl.Lock()
	defer cl.Unlock()
	return cl.changed
}

// ListWorkloads - returns the list of workloads
// for slurm it returns the list of jobs running on the gpus
// the key is the gpu id/render id (integer) and the value is the job info
func (cl *client) ListWorkloads() (map[string]Workload, error) {
	jobs := make(map[string]Workload)
	cl.Lock()
	defer cl.Unlock()
	for k, v := range cl.GpuJobs {
		jobs[k] = Workload{
			Type: Slurm,
			Info: v,
		}
	}
	return jobs, nil
}
func (cl *client) CheckExportLabels(labels map[string]bool) bool {
	for k := range SlurmLabels {
		if ok := labels[k]; ok {
			return true
		}
	}
	return false
}
func (cl *client) Close() error {
	if cl.ctx != nil {
		cl.cancel()
	}
	return nil
}

func (cl *client) Type() SchedulerType {
	return Slurm
}
