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

package metricsserver

import (
	"context"
	"fmt"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MetricsSvcImpl struct {
	sync.Mutex
	enableDebugAPI bool
	metricssvc.UnimplementedMetricsServiceServer
	clients []HealthInterface
}

// GetGPUState retrieves the GPU states for the specified IDs from all registered clients
func (m *MetricsSvcImpl) GetGPUState(ctx context.Context, req *metricssvc.GPUGetRequest) (*metricssvc.GPUStateResponse, error) {
	m.Lock()
	defer m.Unlock()
	resp := &metricssvc.GPUStateResponse{
		GPUState: []*metricssvc.GPUState{},
	}
	for _, client := range m.clients {
		gpuStateMap, err := client.GetGPUHealthStates()
		if err != nil {
			return nil, err
		}
		for _, id := range req.ID {
			if gstate, ok := gpuStateMap[id]; ok {
				state := gstate.(*metricssvc.GPUState)
				resp.GPUState = append(resp.GPUState, state)
			}
		}
	}
	return resp, nil
}

// List returns the GPU states for all registered clients
func (m *MetricsSvcImpl) List(ctx context.Context, e *emptypb.Empty) (*metricssvc.GPUStateResponse, error) {
	m.Lock()
	defer m.Unlock()
	resp := &metricssvc.GPUStateResponse{
		GPUState: []*metricssvc.GPUState{},
	}
	for _, client := range m.clients {
		gpuStateMap, err := client.GetGPUHealthStates()
		if err != nil {
			return nil, err
		}
		for _, gstate := range gpuStateMap {
			state := gstate.(*metricssvc.GPUState)
			resp.GPUState = append(resp.GPUState, state)
		}
	}
	return resp, nil
}

// SetError is a debug API to set GPU error state
func (m *MetricsSvcImpl) SetError(ctx context.Context, req *metricssvc.GPUErrorRequest) (*metricssvc.GPUErrorResponse, error) {

	if !m.enableDebugAPI {
		return nil, fmt.Errorf("invalid function error")
	}

	m.Lock()
	defer m.Unlock()
	if len(req.Fields) != len(req.Counts) {
		return nil, fmt.Errorf("invalid request, fields must be set")
	}
	for _, client := range m.clients {
		_ = client.SetError(req.ID, req.Fields, req.Counts)
	}
	resp := &metricssvc.GPUErrorResponse{
		ID:     req.ID,
		Fields: req.Fields,
	}
	return resp, nil
}

// nolint:unused // mustEmbedUnimplementedMetricsServiceServer is kept for future use
func (m *MetricsSvcImpl) mustEmbedUnimplementedMetricsServiceServer() {}

// NewMetricsServer creates a new instance of MetricsSvcImpl
func NewMetricsServer(enableDebugAPI bool) *MetricsSvcImpl {
	msrv := &MetricsSvcImpl{
		enableDebugAPI: enableDebugAPI,
		clients:        []HealthInterface{},
	}
	return msrv
}

// RegisterHealthClient registers a new HealthInterface client to the Metrics Service
func (m *MetricsSvcImpl) RegisterHealthClient(client HealthInterface) error {
	m.clients = append(m.clients, client)
	return nil
}
