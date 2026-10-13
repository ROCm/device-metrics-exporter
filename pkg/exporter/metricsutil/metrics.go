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
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsHandler struct {
	reg     *prometheus.Registry
	runConf *config.ConfigHandler
	clients []MetricsInterface
	// store is the single source of truth for device samples; writers reach it
	// via GetStore() without touching device-client structs.
	store *snapshotstore.Store
}

func NewMetrics(c *config.ConfigHandler) (*MetricsHandler, error) {
	metricsHandler := MetricsHandler{
		runConf: c,
		store:   snapshotstore.New(),
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

// GetStore returns the shared snapshot store handle.
func (mh *MetricsHandler) GetStore() *snapshotstore.Store {
	return mh.store
}

func (mh *MetricsHandler) wrapRegister(r prometheus.Registerer) prometheus.Registerer {
	return prometheus.WrapRegistererWithPrefix(
		mh.GetPrefix(), r)
}

func (mh *MetricsHandler) RegisterMetric(metric prometheus.Collector) error {
	return mh.wrapRegister(mh.reg).Register(metric)
}

func (mh *MetricsHandler) RegisterMetricsClient(client MetricsInterface) {
	mh.clients = append(mh.clients, client)
}

func (mh *MetricsHandler) InitConfig(ctx context.Context) {
	// Stop every client's long-running services (high-res GPUWatch feed, profiler
	// stream) and wait for their goroutines to exit BEFORE touching the registry,
	// store, or config. A reload rebuilds config-derived label/field maps and the
	// store device keys; doing that while a feed is still writing races those maps
	// and leaves stale slots. Services are restarted at the end of InitConfigs once
	// the new config and a fresh baseline read are in place.
	for _, client := range mh.clients {
		client.StopServices()
	}
	mh.reg = prometheus.NewRegistry()
	// Clear stale device slots on (re)load: a changed label set (e.g. enabling
	// kfd_process_id or a custom label) alters the store device key, so the old
	// slot would otherwise orphan and collide with the freshly-written one on
	// render. Writers repopulate on the next scrape / feed tick.
	mh.store.Reset()
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
			if err := client.UpdateStaticMetrics(ctx); err != nil {
				logger.Log.Printf("failed to update static metrics: %v", err)
			}
		}(client)
	}
	wg.Wait()
}

// UpdateMetrics : send on demand update metrics request
func (mh *MetricsHandler) UpdateMetrics(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, client := range mh.clients {
		wg.Add(1)
		go func(client MetricsInterface) {
			defer wg.Done()
			// Reset before refill in every mode, high-resolution included: a
			// GaugeVec keeps children forever, so without it a label set that
			// disappears (partition change, pod relabel) is served stale
			// alongside the live one. UpdateMetricsStats below repopulates
			// every field from one read, so the scrape stays a single
			// self-consistent snapshot.
			if err := client.ResetMetrics(); err != nil {
				logger.Log.Printf("failed to reset metrics: %v", err)
			}
			if err := client.UpdateMetricsStats(ctx); err != nil {
				logger.Log.Printf("failed to update metrics: %v", err)
			}
		}(client)
	}
	wg.Wait()
	return nil
}

// AnyClientNeedsPerScrapePull reports whether any registered client still serves
// /metrics via the in-place reset+refill pull path, which requires the scrape
// mutex. False only when every registered client serves from the snapshot store
// (GPU store mode, non-high-res), enabling lock-free concurrent scrapes. Returns
// true when no clients are registered so the mutex path (today's behavior) holds.
func (mh *MetricsHandler) AnyClientNeedsPerScrapePull() bool {
	for _, client := range mh.clients {
		if client.NeedsPerScrapePull() {
			return true
		}
	}
	return len(mh.clients) == 0
}

// RefreshStoreServingClients refreshes the snapshot store for every store-serving
// client (NeedsPerScrapePull()==false) via its own singleflight fetch. No GaugeVec
// reset and no scrape mutex: concurrent scrapes dedupe to one hardware pull and the
// store collector reads lock-free. Called only when AnyClientNeedsPerScrapePull()
// is false, so every registered client is store-serving.
func (mh *MetricsHandler) RefreshStoreServingClients(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, client := range mh.clients {
		if client.NeedsPerScrapePull() {
			continue
		}
		wg.Add(1)
		go func(client MetricsInterface) {
			defer wg.Done()
			if err := client.UpdateMetricsStats(ctx); err != nil {
				logger.Log.Printf("store refresh failed: %v", err)
			}
		}(client)
	}
	wg.Wait()
	return nil
}

