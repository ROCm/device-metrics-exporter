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
	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	maxMockGpuNodes  = 16
	totalMetricCount = 0
	mandatoryLables  = []string{"gpu_uuid", "serial_number", "card_model"}
	nodePort         = 32100
	exporterPod      *corev1.Pod
	exporterEp       *corev1.Endpoints
	configmapName    = "test-e2e-config"
)

type gpuconfig struct {
	Fields []string `json:"Fields"`
	Labels []string `json:"Labels"`
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
		fmt.Sprintf("service.type=NodePort"),
		fmt.Sprintf("service.NodePort.nodePort=%d", nodePort),
		fmt.Sprintf("configMap=%v", configmapName),
		"image.pullPolicy=IfNotPresent",
	}
	config := exporterConfig{}
	cfgData, err := json.Marshal(config)
	if err != nil {
		assert.Fail(c, err.Error())
		return
	}
	s.k8sclient.CreateConfigMap(ctx, s.ns, configmapName, string(cfgData))
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
		if len(pods) == 1 {
			exporterPod = &pods[0]
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
	assert.Eventually(c, func() bool {
		labels, fields, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		if len(labels) != len(cmLabels)+len(mandatoryLabels) {
			return false
		}
		return true
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
	assert.Eventually(c, func() bool {
		labels, fields, err := s.k8sclient.GetMetricsCmdFromPod(ctx, s.restConfig, exporterPod)
		if err != nil {
			log.Printf("error : %v", err)
			return false
		}
		log.Printf("got valid payload : %v, %v", labels, fields)
		if len(fields) != len(cmFields)+1 {
			return false
		}
		return true
	}, 90*time.Second, 5*time.Second)
}

func (s *E2ESuite) Test005HelmUninstall(c *C) {
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

func (s *E2ESuite) Test006SecondDeplymentNoConfigMap(c *C) {
	ctx := context.Background()
	log.Print("Testing helm install for exporter")
	values := []string{
		fmt.Sprintf("image.repository=%v", s.registry),
		fmt.Sprintf("image.tag=%v", s.imageTag),
		fmt.Sprintf("service.type=NodePort"),
		fmt.Sprintf("service.NodePort.nodePort=%d", nodePort),
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
		if len(pods) == 1 {
			exporterPod = &pods[0]
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

func (s *E2ESuite) Test007MetricsServer(c *C) {
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

func (s *E2ESuite) Test008HelmUninstall(c *C) {
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
