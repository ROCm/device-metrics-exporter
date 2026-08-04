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

package metricsserver

import (
	"context"
	"fmt"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetricssvc"
	"google.golang.org/protobuf/types/known/emptypb"
	"gotest.tools/assert"
)

// MockHealthInterface is a mock implementation of the HealthInterface
type MockHealthInterface struct {
	nicHealthStateMap map[string]interface{}
	err               error
}

func (m *MockHealthInterface) GetNICHealthStates() (map[string]interface{}, error) {
	return m.nicHealthStateMap, m.err
}

func TestNewMetricsServer(t *testing.T) {
	server := NewMetricsServer()
	assert.Assert(t, server != nil, "expected server to be created")
	assert.Equal(t, 0, len(server.clients), "expected no clients registered initially")
}

func TestRegisterHealthClient(t *testing.T) {
	server := NewMetricsServer()
	mockClient := &MockHealthInterface{}
	err := server.RegisterHealthClient(mockClient)
	assert.Assert(t, err == nil, "expected no error when registering client")
	assert.Equal(t, 1, len(server.clients), "expected one client to be registered")
	assert.Equal(t, mockClient, server.clients[0])
}

func TestList(t *testing.T) {
	server := NewMetricsServer()
	// mock client with valid NIC health states
	mockClient := &MockHealthInterface{
		nicHealthStateMap: map[string]interface{}{
			"nic1": &nicmetricssvc.NICState{
				UUID:   "uuid1",
				Device: "device1",
				Health: "healthy",
			},
		},
		err: nil,
	}
	server.RegisterHealthClient(mockClient)
	resp, err := server.List(context.Background(), &emptypb.Empty{})
	assert.Assert(t, err == nil, "expected no error from List")
	assert.Assert(t, resp != nil, "expected response to be non-nil")
	assert.Equal(t, 1, len(resp.NICState))
	assert.Equal(t, "uuid1", resp.NICState[0].UUID)
	assert.Equal(t, "device1", resp.NICState[0].Device)
	assert.Equal(t, "healthy", resp.NICState[0].Health)

	// mock client with error should fail List() call
	mockClientWithError := &MockHealthInterface{
		nicHealthStateMap: nil,
		err:               fmt.Errorf("mock error"),
	}
	server.RegisterHealthClient(mockClientWithError)
	resp, err = server.List(context.Background(), &emptypb.Empty{})
	assert.Assert(t, err != nil, "expected error from List due to mock error")
	assert.Assert(t, resp == nil, "expected response to be nil due to error")
}
