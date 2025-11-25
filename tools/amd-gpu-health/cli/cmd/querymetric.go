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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	exputils "github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"github.com/ROCm/device-metrics-exporter/tools/amd-gpu-health/cli/utils"
)

var (
	metricName                string
	threshold                 int64
	gaugeMetricThreshold      float64
	metricDuration            string
	nodeName                  string
	exporterRootCAPath        string
	exporterBearerTokenPath   string
	clientCertPath            string
	prometheusBearerTokenPath string
	prometheusRootCAPath      string
	prometheusEndpointUrl     string
	severity                  string
	afid                      uint64
)

func fetchAuthInfo(cmd *cobra.Command) utils.AuthInfo {
	authInfo := utils.AuthInfo{}
	if cmd.Flags().Changed("exporter-root-ca") {
		authInfo.ExporterRootCAPath = exporterRootCAPath
	}
	if cmd.Flags().Changed("exporter-bearer-token") {
		authInfo.ExporterBearerTokenPath = exporterBearerTokenPath
	}
	if cmd.Flags().Changed("client-cert") {
		authInfo.ClientCertPath = clientCertPath
	}
	if cmd.Flags().Changed("prometheus-bearer-token") {
		authInfo.PrometheusBearerTokenPath = prometheusBearerTokenPath
	}
	if cmd.Flags().Changed("prometheus-root-ca") {
		authInfo.PrometheusRootCAPath = prometheusRootCAPath
	}
	return authInfo
}

type EnvironmentProvider interface {
	Initialize(cmd *cobra.Command) error
	GetMetricsEndpointURL() (string, error)
	GetInbandRASErrorsEndpointURL() (string, error)
}

type EnvProvider struct {
	configPath string
	nodeName   string
	isTLS      bool
	authInfo   utils.AuthInfo
	cmd        *cobra.Command
}

func (e *EnvProvider) Initialize(cmd *cobra.Command) error {
	e.cmd = cmd
	if !exputils.IsDebianInstall() {
		e.configPath = viper.GetString("kubeConfigPath")
		//get node name from env variable
		nodeName, err := exputils.GetHostName()
		if err != nil {
			logger.Log.Printf("unable to fetch node name. error:%v", err)
			return fmt.Errorf("unable to fetch node name")
		}
		e.nodeName = nodeName
		//fetch auth info
		e.authInfo = fetchAuthInfo(cmd)
		e.isTLS = e.authInfo.ExporterRootCAPath != ""
	}
	return nil
}

func (e *EnvProvider) GetMetricsEndpointURL() (string, error) {
	if exputils.IsDebianInstall() {
		return utils.GetMetricsEndpointURLLocalhost()
	}
	return utils.GetGPUMetricsEndpointURL(e.configPath, e.nodeName, e.isTLS)
}

func (e *EnvProvider) GetInbandRASErrorsEndpointURL() (string, error) {
	if exputils.IsDebianInstall() {
		return utils.GetInbandRASErrorsEndpointURLLocalhost()
	}
	return utils.GetGPUInbandRASErrorsURL(e.configPath, e.nodeName, e.isTLS)
}

var QueryMetricCmd = &cobra.Command{
	Use:   "query",
	Short: "query a metric",
	Long:  "query a metric",
}

var counterMetricCmd = &cobra.Command{
	Use:   "counter-metric",
	Short: "metric of type counter",
	Long:  "metric of type counter",
	Run:   counterMetricCmdHandler,
}

var gaugeMetricCmd = &cobra.Command{
	Use:   "gauge-metric",
	Short: "metric of type gauge",
	Long:  "metric of type gauge",
	Run:   gaugeMetricCmdHandler,
}

var inbandRasErrorsCmd = &cobra.Command{
	Use:   "inband-ras-errors",
	Short: "query inband ras errors",
	Long:  "query inband ras errors",
	Run:   inbandRasErrorsCmdHandler,
}

func counterMetricCmdHandler(cmd *cobra.Command, args []string) {
	envProvider := EnvProvider{}
	err := envProvider.Initialize(cmd)
	if err != nil {
		fmt.Printf("%s", err.Error())
		os.Exit(2)
	}
	metricsEndpoint, err := envProvider.GetMetricsEndpointURL()
	if err != nil || metricsEndpoint == "" {
		logger.Log.Printf("unable to get metrics endpoint url. error=%v", err)
		fmt.Printf("unable to get metrics endpoint url")
		os.Exit(2)
	}
	logger.Log.Printf("metrics endpoint url=%v", metricsEndpoint)
	// check if the below mandatory args are provided or not
	if !cmd.Flags().Changed("metric") {
		fmt.Printf("mandatory field metric missing in cli query")
		os.Exit(2)
	}

	// query metrics endpoint
	response, err := utils.QueryExporterEndpoint(metricsEndpoint, envProvider.authInfo)
	if err != nil {
		logger.Log.Printf("unable to query metrics endpoint. error:%v", err)
		if !exputils.IsDebianInstall() {
			utils.InvalidateExporterEndpointURLCache()
		}
		fmt.Printf("unable to query metrics endpoint")
		os.Exit(2)
	}
	logger.Log.Printf("counter metrics response=%s", response)
	metrics := utils.ParseGPUMetricsResponse(response, metricName)
	if len(metrics) == 0 {
		fmt.Printf("metric %s not found", metricName)
		os.Exit(2)
	}
	for _, metric := range metrics {
		if int64(metric) > threshold {
			//below message to stdout will be captured in Node condition message
			fmt.Printf("metric %s crossed threshold", metricName)
			os.Exit(1)
		}
	}
	if exputils.IsDebianInstall() {
		fmt.Printf("metric %s is within threshold\n", metricName)
	}
	os.Exit(0)
}

