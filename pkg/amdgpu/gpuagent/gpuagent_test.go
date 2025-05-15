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
	"testing"

	"gotest.tools/assert"
)

func TestGpuAgent(t *testing.T) {
	teardownSuite := setupTest(t)
	defer teardownSuite(t)

	ga := getNewAgent(t)
	defer ga.Close()
	t.Logf("gpuagent : %+v", ga)

	req, err := ga.getGPUs()
	assert.Assert(t, err == nil, "expecting nil response")

	t.Logf("req :%+v", req)

	err = ga.InitConfigs()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateStaticMetrics()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.UpdateMetricsStats()
	assert.Assert(t, err == nil, "expecting success config init")

	err = ga.processHealthValidation()
	assert.Assert(t, err == nil, "expecting success health validation")

}
