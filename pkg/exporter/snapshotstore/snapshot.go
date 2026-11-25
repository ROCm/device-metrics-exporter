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

import (
	"math"
	"runtime"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// hotLoadSpins is how many times Load retries while a Set is in flight before
// yielding the processor.
const hotLoadSpins = 16

// HotCell is a single-writer, multi-reader value slot for high-rate fields. A
// snapshot Sample registered with Store.RegisterHot points at a cell, and the
// writer updates the value in place with Set: no lock, no allocation, no
// snapshot copy. Set must be called by one goroutine only; readers never take
// a lock or wait for a store writer, never see a torn value/ts pair (seqlock
// over atomics), and retry only while a Set is in flight.
type HotCell struct {
	seq atomic.Uint64 // odd while a Set is in progress
	val atomic.Uint64 // math.Float64bits(value)
	ts  atomic.Int64  // TsUs
}

// Set publishes v with sample time tsUs. Single writer only.
func (c *HotCell) Set(v float64, tsUs int64) {
	c.seq.Add(1)
	c.val.Store(math.Float64bits(v))
	c.ts.Store(tsUs)
	c.seq.Add(1)
}

// Load returns the latest value and sample time. It retries only while a Set
// is in flight (a few ns), yielding after hotLoadSpins attempts.
func (c *HotCell) Load() (v float64, tsUs int64) {
	for spins := 0; ; spins++ {
		s1 := c.seq.Load()
		if s1&1 == 0 {
			bits := c.val.Load()
			ts := c.ts.Load()
			if c.seq.Load() == s1 {
				return math.Float64frombits(bits), ts
			}
		}
		if spins >= hotLoadSpins {
			runtime.Gosched()
			spins = 0
		}
	}
}

// Sample is one field's value, label set, and sample time (microseconds).
type Sample struct {
	Value  float64
	Labels prometheus.Labels
	TsUs   int64
	// hot, when set, holds the live Value/TsUs; the fields above are then
	// ignored. Only Store.RegisterHot sets it.
	hot *HotCell
}

// resolve returns s with Value/TsUs read from its hot cell, if any. The copy
// handed to readers is a plain sample (hot cleared).
func (s Sample) resolve() Sample {
	if s.hot != nil {
		s.Value, s.TsUs = s.hot.Load()
		s.hot = nil
	}
	return s
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

// Get returns the sample for field and whether it is present. A hot sample is
// returned with its cell's current Value/TsUs.
func (d *DeviceSnapshot) Get(field string) (Sample, bool) {
	if d == nil {
		return Sample{}, false
	}
	s, ok := d.fields[field]
	return s.resolve(), ok
}

// Len returns the number of fields.
func (d *DeviceSnapshot) Len() int {
	if d == nil {
		return 0
	}
	return len(d.fields)
}

// Range calls fn for every field, stopping early if fn returns false. Hot
// samples are resolved as in Get, each loaded independently, so two hot fields
// of one device may come from different writer updates.
func (d *DeviceSnapshot) Range(fn func(field string, s Sample) bool) {
	if d == nil {
		return
	}
	for f, s := range d.fields {
		if !fn(f, s.resolve()) {
			return
		}
	}
}
