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

import "github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"

type NICInterface interface {
	// fill AINIC stats
	UpdateNICStats(map[string]scheduler.Workload) error
	// Initiate connection and return connection status
	Init() error
	// Return NIC Client name implementing this interface
	GetClientName() string
	// Returns true if the respective client binary is found, false otherwise
	IsActive() bool
}

// NIC represents the card data
// NICToPort: currently it's a 1:1 mapping but this has to be changed if the ports are getting breaked down further
type NIC struct {
	Index                 string
	UUID                  string           `json:"id"`
	ProductName           string           `json:"product_name"`
	SerialNumber          string           `json:"serial_number"`
	EthBDF                string           `json:"eth_bdf"`
	Ports                 map[string]*Port // NIC ports by Port ID
	Lifs                  map[string]*Lif  // NIC lifs by Lif ID
	sriovConfiguredOnHost bool
}

// Port represents the network port data
type Port struct {
	Index      string
	UUID       string `json:"id"`
	Name       string `json:"name"`
	MACAddress string
}

// LIf represents the logical interface data
type Lif struct {
	Index       string
	UUID        string
	Name        string
	PCIeAddress string
	IsPF        bool
}

type NetDevice struct {
	IntfName    string // Intf name in Pod ns
	IntfAlias   string // Intf name in Host ns
	RoceDevName string // RoCE dev linked to Intf
	PCIeBusId   string // Pcie Bus ID of network device
	PodName     string // PID of namespace which contains the Intf
}

// GetPortName returns the name of the first port associated with the NIC.
// This is a simplified method assuming a 1:1 mapping between NIC and Port.
func (n *NIC) GetPortName() string {
	for _, port := range n.Ports {
		return port.Name
	}
	return ""
}

// GetPortIndex returns the index of the first port associated with the NIC.
func (n *NIC) GetPortIndex() string {
	for _, port := range n.Ports {
		return port.Index
	}
	return ""
}

// GetPortPcieAddr returns the pcie bus id of the Port associated with the given NIC.
// When multi-port NIC support is added later , all 3 port fns in this file needs
// to be modified to take port UUID as input param.
func (n *NIC) GetPortPcieAddr() string {
	return n.EthBDF
}

// GetLifName returns the name of the lif associated with the given UUID.
func (n *NIC) GetLifName(uuid string) string {
	if lif, ok := n.Lifs[uuid]; ok {
		return lif.Name
	}
	return ""
}

// GetLifPcieAddr returns the pcie bus id of the lif associated with the given UUID.
func (n *NIC) GetLifPcieAddr(uuid string) string {
	if lif, ok := n.Lifs[uuid]; ok {
		return lif.PCIeAddress
	}
	return ""
}
