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
	"encoding/json"
	"io/ioutil"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/config"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/gpumetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/globals"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsHandler struct {
	reg          *prometheus.Registry
	runConf      *config.Config
	metricConfig *gpumetrics.MetricConfig
	clients      []MetricsInterface
}

func readConfig(c *config.Config) *gpumetrics.MetricConfig {
	var config_fields gpumetrics.MetricConfig
	pmConfigs := &config_fields
	mConfigs, err := ioutil.ReadFile(c.GetMetricsConfigPath())
	if err != nil {
		pmConfigs = nil
	} else {
		_ = json.Unmarshal(mConfigs, pmConfigs)
	}
	return pmConfigs

}

func NewMetrics(c *config.Config) (*MetricsHandler, error) {
	metricsHandler := MetricsHandler{
		runConf: c,
	}
	metricsHandler.clients = []MetricsInterface{}
	return &metricsHandler, nil
}

// GetRunConfig : returns the running config handle
func (mh *MetricsHandler) GetRunConfig() *config.Config {
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
	pmConfigs := readConfig(mh.runConf)
	mh.metricConfig = pmConfigs
	mh.updateServerPort()
	var wg sync.WaitGroup
	for _, client := range mh.clients {
		wg.Add(1)
		go func(client MetricsInterface) {
			defer wg.Done()
			client.InitConfigs()
			client.UpdateStaticMetrics()
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
			client.ResetMetrics()
			client.UpdateMetricsStats()
		}(client)
	}
	wg.Wait()
	return nil
}

func (mh *MetricsHandler) GetMetricsConfig() *gpumetrics.GPUMetricConfig {
	if mh.metricConfig != nil {
		return mh.metricConfig.GetGPUConfig()
	}
	return nil
}

func (mh *MetricsHandler) GetAgentAddr() string {
	return mh.runConf.GetAgentAddr()
}

func (mh *MetricsHandler) updateServerPort() {
	if mh.metricConfig != nil && mh.metricConfig.GetServerPort() != 0 {
		mh.runConf.SetServerPort(mh.metricConfig.GetServerPort())
	} else {
		mh.runConf.SetServerPort(globals.AMDListenPort)
	}
}
