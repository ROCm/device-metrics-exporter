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

package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// AuthInfo contains the mount paths to the required authentication information
// for the device metrics exporter and Prometheus server endpoints.
type AuthInfo struct {
	ExporterRootCAPath        string
	ExporterBearerTokenPath   string
	ClientCertPath            string
	PrometheusBearerTokenPath string
	PrometheusRootCAPath      string
}

type roundTripperWithAuth struct {
	base     http.RoundTripper
	authReq  *http.Request
	authInfo AuthInfo
}

type UrlCache struct {
	Timestamp time.Time `json:"timestamp"`
	URL       string    `json:"url"`
}

func (r roundTripperWithAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	// if Prometheus server has token based authorization, we need to pass the token in the request
	// token is created as generic/opaque secret and volume mounted on below path in NPD pod
	if r.authInfo.ExporterBearerTokenPath != "" {
		if _, err := os.Stat(r.authInfo.PrometheusBearerTokenPath); err == nil {
			token, err1 := os.ReadFile(r.authInfo.PrometheusBearerTokenPath)
			if err1 == nil {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			} else {
				logger.Log.Printf("unable to read prometheus bearer token. error:%v", err1)
			}
		}
	}
	return r.base.RoundTrip(req)
}

// if amd exporter metrics endpoint has TLS enabled, the corresponding
// root ca must be stored as generic secret and mounted as volume in NPD pod
func getTLSConfig(authInfo AuthInfo) (*tls.Config, error) {
	tlsConf := &tls.Config{}
	// if mTLS is configured, client should present it's certificate
	if _, err := os.Stat(authInfo.ClientCertPath); err == nil {
		cert, err1 := tls.LoadX509KeyPair(filepath.Join(authInfo.ClientCertPath, "tls.crt"),
			filepath.Join(authInfo.ClientCertPath, "tls.key"))
		if err1 == nil {
			tlsConf.Certificates = []tls.Certificate{cert}
		} else {
			logger.Log.Printf("unable to load client certificate. error:%v", err1)
		}
	}
	// for TLS, validate server side certificate using it's Root CA
	if _, err := os.Stat(authInfo.ExporterRootCAPath); err == nil {
		cacert, err1 := os.ReadFile(filepath.Join(authInfo.ExporterRootCAPath, "ca.crt"))
		if err1 != nil {
			logger.Log.Printf("unable to read metrics endpoint root ca certificate. error:%v", err1)
			return nil, err1
		}
		caCerts := x509.NewCertPool()
		caCerts.AppendCertsFromPEM(cacert)
		tlsConf.RootCAs = caCerts
	}
	return tlsConf, nil
}

func QueryExporterEndpoint(url string, authInfo AuthInfo) (data string, err error) {
	var resp *http.Response
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Log.Printf("unable to create new http request. error:%v", err)
		return
	}

	// if endpoint has TLS enabled, NPD needs to pass the certs in the http request
	tlsConf, err := getTLSConfig(authInfo)
	if err == nil && tlsConf != nil {
		client.Transport = &http.Transport{
			TLSClientConfig: tlsConf,
		}
	}
	// if endpoint has token based authorization enabled, NPD needs to set the token in all requests
	// token is created as generic/opaque secret and volume mounted on below path
	if _, err := os.Stat(authInfo.ExporterBearerTokenPath); err == nil {
		token, err1 := os.ReadFile(authInfo.ExporterBearerTokenPath)
		if err1 == nil {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		}
	}
	resp, err = client.Do(req)
	if err != nil {
		logger.Log.Printf("unable to query metrics endpoint. error:%v", err)
		return
	}
	defer resp.Body.Close()
	dataBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.Printf("unable to read metrics endpoint response. error:%v", err)
		return
	}
	data = string(dataBytes)
	return
}

