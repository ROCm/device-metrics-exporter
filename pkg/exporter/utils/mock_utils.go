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

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
)

var (
	mockInbandErrorFilePath = "/mockdata/inband-ras/error_list"
)

// generateMockCPEREntry creates a new CPEREntry with dummy data for testing
func generateMockCPEREntry(afid uint64) *amdgpu.CPEREntry {
	return &amdgpu.CPEREntry{
		RecordId:         fmt.Sprintf("CPER-RECORD-%d", afid),
		Severity:         amdgpu.CPERSeverity_CPER_SEVERITY_FATAL,
		Revision:         1,
		Timestamp:        time.Now().UTC().Format("2006-01-02 15:04:05"),
		CreatorId:        "AMD-GPU-MOCK",
		NotificationType: amdgpu.CPERNotificationType_CPER_NOTIFICATION_TYPE_CMC,
		AFId:             []uint64{afid},
	}
}

func getMockCPERRecords(afids []int) []*amdgpu.GPUCPEREntry {
	output := make([]*amdgpu.GPUCPEREntry, 0)
	for _, afid := range afids {
		record := &amdgpu.GPUCPEREntry{
			GPU:       []byte("MOCK-GPU-UUID"),
			CPEREntry: make([]*amdgpu.CPEREntry, 0),
		}
		record.CPEREntry = append(record.CPEREntry, generateMockCPEREntry(uint64(afid)))
		output = append(output, record)
	}

	return output
}

func GetCperRecords() (*amdgpu.GPUCPERGetResponse, error) {
	_, err := os.Stat(mockInbandErrorFilePath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("mock error_list file does not exist")
	}
	data, err := os.ReadFile(mockInbandErrorFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read mock error_list file: %v", err)
	}
	var afids []int
	err = json.Unmarshal(data, &afids)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mock error_list file: %v", err)
	}
	cperResponse := &amdgpu.GPUCPERGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		CPER:      getMockCPERRecords(afids),
	}

	return cperResponse, nil
}
