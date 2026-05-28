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
	"time"

	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

func TestLatestCPERPerGPUPicksNewestEntry(t *testing.T) {
	now := time.Now()
	resp := &amdgpu.GPUCPERGetResponse{
		CPER: []*amdgpu.GPUCPEREntry{{
			GPU: []byte("72ff740f-0000-1000-804c-3b58bf67050e"),
			CPEREntry: []*amdgpu.CPEREntry{
				{
					RecordId:  "old-fatal",
					Severity:  amdgpu.CPERSeverity_CPER_SEVERITY_FATAL,
					Timestamp: now.Add(-2 * time.Hour).Format(cperTimestampLayout),
				},
				{
					RecordId:  "new-corrected",
					Severity:  amdgpu.CPERSeverity_CPER_SEVERITY_NON_FATAL_CORRECTED,
					Timestamp: now.Format(cperTimestampLayout),
				},
			},
		}},
	}

	latest := latestCPERPerGPU(resp)
	assert.Equal(t, 1, len(latest))
	for _, entry := range latest {
		assert.Equal(t, "new-corrected", entry.RecordId)
	}
}

func TestIsFatalCPERActionableRespectsMaxAge(t *testing.T) {
	logger.Init(true)
	ga := &GPUAgentGPUClient{}

	recent := &amdgpu.CPEREntry{
		Severity:  amdgpu.CPERSeverity_CPER_SEVERITY_FATAL,
		Timestamp: time.Now().Format(cperTimestampLayout),
	}
	stale := &amdgpu.CPEREntry{
		Severity:  amdgpu.CPERSeverity_CPER_SEVERITY_FATAL,
		Timestamp: time.Now().Add(-2 * time.Hour).Format(cperTimestampLayout),
	}
	nonFatal := &amdgpu.CPEREntry{
		Severity:  amdgpu.CPERSeverity_CPER_SEVERITY_NON_FATAL_UNCORRECTED,
		Timestamp: time.Now().Format(cperTimestampLayout),
	}

	assert.Assert(t, ga.isFatalCPERActionable(recent, time.Hour))
	assert.Assert(t, !ga.isFatalCPERActionable(stale, time.Hour))
	assert.Assert(t, !ga.isFatalCPERActionable(nonFatal, time.Hour))
	assert.Assert(t, ga.isFatalCPERActionable(stale, 0))
}
