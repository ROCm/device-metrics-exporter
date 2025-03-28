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

package k8e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gpuagent"
	"github.com/ROCm/device-metrics-exporter/test/utils"
	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	nodePort      = 32100
	exporterPod   *corev1.Pod
	configmapName = "test-e2e-config"
)

type gpuconfig struct {
	Fields           []string       `json:"Fields"`
	Labels           []string       `json:"Labels"`
	HealthThresholds map[string]int `json:"HealthThresholds"`
}

type exporterConfig struct {
	GPUConfig *gpuconfig `json:"GPUConfig"`
}

func (s *E2ESuite) Test001FirstDeplymentDefaults(c *C) {
	ctx := context.Background()
	log.Print("Testing helm install for exporter")
	values := []string{
		fmt.Sprintf("image.repository=%v", s.registry),
		fmt.Sprintf("image.tag=%v", s.imageTag),
		"service.type=NodePort",
		fmt.Sprintf("service.NodePort.nodePort=%d", nodePort),
		fmt.Sprintf("configMap=%v", configmapName),
		fmt.Sprintf("platform=%v", s.platform),
		"image.pullPolicy=IfNotPresent",
	}

	config := exporterConfig{}
	cfgData, err := json.Marshal(config)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	err = s.k8sclient.CreateConfigMap(ctx, s.ns, configmapName, string(cfgData))
	assert.NoError(c, err)
	rel, err := s.helmClient.InstallChart(ctx, s.helmChart, values)
	if err != nil {
		log.Printf("failed to install charts")
		assert.Fail(c, err.Error())
		return
	}
	log.Printf("helm installed exporter relName :%v err:%v", rel, err)
	log.Printf("sleep for 20s for pod to be ready")
	time.Sleep(20 * time.Second)
	labelMap := map[string]string{"app": fmt.Sprintf("%v-amdgpu-metrics-exporter", rel)}
	assert.Eventually(c, func() bool {
		pods, err := s.k8sclient.GetPodsByLabel(ctx, s.ns, labelMap)
		if err != nil {
			log.Printf("label get pod err %v", err)
			return false
		}
		log.Printf("pods : %+v", pods)
		if len(pods) >= 1 {
			for _, pod := range pods {
				if pod.Status.Phase == "Running" {
					exporterPod = &pod
					break
				}
			}
			return true
		}
		return false
	}, 2*time.Minute, 10*time.Second)
	assert.Eventually(c, func() bool {
		err := s.k8sclient.ValidatePod(ctx, s.ns, exporterPod.Name)
		if err != nil {
			log.Printf("label get pod err %v", err)
			return false
		}
		return true
	}, 10*time.Second, 1*time.Second)
}

func (s *E2ESuite) Test002MetricsServer(c *C) {
	ctx := context.Background()
	log.Print("Test metrics server is responding")
	assert.Eventually(c, func() bool {
		labels, fields, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		return true
	}, 50*time.Second, 10*time.Second)
}

func (s *E2ESuite) Test003LabelUpdate(c *C) {
	ctx := context.Background()
	log.Print("Test metrics server is updating labels")
	mandatoryLabels := gpuagent.GetGPUAgentMandatoryLabels()
	cmLabels := []string{"pod", "container"}
	config := exporterConfig{
		GPUConfig: &gpuconfig{
			Labels: cmLabels,
		},
	}
	cfgData, err := json.Marshal(config)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	err = s.k8sclient.UpdateConfigMap(ctx, s.ns, configmapName, string(cfgData))
	assert.NoError(c, err)
	assert.Eventually(c, func() bool {
		labels, fields, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		return len(labels) == len(cmLabels)+len(mandatoryLabels)
	}, 90*time.Second, 5*time.Second)
}

func (s *E2ESuite) Test004FieldUpdate(c *C) {
	ctx := context.Background()
	log.Print("Test metrics server is updating fields")
	cmFields := []string{"gpu_package_power", "gpu_edge_temperature"}
	config := exporterConfig{
		GPUConfig: &gpuconfig{
			Fields: cmFields,
		},
	}
	cfgData, err := json.Marshal(config)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	err = s.k8sclient.UpdateConfigMap(ctx, s.ns, configmapName, string(cfgData))
	assert.NoError(c, err)
	assert.Eventually(c, func() bool {
		labels, fields, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		return len(fields) == len(cmFields)+1
	}, 90*time.Second, 5*time.Second)
}

