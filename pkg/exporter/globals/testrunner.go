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

	// GPU partitioning related const
	MemoryPartitionTypePrefix  = "GPU_MEMORY_PARTITION_TYPE_"
	ComputePartitionTypePrefix = "GPU_COMPUTE_PARTITION_TYPE_"

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

	// MI350P ships in two TDP variants with separate RVS recipe folders.
	// When all GPUs on the node report a socket power limit >= this threshold,
	// the 600W recipe folder is selected; otherwise the 450W folder is used.
	MI350PHighPowerFolder    = "MI350P-600W"
	MI350PLowPowerFolder     = "MI350P-450W"
	MI350PHighPowerThreshold = 600 // watts

	EventSourceComponentName = "amd-test-runner"
)

var (
	// reference: https://admin.pci-ids.ucw.cz/read/PC/1002
	// https://github.com/amd/MxGPU-Virtualization/blob/staging/libgv/core/amdgv_marketing_name.c
	GPUDeviceIDToModelName = map[string]string{
		// Instinct
		"0x740f": "MI210",
		"0x7410": "MI210", // MI210 VF
		"0x74a0": "MI300A",
		"0x74a1": "MI300X",
		"0x74b5": "MI300X", // MI300X VF
		"0x74a2": "MI308X",
		"0x74b6": "MI308X",    // MI308X VF
		"0x74a8": "MI308X-HF", // MI308X HF
		"0x74bc": "MI308X-HF", // MI308X HF VF
		"0x74a5": "MI325X",
		"0x74b9": "MI325X", // MI325X VF
		"0x74a9": "MI300X-HF",
		"0x74bd": "MI300X-HF", // VF
		"0x75a0": "MI350X",
		"0x75b0": "MI350X", // MI350X VF
		"0x75a3": "MI355X",
		"0x75b3": "MI355X", // MI355X VF
		"0x75a8": "MI350P",
		"0x75b8": "MI350P", // MI350P VF
		// Radeon Pro
		"0x73a3": "nv21",   // Radeon PRO W6800
		"0x7470": "nv32",   // Radeon PRO W7700
		"0x745e": "nv31",   // Radeon PRO W7800
		"0x7449": "nv31",   // Radeon PRO W7800 48GB
		"0x7448": "nv31",   // Radeon PRO W7900
		"0x744a": "nv31",   // Radeon PRO W7900 Dual Slot
		"0x744b": "nv31",   // Radeon PRO W7900D
		"0x7551": "R9600D", // Radeon AI PRO 9600D/9700/9700S
		// Radeon
		"0x73bf": "nv21",   // Radeon RX 6800/6800XT/6900XT
		"0x73af": "nv21",   // Radeon RX 6900XT
		"0x73a5": "nv21",   // Radeon RX 6950XT
		"0x747e": "nv32",   // Radeon RX 7700/7700XT/7800XT/7800M
		"0x744c": "nv31",   // Radeon RX 7900XT/7900XTX/7900GRE/7900M
		"0x7590": "RX9060", // Radeon RX 9060/9060XT/9060XT(8GB)
		"0x7550": "RX9070", // Radeon RX 9070/9070XT
	}
)
