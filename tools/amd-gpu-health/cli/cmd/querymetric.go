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
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	exputils "github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"github.com/ROCm/device-metrics-exporter/tools/amd-gpu-health/cli/utils"
)

var (
	metricName                string
	counterMetricThreshold    int64
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
)

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

func counterMetricCmdHandler(cmd *cobra.Command, args []string) {
	//TODO: pass configfile namespace info values from config file
	configPath := viper.GetString("kubeConfigPath")
	//get node name from env variable
	nodeName, err := exputils.GetHostName()
	if err != nil {
		logger.Log.Printf("unable to fetch node name. error:%v", err)
		os.Exit(2)
	}

	//fetch auth info
	authInfo := fetchAuthInfo(cmd)

	isTLS := authInfo.ExporterRootCAPath != ""
	metricsEndpoint := utils.GetGPUMetricsEndpointURL(configPath, nodeName, isTLS)
	// if endpoint is not found, we should not exit with error code 1.
	if metricsEndpoint == "" {
		os.Exit(2)
	}
	logger.Log.Printf("metrics endpoint url=%v", metricsEndpoint)
	// check if the below mandatory args are provided or not
	if !cmd.Flags().Changed("metric") || !cmd.Flags().Changed("threshold") {
		os.Exit(2)
	}

	// query metrics endpoint
	response, err := utils.QueryExporterEndpoint(metricsEndpoint, authInfo)
	if err != nil {
		logger.Log.Printf("unable to query metrics endpoint. error:%v", err)
		utils.InvalidateExporterEndpointURLCache()
		os.Exit(2)
	}
	logger.Log.Printf("counter metrics response=%s", response)
	metrics := utils.ParseGPUMetricsResponse(response, metricName)
	if len(metrics) == 0 {
		os.Exit(2)
	}
	for _, metric := range metrics {
		if int64(metric) > counterMetricThreshold {
			//below message to stdout will be captured in Node condition message
			fmt.Printf("metric %s crossed threshold", metricName)
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func gaugeMetricCmdHandler(cmd *cobra.Command, args []string) {
	//TODO: pass configfile namespace info values from config file
	configPath := viper.GetString("configPath")
	//get node name from env variable
	nodeName, err := exputils.GetHostName()
	if err != nil {
		logger.Log.Printf("unable to fetch node name. error:%v", err)
		os.Exit(2)
	}
	//validate all the args
	if !cmd.Flags().Changed("metric") || !cmd.Flags().Changed("threshold") {
		os.Exit(2)
	}

	// fetch auth info
	authInfo := fetchAuthInfo(cmd)
	var metrics []float64
	// if duration is specified, query from prometheus endpoint
	// else query the latest value from local exporter endpoint.
	if cmd.Flags().Changed("duration") {
		var prometheusEndpoint string
		// if prometheus endpoint is specified as cli parameter, use it. otherwise fallback to environment variable.
		if cmd.Flags().Changed("prometheus-endpoint") {
			prometheusEndpoint = prometheusEndpointUrl
		} else {
			prometheusEndpoint = os.Getenv("PROMETHEUS_ENDPOINT")
		}
		if prometheusEndpoint == "" {
			logger.Log.Printf("prometheus endpoint is not specified")
			os.Exit(2)
		}
		promQuery := fmt.Sprintf(`avg_over_time(%s{hostname="%s"}[%s])`, metricName, nodeName, metricDuration)
		response, err := utils.QueryPrometheusEndpoint(prometheusEndpoint, promQuery, authInfo)
		if err != nil {
			os.Exit(2)
		}
		logger.Log.Printf("gauge metrics prometheus endpoint response=%v", response)
		metrics = utils.ParsePromethuesResponse(*response)
	} else {
		isTLS := authInfo.ExporterRootCAPath != ""
		metricsEndpoint := utils.GetGPUMetricsEndpointURL(configPath, nodeName, isTLS)
		// if endpoint is not found, we should not exit with error code 1.
		if metricsEndpoint == "" {
			os.Exit(2)
		}
		response, err := utils.QueryExporterEndpoint(metricsEndpoint, authInfo)
		if err != nil {
			logger.Log.Printf("unable to query metrics endpoint. error:%v", err)
			utils.InvalidateExporterEndpointURLCache()
			os.Exit(2)
		}

		logger.Log.Printf("gauge metrics exporter endpoint response=%s", response)
		metrics = utils.ParseGPUMetricsResponse(response, metricName)
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
	os.Exit(0)
}

func inbandRasErrorsCmdHandler(cmd *cobra.Command, args []string) {
	configPath := viper.GetString("kubeConfigPath")
	//get node name from env variable
	nodeName, err := exputils.GetHostName()
	if err != nil {
		logger.Log.Printf("unable to fetch node name. error:%v", err)
		os.Exit(2)
	}

	//fetch auth info
	authInfo := fetchAuthInfo(cmd)

	isTLS := authInfo.ExporterRootCAPath != ""
	inbandRASErrorsEndpoint := utils.GetGPUInbandRASErrorsURL(configPath, nodeName, isTLS)
	// if endpoint is not found, we should not exit with error code 1.
	if inbandRASErrorsEndpoint == "" {
		os.Exit(2)
	}
	logger.Log.Printf("inband-ras errors endpoint url=%v", inbandRASErrorsEndpoint)

	errorSeverity := ""
	if cmd.Flags().Changed("severity") {
		errorSeverity = severity
		inbandRASErrorsEndpoint = fmt.Sprintf("%s?severity=%s", inbandRASErrorsEndpoint, errorSeverity)
	}
	response, err := utils.QueryExporterEndpoint(inbandRASErrorsEndpoint, authInfo)
	if err != nil {
		logger.Log.Printf("unable to query inband ras errors endpoint. error:%v", err)
		utils.InvalidateExporterEndpointURLCache()
		os.Exit(2)
	}

	cperEntries := utils.ParseInbandRasErrorsResponse(response)
	if len(cperEntries) == 0 {
		os.Exit(0)
	}

	//get latest processed error timestamp
	t, _ := utils.GetLatestProcessedInbandErrorTimestamp()
	for _, gpucperentry := range cperEntries {
		for _, entry := range gpucperentry.CPEREntry {
			timestampFormat := "2006-01-02 15:04:05"
			entryTimestamp, err := time.Parse(timestampFormat, entry.Timestamp)
			if err != nil {
				logger.Log.Printf("unable to parse inband ras error timestamp. error:%v", err)
				continue
			}
			if entryTimestamp.After(t) {
				//below message to stdout will be captured in Node condition message
				fmt.Printf("inband ras error detected for gpu %v timestamp:%v severity:%v", gpucperentry.GPU, entryTimestamp, entry.Severity)
				//update latest processed error timestamp file
				err = utils.UpdateLatestProcessedInbandErrorTimestamp(entryTimestamp)
				if err != nil {
					logger.Log.Printf("unable to update latest processed inband error timestamp. error:%v", err)
				}
				os.Exit(1)
			}
		}
	}
	os.Exit(0)
}

func init() {
	RootCmd.AddCommand(QueryMetricCmd)
	QueryMetricCmd.AddCommand(counterMetricCmd)
	QueryMetricCmd.AddCommand(gaugeMetricCmd)
	QueryMetricCmd.AddCommand(inbandRasErrorsCmd)

	counterMetricCmd.Flags().StringVarP(&nodeName, "node", "n", "", "Specify node name")
	counterMetricCmd.Flags().StringVarP(&metricName, "metric", "m", "", "Specify metric name")
	counterMetricCmd.Flags().Int64VarP(&counterMetricThreshold, "threshold", "t", 0, "Specify threshold value for the metric")
	counterMetricCmd.Flags().StringVar(&exporterRootCAPath, "exporter-root-ca", "", "Specify exporter root CA certificate mount path(If exporter endpoint has TLS/mTLS enabled) - Optional")
	counterMetricCmd.Flags().StringVar(&exporterBearerTokenPath, "exporter-bearer-token", "", "Specify exporter bearer token mount path(If exporter endpoint has Authorization enabled) - Optional")
	counterMetricCmd.Flags().StringVar(&clientCertPath, "client-cert", "", "Specify client certificate mount path(If exporter endpoint has mTLS enabled) - Optional")

	gaugeMetricCmd.Flags().StringVarP(&nodeName, "node", "n", "", "Specify node name")
	gaugeMetricCmd.Flags().StringVarP(&metricName, "metric", "m", "", "Specify metric name")
	gaugeMetricCmd.Flags().Float64VarP(&gaugeMetricThreshold, "threshold", "t", 0, "Specify threshold value for the metric")
	gaugeMetricCmd.Flags().StringVarP(&metricDuration, "duration", "d", "", "Specify duration of query. Ex: 5s, 5m, etc.")
	gaugeMetricCmd.Flags().StringVar(&prometheusEndpointUrl, "prometheus-endpoint", "", "Specify prometheus endpoint URL (If duration is specified) - Optional")
	gaugeMetricCmd.Flags().StringVar(&exporterRootCAPath, "exporter-root-ca", "", "Specify exporter root CA certificate mount path(If exporter endpoint has TLS/mTLS enabled) - Optional")
	gaugeMetricCmd.Flags().StringVar(&exporterBearerTokenPath, "exporter-bearer-token", "", "Specify exporter bearer token mount path(If exporter endpoint has Authorization enabled) - Optional")
	gaugeMetricCmd.Flags().StringVar(&clientCertPath, "client-cert", "", "Specify client certificate mount path(If exporter endpoint has mTLS enabled) - Optional")
	gaugeMetricCmd.Flags().StringVar(&prometheusRootCAPath, "prometheus-root-ca", "", "Specify prometheus root CA certificate mount path(If prometheus endpoint has TLS/mTLS enabled) - Optional")
	gaugeMetricCmd.Flags().StringVar(&prometheusBearerTokenPath, "prometheus-bearer-token", "", "Specify prometheus bearer token mount path(If prometheus endpoint has Authorization enabled) - Optional")

	inbandRasErrorsCmd.Flags().StringVarP(&nodeName, "node", "n", "", "Specify node name")
	inbandRasErrorsCmd.Flags().StringVarP(&severity, "severity", "s", "", "Specify error severity. Allowed values are CPER_SEVERITY_FATAL, CPER_SEVERITY_NON_FATAL_UNCORRECTED, CPER_SEVERITY_NON_FATAL_CORRECTED")
	inbandRasErrorsCmd.Flags().StringVar(&exporterRootCAPath, "exporter-root-ca", "", "Specify exporter root CA certificate mount path(If exporter endpoint has TLS/mTLS enabled) - Optional")
	inbandRasErrorsCmd.Flags().StringVar(&exporterBearerTokenPath, "exporter-bearer-token", "", "Specify exporter bearer token mount path(If exporter endpoint has Authorization enabled) - Optional")
	inbandRasErrorsCmd.Flags().StringVar(&clientCertPath, "client-cert", "", "Specify client certificate mount path(If exporter endpoint has mTLS enabled) - Optional")
}
