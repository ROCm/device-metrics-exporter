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
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/config"
	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"gotest.tools/assert"
)

var (
	mock_resp *amdgpu.GPUGetResponse
	mockCtl   *gomock.Controller
	gpuMockCl *mock_gen.MockGPUSvcClient
	mh        *metricsutil.MetricsHandler
	mConfig   *config.Config
)

func setupTest(t *testing.T) func(t *testing.T) {
	t.Logf("============= TestSetup %v ===============", t.Name())

	fmt.Println("LOGDIR", os.Getenv("LOGDIR"))

	logger.Init()

	mockCtl = gomock.NewController(t)

	gpuMockCl = mock_gen.NewMockGPUSvcClient(mockCtl)

	mock_resp = &amdgpu.GPUGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response: []*amdgpu.GPU{
			{
				Spec: &amdgpu.GPUSpec{
					Id: []byte(uuid.New().String()),
				},
				Status: &amdgpu.GPUStatus{
					SerialNum: "mock-serial",
				},
				Stats: &amdgpu.GPUStats{
					PackagePower: 41,
				},
			},
			{
				Spec: &amdgpu.GPUSpec{
					Id: []byte(uuid.New().String()),
				},
				Status: &amdgpu.GPUStatus{
					SerialNum: "mock-serial-2",
				},
				Stats: &amdgpu.GPUStats{
					PackagePower: 41,
				},
			},
		},
	}

	gomock.InOrder(
		gpuMockCl.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(mock_resp, nil).AnyTimes(),
	)

	mConfig = config.NewConfig("config.json")
	mConfig.SetAgentPort(globals.GPUAgentPort)

	mh, _ = metricsutil.NewMetrics(mConfig)
	mh.InitConfig()

	return func(t *testing.T) {
		t.Logf("============= Test:TearDown %v ===============", t.Name())
		mockCtl.Finish()
	}
}

func getNewAgent(t *testing.T) *GPUAgentClient {
	ga, err := NewAgent(context.Background(), mh)
	assert.Assert(t, err == nil, "error creating new agent : %v", err)
	ga.client = gpuMockCl
	return ga
}
