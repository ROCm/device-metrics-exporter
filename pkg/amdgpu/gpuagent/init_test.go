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
	mockCtl          *gomock.Controller
	gpuMockCl        *mock_gen.MockGPUSvcClient
	eventMockCl      *mock_gen.MockEventSvcClient
	slurmSchedMockCl *mock_gen.MockSchedulerClient
	k8sSchedMockCl   *mock_gen.MockSchedulerClient
	ifoeMockCl       *mock_gen.MockUALSvcClient
	mh               *metricsutil.MetricsHandler
	mConfig          *config.ConfigHandler
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
	slurmSchedMockCl = mock_gen.NewMockSchedulerClient(mockCtl)
	k8sSchedMockCl = mock_gen.NewMockSchedulerClient(mockCtl)
	ifoeMockCl := mock_gen.NewMockUALSvcClient(mockCtl)

	// setup gpu mock responses
	gpumock_resp := &amdgpu.GPUGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response: []*amdgpu.GPU{
			{
				Spec: &amdgpu.GPUSpec{
					Id: []byte(uuid.New().String()),
				},
				Status: &amdgpu.GPUStatus{
					SerialNum: "mock-serial",
					PCIeStatus: &amdgpu.GPUPCIeStatus{
						PCIeBusId: "pcie0",
					},
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
					PCIeStatus: &amdgpu.GPUPCIeStatus{
						PCIeBusId: "pcie1",
					},
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

	cper_mockresp := &amdgpu.GPUCPERGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		CPER: []*amdgpu.GPUCPEREntry{
			{
				GPU: []byte("72ff740f-0000-1000-804c-3b58bf67050e"),
				CPEREntry: []*amdgpu.CPEREntry{
					{
						RecordId:         "1",
						Severity:         amdgpu.CPERSeverity_CPER_SEVERITY_FATAL,
						Revision:         1,
						Timestamp:        "2025-10-09 05:09:13",
						NotificationType: amdgpu.CPERNotificationType_CPER_NOTIFICATION_TYPE_CMC,
						AFId:             []uint64{30, 34},
					},
				},
			},
		},
	}

	// setup ifoe mock responses
	ifoe_mockresp := &amdgpu.UALNetworkPortGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response: []*amdgpu.UALNetworkPort{
			{
				Spec: &amdgpu.UALNetworkPortSpec{
					Id:         []byte("ual-port-1"),
					UALStation: []byte("station-1"),
				},
				Status: &amdgpu.UALNetworkPortStatus{
					Name:           "ual-port-1",
					LogicalIndex:   32,
					LocalPortIndex: 1,
					OperState:      amdgpu.UALPortState_UAL_PORT_STATE_ENABLED,
				},
				Stats: &amdgpu.UALNetworkPortStats{},
			},
		},
	}

	ifoe_mockstationresp := &amdgpu.UALStationGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response: []*amdgpu.UALStation{
			{
				Spec: &amdgpu.UALStationSpec{
					Id: []byte("station-1"),
				},
				Status: &amdgpu.UALStationStatus{
					Name: "ual-station-1",
				},
			},
		},
	}

	ifoe_mockdeviceresp := &amdgpu.UALDeviceGetResponse{
		ApiStatus: amdgpu.ApiStatus_API_STATUS_OK,
		Response: []*amdgpu.UALDevice{
			{
				Spec: &amdgpu.UALDeviceSpec{
					Id: []byte(uuid.New().String()),
				},
			},
		},
	}

	gpuMockCl.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(gpumock_resp, nil).AnyTimes()
	gpuMockCl.EXPECT().GPUCPERGet(gomock.Any(), gomock.Any()).Return(cper_mockresp, nil).AnyTimes()
	eventMockCl.EXPECT().EventGet(gomock.Any(), gomock.Any()).Return(event_mockcriticalresp, nil).AnyTimes()
	ifoeMockCl.EXPECT().UALNetworkPortGet(gomock.Any(), gomock.Any()).Return(ifoe_mockresp, nil).AnyTimes()
	ifoeMockCl.EXPECT().UALStationGet(gomock.Any(), gomock.Any()).Return(ifoe_mockstationresp, nil).AnyTimes()
	ifoeMockCl.EXPECT().UALDeviceGet(gomock.Any(), gomock.Any()).Return(ifoe_mockdeviceresp, nil).AnyTimes()

	mConfig = config.NewConfigHandler("config.json", globals.GPUAgentPort)

	mh, _ = metricsutil.NewMetrics(mConfig)
	mh.InitConfig()

	return func(t *testing.T) {
		t.Logf("============= Test:TearDown %v ===============", t.Name())
		mockCtl.Finish()
	}
}

