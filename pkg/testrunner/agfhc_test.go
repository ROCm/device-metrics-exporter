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
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	types "github.com/ROCm/device-metrics-exporter/pkg/testrunner/interface"
)

func TestNewAgfhcTestRunner(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "agfhc_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock binary file
	mockBinPath := filepath.Join(tmpDir, "agfhc")
	err = os.WriteFile(mockBinPath, []byte("#!/bin/bash\necho 'Mock AGFHC binary'"), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock binary: %v", err)
	}

	// Create mock test suite directory and yml files
	testSuitesDir := filepath.Join(tmpDir, "recipes")
	err = os.MkdirAll(testSuitesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test suites directory: %v", err)
	}
	err = os.WriteFile(filepath.Join(testSuitesDir, "all_lvl1.yml"), []byte("test recipe"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test recipe file: %v", err)
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
	runner, err := NewAgfhcTestRunner(mockBinPath, testSuitesDir, resultLogDir)
	assert.NoError(t, err)
	assert.NotNil(t, runner)
	agfhcRunner, ok := runner.(*AgfhcTestRunner)
	assert.True(t, ok)
	assert.Equal(t, mockBinPath, agfhcRunner.binaryLocation)
	assert.Equal(t, resultLogDir, agfhcRunner.logDir)
	assert.Equal(t, testSuitesDir, agfhcRunner.testSuitesDir)
	assert.NotNil(t, agfhcRunner.testSuites)

	// Test case 2: Empty binary path
	runner, err = NewAgfhcTestRunner("", testSuitesDir, resultLogDir)
	assert.Error(t, err)
	assert.Nil(t, runner)

	// Test case 3: Non-existent binary
	runner, err = NewAgfhcTestRunner(filepath.Join(tmpDir, "nonexistent"), testSuitesDir, resultLogDir)
	assert.Error(t, err)
	assert.Nil(t, runner)

	// Test case 4: Non-existent test suites directory
	// runner, err = NewAgfhcTestRunner(mockBinPath, filepath.Join(tmpDir, "nonexistent"), resultLogDir)
	// assert.NoError(t, err) // Doesn't check if directory exists, just loads test suites which will be empty
	// assert.NotNil(t, runner)
}

func TestAgfhcGetTestHandler(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "agfhc_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock binary file
	mockBinPath := filepath.Join(tmpDir, "agfhc")
	err = os.WriteFile(mockBinPath, []byte("#!/bin/bash\necho 'Mock AGFHC binary'"), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock binary: %v", err)
	}

	// Create mock test suite directory and yml files
	testSuitesDir := filepath.Join(tmpDir, "recipes")
	err = os.MkdirAll(testSuitesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test suites directory: %v", err)
	}

	testName := "all_lvl1"
	err = os.WriteFile(filepath.Join(testSuitesDir, testName+".yml"), []byte("test recipe"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test recipe file: %v", err)
	}

	// Create result log directory
	resultLogDir := filepath.Join(tmpDir, "logs")
	err = os.MkdirAll(resultLogDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	// Setup logger
	logger.Log = log.New(os.Stdout, "TEST: ", log.LstdFlags)

	// Create a test runner
	runner, err := NewAgfhcTestRunner(mockBinPath, testSuitesDir, resultLogDir)
	assert.NoError(t, err)
	assert.NotNil(t, runner)

	// Add test suite manually for testing
	agfhcRunner := runner.(*AgfhcTestRunner)
	agfhcRunner.testSuites = map[string]bool{
		testName: true,
	}

	// Test case 1: Valid test handler
	params := types.TestParams{
		DeviceIDs:     []string{"0", "1"},
		Iterations:    2,
		Timeout:       300,
		StopOnFailure: true,
		ExtraArgs:     []string{"--option1", "value1"},
	}
	handler, err := runner.GetTestHandler(testName, params)
	assert.NoError(t, err)
	assert.NotNil(t, handler)

	// Test case 2: Non-existent test suite
	handler, err = runner.GetTestHandler("nonexistent", params)
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestAgfhcExtractLogLocation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agfhc_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	runner := &AgfhcTestRunner{
		logDir: tmpDir,
	}

	testCases := []struct {
		name     string
		output   string
		expected []string
		hasError bool
	}{
		{
			name:     "Valid log path",
			output:   "Starting test...\nLog directory: " + tmpDir + "/agfhc_20240101-123456\nTest completed.",
			expected: []string{tmpDir + "/agfhc_20240101-123456/results.json", tmpDir + "/agfhc_20240101-123456"},
			hasError: false,
		},
		{
			name:     "No log path",
			output:   "Starting test...\nTest completed with no log directory info.",
			expected: []string{},
			hasError: true,
		},
		{
			name:     "Multiple log paths, take first",
			output:   "Log dir: " + tmpDir + "/agfhc_20240101-123456\nAnother log: " + tmpDir + "/agfhc_20240101-789012",
			expected: []string{tmpDir + "/agfhc_20240101-123456/results.json", tmpDir + "/agfhc_20240101-123456"},
			hasError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, dirPath, err := runner.ExtractLogLocation(tc.output)
			if tc.hasError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected[0], result)
				assert.Equal(t, tc.expected[1], dirPath)
			}
		})
	}
}

func TestAgfhcParseAgfhcTestResult(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "agfhc_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock test result file
	testData := AgfhcTestResult{
		ProgramArgs: Args{
			DeviceIDs: []string{"GPU-00", "GPU-01"},
		},
		TestSummary: map[string]TestSummary{
			"test1": {
				TotalIterations: 1,
				Passed:          1,
				Failed:          0,
				Skipped:         0,
				Queued:          0,
			},
			"test2": {
				TotalIterations: 1,
				Passed:          0,
				Failed:          1,
				Skipped:         0,
				Queued:          0,
			},
			"test3": {
				TotalIterations: 1,
				Passed:          0,
				Failed:          0,
				Skipped:         1,
				Queued:          0,
			},
			"test4": {
				TotalIterations: 1,
				Passed:          0,
				Failed:          0,
				Skipped:         0,
				Queued:          1,
			},
			"test5": {
				TotalIterations: 1,
				Passed:          0,
				Failed:          1,
				Skipped:         0,
				Queued:          0,
			},
		},
		TestResults: map[string]TestResultInfo{
			"test1": {
				Test:            "test1",
				State:           AgfhcTestStatePassed,
				SuggestedAction: "Success",
			},
			"test2": {
				Test:            "test2",
				State:           AgfhcTestStateFailed,
				SuggestedAction: "Service",
				Subject:         "GPU-00:GPU-01",
			},
			"test3": {
				Test:            "test3",
				State:           AgfhcTestStateSkipped,
				SuggestedAction: "Success",
			},
			"test4": {
				Test:            "test4",
				State:           AgfhcTestStateQueued,
				SuggestedAction: "Success",
			},
			"test5": {
				Test:            "test5",
				State:           AgfhcTestStateFailed,
				SuggestedAction: "Service",
				Subject:         "GPU-01",
			},
		},
	}

	resultJSON, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	// Create test results directory
	resultDir := filepath.Join(tmpDir, "agfhc_20240101-123456")
	err = os.MkdirAll(resultDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create result directory: %v", err)
	}

	// Write the test results JSON
	resultFile := filepath.Join(resultDir, "results.json")
	err = os.WriteFile(resultFile, resultJSON, 0644)
	if err != nil {
		t.Fatalf("Failed to write result file: %v", err)
	}

	runner := &AgfhcTestRunner{
		logDir: tmpDir,
	}

	// Mock output that includes the path to the results file
	mockOutput := "Test completed successfully. Results saved to " + resultDir

	// Test parseAgfhcTestResult function
	results, err := runner.parseAgfhcTestResult(mockOutput)
	assert.NoError(t, err)
	assert.NotNil(t, results)

	// Check that results were parsed correctly
	assert.Contains(t, results, "0") // GPU-00's index
	assert.Contains(t, results, "1") // GPU-01's index

	// Check that the test1 passed and test2 failed
	assert.Equal(t, types.Success, results["0"]["test1"])
	assert.Equal(t, types.Success, results["1"]["test1"])

	assert.Equal(t, types.Failure, results["0"]["test2"])
	assert.Equal(t, types.Failure, results["1"]["test2"])

	// Check that test3 was skipped and test4 was queued
	assert.Equal(t, types.Skipped, results["0"]["test3"])
	assert.Equal(t, types.Skipped, results["1"]["test3"])

	assert.Equal(t, types.Queued, results["0"]["test4"])
	assert.Equal(t, types.Queued, results["1"]["test4"])

	// Check that test5 failed on GPU-01
	assert.Equal(t, types.Success, results["0"]["test5"])
	assert.Equal(t, types.Failure, results["1"]["test5"])
}

func TestAgfhcLoadTestSuites(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "agfhc_test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock test suite directory with yml files
	testSuitesDir := filepath.Join(tmpDir, "recipes")
	err = os.MkdirAll(testSuitesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test suites directory: %v", err)
	}

	// Create a few test recipe files
	testNames := []string{"all_lvl1", "all_lvl2", "specific_test"}
	for _, name := range testNames {
		err = os.WriteFile(filepath.Join(testSuitesDir, name+".yml"), []byte("test recipe"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test recipe file %s: %v", name, err)
		}
	}

	// Also create a non-yml file which should be ignored
	err = os.WriteFile(filepath.Join(testSuitesDir, "readme.txt"), []byte("This is not a test recipe"), 0644)
	if err != nil {
		t.Fatalf("Failed to create non-yml file: %v", err)
	}

	// Setup logger
	logger.Log = log.New(os.Stdout, "TEST: ", log.LstdFlags)

	// Create a test runner
	runner := &AgfhcTestRunner{
		testSuitesDir: testSuitesDir,
		testSuites:    make(map[string]bool),
		logger:        logger.Log,
	}

	// Test loadTestSuites function
	err = runner.loadTestSuites()
	assert.NoError(t, err)

	// Check that all yml files were loaded as test suites
	for _, name := range testNames {
		assert.True(t, runner.testSuites[name], "Test suite %s was not loaded", name)
	}

	// Check that non-yml files were not loaded
	assert.False(t, runner.testSuites["readme"], "Non-yml file was incorrectly loaded as a test suite")
}
