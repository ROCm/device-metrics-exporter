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
	"fmt"
	"os"
	"strconv"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
)

type Config struct {
	serverPort        uint32
	agentGRPCPort     int
	metricsConfigPath string
}

func NewConfig(mPath string) *Config {
	c := &Config{
		serverPort:        globals.AMDListenPort,
		metricsConfigPath: mPath,
	}
	logger.Log.Printf("Running Config :%+v", mPath)
	return c
}

func (c *Config) SetServerPort(port uint32) error {
	logger.Log.Printf("Server reconfigured from config file to %v", port)
	c.serverPort = port
	return nil
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

func (c *Config) GetAgentAddr() string {
	return fmt.Sprintf("0.0.0.0:%v", c.agentGRPCPort)
}

// SetAgentPort : set gpuagent pkg grpc port
func (c *Config) SetAgentPort(grpcPort int) {
	if grpcPort > 0 {
		c.agentGRPCPort = grpcPort
	} else {
		logger.Log.Printf("invalid grpcPort set %v, ignoring", grpcPort)
		c.agentGRPCPort = globals.GPUAgentPort
	}
}

func (c *Config) GetMetricsConfigPath() string {
	return c.metricsConfigPath
}
