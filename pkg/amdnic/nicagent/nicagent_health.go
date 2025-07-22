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
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// GetNICHealthStates retrieves the health states of all NICs managed by the NIC agent.
// It returns a map where the keys are the PCIe IDs of the NICs and the values are their health states.
func (na *NICAgentClient) GetNICHealthStates() (map[string]interface{}, error) {
	if healthSettings := na.mh.GetNICHealthCheckConfig(); healthSettings != nil &&
		!healthSettings.InterfaceAdminDownAsUnhealthy {
		// If the health check settings specify that admin down interfaces should not be treated as unhealthy,
		// we skip the health check for admin down interfaces.
		return map[string]interface{}{}, nil
	}

	if len(na.nics) == 0 {
		logger.Log.Printf("No NICs found")
		return nil, nil
	}

	// ensure that there is nicctl; cannot get lif admin state without it
	if na.nicClients != nil {
		for _, client := range na.nicClients {
			if client.GetClientName() == NICCtlClientName && !client.IsActive() {
				logger.Log.Printf("nicctl client is not active")
				return nil, fmt.Errorf("nicctl client is not active")
			}
		}
	}

	nicHealthMap := make(map[string]interface{})
	for _, nic := range na.nics {
		for _, lif := range nic.Lifs {
			// PCIeAddress can be empty for lifs that are not configiured on the host yet
			if lif.PCIeAddress == "" || (lif.IsPF && nic.sriovConfiguredOnHost) {
				continue
			}

			nicState := &nicmetricssvc.NICState{}
			adminState, err := na.getAdminStatus(lif.UUID)
			if err != nil {
				logger.Log.Printf("failed to get admin state for LIF %s, err: %+v", lif.UUID, err)
				return nil, err
			}
			nicState.Device = lif.PCIeAddress
			nicState.UUID = lif.UUID
			switch adminState {
			case strings.ToLower(nicmetricssvc.AdminState_UP.String()):
				nicState.Health = strings.ToLower(nicmetricssvc.Health_HEALTHY.String())
			case strings.ToLower(nicmetricssvc.AdminState_DOWN.String()):
				nicState.Health = strings.ToLower(nicmetricssvc.Health_UNHEALTHY.String())
			}
			nicHealthMap[lif.PCIeAddress] = nicState
		}
	}

	if err := na.sendNodeLabelUpdate(nicHealthMap); err != nil {
		logger.Log.Printf("failed to send node label update for NIC health states, err: %+v", err)
	}
	return nicHealthMap, nil
}

// getAdminStatus retrieves the admin status of a LIF by its UUID.
func (na *NICAgentClient) getAdminStatus(lifUUID string) (string, error) {
	type Response struct {
		NIC []struct {
			ID  string `json:"id"`
			Lif []struct {
				Spec struct {
					ID         string `json:"id"`
					MACAddress string `json:"mac_address"`
					AdminState string `json:"admin_state"`
				} `json:"spec"`
			} `json:"lif"`
		} `json:"nic"`
	}

	lifOut, err := exec.Command("/bin/bash", "-c", fmt.Sprintf("nicctl show lif -l %s -j", lifUUID)).Output()
	if err != nil {
		logger.Log.Printf("failed to get lif statistics, err: %+v", err)
		return "", err
	}
	var resp Response
	err = json.Unmarshal(lifOut, &resp)
	if err != nil {
		logger.Log.Printf("error unmarshalling lif data: %v", err)
		return "", err
	}

	return resp.NIC[0].Lif[0].Spec.AdminState, nil
}
