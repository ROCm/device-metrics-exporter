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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	testrunnerGen "github.com/ROCm/device-metrics-exporter/pkg/testrunner/gen/testrunner"
	types "github.com/ROCm/device-metrics-exporter/pkg/testrunner/interface"
)

var (
	defaultGlobalTestRunnerConfig = &testrunnerGen.TestRunnerConfig{
		TestConfig: map[string]*testrunnerGen.TestCategoryConfig{
			testrunnerGen.TestCategory_GPU_HEALTH_CHECK.String(): {
				TestLocationTrigger: map[string]*testrunnerGen.TestTriggerConfig{
					globals.GlobalTestTriggerKeyword: {
						TestParameters: map[string]*testrunnerGen.TestParameters{
							testrunnerGen.TestTrigger_AUTO_UNHEALTHY_GPU_WATCH.String(): {
								TestCases: []*testrunnerGen.TestParameter{
									{
										Framework:      testrunnerGen.TestParameter_RVS.String(),
										Recipe:         globals.DefaultUnhealthyGPUTestName,
										Iterations:     globals.DefaultUnhealthyGPUTestIterations,
										StopOnFailure:  globals.DefaultUnhealthyGPUTestStopOnFailure,
										TimeoutSeconds: globals.DefaultUnhealthyGPUTestTimeoutSeconds,
									},
								},
							},
							testrunnerGen.TestTrigger_PRE_START_JOB_CHECK.String(): {
								TestCases: []*testrunnerGen.TestParameter{
									{
										Framework:      testrunnerGen.TestParameter_RVS.String(),
										Recipe:         globals.DefaultPreJobCheckTestName,
										Iterations:     globals.DefaultPreJobCheckTestIterations,
										StopOnFailure:  globals.DefaultPreJobCheckTestStopOnFailure,
										TimeoutSeconds: globals.DefaultPreJobCheckTestTimeoutSeconds,
									},
								},
							},
							testrunnerGen.TestTrigger_MANUAL.String(): {
								TestCases: []*testrunnerGen.TestParameter{
									{
										Framework:      testrunnerGen.TestParameter_RVS.String(),
										Recipe:         globals.DefaultManualTestName,
										Iterations:     globals.DefaultManualTestIterations,
										StopOnFailure:  globals.DefaultManualTestStopOnFailure,
										TimeoutSeconds: globals.DefaultManualTestTimeoutSeconds,
									},
								},
							},
						},
					},
				},
			},
		},
	}
)

type TestRunner struct {
	hostName           string
	rvsPath            string
	amdSMIPath         string
	exporterSocketPath string

	// agfhc related fields
	agfhcPath        string
	agfhcTestCaseDir string

	testCategory        string
	testLocation        string
	testTrigger         string
	rvsTestCaseDir      string
	testCfgPath         string
	testCfgGPUModelName string
	gpuIndexToKFDIDMap  map[string]string
	kfdIDToGPUIndexMap  map[string]string

	logDir       string
	statusDBPath string

	jobName  string
	nodeName string

	sync.Mutex             // mutex to protect globalTestRunnerConfig from file watcher
	globalTestRunnerConfig *testrunnerGen.TestRunnerConfig
	testRunnerIntf         types.TestRunner

	// k8s related fields
	isK8s           bool
	k8sClient       *k8sclient.K8sClient
	k8sPodName      string
	k8sPodNamespace string
}

// initTestRunner init the test runner and related configs
// return the test location, either global or specific host name
func NewTestRunner(rvsPath, rvsTestCaseDir, agfhcPath, agfhcTestCaseDir, amdSMIPath, exporterSocketPath, testRunnerConfigPath, testCategory, testTrigger, logDir, jobName, nodeName string) *TestRunner {
	runner := &TestRunner{
		rvsPath:            rvsPath,
		amdSMIPath:         amdSMIPath,
		exporterSocketPath: exporterSocketPath,
		testCategory:       testCategory,
		testTrigger:        testTrigger,
		testCfgPath:        testRunnerConfigPath,
		rvsTestCaseDir:     rvsTestCaseDir,
		agfhcPath:          agfhcPath,
		agfhcTestCaseDir:   agfhcTestCaseDir,
		gpuIndexToKFDIDMap: map[string]string{},
		kfdIDToGPUIndexMap: map[string]string{},
		logDir:             logDir,
		jobName:            jobName,
		nodeName:           nodeName,
	}
	// init test runner config
	// testRunnerConfigPath file existence has been verified
	runner.initLogger()
	runner.readTestRunnerConfig(testRunnerConfigPath)
	runner.getHostName()
	if utils.IsKubernetes() {
		runner.isK8s = true
		k8sClient, err := k8sclient.NewClient(context.Background(), runner.hostName)
		if err != nil {
			logger.Log.Printf("failed to create k8s client: %v", err)
			logger.Log.Fatal(err)
		}
		runner.k8sClient = k8sClient
	}
	runner.validateTestTrigger()
	runner.initTestRunnerConfig()
	logger.Log.Printf("Test runner isKubernetes: %+v config: %+v", runner.isK8s, runner.globalTestRunnerConfig)
	return runner
}

