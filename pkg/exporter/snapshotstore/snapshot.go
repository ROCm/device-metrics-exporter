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

// Package snapshotstore is a device-agnostic in-memory snapshot store: the
// single source of truth for GPU/NIC/IFOE metric samples. Writers publish an
// immutable per-device snapshot via copy-on-write; readers load it lock-free.
package snapshotstore

import "github.com/prometheus/client_golang/prometheus"

// Sample is one field's value, label set, and sample time (microseconds).
type Sample struct {
	Value  float64
	Labels prometheus.Labels
	TsUs   int64
}

// DeviceSnapshot is an immutable set of samples for one device. It is built
// once on the writer path and never mutated after publish; readers only call
// the accessors below.
type DeviceSnapshot struct {
	fields map[string]Sample
}

func newDeviceSnapshot(fields map[string]Sample) *DeviceSnapshot {
	return &DeviceSnapshot{fields: fields}
}

// Get returns the sample for field and whether it is present.
func (d *DeviceSnapshot) Get(field string) (Sample, bool) {
	if d == nil {
		return Sample{}, false
	}
	s, ok := d.fields[field]
	return s, ok
}

// Len returns the number of fields.
func (d *DeviceSnapshot) Len() int {
	if d == nil {
		return 0
	}
	return len(d.fields)
}

// Range calls fn for every field, stopping early if fn returns false.
func (d *DeviceSnapshot) Range(fn func(field string, s Sample) bool) {
	if d == nil {
		return
	}
	for f, s := range d.fields {
		if !fn(f, s) {
			return
		}
	}
}
