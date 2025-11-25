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
package gpuagent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

type GPUAgentIFOEClient struct {
	sync.Mutex
	gpuHandler             *GPUAgentClient
	metrics                *IFOEMetrics
	ualClient              amdgpu.UALSvcClient
	exportLabels           map[string]bool
	exportFieldMap         map[string]bool
	customLabelMap         map[string]string
	computeNodeHealthState bool
	fl                     *fieldLogger
	extraPodLabelsMap      map[string]string
	k8PodLabelsMap         map[string]map[string]string
	allowedCustomLabels    []string
	fieldMetricsMap        map[string]FieldMeta
	staticHostLabels       map[string]string
}

func NewGPUAgentIFOEClient(gpuHandler *GPUAgentClient) (*GPUAgentIFOEClient, error) {
	ifoeClient := &GPUAgentIFOEClient{
		gpuHandler:     gpuHandler,
		exportLabels:   map[string]bool{},
		customLabelMap: map[string]string{},
		allowedCustomLabels: []string{
			exportermetrics.MetricLabel_CLUSTER_NAME.String(),
		},
		fl: gpuHandler.fl,
	}
	return ifoeClient, nil
}

// nolint
func (ga *GPUAgentIFOEClient) Close() {
	// No op for now
}

// InitClients initializes the IFOE client
func (ga *GPUAgentIFOEClient) InitClients() error {
	conn := ga.gpuHandler.GetGRPCConnection()
	if conn == nil {
		return fmt.Errorf("gRPC connection is nil")
	}
	ga.ualClient = amdgpu.NewUALSvcClient(conn)
	return nil
}

// GetGPUHealthStates - no op
func (ga *GPUAgentIFOEClient) GetHealthStates() (map[string]interface{}, error) {
	return nil, nil
}

// SetError - no op
func (ga *GPUAgentIFOEClient) SetError(id string, fields []string, counts []uint32) error {
	return nil
}

// processHealthValidation - no op
func (ga *GPUAgentIFOEClient) processHealthValidation() error {
	return nil
}

// sendNodeLabelUpdate - no op
func (ga *GPUAgentIFOEClient) sendNodeLabelUpdate() error {
	return nil
}

// IsActive checks if the IFOE client is active
func (ga *GPUAgentIFOEClient) isActive() bool {
	return ga.ualClient != nil
}