// validateTestTrigger validates the test category/location/trigger existence
// return test locaiton, either global or specific hostname
func (tr *TestRunner) validateTestTrigger() {
	tr.Lock()
	defer tr.Unlock()

	// 1. verify test category
	// given category config should exist
	if tr.globalTestRunnerConfig.TestConfig == nil {
		fmt.Printf("failed to find any test category config from %+v\n", tr.globalTestRunnerConfig)
		os.Exit(1)
	}
	if _, ok := tr.globalTestRunnerConfig.TestConfig[tr.testCategory]; !ok {
		fmt.Printf("failed to find category %+v from config %+v\n", tr.testCategory, tr.globalTestRunnerConfig)
		os.Exit(1)
	}

	// 2. verify test location
	// global config or given hostname's config should exist
	categoryConfig := tr.globalTestRunnerConfig.TestConfig[tr.testCategory]
	if categoryConfig == nil {
		fmt.Printf("got empty config for test category %+v", tr.testCategory)
		os.Exit(1)
	}

	if categoryConfig.TestLocationTrigger == nil {
		fmt.Printf("failed to find any global or host specific test config under category %+v: %+v\n", tr.testCategory, categoryConfig)
		os.Exit(1)
	}
	_, foundHostSpecificTest := categoryConfig.TestLocationTrigger[tr.hostName]
	_, foundGlobalTest := categoryConfig.TestLocationTrigger[globals.GlobalTestTriggerKeyword]
	if !foundGlobalTest && !foundHostSpecificTest {
		fmt.Printf("cannot find neither global test config nor host specific config under category %+v: %+v\n", tr.testCategory, categoryConfig)
		tr.generateK8sEvent("", v1.EventTypeWarning,
			testrunnerGen.TestEventReason_TestConfigError.String(), nil,
			fmt.Sprintf("failed to find global or node %+v specific trigger for test category %+v", tr.hostName, tr.testCategory), []string{})
		os.Exit(1)
	}

	// 3. validate test trigger's config
	// if host specific config was found
	// validate host specific config's trigger
	if foundHostSpecificTest {
		if categoryConfig.TestLocationTrigger[tr.hostName].TestParameters == nil {
			fmt.Printf("failed to get any test trigger under category %+v config: %+v\n", categoryConfig, categoryConfig.TestLocationTrigger[tr.hostName])
			os.Exit(1)
		}
		if params, ok := categoryConfig.TestLocationTrigger[tr.hostName].TestParameters[tr.testTrigger]; !ok {
			fmt.Printf("failed to get test trigger %+v under category %+v config: %+v\n", tr.testTrigger, categoryConfig, categoryConfig.TestLocationTrigger[tr.hostName])
			os.Exit(1)
		} else if len(params.TestCases) == 0 || params.TestCases[0] == nil {
			fmt.Printf("failed to get test case under category %+v trigger %+v config: %+v\n", categoryConfig, tr.testTrigger, categoryConfig.TestLocationTrigger[tr.hostName])
			os.Exit(1)
		}
		tr.testLocation = tr.hostName
	} else {
		// if host specific config was not found
		// validate global config's trigger
		if categoryConfig.TestLocationTrigger[globals.GlobalTestTriggerKeyword].TestParameters == nil {
			fmt.Printf("failed to get any test trigger under category %+v global config: %+v\n", categoryConfig, categoryConfig.TestLocationTrigger[tr.hostName])
			os.Exit(1)
		}
		if params, ok := categoryConfig.TestLocationTrigger[globals.GlobalTestTriggerKeyword].TestParameters[tr.testTrigger]; !ok {
			fmt.Printf("failed to get test trigger %+v under category %+v global config: %+v\n", tr.testTrigger, categoryConfig, categoryConfig.TestLocationTrigger[tr.hostName])
			os.Exit(1)
		} else if len(params.TestCases) == 0 || params.TestCases[0] == nil {
			fmt.Printf("failed to get test case under category %+v trigger %+v global config: %+v\n", categoryConfig, tr.testTrigger, categoryConfig.TestLocationTrigger[tr.hostName])
			os.Exit(1)
		}
		tr.testLocation = globals.GlobalTestTriggerKeyword
	}
	logger.Log.Printf("applied test config for %+v", tr.testLocation)

	// 4. validate specific GPU model's test recipe
	testParams := tr.getTestParameters(false)
	switch strings.ToUpper(testParams.TestCases[0].Framework) {
	case testrunnerGen.TestParameter_RVS.String():
		gpuModelSubDir, err := getGPUModelTestRecipeDir(tr.amdSMIPath)
		if err != nil {
			logger.Log.Printf("failed to get GPU model specific folder for rvs test recipe err %+v, using recipe from root conf folder", err)
		}

		testCfgPath := filepath.Join(tr.rvsTestCaseDir, testParams.TestCases[0].Recipe+".conf")
		if gpuModelSubDir != "" {
			// special handling for RVS
			// RVS build may use alias for MI350X and MI355X
			// MI350X: gfx950-dlc
			// MI355X: gfx950
			switch gpuModelSubDir {
			case "MI350X":
				// if MI350X subdir does not exist
				// try to use gfx950-dlc alias
				if _, err := os.Stat(filepath.Join(tr.rvsTestCaseDir, gpuModelSubDir)); err != nil {
					logger.Log.Printf("failed to find recipe folder for MI350X, trying gfx950-dlc alias")
					gpuModelSubDir = globals.MI350XAlias
				}
			case "MI355X":
				// if MI355X subdir does not exist
				// try to use gfx950 alias
				if _, err := os.Stat(filepath.Join(tr.rvsTestCaseDir, gpuModelSubDir)); err != nil {
					logger.Log.Printf("failed to find recipe folder for MI355X, trying gfx950 alias")
					gpuModelSubDir = globals.MI355XAlias
				}
			}
			if _, err := os.Stat(filepath.Join(tr.rvsTestCaseDir, gpuModelSubDir)); err != nil {
				logger.Log.Printf("failed to find recipe folder for GPU model %+v, using model-agnostic recipes", gpuModelSubDir)
				gpuModelSubDir = ""
			} else {
				logger.Log.Printf("using test recipe from %+v folder", gpuModelSubDir)
				tr.testCfgGPUModelName = gpuModelSubDir
			}
			// Always assign testCfgPath based on possibly updated gpuModelSubDir
			testCfgPath = filepath.Join(tr.rvsTestCaseDir, gpuModelSubDir, testParams.TestCases[0].Recipe+".conf")
		}
		if _, err := os.Stat(testCfgPath); err != nil {
			fmt.Printf("Trigger %+v cannot find corresponding test config file %+v, err: %+v\n", tr.testTrigger, testCfgPath, err)
			tr.generateK8sEvent("", v1.EventTypeWarning,
				testrunnerGen.TestEventReason_TestConfigError.String(), nil,
				fmt.Sprintf("failed to find test recipe %+v", testCfgPath), []string{})
			os.Exit(1)
		}
	case testrunnerGen.TestParameter_AGFHC.String():
		gpuModelSubDir, err := getGPUModelTestRecipeDir(tr.amdSMIPath)
		if err != nil || gpuModelSubDir == "" {
			logger.Log.Printf("failed to get GPU model specific folder for agfhc test recipe err %+v", err)
			tr.generateK8sEvent("", v1.EventTypeWarning,
				testrunnerGen.TestEventReason_TestConfigError.String(), nil,
				"failed to find AGFHC test recipes folder for current GPU model, please check test runner and AGFHC docs for supported GPU models", []string{})
			os.Exit(1)
		}

		// agfhc uses lower case dir names for GPU models
		gpuModelSubDir = strings.ToLower(gpuModelSubDir)
		logger.Log.Printf("using test recipe from %+v folder", gpuModelSubDir)
		tr.testCfgGPUModelName = gpuModelSubDir
		testCfgPath := filepath.Join(tr.agfhcTestCaseDir, gpuModelSubDir, testParams.TestCases[0].Recipe+".yml")

		if _, err := os.Stat(testCfgPath); err != nil {
			fmt.Printf("Trigger %+v cannot find corresponding test config file %+v, err: %+v\n", tr.testTrigger, testCfgPath, err)
			tr.generateK8sEvent("", v1.EventTypeWarning,
				testrunnerGen.TestEventReason_TestConfigError.String(), nil,
				fmt.Sprintf("failed to find test recipe %+v", testCfgPath), []string{})
			os.Exit(1)
		}
	}

	if testParams.TestCases[0].Iterations == 0 {
		fmt.Printf("Trigger %+v has been configured to run with 0 iteration, should be non-zero iterations\n", tr.testTrigger)
		tr.generateK8sEvent("", v1.EventTypeWarning,
			testrunnerGen.TestEventReason_TestConfigError.String(), nil,
			fmt.Sprintf("unable to execute test with Iterations == 0 test parameters: %+v", testParams.TestCases[0]), []string{})
		os.Exit(1)
	}
	if testParams.TestCases[0].TimeoutSeconds == 0 {
		fmt.Printf("Trigger %+v has been configured to run with 0 TimeoutSeconds, should be non-zero TimeoutSeconds\n", tr.testTrigger)
		tr.generateK8sEvent("", v1.EventTypeWarning,
			testrunnerGen.TestEventReason_TestConfigError.String(), nil,
			fmt.Sprintf("unable to execute test with TimeoutSeconds == 0 test parameters: %+v", testParams.TestCases[0]), []string{})
		os.Exit(1)
	}
}

