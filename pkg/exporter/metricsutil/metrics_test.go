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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// recordingClient records the order UpdateMetrics drives the interface in.
type recordingClient struct {
	calls []string
}

func (c *recordingClient) UpdateStaticMetrics(_ context.Context) error { return nil }

func (c *recordingClient) UpdateMetricsStats(_ context.Context) error {
	c.calls = append(c.calls, "stats")
	return nil
}

func (c *recordingClient) InitConfigs() error { return nil }

func (c *recordingClient) StopServices() {}

func (c *recordingClient) ResetMetrics() error {
	c.calls = append(c.calls, "reset")
	return nil
}

func (c *recordingClient) QueryMetrics() (interface{}, error) { return nil, nil }

func (c *recordingClient) GetDeviceType() globals.DeviceType { return globals.GPUDevice }

func (c *recordingClient) QueryInbandRASErrors(_ string) (interface{}, error) { return nil, nil }

func (c *recordingClient) NeedsPerScrapePull() bool { return true }

// GetStore must return the store created in NewMetrics so downstream writers
// (GPU health, NIC) can reach it via the shared handle.
func TestGetStoreNonNilAfterNewMetrics(t *testing.T) {
	logger.Init(true)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	assert.NilError(t, os.WriteFile(cfgPath, []byte(`{}`), 0644))
	mh, err := NewMetrics(config.NewConfigHandler(cfgPath,
		config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort}))
	assert.NilError(t, err)

	if mh.GetStore() == nil {
		t.Fatal("GetStore() returned nil after NewMetrics")
	}
}

// A GaugeVec keeps children forever, so skipping the reset in high-resolution
// mode would serve label sets that no longer exist alongside the live ones.
func TestUpdateMetricsResetsInHighResolutionMode(t *testing.T) {
	// NewConfigHandler logs on entry; logger.Log is nil until Init.
	logger.Init(true)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	assert.NilError(t, os.WriteFile(cfgPath,
		[]byte(`{"CommonConfig":{"HighResolution":{"Enable":true}}}`), 0644))
	mh, err := NewMetrics(config.NewConfigHandler(cfgPath,
		config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort}))
	assert.NilError(t, err)

	client := &recordingClient{}
	mh.RegisterMetricsClient(client)
	mh.InitConfig(context.Background())
	assert.Equal(t, mh.IsHighResolution(), true)

	assert.NilError(t, mh.UpdateMetrics(context.Background()))
	assert.DeepEqual(t, client.calls, []string{"reset", "stats"})
}

func TestMetricsHandler(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	// expected test suite config default amd string
	prefix := mh.GetPrefix()
	assert.Equal(t, prefix, "amd", fmt.Sprintf("expected configured prefix amd but got %v", prefix))

	// udpate prefix to invalid prefix
	invalidPrefixList := []string{"amd-", "-amd"}
	for _, ipre := range invalidPrefixList {
		invalidPrefixConfig := &exportermetrics.MetricConfig{
			CommonConfig: &exportermetrics.CommonConfig{
				MetricsFieldPrefix: ipre,
			},
		}
		UpdateConfFile(t, invalidPrefixConfig)
		newPref := mh.GetPrefix()
		assert.Equal(t, newPref, "", fmt.Sprintf("expected empty prefix but got %v", newPref))
	}

	// Test with valid prefixes
	validPrefixList := []string{"amd", "gpu", "metrics"}
	for _, vpre := range validPrefixList {
		validPrefixConfig := &exportermetrics.MetricConfig{
			CommonConfig: &exportermetrics.CommonConfig{
				MetricsFieldPrefix: vpre,
			},
		}
		UpdateConfFile(t, validPrefixConfig)
		newPref := mh.GetPrefix()
		assert.Equal(t, newPref, vpre, fmt.Sprintf("expected prefix %v but got %v", vpre, newPref))
	}

	// Test with empty prefix
	emptyPrefixConfig := &exportermetrics.MetricConfig{
		CommonConfig: &exportermetrics.CommonConfig{
			MetricsFieldPrefix: "",
		},
	}
	UpdateConfFile(t, emptyPrefixConfig)
	newPref := mh.GetPrefix()
	assert.Equal(t, newPref, "", fmt.Sprintf("expected empty prefix but got %v", newPref))

	// Test with prefix containing only invalid characters
	invalidOnlyPrefixList := []string{"-", "--", "?", "#", "@"}
	for _, ipre := range invalidOnlyPrefixList {
		invalidPrefixConfig := &exportermetrics.MetricConfig{
			CommonConfig: &exportermetrics.CommonConfig{
				MetricsFieldPrefix: ipre,
			},
		}
		UpdateConfFile(t, invalidPrefixConfig)
		newPref := mh.GetPrefix()
		assert.Equal(t, newPref, "", fmt.Sprintf("expected empty prefix but got %v", newPref))
	}
}
