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
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// One writer Sets (v, ts=v) with v strictly increasing while readers Get and
// Range the hot sample through the store. A reader must never see a torn pair
// (ts != value) or a value going backwards. Must pass under -race.
func TestHotCellConcurrent(t *testing.T) {
	const (
		nReaders = 8
		nWrites  = 200000
	)
	st := New()
	st.Update("gpu-0", map[string]Sample{"cold": {Value: 7, TsUs: 7}})
	cell := st.RegisterHot("gpu-0", "hot", prometheus.Labels{"gpu_id": "0"}, 0, 0)

	var (
		wg   sync.WaitGroup
		done atomic.Bool
		torn atomic.Int64
		back atomic.Int64
	)
	for r := 0; r < nReaders; r++ {
		wg.Add(1)
		go func(useRange bool) {
			defer wg.Done()
			last := -1.0
			check := func(s Sample) {
				if float64(s.TsUs) != s.Value {
					torn.Add(1)
				}
				if s.Value < last {
					back.Add(1)
				}
				last = s.Value
			}
			for !done.Load() {
				snap := st.Snapshot("gpu-0")
				if useRange {
					snap.Range(func(f string, s Sample) bool {
						if f == "hot" {
							check(s)
						}
						return true
					})
					continue
				}
				s, ok := snap.Get("hot")
				if !ok {
					t.Error("hot sample missing")
					return
				}
				check(s)
			}
		}(r%2 == 0)
	}
	for i := 1; i <= nWrites; i++ {
		cell.Set(float64(i), int64(i))
	}
	done.Store(true)
	wg.Wait()

	if n := torn.Load(); n != 0 {
		t.Fatalf("%d torn value/ts reads", n)
	}
	if n := back.Load(); n != 0 {
		t.Fatalf("%d non-monotonic reads", n)
	}
	if v, ts := cell.Load(); v != nWrites || ts != nWrites {
		t.Fatalf("final Load = (%v, %v), want (%v, %v)", v, ts, nWrites, nWrites)
	}
}

// Get and Range resolve a hot sample to its cell's current value and return a
// plain sample; RegisterHot keeps other fields; a COW Update of another field
// keeps the cell.
func TestHotCellResolve(t *testing.T) {
	st := New()
	lbl := prometheus.Labels{"gpu_id": "0"}
	st.Update("gpu-0", map[string]Sample{"cold": {Value: 1, Labels: lbl, TsUs: 10}})

	cell := st.RegisterHot("gpu-0", "hot", lbl, 5, 50)
	snap := st.Snapshot("gpu-0")
	if snap.Len() != 2 {
		t.Fatalf("Len = %d, want 2 (RegisterHot must keep other fields)", snap.Len())
	}
	if s, ok := snap.Get("cold"); !ok || s.Value != 1 || s.TsUs != 10 {
		t.Fatalf("cold = %+v %v", s, ok)
	}
	s, ok := snap.Get("hot")
	if !ok || s.Value != 5 || s.TsUs != 50 || s.Labels["gpu_id"] != "0" {
		t.Fatalf("hot initial = %+v %v", s, ok)
	}
	if s.hot != nil {
		t.Fatal("Get must return a resolved (plain) sample")
	}

	// In-place Set is visible through the already-loaded snapshot.
	cell.Set(6, 60)
	if s, _ := snap.Get("hot"); s.Value != 6 || s.TsUs != 60 {
		t.Fatalf("hot after Set = %+v", s)
	}
	seen := 0
	snap.Range(func(f string, s Sample) bool {
		if f == "hot" {
			seen++
			if s.Value != 6 || s.TsUs != 60 || s.hot != nil {
				t.Fatalf("Range hot = %+v", s)
			}
		}
		return true
	})
	if seen != 1 {
		t.Fatalf("Range saw hot %d times", seen)
	}

	// COW Update / UpdateIfChanged of other fields carry the cell over.
	ep := st.Epoch()
	st.Update("gpu-0", map[string]Sample{"cold": {Value: 2, Labels: lbl}, "other": {Value: 3}})
	st.UpdateIfChanged("gpu-0", map[string]Sample{"const": {Value: 4}})
	cell.Set(7, 70)
	if s, _ := st.Snapshot("gpu-0").Get("hot"); s.Value != 7 || s.TsUs != 70 {
		t.Fatalf("hot after other-field updates = %+v", s)
	}
	if st.Epoch() != ep {
		t.Fatal("updates of other fields must not bump the epoch")
	}
}

