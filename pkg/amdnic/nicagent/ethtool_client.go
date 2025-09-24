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
	"fmt"
	"os/exec"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"gopkg.in/yaml.v2"
)

type EthtoolClient struct {
	sync.Mutex
	na *NICAgentClient
}

func newEthtoolClient(na *NICAgentClient) *EthtoolClient {
	nc := &EthtoolClient{na: na}
	return nc
}

func (ec *EthtoolClient) Init() error {
	ec.Lock()
	defer ec.Unlock()

	// Init logic goes here

	return nil
}
func (ec *EthtoolClient) IsActive() bool {
	ec.Lock()
	defer ec.Unlock()
	if _, err := exec.LookPath(EthtoolBinary); err == nil {
		return true
	}
	return false
}

func (ec *EthtoolClient) GetClientName() string {
	return EthtoolClientName
}

// yamlifyEthtoolOutput removes the header ("NIC statistics:") and leading spaces from each line
func yamlifyEthtoolOutput(res []byte) []byte {
	lines := []byte{}
	for i, line := range bytes.Split(res, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		// Skip the first line (header)
		if i == 0 || len(trimmed) == 0 {
			continue
		}
		lines = append(lines, trimmed...)
		lines = append(lines, '\n')
	}
	return lines
}

func (ec *EthtoolClient) UpdateNICStats(workloads map[string]scheduler.Workload) error {
	if !fetchEthtoolMetrics {
		return nil
	}
	ec.Lock()
	defer ec.Unlock()

	// fetch Host Interface Stats
	var nilPodInfo *scheduler.PodResourceInfo
	if err := ec.fetchEthStatsForDevicesInPod(nilPodInfo); err != nil {
		logger.Log.Printf("failure to fetch stats for devices in Host: %v", err)
	}

	// fetch K8s Workload Pod Interface Stats
	for i := range workloads {
		wlPodInfo := (workloads[i].Info.(scheduler.PodResourceInfo))
		if err := ec.fetchEthStatsForDevicesInPod(&wlPodInfo); err != nil {
			logger.Log.Printf("failure to fetch stats for devices in pod: %v", err)
		}
	}

	return nil
}

// For Host Devices, podInfo arg value will be nil
func (ec *EthtoolClient) fetchEthStatsForDevicesInPod(podInfo *scheduler.PodResourceInfo) error {
	netDevList, err := ec.na.getNetDevicesList(podInfo)
	if err != nil {
		logger.Log.Printf("failed to get netDevices in podInfo %v: %v", podInfo, err)
		return err
	}

	for i := range netDevList {
		labels := ec.na.populateLabelsForNetDevice(netDevList[i], podInfo)
		if err := ec.populateEthStatsForNetDevice(netDevList[i], labels); err != nil {
			logger.Log.Printf("failure in fetch for ethstats of netDev %v : %v", netDevList[i], err)
		}
	}

	return nil
}