func gaugeMetricCmdHandler(cmd *cobra.Command, args []string) {
	envProvider := EnvProvider{}
	err := envProvider.Initialize(cmd)
	if err != nil {
		fmt.Printf("%s", err.Error())
		os.Exit(2)
	}
	//validate all the args
	if !cmd.Flags().Changed("metric") || !cmd.Flags().Changed("threshold") {
		fmt.Printf("mandatory fields missing in cli query")
		os.Exit(2)
	}

	var metrics []float64
	if exputils.IsDebianInstall() || !cmd.Flags().Changed("duration") {
		// if debian install or duration is not specified, query the latest value from local exporter endpoint.
		metricsEndpoint, err := envProvider.GetMetricsEndpointURL()
		if err != nil || metricsEndpoint == "" {
			logger.Log.Printf("unable to get metrics endpoint url. error=%v", err)
			fmt.Printf("unable to get metrics endpoint url")
			os.Exit(2)
		}
		response, err := utils.QueryExporterEndpoint(metricsEndpoint, envProvider.authInfo)
		if err != nil {
			logger.Log.Printf("unable to query metrics endpoint. error:%v", err)
			if !exputils.IsDebianInstall() {
				utils.InvalidateExporterEndpointURLCache()
			}
			fmt.Printf("unable to query metrics endpoint")
			os.Exit(2)
		}
		logger.Log.Printf("gauge metrics exporter endpoint response=%s", response)
		metrics = utils.ParseGPUMetricsResponse(response, metricName)
	} else if cmd.Flags().Changed("duration") {
		// if duration is specified, query from prometheus endpoint
		var prometheusEndpoint string
		// if prometheus endpoint is specified as cli parameter, use it. otherwise fallback to environment variable.
		if cmd.Flags().Changed("prometheus-endpoint") {
			prometheusEndpoint = prometheusEndpointUrl
		} else {
			prometheusEndpoint = os.Getenv("PROMETHEUS_ENDPOINT")
		}
		if prometheusEndpoint == "" {
			logger.Log.Printf("prometheus endpoint is not specified")
			fmt.Printf("prometheus endpoint is not specified")
			os.Exit(2)
		}
		promQuery := fmt.Sprintf(`avg_over_time(%s{hostname="%s"}[%s])`, metricName, nodeName, metricDuration)
		response, err := utils.QueryPrometheusEndpoint(prometheusEndpoint, promQuery, envProvider.authInfo)
		if err != nil {
			fmt.Printf("unable to query prometheus endpoint")
			os.Exit(2)
		}
		logger.Log.Printf("gauge metrics prometheus endpoint response=%v", response)
		metrics = utils.ParsePromethuesResponse(*response)
	}

	if len(metrics) == 0 {
		fmt.Printf("metric %s not found", metricName)
		os.Exit(2)
	}

	//loop over all the items and check value against threshold
	// if crossed threshold, exit(1), else exit(0)
	for _, metric := range metrics {
		if metric > gaugeMetricThreshold {
			//below message to stdout will be captured in Node condition message
			fmt.Printf("metric %s crossed threshold", metricName)
			os.Exit(1)
		}
	}
	if exputils.IsDebianInstall() {
		fmt.Printf("metric %s is within threshold", metricName)
	}
	os.Exit(0)
}

func inbandRasErrorsCmdHandler(cmd *cobra.Command, args []string) {
	envProvider := EnvProvider{}
	err := envProvider.Initialize(cmd)
	if err != nil {
		fmt.Printf("%s", err.Error())
		os.Exit(2)
	}
	inbandRASErrorsEndpoint, err := envProvider.GetInbandRASErrorsEndpointURL()
	if err != nil || inbandRASErrorsEndpoint == "" {
		logger.Log.Printf("unable to get inband ras errors endpoint url. error=%v", err)
		fmt.Printf("unable to get endpoint url")
		os.Exit(2)
	}
	logger.Log.Printf("inband-ras errors endpoint url=%v", inbandRASErrorsEndpoint)

	errorSeverity := ""
	if cmd.Flags().Changed("severity") {
		errorSeverity = severity
		inbandRASErrorsEndpoint = fmt.Sprintf("%s?severity=%s", inbandRASErrorsEndpoint, errorSeverity)
	}
	response, err := utils.QueryExporterEndpoint(inbandRASErrorsEndpoint, envProvider.authInfo)
	if err != nil {
		logger.Log.Printf("unable to query inband ras errors endpoint. error:%v", err)
		if !exputils.IsDebianInstall() {
			utils.InvalidateExporterEndpointURLCache()
		}
		fmt.Printf("unable to query inband ras errors")
		os.Exit(2)
	}

	cperEntries := utils.ParseInbandRasErrorsResponse(response)

	entryCount := 0
	for _, gpucperentry := range cperEntries {
		for _, entry := range gpucperentry.CPEREntry {
			if afid != 0 && !utils.IsAFIDPresentInCPER(entry, afid) {
				continue
			}
			entryCount++
		}
	}
	if entryCount > int(threshold) {
		fmt.Printf("error count %d crossed threshold %d", entryCount, threshold)
		os.Exit(1)
	}
	if exputils.IsDebianInstall() {
		fmt.Printf("inband ras error count=%v is within threshold %v", entryCount, threshold)
	}
	os.Exit(0)
}

