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
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

const (
	rocprofilerTimeout = 15
	cachedTimer        = 10 * time.Second
	failThreshold      = 3
)

type ROCProfilerClient struct {
	Name         string
	MetricFields []string
	cmd          string
	pCache       *profilerCache
}

type profilerCache struct {
	sync.RWMutex
	cachedMetrics       *amdgpu.GpuProfiler
	cacheLastRead       time.Time
	consecutiveFailures int
	fatalFailure        bool
}

func NewRocProfilerClient(name string) *ROCProfilerClient {
	logger.Log.Printf("NewRocProfilerClient %v", name)
	return &ROCProfilerClient{
		Name:         name,
		MetricFields: []string{},
		pCache: &profilerCache{
			fatalFailure: false,
		},
	}
}

func (rpc *ROCProfilerClient) ResetFailureCount() {
	rpc.pCache.Lock()
	defer rpc.pCache.Unlock()
	rpc.pCache.consecutiveFailures = 0
}

func (rpc *ROCProfilerClient) SetFields(fields []string) {
	logger.Log.Printf("rocprofiler fields pulled for %v", strings.Join(fields, ","))
	rpc.MetricFields = fields
	rpc.cmd = fmt.Sprintf("rocpctl %v", strings.Join(fields, " "))
}

// cacheMetrics returns the cached metrics if they are fresh, otherwise it fetches new metrics
// and updates the cache. If the fetch fails, the cache is cleared and the error is returned.
// this is required to avoid frequent calls to rocpctl for metrics to avoid stress on hardware
func (rpc *ROCProfilerClient) cacheMetrics() (*amdgpu.GpuProfiler, error) {
	rpc.pCache.RLock()

	// If cache is fresh, return it
	if time.Since(rpc.pCache.cacheLastRead) < cachedTimer && rpc.pCache.cachedMetrics != nil {
		rpc.pCache.RUnlock()
		logger.Log.Printf("returning metrics from cache")
		return rpc.pCache.cachedMetrics, nil
	}
	rpc.pCache.RUnlock()

	// Otherwise, fetch new metrics and update cache
	metrics, err := rpc.getMetrics()
	rpc.pCache.Lock()
	rpc.pCache.cacheLastRead = time.Now()
	if err == nil {
		rpc.pCache.cachedMetrics = metrics
	} else {
		rpc.pCache.cachedMetrics = nil
	}
	rpc.pCache.Unlock()

	// No cache and failed to fetch
	return metrics, err
}

func (rpc *ROCProfilerClient) GetMetrics() (*amdgpu.GpuProfiler, error) {
	return rpc.cacheMetrics()
}

func (rpc *ROCProfilerClient) IncFailureCount() {
	rpc.pCache.Lock()
	defer rpc.pCache.Unlock()
	rpc.pCache.consecutiveFailures++
	// log only once when consecutive failures reach threshold
	// this is to avoid log spamming
	if rpc.pCache.consecutiveFailures == failThreshold {
		logger.Log.Printf("%v has failed %v times, disabling", rpc.Name, failThreshold)
	}
}

func (rpc *ROCProfilerClient) GetFailureCount() int {
	rpc.pCache.RLock()
	defer rpc.pCache.RUnlock()
	return rpc.pCache.consecutiveFailures
}

// IsDisabledOnFailure returns true if the profiler has been disabled due to consecutive failures
// or if the profiler has been disabled due to a core dump
// this is required to avoid frequent calls to rocpctl for metrics to avoid stress on hardware and avoid core dumps generation
func (rpc *ROCProfilerClient) IsDisabledOnFailure() bool {
	rpc.pCache.RLock()
	defer rpc.pCache.RUnlock()
	return rpc.pCache.fatalFailure || rpc.pCache.consecutiveFailures >= failThreshold
}

func (rpc *ROCProfilerClient) SetFatalFailureState() {
	rpc.pCache.Lock()
	defer rpc.pCache.Unlock()
	rpc.pCache.fatalFailure = true
	logger.Log.Printf(" %v has been disabled after system failure", rpc.Name)
}

func (rpc *ROCProfilerClient) getMetrics() (*amdgpu.GpuProfiler, error) {
	// Check consecutive failure count
	if rpc.IsDisabledOnFailure() {
		return nil, fmt.Errorf("%v disabled after consecutive failures", rpc.Name)
	}

	gpus := amdgpu.GpuProfiler{}

	if len(rpc.MetricFields) == 0 {
		return &gpus, nil
	}

	// Create a context with a 15s timeout
	ctx, cancel := context.WithTimeout(context.Background(), rocprofilerTimeout*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", rpc.cmd)
	gpuMetrics, err := cmd.Output()

	// Kill the process if it's still running (timeout or error case)
	if cmd.Process != nil {
		defer func() {
			if killErr := cmd.Process.Kill(); killErr == nil {
				// log only when process is killed successfully
				logger.Log.Printf("successfully killed process %v", cmd.Process.Pid)
			}
		}()
	}

	if ctx.Err() == context.DeadlineExceeded {
		logger.Log.Printf("command timed out after 15s: %v", rpc.cmd)
		rpc.IncFailureCount()
		return nil, ctx.Err()
	} else if err != nil {
		logger.Log.Printf("error occured : %v", err)

		// Check if error contains "core dumped" or "signal aborted" message
		if strings.Contains(err.Error(), "dumped") || strings.Contains(err.Error(), "aborted") {
			rpc.SetFatalFailureState()
			return nil, fmt.Errorf("%v core dumped, profiler disabled", rpc.Name)
		}

		rpc.IncFailureCount()
		return nil, err
	}

	err = json.Unmarshal(gpuMetrics, &gpus)
	if err != nil {
		logger.Log.Printf("error unmarshaling profiler statistics err :%v -> data: %v", err, string(gpuMetrics))
		rpc.IncFailureCount()
		return nil, err
	}
	rpc.ResetFailureCount()
	return &gpus, nil
}
