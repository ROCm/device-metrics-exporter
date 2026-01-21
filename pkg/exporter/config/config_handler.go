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
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

var (
	defaultNICHealthCheckConfig = &exportermetrics.NICHealthCheckConfig{
		InterfaceAdminDownAsUnhealthy: false,
	}
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
	logger.Log.Printf("Running Config :%+v", configPath)
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

// GetHealthPollingInterval returns the health polling interval
// Default: 5 minutes, Min: 30 seconds, Max: 24 hours
func (c *ConfigHandler) GetHealthPollingInterval() time.Duration {
	c.Lock()
	defer c.Unlock()

	const (
		defaultInterval = 30 * time.Second
		minInterval     = 30 * time.Second
		maxInterval     = 24 * time.Hour
	)

	cfg := c.runningConfig.GetConfig()
	if cfg == nil || cfg.GetCommonConfig() == nil {
		return defaultInterval
	}

	healthCfg := cfg.GetCommonConfig().GetHealthService()
	if healthCfg == nil || healthCfg.GetPollingRate() == "" {
		return defaultInterval
	}

	intervalStr := healthCfg.GetPollingRate()
	duration, err := time.ParseDuration(intervalStr)
	if err != nil {
		logger.Log.Printf("Invalid PollingRate '%s': %v. Using default 30s", intervalStr, err)
		return defaultInterval
	}

	// Validate range
	if duration < minInterval {
		logger.Log.Printf("PollingRate %s is less than minimum 30s. Using 30s", duration)
		return minInterval
	}
	if duration > maxInterval {
		logger.Log.Printf("PollingRate %s exceeds maximum 24h. Using 24h", duration)
		return maxInterval
	}

	return duration
}

func (c *ConfigHandler) GetMetricsConfigPath() string {
	return c.configPath
}

func (c *ConfigHandler) GetAgentAddr() string {
	return fmt.Sprintf("%v:%v", globals.GPUAgentIP, c.grpcAgentPort)
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

func (c *ConfigHandler) GetProfilerConfig() *exportermetrics.ProfilerConfig {
	c.Lock()
	defer c.Unlock()
	cfg := c.runningConfig.GetConfig()
	if cfg != nil && cfg.GetGPUConfig() != nil {
		profilerCfg := cfg.GetGPUConfig().GetProfilerConfig()
		if profilerCfg != nil {
			samplingInterval := profilerCfg.GetSamplingInterval()
			ptlDelay := profilerCfg.GetPtlDelay()

			// Validate SamplingInterval is at least 1000
			if samplingInterval < 1000 {
				logger.Log.Printf("Invalid SamplingInterval value: %d. Must be at least 1000. Defaulting to 1000", samplingInterval)
				samplingInterval = 1000
			}

			return &exportermetrics.ProfilerConfig{
				SamplingInterval: samplingInterval,
				PtlDelay:         ptlDelay,
			}
		}
	}
	return &exportermetrics.ProfilerConfig{
		SamplingInterval: 1000, // default 1000 us (1 millisecond)
		PtlDelay:         0,    // default no delay
	}
}

func (c *ConfigHandler) GetLoggerConfig() *exportermetrics.LoggingConfig {
	c.Lock()
	defer c.Unlock()
	loggerConfig := logger.DefaultLogConfig()
	// set the rest of the fields from config
	cfg := c.runningConfig.GetConfig()
	if cfg != nil && cfg.GetCommonConfig() != nil {
		loggerSettings := cfg.GetCommonConfig().GetLogging()
		if loggerSettings != nil {
			// set log level
			if loggerSettings.GetLevel() != "" {
				level := loggerSettings.GetLevel()
				loggerConfig.Level = level
			}
			// set max file size
			if loggerSettings.GetMaxFileSizeMB() != 0 {
				loggerConfig.MaxFileSizeMB = loggerSettings.GetMaxFileSizeMB()
			}
			// set max backups
			if loggerSettings.GetMaxBackups() != 0 {
				loggerConfig.MaxBackups = loggerSettings.GetMaxBackups()
			}
			// set max age
			if loggerSettings.GetMaxAgeDays() != 0 {
				loggerConfig.MaxAgeDays = loggerSettings.GetMaxAgeDays()
			}
			// set log rotation disable flag
			loggerConfig.LogRotationDisable = loggerSettings.GetLogRotationDisable()
		}
	}
	return loggerConfig
}

// GetNICHealthCheckConfig returns the NIC health check settings
func (c *ConfigHandler) GetNICHealthCheckConfig() *exportermetrics.NICHealthCheckConfig {
	c.Lock()
	defer c.Unlock()
	cfg := c.runningConfig.GetConfig()
	if cfg != nil && cfg.GetNICConfig() != nil {
		nicHealthCheckSettings := cfg.GetNICConfig().GetHealthCheckConfig()
		if nicHealthCheckSettings != nil {
			return nicHealthCheckSettings
		}
	}
	return defaultNICHealthCheckConfig
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
