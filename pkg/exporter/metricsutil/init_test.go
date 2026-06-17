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
	"os"
	"path/filepath"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"gotest.tools/assert"
)

var (
	mh           *MetricsHandler
	chandler     *config.ConfigHandler
	tempDir      string
	confFilePath string
)

func setupTest(t *testing.T) func(t *testing.T) {
	t.Logf("============= TestSetup %v ===============", t.Name())

	t.Logf("LOGDIR %v", os.Getenv("LOGDIR"))

	logger.Init(true)

	tempDir, err := os.MkdirTemp("", "testdata")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %s", err)
	}
	newConf := &exportermetrics.MetricConfig{
		CommonConfig: &exportermetrics.CommonConfig{
			MetricsFieldPrefix: "amd",
		},
	}
	jsonData, err := json.MarshalIndent(newConf, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %s", err)
	}

	filePath := filepath.Join(tempDir, "config.json")
	// Write the JSON data to the file.
	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to write JSON to file: %s", err)
	}
	confFilePath = filePath

	chandler = config.NewConfigHandler(confFilePath, config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort})

	mh, err = NewMetrics(chandler)
	if err != nil {
		t.Fatalf("metrics handler create failed: %s", err)
	}
	mh.InitConfig(context.Background())

	t.Logf("setup completed")

	return func(t *testing.T) {
		t.Logf("============= Test:TearDown %v ===============", t.Name())
		os.RemoveAll(tempDir) // Clean up after the test.
	}
}

func UpdateConfFile(t *testing.T, newConf *exportermetrics.MetricConfig) {
	jsonData, err := json.MarshalIndent(newConf, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %s", err)
	}
	// Write the JSON data to the file.
	err = os.WriteFile(confFilePath, jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to write JSON to file: %s", err)
	}
	assert.Assert(t, chandler.RefreshConfig() == nil, "config update failed")
}