func newSlurmMockClient() scheduler.SchedulerClient {
	workload := map[string]scheduler.Workload{
		"0": scheduler.Workload{
			Type: scheduler.Slurm,
			Info: scheduler.JobInfo{
				Id:        "SLURM_JOB_ID0",
				User:      "SLURM_JOB_USER0",
				Partition: "SLURM_JOB_PARTITION0",
				Cluster:   "SLURM_CLUSTER_NAME0",
			},
		},
		"1": scheduler.Workload{
			Type: scheduler.Slurm,
			Info: scheduler.JobInfo{
				Id:        "SLURM_JOB_ID1",
				User:      "SLURM_JOB_USER1",
				Partition: "SLURM_JOB_PARTITION1",
				Cluster:   "SLURM_CLUSTER_NAME",
			},
		},
	}
	slurmSchedMockCl.EXPECT().ListWorkloads().Return(workload, nil).AnyTimes()
	slurmSchedMockCl.EXPECT().CheckExportLabels(gomock.Any()).Return(true).AnyTimes()
	slurmSchedMockCl.EXPECT().Type().Return(scheduler.Slurm).AnyTimes()
	slurmSchedMockCl.EXPECT().Close().Return(nil).Times(1)
	return slurmSchedMockCl
}

func newK8sSchedulerMock() scheduler.SchedulerClient {
	workload := map[string]scheduler.Workload{
		"pcie0": scheduler.Workload{
			Type: scheduler.Kubernetes,
			Info: scheduler.PodResourceInfo{
				Pod:       "pod0",
				Namespace: "Namespace0",
				Container: "ContainerName0",
			},
		},
		"pcie1": scheduler.Workload{
			Type: scheduler.Kubernetes,
			Info: scheduler.PodResourceInfo{
				Pod:       "pod1",
				Namespace: "Namespace1",
				Container: "ContainerName1",
			},
		},
	}
	k8sSchedMockCl.EXPECT().ListWorkloads().Return(workload, nil).AnyTimes()
	k8sSchedMockCl.EXPECT().CheckExportLabels(gomock.Any()).Return(true).AnyTimes()
	k8sSchedMockCl.EXPECT().Type().Return(scheduler.Slurm).AnyTimes()
	k8sSchedMockCl.EXPECT().Close().Return(nil).Times(1)
	return k8sSchedMockCl
}

func getNewAgent(t *testing.T) *GPUAgentClient {
	// setup zmq mock port
	ga := NewAgent(
		mh,
		WithK8sClient(nil),
		WithZmq(true),
		WithK8sSchedulerClient(nil),
		WithGPUMonitoring(true),
	)
	gpuClient, err := NewGPUAgentGPUClient(ga)
	if err != nil {
		t.Fatalf("error initializing gpu client: %v", err)
	}

	ga.Init()

	ga.gpuClient = gpuClient
	ga.gpuClient.gpuclient = gpuMockCl
	ga.gpuClient.evtclient = eventMockCl

	ga.isKubernetes = false

	return ga
}

func getNewAgentWithoutScheduler(t *testing.T) *GPUAgentClient {
	// setup zmq mock port
	ga := NewAgent(
		mh,
		WithK8sClient(nil),
		WithZmq(false),
		WithK8sSchedulerClient(nil),
		WithSlurmClient(false),
		WithGPUMonitoring(true),
	)

	gpuClient, err := NewGPUAgentGPUClient(ga)
	if err != nil {
		t.Fatalf("error initializing gpu client: %v", err)
	}
	ga.Init()

	ga.gpuClient = gpuClient
	ga.gpuClient.gpuclient = gpuMockCl
	ga.gpuClient.evtclient = eventMockCl
	ga.isKubernetes = false
	return ga
}

func getNewAgentWithOnlyIFOE(t *testing.T) *GPUAgentClient {
	// setup zmq mock port
	ga := NewAgent(
		mh,
		WithK8sClient(nil),
		WithZmq(false),
		WithK8sSchedulerClient(nil),
		WithSlurmClient(false),
		WithGPUMonitoring(false),
		WithIFOEMonitoring(true),
	)

	ga.gpuClient = nil
	ga.isKubernetes = false
	ifoeClient, err := NewGPUAgentIFOEClient(ga)
	if err != nil {
		t.Fatalf("error initializing ifoe client: %v", err)
	}
	ga.Init()

	ga.ifoeClient = ifoeClient
	ga.ifoeClient.ualClient = ifoeMockCl
	return ga
}
