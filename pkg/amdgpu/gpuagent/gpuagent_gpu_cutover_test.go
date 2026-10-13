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
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
	"gotest.tools/assert"
)

// renderGPUMetricsText gathers the full prefix-wrapped /metrics exposition for a
// freshly-initialized non-high-res GPU agent. Store-serving is unconditional, so
// this always exercises the single store-collector path wired into the live
// registry.
func renderGPUMetricsText(t *testing.T) string {
	t.Helper()

	ga := getNewAgent(t)
	defer ga.Close()
	assert.NilError(t, ga.InitConfigs())
	assert.NilError(t, ga.UpdateMetricsStats(context.Background()))

	mfs, err := mh.GetRegistry().Gather()
	assert.NilError(t, err)
	assert.Assert(t, len(mfs) > 0, "no metric families gathered")

	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		assert.NilError(t, enc.Encode(mf))
	}
	return buf.String()
}

// TestGPUStoreServedMetrics gates the serving path: non-high-res /metrics is served
// from the snapshot store (the only path after the flag was removed). The live
// registry must render the prefixed GPU field set end-to-end through the store
// collector wired into mh's registry, including a populated telemetry field and the
// always-on node total.
func TestGPUStoreServedMetrics(t *testing.T) {
	teardown := setupTest(t)
	defer teardown(t)

	out := renderGPUMetricsText(t)
	prefix := mh.GetPrefix()

	assert.Assert(t, strings.Contains(out, prefix+gpuNodesTotalField),
		"store-served /metrics missing node total %q", prefix+gpuNodesTotalField)
	assert.Assert(t, strings.Contains(out, prefix+"gpu_package_power"),
		"store-served /metrics missing gpu_package_power")
}