func (ga *GPUAgentIFOEClient) GetContext() context.Context {
	ctx := ga.gpuHandler.GetContext()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (ga *GPUAgentIFOEClient) GetDeviceType() globals.DeviceType {
	return globals.IFOEDevice
}

func (ga *GPUAgentIFOEClient) GetExporterNonIFOELabels() []string {
	labelList := []string{
		strings.ToLower(exportermetrics.MetricLabel_HOSTNAME.String()),
	}
	// Add custom labels
	for label := range ga.customLabelMap {
		labelList = append(labelList, strings.ToLower(label))
	}
	return labelList
}

func (ga *GPUAgentIFOEClient) GetExportLabels() []string {
	labelList := []string{}
	for key, enabled := range ga.exportLabels {
		if !enabled {
			continue
		}
		labelList = append(labelList, strings.ToLower(key))
	}

	for key := range ga.extraPodLabelsMap {
		exists := false
		for _, label := range labelList {
			if key == label {
				exists = true
				break
			}
		}
		if !exists {
			labelList = append(labelList, key)
		}
	}

	for key := range ga.customLabelMap {
		exists := false
		for _, label := range labelList {
			if key == label {
				exists = true
				break
			}
		}

		// Add only unique labels to export labels
		if !exists {
			labelList = append(labelList, key)
		}
	}

	logger.Log.Printf("IFOE Export labels: %v", labelList)
	return labelList
}

func (ga *GPUAgentIFOEClient) listNetworkPort() (*amdgpu.UALNetworkPortGetResponse, error) {
	ctx := ga.GetContext()
	req := &amdgpu.UALNetworkPortGetRequest{}
	resp, err := ga.ualClient.UALNetworkPortGet(ctx, req)
	if err != nil {
		logger.Log.Printf("UALNetworkPortGet gRPC call failed: %v", err)
		return nil, err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return nil, fmt.Errorf("%v", resp.ApiStatus)
	}
	return resp, nil
}

func (ga *GPUAgentIFOEClient) listStation() (*amdgpu.UALStationGetResponse, error) {
	ctx := ga.GetContext()
	req := &amdgpu.UALStationGetRequest{}
	resp, err := ga.ualClient.UALStationGet(ctx, req)
	if err != nil {
		logger.Log.Printf("UALStationGet gRPC call failed: %v", err)
		return nil, err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return nil, fmt.Errorf("%v", resp.ApiStatus)
	}
	return resp, nil
}

func (ga *GPUAgentIFOEClient) listDevice() (*amdgpu.UALDeviceGetResponse, error) {
	ctx := ga.GetContext()
	req := &amdgpu.UALDeviceGetRequest{}
	resp, err := ga.ualClient.UALDeviceGet(ctx, req)
	if err != nil {
		logger.Log.Printf("UALDeviceGet gRPC call failed: %v", err)
		return nil, err
	}
	if resp != nil && resp.ApiStatus != 0 {
		logger.Log.Printf("resp status :%v", resp.ApiStatus)
		return nil, fmt.Errorf("%v", resp.ApiStatus)
	}
	return resp, nil
}

func (ga *GPUAgentIFOEClient) updateMockMetrics() error {
	ualPort := &amdgpu.UALNetworkPort{
		Spec: &amdgpu.UALNetworkPortSpec{},
		Status: &amdgpu.UALNetworkPortStatus{
			Name: "eth0",
		},
		Stats: &amdgpu.UALNetworkPortStats{
			NumFailedoverStreams:      3,
			NumPausedStreams:          4,
			BitErrorRate:              100,
			FECCodeWordSymbolErrors0:  1000,
			FECCodeWordSymbolErrors1:  900,
			FECCodeWordSymbolErrors2:  800,
			FECCodeWordSymbolErrors3:  700,
			FECCodeWordSymbolErrors4:  600,
			FECCodeWordSymbolErrors5:  500,
			FECCodeWordSymbolErrors6:  400,
			FECCodeWordSymbolErrors7:  300,
			FECCodeWordSymbolErrors8:  200,
			FECCodeWordSymbolErrors9:  100,
			FECCodeWordSymbolErrors10: 90,
			FECCodeWordSymbolErrors11: 80,
			FECCodeWordSymbolErrors12: 70,
			FECCodeWordSymbolErrors13: 60,
			FECCodeWordSymbolErrors14: 50,
			FECCodeWordSymbolErrors15: 40,
		},
	}

	labels := ga.populateLabelsFromObject(nil, nil, nil)

	ga.metrics.totalDevices.With(labels).Set(float64(1))
	ga.metrics.totalStations.With(labels).Set(float64(1))

	ifoeLabels := ga.populateLabelsFromObject(nil, nil, ualPort)
	ifoeLabels["station_uuid"] = "stationUuid"
	ifoeLabels["port_name"] = "portName"
	ifoeLabels["device_uuid"] = "devUuid"
	ga.metrics.totalNetworkPorts.With(labels).Set(float64(16))

	ga.metrics.numFailedoverStreams.With(ifoeLabels).Set(float64(3))
	ga.metrics.numPausedStreams.With(ifoeLabels).Set(float64(4))
	ga.metrics.bitErrorRate.With(ifoeLabels).Set(float64(100))
	ga.metrics.fecCodeWordSymbolErrors0.With(ifoeLabels).Set(float64(1000))
	ga.metrics.fecCodeWordSymbolErrors1.With(ifoeLabels).Set(float64(900))
	ga.metrics.fecCodeWordSymbolErrors2.With(ifoeLabels).Set(float64(800))
	ga.metrics.fecCodeWordSymbolErrors3.With(ifoeLabels).Set(float64(700))
	ga.metrics.fecCodeWordSymbolErrors4.With(ifoeLabels).Set(float64(600))
	ga.metrics.fecCodeWordSymbolErrors5.With(ifoeLabels).Set(float64(500))
	ga.metrics.fecCodeWordSymbolErrors6.With(ifoeLabels).Set(float64(400))
	ga.metrics.fecCodeWordSymbolErrors7.With(ifoeLabels).Set(float64(300))
	ga.metrics.fecCodeWordSymbolErrors8.With(ifoeLabels).Set(float64(200))
	ga.metrics.fecCodeWordSymbolErrors9.With(ifoeLabels).Set(float64(100))
	ga.metrics.fecCodeWordSymbolErrors10.With(ifoeLabels).Set(float64(90))
	ga.metrics.fecCodeWordSymbolErrors11.With(ifoeLabels).Set(float64(80))
	ga.metrics.fecCodeWordSymbolErrors12.With(ifoeLabels).Set(float64(70))
	ga.metrics.fecCodeWordSymbolErrors13.With(ifoeLabels).Set(float64(60))
	ga.metrics.fecCodeWordSymbolErrors14.With(ifoeLabels).Set(float64(50))
	ga.metrics.fecCodeWordSymbolErrors15.With(ifoeLabels).Set(float64(40))

	return nil
}

func (ga *GPUAgentIFOEClient) updateMetrics() error {
	labels := ga.populateLabelsFromObject(nil, nil, nil)

	resp, err := ga.listNetworkPort()
	if err != nil {
		return err
	}
	if resp != nil && resp.ApiStatus != 0 {
		ga.metrics.totalNetworkPorts.With(labels).Set(float64(0))
		logger.Log.Printf("UALNetworkPortGet api status :%v", resp.ApiStatus)
		return fmt.Errorf("UALNetworkPortGet api status: %v", resp.ApiStatus)
	}

	dresp, err := ga.listDevice()
	if err != nil {
		return err
	}
	if dresp != nil && dresp.ApiStatus != 0 {
		ga.metrics.totalDevices.With(labels).Set(float64(0))
		logger.Log.Printf("UALDeviceGet api status :%v", dresp.ApiStatus)
		return fmt.Errorf("UALDeviceGet api status: %v", dresp.ApiStatus)
	}

	sresp, err := ga.listStation()
	if err != nil {
		return err
	}
	if sresp != nil && sresp.ApiStatus != 0 {
		ga.metrics.totalStations.With(labels).Set(float64(0))
		logger.Log.Printf("UALStationGet api status :%v", sresp.ApiStatus)
		return fmt.Errorf("UALStationGet api status: %v", sresp.ApiStatus)
	}
	ualStationMap := make(map[string]*amdgpu.UALStation)
	for _, ualStation := range sresp.Response {
		uuid := utils.UUIDToString(ualStation.Spec.Id)
		ualStationMap[uuid] = ualStation
	}

	ga.metrics.totalNetworkPorts.With(labels).Set(float64(len(resp.Response)))
	ga.metrics.totalDevices.With(labels).Set(float64(len(dresp.Response)))
	ga.metrics.totalStations.With(labels).Set(float64(len(sresp.Response)))

	for _, ualPort := range resp.Response {
		ifoeLabels := ga.populateLabelsFromObject(nil, nil, ualPort)
		portUuid := utils.UUIDToString(ualPort.Spec.Id)
		portName := ""
		if ualPort.Status != nil {
			portName = ualPort.Status.Name
		}
		stationUuid := utils.UUIDToString(ualPort.Spec.UALStation)
		station, ok := ualStationMap[stationUuid]
		if !ok {
			continue
		}
		devUuid := utils.UUIDToString(station.Spec.UALDevice)
		// TBD : remove after testing
		logger.Log.Printf("Processing UALPort: %v, Station: %v Device: %v PortName: %s", portUuid, stationUuid, devUuid, portName)

		ifoeLabels["station_uuid"] = stationUuid
		ifoeLabels["port_name"] = portName
		ifoeLabels["device_uuid"] = devUuid

		stats := ualPort.Stats
		if stats != nil {
			ga.metrics.numFailedoverStreams.With(ifoeLabels).Set(float64(stats.NumFailedoverStreams))
			ga.metrics.numPausedStreams.With(ifoeLabels).Set(float64(stats.NumPausedStreams))
			ga.metrics.bitErrorRate.With(ifoeLabels).Set(float64(stats.BitErrorRate))
			ga.metrics.fecCodeWordSymbolErrors0.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors0))
			ga.metrics.fecCodeWordSymbolErrors1.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors1))
			ga.metrics.fecCodeWordSymbolErrors2.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors2))
			ga.metrics.fecCodeWordSymbolErrors3.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors3))
			ga.metrics.fecCodeWordSymbolErrors4.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors4))
			ga.metrics.fecCodeWordSymbolErrors5.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors5))
			ga.metrics.fecCodeWordSymbolErrors6.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors6))
			ga.metrics.fecCodeWordSymbolErrors7.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors7))
			ga.metrics.fecCodeWordSymbolErrors8.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors8))
			ga.metrics.fecCodeWordSymbolErrors9.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors9))
			ga.metrics.fecCodeWordSymbolErrors10.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors10))
			ga.metrics.fecCodeWordSymbolErrors11.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors11))
			ga.metrics.fecCodeWordSymbolErrors12.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors12))
			ga.metrics.fecCodeWordSymbolErrors13.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors13))
			ga.metrics.fecCodeWordSymbolErrors14.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors14))
			ga.metrics.fecCodeWordSymbolErrors15.With(ifoeLabels).Set(float64(stats.FECCodeWordSymbolErrors15))
		}
	}
	return nil
}
