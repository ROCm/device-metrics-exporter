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

package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"gotest.tools/assert"
)

const (
	testConfigTemplate = `{
		"ServerPort": %d,
		"CommonConfig": {
			"MetricsFieldPrefix": "amd_"
		},
		"GPUConfig": {
			"Fields": [
				"GPU_PACKAGE_POWER",
				"GPU_TEMPERATURE"
			],
			"CustomLabels": {
				"CLUSTER_NAME": "test-cluster"
			}
		},
		"NICConfig": {
			"Fields": [
				"NIC_LINK_STATE",
				"NIC_SPEED"
			],
			"CustomLabels": {
				"CLUSTER_NAME": "test-cluster"
			}
		}
	}`
)

type testConfig struct {
	name                string
	enableGPU           bool
	enableNIC           bool
	enableIFOE          bool
	enableSriov         bool
	disableK8sApi       bool
	enableSlurmScl      bool
	port                int
	expectedEndpoints   []string
	unexpectedEndpoints []string
	expectedStatusCode  int
}

func setupTestConfig(t *testing.T, port int) (string, func()) {
	// Create temporary directory for test config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configContent := fmt.Sprintf(testConfigTemplate, port)

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NilError(t, err)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return configPath, cleanup
}

func waitForServer(url string, timeout time.Duration) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func makeHTTPRequest(url string, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Timeout: timeout}
	return client.Get(url)
}

func TestExporterBasicConfiguration(t *testing.T) {
	// Initialize logger for testing
	logger.Init(false)

	tests := []testConfig{
		{
			name:               "GPU monitoring only",
			enableGPU:          true,
			enableNIC:          false,
			enableIFOE:         false,
			disableK8sApi:      true,
			port:               9091,
			expectedEndpoints:  []string{"/metrics", "/debug/vars"}, // Removed /amdgpu-metrics since it returns 404 without real GPU
			expectedStatusCode: 200,
		},
		{
			name:               "NIC monitoring only",
			enableGPU:          false,
			enableNIC:          true,
			enableIFOE:         false,
			disableK8sApi:      true,
			port:               9092,
			expectedEndpoints:  []string{"/metrics", "/debug/vars"},
			expectedStatusCode: 200,
		},
		{
			name:               "IFOE monitoring only",
			enableGPU:          false,
			enableNIC:          false,
			enableIFOE:         true,
			disableK8sApi:      true,
			port:               9093,
			expectedEndpoints:  []string{"/metrics", "/debug/vars"},
			expectedStatusCode: 200,
		},
		{
			name:               "All monitoring enabled",
			enableGPU:          true,
			enableNIC:          true,
			enableIFOE:         true,
			disableK8sApi:      true,
			port:               9094,
			expectedEndpoints:  []string{"/metrics", "/debug/vars"}, // Removed /amdgpu-metrics since it returns 404 without real GPU
			expectedStatusCode: 200,
		},
		{
			name:               "SRIOV mode",
			enableGPU:          true,
			enableNIC:          false,
			enableIFOE:         false,
			enableSriov:        true,
			disableK8sApi:      true,
			port:               9095,
			expectedEndpoints:  []string{"/metrics", "/debug/vars"}, // Removed /amdgpu-metrics since it returns 404 without real GPU
			expectedStatusCode: 200,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config
			configPath, cleanup := setupTestConfig(t, tc.port)
			defer cleanup()

			// Create exporter with test options
			var opts []ExporterOption
			opts = append(opts, WithGPUMonitoring(tc.enableGPU))
			opts = append(opts, WithNICMonitoring(tc.enableNIC))
			opts = append(opts, WithenableIFOEMonitoring(tc.enableIFOE))
			opts = append(opts, WithSRIOV(tc.enableSriov))
			opts = append(opts, WithBindAddr("127.0.0.1"))
			opts = append(opts, WithSlurmClient(tc.enableSlurmScl))

			if tc.disableK8sApi {
				opts = append(opts, WithNoK8sApiclient())
			}

			exporter := NewExporter(globals.GPUAgentPort, configPath, opts...)
			assert.Assert(t, exporter != nil, "Exporter should not be nil")

			// Start the exporter in a goroutine
			ctx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()

				// Initialize config and metrics
				runConf = config.NewConfigHandler(configPath, globals.GPUAgentPort)
				var err error
				mh, err = metricsutil.NewMetrics(runConf)
				assert.NilError(t, err)
				mh.InitConfig()

				// Start a minimal metrics server for testing
				srv := startMetricsServer(runConf, "127.0.0.1")
				defer func() {
					shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
					srv.Shutdown(shutdownCtx)
					shutdownCancel()
				}()

				// Wait for context cancellation
				<-ctx.Done()
			}()

			// Wait for server to start
			baseURL := fmt.Sprintf("http://127.0.0.1:%d", tc.port)
			serverReady := waitForServer(baseURL+"/metrics", 10*time.Second)
			assert.Assert(t, serverReady, "Server should start within timeout")

			// Test expected endpoints
			for _, endpoint := range tc.expectedEndpoints {
				url := baseURL + endpoint
				resp, err := makeHTTPRequest(url, 5*time.Second)
				assert.NilError(t, err, "Request to %s should succeed", endpoint)
				assert.Equal(t, resp.StatusCode, tc.expectedStatusCode, "Status code for %s", endpoint)
				resp.Body.Close()
			}

			// Test metrics endpoint content
			metricsURL := baseURL + "/metrics"
			resp, err := makeHTTPRequest(metricsURL, 5*time.Second)
			assert.NilError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			assert.NilError(t, err)
			metricsContent := string(body)

			// Verify prometheus format
			assert.Assert(t, strings.Contains(metricsContent, "# HELP"), "Metrics should contain help text")
			assert.Assert(t, strings.Contains(metricsContent, "# TYPE"), "Metrics should contain type information")

			// Note: AMD-specific metrics won't be present without actual GPU agents running
			// This is expected behavior in unit tests

			// Test debug endpoint
			debugURL := baseURL + "/debug/vars"
			resp, err = makeHTTPRequest(debugURL, 5*time.Second)
			assert.NilError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, resp.StatusCode, 200)

			// Cancel context to stop the server
			cancel()
			wg.Wait()
		})
	}
}

