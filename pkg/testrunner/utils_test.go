/*
*
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
*
*/

package testrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	testrunnerGen "github.com/ROCm/device-metrics-exporter/pkg/testrunner/gen/testrunner"
)

func TestGzipResultJson(t *testing.T) {
	// Success case
	jsonPath := "/tmp/sample.json"
	gzPath := "/tmp/sample.gz"
	os.WriteFile(jsonPath, []byte(`{"hello": "world"}`), 0644)
	defer os.RemoveAll(jsonPath)

	err := GzipResultJson(jsonPath, gzPath)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if _, err := os.Stat(gzPath); os.IsNotExist(err) {
		t.Errorf("Expected gzipped file to exist")
	}
	defer os.RemoveAll(gzPath)

	// Failure case: non-existent file
	err = GzipResultJson("/tmp/does_not_exist.json", "/tmp/should_fail.gz")
	if err == nil {
		t.Errorf("Expected error for non-existent input file")
	}
}

func TestGzipFolder(t *testing.T) {
	// Success case
	dir := "/tmp/testdir"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("file one"), 0644)
	os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("file two"), 0644)

	tarGzPath := "/tmp/testdir.tgz"
	err := GzipFolder(dir, tarGzPath)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if _, err := os.Stat(tarGzPath); os.IsNotExist(err) {
		t.Errorf("Expected gzipped tar file to exist")
	}
	defer os.RemoveAll(tarGzPath)

	// Failure case: non-existent directory
	err = GzipFolder("/tmp/does_not_exist_dir", "/tmp/should_fail.tar.gz")
	if err == nil {
		t.Errorf("Expected error for non-existent directory")
	}
}

func TestTransformRunnerStatus(t *testing.T) {
	statusDBPath := "/tmp/test_status.db"
	initialStatus := &testrunnerGen.TestRunnerStatus{
		TestStatus: map[string]string{
			"kfdid1":   "running",
			"kfdid2":   "completed",
			"gpu0":     "running",   // gpu index that doesn't exist anymore
			"kfdid999": "running",   // KFD ID that doesn't exist anymore
			"kfdid123": "completed", // KFD ID that doesn't exist anymore
		},
	}
	kfdIDToIndex := map[string]string{
		"kfdid1": "gpu1",
		"kfdid2": "gpu2",
	}
	gpuIndexToKFDID := map[string]string{
		"gpu1": "kfdid1",
		"gpu2": "kfdid2",
	}

	// Write initial status to file
	data, err := json.Marshal(initialStatus)
	if err != nil {
		t.Fatalf("Failed to marshal initial status: %v", err)
	}
	err = os.WriteFile(statusDBPath, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write initial status to file: %v", err)
	}
	defer os.Remove(statusDBPath)

	// Call the function under test
	err = transformRunnerStatus(statusDBPath, kfdIDToIndex, gpuIndexToKFDID)
	if err != nil {
		t.Fatalf("transformRunnerStatus returned an error: %v", err)
	}

	// Load the transformed status
	transformedStatus, err := LoadRunnerStatus(statusDBPath)
	if err != nil {
		t.Fatalf("Failed to load transformed status: %v", err)
	}

	// Verify the transformation
	expectedStatus := map[string]string{
		"gpu1": "running",
		"gpu2": "completed",
	}
	if !equalMaps(expectedStatus, transformedStatus.TestStatus) {
		t.Errorf("Transformed status does not match expected status. Got: %v, Expected: %v", transformedStatus.TestStatus, expectedStatus)
	}
}