func init() {
	RootCmd.AddCommand(QueryMetricCmd)
	QueryMetricCmd.AddCommand(counterMetricCmd)
	QueryMetricCmd.AddCommand(gaugeMetricCmd)
	QueryMetricCmd.AddCommand(inbandRasErrorsCmd)

	counterMetricCmd.Flags().StringVarP(&metricName, "metric", "m", "", "Specify metric name")
	counterMetricCmd.Flags().Int64VarP(&threshold, "threshold", "t", 0, "Specify threshold value for the metric")
	if !exputils.IsDebianInstall() {
		counterMetricCmd.Flags().StringVar(&exporterRootCAPath, "exporter-root-ca", "", "Specify exporter root CA certificate mount path(If exporter endpoint has TLS/mTLS enabled) - Optional")
		counterMetricCmd.Flags().StringVar(&exporterBearerTokenPath, "exporter-bearer-token", "", "Specify exporter bearer token mount path(If exporter endpoint has Authorization enabled) - Optional")
		counterMetricCmd.Flags().StringVar(&clientCertPath, "client-cert", "", "Specify client certificate mount path(If exporter endpoint has mTLS enabled) - Optional")
	}
	gaugeMetricCmd.Flags().StringVarP(&metricName, "metric", "m", "", "Specify metric name")
	gaugeMetricCmd.Flags().Float64VarP(&gaugeMetricThreshold, "threshold", "t", 0, "Specify threshold value for the metric")
	if !exputils.IsDebianInstall() {
		gaugeMetricCmd.Flags().StringVarP(&metricDuration, "duration", "d", "", "Specify duration of query. Ex: 5s, 5m, etc.")
		gaugeMetricCmd.Flags().StringVar(&prometheusEndpointUrl, "prometheus-endpoint", "", "Specify prometheus endpoint URL (If duration is specified) - Optional")
		gaugeMetricCmd.Flags().StringVar(&exporterRootCAPath, "exporter-root-ca", "", "Specify exporter root CA certificate mount path(If exporter endpoint has TLS/mTLS enabled) - Optional")
		gaugeMetricCmd.Flags().StringVar(&exporterBearerTokenPath, "exporter-bearer-token", "", "Specify exporter bearer token mount path(If exporter endpoint has Authorization enabled) - Optional")
		gaugeMetricCmd.Flags().StringVar(&clientCertPath, "client-cert", "", "Specify client certificate mount path(If exporter endpoint has mTLS enabled) - Optional")
		gaugeMetricCmd.Flags().StringVar(&prometheusRootCAPath, "prometheus-root-ca", "", "Specify prometheus root CA certificate mount path(If prometheus endpoint has TLS/mTLS enabled) - Optional")
		gaugeMetricCmd.Flags().StringVar(&prometheusBearerTokenPath, "prometheus-bearer-token", "", "Specify prometheus bearer token mount path(If prometheus endpoint has Authorization enabled) - Optional")
	}

	inbandRasErrorsCmd.Flags().StringVarP(&severity, "severity", "s", "", "Specify error severity. Allowed values are CPER_SEVERITY_FATAL, CPER_SEVERITY_NON_FATAL_UNCORRECTED, CPER_SEVERITY_NON_FATAL_CORRECTED")
	inbandRasErrorsCmd.Flags().Int64VarP(&threshold, "threshold", "t", 0, "Specify threshold value for inband ras error. Default is 0.")
	inbandRasErrorsCmd.Flags().Uint64Var(&afid, "afid", 0, "Specify AFID to watch for specific inband ras event")
	if !exputils.IsDebianInstall() {
		inbandRasErrorsCmd.Flags().StringVar(&exporterRootCAPath, "exporter-root-ca", "", "Specify exporter root CA certificate mount path(If exporter endpoint has TLS/mTLS enabled) - Optional")
		inbandRasErrorsCmd.Flags().StringVar(&exporterBearerTokenPath, "exporter-bearer-token", "", "Specify exporter bearer token mount path(If exporter endpoint has Authorization enabled) - Optional")
		inbandRasErrorsCmd.Flags().StringVar(&clientCertPath, "client-cert", "", "Specify client certificate mount path(If exporter endpoint has mTLS enabled) - Optional")
	}
}
