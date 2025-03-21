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
	"fmt"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/internal/slurm"

	"github.com/ROCm/device-metrics-exporter/internal/k8s"

	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/logger"
	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/metricsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// cachgpuid are updated after this many pull request
	refreshInterval = 10
)

type GPUAgentClient struct {
	sync.Mutex
	conn             *grpc.ClientConn
	mh               *metricsutil.MetricsHandler
	client           amdgpu.GPUSvcClient
	m                *metrics // client specific metrics
	kubeClient       k8s.PodResourcesService
	isKubernetes     bool
	slurmClient      slurm.JobsService
	cacheGpuids      map[string][]byte
	cachePulls       int
	staticHostLabels map[string]string
}

func NewAgent(ctx context.Context, mh *metricsutil.MetricsHandler) (*GPUAgentClient, error) {
	agentAddr := mh.GetAgentAddr()
	logger.Log.Printf("Agent connecting to %v", agentAddr)
	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Log.Printf("err :%v", err)
		return nil, err
	}
	client := amdgpu.NewGPUSvcClient(conn)

	ga := &GPUAgentClient{
		conn:   conn,
		client: client,
		mh:     mh,
	}

	if k8s.IsKubernetes() {
		kubeClient, err := k8s.NewClient()
		if err != nil {
			return nil, fmt.Errorf("error in kubelet client, %v", err)
		}
		ga.isKubernetes = true
		ga.kubeClient = kubeClient
	} else {
		cli, err := slurm.NewClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("error in slurm client, %v", err)
		}
		ga.slurmClient = cli
		ga.isKubernetes = false
	}

	if err := ga.populateStaticHostLabels(); err != nil {
		return nil, fmt.Errorf("error in populating static host labels, %v", err)
	}

	logger.Log.Printf("monitor %v jobs", map[bool]string{true: "kubernetes", false: "slurm"}[ga.isKubernetes])
	ga.cacheGpuids = make(map[string][]byte)
	mh.RegisterMetricsClient(ga)

	return ga, nil
}

func (ga *GPUAgentClient) getMetricsAll() error {
	// send the req to gpuclient
	resp, err := ga.getMetrics()
	if err != nil {
		// crash to let service restart
		logger.Log.Fatalf("err :%v", err)
		return err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return fmt.Errorf("%v", resp.ApiStatus)
	}
	for _, gpu := range resp.Response {
		ga.updateGPUInfoToMetrics(gpu)
	}

	return nil
}

func (ga *GPUAgentClient) getMetricsBulkReq() error {
	// create multiple workers
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*5))
	defer cancel()
	responses, err := NewWokerRequest(ctx, ga.client, ga.cacheGpuids)
	if err != nil {
		logger.Log.Printf("worker request errored: %v", err)
		return err
	}
	for _, gpuRes := range responses {
		ga.updateGPUInfoToMetrics(gpuRes.Response[0])
	}
	// this handle gpu dynamically being added to the system
	// to refresh the cachedgpu ids
	ga.cachePulls++
	if ga.cachePulls == refreshInterval {
		ga.cachePulls = 0
		go ga.UpdateStaticMetrics()
	}
	return nil
}

func (ga *GPUAgentClient) getMetrics() (*amdgpu.GPUGetResponse, error) {
	ga.Lock()
	defer ga.Unlock()
	if ga.client == nil {
		return nil, fmt.Errorf("client closed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &amdgpu.GPUGetRequest{}
	res, err := ga.client.GPUGet(ctx, req)
	return res, err
}

func (ga *GPUAgentClient) Close() {
	ga.Lock()
	defer ga.Unlock()
	if ga.conn != nil {
		ga.conn.Close()
		ga.client = nil
	}
	if ga.isKubernetes {
		ga.kubeClient.Close()
		ga.kubeClient = nil

	} else {
		ga.slurmClient.Close()
		ga.slurmClient = nil
	}
}
