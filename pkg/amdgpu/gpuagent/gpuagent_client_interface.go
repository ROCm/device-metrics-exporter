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

package gpuagent

import (
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/prometheus/client_golang/prometheus"
)

type FieldMeta struct {
	Metric prometheus.GaugeVec
	Alias  string
}

type GPUAgentClientInterface interface {
	InitConfigs() error

	InitClients() error

	PopulateStaticHostLabels() error

	SetComputeNodeHealthState(bool)

	FetchPodLabelsForNode() (map[string]map[string]string, error)

	Close()

	metricsutil.MetricsInterface
}
