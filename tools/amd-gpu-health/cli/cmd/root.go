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

package cmd

import (
	"os"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	configFile string
)

var RootCmd = &cobra.Command{
	Use:   "amd-gpu-health",
	Short: "AMD GPU Health checker CLI",
	Long:  "AMD GPU Health checker CLI",
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	logger.SetLogDir(globals.GPUHealthCheckerLogDir)
	logger.SetLogFile(globals.GPUHealthCheckerLogFile)
	logger.EnableLogFallback()
	logger.Init(false)
	RootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Config file (default is $HOME/.cobra.yaml)")
}

func initConfig() {
	// code for any config initiaization goes here
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".cobra")
	}
	if err := viper.ReadInConfig(); err == nil {
		logger.Log.Printf("amdgpuhealth is using config file:%v", viper.ConfigFileUsed())
	}
}
