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

package nicagent

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	_ "github.com/alta/protopatch/patch" // nolint: gosec

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetricssvc"
	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

type NICAgentClient struct {
	sync.Mutex
	nicClients             []NICInterface
	mh                     *metricsutil.MetricsHandler
	m                      *metrics // client specific metrics
	isKubernetes           bool
	nics                   map[string]*NIC
	k8sScheduler           scheduler.SchedulerClient
	k8sApiClient           *k8sclient.K8sClient
	staticHostLabels       map[string]string // static labels for the host
	nodeHealthLabellerCfg  *utils.NodeHealthLabellerConfig
	ctx                    context.Context
	cancel                 context.CancelFunc
	rdmaDevToPcieAddr      map[string]string
	podnameToProcessId     map[string]int
	podnameToNetDeviceList map[string][]NetDevice
}

// NICAgentClientOptions defines the options for the NICAgentClient
type NICAgentClientOptions func(na *NICAgentClient)

// WithK8sSchedulerClient sets the Kubernetes scheduler client for the NICAgentClient
func WithK8sSchedulerClient(k8sScheduler scheduler.SchedulerClient) NICAgentClientOptions {
	return func(na *NICAgentClient) {
		if utils.IsKubernetes() {
			na.isKubernetes = true
			logger.Log.Printf("K8sSchedulerClient option set")
			na.k8sScheduler = k8sScheduler
		}
	}
}

// WithK8sClient sets the Kubernetes API client for the NICAgentClient
func WithK8sClient(k8sApiClient *k8sclient.K8sClient) NICAgentClientOptions {
	return func(na *NICAgentClient) {
		if utils.IsKubernetes() {
			na.isKubernetes = true
			logger.Log.Printf("K8sApiClient option set")
			na.k8sApiClient = k8sApiClient
		}
	}
}

func (na *NICAgentClient) initClients() error {
	logger.Log.Printf("Establishing connection to NIC clients")
	var errStr []string
	for _, client := range na.nicClients {
		if err := client.Init(); err != nil {
			errStr = append(errStr, fmt.Sprintf("%s err: %s", client.GetClientName(), err.Error()))
		} else {
			logger.Log.Printf("%s init success", client.GetClientName())
		}
	}
	if len(errStr) != 0 {
		return fmt.Errorf("%v", strings.Join(errStr, ","))
	}
	return nil
}

func NewAgent(mh *metricsutil.MetricsHandler, opts ...NICAgentClientOptions) *NICAgentClient {
	na := &NICAgentClient{
		mh:               mh,
		nics:             make(map[string]*NIC),
		staticHostLabels: make(map[string]string),
		nodeHealthLabellerCfg: &utils.NodeHealthLabellerConfig{
			LabelPrefix: globals.NICHealthLabelPrefix,
		},
	}

	for _, o := range opts {
		o(na)
	}

	na.nicClients = []NICInterface{}
	return na
}

func (na *NICAgentClient) Init() error {
	na.Lock()
	defer na.Unlock()
	na.initializeContext()

	// create NIC clients and init
	nicCtlClient := newNICCtlClient(na)
	na.nicClients = append(na.nicClients, nicCtlClient)

	na.rdmaDevToPcieAddr = make(map[string]string)
	na.podnameToProcessId = make(map[string]int)
	na.podnameToNetDeviceList = make(map[string][]NetDevice)
	rdmaStatsClient := newRDMAStatsClient(na)
	na.nicClients = append(na.nicClients, rdmaStatsClient)

	ethtoolClient := newEthtoolClient(na)
	na.nicClients = append(na.nicClients, ethtoolClient)

	err := na.initClients()
	if err != nil {
		logger.Log.Printf("NIC clients init failure err :%v", err)
		return err
	}

	na.mh.RegisterMetricsClient(na)

	if err := na.populateStaticHostLabels(); err != nil {
		logger.Log.Printf("failed to populate static host labels, err: %v", err)
		return err
	}

	// fetch all the static data that doesn't change (NIC, Port, Lif, etc.)
	nics, err := na.getNICs()
	if err != nil {
		logger.Log.Printf("failed get NICs, Ports and Lifs, err: %v", err)
		return err
	}
	na.nics = nics
	na.printNICs()

	return nil
}

