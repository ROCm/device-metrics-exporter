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
	"strconv"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

type GPUAgentClient struct {
	sync.Mutex
	conn                 *grpc.ClientConn
	mh                   *metricsutil.MetricsHandler
	k8sApiClient         *k8sclient.K8sClient
	k8sScheduler         scheduler.SchedulerClient
	slurmScheduler       scheduler.SchedulerClient
	enableGPUMonitoring  bool
	enableIFOEMonitoring bool
	isKubernetes         bool // pod resource client enabled or not
	enabledK8sApi        bool
	enableSlurmScl       bool
	enableSriov          bool

	ctx    context.Context
	cancel context.CancelFunc

	computeNodeHealthState bool
	extraPodLabelsMap      map[string]string
	k8PodLabelsMap         map[string]map[string]string

	fl      *fieldLogger
	clients []GPUAgentClientInterface
}

// GPUAgentClientOptions set desired options
type GPUAgentClientOptions func(ga *GPUAgentClient)

func WithK8sClient(k8sclient *k8sclient.K8sClient) GPUAgentClientOptions {
	return func(ga *GPUAgentClient) {
		if k8sclient == nil {
			logger.Log.Printf("K8sApiClient disabled")
			ga.k8sApiClient = nil
			return
		}
		ga.enabledK8sApi = true
		logger.Log.Printf("K8sApiClient option set")
		ga.k8sApiClient = k8sclient
	}
}

func WithSRIOV(enableSriov bool) GPUAgentClientOptions {
	return func(ga *GPUAgentClient) {
		logger.Log.Printf("sriov mode set %v", enableSriov)
		ga.enableSriov = enableSriov
	}
}

func WithK8sSchedulerClient(k8sScheduler scheduler.SchedulerClient) GPUAgentClientOptions {
	return func(ga *GPUAgentClient) {
		if utils.IsKubernetes() {
			ga.isKubernetes = true
			logger.Log.Printf("K8sSchedulerClient option set")
			ga.k8sScheduler = k8sScheduler
		}
	}
}

func WithSlurmClient(enable bool) GPUAgentClientOptions {
	return func(ga *GPUAgentClient) {
		logger.Log.Printf("slurm scheduler client set to %v", enable)
		ga.enableSlurmScl = enable
	}
}

func WithGPUMonitoring(enableGPUMonitoring bool) GPUAgentClientOptions {
	return func(ga *GPUAgentClient) {
		logger.Log.Printf("GPU monitoring enable %v", enableGPUMonitoring)
		ga.enableGPUMonitoring = enableGPUMonitoring
	}
}
func WithIFOEMonitoring(enableIFOEMonitoring bool) GPUAgentClientOptions {
	return func(ga *GPUAgentClient) {
		logger.Log.Printf("IFOE monitoring enable %v", enableIFOEMonitoring)
		ga.enableIFOEMonitoring = enableIFOEMonitoring
	}
}

func (ga *GPUAgentClient) GetGRPCConnection() *grpc.ClientConn {
	return ga.conn
}

func (ga *GPUAgentClient) initclients() (err error) {
	agentAddr := ga.mh.GetAgentAddr()
	logger.Log.Printf("Agent connecting to %v", agentAddr)
	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Log.Printf("err :%v", err)
		return
	}
	ga.conn = conn
	for _, client := range ga.clients {
		err = client.InitClients()
		if err != nil {
			logger.Log.Printf("err :%v", err)
			return
		}
	}
	return
}

