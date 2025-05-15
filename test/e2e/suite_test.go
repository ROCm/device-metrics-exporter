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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"testing"

	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	testutils "github.com/ROCm/device-metrics-exporter/test/utils"
)

var skipSetup = flag.Bool("skip-setup", false, "skip setting up testbed")

var cleanAfterTest = flag.Bool("clean-after-test", false, "clean testbed resources")

// All the test config, state and any helper caches for running this test
// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&E2ESuite{})

func (s *E2ESuite) ReadConfig() *exportermetrics.MetricConfig {
	var config exportermetrics.MetricConfig
	pmConfig := &config
	mConfigs, err := ioutil.ReadFile(s.configPath)
	if err == nil {
		_ = json.Unmarshal(mConfigs, pmConfig)
	}
	return pmConfig
}

func (s *E2ESuite) WriteConfig(data *exportermetrics.MetricConfig) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return err
	}

	// Write the JSON data to a file
	err = ioutil.WriteFile(s.configPath, jsonData, 0644)
	if err != nil {
		fmt.Println("Error writing JSON file:", err)
		return err
	}
	return nil
}

func (s *E2ESuite) ExporterLocalCommandOutput(cmd string) string {
	fullCmd := fmt.Sprintf("docker exec -it test_exporter %v", cmd)
	log.Printf("executing cmd [%v]", fullCmd)
	return s.tu.LocalCommandOutput(fullCmd)
}

func (s *E2ESuite) GetExporterURL() string {
	config := s.ReadConfig()
	port := config.ServerPort
	if port == 0 {
		port = 5000
	}
	url := fmt.Sprintf("http://localhost:%d/metrics", port)
	return url
}

func (s *E2ESuite) GetExporter() string {
	return s.tu.LocalCommandOutput("docker ps -q -f Name=test_exporter")
}

func (s *E2ESuite) SetServerPort(port uint32) error {
	config := s.ReadConfig()
	config.ServerPort = port
	return s.WriteConfig(config)
}

func (s *E2ESuite) SetLabels(labels []string) error {
	config := s.ReadConfig()
	if config.GetGPUConfig() == nil {
		config.GPUConfig = &exportermetrics.GPUMetricConfig{}
	}
	config.GPUConfig.Labels = labels
	return s.WriteConfig(config)
}

func (s *E2ESuite) SetFields(fields []string) error {
	config := s.ReadConfig()
	if config.GetGPUConfig() == nil {
		config.GPUConfig = &exportermetrics.GPUMetricConfig{}
	}
	config.GPUConfig.Fields = fields
	return s.WriteConfig(config)
}

func (s *E2ESuite) SetCustomLabels(customLabels map[string]string) error {
	config := s.ReadConfig()
	if config.GetGPUConfig() == nil {
		config.GPUConfig = &exportermetrics.GPUMetricConfig{}
	}
	config.GPUConfig.CustomLabels = customLabels
	return s.WriteConfig(config)
}

func (s *E2ESuite) SetUpSuite(c *C) {
	if os.Getenv("DRY_RUN") != "" {
		return
	}

	dockerRegistry := os.Getenv("DOCKER_REGISTRY")
	exporterImageName := os.Getenv("EXPORTER_IMAGE_NAME")
	exporterImageTag := os.Getenv("EXPORTER_IMAGE_TAG")
	exporterImage := dockerRegistry + "/" + exporterImageName + ":" + exporterImageTag
	log.Printf("Using exporter image: %s", exporterImage)

	var exporterConfigPath string
	var e2eConfigPath string
	e2eConfig := E2EConfig{}

	_, filename, _, _ := runtime.Caller(0)
	filePath := filepath.Dir(filename)

	exporterConfigPath = filePath + "/config_test/config.json"
	e2eConfigPath = filePath + "/config/tb_mock.json"

	data, err := os.ReadFile(e2eConfigPath)
	if err != nil {
		log.Printf("Error reading file: %v", err)
		return
	}
	err = json.Unmarshal(data, &e2eConfig)
	if err != nil {
		log.Printf("Error unmarshalling JSON: %v", err)
		return
	}
	e2eConfig.ImageURL = exporterImage

	log.Printf("e2econfig : %+v", e2eConfig)
	s.e2eConfig = &e2eConfig

	s.exporter = NewMockExporter(e2eConfig.ContainerName, e2eConfig.ImageURL)
	if s.exporter == nil {
		log.Printf("mock exporter creation failed")
		os.Exit(1)
	}

	if e2eConfig.Mode != "EXPORTER_MOCK" {
		log.Printf("Unsupported mode %v", e2eConfig.Mode)
		os.Exit(1)
	}

	s.name = e2eConfig.ContainerName
	if *skipSetup == false {
		_ = s.exporter.Stop()
		time.Sleep(2)

		err := s.exporter.Start()
		if err != nil {
			log.Printf("start error %v", err)
			os.Exit(1)
		}
		time.Sleep(25 * time.Second)
	}

	s.tu = testutils.New()
	s.configPath = exporterConfigPath
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	s.exporterClient = &http.Client{Transport: tr}
	// empty config is default
	err = s.WriteConfig(&exportermetrics.MetricConfig{})
	assert.Nil(c, err)
	log.Printf("SetUpSuite done with config path :%v", exporterConfigPath)
}

func (s *E2ESuite) TearDownSuite(c *C) {
	err := os.Remove(s.configPath)
	assert.Nil(c, err)
	time.Sleep(5 * time.Second) // 5 second timer for config update to take effect
	s.validateCluster(c)
	if *cleanAfterTest {
		log.Print("cleaning setup after test")
		s.tu.LocalCommandOutput(fmt.Sprintf("docker stop %v", s.name))
		time.Sleep(2)
	}
}
