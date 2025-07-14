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
	rocmSMIPath        string
	exporterSocketPath string

	testCategory        string
	testLocation        string
	testTrigger         string
	rvsTestCaseDir      string
	testCfgPath         string
	testCfgGPUModelName string
	gpuIndexToGUIDMap   map[string]string
	gUIDToGPUIndexMap   map[string]string

	logDir       string
	statusDBPath string

	jobName  string
	nodeName string

	sync.Mutex             // mutex to protect globalTestRunnerConfig from file watcher
	globalTestRunnerConfig *testrunnerGen.TestRunnerConfig
	rvsTestRunner          types.TestRunner

	// k8s related fields
	isK8s           bool
	k8sClient       *k8sclient.K8sClient
	k8sPodName      string
	k8sPodNamespace string
}

// initTestRunner init the test runner and related configs
// return the test location, either global or specific host name
func NewTestRunner(rvsPath, rvsTestCaseDir, rocmSMIPath, exporterSocketPath, testRunnerConfigPath, testCategory, testTrigger, logDir, jobName, nodeName string) *TestRunner {
	runner := &TestRunner{
		rvsPath:            rvsPath,
		rocmSMIPath:        rocmSMIPath,
		exporterSocketPath: exporterSocketPath,
		testCategory:       testCategory,
		testTrigger:        testTrigger,
		testCfgPath:        testRunnerConfigPath,
		rvsTestCaseDir:     rvsTestCaseDir,
		gpuIndexToGUIDMap:  map[string]string{},
		gUIDToGPUIndexMap:  map[string]string{},
		logDir:             logDir,
		jobName:            jobName,
		nodeName:           nodeName,
	}
	// init test runner config
	// testRunnerConfigPath file existence has been verified
	runner.initLogger()
	runner.readTestRunnerConfig(testRunnerConfigPath)
	runner.getHostName()
	runner.validateTestTrigger()
	runner.initTestRunnerConfig()
	if utils.IsKubernetes() {
		runner.isK8s = true
		runner.k8sClient = k8sclient.NewClient(context.Background())
	}
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
	_, foundHostSpecifcTest := categoryConfig.TestLocationTrigger[tr.hostName]
	_, foundGlobalTest := categoryConfig.TestLocationTrigger[globals.GlobalTestTriggerKeyword]
	if !foundGlobalTest && !foundHostSpecifcTest {
		fmt.Printf("cannot find neither global test config nor host specific config under category %+v: %+v\n", tr.testCategory, categoryConfig)
		os.Exit(1)
	}

	// 3. validate test trigger's config
	// if host specifc config was found
	// validate host specific config's trigger
	if foundHostSpecifcTest {
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
	gpuModelSubDir, err := getGPUModelTestRecipeDir(tr.rocmSMIPath)
	if err != nil {
		logger.Log.Printf("failed to get GPU model specific folder for test recipe err %+v, using recipe from root conf folder", err)
	}
	testCfgPath := filepath.Join(tr.rvsTestCaseDir, testParams.TestCases[0].Recipe+".conf")
	if gpuModelSubDir != "" {
		logger.Log.Printf("using test recipe from %+v folder", gpuModelSubDir)
		tr.testCfgGPUModelName = gpuModelSubDir
		testCfgPath = filepath.Join(tr.rvsTestCaseDir, gpuModelSubDir, testParams.TestCases[0].Recipe+".conf")
	}
	if _, err := os.Stat(testCfgPath); err != nil {
		fmt.Printf("Trigger %+v cannot find corresponding test config file %+v, err: %+v\n", tr.testTrigger, testCfgPath, err)
		os.Exit(1)
	}
	if testParams.TestCases[0].Iterations == 0 {
		fmt.Printf("Trigger %+v has been configured to run with 0 iteration, should be non-zero iterations\n", tr.testTrigger)
		os.Exit(1)
	}
	if testParams.TestCases[0].TimeoutSeconds == 0 {
		fmt.Printf("Trigger %+v has been configured to run with 0 TimeoutSeconds, should be non-zero TimeoutSeconds\n", tr.testTrigger)
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

func (tr *TestRunner) getTestRecipeDir() string {
	return filepath.Join(tr.rvsTestCaseDir, tr.testCfgGPUModelName)
}

func (tr *TestRunner) generateGPUIDMapping() error {
	var err error
	for i := 0; i < 3; i++ {
		cmd := exec.Command(tr.rocmSMIPath, "-i", "--json")
		output, err := cmd.Output()
		if err != nil {
			logger.Log.Printf("cannot execute command: rocm-smi -i --json, err: %+v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Parse the JSON response
		var result map[string]interface{}
		err = json.Unmarshal(output, &result)
		if err != nil {
			logger.Log.Printf("cannot unmarshal rocm-smi output: %+v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for cardName, cardInfo := range result {
			if cardInfoMap, ok := cardInfo.(map[string]interface{}); ok {
				if guid, ok := cardInfoMap["GUID"].(string); ok {
					index := strings.Trim(strings.ToLower(cardName), "card")
					tr.gpuIndexToGUIDMap[index] = guid
					tr.gUIDToGPUIndexMap[guid] = index
				}
			}
		}
		logger.Log.Printf("generated GPU index to GUID mapping, rocm-smi output: %+v map: %+v", result, tr.gpuIndexToGUIDMap)
		return nil
	}
	return fmt.Errorf("after all attempts still cannot get and parse GPU index and GUID mapping, last error: %+v", err)
}

func (tr *TestRunner) convertIndexesToGUIDs(indexes []string) []string {
	guids := []string{}
	for _, index := range indexes {
		guid, ok := tr.gpuIndexToGUIDMap[index]
		if !ok {
			logger.Log.Printf("failed to get GUID for index %+v from map %+v", index, tr.gpuIndexToGUIDMap)
			continue
		}
		guids = append(guids, guid)
	}
	// if users specified a list of GPU index to test
	// but none of them is available
	// don't return an empty list here since it is going to run test on all GPUs
	// fail the container here
	if len(indexes) > 0 && len(guids) == 0 {
		logger.Log.Printf("error looking for GUID from all provided GPU indexes %+v, exiting...", indexes)
		os.Exit(1)
	}
	return guids
}

func (tr *TestRunner) convertGUIDsToIndexes(guids []string) []string {
	indexes := []string{}
	for _, guid := range guids {
		index, ok := tr.gUIDToGPUIndexMap[guid]
		if !ok {
			logger.Log.Printf("failed to get index for gUID %+v from map %+v", guid, tr.gUIDToGPUIndexMap)
			continue
		}
		indexes = append(indexes, index)
	}
	// if users specified a list of GPU index to test
	// but none of them is available
	// don't return an empty list here since it is going to run test on all GPUs
	// fail the container here
	if len(guids) > 0 && len(indexes) == 0 {
		logger.Log.Printf("error looking for indexes from all provided GPU GUIDs %+v, exiting...", guids)
		os.Exit(1)
	}
	return indexes
}

// the validation functions have make sure that the given category/location/trigger config exists and valid within runnerConfig
// this function will be responsible to trigger the test
func (tr *TestRunner) TriggerTest() {
	switch tr.testCategory {
	case testrunnerGen.TestCategory_GPU_HEALTH_CHECK.String():
		if err := tr.generateGPUIDMapping(); err != nil {
			logger.Log.Printf("failed to get and parse GPU index and GUID mapping: %+v", err)
			os.Exit(1)
		}
		switch tr.testTrigger {
		case testrunnerGen.TestTrigger_AUTO_UNHEALTHY_GPU_WATCH.String():
			// init rvs test runner
			// and start to listen for unix socket to receive the event
			// for triggering the test run on unhealthy GPU
			rvsTestRunner, err := NewRvsTestRunner(tr.rvsPath, tr.getTestRecipeDir(), tr.logDir)
			if err != nil || rvsTestRunner == nil {
				logger.Log.Printf("failed to create rvs test runner, runner: %+v, err: %+v", rvsTestRunner, err)
				os.Exit(1)
			}
			tr.rvsTestRunner = rvsTestRunner
			tr.watchGPUState()
		case testrunnerGen.TestTrigger_MANUAL.String(),
			testrunnerGen.TestTrigger_PRE_START_JOB_CHECK.String():
			rvsTestRunner, err := NewRvsTestRunner(tr.rvsPath, tr.getTestRecipeDir(), tr.logDir)
			if err != nil || rvsTestRunner == nil {
				logger.Log.Printf("failed to create rvs test runner, runner: %+v, err: %+v", rvsTestRunner, err)
				os.Exit(1)
			}
			tr.rvsTestRunner = rvsTestRunner
			tr.manualTestGPU()
		default:
			logger.Log.Printf("unsupported test trigger %+v for category %+v", tr.testTrigger, tr.testCategory)
			os.Exit(1)
		}
	}
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
			if _, ok := tr.gUIDToGPUIndexMap[deviceID]; !ok {
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
				// TODO: currently exporter with gpuagent just returns GPU index number
				// we need to convert it to GUID per rvs's request
				// modify this after rvs starts to accept index number as ID
				id, err := GetGUIDFromIndex(state.ID, tr.rocmSMIPath)
				if err != nil {
					logger.Log.Printf("failed to fetch GUID for GPU card%v, err: %+v", state.ID, err)
					continue
				}
				// if any GPU is not healthy, start a test against those GPUs
				if !strings.EqualFold(state.Health, metricssvc.GPUHealth_HEALTHY.String()) {
					if len(state.AssociatedWorkload) == 0 {
						unHealthyGPUIDs = append(unHealthyGPUIDs, id)
					} else {
						logger.Log.Printf("found GPU %+v unhealthy but still associated with workload %+v", id, state.AssociatedWorkload)
					}
				} else {
					healthyGPUIDs = append(healthyGPUIDs, id)
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
// ids is a list of GUID
func (tr *TestRunner) testGPU(trigger string, ids []string, isRerun bool) {
	parameters := tr.getTestParameters(true)
	// load ongoing test status
	// avoid run multiple test on the same device
	// validIDs will be the list of GUIDs as the test run parameter
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

	handler, err := tr.rvsTestRunner.GetTestHandler(parameters.TestCases[0].Recipe, types.TestParams{
		Iterations:    uint(parameters.TestCases[0].Iterations),
		StopOnFailure: parameters.TestCases[0].StopOnFailure,
		DeviceIDs:     validIDs,
		Timeout:       uint(parameters.TestCases[0].TimeoutSeconds),
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
		ids, err := GetAllGUIDs(tr.rocmSMIPath)
		if err != nil {
			logger.Log.Printf("Error selecting devices: %v", err)
			// TODO: add more error handling when failed to get all GUIDs
		}
		validIDs = ids
	}
	for _, id := range validIDs {
		statusObj.TestStatus[id] = types.TestRunning.String()
	}

	err = SaveRunnerStatus(statusObj, tr.statusDBPath)
	if err != nil {
		logger.Log.Printf("Error saving runner status: %v", err)
		//TODO: add error handling here if new running status failed to be saved
	}

	gpuIndexes := tr.convertGUIDsToIndexes(validIDs)
	tr.AddTestRunningLabel(parameters.TestCases[0].Recipe, gpuIndexes)
	defer tr.RemoveTestRunningLabel(parameters.TestCases[0].Recipe, gpuIndexes)

	select {
	case <-time.After(time.Duration(parameters.TestCases[0].TimeoutSeconds) * time.Second * time.Duration(parameters.TestCases[0].Iterations)):
		logger.Log.Printf("Trigger: %v Test: %v GPU IDs: %v GPU Indexes: %v timeout", trigger, parameters.TestCases[0].Recipe, validIDs, gpuIndexes)
		result := handler.Result()
		result = AppendTimedoutTestSummary(result, validIDs)
		handler.StopTest()
		// when the test timedout
		// save whatever test console logs that are cached
		tr.saveAndExportHandlerLogs(handler, ids, parameters.TestCases[0].Recipe, gpuIndexes, validIDs)
		tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestTimedOut.String(), result, "", gpuIndexes, validIDs)
		// exit on non-auto trigger's failure
		tr.exitOnFailure()
	case <-handler.Done():
		// TODO: this has to change later based on result logs parsing.
		// for now updating same result in all GPU
		result := handler.Result()
		logger.Log.Printf("Trigger: %v Test: %v GPU IDs: %v GPU Indexes: %v completed. Result: %v", trigger, parameters.TestCases[0].Recipe, validIDs, gpuIndexes, result)

		// save log into gzip file
		tr.saveAndExportHandlerLogs(handler, ids, parameters.TestCases[0].Recipe, gpuIndexes, validIDs)

		switch tr.getOverallResult(result) {
		case types.Success:
			tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeNormal, testrunnerGen.TestEventReason_TestPassed.String(), result, "", gpuIndexes, validIDs)
		case types.Failure:
			tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestFailed.String(), result, "", gpuIndexes, validIDs)
			// exit on non-auto trigger's failure
			tr.exitOnFailure()
		case types.Timedout:
			tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestTimedOut.String(), result, "", gpuIndexes, validIDs)
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

func (tr *TestRunner) saveAndExportHandlerLogs(handler types.TestHandlerInterface, ids []string, recipe string, gpuIndexes, validIDs []string) {
	for _, res := range handler.Result() {
		var filesToExport []string
		resultsJson, err := ExtractLogFile(res.Stdout)
		if err != nil {
			logger.Log.Printf("Unable to locate results json file")
		}
		if resultsJson != "" {
			resultsJson = filepath.Join(globals.RVSLogDir, resultsJson)
			filesToExport = append(filesToExport, resultsJson)
		}
		now := time.Now().UTC()
		timestamp := now.Format("2006-01-02T15-04-05.000000Z")
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
		if len(filesToExport) == 0 {
			continue
		}
		cloudFileName := timestamp + ".tar.gz"
		localCombinedTar := filepath.Join(globals.RVSLogDir, cloudFileName)
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
					tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeNormal, testrunnerGen.TestEventReason_LogsExportPassed.String(), nil, msg, gpuIndexes, validIDs)
				} else { // generate failure event
					msg := fmt.Sprintf("Logs export to %s failed", strings.Join(uploadFailed, ", "))
					tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_LogsExportFailed.String(), nil, msg, gpuIndexes, validIDs)
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

func (tr *TestRunner) getOverallResult(result []*types.IterationResult) types.TestResult {
	foundTimedout := false
	for _, iterResult := range result {
		for guid, actionResults := range iterResult.SuitesResult {
			for action, result := range actionResults {
				switch result {
				case types.Failure:
					logger.Log.Printf("test on GPU %+v iteration %+v test action %+v didn't pass due to %+v", guid, iterResult.Number, action, result)
					return types.Failure // if there is any failed action, directly mark overall test run failed
				case types.Timedout:
					foundTimedout = true
				}
			}
		}
		if iterResult.Status == types.TestTimedOut {
			foundTimedout = true
		}
	}
	if foundTimedout {
		return types.Timedout
	}
	return types.Success
}

func (tr *TestRunner) manualTestGPU() {
	// for manual test
	// if there is no GPU detected, fail the test runner process
	allGUIDs, err := GetAllGUIDs(tr.rocmSMIPath)
	if err != nil {
		logger.Log.Printf("failed to detect GPU by rocm-smi err %+v", err)
		os.Exit(1)
	}
	parameters := tr.getTestParameters(true)
	if len(allGUIDs) == 0 {
		logger.Log.Println("no GPU was detected by rocm-smi")
		result := BuildNoGPUTestSummary()
		tr.generateK8sEvent(parameters.TestCases[0].Recipe, v1.EventTypeWarning, testrunnerGen.TestEventReason_TestFailed.String(), result, "", []string{}, []string{})
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
		ids := tr.convertIndexesToGUIDs(parameters.TestCases[0].DeviceIDs)
		tr.testGPU(tr.testTrigger, ids, false)
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

func (tr *TestRunner) generateK8sEvent(testRecipe, evtType, reason string, summary []*types.IterationResult, message string, gpuIndexes, kfdIDs []string) {
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
