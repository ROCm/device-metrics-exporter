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

// GPUAgentConfig holds GPU agent connection configuration
type GPUAgentConfig struct {
	GrpcPort   int
	UseSocket  bool
	SocketPath string
}

// ConfigHandler to update/read config data layer
type ConfigHandler struct {
	sync.Mutex
	// this doesn't change during the life cycle
	agentConfig GPUAgentConfig
	configPath  string
	// running config can change keep updating states
	runningConfig *Config
}

func NewConfigHandler(configPath string, agentConfig GPUAgentConfig) *ConfigHandler {
	logger.Log.Printf("Running Config :%+v", configPath)
	c := &ConfigHandler{
		configPath:    configPath,
		runningConfig: NewConfig(),
		agentConfig:   agentConfig,
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

// GetEnableAPI reports whether the debug APIs (pprof/expvar endpoints and the
// error-injection SetError gRPC API) are enabled. Default false.
func (c *ConfigHandler) GetEnableAPI() bool {
	c.Lock()
	defer c.Unlock()
	cfg := c.runningConfig.GetConfig()
	if cfg != nil && cfg.GetCommonConfig() != nil {
		debugCfg := cfg.GetCommonConfig().GetDebug()
		if debugCfg != nil {
			return debugCfg.GetEnableAPI()
		}
	}
	return false
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

// GetGPUCperMaxAge returns how long a fatal CPER record remains actionable for health.
// Zero disables the age filter (latest fatal CPER always marks the GPU unhealthy).
// Empty or unset config also disables the age filter to preserve legacy behavior.
func (c *ConfigHandler) GetGPUCperMaxAge() time.Duration {
	c.Lock()
	defer c.Unlock()

	cfg := c.runningConfig.GetConfig()
	if cfg == nil || cfg.GetGPUConfig() == nil {
		return 0
	}

	thresholds := cfg.GetGPUConfig().GetHealthThresholds()
	if thresholds == nil {
		return 0
	}

	ageStr := thresholds.GetGPU_CPER_MAX_AGE()
	if ageStr == "" {
		return 0
	}

	maxAge, err := time.ParseDuration(ageStr)
	if err != nil {
		logger.Errorf("Invalid GPU_CPER_MAX_AGE '%s': %v. Disabling age filter", ageStr, err)
		return 0
	}
	if maxAge < 0 {
		logger.Errorf("Invalid GPU_CPER_MAX_AGE '%s': must be >= 0. Disabling age filter", ageStr)
		return 0
	}
	return maxAge
}

func (c *ConfigHandler) GetMetricsConfigPath() string {
	return c.configPath
}

func (c *ConfigHandler) GetAgentAddr() string {
	if c.agentConfig.UseSocket {
		return fmt.Sprintf("unix://%s", c.agentConfig.SocketPath)
	}
	return fmt.Sprintf("%v:%v", globals.GPUAgentIP, c.agentConfig.GrpcPort)
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

// GetHighResolutionState reports whether the GPU-only high-resolution
// streaming profile is enabled. Default (unset) is false.
func (c *ConfigHandler) GetHighResolutionState() bool {
	c.Lock()
	defer c.Unlock()
	cfg := c.runningConfig.GetConfig()
	if cfg == nil || cfg.GetCommonConfig() == nil {
		return false
	}
	hr := cfg.GetCommonConfig().GetHighResolution()
	return hr != nil && hr.GetEnable()
}

// High-res watcher sampling-period defaults applied when config omits them.
// They mirror the gpuagent GPUWatch defaults (AGA_GPU_WATCH_*_PERIOD_DFLT);
// a zero fast period in particular makes rocpctl stream at max rate, so the
// default must be substituted here rather than passed through as 0.
const (
	highResDefaultFastPeriodSecs uint32 = 1
	highResDefaultSlowPeriodSecs uint32 = 5
	highResMinFastPeriodSecs     uint32 = 1
	highResMaxFastPeriodSecs     uint32 = 60
	highResMinSlowPeriodSecs     uint32 = 5
	highResMaxSlowPeriodSecs     uint32 = 3600
)

// GetHighResPeriods returns the fast/slow watcher sampling periods in seconds,
// substituting the high-res defaults for any period the config leaves unset.
func (c *ConfigHandler) GetHighResPeriods() (uint32, uint32) {
	c.Lock()
	defer c.Unlock()
	fast, slow := highResDefaultFastPeriodSecs, highResDefaultSlowPeriodSecs
	cfg := c.runningConfig.GetConfig()
	if cfg == nil || cfg.GetCommonConfig() == nil {
		return fast, slow
	}
	hr := cfg.GetCommonConfig().GetHighResolution()
	if hr == nil {
		return fast, slow
	}
	if v := hr.GetFastPeriodSecs(); v != 0 {
		fast = v
	}
	if v := hr.GetSlowPeriodSecs(); v != 0 {
		slow = v
	}
	// Enforce the documented ranges (fast 1-60, slow 5-3600, slow >= fast) so an
	// out-of-range config can't wedge GPUWatch/profiler in a permanent reconnect
	// loop; fall back to the defaults on any violation.
	if fast < highResMinFastPeriodSecs || fast > highResMaxFastPeriodSecs ||
		slow < highResMinSlowPeriodSecs || slow > highResMaxSlowPeriodSecs || slow < fast {
		logger.Log.Printf("invalid high-res periods (fast=%ds slow=%ds); require fast[%d-%d] slow[%d-%d] slow>=fast; using defaults %ds/%ds",
			fast, slow, highResMinFastPeriodSecs, highResMaxFastPeriodSecs,
			highResMinSlowPeriodSecs, highResMaxSlowPeriodSecs,
			highResDefaultFastPeriodSecs, highResDefaultSlowPeriodSecs)
		fast, slow = highResDefaultFastPeriodSecs, highResDefaultSlowPeriodSecs
	}
	return fast, slow
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