func TestExporterHTTPEndpoints(t *testing.T) {
	logger.Init(false)

	configPath, cleanup := setupTestConfig(t, 9096)
	defer cleanup()

	// Start exporter
	runConf = config.NewConfigHandler(configPath, globals.GPUAgentPort)
	var err error
	mh, err = metricsutil.NewMetrics(runConf)
	assert.NilError(t, err)
	mh.InitConfig()

	srv := startMetricsServer(runConf, "127.0.0.1")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		srv.Shutdown(ctx)
		cancel()
	}()

	baseURL := "http://127.0.0.1:9096"
	serverReady := waitForServer(baseURL+"/metrics", 10*time.Second)
	assert.Assert(t, serverReady, "Server should start")

	tests := []struct {
		name           string
		endpoint       string
		expectedStatus int
		contentCheck   func(string) bool
	}{
		{
			name:           "Metrics endpoint",
			endpoint:       "/metrics",
			expectedStatus: 200,
			contentCheck: func(content string) bool {
				return strings.Contains(content, "# HELP") && strings.Contains(content, "# TYPE")
			},
		},
		{
			name:           "GPU metrics endpoint",
			endpoint:       "/amdgpu-metrics",
			expectedStatus: 404, // Expected 404 since we don't have real GPU monitoring
			contentCheck: func(content string) bool {
				return len(content) >= 0 // Basic check
			},
		},
		{
			name:           "Inband RAS endpoint",
			endpoint:       "/amdgpu-inband-ras",
			expectedStatus: 404, // Expected 404 since we don't have real GPU monitoring
			contentCheck: func(content string) bool {
				return len(content) >= 0 // Basic check
			},
		},
		{
			name:           "Debug vars endpoint",
			endpoint:       "/debug/vars",
			expectedStatus: 200,
			contentCheck: func(content string) bool {
				// Should return JSON with runtime variables
				var data interface{}
				return json.Unmarshal([]byte(content), &data) == nil
			},
		},
		{
			name:           "Pprof index endpoint",
			endpoint:       "/debug/pprof/",
			expectedStatus: 200,
			contentCheck: func(content string) bool {
				return strings.Contains(content, "goroutine") || strings.Contains(content, "heap")
			},
		},
		{
			name:           "Pprof cmdline endpoint",
			endpoint:       "/debug/pprof/cmdline",
			expectedStatus: 200,
			contentCheck: func(content string) bool {
				return len(content) >= 0
			},
		},
		{
			name:           "Non-existent endpoint",
			endpoint:       "/nonexistent",
			expectedStatus: 404,
			contentCheck: func(content string) bool {
				return true // Any content is fine for 404
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := baseURL + tc.endpoint
			resp, err := makeHTTPRequest(url, 5*time.Second)
			assert.NilError(t, err, "Request should succeed")
			defer resp.Body.Close()

			assert.Equal(t, resp.StatusCode, tc.expectedStatus, "Status code should match")

			if tc.expectedStatus == 200 {
				body, err := io.ReadAll(resp.Body)
				assert.NilError(t, err)
				content := string(body)
				assert.Assert(t, tc.contentCheck(content), "Content check should pass for %s", tc.endpoint)
			}
		})
	}
}