func (na *NICAgentClient) addRdmaDevPcieAddrIfAbsent(rdmaDev string) error {
	na.Lock()
	defer na.Unlock()
	if _, ok := na.rdmaDevToPcieAddr[rdmaDev]; !ok {
		cmd := fmt.Sprintf(GetPcieAddrFromRdmaDevCmd, rdmaDev)
		out, err := ExecWithContext(cmd)
		if err != nil {
			return fmt.Errorf("failed to execute cmd %s: %s", cmd, err)
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "=")
		if len(parts) < 2 || parts[1] == "" {
			return fmt.Errorf("pcie addr info not found for %s", rdmaDev)
		}
		na.rdmaDevToPcieAddr[rdmaDev] = parts[1]
	}
	return nil
}

func (na *NICAgentClient) addPodPidIfAbsent(podName string, podNamespace string) error {
	na.Lock()
	defer na.Unlock()

	if _, ok := na.podnameToProcessId[podName]; !ok {
		processId, err := na.getPidOfPod(podName, podNamespace)
		if err != nil {
			logger.Log.Printf("failed to get pid for pod %s : %v", podName, err)
			return err
		}
		na.podnameToProcessId[podName] = processId
		//gsmTODO cache eviction when Pod gets deleted.
	}
	return nil
}

func (na *NICAgentClient) getPidOfPod(podName, ns string) (int, error) {

	logStr := fmt.Sprintf("podname %s, ns %s", podName, ns)
	cid, err := na.k8sApiClient.GetContainerIDforPod(podName, ns)
	if err != nil {
		return -1, fmt.Errorf("failed to find containerID for %s: %v", logStr, err)
	}

	parts := strings.Split(cid, "://")
	if len(parts) != 2 {
		return -1, fmt.Errorf("found invalid containerID: %s for %s", cid, logStr)
	}

	var cmd string
	ctrRuntime := parts[0]
	containerID := parts[1]
	switch ctrRuntime {
	case "cri-o":
		cmd = fmt.Sprintf(GetPIDFromContainerRuntimeCmd, CrioRuntimeSocket, containerID)
	case "containerd":
		cmd = fmt.Sprintf(GetPIDFromContainerRuntimeCmd, ContainerdRuntimeSocket, containerID)
	default:
		return -1, fmt.Errorf("found unsupported runtime %s for %s", ctrRuntime, logStr)
	}

	processID, err := ExecWithContext(cmd)
	if err != nil {
		logStr = fmt.Sprintf("runtime %s, containerID %s, %s", ctrRuntime, containerID, logStr)
		return -1, fmt.Errorf("failed to find pid for %s: %v", logStr, err)
	}
	processID = bytes.TrimSpace(processID)

	pidVal, err := strconv.Atoi(string(processID))
	if err != nil {
		return -1, fmt.Errorf("failed in integer conversion for pid %s, %s: %v", string(processID), logStr, err)
	}

	return pidVal, nil
}

func (na *NICAgentClient) getNetDevicesList(podInfo *scheduler.PodResourceInfo) ([]NetDevice, error) {

	var netDevices []NetDevice
	var cmd string
	var pid int
	var podName string

	// interfaces in workload pod are cached.
	if podInfo != nil {
		podName = podInfo.Pod
		netDevices, ok := na.podnameToNetDeviceList[podName]
		if ok {
			return netDevices, nil
		}
	}

	if podInfo != nil {
		pid = na.podnameToProcessId[podName]
		cmd = fmt.Sprintf(PodNetnsExecCmd+ShowRdmaDevicesCmd, pid)
	} else {
		cmd = ShowRdmaDevicesCmd
	}

	res, err := ExecWithContext(cmd)
	if err != nil {
		return netDevices, fmt.Errorf("failed to run cmd %s: %v", cmd, err)
	}

	lines := strings.Split(string(res), "\n")
	for i := range lines {
		roceDevName := ""
		pcieBusId := ""
		parts := strings.Fields(lines[i])
		partsLen := len(parts)
		for i, p := range parts {
			if p == "link" && i+1 < partsLen {
				roceDevName = strings.Split(parts[i+1], "/")[0]
				if err := na.addRdmaDevPcieAddrIfAbsent(roceDevName); err != nil {
					return netDevices, err
				}
				pcieBusId = na.rdmaDevToPcieAddr[roceDevName]
			}
			if p == "netdev" && i+1 < partsLen {
				intfName := parts[i+1]
				intfAlias := intfName

				var cmd string
				if podInfo != nil {
					cmd = fmt.Sprintf(PodNetnsExecCmd+ShowNetDeviceCmd, pid, intfName)
				} else {
					cmd = fmt.Sprintf(ShowNetDeviceCmd, intfName)
				}
				res, err := ExecWithContext(cmd)
				if err == nil {
					words := strings.Fields(string(res))
					for idx, w := range words {
						if w == "alias" && idx+1 < len(words) {
							//update intfAlias, if present in ip link show output
							intfAlias = words[idx+1]
						}
					}
				}
				netDevices = append(netDevices, NetDevice{
					IntfName:    intfName,
					RoceDevName: roceDevName,
					IntfAlias:   intfAlias,
					PodName:     podName,
					PCIeBusId:   pcieBusId,
				})

			}
		}
	}
	if podInfo != nil {
		na.podnameToNetDeviceList[podName] = netDevices
	}
	return netDevices, nil
}