func (ec *EthtoolClient) populateEthStatsForNetDevice(netDev NetDevice, labels map[string]string) error {
	var cmd string
	if netDev.PodName == "" {
		cmd = fmt.Sprintf(EthToolCmd, netDev.IntfName)
	} else {
		netDevPid := ec.na.podnameToProcessId[netDev.PodName]
		cmd = fmt.Sprintf(PodNetnsExecCmd+EthToolCmd, netDevPid, netDev.IntfName)
	}

	res, err := ExecWithContext(cmd)
	if err != nil {
		logger.Log.Printf(
			"failed to get intf stats for device %s, alias %s in pod %s: %v",
			netDev.IntfName, netDev.IntfAlias, netDev.PodName, err)
		return err
	}

	yamlifiedRes := yamlifyEthtoolOutput(res)

	var ethtoolStats nicmetrics.EthtoolStats
	err = yaml.Unmarshal(yamlifiedRes, &ethtoolStats)
	if err != nil {
		logger.Log.Printf("failed to unmarshal ethtool stats for lif %s IntfAlias %s: %v",
			labels[LabelEthIntfName], labels[LabelEthIntfAlias], err)
		return err
	}

	ec.na.m.ethTxPackets.With(labels).Set(float64(ethtoolStats.TX_PACKETS))
	ec.na.m.ethTxBytes.With(labels).Set(float64(ethtoolStats.TX_BYTES))
	ec.na.m.ethRxPackets.With(labels).Set(float64(ethtoolStats.RX_PACKETS))
	ec.na.m.ethRxBytes.With(labels).Set(float64(ethtoolStats.RX_BYTES))
	ec.na.m.ethFramesRxBroadcast.With(labels).Set(float64(ethtoolStats.FRAMES_RX_BROADCAST))
	ec.na.m.ethFramesRxMulticast.With(labels).Set(float64(ethtoolStats.FRAMES_RX_MULTICAST))
	ec.na.m.ethFramesTxBroadcast.With(labels).Set(float64(ethtoolStats.FRAMES_TX_BROADCAST))
	ec.na.m.ethFramesTxMulticast.With(labels).Set(float64(ethtoolStats.FRAMES_TX_MULTICAST))
	ec.na.m.ethFramesRxPause.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PAUSE))
	ec.na.m.ethFramesTxPause.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PAUSE))
	ec.na.m.ethFramesRx64b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_64B))
	ec.na.m.ethFramesRx65b127b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_65B_127B))
	ec.na.m.ethFramesRx128b255b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_128B_255B))
	ec.na.m.ethFramesRx256b511b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_256B_511B))
	ec.na.m.ethFramesRx512b1023b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_512B_1023B))
	ec.na.m.ethFramesRx1024b1518b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_1024B_1518B))
	ec.na.m.ethFramesRx1519b2047b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_1519B_2047B))
	ec.na.m.ethFramesRx2048b4095b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_2048B_4095B))
	ec.na.m.ethFramesRx4096b8191b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_4096B_8191B))
	ec.na.m.ethFramesRxBadFcs.With(labels).Set(float64(ethtoolStats.FRAMES_RX_BAD_FCS))
	ec.na.m.ethFramesRxPri4.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_4))
	ec.na.m.ethFramesTxPri4.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_4))
	ec.na.m.ethFramesRxPri0.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_0))
	ec.na.m.ethFramesRxPri1.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_1))
	ec.na.m.ethFramesRxPri2.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_2))
	ec.na.m.ethFramesRxPri3.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_3))
	ec.na.m.ethFramesRxPri5.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_5))
	ec.na.m.ethFramesRxPri6.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_6))
	ec.na.m.ethFramesRxPri7.With(labels).Set(float64(ethtoolStats.FRAMES_RX_PRI_7))
	ec.na.m.ethFramesTxPri0.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_0))
	ec.na.m.ethFramesTxPri1.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_1))
	ec.na.m.ethFramesTxPri2.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_2))
	ec.na.m.ethFramesTxPri3.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_3))
	ec.na.m.ethFramesTxPri5.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_5))
	ec.na.m.ethFramesTxPri6.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_6))
	ec.na.m.ethFramesTxPri7.With(labels).Set(float64(ethtoolStats.FRAMES_TX_PRI_7))
	ec.na.m.ethFramesRxDropped.With(labels).Set(float64(ethtoolStats.FRAMES_RX_DROPPED))
	ec.na.m.ethFramesRxAll.With(labels).Set(float64(ethtoolStats.FRAMES_RX_ALL))
	ec.na.m.ethFramesRxBadAll.With(labels).Set(float64(ethtoolStats.FRAMES_RX_BAD_ALL))
	ec.na.m.ethFramesTxAll.With(labels).Set(float64(ethtoolStats.FRAMES_TX_ALL))
	ec.na.m.ethFramesTxBad.With(labels).Set(float64(ethtoolStats.FRAMES_TX_BAD))
	ec.na.m.ethHwTxDropped.With(labels).Set(float64(ethtoolStats.HW_TX_DROPPED))
	ec.na.m.ethHwRxDropped.With(labels).Set(float64(ethtoolStats.HW_RX_DROPPED))
	ec.na.m.ethRx0Dropped.With(labels).Set(float64(ethtoolStats.RX_0_DROPPED))
	ec.na.m.ethRx1Dropped.With(labels).Set(float64(ethtoolStats.RX_1_DROPPED))
	ec.na.m.ethRx2Dropped.With(labels).Set(float64(ethtoolStats.RX_2_DROPPED))
	ec.na.m.ethRx3Dropped.With(labels).Set(float64(ethtoolStats.RX_3_DROPPED))
	ec.na.m.ethRx4Dropped.With(labels).Set(float64(ethtoolStats.RX_4_DROPPED))
	ec.na.m.ethRx5Dropped.With(labels).Set(float64(ethtoolStats.RX_5_DROPPED))
	ec.na.m.ethRx6Dropped.With(labels).Set(float64(ethtoolStats.RX_6_DROPPED))
	ec.na.m.ethRx7Dropped.With(labels).Set(float64(ethtoolStats.RX_7_DROPPED))
	ec.na.m.ethRx8Dropped.With(labels).Set(float64(ethtoolStats.RX_8_DROPPED))
	ec.na.m.ethRx9Dropped.With(labels).Set(float64(ethtoolStats.RX_9_DROPPED))
	ec.na.m.ethRx10Dropped.With(labels).Set(float64(ethtoolStats.RX_10_DROPPED))
	ec.na.m.ethRx11Dropped.With(labels).Set(float64(ethtoolStats.RX_11_DROPPED))
	ec.na.m.ethRx12Dropped.With(labels).Set(float64(ethtoolStats.RX_12_DROPPED))
	ec.na.m.ethRx13Dropped.With(labels).Set(float64(ethtoolStats.RX_13_DROPPED))
	ec.na.m.ethRx14Dropped.With(labels).Set(float64(ethtoolStats.RX_14_DROPPED))
	ec.na.m.ethRx15Dropped.With(labels).Set(float64(ethtoolStats.RX_15_DROPPED))
	ec.na.m.ethFramesRxOk.With(labels).Set(float64(ethtoolStats.FRAMES_RX_OK))
	ec.na.m.ethFramesTxOk.With(labels).Set(float64(ethtoolStats.FRAMES_TX_OK))
	ec.na.m.ethOctetsRxOk.With(labels).Set(float64(ethtoolStats.OCTETS_RX_OK))
	ec.na.m.ethOctetsTxOk.With(labels).Set(float64(ethtoolStats.OCTETS_TX_OK))
	ec.na.m.ethOctetsTxTotal.With(labels).Set(float64(ethtoolStats.OCTETS_TX_TOTAL))
	ec.na.m.ethFramesRxUnicast.With(labels).Set(float64(ethtoolStats.FRAMES_RX_UNICAST))
	ec.na.m.ethFramesTxUnicast.With(labels).Set(float64(ethtoolStats.FRAMES_TX_UNICAST))
	ec.na.m.ethFramesRx8192b9215b.With(labels).Set(float64(ethtoolStats.FRAMES_RX_8192B_9215B))
	ec.na.m.ethFramesTx8192b9215b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_8192B_9215B))
	ec.na.m.ethFramesTx64b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_64B))
	ec.na.m.ethFramesTx65b127b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_65B_127B))
	ec.na.m.ethFramesTx128b255b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_128B_255B))
	ec.na.m.ethFramesTx256b511b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_256B_511B))
	ec.na.m.ethFramesTx512b1023b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_512B_1023B))
	ec.na.m.ethFramesTx1024b1518b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_1024B_1518B))
	ec.na.m.ethFramesTx1519b2047b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_1519B_2047B))
	ec.na.m.ethFramesTx2048b4095b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_2048B_4095B))
	ec.na.m.ethFramesTx4096b8191b.With(labels).Set(float64(ethtoolStats.FRAMES_TX_4096B_8191B))
	return nil
}
