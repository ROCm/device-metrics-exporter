/*
*
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
*
*/
package gpuagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/fsysdevice"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/rocprofiler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

const (
	// cachgpuid are updated after this many pull request
	refreshInterval = 30 * time.Second
	queryTimeout    = 15 * time.Second
	cacheTimer      = 15 * time.Second
)

// Cache fields for GPUAgentClient
type gpuCache struct {
	sync.RWMutex
	lastResponse      *amdgpu.GPUGetResponse
	lastCperResponse  *amdgpu.GPUCPERGetResponse
	lastTimestamp     time.Time
	lastCperTimestamp time.Time
}

// cper cache entry
type cperCacheEntry struct {
	entry     *amdgpu.CPEREntry
	timestamp time.Time
}

type GPUAgentGPUClient struct {
	sync.Mutex
	metrics               *GpuMetrics // client specific metrics
	enableAfidMetrics     bool
	enableProfileMetrics  bool
	gpuclient             amdgpu.GPUSvcClient
	evtclient             amdgpu.EventSvcClient
	rocpclient            *rocprofiler.ROCProfilerClient
	gpuHandler            *GPUAgentClient
	staticHostLabels      map[string]string
	fsysDeviceHandler     *fsysdevice.FsysDevice
	healthState           map[string]*metricssvc.GPUState
	mockEccField          map[string]map[string]uint32 // gpuid->fields->count
	gCache                *gpuCache
	exportLabels          map[string]bool
	exportFieldMap        map[string]bool // all upper case keys
	fieldMetricsMap       map[string]FieldMeta
	gpuSelectorMap        map[int]bool
	customLabelMap        map[string]string
	extraPodLabelsMap     map[string]string
	allowedCustomLabels   []string
	k8PodLabelsMap        map[string]map[string]string
	nodeHealthLabellerCfg *utils.NodeHealthLabellerConfig
	fl                    *fieldLogger

	computeNodeHealthState bool // Tracks the health state of the compute node
}

func NewGPUAgentGPUClient(gpuHandler *GPUAgentClient) (*GPUAgentGPUClient, error) {
	gpuClient := &GPUAgentGPUClient{
		nodeHealthLabellerCfg: &utils.NodeHealthLabellerConfig{
			LabelPrefix: globals.GPUHealthLabelPrefix,
		},
		allowedCustomLabels: []string{
			exportermetrics.MetricLabel_CLUSTER_NAME.String(),
		},
		exportLabels:    make(map[string]bool),
		exportFieldMap:  make(map[string]bool),
		fieldMetricsMap: make(map[string]FieldMeta),
		gpuSelectorMap:  make(map[int]bool),
		gCache:          &gpuCache{},
		gpuHandler:      gpuHandler,
		fl:              gpuHandler.fl,
	}
	gpuClient.rocpclient = rocprofiler.NewRocProfilerClient("rocpclient")
	gpuClient.fsysDeviceHandler = fsysdevice.GetFsysDeviceHandler()
	gpuClient.healthState = make(map[string]*metricssvc.GPUState)
	gpuClient.mockEccField = make(map[string]map[string]uint32)

	if gpuHandler.enableSriov {
		logger.Log.Printf("profiler is disabled on sriov deployment")
		gpuClient.enableProfileMetrics = false
	} else {
		gpuClient.enableProfileMetrics = true
	}

	return gpuClient, nil
}

func (ga *GPUAgentGPUClient) Close() {
	if ga.rocpclient != nil {
		logger.Log.Printf("gpuagent rocp reset connection")
		ga.rocpclient.ResetFailureCount()
	}
}

func (ga *GPUAgentGPUClient) InitPodExtraLabels(config *exportermetrics.GPUMetricConfig) {
	// initialize pod labels maps
	ga.k8PodLabelsMap = make(map[string]map[string]string)
	if config != nil {
		ga.extraPodLabelsMap = utils.NormalizeExtraPodLabels(config.GetExtraPodLabels())
	}
	logger.Log.Printf("export-labels updated to %v", ga.extraPodLabelsMap)
}

