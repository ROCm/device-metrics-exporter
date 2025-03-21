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

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/scheduler"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/utils"
	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// cachgpuid are updated after this many pull request
	refreshInterval = 30 * time.Second
	queryTimeout    = 10 * time.Second
)

type GPUAgentClient struct {
	sync.Mutex
	conn                   *grpc.ClientConn
	mh                     *metricsutil.MetricsHandler
	gpuclient              amdgpu.GPUSvcClient
	evtclient              amdgpu.EventSvcClient
	m                      *metrics // client specific metrics
	k8sLabelClient         *k8sclient.K8sClient
	schedulerCl            scheduler.SchedulerClient
	isKubernetes           bool
	enableZmq              bool
	staticHostLabels       map[string]string
	ctx                    context.Context
	cancel                 context.CancelFunc
	healthState            map[string]*metricssvc.GPUState
	mockEccField           map[string]map[string]uint32 // gpuid->fields->count
	computeNodeHealthState bool
}

func initclients(mh *metricsutil.MetricsHandler) (conn *grpc.ClientConn, gpuclient amdgpu.GPUSvcClient, evtclient amdgpu.EventSvcClient, err error) {
	agentAddr := mh.GetAgentAddr()
	logger.Log.Printf("Agent connecting to %v", agentAddr)
	conn, err = grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Log.Printf("err :%v", err)
		return
	}
	gpuclient = amdgpu.NewGPUSvcClient(conn)
	evtclient = amdgpu.NewEventSvcClient(conn)
	return
}

func initScheduler(ctx context.Context, enableZmq bool) (scheduler.SchedulerClient, error) {
	if utils.IsKubernetes() {
		logger.Log.Printf("NewKubernetesClient creating")
		return scheduler.NewKubernetesClient(ctx)
	}
	logger.Log.Printf("NewSlurmClient creating")
	return scheduler.NewSlurmClient(ctx, enableZmq)
}

func NewAgent(mh *metricsutil.MetricsHandler, enableZmq bool) *GPUAgentClient {
	ga := &GPUAgentClient{mh: mh, enableZmq: enableZmq, computeNodeHealthState: true}
	ga.healthState = make(map[string]*metricssvc.GPUState)
	ga.mockEccField = make(map[string]map[string]uint32)
	mh.RegisterMetricsClient(ga)
	return ga
}

func (ga *GPUAgentClient) Init() error {
	ga.Lock()
	defer ga.Unlock()
	ga.initializeContext()
	conn, gpuclient, evtclient, err := initclients(ga.mh)
	if err != nil {
		logger.Log.Printf("gpu client init failure err :%v", err)
		return err
	}

	ga.conn = conn
	ga.gpuclient = gpuclient
	ga.evtclient = evtclient

	schedulerCl, err := initScheduler(ga.ctx, ga.enableZmq)
	if err != nil {
		logger.Log.Printf("gpu client init failure err :%v", err)
		return err
	}
	ga.schedulerCl = schedulerCl
	if utils.IsKubernetes() {
		ga.isKubernetes = true
		ga.k8sLabelClient = k8sclient.NewClient(ga.ctx)
	}

	if err := ga.populateStaticHostLabels(); err != nil {
		return fmt.Errorf("error in populating static host labels, %v", err)
	}

	logger.Log.Printf("monitor %v jobs", map[bool]string{true: "kubernetes", false: "slurm"}[ga.isKubernetes])

	return nil
}

func (ga *GPUAgentClient) initializeContext() {
	ctx, cancel := context.WithCancel(context.Background())
	ga.ctx = ctx
	ga.cancel = cancel
}

func (ga *GPUAgentClient) reconnect() error {
	ga.Close()
	return ga.Init()
}

func (ga *GPUAgentClient) isActive() bool {
	ga.Lock()
	defer ga.Unlock()
	return ga.gpuclient != nil
}

func (ga *GPUAgentClient) StartMonitor() {
	logger.Log.Printf("GPUAgent monitor started")
	ga.initializeContext()
	pollTimer := time.NewTicker(refreshInterval)
	defer pollTimer.Stop()

	for {
		select {
		case <-ga.ctx.Done():
			logger.Log.Printf("gpuagent client connection closing")
			ga.Close()
			return
		case <-pollTimer.C:
			if !ga.isActive() {
				if err := ga.reconnect(); err != nil {
					logger.Log.Printf("gpuagent connection failed %v", err)
					continue
				}
			}
			ga.processHealthValidation()
			ga.sendNodeLabelUpdate()
		}
	}
}

func (ga *GPUAgentClient) sendNodeLabelUpdate() error {
	if !ga.isKubernetes {
		return nil
	}
	// send update to label , reconnect logic tbd
	nodeName := utils.GetNodeName()
	if nodeName == "" {
		logger.Log.Printf("error getting node name on k8s deployment, skip label update")
		return fmt.Errorf("node name not found")
	}
	gpuHealthStates := make(map[string]string)
	ga.Lock()
	for gpuid, hs := range ga.healthState {
		gpuHealthStates[gpuid] = hs.Health
	}
	ga.Unlock()
	_ = ga.k8sLabelClient.UpdateHealthLabel(nodeName, gpuHealthStates)
	return nil
}

func (ga *GPUAgentClient) getMetricsAll() error {
	// send the req to gpuclient
	resp, err := ga.getGPUs()
	if err != nil {
		return err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return fmt.Errorf("%v", resp.ApiStatus)
	}
	wls, _ := ga.schedulerCl.ListWorkloads()
	for _, gpu := range resp.Response {
		ga.updateGPUInfoToMetrics(wls, gpu)
	}

	return nil
}

func (ga *GPUAgentClient) getGPUs() (*amdgpu.GPUGetResponse, error) {
	if !ga.isActive() {
		ga.reconnect()
	}

	ctx, cancel := context.WithTimeout(ga.ctx, queryTimeout)
	defer cancel()

	req := &amdgpu.GPUGetRequest{}
	res, err := ga.gpuclient.GPUGet(ctx, req)
	return res, err
}

func (ga *GPUAgentClient) getEvents(severity amdgpu.EventSeverity) (*amdgpu.EventResponse, error) {
	req := &amdgpu.EventRequest{}
	if severity != amdgpu.EventSeverity_EVENT_SEVERITY_NONE {
		req.Filter = &amdgpu.EventFilter{
			Filter: &amdgpu.EventFilter_MatchAttrs{
				MatchAttrs: &amdgpu.EventMatchAttrs{
					Severity: severity,
				},
			},
		}
	}
	res, err := ga.evtclient.EventGet(ga.ctx, req)
	return res, err
}

func (ga *GPUAgentClient) Close() {
	ga.Lock()
	defer ga.Unlock()
	if ga.conn != nil {
		logger.Log.Printf("gpuagent client closing")
		ga.conn.Close()
		ga.gpuclient = nil
		ga.conn = nil
	}
	if ga.schedulerCl != nil {
		logger.Log.Printf("gpuagent scheduler closing")
		ga.schedulerCl.Close()
		ga.schedulerCl = nil
	}
}
