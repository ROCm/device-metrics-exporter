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

package utils

import "strings"

// same replacer as the k8s node labeller's product-name
// (ROCm/k8s-device-plugin cmd/k8s-node-labeller/main.go)
var gpuProductNameReplacer = strings.NewReplacer(" ", "_", "(", "", ")", "", "/", "_")

// GPUProductName follows the node labeller's amd.com/gpu.product-name: board
// product name, else the device-id based market name (the labeller's libdrm
// amdgpu.ids fallback). Board FRU fields can be "\n" on some boards (MI350P).
// Separator runs are collapsed, unlike the labeller.
func GPUProductName(boardProductName, marketName string) string {
	name := strings.TrimSpace(boardProductName)
	if name == "" || name == "N/A" {
		name = strings.TrimSpace(marketName)
	}
	if name == "" || name == "N/A" {
		return ""
	}
	parts := strings.FieldsFunc(gpuProductNameReplacer.Replace(name), func(r rune) bool { return r == '_' })
	return strings.Join(parts, "_")
}
