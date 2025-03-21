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
	"log"
	"os/exec"
	"regexp"
	"strings"
)

type TestUtils struct {
}

func New() *TestUtils {
	return &TestUtils{}
}

// LocalCommandOutput runs a command on a node and returns output in string format
func (tu *TestUtils) LocalCommandOutput(command string) string {
	out, err := exec.Command("bash", "-c", command).CombinedOutput()
	if err != nil {
		log.Printf("local command out err %+v", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

type MetricData struct {
	Labels map[string]string
}

func (m *MetricData) String() string {
	return fmt.Sprintf("labels : %+v", m.Labels)
}

type GPUMetric struct {
	Fields map[string]MetricData
}

func (m *GPUMetric) String() string {
	return fmt.Sprintf("field: %+v", m.Fields)
}

func parseKeyValueStrings(kvStr string) (map[string]string, error) {
	kvMap := make(map[string]string)

	pairs := strings.Split(kvStr, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			return kvMap, fmt.Errorf("invalid string, expecting format key=value,key1=value2")
		}

		kv := strings.Split(pair, "=")
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			kvMap[key] = value
		}
	}

	return kvMap, nil
}

func ParsePrometheusMetrics(payload string) (map[string]*GPUMetric, error) {
	metrics := make(map[string]*GPUMetric)

	// example : `http_requests_total{method="GET", status="200"} 123`
	// gpu_gfx_activity has exponential value which is not included to be
	// revisited
	re := regexp.MustCompile(`^(\w+)\{([^}]+)\}\s(\d+)$`)

	metricLines := strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n")
	for _, metricLine := range metricLines {
		matches := re.FindStringSubmatch(metricLine)
		if len(matches) != 4 {
			// ignore the non metric lines
			continue
		}
		metricName := matches[1]
		labels, err := parseKeyValueStrings(matches[2])
		if err != nil {
			return metrics, err
		}
		// filter only exporter metrics, with labels
		gpu_id, ok := labels["gpu_id"]
		if !ok {
			continue
		}
		if _, ok := metrics[gpu_id]; !ok {
			metrics[gpu_id] = &GPUMetric{
				Fields: make(map[string]MetricData),
			}
		}
		metric := metrics[gpu_id]
		metric.Fields[metricName] = MetricData{
			Labels: labels,
		}
		// ignoring value for mocked env
		//value := matches[3]
	}

	//log.Printf("metrics : %+v", metrics)

	if len(metrics) == 0 {
		return metrics, fmt.Errorf("payload invalid")
	}

	return metrics, nil
}
