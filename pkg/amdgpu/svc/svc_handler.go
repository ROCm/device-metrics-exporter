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
	"fmt"
	"net"
	"os"
	"path"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/config"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
	"google.golang.org/grpc"
)

type SvcHandler struct {
	grpc      *grpc.Server
	healthSvc *MetricsSvcImpl
	config    *config.ConfigHandler
}

func InitSvcs(enableDebugAPI bool) *SvcHandler {
	s := &SvcHandler{
		grpc:      grpc.NewServer(),
		healthSvc: newMetricsServer(enableDebugAPI),
	}
	return s
}

func (s *SvcHandler) RegisterHealthClient(client HealthInterface) error {
	return s.healthSvc.RegisterHealthClient(client)
}

func (s *SvcHandler) Run() error {
	socketPath := globals.MetricsSocketPath
	// Remove any existing socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Failed to remove socket file: %v", err)
	}

	os.MkdirAll(path.Dir(socketPath), 0755)

	logger.Log.Printf("starting listening on socket : %v", socketPath)
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on port: %v", err)
	}
	// world readable socket
	if err = os.Chmod(socketPath, 777); err != nil {
		logger.Log.Printf("socket %v chmod to 777 failed, set it on host", socketPath)
	}
	logger.Log.Printf("Listening on socket %v", socketPath)

	// server registration for grpc services
	metricssvc.RegisterMetricsServiceServer(s.grpc, s.healthSvc)

	if err := s.grpc.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}
