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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

var (
	Version   string
	BuildDate string
	GitCommit string
	Publish   string
)

func main() {
	// Check environment variable to determine error handling behavior
	relaxedMode := os.Getenv("AMD_EXPORTER_RELAXED_FLAGS_PARSING") != ""

	var errorHandling flag.ErrorHandling
	if relaxedMode {
		errorHandling = flag.ContinueOnError
		fmt.Fprintf(os.Stderr, "Info: Relaxed flag parsing enabled via AMD_EXPORTER_RELAXED_FLAGS_PARSING\n")
	} else {
		errorHandling = flag.ExitOnError
	}

	fs := flag.NewFlagSet(os.Args[0], errorHandling)

	// Define our supported flags - these return pointers just like flag.String(), flag.Bool(), etc.
	metricsConfig := fs.String("amd-metrics-config", globals.AMDMetricsFile, "AMD metrics exporter config file")
	agentGrpcPort := fs.Int("agent-grpc-port", globals.GPUAgentPort, "Agent GRPC port")
	versionOpt := fs.Bool("version", false, "show version")
	bindAddr := fs.String("bind", "0.0.0.0", "bind address for metrics server (default: 0.0.0.0)")

	// Parse with error handling
	err := fs.Parse(os.Args[1:])
	if err != nil {
		// Log warnings for unsupported flags but continue
		fmt.Fprintf(os.Stderr, "Warning: %v - continuing with supported flags for backward compatibility\n", err)
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Log.Printf("panic occured: %+v", r)
			os.Exit(1)
		}
	}()

	deploymentType := "container deployment (not k8s)"
	if utils.IsKubernetes() {
		deploymentType = "k8s deployment"
	} else if utils.IsDebianInstall() {
		deploymentType = "debian package deployment"
	}

	if *versionOpt {
		fmt.Printf("Version : %v\n", Version)
		fmt.Printf("BuildDate: %v\n", BuildDate)
		fmt.Printf("GitCommit: %v\n", GitCommit)
		fmt.Printf("Deployment: %v\n", deploymentType)
		os.Exit(0)
	}

	if (0 >= *agentGrpcPort) || (*agentGrpcPort > 65535) {
		fmt.Printf("invalid agent-grpc-port exiting")
		os.Exit(1)
	}

	logger.Init(utils.IsKubernetes())

	logger.Log.Printf("Version : %v", Version)
	logger.Log.Printf("BuildDate: %v", BuildDate)
	logger.Log.Printf("GitCommit: %v", GitCommit)
	logger.Log.Printf("Deployment: %v", deploymentType)

	exporterHandler := exporter.NewExporter(*agentGrpcPort, *metricsConfig,
		exporter.WithBindAddr(*bindAddr),
	)

	enableDebugAPI := true // default
	if len(Publish) != 0 {
		enableDebugAPI = false
	}

	if enableDebugAPI {
		logger.Log.Printf("Debug APIs enabled")
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Log.Printf("Received signal: %v, shutting down...", sig)
		exporterHandler.Close()
		os.Exit(0)
	}()
	exporterHandler.StartMain(enableDebugAPI)

}
