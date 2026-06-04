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

package fsysdevice

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

const (
	AMDLogicalDevicePrefix = "amdgpu_xcp_"
)

var (
	once sync.Once
)

func getUsedVRAM(nodeid string) (float64, error) {
	if nodeid == "" {
		return 0, fmt.Errorf("nodeid is empty")
	}

	filePath := filepath.Join("/sys/class/kfd/kfd/topology/nodes", nodeid, "mem_banks/0/used_memory")
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to stat file: %w", err)
	}
	if fileInfo.IsDir() {
		return 0, fmt.Errorf("expected file but found directory: %s", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}

	strVal := string(data)
	strVal = regexp.MustCompile(`\s+`).ReplaceAllString(strVal, "")
	if strVal == "" {
		return 0, fmt.Errorf("file is empty: %s", filePath)
	}

	uintVal, err := strconv.ParseUint(strVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse uint64 from file: %w", err)
	}

	usedVRAM := float64(uintVal) / (1024 * 1024)
	return usedVRAM, nil
}

func getAllUsedVRAM() (map[string]float64, error) {
	result := make(map[string]float64)
	nodesPath := "/sys/class/kfd/kfd/topology/nodes"

	entries, err := os.ReadDir(nodesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read nodes directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		nodeid := entry.Name()
		usedVRAM, err := getUsedVRAM(nodeid)
		if err != nil {
			// Optionally log error and continue
			continue
		}
		result[nodeid] = usedVRAM
	}
	return result, nil
}

// FindAMDGPUDevices scans the system for AMDGPU XCP devices and returns a map
// where the key is render id and value is device name "amdgpu_xcp_N"
func FindAMDGPUDevices() (map[string]string, error) {
	result := make(map[string]string)

	basePattern := "/sys/devices/platform/amdgpu_xcp_*/drm/renderD*"
	matches, err := filepath.Glob(basePattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}

	// Regex to extract amdgpu_xcpN and renderDN
	xcpRe := regexp.MustCompile(`amdgpu_xcp_(\d+)`)
	renderRe := regexp.MustCompile(`renderD(\d+)`)

	for _, path := range matches {
		// Check if the path exists and is a directory or symlink
		if _, err := os.Stat(path); err != nil {
			continue
		}

		// Extract amdgpu_xcpN
		xcpMatch := xcpRe.FindStringSubmatch(path)
		renderMatch := renderRe.FindStringSubmatch(path)
		if len(xcpMatch) < 2 || len(renderMatch) < 2 {
			continue
		}

		xcpVal := AMDLogicalDevicePrefix + xcpMatch[1]
		renderKey, err := strconv.Atoi(renderMatch[1])
		if err != nil {
			continue
		}

		renderStr := fmt.Sprintf("%v", renderKey)
		result[renderStr] = xcpVal
	}

	return result, nil
}

type FsysDevice struct {
	mu      sync.Mutex
	lgpuMap map[string]string
}

var FsysDeviceHandler *FsysDevice

func GetFsysDeviceHandler() *FsysDevice {
	if FsysDeviceHandler == nil {
		FsysDeviceHandler = &FsysDevice{
			lgpuMap: make(map[string]string),
		}
		FsysDeviceHandler.init()
	}
	return FsysDeviceHandler
}

func (fs *FsysDevice) init() {
	lgpuMap, err := FindAMDGPUDevices()
	if err != nil {
		logger.Log.Printf("FindAMDGPUDevices error :%v", err)
		return
	}
	fs.lgpuMap = lgpuMap
}

func (fs *FsysDevice) GetDeviceNameFromRenderID(renderId string) (string, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if devicename, ok := fs.lgpuMap[renderId]; ok {
		return devicename, nil
	}
	return "", fmt.Errorf("device not found")
}

func (fs *FsysDevice) GetUsedVRAM(nodeid string) (float64, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return getUsedVRAM(nodeid)
}

func (fs *FsysDevice) GetAllUsedVRAM() (map[string]float64, error) {
	return getAllUsedVRAM()
}
