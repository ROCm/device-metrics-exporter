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
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MetricsSvcImpl struct {
	sync.Mutex
	nicmetricssvc.UnimplementedMetricsServiceServer
	clients []HealthInterface
}

func NewMetricsServer() *MetricsSvcImpl {
	msrv := &MetricsSvcImpl{
		clients: []HealthInterface{},
	}
	return msrv
}

func (m *MetricsSvcImpl) RegisterHealthClient(client HealthInterface) error {
	m.clients = append(m.clients, client)
	return nil
}

// GetNICHealthStates returns the health states of all NICs
func (m *MetricsSvcImpl) List(ctx context.Context, e *emptypb.Empty) (*nicmetricssvc.NICStateResponse, error) {
	m.Lock()
	defer m.Unlock()
	resp := &nicmetricssvc.NICStateResponse{
		NICState: []*nicmetricssvc.NICState{},
	}
	for _, client := range m.clients {
		nicHealthStateMap, err := client.GetNICHealthStates()
		if err != nil {
			return nil, err
		}
		if nicHealthStateMap == nil {
			logger.Log.Printf("no NIC health states found")
			continue
		}
		for _, state := range nicHealthStateMap {
			if nicState, ok := state.(*nicmetricssvc.NICState); ok {
				resp.NICState = append(resp.NICState, &nicmetricssvc.NICState{
					UUID:   nicState.UUID,
					Device: nicState.Device,
					Health: nicState.Health,
				})
			}
		}
	}
	return resp, nil
}

// nolint:unused // mustEmbedUnimplementedMetricsServiceServer is kept for future use
func (m *MetricsSvcImpl) mustEmbedUnimplementedMetricsServiceServer() {}