func (mh *MetricsHandler) HandleGPUMetricsQuery(w http.ResponseWriter, req *http.Request) {
	var gpuResponse *amdgpu.GPUGetResponse
	var resp interface{}
	var err error
	for _, client := range mh.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			resp, err = client.QueryMetrics()
			break
		}
	}
	if err != nil {
		http.Error(w, "An error occured while querying metrics:\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	gpuResponse, ok := resp.(*amdgpu.GPUGetResponse)
	if ok && gpuResponse != nil {
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(gpuResponse)
		if err == nil {
			return
		}
	}
	http.Error(w, "An error occured while querying metrics:\n"+err.Error(), http.StatusInternalServerError)
}

func (mh *MetricsHandler) HandleInbandRASErrorsQuery(w http.ResponseWriter, req *http.Request) {
	var cperResponse *amdgpu.GPUCPERGetResponse
	var resp interface{}
	var err error
	for _, client := range mh.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			severity := req.URL.Query().Get("severity")
			resp, err = client.QueryInbandRASErrors(severity)
			break
		}
	}
	if err != nil {
		http.Error(w, "an error occured while querying inband ras errors:\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	cperResponse, ok := resp.(*amdgpu.GPUCPERGetResponse)
	if ok && cperResponse != nil {
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(cperResponse)
		if err == nil {
			return
		}
	}
	http.Error(w, "an error occured while querying inband ras errors:\n"+err.Error(), http.StatusInternalServerError)
}

func (mh *MetricsHandler) GetMetricsConfig() *exportermetrics.MetricConfig {
	config := mh.runConf.GetConfig()
	if config != nil {
		return config
	}
	return nil
}

// IsHighResolution reports whether the GPU-only high-resolution profile is on.
func (mh *MetricsHandler) IsHighResolution() bool {
	return mh.runConf.GetHighResolutionState()
}

// GetHighResPeriods returns the fast/slow watcher sampling periods in seconds.
// The config handler substitutes the high-res defaults for any unset period, so
// both values are always non-zero.
func (mh *MetricsHandler) GetHighResPeriods() (uint32, uint32) {
	return mh.runConf.GetHighResPeriods()
}

func (mh *MetricsHandler) GetGPUMetricsConfig() *exportermetrics.GPUMetricConfig {
	config := mh.runConf.GetConfig()
	if config != nil {
		return config.GetGPUConfig()
	}
	return nil
}

func (mh *MetricsHandler) GetIFOEMetricsConfig() *exportermetrics.IFOEMetricConfig {
	config := mh.runConf.GetConfig()
	if config != nil {
		return config.GetIFOEConfig()
	}
	return nil
}

func (mh *MetricsHandler) GetNICMetricsConfig() *exportermetrics.NICMetricConfig {
	config := mh.runConf.GetConfig()
	if config != nil {
		return config.GetNICConfig()
	}
	return nil
}

// GetHealthServiceState : returns the health service state
// true : health service is enabled
// false : health service is disabled
func (mh *MetricsHandler) GetHealthServiceState() bool {
	return mh.runConf.GetHealthServiceState()
}

// AnyExtraPodLabelsConfigured returns true if any of GPU, NIC, or IFOE config
// has ExtraPodLabels set, meaning the k8s pod watcher is needed.
func (mh *MetricsHandler) AnyExtraPodLabelsConfigured() bool {
	if len(mh.GetGPUMetricsConfig().GetExtraPodLabels()) > 0 {
		return true
	}
	if len(mh.GetNICMetricsConfig().GetExtraPodLabels()) > 0 {
		return true
	}
	if len(mh.GetIFOEMetricsConfig().GetExtraPodLabels()) > 0 {
		return true
	}
	return false
}

// GetHealthPollingInterval returns the health polling interval
func (mh *MetricsHandler) GetHealthPollingInterval() time.Duration {
	return mh.runConf.GetHealthPollingInterval()
}

func (mh *MetricsHandler) GetAgentAddr() string {
	return mh.runConf.GetAgentAddr()
}

func (mh *MetricsHandler) GetPrefix() string {
	config := mh.runConf.GetConfig()
	if config == nil || config.GetCommonConfig() == nil {
		return ""
	}

	if config.GetCommonConfig().GetMetricsFieldPrefix() == "" {
		return ""
	}

	// validate prometheus accepted pattern
	prometheusFieldPattern := `^[a-zA-Z_][a-zA-Z0-9_]*$`
	configPrefix := config.GetCommonConfig().GetMetricsFieldPrefix()
	re := regexp.MustCompile(prometheusFieldPattern)
	if re.MatchString(configPrefix) {
		return configPrefix
	}
	// invalid prefix
	logger.Log.Printf("invalid prefix configured %v", configPrefix)
	logger.Log.Printf("accepted pattern should match %v", prometheusFieldPattern)
	logger.Log.Printf("defaulting to no prefix behavior")
	return ""
}

func (mh *MetricsHandler) GetNICHealthCheckConfig() *exportermetrics.NICHealthCheckConfig {
	return mh.runConf.GetNICHealthCheckConfig()
}

func (mh *MetricsHandler) GetProfilerConfig() *exportermetrics.ProfilerConfig {
	return mh.runConf.GetProfilerConfig()
}