// Helper function to compare two maps
func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestParseAMDSMIStaticOutput(t *testing.T) {
	// Example JSON output from amd-smi
	jsonOutput := `{
    "gpu_data": [
        {
            "gpu": 0,
            "asic": {
                "market_name": "AMD Instinct MI210",
                "vendor_id": "0x1002",
                "vendor_name": "Advanced Micro Devices Inc. [AMD/ATI]",
                "subvendor_id": "0x1002",
                "device_id": "0x740f",
                "subsystem_id": "0x0c34",
                "rev_id": "0x02",
                "oam_id": "N/A",
                "num_compute_units": 104,
                "target_graphics_version": "gfx90a"
            },
            "bus": {
                "max_pcie_width": 16,
                "max_pcie_speed": {
                    "value": 16,
                    "unit": "GT/s"
                },
                "pcie_interface_version": "Gen 4",
                "slot_type": "OAM"
            },
            "vbios": {
                "name": "Mi200 pcie"
            },
            "limit": {
                "max_power": {
                    "value": 300,
                    "unit": "W"
                },
                "min_power": {
                    "value": 0,
                    "unit": "W"
                },
                "socket_power": {
                    "value": 300,
                    "unit": "W"
                },
                "slowdown_edge_temperature": {
                    "value": 99,
                    "unit": "C"
                },
                "slowdown_hotspot_temperature": {
                    "value": 100,
                    "unit": "C"
                },
                "slowdown_vram_temperature": {
                    "value": 94,
                    "unit": "C"
                },
                "shutdown_edge_temperature": {
                    "value": 99,
                    "unit": "C"
                },
                "shutdown_hotspot_temperature": {
                    "value": 105,
                    "unit": "C"
                },
                "shutdown_vram_temperature": {
                    "value": 99,
                    "unit": "C"
                }
            },
            "driver": {
                "name": "amdgpu",
                "version": "Linuxversion6.14.0-24-generic(buildd@lcy02-amd64-010)(x86_64-linux-gnu-gcc-13(Ubuntu13.3.0-6ubuntu2~24.04)13.3.0,GNUld(GNUBinutilsforUbuntu)2.42)#24~24.04.3-UbuntuSMPPREEMPT_DYNAMICMonJul716:39:17UTC2"
            },
            "board": {
                "model_number": "N/A",
                "product_serial": "0",
                "fru_id": "N/A",
                "product_name": "N/A",
                "manufacturer_name": "N/A"
            },
            "ras": {
                "eeprom_version": "0x10000",
                "bad_page_threshold": 655,
                "parity_schema": "DISABLED",
                "single_bit_schema": "DISABLED",
                "double_bit_schema": "DISABLED",
                "poison_schema": "ENABLED",
                "ecc_block_state": {
                    "UMC": "ENABLED",
                    "SDMA": "ENABLED",
                    "GFX": "ENABLED",
                    "MMHUB": "ENABLED",
                    "ATHUB": "ENABLED",
                    "PCIE_BIF": "ENABLED",
                    "HDP": "ENABLED",
                    "XGMI_WAFL": "DISABLED",
                    "DF": "ENABLED",
                    "SMN": "ENABLED",
                    "SEM": "ENABLED",
                    "MP0": "ENABLED",
                    "MP1": "ENABLED",
                    "FUSE": "ENABLED",
                    "MCA": "ENABLED",
                    "VCN": "ENABLED",
                    "JPEG": "ENABLED",
                    "IH": "ENABLED",
                    "MPIO": "ENABLED"
                }
            },
            "soc_pstate": "N/A",
            "xgmi_plpd": {
                "num_supported": 2,
                "current_id": 0,
                "plpds": [
                    {
                        "policy_id": 0,
                        "policy_description": "plpd_disallow"
                    },
                    {
                        "policy_id": 0,
                        "policy_description": ""
                    }
                ]
            },
            "process_isolation": "Disabled",
            "numa": {
                "node": 1,
                "affinity": 1,
                "cpu_affinity": {
                    "cpu_list_0": {
                        "bitmask": "AAAAAAAAAAAAAAAA",
                        "cpu_cores_affinity": "1-63"
                    },
                    "cpu_list_1": {
                        "bitmask": "0000AAAAAAAAAAAA",
                        "cpu_cores_affinity": "65-111"
                    }
                },
                "socket_affinity": {
                    "socket_0": 0,
                    "socket_1": 1
                }
            },
            "vram": {
                "type": "HBM",
                "vendor": "HYNIX",
                "size": {
                    "value": 65520,
                    "unit": "MB"
                },
                "bit_width": 4096,
                "max_bandwidth": {
                    "value": "N/A",
                    "unit": "GB/s"
                }
            },
            "cache_info": [
                {
                    "cache": 0,
                    "cache_properties": [
                        "DATA_CACHE",
                        "SIMD_CACHE"
                    ],
                    "cache_size": {
                        "value": 16,
                        "unit": "KB"
                    },
                    "cache_level": 1,
                    "max_num_cu_shared": 1,
                    "num_cache_instance": 112
                },
                {
                    "cache": 1,
                    "cache_properties": [
                        "INST_CACHE",
                        "SIMD_CACHE"
                    ],
                    "cache_size": {
                        "value": 32,
                        "unit": "KB"
                    },
                    "cache_level": 1,
                    "max_num_cu_shared": 2,
                    "num_cache_instance": 48
                },
                {
                    "cache": 2,
                    "cache_properties": [
                        "INST_CACHE",
                        "SIMD_CACHE"
                    ],
                    "cache_size": {
                        "value": 32,
                        "unit": "KB"
                    },
                    "cache_level": 1,
                    "max_num_cu_shared": 1,
                    "num_cache_instance": 8
                },
                {
                    "cache": 3,
                    "cache_properties": [
                        "DATA_CACHE",
                        "SIMD_CACHE"
                    ],
                    "cache_size": {
                        "value": 16,
                        "unit": "KB"
                    },
                    "cache_level": 1,
                    "max_num_cu_shared": 2,
                    "num_cache_instance": 48
                },
                {
                    "cache": 4,
                    "cache_properties": [
                        "DATA_CACHE",
                        "SIMD_CACHE"
                    ],
                    "cache_size": {
                        "value": 8192,
                        "unit": "KB"
                    },
                    "cache_level": 2,
                    "max_num_cu_shared": 104,
                    "num_cache_instance": 1
                }
            ],
            "clock": {
                "sys": {
                    "current level": 1,
                    "frequency_levels": {
                        "Level 0": "500 MHz",
                        "Level 1": "800 MHz",
                        "Level 2": "1700 MHz"
                    }
                },
                "mem": {
                    "current level": 3,
                    "frequency_levels": {
                        "Level 0": "400 MHz",
                        "Level 1": "700 MHz",
                        "Level 2": "1200 MHz",
                        "Level 3": "1600 MHz"
                    }
                },
                "df": {
                    "current level": 0,
                    "frequency_levels": {
                        "Level 0": "400 MHz"
                    }
                },
                "soc": {
                    "current level": 3,
                    "frequency_levels": {
                        "Level 0": "666 MHz",
                        "Level 1": "857 MHz",
                        "Level 2": "1000 MHz",
                        "Level 3": "1090 MHz",
                        "Level 4": "1333 MHz"
                    }
                },
                "dcef": "N/A",
                "vclk0": {
                    "current level": 0,
                    "frequency_levels": {
                        "Level 0": "1000 MHz"
                    }
                },
                "vclk1": "N/A",
                "dclk0": {
                    "current level": 0,
                    "frequency_levels": {
                        "Level 0": "875 MHz"
                    }
                },
                "dclk1": "N/A"
            }
        }
    ]
}`

	// Call the function under test
	deviceID, err := parseAMDSMIStaticOutput([]byte(jsonOutput))
	if err != nil {
		t.Errorf("parseAMDSMIStaticOutput returned an error: %v", err)
	}

	// Verify the result
	expectedDeviceID := "MI210"
	if deviceID != expectedDeviceID {
		t.Errorf("Expected Device ID %s, got %s", expectedDeviceID, deviceID)
	}
}

