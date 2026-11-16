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

import "testing"

func TestGPUProductName(t *testing.T) {
	tests := []struct {
		name, board, market, want string
	}{
		{"board name preferred like labeller", "AMD Instinct MI300X OAM", "AMD Instinct MI300X", "AMD_Instinct_MI300X_OAM"},
		{"blank FRU falls back to market", "\n", "AMD Instinct MI350P", "AMD_Instinct_MI350P"},
		{"empty board falls back to market", "", "AMD Instinct MI210", "AMD_Instinct_MI210"},
		{"N/A board falls back to market", "N/A", "AMD Radeon AI PRO R9700", "AMD_Radeon_AI_PRO_R9700"},
		{"board trimmed", "  Instinct MI210 \n", "AMD Instinct MI210", "Instinct_MI210"},
		{"parens dropped", "AMD INSTINCT MI300 (MCM) OAM", "", "AMD_INSTINCT_MI300_MCM_OAM"},
		{"slash separator runs collapsed", "", "AMD Instinct MI250X / MI250", "AMD_Instinct_MI250X_MI250"},
		{"inner whitespace runs collapsed", "AMD  Radeon   AI PRO R9700", "", "AMD_Radeon_AI_PRO_R9700"},
		{"both empty", "\n", "", ""},
		{"both N/A", "N/A", "N/A", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := GPUProductName(tc.board, tc.market); got != tc.want {
				t.Errorf("GPUProductName(%q, %q) = %q, want %q", tc.board, tc.market, got, tc.want)
			}
		})
	}
}
