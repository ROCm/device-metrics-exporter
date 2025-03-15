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

var (
	metricsConfig = flag.String("amd-metrics-config", globals.AMDMetricsFile, "AMD metrics exporter config file")
	agentGrpcPort = flag.Int("agent-grpc-port", globals.GPUAgentPort, "Agent GRPC port")
	versionOpt    = flag.Bool("version", false, "show version")
)

func main() {
	flag.Parse()

	defer func() {
		if r := recover(); r != nil {
			logger.Log.Printf("panic occured: %+v", r)
			os.Exit(1)
		}
	}()

	if *versionOpt {
		fmt.Printf("Version : %v\n", Version)
		fmt.Printf("BuildDate: %v\n", BuildDate)
		fmt.Printf("GitCommit: %v\n", GitCommit)
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

	exporterHandler := exporter.NewExporter(*agentGrpcPort, *metricsConfig)

	enableDebugAPI := true // default
	if len(Publish) != 0 {
		enableDebugAPI = false
	}

	if enableDebugAPI {
		logger.Log.Printf("Debug APIs enabled")
	}
	exporterHandler.StartMain(enableDebugAPI)

}