func (tr *TestRunner) initLogger() {
	logger.SetLogDir(tr.logDir)
	logger.SetLogFile(globals.DefaultRunnerLogSubPath)
	logger.SetLogPrefix(globals.LogPrefix)
	logger.Init(utils.IsKubernetes())
}

// readTestRunnerConfig try to user provided customized test runner config from given file
func (tr *TestRunner) readTestRunnerConfig(configPath string) {
	tr.Lock()
	defer tr.Unlock()

	defer func() {
		tr.normalizeConfig()
	}()

	file, err := os.Open(configPath)
	if err != nil {
		tr.globalTestRunnerConfig = defaultGlobalTestRunnerConfig
		logger.Log.Printf("cannot read provided test runner config at %+v, err: %+v, using default test runner config", configPath, err)
		return
	}
	defer file.Close()
	bytes, err := io.ReadAll(file)
	if err != nil {
		tr.globalTestRunnerConfig = defaultGlobalTestRunnerConfig
		logger.Log.Printf("cannot read provided test runner config at %+v, err: %+v, using default test runner config", configPath, err)
		return
	}
	var config testrunnerGen.TestRunnerConfig
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		tr.globalTestRunnerConfig = defaultGlobalTestRunnerConfig
		logger.Log.Printf("cannot read provided test runner config at %+v, err: %+v, using default test runner config", configPath, err)
		return
	}
	tr.globalTestRunnerConfig = &config
	tr.setConfigDefaults()
}

func (tr *TestRunner) setConfigDefaults() {
	if tr.globalTestRunnerConfig == nil {
		return
	}
	for _, categoryConfig := range tr.globalTestRunnerConfig.TestConfig {
		if categoryConfig == nil {
			continue
		}
		for _, triggerConfig := range categoryConfig.TestLocationTrigger {
			if triggerConfig == nil {
				continue
			}
			for _, params := range triggerConfig.TestParameters {
				if params == nil {
					continue
				}
				for _, testCase := range params.TestCases {
					if testCase == nil {
						continue
					}
					// set default values for framework for backward compatibility
					if testCase.Framework == "" {
						testCase.Framework = testrunnerGen.TestParameter_RVS.String()
					}
				}
			}
		}
	}
}

func (tr *TestRunner) initTestRunnerConfig() {
	if tr.logDir == "" {
		tr.logDir = globals.DefaultRunnerLogDir
	}

	// init test runner log
	err := os.MkdirAll(tr.logDir, 0755)
	if err != nil {
		fmt.Printf("Failed to create dir for test runner logs %+v, err: %+v\n", tr.logDir, err)
		os.Exit(1)
	}

	// init status db
	// don't try to create if status db already exists
	// test runner needs to read the existing db and rerun incomplete test before crash/restart
	statusDBPath := filepath.Join(tr.logDir, globals.DefaultStatusDBSubPath)
	if _, err := os.Stat(statusDBPath); err != nil && os.IsNotExist(err) {
		_, err = os.Create(statusDBPath)
		if err != nil {
			fmt.Printf("Failed to create test status db %+v, err: %+v\n", statusDBPath, err)
			os.Exit(1)
		}
		runnerStatus := &testrunnerGen.TestRunnerStatus{
			TestStatus: map[string]string{},
		}
		err = SaveRunnerStatus(runnerStatus, statusDBPath)
		if err != nil {
			fmt.Printf("Failed to init test runner status db %+v, err: %+v\n", statusDBPath, err)
			os.Exit(1)
		}
	}
	tr.statusDBPath = statusDBPath
}

func (tr *TestRunner) getTestRecipeDir(runnerType types.TestRunnerType) string {
	dir := ""
	switch runnerType {
	case types.RVSRunner:
		dir = tr.rvsTestCaseDir
	case types.AGFHCRunner:
		dir = tr.agfhcTestCaseDir
	}
	return filepath.Join(dir, tr.testCfgGPUModelName)
}