func (s *E2ESuite) Test005HealthFieldUpdate(c *C) {
	ctx := context.Background()
	log.Print("Test metrics server is updating health field")
	cmFields := []string{"gpu_health"}
	config := exporterConfig{
		GPUConfig: &gpuconfig{
			Fields: cmFields,
		},
	}
	cfgData, err := json.Marshal(config)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	err = s.k8sclient.UpdateConfigMap(ctx, s.ns, configmapName, string(cfgData))
	assert.NoError(c, err)
	assert.Eventually(c, func() bool {
		labels, fields, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		return len(fields) == len(cmFields)+1
	}, 90*time.Second, 5*time.Second)
}

func (s *E2ESuite) Test007MarkAndVerifyGPUUnhealthyLabel(c *C) {
	ctx := context.Background()
	log.Print("Marking gpu 0 as unhealthy using metricsclient tool")
	cmd := `echo "{\"ID\": \"0\",\"Fields\": [\"GPU_ECC_UNCORRECT_SEM\",\"GPU_ECC_UNCORRECT_FUSE\"],\"Counts\" : [1, 2]}" > /tmp/ecc.json`
	_, err := s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	cmd1 := "metricsclient -ecc-file-path /tmp/ecc.json"
	_, err = s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd1)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	labelMap := make(map[string]string)
	labelMap["metricsexporter.amd.com.gpu.0.state"] = "unhealthy"
	log.Print("Verifying unhealthy label on the node(s)")
	assert.Eventually(c, func() bool {
		nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
		if err != nil || len(nodes) == 0 {
			return false
		}
		log.Printf("Got %d nodes with unhealthy label", len(nodes))
		return true
	}, 90*time.Second, 10*time.Second, "expected gpu 0 to become unhealthy but got healthy")
}

func (s *E2ESuite) Test008MarkAndVerifyGPUHealthyLabel(c *C) {
	ctx := context.Background()
	log.Print("Marking gpu 0 back as healthy using metricsclient tool")
	cmd := `echo "{\"ID\": \"0\",\"Fields\": [\"GPU_ECC_UNCORRECT_SEM\",\"GPU_ECC_UNCORRECT_FUSE\"],\"Counts\" : [0, 0]}" > /tmp/ecc.json`
	_, err := s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	cmd1 := "metricsclient -ecc-file-path /tmp/ecc.json"
	_, err = s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd1)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	labelMap := make(map[string]string)
	labelMap["metricsexporter.amd.com.gpu.0.state"] = "unhealthy"
	log.Print("Verifying healthy label on the node(s)")
	assert.Eventually(c, func() bool {
		nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
		if err != nil || len(nodes) == 0 {
			return true
		}
		log.Printf("Got %d nodes with healthy label", len(nodes))
		return false
	}, 90*time.Second, 10*time.Second, "expected gpu 0 to become healthy but got unhealthy")
}

func (s *E2ESuite) Test009VerifyHealthThresholds(c *C) {
	log.Print("Test to Verify Health Thresholds are considered")
	ctx := context.Background()
	// set thresholds to 1
	fields := utils.GetUncorrectableErrorFields()
	thresholds := make(map[string]int)
	for _, field := range fields {
		thresholds[field] = 1
	}
	config := exporterConfig{
		GPUConfig: &gpuconfig{
			Fields:           fields,
			HealthThresholds: thresholds,
		},
	}
	cfgData, err := json.Marshal(config)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	err = s.k8sclient.UpdateConfigMap(ctx, s.ns, configmapName, string(cfgData))
	assert.NoError(c, err)
	assert.Eventually(c, func() bool {
		labels, flds, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, flds)
		return len(flds) == len(fields)+1
	}, 90*time.Second, 5*time.Second)

	// use metricsclient to set the counters to 1
	log.Print("Set Metrics fields values to 1")
	cmd := fmt.Sprintf(`echo "%s" > /tmp/ecc.json`, utils.GetMockECCJSON(fields, 0, 1))
	_, err = s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	cmd1 := "metricsclient -ecc-file-path /tmp/ecc.json"
	_, err = s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd1)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}

	//verify GPU is healthy as the counters did not exceed threshold
	log.Print("Verifying gpu 0 is healthy")
	labelMap := make(map[string]string)
	labelMap["metricsexporter.amd.com.gpu.0.state"] = "unhealthy"
	log.Print("Verifying healthy label on the node(s)")
	assert.Eventually(c, func() bool {
		nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
		if err != nil || len(nodes) == 0 {
			return true
		}
		log.Printf("Got %d nodes with healthy label", len(nodes))
		return false
	}, 90*time.Second, 10*time.Second, "expected gpu 0 to be healthy but got unhealthy")

	log.Print("Increasing metrics values to exceed thresholds")
	cmd = fmt.Sprintf(`echo "%s" > /tmp/ecc.json`, utils.GetMockECCJSON(fields, 0, 2))
	_, err = s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	cmd1 = "metricsclient -ecc-file-path /tmp/ecc.json"
	_, err = s.k8sclient.ExecCmdOnPod(ctx, s.restConfig, exporterPod, "amdgpu-metrics-exporter-container", cmd1)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}

	labelMap["metricsexporter.amd.com.gpu.0.state"] = "unhealthy"
	log.Print("Verifying unhealthy label on the node(s)")
	assert.Eventually(c, func() bool {
		nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
		if err != nil || len(nodes) == 0 {
			return false
		}
		log.Printf("Got %d nodes with unhealthy label", len(nodes))
		return true
	}, 90*time.Second, 10*time.Second, "expected gpu 0 to become unhealthy but got healthy")

	log.Print("Increase threshold and verify gpu becomes healthy")
	for _, field := range fields {
		thresholds[field] = 3
	}
	config.GPUConfig.HealthThresholds = thresholds
	cfgData, err = json.Marshal(config)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	err = s.k8sclient.UpdateConfigMap(ctx, s.ns, configmapName, string(cfgData))
	assert.NoError(c, err)
	assert.Eventually(c, func() bool {
		labels, flds, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		return len(flds) == len(fields)+1
	}, 90*time.Second, 5*time.Second)
	labelMap["metricsexporter.amd.com.gpu.0.state"] = "healthy"
	log.Print("Verifying healthy label on the node(s)")
	assert.Eventually(c, func() bool {
		nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
		if err != nil || len(nodes) == 0 {
			return true
		}
		log.Printf("Got %d nodes with healthy label", len(nodes))
		return false
	}, 90*time.Second, 10*time.Second, "expected gpu 0 to be healthy but got unhealthy")
}

