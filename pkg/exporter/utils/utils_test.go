/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"testing"
)

func TestGetPCIeBaseAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard PCIe address with function",
			input:    "0000:03:00.0",
			expected: "0000:03:00",
		},
		{
			name:     "PCIe address with multi-digit function",
			input:    "0000:03:00.12",
			expected: "0000:03:00",
		},
		{
			name:     "Malformed address no dot",
			input:    "0000:03:00",
			expected: "0000:03:00",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only function",
			input:    ".0",
			expected: "",
		},
		{
			name:     "Multiple dots",
			input:    "0000:03:00.0.1",
			expected: "0000:03:00.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPCIeBaseAddress(tt.input)
			if got != tt.expected {
				t.Errorf("GetPCIeBaseAddress(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}