func (tr *TestRunner) generateKFDIDMapping() error {
	var err error
	for i := 0; i < 5; i++ {
		cmd := exec.Command(tr.amdSMIPath, "list", "--json")
		output, err := cmd.Output()
		if err != nil {
			logger.Log.Printf("cannot execute command: amd-smi list --json, err: %+v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Parse the JSON response
		var result []map[string]interface{}
		err = json.Unmarshal(output, &result)
		if err != nil {
			logger.Log.Printf("cannot unmarshal amd-smi output: %+v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, gpuInfo := range result {
			gpuIndexIntf, ok1 := gpuInfo["gpu"]
			kfdIDIntf, ok2 := gpuInfo["kfd_id"]
			if !ok1 || !ok2 {
				logger.Log.Printf("failed to find both gpu index and KFD ID from amd-smi output %+v", gpuInfo)
				continue
			}
			gpuIndex, ok1 := gpuIndexIntf.(float64)
			kfdID, ok2 := kfdIDIntf.(float64)
			if !ok1 || !ok2 {
				logger.Log.Printf("failed to convert both gpu index %+v %T (%+v) and KFD ID %+v %T (%+v) to float64",
					gpuIndexIntf, gpuIndexIntf, ok1, kfdIDIntf, kfdIDIntf, ok2)
				continue
			}
			tr.gpuIndexToKFDIDMap[fmt.Sprintf("%v", gpuIndex)] = fmt.Sprintf("%v", kfdID)
			tr.kfdIDToGPUIndexMap[fmt.Sprintf("%v", kfdID)] = fmt.Sprintf("%v", gpuIndex)
		}

		logger.Log.Printf("generated GPU index to KFD ID mapping, amd-smi output: %+v map: %+v", result, tr.gpuIndexToKFDIDMap)
		return nil
	}
	return fmt.Errorf("after all attempts still cannot get and parse GPU index and KFD ID mapping, last error: %+v", err)
}

func (tr *TestRunner) convertIndexesToKFDIDs(indexes []string) []string {
	kfdIDs := []string{}
	for _, index := range indexes {
		kfdID, ok := tr.gpuIndexToKFDIDMap[index]
		if !ok {
			logger.Log.Printf("failed to get KFD ID for index %+v from map %+v", index, tr.gpuIndexToKFDIDMap)
			continue
		}
		kfdIDs = append(kfdIDs, kfdID)
	}
	// if users specified a list of GPU index to test
	// but none of them is available
	// don't return an empty list here since it is going to run test on all GPUs
	// fail the container here
	if len(indexes) > 0 && len(kfdIDs) == 0 {
		logger.Log.Printf("error looking for KFD ID from all provided GPU indexes %+v, exiting...", indexes)
		os.Exit(1)
	}
	return kfdIDs
}

func (tr *TestRunner) convertKFDIDsToIndexes(kfdIDs []string) []string {
	indexes := []string{}
	for _, kfdID := range kfdIDs {
		index, ok := tr.kfdIDToGPUIndexMap[kfdID]
		if !ok {
			logger.Log.Printf("failed to get index for KFD ID %+v from map %+v", kfdID, tr.kfdIDToGPUIndexMap)
			continue
		}
		indexes = append(indexes, index)
	}
	// if users specified a list of GPU index to test
	// but none of them is available
	// don't return an empty list here since it is going to run test on all GPUs
	// fail the container here
	if len(kfdIDs) > 0 && len(indexes) == 0 {
		logger.Log.Printf("error looking for indexes from all provided GPU kfd ids %+v, exiting...", kfdIDs)
		os.Exit(1)
	}
	return indexes
}

// the validation functions have make sure that the given category/location/trigger config exists and valid within runnerConfig
// this function will be responsible to trigger the test
func (tr *TestRunner) TriggerTest() {
	switch tr.testCategory {
	case testrunnerGen.TestCategory_GPU_HEALTH_CHECK.String():
		if err := tr.generateKFDIDMapping(); err != nil {
			logger.Log.Printf("failed to get and parse GPU index and kfd id mapping: %+v", err)
			os.Exit(1)
		}
		// handle upgrade case ; convert existing entries with KFD ID to use GPU index
		if err := transformRunnerStatus(tr.statusDBPath, tr.kfdIDToGPUIndexMap, tr.gpuIndexToKFDIDMap); err != nil {
			logger.Log.Printf("failed to transform runner status db %+v", err)
			os.Exit(1)
		}

		switch tr.testTrigger {
		case testrunnerGen.TestTrigger_AUTO_UNHEALTHY_GPU_WATCH.String():
			// init rvs test runner
			// and start to listen for unix socket to receive the event
			// for triggering the test run on unhealthy GPU
			tr.setTestRunner()
			tr.watchGPUState()
		case testrunnerGen.TestTrigger_MANUAL.String(),
			testrunnerGen.TestTrigger_PRE_START_JOB_CHECK.String():
			tr.setTestRunner()
			tr.manualTestGPU()
		default:
			logger.Log.Printf("unsupported test trigger %+v for category %+v", tr.testTrigger, tr.testCategory)
			os.Exit(1)
		}
	}
}

func (tr *TestRunner) setTestRunner() {
	testParams := tr.getTestParameters(true)

	var runnerType types.TestRunnerType
	var binPath string
	switch strings.ToUpper(testParams.TestCases[0].Framework) {
	case testrunnerGen.TestParameter_RVS.String():
		runnerType = types.RVSRunner
		binPath = tr.rvsPath
	case testrunnerGen.TestParameter_AGFHC.String():
		runnerType = types.AGFHCRunner
		binPath = tr.agfhcPath
	default:
		logger.Log.Printf("unsupported test framework %v for category %v, location %v, trigger %v", testParams.TestCases[0].Framework, tr.testCategory, tr.testLocation, tr.testTrigger)
		tr.generateK8sEvent("", v1.EventTypeWarning,
			testrunnerGen.TestEventReason_TestConfigError.String(), nil,
			fmt.Sprintf("cannot execute unsupported test framework %+v", testParams.TestCases[0].Framework), []string{})
		os.Exit(1)
	}

	runner, err := NewTestRunnerIntf(runnerType, binPath, tr.getTestRecipeDir(runnerType), globals.TestLogDir)
	if err != nil || runner == nil {
		logger.Log.Printf("failed to create %v test runner; err: %v", runnerType, err)
		os.Exit(1)
	}
	tr.testRunnerIntf = runner
}

func (tr *TestRunner) watchGPUState() {
	ticker := time.NewTicker(globals.GPUStateConnRetryFreq)
	defer ticker.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), globals.GPUStateConnREtryTimeout)
	defer cancel()

	var err error
	var conn *grpc.ClientConn
	connected := false

	for !connected {
		select {
		case <-ticker.C:
			conn, err = grpc.NewClient("unix:"+tr.exporterSocketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				logger.Log.Printf("testrunner cannot connect to %v: %v", "unix:"+tr.exporterSocketPath, err)
				continue
			}
			connected = true
			defer conn.Close()
		case <-ctx.Done():
			logger.Log.Fatalf("retry exhausted: testrunner cannot connect to %v", "unix:"+tr.exporterSocketPath)
		}
	}

	c := metricssvc.NewMetricsServiceClient(conn)
	watchTicker := time.NewTicker(globals.GPUStateWatchFreq)
	defer watchTicker.Stop()

	// handle test runner crash or restart
	// read existing test runner status db
	// immediately start test on interrupted test before restarting
	statusObj, _ := LoadRunnerStatus(tr.statusDBPath)
	ids := []string{}
	if statusObj != nil && len(statusObj.TestStatus) > 0 {
		updateStatusDB := false
		for deviceID, status := range statusObj.TestStatus {
			// check whether the deviceID has expired in the map or not
			// it is possible that the GPU has been partitioned and triggered test but got cutoff in between
			// after the restart of test runner the GPU is no longer partitioned
			// then the pre-existing test status info of partitioned deviceID is no longer existing
			// need to remove those expired deviceIDs from status DB
			// otherwise the SMI lib keeps cannot retrieve information of those expired deviceIDs
			if _, ok := tr.kfdIDToGPUIndexMap[deviceID]; !ok {
				delete(statusObj.TestStatus, deviceID)
				logger.Log.Printf("removing expired deviceID %v from status DB", deviceID)
				updateStatusDB = true
				continue
			}
			if status == types.TestRunning.String() {
				ids = append(ids, deviceID)
			}
		}
		if updateStatusDB {
			// remove expired deviceIDs from status DB if needed
			SaveRunnerStatus(statusObj, tr.statusDBPath)
		}
		if len(ids) > 0 {
			logger.Log.Printf("found GPU %+v with incomplete test before restart %+v, start to rerun test", ids, statusObj)
			go tr.testGPU(testrunnerGen.TestTrigger_AUTO_UNHEALTHY_GPU_WATCH.String(), ids, true)
		}
	}

	go tr.watchConfigFile()
	for range watchTicker.C {
		ctx, cancel := context.WithTimeout(context.Background(), globals.GPUStateReqTimeout)
		r, err := c.List(ctx, &emptypb.Empty{})
		if err != nil {
			logger.Log.Printf("could not list GPU state: %v", err)
			cancel()
			continue
		}
		logger.Log.Printf("GPU State: %s", r.String())
		cancel()

		healthyGPUIDs := []string{}
		unHealthyGPUIDs := []string{}
		if r != nil {
			for _, state := range r.GPUState {
				// if any GPU is not healthy, start a test against those GPUs
				if !strings.EqualFold(state.Health, metricssvc.GPUHealth_HEALTHY.String()) {
					if len(state.AssociatedWorkload) == 0 {
						unHealthyGPUIDs = append(unHealthyGPUIDs, state.ID)
					} else {
						logger.Log.Printf("found GPU %+v unhealthy but still associated with workload %+v", state.ID, state.AssociatedWorkload)
					}
				} else {
					healthyGPUIDs = append(healthyGPUIDs, state.ID)
				}
			}
		}

		// start test on unhealthy GPU
		if len(unHealthyGPUIDs) > 0 {
			logger.Log.Printf("found GPU with unhealthy state %+v", unHealthyGPUIDs)
			go tr.testGPU(testrunnerGen.TestTrigger_AUTO_UNHEALTHY_GPU_WATCH.String(), unHealthyGPUIDs, false)
		} else {
			logger.Log.Printf("all GPUs are healthy or associated with workloads, skip testing")
		}

		tr.cleanupHealthyGPUTestStatus(healthyGPUIDs)
	}
}

func (tr *TestRunner) watchConfigFile() {
	// if config file doesn't exist, create dir in case it doesn't exist
	// so that fsnotify file watcher won't fail to init the watcher
	directory := path.Dir(tr.testCfgPath)
	if err := os.MkdirAll(directory, 0755); err != nil {
		logger.Log.Fatal(err)
	}
	logger.Log.Printf("starting file watcher for %v", directory)

	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Log.Fatal(err)
	}
	defer watcher.Close()
	ctx := context.Background()
	// Start listening for events.
	go func() {
		for ctx.Err() == nil {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// k8s has to many cases to handle because of symlink, to be
				// safe handle all cases
				if event.Has(fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename) {
					logger.Log.Printf("loading new config on %v", tr.testCfgPath)
					tr.readTestRunnerConfig(tr.testCfgPath)
					tr.validateTestTrigger()
					logger.Log.Printf("Test runner isKubernetes: %+v config: %+v", tr.isK8s, tr.globalTestRunnerConfig)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					logger.Log.Printf("error watching for config file: %v", err)
					return
				}
			}
		}
	}()

	// Add a path.
	err = watcher.Add(directory)
	if err != nil {
		logger.Log.Printf("failed to start the config file watcher err %+v", err)
		log.Fatal(err)
	}

	<-make(chan struct{})
}

