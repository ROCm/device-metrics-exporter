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

package globals

const (
	// metrics exporter default server port
	AMDListenPort = 5000

	// metrics exporter configuraiton file path
	AMDMetricsFile = "/etc/metrics/config.json"

	// GPUAgent internal clien port
	GPUAgentPort = 50061

	ZmqPort = "6601"

	SlurmDir = "/var/run/exporter/"

	MetricsSocketPath = "/var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket"

	//PodResourceSocket - k8s pod grpc socket
	PodResourceSocket = "/var/lib/kubelet/pod-resources/kubelet.sock"

	// AMDGPUResourceLabel - k8s AMD gpu resource label
	AMDGPUResourceLabel = "amd.com/gpu"

	// max number of custom labels that will be exported in the logs
	MaxSupportedCustomLabels = 10
)
