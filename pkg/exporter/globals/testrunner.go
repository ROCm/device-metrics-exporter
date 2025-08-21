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

package globals

import "time"

const (
	DefaultTestCategory      = "GPU_HEALTH_CHECK"
	DefaultTestTrigger       = "AUTO_UNHEALTHY_GPU_WATCH"
	GlobalTestTriggerKeyword = "global"

	// rvs binary path
	RVSPath = "/opt/rocm/bin/rvs"

	// amd-smi binary path
	AMDSMIPath = "/opt/rocm/bin/amd-smi"

	// rvs test runner configuration file path
	RVSTestCaseDir = "/opt/rocm/share/rocm-validation-suite/conf/"

	// agfhc binary path
	AGFHCPath = "/opt/amd/agfhc/agfhc"

	// agfhc test runner configuration file path
	AGFHCTestCaseDir = "/opt/amd/agfhc/recipes/"

	// AMDTestRunnerCfgPath path to the test runner config
	AMDTestRunnerCfgPath = "/etc/test-runner/config.json"

	LogPrefix = "test-runner "

	GPUStateWatchFreq        = 30 * time.Second // frequency to watch GPU health state from exporter
	GPUStateReqTimeout       = 10 * time.Second // timeout for gRPC request sending to exporter socket
	GPUStateConnRetryFreq    = 5 * time.Second
	GPUStateConnREtryTimeout = 60 * time.Second

	// DefaultResultLogDir directory to save test runner result logs
	DefaultRunnerLogDir     = "/var/log/amd-test-runner"
	DefaultRunnerLogSubPath = "test-runner.log"
	DefaultStatusDBSubPath  = "status.db"

	// NoGPUErrMsg error message when no gpu is detected by amd-smi for manual test trigger
	NoGPUErrMsg = "No GPU detected by amd-smi"

	// test log dir
	TestLogDir = "/var/tmp"

	// rvs team recommended use gst_single as default test recipe
	// these are the default profiles for the test runner
	DefaultUnhealthyGPUTestName                  = "gst_single"
	DefaultUnhealthyGPUTestIterations     uint32 = 1
	DefaultUnhealthyGPUTestStopOnFailure         = true
	DefaultUnhealthyGPUTestTimeoutSeconds uint32 = 3600
	DefaultPreJobCheckTestName                   = "gst_single"
	DefaultPreJobCheckTestIterations      uint32 = 1
	DefaultPreJobCheckTestStopOnFailure          = true
	DefaultPreJobCheckTestTimeoutSeconds  uint32 = 3600
	DefaultManualTestName                        = "gst_single"
	DefaultManualTestIterations           uint32 = 1
	DefaultManualTestStopOnFailure               = true
	DefaultManualTestTimeoutSeconds       uint32 = 3600

	// these are the default values for the missing fields that users didn't provided test configs
	DefaultRVSRecipeName                   = "gst_single"
	DefaultAGFHCRecipeName                 = "all_lvl1"
	DefaultTestCaseTimeoutInSeconds uint32 = 3600
	DefaultIterations               uint32 = 1
	DefaultStopOnFailure                   = true

	// rvs build may use these aliases for MI350X and MI355X test recipes folder names
	MI350XAlias = "gfx950-dlc"
	MI355XAlias = "gfx950"

	EventSourceComponentName = "amd-test-runner"
)

var (
	// reference: https://admin.pci-ids.ucw.cz/read/PC/1002
	GPUDeviceIDToModelName = map[string]string{
		"0x740f": "MI210",
		"0x7410": "MI210", // MI210 VF
		"0x74a0": "MI300A",
		"0x74a1": "MI300X",
		"0x74b5": "MI300X", // MI300X VF
		"0x74a2": "MI308X",
		"0x74a5": "MI325X",
		"0x74b9": "MI325X", // MI325X VF
		"0x74a9": "MI300X-HF",
		"0x74bd": "MI300X-HF",
		"0x75a0": "MI350X",
		"0x75b0": "MI350X", // MI350X VF
		"0x75a3": "MI355X",
		"0x75b3": "MI355X", // MI355X VF
	}
)
