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

	NICMetricsSocketPath = "/var/lib/amd-metrics-exporter/amdnic_device_metrics_exporter_grpc.socket"

	//PodResourceSocket - k8s pod grpc socket
	PodResourceSocket = "/var/lib/kubelet/pod-resources/kubelet.sock"

	// AMDGPUResourcePrefix - k8s AMD gpu resource prefix
	AMDGPUResourcePrefix = "amd.com/"

	// max number of custom labels that will be exported in the logs
	MaxSupportedCustomLabels = 10

	// max number of extra pod labels that will be exported in the logs
	MaxSupportedPodLabels = 20

	// amdgpuhealth tool log file name
	GPUHealthCheckerLogFile = "amdgpuhealth.log"

	// log directory path of amdgpuhealth tool
	GPUHealthCheckerLogDir = "/var/log/"

	// default exporter label that is applied
	DefaultExporterLabel = "app.kubernetes.io/name"

	// default value of exporter label
	DefaultExporterLabelValue = "metrics-exporter"

	// Metrics endpoint - returns user configured metrics in prometheus format
	MetricsHandlerPrefix = "/metrics"

	// Metrics endpoint - returns all static metrics in JSON format
	AMDGPUHandlerPrefix = "/gpumetrics"

	// Host directory where amdgpuhealth utility is copied to
	AMDGPUHealthHostDirPath = "/var/lib/amd-metrics-exporter"

	// Path of amdgpuhealth utility
	AMDGPUHealthContainerPath = "/home/amd/bin/amdgpuhealth"

	// GPUHealthLabelPrefix - prefix for GPU health labels
	GPUHealthLabelPrefix = "metricsexporter.amd.com.gpu"

	// NICHealthLabelPrefix - prefix for NIC health labels
	NICHealthLabelPrefix = "metricsexporter.amd.com.nic"

	// Kube RBAC Proxy port name
	KubeRBACProxyPortName = "exporter-port"

	// ExporterEndpointURLCachePath - path to the cache file that stores metrics endpoint URL in NPD
	ExporterEndpointURLCachePath = "/tmp/exporter_endpoint_cache.txt"

	// AMDGPUInbandRASHandlerPrefix - endpoint to get inband RAS errors in JSON format
	AMDGPUInbandRASHandlerPrefix = "/inbandraserrors"

	// LatestProcessedErrorTimestampFilePath - path to the file that stores the latest processed inband RAS error timestamp
	LatestProcessedErrorTimestampFilePath = "/tmp/latest_processed_inband_ras_error.txt"
)

type DeviceType string

const (
	GPUDevice DeviceType = "GPU"
	NICDevice DeviceType = "NIC"
)
