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
	"math"
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
func TestNormalizeUint64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
	}{
		{
			name:     "Valid uint64",
			input:    uint64(100),
			expected: 100.0,
		},
		{
			name:     "Valid uint32",
			input:    uint32(50),
			expected: 50.0,
		},
		{
			name:     "Valid uint16",
			input:    uint16(25),
			expected: 25.0,
		},
		{
			name:     "Valid uint8",
			input:    uint8(10),
			expected: 10.0,
		},
		{
			name:     "MaxUint64 returns 0",
			input:    uint64(18446744073709551615), // math.MaxUint64
			expected: 0.0,
		},
		{
			name:     "MaxUint32 in uint64 returns 0",
			input:    uint64(4294967295), // math.MaxUint32
			expected: 0.0,
		},
		{
			name:     "MaxUint32 returns 0",
			input:    uint32(4294967295), // math.MaxUint32
			expected: 0.0,
		},
		{
			name:     "MaxUint16 in uint32 returns 0",
			input:    uint32(65535), // math.MaxUint16
			expected: 0.0,
		},
		{
			name:     "MaxUint16 returns 0",
			input:    uint16(65535), // math.MaxUint16
			expected: 0.0,
		},
		{
			name:     "MaxUint8 in uint16 returns 0",
			input:    uint16(255), // math.MaxUint8
			expected: 0.0,
		},
		{
			name:     "MaxUint8 returns 0",
			input:    uint8(255), // math.MaxUint8
			expected: 0.0,
		},
		{
			name:     "Zero uint64",
			input:    uint64(0),
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUint64(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeUint64(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
	}{
		{
			name:     "Valid float64",
			input:    float64(123.45),
			expected: 123.45,
		},
		{
			name:     "Valid float32",
			input:    float32(648),
			expected: 648,
		},
		{
			name:     "Zero float64",
			input:    float64(0.0),
			expected: 0.0,
		},
		{
			name:     "Zero float32",
			input:    float32(0.0),
			expected: 0.0,
		},
		{
			name:     "Negative float64",
			input:    float64(-45.67),
			expected: -45.67,
		},
		{
			name:     "MaxFloat64 returns 0",
			input:    float64(1.7976931348623157e+308), // math.MaxFloat64
			expected: 0.0,
		},
		{
			name:     "MaxFloat32 in float64 returns 0",
			input:    math.MaxFloat32, // math.MaxFloat32
			expected: 0.0,
		},
		{
			name:     "MaxFloat32 returns 0",
			input:    math.MaxFloat64, // math.MaxFloat32
			expected: 0.0,
		},
		{
			name:     "max  65535 in float64 returns 0",
			input:    float64(65535), // math.MaxUint16
			expected: 0.0,
		},
		{
			name:     "Small positive float64",
			input:    float64(0.001),
			expected: 0.001,
		},
		{
			name:     "Small positive float32",
			input:    float32(0.001),
			expected: 0.0010000000474974513, // float32 precision
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeFloat(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeFloat(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}