// Epoch changes exactly on the events that can orphan a registered cell.
func TestStoreEpoch(t *testing.T) {
	st := New()
	lbl := prometheus.Labels{"gpu_id": "0"}
	expect := func(name string, bumped bool, fn func()) {
		t.Helper()
		before := st.Epoch()
		fn()
		after := st.Epoch()
		if bumped && after <= before {
			t.Fatalf("%s: epoch %d -> %d, want bump", name, before, after)
		}
		if !bumped && after != before {
			t.Fatalf("%s: epoch %d -> %d, want unchanged", name, before, after)
		}
	}

	var cell *HotCell
	expect("plain Update", false, func() {
		st.Update("gpu-0", map[string]Sample{"cold": {Value: 1, Labels: lbl}})
	})
	expect("RegisterHot", false, func() {
		cell = st.RegisterHot("gpu-0", "hot", lbl, 1, 1)
	})
	expect("HotCell.Set", false, func() { cell.Set(2, 2) })
	// Re-registering a live hot field (a writer rebuilding its cache) must not
	// bump the epoch, or every rebuild would trigger the next one.
	expect("RegisterHot over hot", false, func() {
		cell = st.RegisterHot("gpu-0", "hot", lbl, 2, 2)
	})
	cell.Set(3, 3)
	if s, _ := st.Snapshot("gpu-0").Get("hot"); s.Value != 3 {
		t.Fatalf("re-registered cell not live: %+v", s)
	}
	cell.Set(2, 2)
	expect("Update other field", false, func() {
		st.Update("gpu-0", map[string]Sample{"cold": {Value: 2}})
	})
	expect("UpdateIfChanged equal to hot value", false, func() {
		st.UpdateIfChanged("gpu-0", map[string]Sample{"hot": {Value: 2}})
	})
	expect("DropFields removing nothing", false, func() {
		st.DropFields(map[string]struct{}{"absent": {}})
	})
	expect("DropFields empty", false, func() { st.DropFields(nil) })

	expect("Update replacing hot", true, func() {
		st.Update("gpu-0", map[string]Sample{"hot": {Value: 9}})
	})
	if s, _ := st.Snapshot("gpu-0").Get("hot"); s.Value != 9 {
		t.Fatalf("replaced hot = %+v", s)
	}
	cell.Set(10, 10) // orphaned cell: must not affect the store
	if s, _ := st.Snapshot("gpu-0").Get("hot"); s.Value != 9 {
		t.Fatalf("orphaned cell leaked into store: %+v", s)
	}

	expect("Update replacing plain", false, func() {
		st.Update("gpu-0", map[string]Sample{"hot": {Value: 11}})
	})

	cell = st.RegisterHot("gpu-0", "hot", lbl, 1, 1)
	expect("UpdateIfChanged replacing hot", true, func() {
		st.UpdateIfChanged("gpu-0", map[string]Sample{"hot": {Value: 12}})
	})

	cell = st.RegisterHot("gpu-0", "hot", lbl, 1, 1)
	expect("DropFields removing hot", true, func() {
		st.DropFields(map[string]struct{}{"hot": {}})
	})
	if _, ok := st.Snapshot("gpu-0").Get("hot"); ok {
		t.Fatal("DropFields left the hot sample")
	}

	expect("ReplaceOwned", true, func() {
		st.ReplaceOwned("wl-", map[string]map[string]Sample{"wl-1": {"x": {Value: 1}}})
	})
	expect("Reset", true, func() { st.Reset() })
}

// StoreCollector renders a hot sample exactly like a plain sample with the
// same value and labels.
func TestHotCellCollectorEqualsPlain(t *testing.T) {
	descs := []FieldDesc{{
		Field: "power", Name: "gpu_power", Help: "h",
		ValueType: prometheus.GaugeValue, LabelNames: []string{"gpu_id"},
	}}
	lbl := prometheus.Labels{"gpu_id": "3"}

	plain := New()
	plain.Update("gpu-3", map[string]Sample{"power": {Value: 123.5, Labels: lbl, TsUs: 1}})

	hot := New()
	cell := hot.RegisterHot("gpu-3", "power", lbl, 0, 0)
	cell.Set(123.5, 1)

	want := collectOne(t, NewStoreCollector(plain, descs))
	got := collectOne(t, NewStoreCollector(hot, descs))
	if got.String() != want.String() {
		t.Fatalf("hot collector output %q != plain %q", got.String(), want.String())
	}
}

