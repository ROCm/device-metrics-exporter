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
	"fmt"
	"log"
	"os"
	"strings"

	testutils "github.com/ROCm/device-metrics-exporter/test/e2e/utils"
)

var (
	configPath = "/config_test/"
	ports      = []int{5000, 5002}
)

type MockExporter struct {
	Name       string
	ImageURL   string
	configPath string
	tu         *testutils.TestUtils
	portMap    map[int]int
}

func NewMockExporter(name, url string) *MockExporter {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current directory:", err)
		return nil
	}
	portMap := make(map[int]int)
	for _, port := range ports {
		portMap[port] = port
	}
	return &MockExporter{
		Name:       name,
		ImageURL:   url,
		configPath: fmt.Sprintf("%v%v", dir, configPath),
		tu:         testutils.New(),
		portMap:    portMap,
	}
}

func (m *MockExporter) SetPortMap(pMap map[int]int) error {
	m.portMap = pMap
	return nil
}

func (m *MockExporter) SkipConfigMount() {
	m.configPath = ""
}

func (m *MockExporter) Start() error {
	portsExposed := []string{}
	for hport, cport := range m.portMap {
		dockerPort := fmt.Sprintf(" -p %v:%v", hport, cport)
		portsExposed = append(portsExposed, dockerPort)
	}
	mountOps := ""
	if m.configPath != "" {
		mountOps = fmt.Sprintf(" -v %v:/etc/metrics ", m.configPath)
	}
	cmd := fmt.Sprintf("docker run --rm -itd --privileged --name %v %v %v -e PATH=$PATH:/home/amd/bin/ %v", m.Name, strings.Join(portsExposed, " "), mountOps, m.ImageURL)
	log.Print(cmd)
	resp := m.tu.LocalCommandOutput(cmd)
	if resp == "" {
		return fmt.Errorf("cmd %v failed", cmd)
	}
	return nil
}

func (m *MockExporter) Restart() error {
	cmd := fmt.Sprintf("docker restart %v", m.Name)
	log.Print(cmd)
	resp := m.tu.LocalCommandOutput(cmd)
	if resp == "" {
		return fmt.Errorf("cmd %v failed", cmd)
	}
	return nil
}

func (m *MockExporter) RunCmd(cmd string) (string, error) {
	fullCmd := fmt.Sprintf("docker exec %v %v", m.Name, cmd)
	log.Print(fullCmd)
	resp := m.tu.LocalCommandOutput(fullCmd)
	if resp == "" {
		return resp, fmt.Errorf("empty response")
	}
	return resp, nil
}

func (m *MockExporter) Stop() error {
	cmd := fmt.Sprintf("docker stop %v", m.Name)
	log.Print(cmd)
	_ = m.tu.LocalCommandOutput(cmd)
	return nil
}

func (m *MockExporter) Cleanup() error {
	cmd := fmt.Sprintf("docker rm %v -f", m.Name)
	log.Print(cmd)
	_ = m.tu.LocalCommandOutput(cmd)
	return nil
}
