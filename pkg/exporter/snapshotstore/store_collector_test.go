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

package snapshotstore

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Two devices, a gauge + counter field with per-device labels, plus one field
// with no descriptor. Asserts exact exposition text: described fields render,
// the undescribed field is omitted, counter vs gauge TYPE is honored.
func TestStoreCollectorRendersDescribedFields(t *testing.T) {
	st := New()
	st.Update("gpu-0", map[string]Sample{
		"gpu_power":     {Value: 100, Labels: prometheus.Labels{"gpu_id": "0"}},
		"gpu_pkg_power": {Value: 5, Labels: prometheus.Labels{"gpu_id": "0"}},
		"undescribed":   {Value: 7, Labels: prometheus.Labels{"gpu_id": "0"}},
	})
	st.Update("gpu-1", map[string]Sample{
		"gpu_power":     {Value: 200, Labels: prometheus.Labels{"gpu_id": "1"}},
		"gpu_pkg_power": {Value: 9, Labels: prometheus.Labels{"gpu_id": "1"}},
	})

	descs := []FieldDesc{
		{Field: "gpu_power", Name: "amd_gpu_power", Help: "GPU power.",
			ValueType: prometheus.GaugeValue, LabelNames: []string{"gpu_id"}},
		{Field: "gpu_pkg_power", Name: "amd_gpu_pkg_power_total", Help: "Package power counter.",
			ValueType: prometheus.CounterValue, LabelNames: []string{"gpu_id"}},
	}

	c := NewStoreCollector(st, descs)

	// Register in a fresh registry to exercise Describe + duplicate detection.
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Gather sorts families by name and samples by label, so output is stable.
	want := `# HELP amd_gpu_pkg_power_total Package power counter.
# TYPE amd_gpu_pkg_power_total counter
amd_gpu_pkg_power_total{gpu_id="0"} 5
amd_gpu_pkg_power_total{gpu_id="1"} 9
# HELP amd_gpu_power GPU power.
# TYPE amd_gpu_power gauge
amd_gpu_power{gpu_id="0"} 100
amd_gpu_power{gpu_id="1"} 200
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Fatalf("unexpected collect output:\n%v", err)
	}
}

// Label values are pulled in LabelNames order; a declared label absent from the
// sample renders as empty string. The positional desc->value mapping is what is
// under test (the text encoder sorts label names alphabetically on output).
func TestStoreCollectorLabelOrderingAndMissingKey(t *testing.T) {
	st := New()
	st.Update("gpu-0", map[string]Sample{
		"temp": {Value: 42, Labels: prometheus.Labels{"gpu_id": "0", "zone": "edge"}},
	})
	descs := []FieldDesc{
		{Field: "temp", Name: "amd_temp", Help: "Temp.",
			ValueType: prometheus.GaugeValue, LabelNames: []string{"zone", "gpu_id", "card"}},
	}
	c := NewStoreCollector(st, descs)

	want := `# HELP amd_temp Temp.
# TYPE amd_temp gauge
amd_temp{card="",gpu_id="0",zone="edge"} 42
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Fatalf("unexpected collect output:\n%v", err)
	}
}
