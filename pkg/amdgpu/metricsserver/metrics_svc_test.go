/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package metricsserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"gotest.tools/assert"
)

// mockHealthClient is a minimal HealthInterface implementation that records
// whether SetError reached it.
type mockHealthClient struct {
	setErrorCalled bool
}

func (m *mockHealthClient) GetGPUHealthStates() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (m *mockHealthClient) SetError(gpuid string, fields []string, values []uint32) error {
	m.setErrorCalled = true
	return nil
}

// newTestConfigHandler builds a ConfigHandler backed by a temp config.json with
// the given Debug.EnableAPI value.
func newTestConfigHandler(t *testing.T, enableAPI bool) *config.ConfigHandler {
	t.Helper()
	logger.Init(false)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
		"CommonConfig": { "MetricsFieldPrefix": "amd_", "Debug": { "EnableAPI": false } }
	}`
	if enableAPI {
		body = `{
			"CommonConfig": { "MetricsFieldPrefix": "amd_", "Debug": { "EnableAPI": true } }
		}`
	}
	assert.NilError(t, os.WriteFile(path, []byte(body), 0644))
	ch := config.NewConfigHandler(path, config.GPUAgentConfig{})
	assert.NilError(t, ch.RefreshConfig())
	return ch
}

// TestSetErrorGatedByEnableAPI verifies the error-injection gRPC API is gated on
// CommonConfig.Debug.EnableAPI: rejected by default, only functional when true.
func TestSetErrorGatedByEnableAPI(t *testing.T) {
	req := &metricssvc.GPUErrorRequest{
		ID:     "test-gpu-0",
		Fields: []string{"GPU_ECC_UNCORRECT_GFX"},
		Counts: []uint32{1},
	}

	// EnableAPI=false (default): SetError must return an error and must not reach
	// the registered client.
	t.Run("disabled by default", func(t *testing.T) {
		client := &mockHealthClient{}
		srv := NewMetricsServer(newTestConfigHandler(t, false))
		assert.NilError(t, srv.RegisterHealthClient(client))

		resp, err := srv.SetError(context.Background(), req)
		assert.Assert(t, err != nil, "expected SetError to be rejected when EnableAPI is false")
		assert.Assert(t, resp == nil, "expected nil response when rejected")
		assert.Assert(t, !client.setErrorCalled, "client.SetError must not be invoked when disabled")
	})

	// nil config handler is treated as disabled.
	t.Run("nil config handler rejected", func(t *testing.T) {
		client := &mockHealthClient{}
		srv := NewMetricsServer(nil)
		assert.NilError(t, srv.RegisterHealthClient(client))

		_, err := srv.SetError(context.Background(), req)
		assert.Assert(t, err != nil, "expected SetError to be rejected when config handler is nil")
		assert.Assert(t, !client.setErrorCalled, "client.SetError must not be invoked with nil config")
	})

	// EnableAPI=true: SetError succeeds and reaches the client.
	t.Run("enabled when EnableAPI true", func(t *testing.T) {
		client := &mockHealthClient{}
		srv := NewMetricsServer(newTestConfigHandler(t, true))
		assert.NilError(t, srv.RegisterHealthClient(client))

		resp, err := srv.SetError(context.Background(), req)
		assert.NilError(t, err)
		assert.Assert(t, resp != nil, "expected non-nil response when enabled")
		assert.Equal(t, resp.ID, req.ID)
		assert.Assert(t, client.setErrorCalled, "client.SetError must be invoked when enabled")
	})
}