func getPrometheusTLSConfig(authInfo AuthInfo) (*tls.Config, error) {
	tlsConf := tls.Config{}
	// if Prometheus server has HTTPS enabled, client needs the Prometheus Root CA to validate server's certificate
	if _, err := os.Stat(authInfo.PrometheusRootCAPath); err == nil {
		cacert, err1 := os.ReadFile(authInfo.PrometheusRootCAPath)
		if err1 != nil {
			logger.Log.Printf("unable to read prometheus endpoint root ca certificate. error:%v", err1)
			return nil, err1
		}
		caCerts := x509.NewCertPool()
		caCerts.AppendCertsFromPEM(cacert)
		tlsConf.RootCAs = caCerts
	}
	// if Prometheus has mTLS enabled, we need to present client certificate during authentication
	if _, err := os.Stat(authInfo.ClientCertPath); err == nil {
		cert, err1 := tls.LoadX509KeyPair(filepath.Join(authInfo.ClientCertPath, "tls.crt"),
			filepath.Join(authInfo.ClientCertPath, "tls.key"))
		if err1 == nil {
			tlsConf.Certificates = []tls.Certificate{cert}
		} else {
			logger.Log.Printf("unable to load client certificate. error:%v", err1)
		}
	}
	return &tlsConf, nil
}

func QueryPrometheusEndpoint(url, promQuery string, authInfo AuthInfo) (*model.Value, error) {
	conf := api.Config{
		Address: url,
	}
	req, _ := http.NewRequest("GET", url, nil)
	rt := roundTripperWithAuth{
		base:     &http.Transport{},
		authReq:  req,
		authInfo: authInfo,
	}
	tlsConf, err := getPrometheusTLSConfig(authInfo)
	if err == nil {
		rt.base = &http.Transport{
			TLSClientConfig: tlsConf,
		}
	}
	conf.RoundTripper = rt

	client, err := api.NewClient(conf)
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %v", err)
	}
	v1api := v1.NewAPI(client)
	ctx := context.Background()

	result, _, err := v1api.Query(ctx, promQuery, time.Now())
	if err != nil {
		return nil, fmt.Errorf("query prometheus endpoint failed with error: %v", err)
	}
	return &result, nil
}

func ParsePromethuesResponse(response model.Value) []float64 {
	var metricVals []float64
	switch response.Type() {
	case model.ValVector:
		vecs := response.(model.Vector)
		for _, vec := range vecs {
			metricVals = append(metricVals, float64(vec.Value))
		}
	}
	return metricVals
}

// ParseGPUMetricResponse parses GetGPUs response and extract value(s) of specific metricName for all the GPUs.
// All the reported metrics are of integer or float type.
// Return type is array of float to handle both type of metrics.
func ParseGPUMetricsResponse(response, metricName string) []float64 {
	var gpuResponse amdgpu.GPUGetResponse
	err := json.Unmarshal([]byte(response), &gpuResponse)
	var res []float64
	if err != nil {
		logger.Log.Printf("unable to parse metrics response. error:%v", err)
		return res
	}
	if exportermetrics.GPUMetricField_GPU_NODES_TOTAL.String() == strings.ToUpper(metricName) {
		res = append(res, float64(len(gpuResponse.Response)))
		return res
	}
	for _, gpu := range gpuResponse.Response {
		gpuMetrics := FetchMetricFromGPU(gpu, metricName)
		res = append(res, gpuMetrics...)
	}
	return res
}

