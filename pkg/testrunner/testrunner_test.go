/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testrunner

import (
	"os"
	"path/filepath"
	"testing"

	testrunnerGen "github.com/ROCm/device-metrics-exporter/pkg/testrunner/gen/testrunner"
	types "github.com/ROCm/device-metrics-exporter/pkg/testrunner/interface"
	"github.com/stretchr/testify/assert"
)

func TestSetConfigDefaults_FrameworkDefault(t *testing.T) {
	cfg := &testrunnerGen.TestRunnerConfig{
		TestConfig: map[string]*testrunnerGen.TestCategoryConfig{
			"GPU_HEALTH_CHECK": {
				TestLocationTrigger: map[string]*testrunnerGen.TestTriggerConfig{
					"global": {
						TestParameters: map[string]*testrunnerGen.TestParameters{
							"MANUAL": {
								TestCases: []*testrunnerGen.TestParameter{
									{Framework: ""},
								},
							},
						},
					},
				},
			},
		},
	}
	tr := &TestRunner{globalTestRunnerConfig: cfg}
	tr.setConfigDefaults()
	got := tr.globalTestRunnerConfig.TestConfig["GPU_HEALTH_CHECK"].TestLocationTrigger["global"].TestParameters["MANUAL"].TestCases[0].Framework
	assert.Equal(t, testrunnerGen.TestParameter_RVS.String(), got)
}

func TestNormalizeConfig_UppercaseKeys(t *testing.T) {
	cfg := &testrunnerGen.TestRunnerConfig{
		TestConfig: map[string]*testrunnerGen.TestCategoryConfig{
			"gpu_health_check": {
				TestLocationTrigger: map[string]*testrunnerGen.TestTriggerConfig{
					"global": {
						TestParameters: map[string]*testrunnerGen.TestParameters{
							"manual": {TestCases: []*testrunnerGen.TestParameter{{Framework: "RVS"}}},
						},
					},
				},
			},
		},
	}
	tr := &TestRunner{globalTestRunnerConfig: cfg}
	tr.normalizeConfig()
	_, ok := tr.globalTestRunnerConfig.TestConfig["GPU_HEALTH_CHECK"]
	assert.True(t, ok)
	_, ok = tr.globalTestRunnerConfig.TestConfig["gpu_health_check"]
	assert.False(t, ok)
}

func TestGetTestRecipeDir(t *testing.T) {
	tr := &TestRunner{
		rvsTestCaseDir:      "/tmp/rvs",
		agfhcTestCaseDir:    "/tmp/agfhc",
		testCfgGPUModelName: "MI300A",
	}
	assert.Equal(t, filepath.Join("/tmp/rvs", "MI300A"), tr.getTestRecipeDir(types.RVSRunner))
	assert.Equal(t, filepath.Join("/tmp/agfhc", "MI300A"), tr.getTestRecipeDir(types.AGFHCRunner))
}

func TestGetHostName(t *testing.T) {
	tr := &TestRunner{}
	orig := os.Getenv("NODE_NAME")
	defer os.Setenv("NODE_NAME", orig)
	os.Setenv("NODE_NAME", "testnode")
	tr.getHostName()
	assert.NotEmpty(t, tr.hostName)
}

func TestGetOverallResult(t *testing.T) {
	tr := &TestRunner{}
	tr.initLogger()
	validIDs := []string{"gpu0", "gpu1"}

	type testCase struct {
		name     string
		result   []*types.IterationResult
		expected types.TestResult
		check    func(t *testing.T, result []*types.IterationResult)
	}

	cases := []testCase{
		{
			name: "Failure",
			result: []*types.IterationResult{
				{
					Number: 1,
					SuitesResult: map[string]types.TestResults{
						"gpu0": {"action1": types.Failure},
					},
				},
			},
			expected: types.Failure,
		},
		{
			name: "Skipped",
			result: []*types.IterationResult{
				{
					Number: 1,
					SuitesResult: map[string]types.TestResults{
						"gpu0": {"action1": types.Skipped},
					},
				},
			},
			expected: types.Skipped,
		},
		{
			name: "Partially Skipped",
			result: []*types.IterationResult{
				{
					Number: 1,
					SuitesResult: map[string]types.TestResults{
						"gpu0": {"action1": types.Skipped, "action2": types.Success},
						"gpu1": {"action1": types.Success, "action2": types.Success},
					},
				},
			},
			expected: types.Success,
		},
		{
			name: "Queued",
			result: []*types.IterationResult{
				{
					Number: 1,
					SuitesResult: map[string]types.TestResults{
						"gpu0": {"action1": types.Queued},
					},
				},
			},
			expected: types.Queued,
		},
		{
			name: "TimedoutAction",
			result: []*types.IterationResult{
				{
					Number: 1,
					SuitesResult: map[string]types.TestResults{
						"gpu0": {"action1": types.Timedout},
					},
				},
			},
			expected: types.Timedout,
		},
		{
			name: "NoTestResult",
			result: []*types.IterationResult{
				{
					Number:       1,
					SuitesResult: map[string]types.TestResults{},
				},
			},
			expected: types.Failure,
			check: func(t *testing.T, result []*types.IterationResult) {
				for _, gpuID := range validIDs {
					_, ok := result[0].SuitesResult[gpuID]
					assert.True(t, ok)
					assert.Equal(t, types.Failure, result[0].SuitesResult[gpuID]["result"])
				}
			},
		},
		{
			name: "Success",
			result: []*types.IterationResult{
				{
					Number: 1,
					SuitesResult: map[string]types.TestResults{
						"gpu0": {"action1": types.Success},
						"gpu1": {"action1": types.Success},
					},
					Status: types.TestCompleted,
				},
			},
			expected: types.Success,
		},
		{
			name: "TimedoutStatus",
			result: []*types.IterationResult{
				{
					Number: 1,
					SuitesResult: map[string]types.TestResults{
						"gpu0": {"action1": types.Success},
					},
					Status: types.TestTimedOut,
				},
			},
			expected: types.Timedout,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tr.getOverallResult(tc.result, validIDs)
			assert.Equal(t, tc.expected, got)
			if tc.check != nil {
				tc.check(t, tc.result)
			}
		})
	}
}
