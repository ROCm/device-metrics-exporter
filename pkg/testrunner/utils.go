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
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	testrunnerGen "github.com/ROCm/device-metrics-exporter/pkg/testrunner/gen/testrunner"
	types "github.com/ROCm/device-metrics-exporter/pkg/testrunner/interface"
)

var statusDBLock sync.Mutex

// ValidateArgs validate argument to make sure the mandatory tools/configs are available
func ValidateArgs(testCategory, testTrigger, rvsPath, amdSMIPath, rvsTestCaseDir, exporterSocketPath, agfhcPath, agfhcTestCaseDir string) {
	validateArgCategory(testCategory)
	validateArgTrigger(testTrigger)
	statOrExit(rvsPath, false)
	statOrExit(amdSMIPath, false)
	switch testCategory {
	case testrunnerGen.TestCategory_GPU_HEALTH_CHECK.String():
		switch testTrigger {
		case testrunnerGen.TestTrigger_AUTO_UNHEALTHY_GPU_WATCH.String():
			statOrExit(exporterSocketPath, false)
		}
	}
	statOrExit(rvsTestCaseDir, true)
	dryRunBinary(rvsPath, "-g")      // run rvs to list GPU to make sure rvs is working
	dryRunBinary(amdSMIPath, "list") // run amd-smi to list GPU IDs to make sure GPU info is available

	// TODO agfhc validation is dependent on test configuration being used as agfhc may not always be available
	// dryRunBinary(agfhcPath, "-l")   // run agfhc to list available tests and recipes to make sure agfhc is working
	// statOrExit(agfhcTestCaseDir, true)
}

func validateArgCategory(category string) {
	if _, ok := testrunnerGen.TestCategory_value[category]; !ok {
		fmt.Printf("cannot find %v in listed supported category %+v\n", category, testrunnerGen.TestCategory_value)
		os.Exit(1)
	}
}

func validateArgTrigger(trigger string) {
	if _, ok := testrunnerGen.TestTrigger_value[trigger]; !ok {
		fmt.Printf("cannot find %v in listed supported trigger %+v\n", trigger, testrunnerGen.TestTrigger_value)
		os.Exit(1)
	}
}

// dryRunBinary dry run the executable binary to make sure it is working, otherwise exit
func dryRunBinary(binPath, arg string) {
	cmd := exec.Command(binPath, arg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing %+v %+v: %+v\n", binPath, arg, err)
		fmt.Printf("Output: %+v\n", string(output))
		os.Exit(1)
	}
}

// statOrExit look given file/dir exists otherwise exit
func statOrExit(path string, isFolder bool) {
	if info, err := os.Stat(path); err != nil {
		fmt.Printf("Failed to find %+v, err: %+v\n", path, err)
		os.Exit(1)
	} else if info != nil && info.IsDir() != isFolder {
		fmt.Printf("Expect %+v IsDir %+v got %+v\n", path, isFolder, info.IsDir())
		os.Exit(1)
	}
}

