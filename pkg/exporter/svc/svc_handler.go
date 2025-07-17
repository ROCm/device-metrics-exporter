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

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"google.golang.org/grpc"
)

type SvcHandler struct {
	grpc      *grpc.Server
	healthSvc *MetricsSvcImpl
	mh        *metricsutil.MetricsHandler
}

func InitSvcs(enableDebugAPI bool, mh *metricsutil.MetricsHandler) *SvcHandler {
	s := &SvcHandler{
		grpc:      grpc.NewServer(),
		healthSvc: newMetricsServer(enableDebugAPI),
		mh:        mh,
	}
	return s
}

func (s *SvcHandler) RegisterHealthClient(client HealthInterface) error {
	return s.healthSvc.RegisterHealthClient(client)
}

func (s *SvcHandler) Stop() {
	if s.grpc != nil {
		logger.Log.Printf("stopping Health gRPC server")
		s.grpc.GracefulStop()
		s.grpc = nil
	}
}

func (s *SvcHandler) Run() error {
	if s.mh != nil {
		if enabled := s.mh.GetHealthServiceState(); !enabled {
			logger.Log.Printf("health service is disabled")
			return nil
		}
	}

	if s.grpc == nil {
		logger.Log.Printf("creating new gRPC server")
		s.grpc = grpc.NewServer()
	}

	socketPath := globals.MetricsSocketPath
	// Remove any existing socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove socket file: %v", err)
	}

	if err := os.MkdirAll(path.Dir(socketPath), 0755); err != nil {
		return fmt.Errorf("failed to create socket file: %v", err)
	}

	logger.Log.Printf("starting listening on socket : %v", socketPath)
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on port: %v", err)
	}
	// world readable socket
	if err = os.Chmod(socketPath, 0777); err != nil {
		logger.Log.Printf("socket %v chmod to 777 failed, set it on host", socketPath)
	}
	logger.Log.Printf("listening on socket %v", socketPath)

	// server registration for grpc services
	metricssvc.RegisterMetricsServiceServer(s.grpc, s.healthSvc)

	if err := s.grpc.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}
