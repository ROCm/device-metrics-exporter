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

package e2e

import (
	"net/http"

	testutils "github.com/ROCm/device-metrics-exporter/test/utils"
	check "gopkg.in/check.v1"
)

// E2ESuite e2e config
type E2ESuite struct {
	name           string
	exporter       Exporter
	exporterClient *http.Client
	tu             *testutils.TestUtils
	configPath     string
	e2eConfig      *E2EConfig
	// setupC holds the SetUpTest's *C which shares the same logb as the test method's *C.
	// check.v1 creates separate *C instances for fixtures vs test methods, so c.Failed()
	// in TearDownTest never reflects the test's actual status. Instead, we check the shared
	// log buffer for error output written by assert calls during the test.
	setupC *check.C
}

type E2EConfig struct {
	ContainerName string `json:"ExporterName"`
	Mode          string `json:"E2EMode"`
	ImageURL      string `json:"ImageName"`
}
