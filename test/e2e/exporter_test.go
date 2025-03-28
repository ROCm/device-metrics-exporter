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

package e2e

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gpuagent"
	testutils "github.com/ROCm/device-metrics-exporter/test/utils"
)

var (
	maxMockGpuNodes  = 16
	totalMetricCount = 0
	previousFields   = []string{}
	previousLabels   = []string{}
	mandatoryLabels  = []string{}
)

func (s *E2ESuite) Test001FirstDeplymentDefaults(c *C) {
	for _, label := range gpuagent.GetGPUAgentMandatoryLabels() {
		mandatoryLabels = append(mandatoryLabels, strings.ToLower(label))
	}
	log.Print("Testing basic http response after docker deployment")
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	//log.Print(response)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	maxMockGpuNodes = len(allgpus)
	// verify all mandatory labels are present on each metrics
	for _, gpu := range allgpus {
		totalMetricCount = totalMetricCount + len(gpu.Fields)

		for _, metricData := range gpu.Fields {
			for _, label := range mandatoryLabels {
				_, ok := metricData.Labels[label]
				assert.Equal(c, true, ok, fmt.Sprintf("expecting label %v not found", label))
			}
		}
	}
}

func (s *E2ESuite) Test002NonMandatoryLabelUpdate(c *C) {
	log.Print("Testing non mandatatory label update")
	labels := []string{"gpu_uuid"}
	err := s.SetLabels(labels)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	expectedLabels := append(labels, mandatoryLabels...)
	err = verifyMetricsLablesFields(allgpus, expectedLabels, []string{})
	assert.Nil(c, err)
}

func (s *E2ESuite) Test003InvalidLabel(c *C) {
	log.Print("test non mandatory invalid label update, should pick only valid labels")
	labels := []string{"gpu_if", "gpu_uuid"}
	err := s.SetLabels(labels)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	previousLabels = append(mandatoryLabels, "gpu_uuid")
	err = verifyMetricsLablesFields(allgpus, previousLabels, []string{})
	assert.Nil(c, err)
}