func NewAgent(mh *metricsutil.MetricsHandler, opts ...GPUAgentClientOptions) *GPUAgentClient {
	ga := &GPUAgentClient{
		mh:                     mh,
		computeNodeHealthState: true,
		enableGPUMonitoring:    true,
		enableIFOEMonitoring:   false,
	}
	for _, o := range opts {
		o(ga)
	}

	if !ga.enableGPUMonitoring && !ga.enableIFOEMonitoring {
		logger.Log.Printf("both GPU and IFOE monitoring are disabled, returning nil agent")
		return nil
	}

	ga.fl = NewFieldLogger()

	if ga.enableGPUMonitoring {
		gpuClient, err := NewGPUAgentGPUClient(ga)
		if err != nil {
			logger.Log.Printf("error creating GPU client: %v", err)
			return nil
		}
		ga.clients = append(ga.clients, gpuClient)
	}

	if ga.enableIFOEMonitoring {
		ifoeClient, err := NewGPUAgentIFOEClient(ga)
		if err != nil {
			logger.Log.Printf("error creating IFOE client: %v", err)
			return nil
		}
		ga.clients = append(ga.clients, ifoeClient)
	}

	mh.RegisterMetricsClient(ga)

	return ga
}

func (ga *GPUAgentClient) InitConfigs() error {
	for _, client := range ga.clients {
		err := client.InitConfigs()
		if err != nil {
			return err
		}
	}
	return nil
}

func (ga *GPUAgentClient) initalizeScheduler() error {
	if ga.enableSlurmScl {
		slurmScl, err := scheduler.NewSlurmClient(ga.ctx)
		if err != nil {
			logger.Log.Printf("gpu client init failure err :%v", err)
			return err
		}
		ga.slurmScheduler = slurmScl
	}
	return nil
}

func (ga *GPUAgentClient) Init() error {
	ga.Lock()
	defer ga.Unlock()
	ga.initializeContext()
	err := ga.initclients()
	if err != nil {
		logger.Log.Printf("gpu client init failure err :%v", err)
		return err
	}

	err = ga.initalizeScheduler()
	if err != nil {
		logger.Log.Printf("gpu client scheduler init failure err :%v", err)
		return err
	}

	if err := ga.populateStaticHostLabels(); err != nil {
		return fmt.Errorf("error in populating static host labels, %v", err)
	}

	return nil
}

func (ga *GPUAgentClient) populateStaticHostLabels() error {
	for _, client := range ga.clients {
		_ = client.PopulateStaticHostLabels()

	}
	return nil

}

func (ga *GPUAgentClient) GetGPUHealthStates() (map[string]interface{}, error) {
	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		return client.GetHealthStates()
	}
	return nil, nil
}

func (ga *GPUAgentClient) SetError(id string, fields []string, counts []uint32) error {
	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		return client.SetError(id, fields, counts)
	}
	return nil
}

func (ga *GPUAgentClient) SetComputeNodeHealthState(state bool) {
	for _, client := range ga.clients {
		client.SetComputeNodeHealthState(state)
	}
}

func (ga *GPUAgentClient) GetContext() context.Context {
	return ga.ctx
}

func (ga *GPUAgentClient) initializeContext() {
	ctx, cancel := context.WithCancel(context.Background())
	ga.ctx = ctx
	ga.cancel = cancel
	for _, client := range ga.clients {
		//nolint
		_ = client.InitClients()
	}
}

func (ga *GPUAgentClient) reconnect() error {
	ga.Close()
	return ga.Init()
}

func (ga *GPUAgentClient) isActive() bool {
	ga.Lock()
	defer ga.Unlock()
	for _, client := range ga.clients {
		if !client.isActive() {
			return false
		}
	}
	return true
}

func (ga *GPUAgentClient) StartMonitor() {
	logger.Log.Printf("GPUAgent monitor started")
	ga.initializeContext()

	// Get health polling interval from configuration
	pollInterval := ga.mh.GetHealthPollingInterval()
	logger.Log.Printf("Health polling interval set to %v", pollInterval)

	pollTimer := time.NewTicker(pollInterval)
	defer pollTimer.Stop()

	// nolint
	for {
		select {
		case <-pollTimer.C:
			if !ga.isActive() {
				if err := ga.reconnect(); err != nil {
					logger.Log.Printf("gpuagent connection failed %v", err)
					continue
				}
			}

			if ga.enableGPUMonitoring {
				if err := ga.processHealthValidation(); err != nil {
					logger.Log.Printf("gpuagent health validation failed %v", err)
				}
				if err := ga.sendNodeLabelUpdate(); err != nil {
					logger.Log.Printf("gpuagent failed to send node label update %v", err)
				}
			}
		}
	}
}

