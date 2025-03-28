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

import (
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsHandler struct {
	reg     *prometheus.Registry
	runConf *config.ConfigHandler
	clients []MetricsInterface
}

func NewMetrics(c *config.ConfigHandler) (*MetricsHandler, error) {
	metricsHandler := MetricsHandler{
		runConf: c,
	}
	metricsHandler.clients = []MetricsInterface{}
	return &metricsHandler, nil
}

// GetRunConfig : returns the running config handle
func (mh *MetricsHandler) GetRunConfig() *config.ConfigHandler {
	return mh.runConf
}

// GetRegistry : returns the registry handle
func (mh *MetricsHandler) GetRegistry() *prometheus.Registry {
	return mh.reg
}

func (mh *MetricsHandler) RegisterMetricsClient(client MetricsInterface) {
	mh.clients = append(mh.clients, client)
}

func (mh *MetricsHandler) InitConfig() {
	mh.reg = prometheus.NewRegistry()
	if err := mh.runConf.RefreshConfig(); err != nil {
		logger.Log.Printf("failed to refresh config: %v", err)
	}
	var wg sync.WaitGroup
	for _, client := range mh.clients {
		wg.Add(1)
		go func(client MetricsInterface) {
			defer wg.Done()
			if err := client.InitConfigs(); err != nil {
				logger.Log.Printf("failed to init configs: %v", err)
			}
			if err := client.UpdateStaticMetrics(); err != nil {
				logger.Log.Printf("failed to update static metrics: %v", err)
			}
		}(client)
	}
	wg.Wait()
}

// UpdateMetrics : send on demand update metrics request
func (mh *MetricsHandler) UpdateMetrics() error {
	var wg sync.WaitGroup
	for _, client := range mh.clients {
		wg.Add(1)
		go func(client MetricsInterface) {
			defer wg.Done()
			if err := client.ResetMetrics(); err != nil {
				logger.Log.Printf("failed to resetb metrics: %v", err)
			}
			if err := client.UpdateStaticMetrics(); err != nil {
				logger.Log.Printf("failed to update static metrics: %v", err)
			}
		}(client)
	}
	wg.Wait()
	return nil
}

func (mh *MetricsHandler) GetMetricsConfig() *exportermetrics.GPUMetricConfig {
	config := mh.runConf.GetConfig()
	if config != nil {
		return config.GetGPUConfig()
	}
	return nil
}

func (mh *MetricsHandler) GetAgentAddr() string {
	return mh.runConf.GetAgentAddr()
}
