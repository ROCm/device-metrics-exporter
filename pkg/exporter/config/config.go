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
	"os"
	"strconv"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// Config - holds dynamic value changes to the config file
type Config struct {
	serverPort    uint32
	metricsConfig *exportermetrics.MetricConfig
}

func NewConfig() *Config {
	c := &Config{
		serverPort:    globals.AMDListenPort,
		metricsConfig: &exportermetrics.MetricConfig{},
	}
	return c
}

func (c *Config) Update(newConfig *exportermetrics.MetricConfig) error {
	c.serverPort = globals.AMDListenPort
	if newConfig != nil && newConfig.GetServerPort() != 0 {
		c.serverPort = newConfig.GetServerPort()
	}
	// reset to default
	c.metricsConfig = &exportermetrics.MetricConfig{}
	if newConfig != nil {
		c.metricsConfig = newConfig
	}
	return nil
}

func (c *Config) GetConfig() *exportermetrics.MetricConfig {
	return c.metricsConfig
}

func (c *Config) GetServerPort() uint32 {
	if os.Getenv("METRICS_EXPORTER_PORT") != "" {
		logger.Log.Printf("METRICS_EXPORTER_PORT env set, override serport")
		portStr := os.Getenv("METRICS_EXPORTER_PORT")
		number, err := strconv.Atoi(portStr)
		if err != nil {
			return c.serverPort
		}
		return uint32(number)
	}
	return c.serverPort
}