// ListWorkloads returns the list of workloads by device ID
func (na *NICAgentClient) ListWorkloads() (map[string]scheduler.Workload, error) {
	if na.isKubernetes && na.k8sScheduler != nil {
		return na.k8sScheduler.ListWorkloads()
	}
	return nil, fmt.Errorf("scheduler is not initialized")
}

func (na *NICAgentClient) initializeContext() {
	ctx, cancel := context.WithCancel(context.Background())
	na.ctx = ctx
	na.cancel = cancel
}
func (na *NICAgentClient) initLocalCacheIfRequired() {
	na.Lock()
	defer na.Unlock()
	if na.nicClients != nil {
		for _, client := range na.nicClients {
			if len(na.nics) == 0 && client.GetClientName() == NICCtlClientName && client.IsActive() {
				// fetch all the static data that doesn't change (NIC, Port, Lif, etc.)
				nics, err := na.getNICs()
				if err != nil {
					logger.Log.Printf("failed get NICs, Ports and Lifs, err: %v", err)
				} else {
					na.nics = nics
				}
			}
		}
	}
}

func (na *NICAgentClient) getMetricsAll() error {
	var wg sync.WaitGroup
	na.initLocalCacheIfRequired()

	workloads, err := na.ListWorkloads()
	if err != nil {
		logger.Log.Printf("failed to list workloads, err: %v", err)
	}
	for i := range workloads {
		podInfo := workloads[i].Info.(scheduler.PodResourceInfo)
		if err := na.addPodPidIfAbsent(podInfo.Pod, podInfo.Namespace); err != nil {
			logger.Log.Printf("failure in pod2pid update for pod %s ns %s: %v",
				podInfo.Pod, podInfo.Namespace, err)
		}
	}
	k8PodLabelsMap, _ = na.fetchPodLabelsForNode()

	labels := na.populateLabelsFromNIC("")
	na.m.nicNodesTotal.With(labels).Set(float64(len(na.nics)))

	for _, client := range na.nicClients {
		wg.Add(1)
		go func(client NICInterface) {
			defer wg.Done()
			if client.IsActive() {
				if err := client.UpdateNICStats(workloads); err != nil {
					logger.Log.Printf("failed to update NIC stats, err: %v", err)
				}
			}
		}(client)
	}
	wg.Wait()
	return nil
}

func (na *NICAgentClient) sendNodeLabelUpdate(healthState map[string]interface{}) error {
	if !na.isKubernetes {
		return nil
	}

	// send update to label , reconnect logic tbd
	nodeName := utils.GetNodeName()
	if nodeName == "" {
		logger.Log.Printf("error getting node name on k8s deployment, skip label update")
		return fmt.Errorf("node name not found")
	}
	nicHealthStates := make(map[string]string)
	for nicPCIeAddr, h := range healthState {
		hs := h.(*nicmetricssvc.NICState)
		if hs.Health == strings.ToLower(nicmetricssvc.Health_HEALTHY.String()) {
			logger.Log.Printf("NIC %s is healthy, skipping label update", nicPCIeAddr)
			continue
		}

		nicPCIeAddr = strings.ReplaceAll(nicPCIeAddr, ":", "_") // replace ':' with '_' for label compatibility
		nicPCIeAddr = strings.ReplaceAll(nicPCIeAddr, ".", "_")
		nicHealthStates[nicPCIeAddr] = hs.Health
	}
	_ = na.k8sApiClient.UpdateHealthLabel(na.nodeHealthLabellerCfg, nodeName, nicHealthStates)
	return nil
}

func (na *NICAgentClient) fetchPodLabelsForNode() (map[string]map[string]string, error) {
	listMap := make(map[string]map[string]string)
	if utils.IsKubernetes() && len(extraPodLabelsMap) > 0 {
		return na.k8sApiClient.GetAllPods()
	}
	return listMap, nil
}

func (na *NICAgentClient) Close() {
	na.Lock()
	defer na.Unlock()
	if na.cancel != nil {
		na.cancel()
	}
	na.nicClients = []NICInterface{}
}
