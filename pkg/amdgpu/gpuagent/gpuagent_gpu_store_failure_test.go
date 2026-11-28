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

package gpuagent

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

// isGPUSignalField reports whether field is one of the two fields that must stay
// served while gpuagent is unreachable (see dropStaleGPUTelemetry).
func isGPUSignalField(field string) bool {
	return field == gpuHealthField || field == gpuNodesTotalField
}

// gpuStoreFields returns the sorted set of field names currently held in the
// store, across all device slots.
func gpuStoreFields() []string {
	set := map[string]struct{}{}
	for _, snap := range mh.GetStore().Snapshots() {
		snap.Range(func(field string, _ snapshotstore.Sample) bool {
			set[field] = struct{}{}
			return true
		})
	}
	return sortedKeys(set)
}

// gpuScrapedFields returns the sorted set of field names (prefix stripped) that a
// scrape of the GPU store collector serves, i.e. what /metrics would show.
func gpuScrapedFields(t *testing.T, gpu *GPUAgentGPUClient) []string {
	reg := prometheus.NewRegistry()
	assert.NilError(t, reg.Register(&gpuStoreCollector{ga: gpu}))
	fams, err := reg.Gather()
	assert.NilError(t, err)
	set := map[string]struct{}{}
	for _, mf := range fams {
		if len(mf.GetMetric()) == 0 {
			continue
		}
		set[strings.TrimPrefix(mf.GetName(), mh.GetPrefix())] = struct{}{}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func telemetryOnly(fields []string) []string {
	out := []string{}
	for _, f := range fields {
		if !isGPUSignalField(f) {
			out = append(out, f)
		}
	}
	return out
}

func signalOnly(fields []string) []string {
	out := []string{}
	for _, f := range fields {
		if isGPUSignalField(f) {
			out = append(out, f)
		}
	}
	return out
}

// TestGPUStoreDropsTelemetryOnFetchFailure reproduces the failure path of the
// pull-mode store writer: after a successful scrape, gpuagent becomes
// unreachable (GPUGet returns an error). The contract documented on
// fetchMetricsOnce / dropStaleGPUTelemetry is that /metrics then serves only
// gpu_nodes_total + gpu_health, and no last-known device telemetry, until the
// next successful fetch repopulates the full field set.
//
// The drop set must not depend on the GaugeVecs still holding rows: a
// ResetMetrics() before the fetch leaves them empty when the fetch fails.
func TestGPUStoreDropsTelemetryOnFetchFailure(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	ga := getNewAgent(t)
	defer ga.Close()
	assert.NilError(t, ga.InitConfigs())

	gpu := gpuClientFrom(t, ga)
	// Every scrape must reach GPUGet; a cached response would hide the failure.
	gpu.gpuGetCacheTTL = 0
	ctx := context.Background()

	// 1. Healthy scrape: telemetry and the signal fields are stored and served.
	assert.NilError(t, gpu.UpdateMetricsStats(ctx))
	storeBefore := gpuStoreFields()
	scrapeBefore := gpuScrapedFields(t, gpu)
	t.Logf("healthy: store has %d fields, scrape serves %d fields", len(storeBefore), len(scrapeBefore))
	assert.Assert(t, len(telemetryOnly(storeBefore)) > 0, "fixture stored no GPU telemetry; nothing to drop")
	assert.Assert(t, len(telemetryOnly(scrapeBefore)) > 0, "fixture served no GPU telemetry; nothing to drop")
	signalStoreBefore := signalOnly(storeBefore)
	signalScrapeBefore := signalOnly(scrapeBefore)
	assert.Assert(t, len(signalScrapeBefore) > 0, "fixture served neither gpu_health nor gpu_nodes_total")

	// 2. gpuagent goes away: GPUGet fails.
	healthyClient := gpu.gpuclient
	failing := mock_gen.NewMockGPUSvcClient(mockCtl)
	failing.EXPECT().GPUGet(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("gpuagent unreachable")).AnyTimes()
	failing.EXPECT().GPUCPERGet(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("gpuagent unreachable")).AnyTimes()
	gpu.gpuclient = failing

	err := gpu.UpdateMetricsStats(ctx)
	assert.Assert(t, err != nil, "scrape must report the GPUGet failure")

	storeAfter := gpuStoreFields()
	scrapeAfter := gpuScrapedFields(t, gpu)
	t.Logf("agent down: store fields %v", storeAfter)
	t.Logf("agent down: scraped fields %v", scrapeAfter)

	// No last-known telemetry may remain, neither in the store nor in the scrape.
	assert.DeepEqual(t, telemetryOnly(storeAfter), []string{})
	assert.DeepEqual(t, telemetryOnly(scrapeAfter), []string{})
	// The signal fields that were there before the outage must survive it.
	assert.DeepEqual(t, signalOnly(storeAfter), signalStoreBefore)
	assert.DeepEqual(t, signalOnly(scrapeAfter), signalScrapeBefore)

	// A second failed scrape must not change that.
	assert.Assert(t, gpu.UpdateMetricsStats(ctx) != nil)
	assert.DeepEqual(t, telemetryOnly(gpuStoreFields()), []string{})
	assert.DeepEqual(t, signalOnly(gpuScrapedFields(t, gpu)), signalScrapeBefore)

	// 3. gpuagent comes back: the full field set is served again.
	gpu.gpuclient = healthyClient
	assert.NilError(t, gpu.UpdateMetricsStats(ctx))
	assert.DeepEqual(t, gpuStoreFields(), storeBefore)
	assert.DeepEqual(t, gpuScrapedFields(t, gpu), scrapeBefore)
}
