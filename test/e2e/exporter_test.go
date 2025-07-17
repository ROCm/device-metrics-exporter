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
	"encoding/json"
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

func (s *E2ESuite) Test015FieldPrefixUpdate(c *C) {
	log.Print("test prefix update")
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
	err = s.SetPrefix("amd_")
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	config := s.ReadConfig()
	log.Printf("Prefix Config file : %+v", config)
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
	newFields := []string{
		"amd_gpu_power_usage",
		"amd_gpu_total_vram",
		"amd_gpu_ecc_uncorrect_gfx",
		"amd_gpu_umc_activity",
		"amd_gpu_mma_activity",
	}
	err = verifyMetricsLablesFields(allgpus, previousLabels, newFields)
	assert.Nil(c, err)
	// remove the prefix and verify
	err = s.SetPrefix("")
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	config = s.ReadConfig()
	log.Printf("SetUpTest Config file : %+v", config)
	log.Printf("Prefix Config file : %+v", config)

	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err = testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	err = verifyMetricsLablesFields(allgpus, previousLabels, previousFields)
	assert.Nil(c, err)
}

func (s *E2ESuite) Test016HealthSvcReconnect(c *C) {
	log.Print("Health Service Reconnect")
	fields := []string{
		"gpu_health",
	}
	err := s.SetFields(fields)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	config := s.ReadConfig()
	log.Printf("Prefix Config file : %+v", config)
	var response string

	// expect healthy for all gpu
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		return response != ""
	}, 3*time.Second, 1*time.Second)
	allgpus, err := testutils.ParsePrometheusMetrics(response)
	assert.Nil(c, err)
	err = verifyHealth(allgpus, "1")
	assert.Nil(c, err)

	// kill gpuagent and expect unhealthy
	_ = s.ExporterLocalCommandOutput("pkill gpuagent")
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		if response == "" {
			return false
		}
		log.Printf("gpu response : %+v", response)
		allgpus, _ = testutils.ParsePrometheusMetrics(response)
		// gpu_health field will not be prsent on gpuagent kill case
		err = verifyHealth(allgpus, "0")
		if err == nil {
			return false
		}
		return true
	}, 30*time.Second, 5*time.Second)

	// respawn gpuagent and expect healthy state again
	_ = s.ExporterLocalCommandOutput("gpuagent &")
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	assert.Eventually(c, func() bool {
		response, _ = s.getExporterResponse()
		if response == "" {
			return false
		}
		log.Printf("gpu response : %+v", response)
		allgpus, err = testutils.ParsePrometheusMetrics(response)
		if err != nil {
			return false
		}
		err = verifyHealth(allgpus, "1")
		if err != nil {
			return false
		}
		return true
	}, 30*time.Second, 1*time.Second)
}

func (s *E2ESuite) Test017SlurmWorkloadSim(c *C) {
	labels := []string{"job_id", "job_partition", "job_user"}
	err := s.SetLabels(labels)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	job_mock := map[string]string{
		"CUDA_VISIBLE_DEVICES": "0,1,2,3,4,5,6,7",
		"SLURM_CLUSTER_NAME":   "aac11",
		"SLURM_JOB_GPUS":       "0,1,2,3,4,5,6,7",
		"SLURM_JOB_ID":         "742",
		"SLURM_JOB_PARTITION":  "256C8G1H_MI325X_Ubuntu22",
		"SLURM_JOB_USER":       "yaoming_mu_7kq",
		"SLURM_SCRIPT_CONTEXT": "prolog_slurmd",
	}
	// Convert map to JSON
	jsonBytes, err := json.MarshalIndent(job_mock, "", "  ")
	assert.Nil(c, err)

	// Write JSON to file
	jobFile := "slurm_job.json"
	err = os.WriteFile(jobFile, jsonBytes, 0644)
	assert.Nil(c, err)
	defer os.Remove(jobFile)
	_, _ = s.exporter.CopyFileTo("slurm_job.json", "/var/run/exporter/3")
	time.Sleep(5 * time.Second) // 5 second timer for job to be picked up

	// Verify that job-related labels are present with correct values
	assert.Eventually(c, func() bool {
		response, _ := s.getExporterResponse()
		if response == "" {
			return false
		}

		allgpus, err := testutils.ParsePrometheusMetrics(response)
		if err != nil {
			log.Printf("Failed to parse metrics: %v", err)
			return false
		}

		// Verify job labels are present with expected values
		expectedJobLabels := map[string]string{
			"job_id":        "\"742\"",
			"job_partition": "\"256C8G1H_MI325X_Ubuntu22\"",
			"job_user":      "\"yaoming_mu_7kq\"",
		}

		// Verify job labels are present for all GPU IDs "0" through "7"
		for i := 0; i <= 7; i++ {
			gpuId := fmt.Sprintf("\"%d\"", i)
			if _, exists := allgpus[gpuId]; !exists {
				log.Printf("Expected GPU[%v] not found in metrics", gpuId)
				return false
			}

			targetGpu := allgpus[gpuId]

			err = verifyJobLabels(targetGpu, expectedJobLabels, gpuId)
			if err != nil {
				log.Printf("Job label verification failed for GPU[%v]: %v", gpuId, err)
				return false
			}
		}

		log.Printf("Job labels verified successfully: present on GPUs 0-7")
		return true
	}, 10*time.Second, 5*time.Second)
}