func (tr *TestRunner) cleanupHealthyGPUTestStatus(ids []string) {
	// for healthy GPU
	// check if there is test status cached
	// 1. if there is test already running
	// don't interrupt the running test
	// 2. if there is test completed
	// remove the status so that next time it turns unhealthy, test will be triggered again
	statusObj, _ := LoadRunnerStatus(tr.statusDBPath)
	writeBack := false
	if statusObj != nil && statusObj.TestStatus != nil {
		for _, healthyID := range ids {
			if status, ok := statusObj.TestStatus[healthyID]; ok && status != types.TestRunning.String() {
				delete(statusObj.TestStatus, healthyID)
				writeBack = true
			}
		}
	} else {
		statusObj = &testrunnerGen.TestRunnerStatus{}
		writeBack = true
	}
	if writeBack {
		if err := SaveRunnerStatus(statusObj, tr.statusDBPath); err != nil {
			logger.Log.Printf("Error saving runner status: %+v", err)
		}
	}
}

// testGPU is the function to manipulate the handler to run test and report test result
// ids is a list of KFD IDs
func (tr *TestRunner) testGPU(trigger string, ids []string, isRerun bool) {
	parameters := tr.getTestParameters(true)
	// load ongoing test status
	// avoid run multiple test on the same device
	// validIDs will be the list of KFD IDs as the test run parameter
	validIDs, statusObj := removeIDsWithExistingTest(trigger, tr.statusDBPath, ids, parameters, isRerun)
	if isRerun {
		// for rerun after test runner restart
		// we need to force to run the incomplete test
		// ignore the status db temporarily
		validIDs = ids
	}
	if len(ids) > 0 && len(validIDs) == 0 {
		// all original target devices have existing running test, skip for now
		return
	}
	// if both len(ids) and len(validIDs) are 0
	// that means all devices were selected

	var extraArgs []string
	if parameters.TestCases[0].Arguments != "" {
		extraArgs = strings.Split(parameters.TestCases[0].Arguments, ",")
	}
	handler, err := tr.testRunnerIntf.GetTestHandler(parameters.TestCases[0].Recipe, types.TestParams{
		Iterations:    uint(parameters.TestCases[0].Iterations),
		StopOnFailure: parameters.TestCases[0].StopOnFailure,
		DeviceIDs:     validIDs,
		Timeout:       uint(parameters.TestCases[0].TimeoutSeconds),
		ExtraArgs:     extraArgs,
	})
	if err != nil {
		logger.Log.Fatalf("failed to get test run handler, err: %+v", err)
	}

	err = handler.StartTest()
	if err != nil {
		logger.Log.Fatalf("failed to start test run, err: %+v", err)
	}

	if len(validIDs) == 0 {
		// all devices were selected
		validIDs = tr.GetAllGPUIndexes()
	}
	for _, id := range validIDs {
		statusObj.TestStatus[id] = types.TestRunning.String()
	}

	err = SaveRunnerStatus(statusObj, tr.statusDBPath)
	if err != nil {
		logger.Log.Printf("Error saving runner status: %v", err)
		// TODO: add error handling here if new running status failed to be saved
	}

	tr.AddTestRunningLabel(parameters.TestCases[0].Recipe, validIDs)
	defer tr.RemoveTestRunningLabel(parameters.TestCases[0].Recipe, validIDs)

	select {
	case <-time.After(time.Duration(parameters.TestCases[0].TimeoutSeconds) * time.Second * time.Duration(parameters.TestCases[0].Iterations)):
		logger.Log.Printf("Trigger: %v Test: %v GPU Indexes: %v timeout", trigger, parameters.TestCases[0].Recipe, validIDs)
		result := handler.Result()
		result = AppendTimedoutTestSummary(result, validIDs)
		handler.StopTest()
		// when the test timedout
		// save whatever test console logs that are cached
		tr.saveAndExportHandlerLogs(handler, ids, parameters.TestCases[0].Recipe, validIDs)
		tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestTimedOut.String(), result, "", validIDs)
		// exit on non-auto trigger's failure
		tr.exitOnFailure()
	case <-handler.Done():
		// TODO: this has to change later based on result logs parsing.
		// for now updating same result in all GPU
		result := handler.Result()
		logger.Log.Printf("Trigger: %v Test: %v GPU Indexes: %v completed. Result: %v", trigger, parameters.TestCases[0].Recipe, validIDs, result)

		// save log into gzip file
		tr.saveAndExportHandlerLogs(handler, ids, parameters.TestCases[0].Recipe, validIDs)

		switch tr.getOverallResult(result, validIDs) {
		case types.Success:
			tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeNormal, testrunnerGen.TestEventReason_TestPassed.String(), result, "", validIDs)
		case types.Failure:
			tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestFailed.String(), result, "", validIDs)
			// exit on non-auto trigger's failure
			tr.exitOnFailure()
		case types.Timedout:
			tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestTimedOut.String(), result, "", validIDs)
			// exit on non-auto trigger's failure
			tr.exitOnFailure()
		}
	}

	statusObj, _ = LoadRunnerStatus(tr.statusDBPath)
	for _, id := range validIDs {
		switch tr.testTrigger {
		case testrunnerGen.TestTrigger_MANUAL.String(),
			testrunnerGen.TestTrigger_PRE_START_JOB_CHECK.String():
			// the status db is for internal usage only
			// for MANUAL and PRE_START_JOB_CHECK test trigger
			// remove the device id from status db once the test was completed
			// so that the next time the device won't be recognized with incomplete test
			delete(statusObj.TestStatus, id)
		case testrunnerGen.TestTrigger_AUTO_UNHEALTHY_GPU_WATCH.String():
			// the status db is for internal usage only
			// for AUTO_UNHEALTHY_GPU_WATCH just mark all finished test as completed
			// so that there won't be another test happened on the same unhealthy device
			// the test completed status will be removed if device becomes healthy again
			statusObj.TestStatus[id] = types.TestCompleted.String()
		}
	}
	if err := SaveRunnerStatus(statusObj, tr.statusDBPath); err != nil {
		logger.Log.Fatalf("Error saving runner status: %v", err)
	}
}

