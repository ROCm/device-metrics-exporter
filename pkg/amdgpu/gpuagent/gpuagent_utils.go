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

package gpuagent

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

type fieldLogger struct {
	unsupportedFieldMap map[string]bool
	filterDone          bool // filtering should be done only once on startup of new config
	sync.RWMutex
}

func NewFieldLogger() *fieldLogger {
	return &fieldLogger{
		unsupportedFieldMap: make(map[string]bool),
		RWMutex:             sync.RWMutex{},
	}
}

func (fl *fieldLogger) checkUnsupportedFields(gpuid, fieldName string) bool {
	fl.RLock()
	defer fl.RUnlock()
	if fl.unsupportedFieldMap == nil {
		return false
	}
	key := gpuid + "-" + fieldName
	_, exists := fl.unsupportedFieldMap[key]
	return exists
}

// logUnsupportedField logs the unsupported field name
// and adds it to the map of unsupported fields
// to avoid logging it again
func (fl *fieldLogger) logUnsupportedField(gpuid, fieldName string) {
	fl.Lock()
	defer fl.Unlock()
	if fl.filterDone {
		return
	}
	if fl.unsupportedFieldMap == nil {
		fl.unsupportedFieldMap = make(map[string]bool)
	}
	key := gpuid + "-" + fieldName
	if _, exists := fl.unsupportedFieldMap[key]; !exists {
		logger.Log.Printf("GPU %v Platform doesn't support field name: %s", gpuid, fieldName)
		fl.unsupportedFieldMap[key] = true
	}
}

func (fl *fieldLogger) logWithValidateAndExport(gpuid string, metrics prometheus.GaugeVec, fieldName string,
	labels map[string]string, value interface{}) {

	if fl.checkUnsupportedFields(gpuid, fieldName) {
		return
	}
	err := utils.ValidateAndExport(metrics, fieldName, labels, value)
	if err != utils.ErrorNone {
		if err == utils.ErrorNotApplicable {
			fl.logUnsupportedField(gpuid, fieldName)
		} else {
			logger.Log.Printf("Failed to export metric %s: %v", fieldName, err)
		}
	}
}

func (fl *fieldLogger) Reset() {
	fl.Lock()
	defer fl.Unlock()
	fl.filterDone = false
	fl.unsupportedFieldMap = make(map[string]bool)
}

func (fl *fieldLogger) SetFilterDone() {
	fl.Lock()
	defer fl.Unlock()
	fl.filterDone = true
	logger.Log.Println("Unsupported fields filtering completed.")
}
