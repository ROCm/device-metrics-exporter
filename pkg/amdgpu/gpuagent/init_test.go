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
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gotest.tools/assert"
)

var (
	mockCtl     *gomock.Controller
	gpuMockCl   *mock_gen.MockGPUSvcClient
	eventMockCl *mock_gen.MockEventSvcClient
	mh          *metricsutil.MetricsHandler
	mConfig     *config.ConfigHandler
)

func setupTest(t *testing.T) func(t *testing.T) {
	t.Logf("============= TestSetup %v ===============", t.Name())

	fmt.Println("LOGDIR", os.Getenv("LOGDIR"))

	logger.Init(true)

	dir := path.Dir(globals.SlurmDir)
	t.Logf("setting up slurmdir %v", dir)
	err := os.MkdirAll(dir, 0644)
	assert.Assert(t, err == nil, "error setting up slurmdir : %v", err)

	mockCtl = gomock.NewController(t)

	gpuMockCl = mock_gen.NewMockGPUSvcClient(mockCtl)
	eventMockCl = mock_gen.NewMockEventSvcClient(mockCtl)

	gpumock_resp := &amdgpu.GPUGetResponse{
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

	event_mockcriticalresp := &amdgpu.EventResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Event: []*amdgpu.Event{
			{
				Id:       1,
				Category: 1,
				Severity: 4,
				Time:     timestamppb.New(time.Now()),
				GPU:      []byte("72ff740f-0000-1000-804c-3b58bf67050e"),
			},
		},
	}

	gomock.InOrder(
		gpuMockCl.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(gpumock_resp, nil).AnyTimes(),
		eventMockCl.EXPECT().EventGet(gomock.Any(), gomock.Any()).Return(event_mockcriticalresp, nil).AnyTimes(),
	)

	mConfig = config.NewConfigHandler("config.json", globals.GPUAgentPort)

	mh, _ = metricsutil.NewMetrics(mConfig)
	mh.InitConfig()

	return func(t *testing.T) {
		t.Logf("============= Test:TearDown %v ===============", t.Name())
		mockCtl.Finish()
	}
}

func getNewAgent(t *testing.T) *GPUAgentClient {
	// setup zmq mock port
	ga := NewAgent(mh, nil, true)
	ga.initializeContext()
	ga.gpuclient = gpuMockCl
	ga.evtclient = eventMockCl
	schedulerCl, err := scheduler.NewSlurmClient(ga.ctx, ga.enableZmq)
	assert.Assert(t, err == nil, "error creating new agent : %v", err)
	ga.slurmScheduler = schedulerCl
	ga.isKubernetes = false
	return ga
}
