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
	"os"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
)

const (
	MaxGPUPerServer     = 16 // current max is 8, gpuagent mock has 16
	NodeGPUHealthPrefix = "metricsexporter.amd.com.gpu.%v.state"
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

func IsKubernetes() bool {
	if s := os.Getenv("KUBERNETES_SERVICE_HOST"); s != "" {
		return true
	}
	if _, err := os.Stat(globals.PodResourceSocket); err == nil {
		return true
	}
	return false
}
