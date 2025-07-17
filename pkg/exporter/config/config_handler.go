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
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// ConfigHandler to update/read config data layer
type ConfigHandler struct {
	sync.Mutex
	// this doesn't change during the life cycle
	grpcAgentPort int
	configPath    string
	// running config can change keep updating states
	runningConfig *Config
}

func NewConfigHandler(configPath string, port int) *ConfigHandler {
	logger.Log.Printf("Running Config :%+v, gpuagent port %v", configPath, port)
	c := &ConfigHandler{
		configPath:    configPath,
		runningConfig: NewConfig(),
		grpcAgentPort: port,
	}
	return c
}

func (c *ConfigHandler) RefreshConfig() error {
	c.Lock()
	defer c.Unlock()
	newConfig, err := readConfig(c.configPath)
	if err != nil {
		logger.Log.Printf("config read err: %v, reverting to defaults", err)
		return c.runningConfig.Update(nil)
	}
	return c.runningConfig.Update(newConfig)
}

// GetHealthServiceState returns the health service state
// if not set, it returns true
// if set, it returns the value
func (c *ConfigHandler) GetHealthServiceState() bool {
	c.Lock()
	defer c.Unlock()
	cfg := c.runningConfig.GetConfig()
	if cfg != nil && cfg.GetCommonConfig() != nil {
		healthCfg := cfg.GetCommonConfig().GetHealthService()
		if healthCfg != nil {
			return healthCfg.GetEnable()
		}
	}
	return true
}

func (c *ConfigHandler) GetMetricsConfigPath() string {
	return c.configPath
}

func (c *ConfigHandler) GetAgentAddr() string {
	return fmt.Sprintf("0.0.0.0:%v", c.grpcAgentPort)
}

func (c *ConfigHandler) GetConfig() *exportermetrics.MetricConfig {
	c.Lock()
	defer c.Unlock()
	return c.runningConfig.GetConfig()
}

func (c *ConfigHandler) GetServerPort() uint32 {
	c.Lock()
	defer c.Unlock()
	return c.runningConfig.GetServerPort()
}

func readConfig(filepath string) (*exportermetrics.MetricConfig, error) {
	var config_fields exportermetrics.MetricConfig
	pmConfigs := &config_fields
	mConfigs, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	} else {
		err = json.Unmarshal(mConfigs, pmConfigs)
		if err != nil {
			return nil, err
		}
	}
	return pmConfigs, nil
}
