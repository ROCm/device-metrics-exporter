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
package exporter

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

func copyFilesToHost() {
	if _, err1 := os.Stat(globals.AMDGPUHealthContainerPath); err1 == nil {
		if dir, err := os.Stat(globals.AMDGPUHealthHostDirPath); err == nil && dir.IsDir() {
			cmd := exec.Command("cp", globals.AMDGPUHealthContainerPath, globals.AMDGPUHealthHostDirPath)
			err2 := cmd.Run()
			if err2 != nil {
				logger.Log.Printf("Unable to copy amdgpuhealth binary to host")
			}
		}
	}
}

func removeIfExists(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Log.Printf("failed to remove %s: %v", path, err)
	} else if err == nil {
		logger.Log.Printf("removed %s", path)
	}
}

func cleanupResources(enableGPUMonitoring, enableNICMonitoring bool) {
	if enableGPUMonitoring {
		removeIfExists(globals.MetricsSocketPath)
	}
	if enableNICMonitoring {
		removeIfExists(globals.NICMetricsSocketPath)
	}
	if utils.IsKubernetes() {
		removeIfExists(filepath.Join(globals.AMDGPUHealthHostDirPath, filepath.Base(globals.AMDGPUHealthContainerPath)))
	}
}