func collectOne(t *testing.T, c prometheus.Collector) *dto.Metric {
	t.Helper()
	m, err := collectSingle(c)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// collectSingle runs one Collect and returns its only metric. Safe to call off
// the test goroutine.
func collectSingle(c prometheus.Collector) (*dto.Metric, error) {
	ch := make(chan prometheus.Metric, 4)
	c.Collect(ch)
	close(ch)
	var out []*dto.Metric
	for m := range ch {
		pb := &dto.Metric{}
		if err := m.Write(pb); err != nil {
			return nil, err
		}
		out = append(out, pb)
	}
	if len(out) != 1 {
		return nil, fmt.Errorf("collected %d metrics, want 1", len(out))
	}
	return out[0], nil
}

// A writer Setting as fast as it can never blocks a scraper, and a scrape never
// waits for a store writer: even with the device's write mutex held, Get and
// Collect complete and show the hot cell's newest value.
func TestHotCellScrapeNotBlocked(t *testing.T) {
	st := New()
	lbl := prometheus.Labels{"gpu_id": "0"}
	cell := st.RegisterHot("gpu-0", "power", lbl, 0, 0)
	col := NewStoreCollector(st, []FieldDesc{{
		Field: "power", Name: "gpu_power", Help: "h",
		ValueType: prometheus.GaugeValue, LabelNames: []string{"gpu_id"},
	}})

	// Stall every COW writer on the device for the whole test.
	slot := (*st.devices.Load())["gpu-0"]
	slot.writeMu.Lock()
	defer slot.writeMu.Unlock()

	var (
		done    atomic.Bool
		writes  atomic.Int64
		scrapes atomic.Int64
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() { // single hot-cell writer
		defer wg.Done()
		for i := int64(1); !done.Load(); i++ {
			cell.Set(float64(i), i)
			writes.Store(i)
		}
	}()
	scraped := make(chan struct{})
	go func() {
		defer close(scraped)
		for i := 0; i < 2000; i++ {
			if _, err := collectSingle(col); err != nil {
				t.Error(err)
				return
			}
			if _, ok := st.Snapshot("gpu-0").Get("power"); !ok {
				t.Error("power missing")
				return
			}
			scrapes.Add(1)
		}
	}()
	select {
	case <-scraped:
	case <-time.After(30 * time.Second):
		t.Fatalf("scraper blocked: %d scrapes done", scrapes.Load())
	}
	done.Store(true)
	wg.Wait()
	if writes.Load() == 0 {
		t.Fatal("writer made no progress")
	}
	// The final value is visible to a reader without any store write.
	v, _ := cell.Load()
	if s, _ := st.Snapshot("gpu-0").Get("power"); s.Value != v {
		t.Fatalf("Get = %v, cell = %v", s.Value, v)
	}
}

// The hot write path must not allocate.
func TestHotCellSetNoAlloc(t *testing.T) {
	cell := New().RegisterHot("gpu-0", "power", nil, 0, 0)
	i := 0.0
	if n := testing.AllocsPerRun(1000, func() {
		i++
		cell.Set(i, int64(i))
	}); n != 0 {
		t.Fatalf("HotCell.Set allocs/op = %v, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() { _, _ = cell.Load() }); n != 0 {
		t.Fatalf("HotCell.Load allocs/op = %v, want 0", n)
	}
}

func BenchmarkHotCellSet(b *testing.B) {
	cell := New().RegisterHot("gpu-0", "power", nil, 0, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cell.Set(float64(i), int64(i))
	}
}

func BenchmarkHotCellGet(b *testing.B) {
	st := New()
	cell := st.RegisterHot("gpu-0", "power", nil, 0, 0)
	cell.Set(1, 1)
	snap := st.Snapshot("gpu-0")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = snap.Get("power")
	}
}
