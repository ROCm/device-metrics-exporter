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
	"os/signal"
	"path"
	"sync"
	"syscall"

	"google.golang.org/grpc"

	gpumetricsserver "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/metricsserver"
	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetricssvc"
	nicmetricsserver "github.com/ROCm/device-metrics-exporter/pkg/amdnic/metricsserver"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
)

// SvcHandler is a struct that manages the gRPC server and metrics services.
type SvcHandler struct {
	grpc                *grpc.Server
	mh                  *metricsutil.MetricsHandler
	gpuHealthSvc        *gpumetricsserver.MetricsSvcImpl
	nicHealthSvc        *nicmetricsserver.MetricsSvcImpl
	enableNICMonitoring bool
	enableGPUMonitoring bool
	enableDebugAPI      bool
	serverWg            sync.WaitGroup
	errChan             chan error
}

// SvcHandlerOption set desired option
type SvcHandlerOption func(s *SvcHandler)

// WithNICMonitoring is an option to enable or disable the NIC agent.
func WithNICMonitoring(enableNICMonitoring bool) SvcHandlerOption {
	return func(s *SvcHandler) {
		s.enableNICMonitoring = enableNICMonitoring
	}
}

// WithDebugAPIOption is an option to enable or disable the debug API.
func WithDebugAPIOption(enableDebugAPI bool) SvcHandlerOption {
	return func(s *SvcHandler) {
		s.enableDebugAPI = enableDebugAPI
	}
}

// WithGPUMonitoring is an option to enable or disable the GPU agent.
func WithGPUMonitoring(enableGPUMonitoring bool) SvcHandlerOption {
	return func(s *SvcHandler) {
		s.enableGPUMonitoring = enableGPUMonitoring
	}
}

// InitSvcs initializes the service handler with gRPC server and metrics services.
func InitSvcs(mh *metricsutil.MetricsHandler, opts ...SvcHandlerOption) *SvcHandler {
	svcHandler := &SvcHandler{
		mh:      mh,
		grpc:    grpc.NewServer(),
		errChan: make(chan error, 2), // Buffered channel for 2 potential error from 2 listeners
	}
	for _, o := range opts {
		o(svcHandler)
	}

	svcHandler.gpuHealthSvc = gpumetricsserver.NewMetricsServer(svcHandler.enableDebugAPI)
	svcHandler.nicHealthSvc = nicmetricsserver.NewMetricsServer(svcHandler.enableDebugAPI)
	return svcHandler
}

// RegisterGPUHealthClient registers a GPU health client with the GPU metrics service.
func (s *SvcHandler) RegisterGPUHealthClient(client gpumetricsserver.HealthInterface) error {
	return s.gpuHealthSvc.RegisterHealthClient(client)
}

// RegisterNICHealthClient registers a NIC health client with the NIC metrics service.
func (s *SvcHandler) RegisterNICHealthClient(client nicmetricsserver.HealthInterface) error {
	return s.nicHealthSvc.RegisterHealthClient(client)
}

// Stop stops the gRPC server.
func (s *SvcHandler) Stop() {
	if s.grpc != nil {
		logger.Log.Printf("stopping Health gRPC server")
		s.grpc.GracefulStop()
		s.grpc = nil
	}
}

// Run starts the gRPC server and listens for incoming connections on the specified sockets.
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

	// register all the services with the gRPC server
	// all the services should be registered before starting the server
	if s.enableGPUMonitoring {
		metricssvc.RegisterMetricsServiceServer(s.grpc, s.gpuHealthSvc)
	}
	if s.enableNICMonitoring {
		nicmetricssvc.RegisterMetricsServiceServer(s.grpc, s.nicHealthSvc)
	}

	if s.enableGPUMonitoring {
		// start listening on the socket for GPU metrics
		gpuLis, err := s.listenOnSocket(globals.MetricsSocketPath)
		if err != nil {
			return fmt.Errorf("failed to listen on socket %s: %v", globals.MetricsSocketPath, err)
		}
		s.serverWg.Add(1)
		go s.startAndServeGRPC(gpuLis)
	}

	// start listening on the socket for NIC metrics if enabled
	if s.enableNICMonitoring {
		nicLis, err := s.listenOnSocket(globals.NICMetricsSocketPath)
		if err != nil {
			return fmt.Errorf("failed to listen on socket %s: %v", globals.NICMetricsSocketPath, err)
		}
		s.serverWg.Add(1)
		go s.startAndServeGRPC(nicLis)
	}

	// Wait for any server to report an error, or for a shutdown signal
	select {
	case err := <-s.errChan:
		// An error occurred in one of the serving goroutines
		logger.Log.Printf("gRPC server encountered an error: %v. Initiating graceful shutdown...", err)
		s.grpc.GracefulStop() // Gracefully stop all serving goroutines
		s.serverWg.Wait()     // Wait for all goroutines to finish
		return err
	case <-s.setupSignalHandler():
		// Received a termination signal (e.g., Ctrl+C, SIGTERM)
		logger.Log.Println("received termination signal. Initiating graceful shutdown...")
		s.grpc.GracefulStop() // Gracefully stop all serving goroutines
		s.serverWg.Wait()     // Wait for all goroutines to finish
		logger.Log.Println("all gRPC servers stopped gracefully.")
		return nil
	}
}

// listenOnSocket creates a Unix socket listener at the specified path.
func (s *SvcHandler) listenOnSocket(socketPath string) (net.Listener, error) {
	// Remove any existing socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove socket file: %v", err)
	}

	if err := os.MkdirAll(path.Dir(socketPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket file: %v", err)
	}

	logger.Log.Printf("starting listening on socket : %v", socketPath)
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port: %v", err)
	}
	// world readable socket
	if err = os.Chmod(socketPath, 0777); err != nil {
		logger.Log.Printf("socket %v chmod to 777 failed, set it on host", socketPath)
	}
	logger.Log.Printf("listening on socket %v", socketPath)

	return lis, nil
}

// setupSignalHandler sets up a listener for OS signals to trigger graceful shutdown.
func (s *SvcHandler) setupSignalHandler() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	// Listen for interrupt (Ctrl+C) and termination signals
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	return sigChan
}

// startAndServeGRPC starts a gRPC server on a given listener.
func (s *SvcHandler) startAndServeGRPC(lis net.Listener) {
	defer s.serverWg.Done()
	if err := s.grpc.Serve(lis); err != nil {
		// Send error to the channel, but only if the channel is not full
		select {
		case s.errChan <- fmt.Errorf("failed to serve on: %v, err: %v", lis.Addr().String(), err):
			// Error sent
		default:
			// Channel full/blocked, log directly
			logger.Log.Printf("error channel full, directly logging: failed to serve on: %v, err: %v", lis.Addr().String(), err)
		}
	}
	logger.Log.Printf("gRPC server on %s stopped.", lis.Addr().String())
}