func (tr *TestRunner) saveAndExportHandlerLogs(handler types.TestHandlerInterface, ids []string, recipe string, gpuIndexes []string) {
	for _, res := range handler.Result() {
		var filesToExport []string
		resultsJson, resultDir, err := tr.testRunnerIntf.ExtractLogLocation(res.Stdout)
		if err != nil {
			logger.Log.Printf("Unable to locate results json file")
		}
		now := time.Now().UTC()
		timestamp := now.Format("2006-01-02T15-04-05.000000Z")
		if resultsJson != "" {
			if err := GzipResultJson(resultsJson, GetLogFilePath(tr.logDir, timestamp, tr.testTrigger, recipe, "result")); err != nil {
				logger.Log.Printf("Unable to save results json gzip file")
			}
			filesToExport = append(filesToExport, resultsJson)
		}
		if res.Stdout != "" {
			stdoutFilePath := GetLogFilePath(tr.logDir, timestamp, tr.testTrigger, recipe, "stdout")
			SaveTestResultToGz(res.Stdout, stdoutFilePath)
			filesToExport = append(filesToExport, stdoutFilePath)
		}
		if res.Stderr != "" {
			stderrFilePath := GetLogFilePath(tr.logDir, timestamp, tr.testTrigger, recipe, "stderr")
			SaveTestResultToGz(res.Stderr, stderrFilePath)
			filesToExport = append(filesToExport, stderrFilePath)
		}

		switch tr.testRunnerIntf.(type) {
		case *AgfhcTestRunner:
			// if the result directory was generated by AGFHC
			// and was not empty
			if resultDir != "" {
				agfhcGzipFilePath := GetLogFilePath(tr.logDir, timestamp, tr.testTrigger, recipe, "agfhc_all.tar")
				if err := GzipFolder(resultDir, agfhcGzipFilePath); err != nil {
					logger.Log.Printf("failed to gzip the AGFHC result directory %v: %v", resultDir, err)
				} else {
					filesToExport = append(filesToExport, agfhcGzipFilePath)
				}
			}
		}

		if len(filesToExport) == 0 {
			continue
		}
		cloudFileName := timestamp + ".tar.gz"
		localCombinedTar := filepath.Join(globals.TestLogDir, cloudFileName)
		err = CreateTarFile(localCombinedTar, filesToExport)
		// export the logs to cloud provider
		if err == nil {
			cloudFolderPath := filepath.Join(tr.testTrigger, tr.jobName, tr.nodeName)
			gpuids := strings.Join(ids, "_")
			if gpuids != "" {
				cloudFolderPath = cloudFolderPath + "_" + gpuids
			}
			cloudFolderPath = filepath.Join(cloudFolderPath, timestamp)
			exportConfigs := tr.getLogsExportConfig()
			uploadFailed := make([]string, 0)
			uploadPassed := make([]string, 0)
			for _, exportConf := range exportConfigs {
				logger.Log.Printf("exporting logs to provider=%s bucketName=%s under folder=%s", exportConf.Provider.String(), exportConf.BucketName, cloudFolderPath)
				e1 := UploadFileToCloudBucket(exportConf.Provider.String(), exportConf.BucketName, cloudFolderPath, cloudFileName, localCombinedTar, exportConf.SecretName)
				if e1 != nil {
					logger.Log.Printf("export logs to provider=%s bucket=%s failed", exportConf.Provider.String(), exportConf.BucketName)
					uploadFailed = append(uploadFailed, exportConf.Provider.String())
				} else {
					logger.Log.Printf("export logs to provider=%s bucket=%s succeeded", exportConf.Provider.String(), exportConf.BucketName)
					uploadPassed = append(uploadPassed, exportConf.Provider.String())
				}
			}
			if len(exportConfigs) > 0 {
				parameters := tr.getTestParameters(true)
				if len(uploadFailed) == 0 { // generate success event
					msg := fmt.Sprintf("Logs export to %s succeeded", strings.Join(uploadPassed, ", "))
					tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeNormal, testrunnerGen.TestEventReason_LogsExportPassed.String(), nil, msg, gpuIndexes)
				} else { // generate failure event
					msg := fmt.Sprintf("Logs export to %s failed", strings.Join(uploadFailed, ", "))
					tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_LogsExportFailed.String(), nil, msg, gpuIndexes)
				}
			}

		}
	}
}