// Fetch a specific metric value from a GPU
func FetchMetricFromGPU(gpu *amdgpu.GPU, metricName string) []float64 {
	var res float64
	stats := gpu.Stats
	if stats == nil {
		return []float64{}
	}
	metricName = strings.ToUpper(metricName)
	switch metricName {
	case exportermetrics.GPUMetricField_GPU_PACKAGE_POWER.String():
		res = float64(stats.PackagePower)
	case exportermetrics.GPUMetricField_GPU_AVERAGE_PACKAGE_POWER.String():
		res = float64(stats.AvgPackagePower)
	case exportermetrics.GPUMetricField_GPU_EDGE_TEMPERATURE.String():
		if stats.Temperature != nil {
			res = float64(stats.Temperature.EdgeTemperature)
		}
	case exportermetrics.GPUMetricField_GPU_JUNCTION_TEMPERATURE.String():
		if stats.Temperature != nil {
			res = float64(stats.Temperature.JunctionTemperature)
		}
	case exportermetrics.GPUMetricField_GPU_MEMORY_TEMPERATURE.String():
		if stats.Temperature != nil {
			res = float64(stats.Temperature.MemoryTemperature)
		}
	case exportermetrics.GPUMetricField_GPU_HBM_TEMPERATURE.String():
		if stats.Temperature != nil {
			out := []float64{}
			for _, temp := range stats.Temperature.HBMTemperature {
				out = append(out, float64(temp))
			}
			return out
		}
	case exportermetrics.GPUMetricField_GPU_GFX_ACTIVITY.String():
		if stats.Usage != nil {
			res = float64(stats.Usage.GFXActivity)
		}
	case exportermetrics.GPUMetricField_GPU_UMC_ACTIVITY.String():
		if stats.Usage != nil {
			res = float64(stats.Usage.UMCActivity)
		}
	case exportermetrics.GPUMetricField_GPU_MMA_ACTIVITY.String():
		if stats.Usage != nil {
			res = float64(stats.Usage.MMActivity)
		}
	case exportermetrics.GPUMetricField_GPU_VCN_ACTIVITY.String():
		if stats.Usage != nil {
			out := []float64{}
			for _, vcna := range stats.Usage.VCNActivity {
				out = append(out, float64(vcna))
			}
			return out
		}
	case exportermetrics.GPUMetricField_GPU_JPEG_ACTIVITY.String():
		if stats.Usage != nil {
			out := []float64{}
			for _, jpega := range stats.Usage.JPEGActivity {
				out = append(out, float64(jpega))
			}
			return out
		}
	case exportermetrics.GPUMetricField_GPU_VOLTAGE.String():
		if stats.Voltage != nil {
			res = float64(stats.Voltage.Voltage)
		}
	case exportermetrics.GPUMetricField_GPU_GFX_VOLTAGE.String():
		if stats.Voltage != nil {
			res = float64(stats.Voltage.GFXVoltage)
		}
	case exportermetrics.GPUMetricField_GPU_MEMORY_VOLTAGE.String():
		if stats.Voltage != nil {
			res = float64(stats.Voltage.MemoryVoltage)
		}
	case exportermetrics.GPUMetricField_PCIE_SPEED.String():
		if gpu.Status.PCIeStatus != nil {
			res = float64(gpu.Status.PCIeStatus.Speed)
		}
	case exportermetrics.GPUMetricField_PCIE_MAX_SPEED.String():
		if gpu.Status.PCIeStatus != nil {
			res = float64(gpu.Status.PCIeStatus.MaxSpeed)
		}
	case exportermetrics.GPUMetricField_PCIE_BANDWIDTH.String():
		if gpu.Status.PCIeStatus != nil {
			res = float64(gpu.Status.PCIeStatus.Bandwidth)
		}
	case exportermetrics.GPUMetricField_GPU_ENERGY_CONSUMED.String():
		res = float64(stats.EnergyConsumed)
	case exportermetrics.GPUMetricField_PCIE_REPLAY_COUNT.String():
		if stats.PCIeStats != nil {
			res = float64(stats.PCIeStats.ReplayCount)
		}
	case exportermetrics.GPUMetricField_PCIE_RECOVERY_COUNT.String():
		if stats.PCIeStats != nil {
			res = float64(stats.PCIeStats.RecoveryCount)
		}
	case exportermetrics.GPUMetricField_PCIE_REPLAY_ROLLOVER_COUNT.String():
		if stats.PCIeStats != nil {
			res = float64(stats.PCIeStats.ReplayRolloverCount)
		}
	case exportermetrics.GPUMetricField_PCIE_NACK_SENT_COUNT.String():
		if stats.PCIeStats != nil {
			res = float64(stats.PCIeStats.NACKSentCount)
		}
	case exportermetrics.GPUMetricField_PCIE_NACK_RECEIVED_COUNT.String():
		if stats.PCIeStats != nil {
			res = float64(stats.PCIeStats.NACKReceivedCount)
		}
	case exportermetrics.GPUMetricField_GPU_CLOCK.String():
		if gpu.Status.ClockStatus != nil {
			out := []float64{}
			for _, c := range gpu.Status.ClockStatus {
				out = append(out, float64(c.Frequency))
			}
			return out
		}
	case exportermetrics.GPUMetricField_GPU_POWER_USAGE.String():
		res = float64(stats.PowerUsage)
	case exportermetrics.GPUMetricField_GPU_TOTAL_VRAM.String():
		if gpu.Status.GetVRAMStatus() != nil {
			res = float64(gpu.Status.GetVRAMStatus().Size)
		}
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_TOTAL.String():
		res = float64(stats.TotalCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_TOTAL.String():
		res = float64(stats.TotalUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_SDMA.String():
		res = float64(stats.SDMACorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_SDMA.String():
		res = float64(stats.SDMAUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_GFX.String():
		res = float64(stats.GFXCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_GFX.String():
		res = float64(stats.GFXUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_MMHUB.String():
		res = float64(stats.MMHUBCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_MMHUB.String():
		res = float64(stats.MMHUBUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_ATHUB.String():
		res = float64(stats.ATHUBCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_ATHUB.String():
		res = float64(stats.ATHUBUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_BIF.String():
		res = float64(stats.BIFCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_BIF.String():
		res = float64(stats.BIFUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_HDP.String():
		res = float64(stats.HDPCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_HDP.String():
		res = float64(stats.HDPUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_XGMI_WAFL.String():
		res = float64(stats.XGMIWAFLCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_XGMI_WAFL.String():
		res = float64(stats.XGMIWAFLUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_DF.String():
		res = float64(stats.DFCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_DF.String():
		res = float64(stats.DFUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_SMN.String():
		res = float64(stats.SMNCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_SMN.String():
		res = float64(stats.SMNUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_SEM.String():
		res = float64(stats.SEMCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_SEM.String():
		res = float64(stats.SEMUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_MP0.String():
		res = float64(stats.MP0CorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_MP0.String():
		res = float64(stats.MP0UncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_MP1.String():
		res = float64(stats.MP1CorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_MP1.String():
		res = float64(stats.MP1UncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_FUSE.String():
		res = float64(stats.FUSECorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_FUSE.String():
		res = float64(stats.FUSEUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_UMC.String():
		res = float64(stats.UMCCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_UMC.String():
		res = float64(stats.UMCUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_0_NOP_TX.String():
		res = float64(stats.XGMINeighbor0TxNOPs)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_0_REQ_TX.String():
		res = float64(stats.XGMINeighbor0TxRequests)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_0_RESP_TX.String():
		res = float64(stats.XGMINeighbor0TxResponses)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_0_BEATS_TX.String():
		res = float64(stats.XGMINeighbor0TXBeats)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_1_NOP_TX.String():
		res = float64(stats.XGMINeighbor1TxNOPs)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_1_REQ_TX.String():
		res = float64(stats.XGMINeighbor1TxRequests)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_1_RESP_TX.String():
		res = float64(stats.XGMINeighbor1TxResponses)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_1_BEATS_TX.String():
		res = float64(stats.XGMINeighbor1TXBeats)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_0_TX_THRPUT.String():
		res = float64(stats.XGMINeighbor0TxThroughput)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_1_TX_THRPUT.String():
		res = float64(stats.XGMINeighbor1TxThroughput)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_2_TX_THRPUT.String():
		res = float64(stats.XGMINeighbor2TxThroughput)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_3_TX_THRPUT.String():
		res = float64(stats.XGMINeighbor3TxThroughput)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_4_TX_THRPUT.String():
		res = float64(stats.XGMINeighbor4TxThroughput)
	case exportermetrics.GPUMetricField_GPU_XGMI_NBR_5_TX_THRPUT.String():
		res = float64(stats.XGMINeighbor5TxThroughput)
	case exportermetrics.GPUMetricField_GPU_USED_VRAM.String():
		if stats.VRAMUsage != nil {
			res = float64(stats.VRAMUsage.UsedVRAM)
		}
	case exportermetrics.GPUMetricField_GPU_FREE_VRAM.String():
		var total, used float64
		if gpu.Status.GetVRAMStatus() != nil {
			total = float64(gpu.Status.GetVRAMStatus().Size)
		}
		if stats.VRAMUsage != nil {
			used = float64(stats.VRAMUsage.UsedVRAM)
		}
		if total > 0 {
			res = float64(total - used)
		}
	case exportermetrics.GPUMetricField_GPU_TOTAL_VISIBLE_VRAM.String():
		if stats.VRAMUsage != nil {
			res = float64(stats.VRAMUsage.TotalVisibleVRAM)
		}
	case exportermetrics.GPUMetricField_GPU_USED_VISIBLE_VRAM.String():
		if stats.VRAMUsage != nil {
			res = float64(stats.VRAMUsage.UsedVisibleVRAM)
		}
	case exportermetrics.GPUMetricField_GPU_FREE_VISIBLE_VRAM.String():
		if stats.VRAMUsage != nil {
			res = float64(stats.VRAMUsage.FreeVisibleVRAM)
		}
	case exportermetrics.GPUMetricField_GPU_TOTAL_GTT.String():
		if stats.VRAMUsage != nil {
			res = float64(stats.VRAMUsage.TotalGTT)
		}
	case exportermetrics.GPUMetricField_GPU_USED_GTT.String():
		if stats.VRAMUsage != nil {
			res = float64(stats.VRAMUsage.UsedGTT)
		}
	case exportermetrics.GPUMetricField_GPU_FREE_GTT.String():
		if stats.VRAMUsage != nil {
			res = float64(stats.VRAMUsage.FreeGTT)
		}
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_MCA.String():
		res = float64(stats.MCACorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_MCA.String():
		res = float64(stats.MCAUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_VCN.String():
		res = float64(stats.VCNCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_VCN.String():
		res = float64(stats.VCNUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_JPEG.String():
		res = float64(stats.JPEGCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_JPEG.String():
		res = float64(stats.JPEGUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_IH.String():
		res = float64(stats.IHCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_IH.String():
		res = float64(stats.IHUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_CORRECT_MPIO.String():
		res = float64(stats.MPIOCorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_ECC_UNCORRECT_MPIO.String():
		res = float64(stats.MPIOUncorrectableErrors)
	case exportermetrics.GPUMetricField_GPU_XGMI_LINK_RX.String():
		if stats.XGMILinkStats != nil {
			out := []float64{}
			for _, linkStat := range stats.XGMILinkStats {
				out = append(out, float64(linkStat.DataRead))
			}
			return out
		}
	case exportermetrics.GPUMetricField_GPU_XGMI_LINK_TX.String():
		if stats.XGMILinkStats != nil {
			out := []float64{}
			for _, linkStat := range stats.XGMILinkStats {
				out = append(out, float64(linkStat.DataWrite))
			}
			return out
		}
	case exportermetrics.GPUMetricField_GPU_VIOLATION_CURRENT_ACCUMULATED_COUNTER.String():
		if stats.ViolationStats != nil {
			res = float64(stats.ViolationStats.CurrentAccumulatedCounter)
		}
	case exportermetrics.GPUMetricField_GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_ACCUMULATED.String():
		if stats.ViolationStats != nil {
			res = float64(stats.ViolationStats.ProcessorHotResidencyAccumulated)
		}
	case exportermetrics.GPUMetricField_GPU_VIOLATION_PPT_RESIDENCY_ACCUMULATED.String():
		if stats.ViolationStats != nil {
			res = float64(stats.ViolationStats.PPTResidencyAccumulated)
		}
	case exportermetrics.GPUMetricField_GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACCUMULATED.String():
		if stats.ViolationStats != nil {
			res = float64(stats.ViolationStats.SocketThermalResidencyAccumulated)
		}
	case exportermetrics.GPUMetricField_GPU_VIOLATION_VR_THERMAL_RESIDENCY_ACCUMULATED.String():
		if stats.ViolationStats != nil {
			res = float64(stats.ViolationStats.VRThermalResidencyAccumulated)
		}
	case exportermetrics.GPUMetricField_GPU_VIOLATION_HBM_THERMAL_RESIDENCY_ACCUMULATED.String():
		if stats.ViolationStats != nil {
			res = float64(stats.ViolationStats.HBMThermalResidencyAccumulated)
		}
	}
	return []float64{res}
}

func readFromCache(cacheFilePath string) (UrlCache, error) {
	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		logger.Log.Printf("unable to read cache file %s. error:%v", cacheFilePath, err)
		return UrlCache{}, err
	}
	var cache UrlCache
	err = json.Unmarshal(data, &cache)
	if err != nil {
		logger.Log.Printf("unable to unmarshal cache data. error:%v", err)
		return UrlCache{}, err
	}
	return cache, nil
}

func writeToCache(cacheFilePath string, cache UrlCache) error {
	dataBytes, err := json.Marshal(cache)
	if err != nil {
		logger.Log.Printf("unable to marshal cache data. error:%v", err)
		return err
	}
	err = os.WriteFile(cacheFilePath, dataBytes, 0644)
	if err != nil {
		logger.Log.Printf("unable to write to cache file %s. error:%v", cacheFilePath, err)
		return err
	}
	return nil
}

func InvalidateExporterEndpointURLCache() {
	os.Remove(globals.ExporterEndpointURLCachePath)
}

// GetGPUMetricsEndpointURL returns the URL for the GPU metrics endpoint.
// We try to fetch metrics from cache first. If not found, we query K8s API server
func GetGPUMetricsEndpointURL(configPath, nodeName string, isTLS bool) string {
	cacheData, err := readFromCache(globals.ExporterEndpointURLCachePath)
	if err == nil && cacheData.URL != "" {
		cachedURL := fmt.Sprintf("%s%s", cacheData.URL, globals.AMDGPUHandlerPrefix)
		logger.Log.Printf("returning cached metrics endpoint URL: %s", cachedURL)
		return cachedURL
	}
	logger.Log.Printf("cache not found or stale. Querying K8s API server for metrics endpoint URL")
	//cache not found, or the cache is stale. Query K8s API server for metrics endpoint URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	k8sc, err := k8sclient.NewClient(ctx, configPath, nodeName)
	if err != nil {
		logger.Log.Printf("unable to instantiate k8s client. error:%v", err)
		return ""
	}
	exporterEndpoint := k8sc.GetExporterEndpoint(nodeName, isTLS)
	var metricsEndpoint string
	// cache the metrics endpoint URL
	if exporterEndpoint != "" {
		metricsEndpoint = exporterEndpoint + globals.AMDGPUHandlerPrefix
		_ = writeToCache(globals.ExporterEndpointURLCachePath, UrlCache{Timestamp: time.Now().UTC(), URL: exporterEndpoint})
	}
	return metricsEndpoint
}

// GetGPUInbandRASErrorsURL returns the URL for the GPU inband RAS errors endpoint.
// We try to fetch URL from cache first. If not found, we query K8s API server
func GetGPUInbandRASErrorsURL(configPath, nodeName string, isTLS bool) string {
	cacheData, err := readFromCache(globals.ExporterEndpointURLCachePath)
	if err == nil && cacheData.URL != "" {
		cachedURL := fmt.Sprintf("%s%s", cacheData.URL, globals.AMDGPUInbandRASHandlerPrefix)
		logger.Log.Printf("returning cached inband RAS errors URL: %s", cachedURL)
		return cachedURL
	}
	logger.Log.Printf("cache not found or stale for inband-ras errors url. querying k8s api server.")
	//cache not found, or the cache is stale. Query K8s API server for inband RAS errors URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	k8sc, err := k8sclient.NewClient(ctx, configPath, nodeName)
	if err != nil {
		logger.Log.Printf("unable to instantiate k8s client. error:%v", err)
		return ""
	}
	exporterEndpoint := k8sc.GetExporterEndpoint(nodeName, isTLS)
	var inbandRASErrorsURL string
	// cache the inband RAS errors URL
	if exporterEndpoint != "" {
		inbandRASErrorsURL = exporterEndpoint + globals.AMDGPUInbandRASHandlerPrefix
		_ = writeToCache(globals.ExporterEndpointURLCachePath, UrlCache{Timestamp: time.Now().UTC(), URL: exporterEndpoint})
	}
	return inbandRASErrorsURL
}

func ParseInbandRasErrorsResponse(response string) []*amdgpu.GPUCPEREntry {
	var cperResponse amdgpu.GPUCPERGetResponse
	err := json.Unmarshal([]byte(response), &cperResponse)
	var res []*amdgpu.GPUCPEREntry
	if err != nil {
		logger.Log.Printf("unable to parse inband RAS errors response. error:%v", err)
		return res
	}
	if cperResponse.ApiStatus != 0 {
		logger.Log.Printf("inband ras errors response has non-zero api status: %d", cperResponse.ApiStatus)
		return res
	}
	return cperResponse.CPER
}

func GetLatestProcessedInbandErrorTimestamp() (time.Time, error) {
	data, err := os.ReadFile(globals.LatestProcessedErrorTimestampFilePath)
	var t time.Time
	if err != nil {
		logger.Log.Printf("unable to read cache file %s. error:%v", globals.LatestProcessedErrorTimestampFilePath, err)
		return t, err
	}
	t, err = time.Parse(time.RFC3339, string(data))
	if err != nil {
		logger.Log.Printf("unable to parse latest processed inband error timestamp. error:%v", err)
		return t, err
	}
	return t, nil
}

func UpdateLatestProcessedInbandErrorTimestamp(t time.Time) error {
	return os.WriteFile(globals.LatestProcessedErrorTimestampFilePath, []byte(t.Format(time.RFC3339)), 0644)
}