func SaveRunnerStatus(statusObj *testrunnerGen.TestRunnerStatus, statusDBPath string) error {
	statusDBLock.Lock()
	defer statusDBLock.Unlock()
	data, err := json.Marshal(statusObj)
	if err != nil {
		return err
	}
	err = os.WriteFile(statusDBPath, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

func LoadRunnerStatus(statusDBPath string) (*testrunnerGen.TestRunnerStatus, error) {
	statusDBLock.Lock()
	defer statusDBLock.Unlock()
	var status testrunnerGen.TestRunnerStatus
	data, err := os.ReadFile(statusDBPath)
	if err != nil {
		return &status, err
	}
	err = json.Unmarshal(data, &status)
	if err != nil {
		return &status, err
	}
	return &status, nil
}

// transformRunnerStatus transform the KFD IDs in the statusDB to gpu indexes
func transformRunnerStatus(statusDBPath string, kfdIDToIndex, gpuIndexToKFDID map[string]string) error {
	statusObj, err := LoadRunnerStatus(statusDBPath)
	if err != nil {
		return fmt.Errorf("failed to load test runner status from %v, err: %v", statusDBPath, err)
	}

	newTestStatus := make(map[string]string)

	for id, status := range statusObj.TestStatus {
		if _, ok := gpuIndexToKFDID[id]; ok {
			// no need to convert
			newTestStatus[id] = status
			continue
		}

		if _, ok := kfdIDToIndex[id]; ok {
			// If the id is a KFD ID, convert to gpu-index
			newTestStatus[kfdIDToIndex[id]] = status
		}

		// If the index is not in either mapping, do not add it to the new status.
		// This can happen for example when gpu has been partitioned and test-runner restarted
	}

	statusObj.TestStatus = newTestStatus

	return SaveRunnerStatus(statusObj, statusDBPath)
}

func SaveTestResultToGz(output, path string) {
	// Create the file
	file, err := os.Create(path)
	if err != nil {
		logger.Log.Printf("failed to create gzip file %v, err: %v", path, err)
	}
	defer file.Close()

	// Create a gzip writer
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	// Write the string data to the gzip writer
	_, err = gzipWriter.Write([]byte(output))
	if err != nil {
		logger.Log.Printf("failed to write to gzip writer %v, err: %v", path, err)
	}
}

func GetLogFilePath(resultLogDir, ts, trigger, testName, suffix string) string {
	fileName := ts + "_" + trigger + "_" + testName + "_" + suffix + ".gz"
	return filepath.Join(resultLogDir, fileName)
}

// GzipResultJson takes a result json path as input, reads the file,
// gzips its content and generate gzipped version in target location.
func GzipResultJson(jsonFilePath, targetFilePath string) error {
	originalContent, err := os.ReadFile(jsonFilePath)
	if err != nil {
		return fmt.Errorf("failed to read original file %s: %v", jsonFilePath, err)
	}

	targetFile, err := os.Create(targetFilePath)
	if err != nil {
		return fmt.Errorf("failed to create gzipped file %s: %v", targetFilePath, err)
	}
	defer targetFile.Close()

	gzWriter := gzip.NewWriter(targetFile)
	if _, err := gzWriter.Write(originalContent); err != nil {
		return fmt.Errorf("failed to write gzipped content to target file %s: %v", targetFilePath, err)
	}
	defer gzWriter.Close()

	return nil
}

// GzipFolder takes the whole directory into one gzipped file
func GzipFolder(sourceDir, targetFile string) error {
	// Create output file
	f, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Create a gzip writer
	gw := gzip.NewWriter(f)
	defer gw.Close()

	// Create a tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Walk through the source directory
	return filepath.Walk(sourceDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories (they will be added through their files)
		if fi.IsDir() {
			return nil
		}

		// Open file
		fr, err := os.Open(file)
		if err != nil {
			return err
		}
		defer fr.Close()

		// Create tar header
		header, err := tar.FileInfoHeader(fi, file)
		if err != nil {
			return err
		}

		// Adjust name to keep relative path
		relPath, err := filepath.Rel(sourceDir, file)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Copy file data into tar writer
		if _, err := io.Copy(tw, fr); err != nil {
			return err
		}

		return nil
	})
}

// getAllKFDIDs list all kfd ids from amd-smi
func (tr *TestRunner) GetAllKFDIDs() []string {
	kfdIDs := []string{}
	for kfdID := range tr.kfdIDToGPUIndexMap {
		kfdIDs = append(kfdIDs, kfdID)
	}
	return kfdIDs
}

// GetAllGPUIndexes list all gpu indexes
func (tr *TestRunner) GetAllGPUIndexes() []string {
	indexes := []string{}
	for idx := range tr.gpuIndexToKFDIDMap {
		indexes = append(indexes, idx)
	}
	return indexes
}

func getGPUModelTestRecipeDir(amdSMIPath string) (string, error) {
	cmd := exec.Command(amdSMIPath, "static", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return parseAMDSMIStaticOutput(output)
}

func parseAMDSMIStaticOutput(output []byte) (string, error) {
	// Parse the JSON response
	var result map[string]interface{}
	err := json.Unmarshal(output, &result)
	if err != nil {
		return "", err
	}

	gpuDataIntf, ok := result["gpu_data"]
	if !ok {
		return "", fmt.Errorf("failed to find gpu_data in amd-smi static output %+v", result)
	}

	gpuDataSlice, ok := gpuDataIntf.([]interface{})
	if !ok {
		return "", fmt.Errorf("failed to parse gpu data slice as []interface{} %+v", gpuDataIntf)
	}
	if len(gpuDataSlice) == 0 {
		return "", fmt.Errorf("no GPU data found in amd-smi static output %+v", gpuDataSlice)
	}

	// currently we assume the setup is homogeneous
	// which means only same type GPUs can be installed on one node
	for _, gpuDataIntf := range gpuDataSlice {
		gpuData, ok := gpuDataIntf.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("failed to parse gpu data as map[string]interface{} %+v", gpuDataIntf)
		}
		asicInfoIntf, ok := gpuData["asic"]
		if !ok {
			logger.Log.Printf("failed to find GPU asic info %+v", gpuData)
			continue
		}
		asicInfo, ok := asicInfoIntf.(map[string]interface{})
		if !ok {
			logger.Log.Printf("failed to parse GPU asic info %+v err: %+v", asicInfoIntf, err)
			continue
		}
		for _, pciDeviceIDField := range []string{"device_id", "subsystem_id"} {
			deviceIDIntf, ok := asicInfo[pciDeviceIDField]
			if !ok {
				logger.Log.Printf("failed to find pci device id from asic info %+v", asicInfo)
				continue
			}
			deviceID, ok := deviceIDIntf.(string)
			if !ok {
				logger.Log.Printf("failed to parse deviceID string %+v err: %+v", deviceIDIntf, err)
				continue
			}
			if dir, ok := globals.GPUDeviceIDToModelName[deviceID]; ok {
				return dir, nil
			}
		}
	}
	return "", fmt.Errorf("failed to get Device ID from amd-smi")
}

func removeIDsWithExistingTest(trigger, statusDBPath string, ids []string, parameters *testrunnerGen.TestParameters, isRerun bool) ([]string, *testrunnerGen.TestRunnerStatus) {
	// load ongoing test status
	// avoid run multiple test on the same device
	statusObj, err := LoadRunnerStatus(statusDBPath)
	if err != nil {
		logger.Log.Printf("failed to load test runner status %+v, err: %+v", statusDBPath, err)
		if os.IsNotExist(err) {
			if _, err := os.Create(statusDBPath); err != nil {
				logger.Log.Printf("failed to create status db file at %+v, err: %+v", statusDBPath, err)
				// TODO: add error handling
			}
		}
		// TODO: add more error handling when failed to load runner running status
	}
	if statusObj == nil || statusObj.TestStatus == nil {
		statusObj = &testrunnerGen.TestRunnerStatus{}
		statusObj.TestStatus = map[string]string{}
	}
	validIDs := []string{}
	for _, id := range ids {
		if testStatus, ok := statusObj.TestStatus[id]; ok && !isRerun {
			logger.Log.Printf("trigger %+v is trying to run test %+v on device %+v but found existing %v test, skip for now",
				trigger, parameters.TestCases[0].Recipe, id, testStatus)
		} else {
			validIDs = append(validIDs, id)
		}
	}
	return validIDs, statusObj
}

func GetEventNamePrefix(testCategory string) string {
	return strings.ToLower("amd-test-runner-" + testCategory + "-")
}

func AppendTimedoutTestSummary(existingResults []*types.IterationResult, ids []string) []*types.IterationResult {
	for _, id := range ids {
		newIteration := uint32(len(existingResults) + 1)
		newResult := map[string]types.TestResults{}
		newResult[id] = map[string]types.TestResult{}
		newResult[id]["result"] = types.Timedout

		existingResults = append(existingResults, &types.IterationResult{
			Number:       newIteration,
			SuitesResult: newResult,
			Status:       types.TestTimedOut,
		})
	}
	return existingResults
}

func BuildNoGPUTestSummary() []*types.IterationResult {
	result := []*types.IterationResult{}
	result = append(result, &types.IterationResult{
		Number:       1,
		SuitesResult: map[string]types.TestResults{},
		Status:       types.TestCompleted,
	})
	result[0].SuitesResult[globals.NoGPUErrMsg] = map[string]types.TestResult{}
	result[0].SuitesResult[globals.NoGPUErrMsg]["detect_gpu"] = types.Failure
	return result
}

func GetTestRunningLabelKeyValue(category, recipe string, indexes []string) ([]string, string) {
	keys := []string{}
	for _, idx := range indexes {
		keys = append(keys, strings.ToLower(fmt.Sprintf("testrunner.amd.com.%v.%v.gpu%v", category, recipe, idx)))
	}
	return keys, "running"
}

func GetEventLabels(category, trigger, recipe, hostName string, gpuIndexes, kfdIDs []string) map[string]string {
	labels := map[string]string{
		"testrunner.amd.com/category": strings.ToLower(category),
		"testrunner.amd.com/trigger":  strings.ToLower(trigger),
		"testrunner.amd.com/recipe":   recipe,
		"testrunner.amd.com/hostname": hostName,
	}
	// find the commonly shorter length of 2 list: gpuIndexes and kfdIDs
	// make sure the loop is not out of boundary
	size := len(kfdIDs)
	if len(gpuIndexes) < size {
		size = len(gpuIndexes)
	}
	for i := 0; i < size; i++ {
		labels[fmt.Sprintf("testrunner.amd.com/gpu.id.%v", gpuIndexes[i])] = kfdIDs[i]
		labels[fmt.Sprintf("testrunner.amd.com/gpu.kfd.%v", kfdIDs[i])] = gpuIndexes[i]
	}
	return labels
}

func addFileToTar(tw *tar.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		logger.Log.Printf("Unable to open file %s. Error:%v", path, err)
		return err
	}
	defer file.Close()
	if stat, err := file.Stat(); err == nil {
		// create metadata
		header := new(tar.Header)
		header.Name = path
		header.Size = stat.Size()
		header.Mode = int64(stat.Mode())
		header.ModTime = stat.ModTime()
		// write the header to the tarball archive
		if err := tw.WriteHeader(header); err != nil {
			logger.Log.Printf("Unable to write %s header info to tar. Error:%v", path, err)
			return err
		}
		// copy the file data to the tarball
		if _, err := io.Copy(tw, file); err != nil {
			logger.Log.Printf("Unable to copy file %s to tar. Error:%v", path, err)
			return err
		}
	}
	return nil
}

func CreateTarFile(outputPath string, inputPaths []string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		logger.Log.Printf("Unable to create tar file. Err:%v", err)
		return fmt.Errorf("Unable to create tar file. Err:%v", err)
	}
	defer file.Close()
	gw := gzip.NewWriter(file)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	for i := range inputPaths {
		if err = addFileToTar(tw, inputPaths[i]); err != nil {
			logger.Log.Printf("Unable to add %s file to tar. Error:%v", inputPaths[i], err)
			continue
		}
	}
	return err
}
