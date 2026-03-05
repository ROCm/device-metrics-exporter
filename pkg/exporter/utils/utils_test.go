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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

func TestIsValueApplicable(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected bool
	}{
		{
			name:     "uint64 - na",
			input:    0xFFFFFFFFFFFFFFFF,
			expected: false,
		},
		{
			name:     "uint32 - na",
			input:    4294967295,
			expected: false,
		},
		{
			name:     "uint32 - na",
			input:    0xFFFFFFFF,
			expected: false,
		},
		{
			name:     "uint16 - na",
			input:    65535,
			expected: false,
		},
		{
			name:     "uint16 - na",
			input:    0xFFFF,
			expected: false,
		},
		{
			name:     "uint8 - na",
			input:    255,
			expected: false,
		},
		{
			name:     "uint8 - na",
			input:    0xFF,
			expected: false,
		},
		{
			name:     "uint32 - valid",
			input:    200,
			expected: true,
		},
		{
			name:     "uint16 - valid",
			input:    100,
			expected: true,
		},
		{
			name:     "uint8 - valid",
			input:    50,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValueApplicable(tt.input)
			if got != tt.expected {
				t.Errorf("IsApplicable(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected float64
	}{
		{
			name:     "uint64 - na",
			input:    0xFFFFFFFFFFFFFFFF,
			expected: 0,
		},
		{
			name:     "uint32 - na",
			input:    0xFFFFFFFF,
			expected: 0,
		},
		{
			name:     "uint16 - na",
			input:    0xFFFF,
			expected: 0,
		},
		{
			name:     "uint8 - na",
			input:    0xFF,
			expected: 0,
		},
		{
			name:     "uint32 - valid",
			input:    200,
			expected: 200,
		},
		{
			name:     "uint16 - valid",
			input:    100,
			expected: 100,
		},
		{
			name:     "uint8 - valid",
			input:    50,
			expected: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUint64(tt.input)
			if got != tt.expected {
				t.Errorf("IsApplicable(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}
func TestNormalizePercent(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{name: "in range", input: 50.0, expected: 50.0},
		{name: "exact lower bound", input: 0.0, expected: 0.0},
		{name: "exact upper bound", input: 100.0, expected: 100.0},
		{name: "marginally above upper bound", input: 100.001, expected: 100.0},
		{name: "marginally below lower bound", input: -0.001, expected: 0.0},
		{name: "fp rounding artifact from rocprofiler", input: 100.009, expected: 100.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePercent(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizePercent(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeFraction(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{name: "in range", input: 0.5, expected: 0.5},
		{name: "exact lower bound", input: 0.0, expected: 0.0},
		{name: "exact upper bound", input: 1.0, expected: 1.0},
		{name: "marginally above upper bound", input: 1.00008, expected: 1.0},
		{name: "marginally below lower bound", input: -0.001, expected: 0.0},
		{name: "fp rounding artifact from rocprofiler", input: 1.00006, expected: 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeFraction(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeFraction(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestValidateAndExport(t *testing.T) {
	type labelSet map[string]string

	tests := []struct {
		name         string
		fieldName    string
		labels       labelSet
		value        interface{}
		expected     float64
		expectExport bool
	}{
		{
			name:         "Valid uint64 value",
			fieldName:    "test_field",
			labels:       labelSet{"gpu": "0"},
			value:        uint64(123),
			expected:     123,
			expectExport: true,
		},
		{
			name:         "NA uint64 value",
			fieldName:    "test_field",
			labels:       labelSet{"gpu": "1"},
			value:        uint64(math.MaxUint64),
			expected:     0,
			expectExport: false,
		},
		{
			name:         "Valid uint32 value",
			fieldName:    "test_field",
			labels:       labelSet{"gpu": "2"},
			value:        uint32(456),
			expected:     456,
			expectExport: true,
		},
		{
			name:         "NA uint32 value",
			fieldName:    "test_field",
			labels:       labelSet{"gpu": "3"},
			value:        uint32(math.MaxUint32),
			expected:     0,
			expectExport: false,
		},
		{
			name:         "Nil labels",
			fieldName:    "test_field",
			labels:       nil,
			value:        uint64(789),
			expected:     0,
			expectExport: false,
		},
		{
			name:         "Nil value",
			fieldName:    "test_field",
			labels:       labelSet{"gpu": "4"},
			value:        nil,
			expected:     0,
			expectExport: false,
		},
	}

	for _, tt := range tests {
		metric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_metric",
			Help: "Test metric for ValidateAndExport",
		}, []string{"gpu"})

		t.Run(tt.name, func(t *testing.T) {

			ValidateAndExport(*metric, tt.fieldName, tt.labels, tt.value)
			labels := prometheus.Labels(tt.labels)
			if labels != nil && tt.expectExport {
				// Actually get the value from the metric
				// Use prometheus/testutil to get the value
				val := testutil.ToFloat64(metric.With(labels))
				if val != tt.expected {
					t.Errorf("ValidateAndExport set value %v; want %v", val, tt.expected)
				}
				t.Logf("ValidateAndExport fieldName %v set value %v; want %v", tt.fieldName, val, tt.expected)
			} else {
				// Should not export, value should be zero
				if tt.labels != nil {
					val := testutil.ToFloat64(metric.With(labels))
					if val != 0 {
						t.Errorf("ValidateAndExport should not export, got %v", val)
					} else {
						t.Logf("ValidateAndExport should not export, got %v", val)
					}
				}
			}
		})
	}
}