func (s *E2ESuite) Test018HealthSvcToggle(c *C) {
	log.Print("Disabling health service via SetCommonConfigHealth(false)")
	err := s.SetCommonConfigHealth(false)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // Wait for config update to take effect

	healthCmd := "docker exec -t test_exporter metricsclient --json"
	assert.Eventually(c, func() bool {
		// Run metricsclient  inside the exporter container and expect empty output
		output := s.tu.LocalCommandOutput(healthCmd)
		log.Print(output)
		return output == ""
	}, 10*time.Second, 1*time.Second)

	log.Print("Removing commonconfig and verifying metricsclient --json returns non-empty output")
	err = s.RemoveCommonConfig() // Re-enable health service to restore config
	assert.Nil(c, err)
	time.Sleep(5 * time.Second)

	// Run metricsclient  inside the exporter container and expect non-empty output
	assert.Eventually(c, func() bool {
		output := s.tu.LocalCommandOutput(healthCmd)
		log.Print(output)
		return output != ""
	}, 10*time.Second, 1*time.Second)

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

func verifyHealth(allgpus map[string]*testutils.GPUMetric, state string) error {
	if len(allgpus) == 0 {
		return fmt.Errorf("invalid input, expecting non empty payload")
	}
	healthField := "gpu_health"
	for id, gpu := range allgpus {
		healthState, ok := gpu.Fields[healthField]
		if !ok {
			log.Printf("gpu_health not found, gpuagent may be killed")
			return fmt.Errorf("health not found")
		}
		if healthState.Value != state {
			return fmt.Errorf("gpu[%v] expected health[%v] but got[%v]", id, state, healthState.Value)
		}
	}

	log.Printf("all gpu in expected health state [%v]", state)
	return nil
}

func verifyJobLabels(gpu *testutils.GPUMetric, expectedJobLabels map[string]string, gpuId string) error {
	if gpu == nil {
		return fmt.Errorf("GPU metric is nil")
	}

	if len(gpu.Fields) == 0 {
		return fmt.Errorf("GPU[%v] has no metric fields", gpuId)
	}

	// Check that the GPU has the expected job labels
	foundJobLabels := false
	for fieldName, metricField := range gpu.Fields {
		// Verify that gpu_id label matches our target (if present)
		if gpuIdValue, exists := metricField.Labels["gpu_id"]; exists && gpuIdValue != gpuId {
			continue // Skip fields that don't match the expected GPU ID
		}

		hasAllJobLabels := true
		for expectedLabel, expectedValue := range expectedJobLabels {
			actualValue, exists := metricField.Labels[expectedLabel]
			if !exists {
				log.Printf("GPU[%v] field[%v] missing job label: %v", gpuId, fieldName, expectedLabel)
				hasAllJobLabels = false
				break
			}
			if actualValue != expectedValue {
				return fmt.Errorf("GPU[%v] field[%v] job label[%v] expected value[%v] but got[%v]",
					gpuId, fieldName, expectedLabel, expectedValue, actualValue)
			}
		}

		if hasAllJobLabels {
			foundJobLabels = true
			log.Printf("GPU[%v] field[%v] has correct job labels", gpuId, fieldName)
			break // Found correct labels for this field, no need to check other fields
		}
	}

	if !foundJobLabels {
		return fmt.Errorf("GPU[%v] metrics do not contain the required job labels", gpuId)
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