func TestGetAddr(t *testing.T) {
	// Test with int
	val := 42
	ptr := GetAddr(val)
	if ptr == nil {
		t.Errorf("GetAddr returned nil pointer")
	}
	if *ptr != val {
		t.Errorf("GetAddr returned pointer to wrong value: got %v, want %v", *ptr, val)
	}

	// Test with string
	str := "hello"
	strPtr := GetAddr(str)
	if strPtr == nil {
		t.Errorf("GetAddr returned nil pointer for string")
	}
	if *strPtr != str {
		t.Errorf("GetAddr returned pointer to wrong string: got %v, want %v", *strPtr, str)
	}
}

func TestDeref(t *testing.T) {
	// Test with non-nil pointer
	val := 100
	ptr := &val
	got := Deref(ptr)
	if got != val {
		t.Errorf("Deref returned wrong value: got %v, want %v", got, val)
	}

	// Test with nil pointer (int)
	var nilIntPtr *int
	gotZero := Deref(nilIntPtr)
	if gotZero != 0 {
		t.Errorf("Deref(nil) for int did not return zero value: got %v, want 0", gotZero)
	}

	// Test with nil pointer (string)
	var nilStrPtr *string
	gotStr := Deref(nilStrPtr)
	if gotStr != "" {
		t.Errorf("Deref(nil) for string did not return zero value: got %q, want \"\"", gotStr)
	}
}
