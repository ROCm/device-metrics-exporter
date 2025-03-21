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
	"strconv"
	"strings"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/gpumetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
	"github.com/fsnotify/fsnotify"
	zmq "github.com/go-zeromq/zmq4"
)

var SlurmLabels = map[string]bool{
	gpumetrics.GPUMetricLabel_JOB_ID.String(): true,
}

type client struct {
	sync.Mutex
	zmqSock zmq.Socket
	GpuJobs map[string]JobInfo
	ctx     context.Context
}

// NewSlurmClient - creates a slurm schedler client
func NewSlurmClient(ctx context.Context, enableZmq bool) (SchedulerClient, error) {
	sock := zmq.NewPull(ctx)

	if enableZmq {
		logger.Log.Printf("Starting Listen on port %v", globals.ZmqPort)
		if err := sock.Listen(fmt.Sprintf("tcp://*:%v", globals.ZmqPort)); err != nil {
			return nil, fmt.Errorf("failed to listen on port %v, %v ", globals.ZmqPort, err)
		}
	}

	cl := &client{
		zmqSock: sock,
		GpuJobs: make(map[string]JobInfo),
		ctx:     ctx,
	}

	go func() {
		os.MkdirAll(path.Dir(globals.SlurmDir), 0644)

		// Create new watcher.
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			logger.Log.Fatal(err)
		}
		defer watcher.Close()

		// Start listening for events.
		go func() {
			for ctx.Err() == nil {
				select {
				case event, ok := <-watcher.Events:
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
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					logger.Log.Printf("error: %v", err)
				}
			}
		}()

		// Add a path.
		err = watcher.Add(globals.SlurmDir)
		if err != nil {
			logger.Log.Fatalf("fsnotify watch err: %v", err)
		}

		// read existing
		if fds, err := os.ReadDir(globals.SlurmDir); err == nil {
			for _, f := range fds {
				watcher.Events <- fsnotify.Event{Name: globals.SlurmDir + "/" + f.Name(), Op: fsnotify.Write}
			}
		}

		// Block main goroutine forever.
		<-make(chan struct{})
	}()

	logger.Log.Printf("created slurm scheduler client")
	return cl, nil
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
			cl.Lock()
			for _, allocGPU := range strings.Split(gpus, ",") {
				cl.GpuJobs[allocGPU] = JobInfo{
					Id:        jobEnv["SLURM_JOB_ID"],
					User:      jobEnv["SLURM_JOB_USER"],
					Partition: jobEnv["SLURM_JOB_PARTITION"],
					Cluster:   jobEnv["SLURM_CLUSTER_NAME"],
				}
			}
			cl.Unlock()
			logger.Log.Printf("updated %v", cl.GpuJobs)
		}
	} else {
		cl.Lock()
		delete(cl.GpuJobs, fmt.Sprintf("%v", name))

		cl.Unlock()
		logger.Log.Printf("updated gpu %v jobs %v", name, cl.GpuJobs)
	}
}

func (cl *client) ListWorkloads() (map[string]interface{}, error) {
	jobs := make(map[string]interface{})
	cl.Lock()
	defer cl.Unlock()
	for k, v := range cl.GpuJobs {
		jobs[k] = v
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
	cl.zmqSock.Close()
	return nil
}

func (cl *client) Type() SchedulerType {
	return Slurm
}
