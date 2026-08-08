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

package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// NodeHealthLabellerConfig holds the configuration for node labelling.
type NodeHealthLabellerConfig struct {
	LabelPrefix string
}

// NewNodeHealthLabellerConfig creates a new NodeLabellerConfig with the given label prefix.
func NewNodeHealthLabellerConfig(labelPrefix string) *NodeHealthLabellerConfig {
	return &NodeHealthLabellerConfig{
		LabelPrefix: labelPrefix,
	}
}

// ParseNodeHealthLabel converts k8s node labels to a device health map.
func (cfg *NodeHealthLabellerConfig) ParseNodeHealthLabel(nodeLabels map[string]string) map[string]string {
	healthMap := make(map[string]string)
	for key, value := range nodeLabels {
		if strings.HasPrefix(key, cfg.LabelPrefix) && strings.HasSuffix(key, ".state") { // node health label
			// Extract the device ID from the label key
			deviceID := cfg.extractDeviceID(key)
			if deviceID == "" {
				logger.Log.Printf("Failed to extract device ID from label: %s", key)
				continue // Skip if device ID cannot be extracted
			}
			healthMap[deviceID] = value
		}
	}

	return healthMap
}

// RemoveNodeHealthLabel deletes all node health labels from node labels.
func (cfg *NodeHealthLabellerConfig) RemoveNodeHealthLabel(nodeLabels map[string]string) {
	for key := range nodeLabels {
		if strings.HasPrefix(key, cfg.LabelPrefix) && strings.HasSuffix(key, ".state") {
			delete(nodeLabels, key)
		}
	}
}

// AddNodeHealthLabel adds all health labels to node labels from a map.
func (cfg *NodeHealthLabellerConfig) AddNodeHealthLabel(nodeLabels map[string]string, healthMap map[string]string) {
	for deviceID, state := range healthMap {
		if deviceID == "" || state == strings.ToLower(nicmetricssvc.Health_HEALTHY.String()) || state == strings.ToLower(metricssvc.GPUHealth_HEALTHY.String()) {
			continue
		}
		labelKey := fmt.Sprintf("%s.%s.state", cfg.LabelPrefix, deviceID)
		nodeLabels[labelKey] = state
	}
}

func (cfg *NodeHealthLabellerConfig) extractDeviceID(label string) string {
	escapedPrefix := regexp.QuoteMeta(cfg.LabelPrefix)
	// Pattern explanation:
	// ^escapedPrefix\.      => prefix + dot at start
	// (.+)                  => capture anything (deviceID)
	// \.state$              => literal ".state" at end of string
	pattern := fmt.Sprintf(`^%s\.(.+)\.state$`, escapedPrefix)
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(label)
	// matches[0] is the entire matched string
	// matches[1] is the first captured group
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