func (tr *TestRunner) exitOnFailure() {
	switch tr.testTrigger {
	case testrunnerGen.TestTrigger_MANUAL.String(),
		testrunnerGen.TestTrigger_PRE_START_JOB_CHECK.String():
		os.Exit(1)
	}
}

func (tr *TestRunner) getOverallResult(result []*types.IterationResult, validIDs []string) types.TestResult {
	foundEmptyResultIteration := false
	foundTimedoutIteration := false
	for _, iterResult := range result {
		for gpuIdx, actionResults := range iterResult.SuitesResult {
			for action, result := range actionResults {
				switch result {
				case types.Failure, types.Skipped, types.Queued:
					logger.Log.Printf("test on GPU %+v iteration %+v test action %+v didn't pass due to %+v", gpuIdx, iterResult.Number, action, result)
					return types.Failure // if there is any failed action, directly mark overall test run failed
				case types.Timedout:
					foundTimedoutIteration = true
				}
			}
		}
		if iterResult.Status == types.TestTimedOut {
			foundTimedoutIteration = true
		} else {
			// if there is no test result for this iteration
			// it means the test didn't run at all, or the test parser failed
			// we need to put the failure result in the output
			// otherwise there is no visibility for users
			if len(iterResult.SuitesResult) == 0 {
				foundEmptyResultIteration = true
				logger.Log.Printf("test iteration %+v didn't pass due to no test result", iterResult.Number)
				failedSuiteResult := map[string]types.TestResults{}
				for _, gpuID := range validIDs {
					failedSuiteResult[gpuID] = map[string]types.TestResult{
						"result": types.Failure,
					}
				}
				iterResult.SuitesResult = failedSuiteResult
			}
		}
	}
	// firstly check if there is any empty result iteration
	// if there is, return failure
	if foundEmptyResultIteration {
		return types.Failure
	}
	// secondly given that there is no failed iteration
	// check if there is any timedout iteration
	if foundTimedoutIteration {
		return types.Timedout
	}
	return types.Success
}

func (tr *TestRunner) manualTestGPU() {
	// for manual test
	// if there is no GPU detected, fail the test runner process
	allKFDIDs := tr.GetAllKFDIDs()
	parameters := tr.getTestParameters(true)
	if len(allKFDIDs) == 0 {
		logger.Log.Println("no GPU was detected by amd-smi")
		result := BuildNoGPUTestSummary()
		tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestFailed.String(), result, "", []string{})
		// exit on non-auto trigger's failure
		tr.exitOnFailure()
	}

	// handle test runner crash or restart
	// read existing test runner status db
	// immediately start test on interrupted test before restarting
	statusObj, _ := LoadRunnerStatus(tr.statusDBPath)
	if statusObj != nil && len(statusObj.TestStatus) > 0 {
		ids := []string{}
		for deviceID := range statusObj.TestStatus {
			ids = append(ids, deviceID)
		}
		logger.Log.Printf("found GPU %+v with incomplete test before restart %+v, start to rerun test", ids, statusObj)
		tr.testGPU(tr.testTrigger, ids, true)
	} else {
		tr.testGPU(tr.testTrigger, parameters.TestCases[0].DeviceIDs, false)
	}
}

func (tr *TestRunner) ReadPodInfo() {
	if tr.k8sPodName == "" {
		tr.k8sPodName = os.Getenv("POD_NAME")
	}
	if tr.k8sPodNamespace == "" {
		tr.k8sPodNamespace = os.Getenv("POD_NAMESPACE")
	}
}

func (tr *TestRunner) AddTestRunningLabel(recipe string, indexes []string) {
	if !tr.isK8s {
		return
	}
	keys, val := GetTestRunningLabelKeyValue(tr.testCategory, recipe, indexes)
	if err := tr.k8sClient.AddNodeLabel(tr.hostName, keys, val); err != nil {
		logger.Log.Printf("Failed to add node label: %+v", err)
	}
}