func (ga *GPUAgentGPUClient) InitClients() error {
	conn := ga.gpuHandler.GetGRPCConnection()
	if conn == nil {
		return fmt.Errorf("grpc connection is nil")
	}
	ga.gpuclient = amdgpu.NewGPUSvcClient(conn)
	ga.evtclient = amdgpu.NewEventSvcClient(conn)
	return nil
}

func (ga *GPUAgentGPUClient) PopulateStaticHostLabels() error {
	ga.staticHostLabels = map[string]string{}
	hostname, err := utils.GetHostName()
	if err != nil {
		return err
	}
	logger.Log.Printf("hostame %v", hostname)
	ga.staticHostLabels[exportermetrics.MetricLabel_HOSTNAME.String()] = hostname
	return nil
}

// make it easy to parse from json
func (ga *GPUAgentGPUClient) getProfilerMetrics() (map[string]map[string]float64, error) {
	gpuMetrics := make(map[string]map[string]float64)
	// stop exporting fields when disabled
	if !ga.isProfilerEnabled() {
		return gpuMetrics, nil
	}
	gpuProfiler, err := ga.rocpclient.GetMetrics()
	if err != nil {
		return gpuMetrics, err
	}
	for _, gpu := range gpuProfiler.GpuMetrics {
		gpuMetric := make(map[string]float64)
		for _, m := range gpu.Metrics {
			gpuMetric[m.Field] = strDoubleToFloat(m.Value)
		}
		// associate using logical node id
		gpuMetrics[gpu.LogicalNodeId] = gpuMetric
	}
	return gpuMetrics, nil
}

func (ga *GPUAgentGPUClient) isProfilerEnabled() bool {
	if !ga.gpuHandler.enableGPUMonitoring {
		// gpu monitoring is disabled
		return false
	}
	if ga.rocpclient == nil || !ga.enableProfileMetrics {
		// profiler is disabled either at boot time or through configmap
		return false
	}
	return true
}

func (ga *GPUAgentGPUClient) getMetricsAll() error {
	// send the req to gpuclient
	resp, partitionMap, err := ga.getGPUs()
	if err != nil {
		return err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return fmt.Errorf("%v", resp.ApiStatus)
	}
	cper, err := ga.getLatestCPER()
	if err != nil {
		logger.Log.Printf("getLatestCPER failed with err : %v", err)
		cper = nil
	}
	wls, _ := ga.gpuHandler.ListWorkloads()
	pmetrics, err := ga.getProfilerMetrics()
	if err != nil {
		//continue as this may not be available at this time
		pmetrics = nil
	}
	ga.k8PodLabelsMap, err = ga.FetchPodLabelsForNode()
	if err != nil {
		logger.Log.Printf("FetchPodLabelsForNode failed with err : %v", err)
	}
	nonGpuLabels := ga.populateLabelsFromGPU(nil, nil, nil)
	ga.metrics.gpuNodesTotal.With(nonGpuLabels).Set(float64(len(resp.Response)))
	for _, gpu := range resp.Response {
		var gpuProfMetrics map[string]float64
		// if available use the data
		if pmetrics != nil {
			gpuid := getGPUNodeID(gpu)
			//nolint
			gpuProfMetrics, _ = pmetrics[gpuid]
		}
		ga.updateGPUInfoToMetrics(wls, gpu, partitionMap, gpuProfMetrics, cper)
	}

	return nil
}

// IsActive returns true if the client is initialized and active
func (ga *GPUAgentGPUClient) isActive() bool {
	return ga.gpuclient != nil && ga.evtclient != nil
}

// FetchPodLabelsForNode fetches pod labels for all pods running on this node
func (ga *GPUAgentGPUClient) FetchPodLabelsForNode() (map[string]map[string]string, error) {
	if !ga.gpuHandler.enabledK8sApi {
		return nil, nil
	}
	k8sSchedClient := ga.gpuHandler.GetK8sApiClient()
	if k8sSchedClient == nil {
		return nil, fmt.Errorf("k8s scheduler client is nil")
	}
	listMap := make(map[string]map[string]string)
	if ga.gpuHandler.enabledK8sApi && len(ga.extraPodLabelsMap) > 0 {
		return k8sSchedClient.GetAllPods()
	}
	return listMap, nil
}

