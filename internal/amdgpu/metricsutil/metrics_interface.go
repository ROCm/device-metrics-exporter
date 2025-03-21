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

package metricsutil

type MetricsInterface interface {
	// one time statistic pull for clients
	UpdateStaticMetrics() error

	// ondemand query request for client to update current stat
	UpdateMetricsStats() error

	// metric lable for interal usage within client
	GetExportLabels() []string

	// metrics registration must be done in this
	InitConfigs() error

	// reset metric states
	ResetMetrics() error
}

type MetricsClient interface {
	// client registration to the metric handler
	RegisterMetricsClient(MetricsInterface) error
}
