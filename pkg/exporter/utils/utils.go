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
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

const (
	MaxGPUPerServer     = 16 // current max is 8, gpuagent mock has 16
	NodeGPUHealthPrefix = "metricsexporter.amd.com.gpu.%v.state"
	ServiceFile         = "/usr/lib/systemd/system/amd-metrics-exporter.service"
)

// ParseNodeHealthLabel - converts k8s nod label to gpu,health map
func ParseNodeHealthLabel(nodeLabels map[string]string) map[string]string {
	healthMap := make(map[string]string)
	for i := 0; i < MaxGPUPerServer; i++ {
		labelKey := fmt.Sprintf(NodeGPUHealthPrefix, i)
		gpuid := fmt.Sprintf("%v", i)
		if state, ok := nodeLabels[labelKey]; ok {
			healthMap[gpuid] = state
		}
	}
	return healthMap
}

// delete all node health labels from nodelabel
func RemoveNodeHealthLabel(nodeLabels map[string]string) {
	for i := 0; i < MaxGPUPerServer; i++ {
		labelKey := fmt.Sprintf(NodeGPUHealthPrefix, i)
		delete(nodeLabels, labelKey)
	}
}

// add all health labels to node label from map
func AddNodeHealthLabel(nodeLabels map[string]string, healthMap map[string]string) {
	for gpuid, state := range healthMap {
		if state == "healthy" {
			continue
		}
		labelKey := fmt.Sprintf(NodeGPUHealthPrefix, gpuid)
		nodeLabels[labelKey] = state
	}
}

func GetNodeName() string {
	if os.Getenv("DS_NODE_NAME") != "" {
		return os.Getenv("DS_NODE_NAME")
	}
	if os.Getenv("NODE_NAME") != "" {
		return os.Getenv("NODE_NAME")
	}
	return ""
}

func IsDebianInstall() bool {
	_, err := os.Stat(ServiceFile)
	return err == nil
}

func IsKubernetes() bool {
	if s := os.Getenv("KUBERNETES_SERVICE_HOST"); s != "" {
		return true
	}
	if IsDebianInstall() {
		return false
	}
	if _, err := os.Stat(globals.PodResourceSocket); err == nil {
		return true
	}
	return false
}

// GetPCIeBaseAddress extracts the base address (domain:bus:device) from a full PCIe address.
func GetPCIeBaseAddress(fullAddr string) string {
	parts := strings.Split(fullAddr, ".")
	if len(parts) == 2 {
		return parts[0]
	}
	return fullAddr // If malformed or no function, return as-is
}

func GetHostName() (string, error) {
	hostname := ""
	var err error
	if nodeName := GetNodeName(); nodeName != "" {
		hostname = nodeName
	} else {
		hostname, err = os.Hostname()
		if err != nil {
			return "", err
		}
	}
	return hostname, nil
}

// IsValueApplicable - return false if any of the value is all 0xf for max
//
//					       datatype size, this represents NA (not applicable from the metrics field)
//	                  - return true otherwise
func IsValueApplicable(x interface{}) bool {
	switch x := x.(type) {
	case uint64:
		if x == math.MaxUint64 || x == math.MaxUint32 || x == math.MaxUint16 || x == math.MaxUint8 {
			return false
		}
	case uint32:
		if x == math.MaxUint32 || x == math.MaxUint16 || x == math.MaxUint8 {
			return false
		}
	case uint16:
		if x == math.MaxUint16 || x == math.MaxUint8 {
			return false
		}
	case uint8:
		if x == math.MaxUint8 {
			return false
		}
	}
	return true

}

// NormalizeUint64 - return 0 if any of the value is of 0xf indication NA as
//
//	  per the max data size
//	- return x as is otherwise
func NormalizeUint64(x interface{}) float64 {
	switch x := x.(type) {
	case uint64:
		if x == math.MaxUint64 || x == math.MaxUint32 || x == math.MaxUint16 || x == math.MaxUint8 {
			return 0
		}
		return float64(x)
	case uint32:
		if x == math.MaxUint32 || x == math.MaxUint16 || x == math.MaxUint8 {
			return 0
		}
		return float64(x)
	case uint16:
		if x == math.MaxUint16 || x == math.MaxUint8 {
			return 0
		}
		return float64(x)
	case uint8:
		if x == math.MaxUint8 {
			return 0
		}
		return float64(x)
	}
	logger.Log.Fatalf("only uint64, uint32, uint16, uint8 are expected but got %v", reflect.TypeOf(x))
	return 0
}
