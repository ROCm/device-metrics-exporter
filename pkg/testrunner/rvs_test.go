/*
*
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
*
*/

package testrunner

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	types "github.com/ROCm/device-metrics-exporter/pkg/testrunner/interface"
)

func TestNewRvsTestRunner(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "rvs_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock binary file
	mockBinPath := filepath.Join(tmpDir, "rvs")
	err = os.WriteFile(mockBinPath, []byte("#!/bin/bash\necho 'Mock RVS binary'"), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock binary: %v", err)
	}

	// Create mock test suite directory and conf file
	testSuitesDir := filepath.Join(tmpDir, "conf")
	err = os.MkdirAll(testSuitesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test suites directory: %v", err)
	}
	err = os.WriteFile(filepath.Join(testSuitesDir, "gst_single.conf"), []byte("test config"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Create result log directory
	resultLogDir := filepath.Join(tmpDir, "logs")
	err = os.MkdirAll(resultLogDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	// Setup logger
	logger.Log = log.New(os.Stdout, "TEST: ", log.LstdFlags)

	// Test case 1: Valid initialization
	runner, err := NewRvsTestRunner(mockBinPath, testSuitesDir, resultLogDir)
	assert.NoError(t, err)
	assert.NotNil(t, runner)

	// Test case 2: Empty binary path
	runner, err = NewRvsTestRunner("", testSuitesDir, resultLogDir)
	assert.Error(t, err)
	assert.Nil(t, runner)

	// Test case 3: Non-existent binary
	runner, err = NewRvsTestRunner(filepath.Join(tmpDir, "nonexistent"), testSuitesDir, resultLogDir)
	assert.Error(t, err)
	assert.Nil(t, runner)
}

func TestGetTestHandler(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "rvs_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock binary file
	mockBinPath := filepath.Join(tmpDir, "rvs")
	err = os.WriteFile(mockBinPath, []byte("#!/bin/bash\necho 'Mock RVS binary'"), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock binary: %v", err)
	}

	// Create mock test suite directory and conf file
	testSuitesDir := filepath.Join(tmpDir, "conf")
	err = os.MkdirAll(testSuitesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test suites directory: %v", err)
	}
	testSuiteName := "gst_single"
	err = os.WriteFile(filepath.Join(testSuitesDir, testSuiteName+".conf"), []byte("test config"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Create result log directory
	resultLogDir := filepath.Join(tmpDir, "logs")
	err = os.MkdirAll(resultLogDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	// Setup logger
	logger.Log = log.New(io.Discard, "", 0) // Use a silent logger for tests

	// Create an RVSTestRunner instance for testing
	rvsRunner := &RVSTestRunner{
		binaryLocation: mockBinPath,
		logDir:         resultLogDir,
		logger:         logger.Log,
		testSuites:     map[string]bool{testSuiteName: true},
		testSuitesDir:  testSuitesDir,
	}

	// Test case 1: Valid test handler creation
	params := types.TestParams{
		Iterations:    1,
		StopOnFailure: true,
		DeviceIDs:     []string{"0", "1"},
		Timeout:       300,
		ExtraArgs:     []string{"--arg1", "--arg2"},
	}
	handler, err := rvsRunner.GetTestHandler(testSuiteName, params)
	assert.NoError(t, err)
	assert.NotNil(t, handler)

	// Test case 2: Invalid test name
	handler, err = rvsRunner.GetTestHandler("nonexistent_test", params)
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestExtractLogLocation(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "rvs_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create an RVSTestRunner instance for testing
	rvsRunner := &RVSTestRunner{
		logDir: tmpDir,
	}

	// Test case 1: Valid log file path in output
	output := "Test output\n" + tmpDir + "/rvs_123456.json\nMore output"
	logFilePath, _, err := rvsRunner.ExtractLogLocation(output)
	assert.NoError(t, err)
	assert.Equal(t, tmpDir+"/rvs_123456.json", logFilePath)

	// Test case 2: No log file path in output
	output = "Test output without log file path"
	logFilePath, _, err = rvsRunner.ExtractLogLocation(output)
	assert.Error(t, err)
	assert.Equal(t, "", logFilePath)
}

func TestParseRvsTestResult(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "rvs_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a sample RVS result JSON file with version field
	jsonData := `{
		"version": "1.0",
		"testsuite1": {
			"test1": [
				{"gpu_index": "0", "pass": "true"},
				{"gpu_index": "1", "pass": "false"}
			],
			"test2": [
				{"gpu_index": "0", "pass": "true"},
				{"gpu_index": "1", "pass": "true"}
			]
		}
	}`

	jsonFilePath := filepath.Join(tmpDir, "rvs_1742337000348.json")
	err = os.WriteFile(jsonFilePath, []byte(jsonData), 0644)
	if err != nil {
		t.Fatalf("Failed to create sample JSON file: %v", err)
	}

	// Create an RVSTestRunner instance for testing
	rvsRunner := &RVSTestRunner{
		logDir: tmpDir,
	}

	// Prepare a stdout that contains the log file path
	stdout := "Test output with log file " + jsonFilePath + " mentioned"

	// Parse the RVS test result
	results, err := rvsRunner.parseRvsTestResult(stdout)
	assert.NoError(t, err)
	assert.NotNil(t, results)

	// Verify the parsed results
	assert.Equal(t, types.Success, results["0"]["test1"])
	assert.Equal(t, types.Failure, results["1"]["test1"])
	assert.Equal(t, types.Success, results["0"]["test2"])
	assert.Equal(t, types.Success, results["1"]["test2"])

	// Test case 2: JSON without version field
	jsonDataNoVersion := `{
		"testsuite2": {
			"test3": [
				{"gpu_index": "0", "pass": "true"},
				{"gpu_index": "1", "pass": "false"}
			],
			"test4": [
				{"gpu_index": "0", "pass": "false"},
				{"gpu_index": "1", "pass": "true"}
			]
		}
	}`

	jsonFilePathNoVersion := filepath.Join(tmpDir, "rvs_1742337000349.json")
	err = os.WriteFile(jsonFilePathNoVersion, []byte(jsonDataNoVersion), 0644)
	if err != nil {
		t.Fatalf("Failed to create sample JSON file without version: %v", err)
	}

	// Prepare a stdout that contains the log file path for the no-version JSON
	stdoutNoVersion := "Test output with log file " + jsonFilePathNoVersion + " mentioned"

	// Parse the RVS test result - this should succeed even without version
	resultsNoVersion, err := rvsRunner.parseRvsTestResult(stdoutNoVersion)
	assert.NoError(t, err)
	assert.NotNil(t, resultsNoVersion)

	// Verify the parsed results
	assert.Equal(t, types.Success, resultsNoVersion["0"]["test3"])
	assert.Equal(t, types.Failure, resultsNoVersion["1"]["test3"])
	assert.Equal(t, types.Failure, resultsNoVersion["0"]["test4"])
	assert.Equal(t, types.Success, resultsNoVersion["1"]["test4"])

	// Test case 3: Invalid log file path
	stdout = "Test output with no log file path"
	results, err = rvsRunner.parseRvsTestResult(stdout)
	assert.Error(t, err)
	assert.Nil(t, results)

	// Test case 4: Valid log file path but invalid JSON
	invalidJSONPath := filepath.Join(tmpDir, "invalid.json")
	err = os.WriteFile(invalidJSONPath, []byte("not valid json"), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid JSON file: %v", err)
	}
	stdout = "Test output with log file " + invalidJSONPath + " mentioned"
	results, err = rvsRunner.parseRvsTestResult(stdout)
	assert.Error(t, err)
	assert.Nil(t, results)
}

func TestLoadTestSuites(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "rvs_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock test suite directory and conf files
	testSuitesDir := filepath.Join(tmpDir, "conf")
	err = os.MkdirAll(testSuitesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test suites directory: %v", err)
	}

	// Create multiple test config files
	testSuites := []string{"gst_single", "gpuinfo", "memory"}
	for _, suite := range testSuites {
		err = os.WriteFile(filepath.Join(testSuitesDir, suite+".conf"), []byte("test config"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}
	}

	// Also create some non-conf files that should be ignored
	err = os.WriteFile(filepath.Join(testSuitesDir, "readme.txt"), []byte("readme"), 0644)
	if err != nil {
		t.Fatalf("Failed to create non-conf file: %v", err)
	}

	// Create a subdirectory that should be ignored
	err = os.MkdirAll(filepath.Join(testSuitesDir, "subdir"), 0755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Setup logger
	logger.Log = log.New(io.Discard, "", 0) // Use a silent logger for tests

	// Create an RVSTestRunner instance for testing
	rvsRunner := &RVSTestRunner{
		testSuitesDir: testSuitesDir,
		testSuites:    make(map[string]bool),
		logger:        logger.Log,
	}

	// Call loadTestSuites
	err = rvsRunner.loadTestSuites()
	assert.NoError(t, err)

	// Verify that all test suites were loaded
	for _, suite := range testSuites {
		assert.True(t, rvsRunner.testSuites[suite])
	}

	// Verify that non-conf files were not loaded
	assert.False(t, rvsRunner.testSuites["readme"])
	assert.False(t, rvsRunner.testSuites["subdir"])

	// Test case 2: Non-existent test suite directory
	rvsRunner = &RVSTestRunner{
		testSuitesDir: filepath.Join(tmpDir, "nonexistent"),
		testSuites:    make(map[string]bool),
		logger:        logger.Log,
	}

	err = rvsRunner.loadTestSuites()
	assert.Error(t, err)
}

func TestGetTestResultEnum(t *testing.T) {
	// Test case 1: Pass = "true"
	result := getTestResultEnum("true")
	assert.Equal(t, types.Success, result)

	// Test case 2: Any other value
	result = getTestResultEnum("false")
	assert.Equal(t, types.Failure, result)

	result = getTestResultEnum("")
	assert.Equal(t, types.Failure, result)

	result = getTestResultEnum("TRUE")
	assert.Equal(t, types.Failure, result)
}
