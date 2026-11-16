/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gpuagent

import (
	"testing"

	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

const productNameKey = "product_name"

func productNameGPU(bdf, market, board string) *amdgpu.GPU {
	return &amdgpu.GPU{
		Spec: &amdgpu.GPUSpec{Id: []byte(bdf)},
		Status: &amdgpu.GPUStatus{
			MarketName: market,
			CardSeries: board,
			PCIeStatus: &amdgpu.GPUPCIeStatus{PCIeBusId: bdf},
		},
	}
}

func TestProductNameLabelOptional(t *testing.T) {
	logger.Init(true)
	ga := &GPUAgentGPUClient{}
	gpu := productNameGPU("0000:e3:00.0", "AMD Instinct MI350P", "\n")

	ga.initLabelConfigs(nil)
	_, ok := ga.populateLabelsFromGPU(nil, gpu, nil)[productNameKey]
	assert.Assert(t, !ok, "product_name must be off by default")

	ga.initLabelConfigs(&exportermetrics.GPUMetricConfig{Labels: []string{"product_name"}})
	assert.Equal(t, ga.populateLabelsFromGPU(nil, gpu, nil)[productNameKey], "AMD_Instinct_MI350P")
}

func TestProductNameLabelUsesParentPartition(t *testing.T) {
	logger.Init(true)
	ga := &GPUAgentGPUClient{}
	ga.initLabelConfigs(&exportermetrics.GPUMetricConfig{Labels: []string{"PRODUCT_NAME"}})

	// partition children carry no identity of their own; the parent supplies it
	child := productNameGPU("0000:05:00.1", "", "")
	parent := productNameGPU("0000:05:00.0", "AMD Instinct MI300X", "AMD Instinct MI300X OAM")
	partitionMap := map[string]*amdgpu.GPU{"0000:05:00": parent}

	assert.Equal(t, ga.populateLabelsFromGPU(nil, child, partitionMap)[productNameKey], "AMD_Instinct_MI300X_OAM")
}