// GetContext returns the context
func (ga *GPUAgentGPUClient) GetContext() context.Context {
	ctx := ga.gpuHandler.GetContext()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// cacheRead reads from cache if the last read was successful and within the cacheTimer
// otherwise it reads from hardware
// this ensures that we don't read from hardware too frequently as more clients are added
// and the number of reads increases
func (ga *GPUAgentGPUClient) cacheRead() (*amdgpu.GPUGetResponse, error) {
	now := time.Now()

	// First try fast path with RLock
	ga.gCache.RLock()
	if ga.gCache.lastResponse != nil && now.Sub(ga.gCache.lastTimestamp) < cacheTimer {
		res := ga.gCache.lastResponse
		ga.gCache.RUnlock()
		logger.Log.Printf("returning metrics from cache")
		return res, nil
	}
	ga.gCache.RUnlock()

	// Acquire full Lock to update cache
	ga.gCache.Lock()
	defer ga.gCache.Unlock()

	// Check again after acquiring Lock to handle the case where another goroutine has already updated the cache
	if ga.gCache.lastResponse != nil && time.Since(ga.gCache.lastTimestamp) < cacheTimer {
		logger.Log.Printf("returning metrics from cache (after double-check)")
		return ga.gCache.lastResponse, nil
	}

	// Perform query and update cache
	ctx, cancel := context.WithTimeout(ga.GetContext(), queryTimeout)
	defer cancel()

	res, err := ga.gpuclient.GPUGet(ctx, &amdgpu.GPUGetRequest{})
	ga.gCache.lastTimestamp = time.Now()
	if err == nil {
		ga.gCache.lastResponse = res
	} else {
		ga.gCache.lastResponse = nil
	}
	return res, err
}

func (ga *GPUAgentGPUClient) cacheCperRead() (*amdgpu.GPUCPERGetResponse, error) {
	now := time.Now()

	// First try fast path with RLock
	ga.gCache.RLock()
	if ga.gCache.lastCperResponse != nil && now.Sub(ga.gCache.lastCperTimestamp) < cacheTimer {
		res := ga.gCache.lastCperResponse
		ga.gCache.RUnlock()
		logger.Log.Printf("returning CPER metrics from cache")
		return res, nil
	}
	ga.gCache.RUnlock()

	// Acquire full Lock to update cache
	ga.gCache.Lock()
	defer ga.gCache.Unlock()

	// Check again after acquiring Lock to handle the case where another goroutine has already updated the cache
	if ga.gCache.lastCperResponse != nil && time.Since(ga.gCache.lastCperTimestamp) < cacheTimer {
		logger.Log.Printf("returning CPER metrics from cache (after double-check)")
		return ga.gCache.lastCperResponse, nil
	}

	// Perform query and update cache
	ctx, cancel := context.WithTimeout(ga.GetContext(), queryTimeout)
	defer cancel()

	res, err := ga.gpuclient.GPUCPERGet(ctx, &amdgpu.GPUCPERGetRequest{})
	ga.gCache.lastCperTimestamp = time.Now()
	if err == nil {
		ga.gCache.lastCperResponse = res
	} else {
		ga.gCache.lastCperResponse = nil
	}
	return res, err
}

// getLatestCPER fetches the latest CPER entry per GPU from the cached CPER data
func (ga *GPUAgentGPUClient) getLatestCPER() (map[string]*amdgpu.CPEREntry, error) {
	// skip if afid metrics are disabled - saves time on fetching CPER data
	if !ga.enableAfidMetrics {
		return nil, nil
	}
	gpuCpers, err := ga.cacheCperRead()
	if err != nil {
		return nil, err
	}
	if gpuCpers != nil && gpuCpers.ApiStatus != 0 {
		logger.Log.Printf("CPER resp status :%v", gpuCpers.ApiStatus)
		return nil, fmt.Errorf("%v", gpuCpers.ApiStatus)
	}

	latestCPER := make(map[string]*cperCacheEntry)

	for _, c := range gpuCpers.CPER {
		gpuuid := utils.UUIDToString(c.GPU)

		for _, record := range c.CPEREntry {
			ts := record.GetTimestamp() // this is of format Timestamp:"2025-09-12 15:00:27"
			// Parse current timestamp
			currentTS, err := time.Parse("2006-01-02 15:04:05", ts)
			if err != nil {
				logger.Log.Printf("Failed to parse current timestamp: %v", err)
				continue
			}

			if existingCPER, exists := latestCPER[gpuuid]; !exists {
				latestCPER[gpuuid] = &cperCacheEntry{
					entry:     record,
					timestamp: currentTS,
				}
			} else {
				// Update if current is more recent
				if currentTS.After(existingCPER.timestamp) {
					latestCPER[gpuuid] = &cperCacheEntry{
						entry:     record,
						timestamp: currentTS,
					}
				}
			}
		}
	}

	// Convert to return type and log summary
	result := make(map[string]*amdgpu.CPEREntry)
	for gpuuid, cacheEntry := range latestCPER {
		entry := cacheEntry.entry
		result[gpuuid] = entry
	}

	return result, nil
}

func (ga *GPUAgentGPUClient) getGPUs() (*amdgpu.GPUGetResponse, map[string]*amdgpu.GPU, error) {
	res, err := ga.cacheRead()
	if err != nil {
		return nil, nil, err
	}
	// filter out logical GPU
	nres := &amdgpu.GPUGetResponse{
		ApiStatus: res.ApiStatus,
		Response:  []*amdgpu.GPU{},
		ErrorCode: res.ErrorCode,
	}
	partitionMap := make(map[string]*amdgpu.GPU)
	for _, gpu := range res.Response {
		if gpu.Status.PCIeStatus != nil {
			gpuPcieAddr := strings.ToLower(gpu.Status.PCIeStatus.PCIeBusId)
			pcieBaseAddr := utils.GetPCIeBaseAddress(gpuPcieAddr)
			// parent gpu map is created only for partitioned gpu
			if (pcieBaseAddr != gpuPcieAddr) && (gpu.Status.GetPartitionId() == 0) {
				partitionMap[pcieBaseAddr] = gpu
			}
		}
		if len(gpu.Status.GPUPartition) != 0 {
			// skip logical gpu objects
			continue
		}
		nres.Response = append(nres.Response, gpu)
	}
	return nres, partitionMap, err
}

func (ga *GPUAgentGPUClient) getEvents(severity amdgpu.EventSeverity) (*amdgpu.EventResponse, error) {
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
	res, err := ga.evtclient.EventGet(ga.GetContext(), req)
	return res, err
}

func (ga *GPUAgentGPUClient) getGPUCPER(severity string) (*amdgpu.GPUCPERGetResponse, error) {
	ctx, cancel := context.WithTimeout(ga.GetContext(), queryTimeout)
	defer cancel()

	req := &amdgpu.GPUCPERGetRequest{}
	if severity != "" {
		if sevId, ok := amdgpu.CPERSeverity_value[strings.ToUpper(severity)]; ok {
			req.Severity = amdgpu.CPERSeverity(sevId)
		} else {
			logger.Log.Printf("invalid severity value %v. fetching all cper records", severity)
		}
	}
	res, err := ga.gpuclient.GPUCPERGet(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (ga *GPUAgentGPUClient) sendNodeLabelUpdate() error {
	if !ga.gpuHandler.enabledK8sApi {
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
	hss := ga.GetHealthState()
	ga.Unlock()
	for gpuid, hs := range hss {
		if hs.Health == strings.ToLower(metricssvc.GPUHealth_HEALTHY.String()) {
			continue // skip healthy state
		}
		gpuHealthStates[gpuid] = hs.Health
	}

	k8sClient := ga.gpuHandler.GetK8sApiClient()
	if k8sClient != nil {
		_ = k8sClient.UpdateHealthLabel(ga.nodeHealthLabellerCfg, nodeName, gpuHealthStates)
	}
	return nil
}

func (ga *GPUAgentGPUClient) GetHealthState() map[string]*metricssvc.GPUState {
	return ga.healthState
}
