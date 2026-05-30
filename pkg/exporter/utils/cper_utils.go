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

package utils

import (
	"os"
	"strings"
	"sync"
)

const cperEnableEnvVar = "AMD_METRICS_EXPORTER_ENABLE_CPER"

var (
	cperEnabledOnce sync.Once
	cperEnabled     bool
)

// IsCperEnabled reports whether CPER fetching is enabled.
// CPER is OFF by default; opt in by setting AMD_METRICS_EXPORTER_ENABLE_CPER
// to 1, true, yes, or on. The env var is resolved once on first call.
func IsCperEnabled() bool {
	cperEnabledOnce.Do(func() {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(cperEnableEnvVar))) {
		case "1", "true", "yes", "on":
			cperEnabled = true
		}
	})
	return cperEnabled
}
