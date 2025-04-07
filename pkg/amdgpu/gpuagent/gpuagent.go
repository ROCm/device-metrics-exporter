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

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
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
	k8sScheduler           scheduler.SchedulerClient
	slurmScheduler         scheduler.SchedulerClient
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

	if utils.IsKubernetes() {
		ga.isKubernetes = true
		k8sScl, err := scheduler.NewKubernetesClient(ga.ctx)
		if err != nil {
			logger.Log.Printf("gpu client init failure err :%v", err)
			return err
		}
		ga.k8sScheduler = k8sScl
	}
	slurmScl, err := scheduler.NewSlurmClient(ga.ctx, ga.enableZmq)
	if err != nil {
		logger.Log.Printf("gpu client init failure err :%v", err)
		return err
	}
	ga.slurmScheduler = slurmScl
	if ga.isKubernetes {
		ga.k8sLabelClient = k8sclient.NewClient(ga.ctx)
	}

	if err := ga.populateStaticHostLabels(); err != nil {
		return fmt.Errorf("error in populating static host labels, %v", err)
	}

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
	wls, _ := ga.ListWorkloads()
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
	if err != nil {
		return res, err
	}
	// filter out logical GPU
	nres := &amdgpu.GPUGetResponse{
		ApiStatus: res.ApiStatus,
		Response:  []*amdgpu.GPU{},
		ErrorCode: res.ErrorCode,
	}
	for _, gpu := range res.Response {
		if len(gpu.Status.GPUPartition) != 0 {
			// skip logical gpu objects
			continue
		}
		nres.Response = append(nres.Response, gpu)
	}
	return nres, err
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

// ListWorkloads - get all workloads from every client , lock must be taken by
// the caller
func (ga *GPUAgentClient) ListWorkloads() (wls map[string]scheduler.Workload, err error) {
	if ga.isKubernetes {
		wls, err = ga.k8sScheduler.ListWorkloads()
		if err != nil {
			return
		}
	}
	swls, err := ga.slurmScheduler.ListWorkloads()
	if err != nil {
		return
	}
	// return combined list
	for k, wl := range swls {
		wls[k] = wl
	}
	return
}

func (ga *GPUAgentClient) checkExportLabels(exportLabels map[string]bool) bool {
	if ga.isKubernetes {
		if ga.k8sScheduler.CheckExportLabels(exportLabels) {
			return true
		}
	}
	if ga.slurmScheduler.CheckExportLabels(exportLabels) {
		return true
	}
	return false
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
	if ga.k8sScheduler != nil {
		logger.Log.Printf("gpuagent k8s scheduler closing")
		ga.k8sScheduler.Close()
		ga.k8sScheduler = nil
	}

	if ga.slurmScheduler != nil {
		logger.Log.Printf("gpuagent slurm scheduler closing")
		ga.slurmScheduler.Close()
		ga.slurmScheduler = nil
	}
}