func (s *E2ESuite) Test100HelmUninstall(c *C) {
	err := s.helmClient.UninstallChart()
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	ctx := context.Background()
	err = s.k8sclient.DeleteConfigMap(ctx, s.ns, configmapName)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
}

func (s *E2ESuite) Test101SecondDeplymentNoConfigMap(c *C) {
	ctx := context.Background()
	log.Print("Testing helm install for exporter")
	values := []string{
		fmt.Sprintf("image.repository=%v", s.registry),
		fmt.Sprintf("image.tag=%v", s.imageTag),
		"service.type=NodePort",
		fmt.Sprintf("service.NodePort.nodePort=%d", nodePort),
		fmt.Sprintf("platform=%v", s.platform),
		"image.pullPolicy=IfNotPresent",
	}
	rel, err := s.helmClient.InstallChart(ctx, s.helmChart, values)
	if err != nil {
		log.Printf("failed to install charts")
		assert.Fail(c, err.Error())
		return
	}
	log.Printf("helm installed exporter relName :%v err:%v", rel, err)
	log.Printf("sleep for 20s for pod to be ready")
	time.Sleep(20 * time.Second)
	labelMap := map[string]string{"app": fmt.Sprintf("%v-amdgpu-metrics-exporter", rel)}
	assert.Eventually(c, func() bool {
		pods, err := s.k8sclient.GetPodsByLabel(ctx, s.ns, labelMap)
		if err != nil {
			log.Printf("label get pod err %v", err)
			return false
		}
		log.Printf("pods : %+v", pods)
		if len(pods) >= 1 {
			for _, pod := range pods {
				if pod.Status.Phase == "Running" {
					exporterPod = &pod
					break
				}
			}
			return true
		}
		return false
	}, 2*time.Minute, 10*time.Second)
	assert.Eventually(c, func() bool {
		err := s.k8sclient.ValidatePod(ctx, s.ns, exporterPod.Name)
		if err != nil {
			log.Printf("label get pod err %v", err)
			return false
		}
		return true
	}, 10*time.Second, 1*time.Second)
}

func (s *E2ESuite) Test102MetricsServer(c *C) {
	ctx := context.Background()
	log.Print("Test noconfigmap metrics server is responding")
	assert.Eventually(c, func() bool {
		labels, fields, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		return true
	}, 50*time.Second, 10*time.Second)
}

func (s *E2ESuite) Test103HelmUninstall(c *C) {
	err := s.helmClient.UninstallChart()
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
}

func (s *E2ESuite) SetUpTest(c *C) {
	s.validateCluster(c)
}

func (s *E2ESuite) validateCluster(c *C) {
	log.Printf("s:%s Validating Cluster", time.Now().String())
}