// processHealthValidation - process health validation for all clients
func (ga *GPUAgentClient) processHealthValidation() error {
	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		err := client.processHealthValidation()
		if err != nil {
			logger.Debugf("gpuagent client %v health validation failed %v", client.GetDeviceType(), err)
			return err
		}
	}
	return nil
}

// sendNodeLabelUpdate - send node label update for all clients
func (ga *GPUAgentClient) sendNodeLabelUpdate() error {
	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		err := client.sendNodeLabelUpdate()
		if err != nil {
			return err
		}
	}
	return nil
}

func strDoubleToFloat(strValue string) float64 {
	floatValue, err := strconv.ParseFloat(strValue, 64)
	if err != nil {
		fmt.Println("Error parsing string:", err)
		return 0.0
	}
	return floatValue
}

// ListWorkloads - get all workloads from every client , lock must be taken by
// the caller
func (ga *GPUAgentClient) ListWorkloads() (wls map[string]scheduler.Workload, err error) {
	wls = make(map[string]scheduler.Workload)
	if ga.isKubernetes && ga.k8sScheduler != nil {
		var k8sWls map[string]scheduler.Workload
		k8sWls, err = ga.k8sScheduler.ListWorkloads()
		if err != nil {
			return
		}
		for k, wl := range k8sWls {
			wls[k] = wl
		}
	}
	if ga.slurmScheduler == nil {
		return wls, nil
	}
	var swls map[string]scheduler.Workload
	swls, err = ga.slurmScheduler.ListWorkloads()
	if err != nil {
		return
	}
	// return combined list
	for k, wl := range swls {
		wls[k] = wl
	}
	return
}

func (ga *GPUAgentClient) Close() {
	ga.Lock()
	defer ga.Unlock()
	if ga.conn != nil {
		logger.Log.Printf("gpuagent closing")
		ga.conn.Close()
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

	for _, client := range ga.clients {
		logger.Log.Printf("gpuagent client %v closing", client.GetDeviceType())
		client.Close()
	}
	// cancel all context
	ga.cancel()
}

func (ga *GPUAgentClient) GetDeviceType() globals.DeviceType {
	return globals.GPUDevice

}

func (ga *GPUAgentClient) GetK8sApiClient() *k8sclient.K8sClient {
	return ga.k8sApiClient
}

func (ga *GPUAgentClient) UpdateStaticMetrics() error {
	for _, client := range ga.clients {
		//nolint
		_ = client.UpdateStaticMetrics()
	}
	return nil
}

func (ga *GPUAgentClient) UpdateMetricsStats() error {
	for _, client := range ga.clients {
		//nolint
		_ = client.UpdateMetricsStats()
	}
	return nil
}

func (ga *GPUAgentClient) ResetMetrics() error {
	for _, client := range ga.clients {
		_ = client.ResetMetrics()
	}
	return nil
}

// applicable only for GPU device type for now
func (ga *GPUAgentClient) QueryMetrics() (interface{}, error) {
	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		resp, err := client.QueryMetrics()
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	return nil, fmt.Errorf("no clients available for querying metrics")
}

// applicable only for GPU device type
func (ga *GPUAgentClient) QueryInbandRASErrors(severity string) (interface{}, error) {
	for _, client := range ga.clients {
		if client.GetDeviceType() != globals.GPUDevice {
			continue
		}
		resp, err := client.QueryInbandRASErrors(severity)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	return nil, fmt.Errorf("no clients enabled/available for querying inband ras errors")
}
