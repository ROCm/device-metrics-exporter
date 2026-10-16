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

const (
	maxRVSParseRetries = 10
)

// RvsTestResult is used to convert test json output to struct
type RvsTestResult struct {
	Version    string                     `json:"version,omitempty"`
	TestSuites map[string]json.RawMessage `json:"-"`
}

// unmarshalJSON custom unmarshaler to handle arbitrary test suite names
func (r *RvsTestResult) unmarshalJSON(data []byte) error {
	// Parse the full JSON to extract the test suites
	var rawResult map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawResult); err != nil {
		return err
	}

	r.TestSuites = make(map[string]json.RawMessage)

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
			// RVS suite result layouts are not uniform. Keep each suite as raw
			// JSON and walk it recursively so both flat records and nested
			// per-subtest records can be handled.
			r.TestSuites[key] = value
		}
	}

	return nil
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

	// kfdIDToGPUIndexMap normalizes RVS gpu_id values to the canonical indexes
	// used by runner status and events.
	kfdIDToGPUIndexMap map[string]string
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
	return filepath.WalkDir(rts.testSuitesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".conf") {
			// Get relative path from testSuitesDir
			relPath, err := filepath.Rel(rts.testSuitesDir, path)
			if err != nil {
				return err
			}
			// Remove .conf extension to get test suite name
			testSuiteName := strings.TrimSuffix(relPath, ".conf")
			logger.Log.Printf("loaded test suite %+v", testSuiteName)
			rts.testSuites[testSuiteName] = true
		}
		return nil
	})
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

	// Retry up to 10 times, removing trailing } on each attempt
	currentBytes := bytes
	for i := 0; i <= maxRVSParseRetries; i++ {
		err = data.unmarshalJSON(currentBytes)
		if err == nil {
			break
		}

		// RVS with ROCm 7.1.1 has an issue where it appends multiple extra } at the end of the JSON output
		// Attempt to fix by removing trailing } characters
		// Check if error is about "invalid character '}' after top-level value"
		if i < maxRVSParseRetries && strings.Contains(err.Error(), "invalid character") && strings.Contains(err.Error(), "}") {
			// Remove trailing }
			trimmed := strings.TrimSpace(string(currentBytes))
			if strings.HasSuffix(trimmed, "}") {
				trimmed = strings.TrimSuffix(trimmed, "}")
				currentBytes = []byte(trimmed)
				//rts.logger.Printf("Retry %d: Removed trailing '}' from JSON, err: %v", i+1, err)
				continue
			}
		}

		// If we can't fix it or it's a different error, return
		return nil, fmt.Errorf("failed to unmarshal JSON after %d retries: %v", i, err)
	}

	testResult := make(map[string]types.TestResults)
	for _, suite := range data.TestSuites {
		var result any
		if err := json.Unmarshal(suite, &result); err != nil {
			return nil, fmt.Errorf("failed to decode RVS test suite result: %w", err)
		}
		if err := collectRVSResults(result, "", "", rts.kfdIDToGPUIndexMap, testResult); err != nil {
			return nil, fmt.Errorf("failed to associate RVS test result with a GPU index: %w", err)
		}
	}

	return testResult, nil
}

// collectRVSResults finds result records at any depth in an RVS suite. Most
// suites emit flat action records. Babel emits action metadata with pass/fail
// values nested under results[]. GPU identity is inherited from parent records
// when a nested subtest does not repeat it.
func collectRVSResults(value any, testName, gpuIndex string, kfdIDToGPUIndex map[string]string, results map[string]types.TestResults) error {
	switch node := value.(type) {
	case map[string]any:
		if index := rvsString(node["gpu_index"]); index != "" {
			gpuIndex = index
		}
		if gpuID := rvsString(node["gpu_id"]); gpuID != "" {
			if index, ok := kfdIDToGPUIndex[gpuID]; ok {
				if gpuIndex != "" && gpuIndex != index {
					return fmt.Errorf("RVS gpu_id %q maps to GPU index %q but record says %q", gpuID, index, gpuIndex)
				}
				gpuIndex = index
			} else if gpuIndex == "" {
				return fmt.Errorf("RVS gpu_id %q has no canonical GPU index", gpuID)
			}
		}

		if subtest := firstRVSString(node, "subtest", "Function"); subtest != "" {
			testName = joinRVSResultName(testName, subtest)
		}

		if pass, ok := node["pass"]; ok {
			gpuIndexes := []string{gpuIndex}
			if gpuIndex == "" {
				// Topology suites (PBQT/PEBB) identify participating GPUs by
				// endpoint rather than by gpu_index.
				gpuIndexes = uniqueRVSStrings(rvsString(node["srcgpu"]), rvsString(node["dstgpu"]))
			}
			if testName == "" {
				testName = "result"
			}
			if passValue, valid := rvsPassValue(pass); valid {
				for _, index := range gpuIndexes {
					if index == "" {
						continue
					}
					if results[index] == nil {
						results[index] = make(types.TestResults)
					}
					parsed := getTestResultEnum(passValue)
					// If multiple records map to the same action, a failure must
					// not be hidden by a later passing record.
					if previous, exists := results[index][testName]; !exists || parsed == types.Failure || previous != types.Failure {
						results[index][testName] = parsed
					}
				}
			}
		}

		for key, child := range node {
			if key == "pass" || key == "subtest" || key == "Function" || isRVSMetadataKey(key) {
				continue
			}
			childName := testName
			if key != "results" {
				childName = joinRVSResultName(testName, key)
			}
			if err := collectRVSResults(child, childName, gpuIndex, kfdIDToGPUIndex, results); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := collectRVSResults(child, testName, gpuIndex, kfdIDToGPUIndex, results); err != nil {
				return err
			}
		}
	}
	return nil
}

func isRVSMetadataKey(key string) bool {
	switch key {
	case "version", "gpu_index", "gpu_id", "srcgpu", "dstgpu":
		return true
	default:
		return false
	}
}

func joinRVSResultName(parent, child string) string {
	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}
	return parent + "/" + child
}

func firstRVSString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := rvsString(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func rvsString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%v", typed)
	default:
		return ""
	}
}

func rvsPassValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		pass := strings.ToLower(strings.TrimSpace(typed))
		return pass, pass != ""
	case bool:
		if typed {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

func uniqueRVSStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
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
		logger:         logger.Log.Log,
		testSuites:     make(map[string]bool),
		testSuitesDir:  testSuitesDir,
	}

	err := obj.loadTestSuites()
	if err != nil {
		return nil, err
	}

	return obj, nil
}
