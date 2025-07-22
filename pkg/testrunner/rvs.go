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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	types "github.com/ROCm/device-metrics-exporter/pkg/testrunner/interface"
)

// RvsTestResult is used to convert test json output to struct
type RvsTestResult struct {
	Version    string `json:"version,omitempty"`
	TestSuites map[string]map[string][]RvsGPUTestResult
}

// unmarshalJSON custom unmarshaler to handle arbitrary test suite names
func (r *RvsTestResult) unmarshalJSON(data []byte) error {
	// Parse the full JSON to extract the test suites
	var rawResult map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawResult); err != nil {
		return err
	}

	r.TestSuites = make(map[string]map[string][]RvsGPUTestResult)

	// Process all fields in the JSON
	for key, value := range rawResult {
		if key == "version" {
			// If version field exists, extract it
			var version string
			if err := json.Unmarshal(value, &version); err != nil {
				return err
			}
			r.Version = version
		} else {
			// Any other field is considered a test suite
			var testSuite map[string][]RvsGPUTestResult
			if err := json.Unmarshal(value, &testSuite); err != nil {
				return err
			}
			r.TestSuites[key] = testSuite
		}
	}

	return nil
}

// RvsGPUTestResult is struct for GPU ID and it's test result
type RvsGPUTestResult struct {
	GPUID string `json:"gpu_index"`
	Pass  string `json:"pass,omitempty"`
}

// TestRunner is a test framework for testing GPUs
type RVSTestRunner struct {
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
func (rts *RVSTestRunner) GetTestHandler(testName string, params types.TestParams) (types.TestHandlerInterface, error) {
	if _, ok := rts.testSuites[testName]; !ok {
		return nil, fmt.Errorf("testsuite %v not found", testName)
	}
	cmdArgs := []string{}
	cmdArgs = append(cmdArgs, rts.binaryLocation)
	confFile := filepath.Join(rts.testSuitesDir, fmt.Sprintf("%v.conf", testName))
	cmdArgs = append(cmdArgs, "-c", confFile)

	if len(params.DeviceIDs) > 0 {
		cmdArgs = append(cmdArgs, "-i", strings.Join(params.DeviceIDs, ","))
	}

	cmdArgs = append(cmdArgs, "-j")
	if len(params.ExtraArgs) > 0 {
		cmdArgs = append(cmdArgs, params.ExtraArgs...)
	}
	var options []types.TOption
	if params.Timeout > 0 {
		options = append(options, types.TestWithTimeout(params.Timeout))
	}
	options = append(options, types.TestWithResultParser(rts.parseRvsTestResult),
		types.TestWithIteration(uint32(params.Iterations)), types.TestWithStopOnFailure(params.StopOnFailure))
	return types.NewTestHandler(testName, rts.logger, cmdArgs, options...), nil
}

// loadTestSuites loads the testsuite info
func (rts *RVSTestRunner) loadTestSuites() error {
	files, err := os.ReadDir(rts.testSuitesDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".conf") {
			// Add the testsuite to the map
			testSuiteName := strings.Split(file.Name(), ".")[0]
			logger.Log.Printf("loaded test suite %+v", testSuiteName)
			rts.testSuites[testSuiteName] = true
		}
	}
	return nil
}

// ExtractLogFile uses a simple regex to find the json log file path
func (rts *RVSTestRunner) ExtractLogLocation(output string) (string, string, error) {
	// Pattern: matches /var/tmp/<test_name>_<timestamp>.json
	pattern := rts.logDir + `/[^/]+_\d+\.json`

	re := regexp.MustCompile(pattern)
	resultsFilePath := re.FindString(output)

	if resultsFilePath == "" {
		return "", "", fmt.Errorf("log file path not found")
	}

	return resultsFilePath, "", nil
}

func (rts *RVSTestRunner) parseRvsTestResult(stdout string) (map[string]types.TestResults, error) {
	// get the log file
	file, _, err := rts.ExtractLogLocation(stdout)
	if err != nil {
		return nil, err
	}

	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var data RvsTestResult
	err = data.unmarshalJSON(bytes)
	if err != nil {
		return nil, err
	}

	testResult := make(map[string]types.TestResults)

	for _, testsuite := range data.TestSuites {
		for name, test := range testsuite {
			for _, gpu := range test {
				if len(gpu.Pass) == 0 {
					continue
				}
				if testResult[gpu.GPUID] == nil {
					testResult[gpu.GPUID] = make(map[string]types.TestResult)
					testResult[gpu.GPUID] = make(types.TestResults)
				}
				testResult[gpu.GPUID][name] = getTestResultEnum(gpu.Pass)
			}
		}
	}

	return testResult, nil
}

func getTestResultEnum(val string) types.TestResult {
	if val == "true" {
		return types.Success
	}
	return types.Failure
}

// NewRvsTestRunner returns instance of RvsTestRunner
func NewRvsTestRunner(rvsBinPath, testSuitesDir, resultLogDir string) (types.TestRunner, error) {
	if len(rvsBinPath) == 0 {
		return nil, fmt.Errorf("rocm path is not set")
	}
	if _, err := os.Stat(rvsBinPath); err != nil {
		return nil, fmt.Errorf("failed to get rvs binary from %+v err %+v", rvsBinPath, err)
	}
	if logger.Log == nil {
		return nil, fmt.Errorf("test runner logger is not initialized")
	}

	obj := &RVSTestRunner{
		binaryLocation: rvsBinPath,
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
