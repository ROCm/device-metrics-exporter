/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nicagent

import (
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
)

const (
	// Port name label for port metrics
	LabelPortName = "port_name"
	// Port ID label for port metrics
	LabelPortID = "port_id"
	// RoCE Interface name label for RDMA  metrics
	LabelRdmaDevName = "rdma_dev_name"
	// Netdevice name label for RDMA  metrics
	LabelPcieBusId = "pcie_bus_id"
	// Lif name label for Lif metrics
	LabelEthIntfName = "eth_intf_name"
	// Intf alias label for Intf metrics
	LabelEthIntfAlias = "eth_intf_alias"
	// Queue-Pair ID for QP metrics
	LabelQPID = "qp_id"

	RDMAClientName                = "RDMA_Stats_Client"
	NICCtlClientName              = "NICCTL_Client"
	EthtoolClientName             = "Ethtool_Client"
	NICCtlBinary                  = "nicctl"
	RDMABinary                    = "rdma"
	EthtoolBinary                 = "ethtool"
	PodNetnsExecCmd               = "nsenter --net=/host/proc/%d/ns/net "
	ShowRdmaDevicesCmd            = "rdma link"
	ShowNetDeviceCmd              = "ip link show %s"
	EthToolCmd                    = "ethtool -S %s"
	GetPcieAddrFromRdmaDevCmd     = "cat /sys/class/infiniband/%s/device/uevent  | grep PCI_SLOT"
	CrioRuntimeSocket             = "/host/run/crio/crio.sock"
	ContainerdRuntimeSocket       = "/host/run/containerd/containerd.sock"
	GetPIDFromContainerRuntimeCmd = "crictl --runtime-endpoint unix://%s  inspect %s | jq .info.pid"
)

var (
	workloadLabels = []string{
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_POD.String()),
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_NAMESPACE.String()),
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_CONTAINER.String()),
	}
)
