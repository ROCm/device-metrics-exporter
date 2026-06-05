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

package config

import (
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

func TestGetGPUCperMaxAge(t *testing.T) {
	logger.Init(true)
	handler := NewConfigHandler("config.json", globals.GPUAgentPort)

	assert.Equal(t, time.Duration(0), handler.GetGPUCperMaxAge())

	cfg := handler.GetConfig()
	cfg.GPUConfig = &exportermetrics.GPUMetricConfig{
		HealthThresholds: &exportermetrics.GPUHealthThresholds{
			GPU_CPER_MAX_AGE: "1h",
		},
	}
	assert.Equal(t, time.Hour, handler.GetGPUCperMaxAge())

	cfg.GPUConfig.HealthThresholds.GPU_CPER_MAX_AGE = "not-a-duration"
	assert.Equal(t, time.Duration(0), handler.GetGPUCperMaxAge())
}
