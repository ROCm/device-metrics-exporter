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
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	types "github.com/ROCm/device-metrics-exporter/pkg/testrunner/interface"
)

const (
	successCode = "AGFHC_SUCCESS"
)

const (
	AgfhcTestStatePassed  = "passed"
	AgfhcTestStateFailed  = "failed"
	AgfhcTestStateSkipped = "skipped"
	AgfhcTestStateQueued  = "queued"
)

type OverallSummary struct {
	TotalTests int    `json:"total_selected_tests"`
	Passed     int    `json:"total_passed"`
	StatusCode string `json:"status_code_text"`
}

type Args struct {
	DeviceIDs []string `json:"device_ids"`
}

type TestSummary struct {
	TotalIterations int `json:"total_iterations"`
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	Skipped         int `json:"skipped"`
	Queued          int `json:"queued"`
}

type TestResultInfo struct {
	Test            string `json:"test"`
	State           string `json:"state"`
	SuggestedAction string `json:"primary_suggested_action"`
	Subject         string `json:"primary_action_subject"`
}

type AgfhcTestResult struct {
	ProgramArgs Args                      `json:"program_args"`
	TestSummary map[string]TestSummary    `json:"test_summary"`
	TestResults map[string]TestResultInfo `json:"test_results"`
}

// TestRunner is a test framework for testing GPUs
type AgfhcTestRunner struct {
	// binaryLocation is the location where the test framework binary is present
	binaryLocation string

	// logDir represents the path where all the test run logs will be available
	logDir string

	// logger is the logger for the test runner process
	logger *log.Logger

	// testsuiteDir
	testSuitesDir string

	// testsuites
	testSuites map[string]bool
}

// GetTestHandler returns test handler for the given test and params
func (atr *AgfhcTestRunner) GetTestHandler(testName string, params types.TestParams) (types.TestHandlerInterface, error) {
	if _, ok := atr.testSuites[testName]; !ok {
		return nil, fmt.Errorf("testsuite %v not found", testName)
	}
	cmdArgs := []string{}
	cmdArgs = append(cmdArgs, atr.binaryLocation)
	cmdArgs = append(cmdArgs, "-r", testName)

	if len(params.DeviceIDs) > 0 {
		cmdArgs = append(cmdArgs, "-i")
		cmdArgs = append(cmdArgs, params.DeviceIDs...)
	}

	cmdArgs = append(cmdArgs, "-o", atr.logDir)
	if len(params.ExtraArgs) > 0 {
		cmdArgs = append(cmdArgs, params.ExtraArgs...)
	}
	var options []types.TOption
	if params.Timeout > 0 {
		options = append(options, types.TestWithTimeout(params.Timeout))
	}
	options = append(options, types.TestWithResultParser(atr.parseAgfhcTestResult),
		types.TestWithIteration(uint32(params.Iterations)), types.TestWithStopOnFailure(params.StopOnFailure))
	return types.NewTestHandler(testName, atr.logger, cmdArgs, options...), nil
}

// ExtractLogFile uses a simple regex to find the json results log file path and logs directory path
func (atr *AgfhcTestRunner) ExtractLogLocation(output string) (string, string, error) {
	// Pattern: matches test log directory /var/tmp/agfhc_YYYYMMDD-HHMMSS
	// Example: /var/tmp/agfhc_20231001-123456
	pattern := atr.logDir + `/agfhc_\d{8}-\d{6}`

	re := regexp.MustCompile(pattern)
	dirPath := re.FindString(output)

	if dirPath == "" {
		return "", "", fmt.Errorf("log file path not found")
	}

	resultsFilePath := fmt.Sprintf("%s/results.json", dirPath)

	return resultsFilePath, dirPath, nil
}

func (atr *AgfhcTestRunner) parseAgfhcTestResult(stdout string) (map[string]types.TestResults, error) {
	// get the log file
	file, _, err := atr.ExtractLogLocation(stdout)
	if err != nil {
		return nil, err
	}

	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var data AgfhcTestResult
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}

	removeLeadingZeros := func(id string) string {
		// ID looks like GPU-00; remove GPU- prefix and leading zeros
		gpuID := strings.TrimLeft(strings.Split(id, "-")[1], "0")
		if gpuID == "" {
			gpuID = "0"
		}
		return gpuID
	}

	failedDeviceIDs := func(subject string) map[string]bool {
		failedDeviceIDs := map[string]bool{}
		if len(subject) == 0 {
			return failedDeviceIDs
		}
		// Example subject: "GPU-00:GPU-01"
		subjectsList := strings.Split(subject, ":")
		for _, id := range subjectsList {
			failedDeviceIDs[id] = true
		}
		return failedDeviceIDs
	}

	mapToTestResultEnum := func(gpuID string, result, subject string) types.TestResult {
		switch {
		case AgfhcTestStatePassed == strings.ToLower(result):
			return types.Success
		case AgfhcTestStateFailed == strings.ToLower(result):
			failedDevices := failedDeviceIDs(subject)
			if len(failedDevices) > 0 {
				// If the subject is not empty, it means the test failed on specific devices
				if _, ok := failedDevices[gpuID]; ok {
					// If the GPU ID is in the failed devices, return failure
					return types.Failure
				} else {
					// If the GPU ID is not in the failed devices, return success
					return types.Success
				}
			}
			return types.Failure
		case AgfhcTestStateSkipped == strings.ToLower(result):
			return types.Skipped
		case AgfhcTestStateQueued == strings.ToLower(result):
			return types.Queued
		}

		return types.Failure
	}

	testResult := make(map[string]types.TestResults)
	for _, id := range data.ProgramArgs.DeviceIDs {
		// ID looks like GPU-00; remove GPU- prefix and leading zeros
		gpuID := removeLeadingZeros(id)
		for _, result := range data.TestResults {
			if testResult[gpuID] == nil {
				testResult[gpuID] = make(map[string]types.TestResult)
				testResult[gpuID] = make(types.TestResults)
			}
			testResult[gpuID][result.Test] = mapToTestResultEnum(id, result.State, result.Subject)
		}
	}
	return testResult, nil
}

// loadTestSuites loads the testsuite info
func (atr *AgfhcTestRunner) loadTestSuites() error {
	// E.g. /opt/amd/agfhc/recipes/mi300x/all_lvl1.yml
	files, err := os.ReadDir(atr.testSuitesDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yml") {
			// Add the testsuite to the map
			testSuiteName := strings.Split(file.Name(), ".")[0]
			logger.Log.Printf("loaded test suite %+v", testSuiteName)
			atr.testSuites[testSuiteName] = true
		}
	}
	return nil
}

// NewAgfhcTestRunner returns instance of NewAgfhcTestRunner
func NewAgfhcTestRunner(binPath, testSuitesDir, resultLogDir string) (types.TestRunner, error) {
	if len(binPath) == 0 {
		return nil, fmt.Errorf("rocm path is not set")
	}
	if _, err := os.Stat(binPath); err != nil {
		return nil, fmt.Errorf("failed to get agfhc binary from %+v err %+v", binPath, err)
	}
	if logger.Log == nil {
		return nil, fmt.Errorf("test runner logger is not initialized")
	}

	obj := &AgfhcTestRunner{
		binaryLocation: binPath,
		logDir:         resultLogDir,
		logger:         logger.Log,
		testSuites:     make(map[string]bool),
		testSuitesDir:  testSuitesDir,
	}

	err := obj.loadTestSuites()
	if err != nil {
		return nil, err
	}

	return obj, nil
}