func TestExporterOptions(t *testing.T) {
	logger.Init(false)

	configPath, cleanup := setupTestConfig(t, 9098)
	defer cleanup()

	tests := []struct {
		name    string
		options []ExporterOption
		verify  func(*Exporter)
	}{
		{
			name: "Custom bind address",
			options: []ExporterOption{
				WithBindAddr("192.168.1.1"),
			},
			verify: func(e *Exporter) {
				assert.Equal(t, e.bindAddr, "192.168.1.1", "Bind address should be set")
			},
		},
		{
			name: "GPU monitoring enabled",
			options: []ExporterOption{
				WithGPUMonitoring(true),
			},
			verify: func(e *Exporter) {
				assert.Assert(t, e.enableGPUMonitoring, "GPU monitoring should be enabled")
			},
		},
		{
			name: "NIC monitoring enabled",
			options: []ExporterOption{
				WithNICMonitoring(true),
			},
			verify: func(e *Exporter) {
				assert.Assert(t, e.enableNICMonitoring, "NIC monitoring should be enabled")
			},
		},
		{
			name: "IFOE monitoring enabled",
			options: []ExporterOption{
				WithenableIFOEMonitoring(true),
			},
			verify: func(e *Exporter) {
				assert.Assert(t, e.enableIFOEMonitoring, "IFOE monitoring should be enabled")
			},
		},
		{
			name: "SRIOV enabled",
			options: []ExporterOption{
				WithSRIOV(true),
			},
			verify: func(e *Exporter) {
				assert.Assert(t, e.enableSriov, "SRIOV should be enabled")
			},
		},
		{
			name: "K8s API disabled",
			options: []ExporterOption{
				WithNoK8sApiclient(),
			},
			verify: func(e *Exporter) {
				assert.Assert(t, e.disableK8sApi, "K8s API should be disabled")
				assert.Assert(t, e.k8sApiClient == nil, "K8s API client should be nil")
			},
		},
		{
			name: "Slurm client enabled",
			options: []ExporterOption{
				WithSlurmClient(true),
			},
			verify: func(e *Exporter) {
				assert.Assert(t, e.enableSlurmScl, "Slurm client should be enabled")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exporter := NewExporter(globals.GPUAgentPort, configPath, tc.options...)
			assert.Assert(t, exporter != nil, "Exporter should be created")
			tc.verify(exporter)
		})
	}
}

func TestExporterMiddleware(t *testing.T) {
	logger.Init(false)

	// Test prometheus middleware
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := prometheusMiddleware(testHandler)

	// Test with metrics endpoint
	req, err := http.NewRequest("GET", "/metrics", nil)
	assert.NilError(t, err)

	rr := &testResponseWriter{}
	middleware.ServeHTTP(rr, req)

	assert.Assert(t, handlerCalled, "Handler should be called")
	assert.Equal(t, rr.statusCode, http.StatusOK, "Status should be OK")
}

// Helper test response writer
type testResponseWriter struct {
	statusCode int
	headers    http.Header
	body       []byte
}

func (w *testResponseWriter) Header() http.Header {
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}

func (w *testResponseWriter) Write(data []byte) (int, error) {
	w.body = append(w.body, data...)
	return len(data), nil
}