func (s *E2ESuite) Test004FieldUpdate(c *C) {
	log.Print("test non mandatory field update")
	// indexed metrics are not parsed yet on testing, revisit
	fields := []string{
		"gpu_power_usage",
		"gpu_total_vram",
		"gpu_ecc_uncorrect_gfx",
		"gpu_umc_activity",
		"gpu_mma_activity",
	}
	err := s.SetFields(fields)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	previousFields = []string{
		"gpu_power_usage",
		"gpu_total_vram",
		"gpu_ecc_uncorrect_gfx",
		"gpu_umc_activity",
		"gpu_mma_activity",
	}
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test005InvalidFieldUpdate(c *C) {
	log.Print("test non mandatory invalid field update")
	// indexed metrics are not parsed yet on testing, revisit
	fields := []string{
		"invalid_config",
		"gpu_power_usage",
		"gpu_ecc_uncorrect_gfx",
		"gpu_umc_activity",
		"gpu_mma_activity",
	}
	err := s.SetFields(fields)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	previousFields = []string{
		"gpu_power_usage",
		"gpu_ecc_uncorrect_gfx",
		"gpu_umc_activity",
		"gpu_mma_activity",
	}
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test006ServerPortUpdate(c *C) {
	log.Print("update server port")
	err := s.SetServerPort(5002)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second)
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 10*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test007DeleteConfig(c *C) {
	log.Print("delete metric config should revert all configs and back to default")
	// delete config file
	err := os.Remove(s.configPath)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	previousLabels = []string{}
	previousFields = []string{}
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test008RecreateConfigFile(c *C) {
	log.Print("create config file after delete")
	labels := []string{"gpu_id", "job_id"}
	err := s.SetLabels(labels)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	previousLabels = labels
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test009ServerPortAfterRecreateConfig(c *C) {
	log.Print("update server port after config recreate")
	err := s.SetServerPort(5002)
	assert.Nil(c, err)
	time.Sleep(10 * time.Second)
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test010ServerInvalidPortUpdate(c *C) {
	log.Print("update server port with 0")
	err := s.SetServerPort(0)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second)
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test011ContainerWithoutConfig(c *C) {
	log.Print("creating new server server_noconfig")
	cname := "server_noconfig"
	tc := NewMockExporter(cname, s.e2eConfig.ImageURL)
	assert.NotNil(c, tc)

	log.Printf("cleaning up any old instances of same name %v", cname)
	_ = tc.Stop()
	time.Sleep(2 * time.Second)

	pMap := map[int]int{
		5003: 5000,
	}
	assert.Nil(c, tc.SetPortMap(pMap))
	tc.SkipConfigMount()
	err := tc.Start()
	assert.Nil(c, err)
	log.Printf("waiting for container %v to start", cname)
	time.Sleep(25 * time.Second)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	exporterClient := &http.Client{Transport: tr}
	log.Printf("creation of new container %v done", cname)
	url := "http://localhost:5003/metrics"

	var response string
	assert.Eventually(c, func() bool {
		resp, err := exporterClient.Get(url)
		if err != nil {
			return false
		}
		bytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return false
		}
		response = string(bytes)
		return response != ""
	}, 5*time.Second, 1*time.Second)

	// check if we have valid payload
	_, err = testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	// Stopping newly created container
	log.Printf("deleting container %v", cname)
	assert.Nil(c, tc.Stop())

}

func (s *E2ESuite) Test012CustomLabelUpdate(c *C) {
	log.Print("Testing custom label update")
	customLabels := map[string]string{
		"cLabel1": "cValue1",
		"cLabel2": "cValue2",
	}
	customLabelKeys := []string{"clabel1", "clabel2"}
	err := s.SetCustomLabels(customLabels)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	expectedLabels := append(customLabelKeys, mandatoryLabels...)
	err = verifyMetricsLablesFields(allgpus, expectedLabels, []string{})
	assert.Nil(c, err)
}

func (s *E2ESuite) Test013MandatoryLabelsAsCustomLabels(c *C) {
	log.Print("Testing mandatory labels supplied as custom labels")
	customLabels := map[string]string{
		"card_model":    "custom_card_model",
		"serial_number": "custom_serial_number",
		"gpu_id":        "custom_gpu_id",
		"cLabel1":       "cValue1",
	}
	customLabelKeys := []string{"clabel1"}
	err := s.SetCustomLabels(customLabels)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	// Not expecting the mandatory labels
	expectedLabels := append(customLabelKeys, mandatoryLabels...)
	err = verifyMetricsLablesFields(allgpus, expectedLabels, []string{})
	assert.Nil(c, err)
}

func (s *E2ESuite) Test014ExistingLabelsAsCustomLabels(c *C) {
	log.Print("Testing existing labels supplied as custom labels")
	customLabels := map[string]string{
		"card_model":     "custom_card_model",
		"serial_number":  "custom_serial_number",
		"gpu_id":         "custom_gpu_id",
		"cluster_name":   "cValue1",
		"card_vendor":    "cValue2",
		"driver_version": "cValue3",
	}
	// Only cluster_name is allowed to be customized from existing labels
	customLabelKeys := []string{"cluster_name"}
	err := s.SetCustomLabels(customLabels)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	var response string
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	// Not expecting the mandatory labels
	expectedLabels := append(customLabelKeys, mandatoryLabels...)
	err = verifyMetricsLablesFields(allgpus, expectedLabels, []string{})
	assert.Nil(c, err)
}

func verifyMetricsLablesFields(allgpus map[string]*testutils.GPUMetric, labels []string, fields []string) error {
	if len(allgpus) == 0 {
		return fmt.Errorf("invalid input, expecting non empty payload")
	}
	for id, gpu := range allgpus {
		if len(fields) != 0 {
			if len(gpu.Fields) != len(fields) {
				return fmt.Errorf("GPU[%v] expecting total field per gpu %v but got %v", id, len(fields), len(gpu.Fields))
			}

			for _, metricFieldData := range gpu.Fields {
				for _, cField := range fields {
					if _, ok := gpu.Fields[cField]; !ok {
						return fmt.Errorf("expecting field %v not found", cField)
					}
				}
				for _, label := range labels {
					if _, ok := metricFieldData.Labels[label]; !ok {
						return fmt.Errorf("expecting label %v not found", label)
					}
				}
			}
		}
	}
	return nil
}

func (s *E2ESuite) SetUpTest(c *C) {
	s.validateCluster(c)
	config := s.ReadConfig()
	log.Printf("SetUpTest Config file : %+v", config)
}

func (s *E2ESuite) getExporterResponse() (string, error) {
	url := s.GetExporterURL()
	//log.Print(url)
	if s.exporterClient == nil {
		log.Print("exporter http not initialized")
		return "", fmt.Errorf("exporter http not initialized")
	}
	resp, err := s.exporterClient.Get(url)
	if err != nil {
		return "", err
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	bodyString := string(bodyBytes)
	//log.Print(bodyString)
	return bodyString, nil
}

func (s *E2ESuite) validateCluster(c *C) {
	log.Printf("s:%s Validating Cluster", time.Now().String())

	assert.Eventually(c, func() bool {
		response := s.GetExporter()
		return response != ""
	}, 3*time.Second, 1*time.Second)
}
