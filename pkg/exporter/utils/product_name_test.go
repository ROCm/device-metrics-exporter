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

func TestNormalizeProductName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"Instinct MI210", "Instinct_MI210"},
		{"  Instinct MI210 \n", "Instinct_MI210"},
		{"Radeon RX 9070 XT", "Radeon_RX_9070_XT"},
		{"AMD Instinct MI250X / MI250", "AMD_Instinct_MI250X_MI250"},
		{"AMD INSTINCT MI300 (MCM) OAM", "AMD_INSTINCT_MI300_MCM_OAM"},
		{"Foo ( Bar )", "Foo_Bar"},
		{"MI300X-HF", "MI300X-HF"},
	}
	for _, tt := range tests {
		if got := NormalizeProductName(tt.in); got != tt.want {
			t.Errorf("NormalizeProductName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