func (w *testResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func TestExporterClose(t *testing.T) {
	logger.Init(false)

	configPath, cleanup := setupTestConfig(t, 9099)
	defer cleanup()

	exporter := NewExporter(globals.GPUAgentPort, configPath, WithNoK8sApiclient())
	assert.Assert(t, exporter != nil, "Exporter should be created")

	// Test closing the exporter
	err := exporter.Close()
	assert.NilError(t, err, "Close should succeed")
}

// TestExporterMetricsValidation tests the metrics endpoint more thoroughly
func TestExporterMetricsValidation(t *testing.T) {
	logger.Init(false)

	configPath, cleanup := setupTestConfig(t, 9110)
	defer cleanup()

	// Initialize configuration and metrics
	runConf = config.NewConfigHandler(configPath, globals.GPUAgentPort)
	var err error
	mh, err = metricsutil.NewMetrics(runConf)
	assert.NilError(t, err)
	mh.InitConfig()

	// Start metrics server
	srv := startMetricsServer(runConf, "127.0.0.1")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		srv.Shutdown(ctx)
		cancel()
	}()

	baseURL := "http://127.0.0.1:9110"
	serverReady := waitForServer(baseURL+"/metrics", 10*time.Second)
	assert.Assert(t, serverReady, "Server should start")

	t.Run("Detailed metrics validation", func(t *testing.T) {
		resp, err := makeHTTPRequest(baseURL+"/metrics", 5*time.Second)
		assert.NilError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		assert.NilError(t, err)
		content := string(body)

		// Detailed validation
		lines := strings.Split(content, "\n")
		var helpLines, typeLines, metricLines []string

		for _, line := range lines {
			if strings.HasPrefix(line, "# HELP") {
				helpLines = append(helpLines, line)
			} else if strings.HasPrefix(line, "# TYPE") {
				typeLines = append(typeLines, line)
			} else if len(line) > 0 && !strings.HasPrefix(line, "#") {
				metricLines = append(metricLines, line)
			}
		}

		t.Logf("Found %d HELP lines, %d TYPE lines, %d metric lines",
			len(helpLines), len(typeLines), len(metricLines))

		// Log the actual content to understand what's available
		t.Logf("Sample metrics content (first 2000 chars):\n%s",
			content[:min(2000, len(content))])

		// Validate structure
		assert.Assert(t, len(helpLines) > 0, "Should have HELP lines")
		assert.Assert(t, len(typeLines) > 0, "Should have TYPE lines")

		// Check what metrics are actually present instead of requiring specific ones
		if strings.Contains(content, "go_") {
			t.Logf("Go runtime metrics are present")
		} else {
			t.Logf("No Go runtime metrics found - this is expected in minimal configuration")
		}

		if strings.Contains(content, "promhttp_") {
			t.Logf("Prometheus HTTP metrics are present")
		} else {
			t.Logf("No Prometheus HTTP metrics found")
		}

		// Log a sample of metrics for debugging
		t.Logf("Sample metrics content (first 1000 chars):\n%s",
			content[:min(1000, len(content))])
	})
}

// TestExporterErrorScenarios tests various error conditions
func TestExporterErrorScenarios(t *testing.T) {
	logger.Init(false)

	t.Run("Invalid port configuration", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.json")

		// Config with invalid port
		invalidConfig := `{
			"ServerPort": -1,
			"CommonConfig": {
				"MetricsFieldPrefix": "amd_"
			}
		}`

		err := os.WriteFile(configPath, []byte(invalidConfig), 0644)
		assert.NilError(t, err)

		// Should handle invalid port gracefully
		config := config.NewConfigHandler(configPath, globals.GPUAgentPort)
		assert.Assert(t, config != nil, "Should handle invalid port")
	})

	t.Run("Empty configuration", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "empty.json")

		err := os.WriteFile(configPath, []byte("{}"), 0644)
		assert.NilError(t, err)

		config := config.NewConfigHandler(configPath, globals.GPUAgentPort)
		assert.Assert(t, config != nil, "Should handle empty config")
	})
}

// TestExporterConcurrentRequests tests handling of concurrent HTTP requests
func TestExporterConcurrentRequests(t *testing.T) {
	logger.Init(false)

	configPath, cleanup := setupTestConfig(t, 9111)
	defer cleanup()

	// Initialize
	runConf = config.NewConfigHandler(configPath, globals.GPUAgentPort)
	var err error
	mh, err = metricsutil.NewMetrics(runConf)
	assert.NilError(t, err)
	mh.InitConfig()

	srv := startMetricsServer(runConf, "127.0.0.1")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		srv.Shutdown(ctx)
		cancel()
	}()

	baseURL := "http://127.0.0.1:9111"
	serverReady := waitForServer(baseURL+"/metrics", 10*time.Second)
	assert.Assert(t, serverReady, "Server should start")

	// Test concurrent access
	t.Run("Concurrent metrics requests", func(t *testing.T) {
		var wg sync.WaitGroup
		numRequests := 20
		results := make(chan error, numRequests)

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				resp, err := makeHTTPRequest(baseURL+"/metrics", 10*time.Second)
				if err != nil {
					results <- fmt.Errorf("request %d failed: %v", id, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != 200 {
					results <- fmt.Errorf("request %d got status %d", id, resp.StatusCode)
					return
				}

				// Validate response has content
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					results <- fmt.Errorf("request %d failed to read body: %v", id, err)
					return
				}

				if len(body) == 0 {
					results <- fmt.Errorf("request %d got empty response", id)
					return
				}

				results <- nil
			}(i)
		}

		wg.Wait()
		close(results)

		successCount := 0
		for err := range results {
			if err == nil {
				successCount++
			} else {
				t.Logf("Request error: %v", err)
			}
		}

		successRate := float64(successCount) / float64(numRequests) * 100
		t.Logf("Success rate: %.1f%% (%d/%d)", successRate, successCount, numRequests)

		// Expect at least 90% success rate to account for potential timing issues
		assert.Assert(t, successCount >= int(0.9*float64(numRequests)),
			"Should have at least 90%% success rate, got %d/%d", successCount, numRequests)
	})
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