func (tr *TestRunner) RemoveTestRunningLabel(recipe string, indexes []string) {
	if !tr.isK8s {
		return
	}
	keys, _ := GetTestRunningLabelKeyValue(tr.testCategory, recipe, indexes)
	if err := tr.k8sClient.RemoveNodeLabel(tr.hostName, keys); err != nil {
		logger.Log.Printf("Failed to remove node label: %+v", err)
	}
}

func (tr *TestRunner) normalizeConfig() {
	// convert category to uppercase so that config map won't be case sensitive
	if tr.globalTestRunnerConfig != nil {
		newConfigMap := map[string]*testrunnerGen.TestCategoryConfig{}
		for category, categoryConfig := range tr.globalTestRunnerConfig.TestConfig {
			if categoryConfig != nil {
				newConfigMap[strings.ToUpper(category)] = categoryConfig
				newLocationConfig := map[string]*testrunnerGen.TestTriggerConfig{}
				for location, triggerConfig := range categoryConfig.TestLocationTrigger {
					if triggerConfig != nil {
						newParams := map[string]*testrunnerGen.TestParameters{}
						for trigger, params := range triggerConfig.TestParameters {
							newParams[strings.ToUpper(trigger)] = params
						}
						newLocationConfig[location] = &testrunnerGen.TestTriggerConfig{
							TestParameters: newParams,
						}
					}
				}
				categoryConfig.TestLocationTrigger = newLocationConfig
			}
		}
		tr.globalTestRunnerConfig.TestConfig = newConfigMap
	}
}

func (tr *TestRunner) getTestParameters(lock bool) *testrunnerGen.TestParameters {
	if lock {
		tr.Lock()
		defer tr.Unlock()
	}
	return tr.globalTestRunnerConfig.TestConfig[tr.testCategory].TestLocationTrigger[tr.testLocation].TestParameters[tr.testTrigger]
}

func (tr *TestRunner) getLogsExportConfig() []*testrunnerGen.TestLogsExportConfig {
	tr.Lock()
	defer tr.Unlock()
	return tr.globalTestRunnerConfig.TestConfig[tr.testCategory].TestLocationTrigger[tr.testLocation].TestParameters[tr.testTrigger].LogsExportConfig
}

func (tr *TestRunner) getHostName() {
	hostName, err := os.Hostname()
	if err != nil {
		logger.Log.Printf("failed to get hostname, err: %+v", err)
	}
	tr.hostName = hostName
	if utils.IsKubernetes() {
		tr.hostName = os.Getenv("NODE_NAME")
	}
	logger.Log.Printf("HostName: %v", tr.hostName)
}

func (tr *TestRunner) generateK8sEvent(testRecipe, evtType, reason string, summary []*types.IterationResult, message string, gpuIndexes []string) {
	if !tr.isK8s {
		// return if it is not running in k8s cluster
		return
	}
	tr.ReadPodInfo()
	if tr.k8sPodName == "" || tr.k8sPodNamespace == "" {
		logger.Log.Printf("failed to get pod name or pod namespace: name: %+v namespace: %+v, skip generating event for recipe %+v evtType %+v reason %+v summary %+v",
			tr.k8sPodName, tr.k8sPodNamespace, testRecipe, evtType, reason, summary)
		return
	}
	var msg string
	if summary != nil {
		// don't put stdout and stderr large string into the event message
		// they will be saved into zipped log file
		for _, res := range summary {
			res.Stdout = ""
			res.Stderr = ""
		}

		// just save result into json message
		msgbytes, err := json.Marshal(summary)
		if err != nil {
			logger.Log.Panicf("failed to marshal test summary %+v err %+v", summary, err)
			return
		}
		msg = string(msgbytes)
	} else {
		msg = message
	}

	kfdIDs := tr.convertIndexesToKFDIDs(gpuIndexes)
	evtNamePrefix := GetEventNamePrefix(tr.testCategory)
	// if there is no event exist, create a new one
	currTime := time.Now().UTC()
	evtObj := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: evtNamePrefix,
			Namespace:    tr.k8sPodNamespace,
			Labels:       GetEventLabels(tr.testCategory, tr.testTrigger, testRecipe, tr.hostName, gpuIndexes, kfdIDs),
		},
		FirstTimestamp: metav1.Time{
			Time: currTime,
		},
		LastTimestamp: metav1.Time{
			Time: currTime,
		},
		Count:   1,
		Type:    evtType,
		Reason:  reason,
		Message: string(msg),
		InvolvedObject: v1.ObjectReference{
			Kind:      "Pod",
			Namespace: tr.k8sPodNamespace,
			Name:      tr.k8sPodName,
		},
		Source: v1.EventSource{
			Host:      tr.hostName,
			Component: globals.EventSourceComponentName,
		},
	}
	// TODO: handle error properly for failing to generate event
	if err := tr.k8sClient.CreateEvent(evtObj); err != nil {
		logger.Log.Printf("create event failed. err: %+v", err)
	}
}

// NewTestRunnerIntf creates a types.TestRunner based on the given type and arguments.
func NewTestRunnerIntf(runnerType types.TestRunnerType, args ...interface{}) (types.TestRunner, error) {
	switch runnerType {
	case types.RVSRunner:
		// expects: rvsBinPath, testSuitesDir, resultLogDir string
		if len(args) != 3 {
			return nil, fmt.Errorf("rvs runner requires 3 arguments: rvsBinPath, testSuitesDir, resultLogDir")
		}
		rvsBinPath, ok1 := args[0].(string)
		testSuitesDir, ok2 := args[1].(string)
		resultLogDir, ok3 := args[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("invalid argument types for rvs runner")
		}
		return NewRvsTestRunner(rvsBinPath, testSuitesDir, resultLogDir)
	case types.AGFHCRunner:
		// expects: agfhcBinPath, testSuitesDir, resultLogDir string
		if len(args) != 3 {
			return nil, fmt.Errorf("AGFHC runner requires 3 arguments: agfhcBinPath, testSuitesDir, resultLogDir")
		}
		agfhcBinPath, ok1 := args[0].(string)
		testSuitesDir, ok2 := args[1].(string)
		resultLogDir, ok3 := args[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("invalid argument types for AGFHC runner")
		}
		return NewAgfhcTestRunner(agfhcBinPath, testSuitesDir, resultLogDir)
	default:
		return nil, fmt.Errorf("unknown TestRunner type: %s", runnerType)
	}
}
