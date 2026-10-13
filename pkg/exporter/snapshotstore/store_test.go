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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func genDeltas(fields []string, g float64) map[string]Sample {
	d := make(map[string]Sample, len(fields))
	for _, f := range fields {
		d[f] = Sample{Value: g, Labels: prometheus.Labels{"gen": "x"}, TsUs: int64(g)}
	}
	return d
}

// Every Update stamps all fields with one generation value g. A reader that
// observes two different values across a snapshot's fields saw a torn or
// half-applied swap. Must pass under -race.
func TestStoreConcurrentIntegrity(t *testing.T) {
	st := New()
	const nFields = 16
	uuids := []string{"gpu-0", "gpu-1", "gpu-2"}
	fields := make([]string, nFields)
	for i := range fields {
		fields[i] = fmt.Sprintf("f%02d", i)
	}
	for _, u := range uuids {
		st.Update(u, genDeltas(fields, 0))
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for _, u := range uuids {
		u := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			var g float64
			for {
				select {
				case <-stop:
					return
				default:
				}
				g++
				st.Update(u, genDeltas(fields, g))
			}
		}()
	}

	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, u := range uuids {
					snap := st.Snapshot(u)
					if snap == nil {
						continue
					}
					want, ok := snap.Get(fields[0])
					if !ok {
						t.Errorf("missing field %s", fields[0])
						return
					}
					torn := false
					snap.Range(func(_ string, s Sample) bool {
						if s.Value != want.Value {
							torn = true
							return false
						}
						return true
					})
					if torn {
						t.Errorf("torn snapshot for %s: field != gen %v", u, want.Value)
						return
					}
					if snap.Len() != nFields {
						t.Errorf("snapshot %s has %d fields, want %d", u, snap.Len(), nFields)
						return
					}
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// Snapshot must never acquire the per-device writer mutex. Holding writeMu and
// then calling Snapshot in the same goroutine would deadlock (hang the test) if
// it did.
func TestLockFreeReadWhileWriterHoldsMutex(t *testing.T) {
	st := New()
	st.Update("gpu-0", map[string]Sample{"f": {Value: 1}})

	slot := st.getOrCreateSlot("gpu-0")
	slot.writeMu.Lock()
	defer slot.writeMu.Unlock()

	for i := 0; i < 1000; i++ {
		if snap := st.Snapshot("gpu-0"); snap == nil {
			t.Fatal("reader got nil while writer held the mutex")
		}
	}
}

func TestStoreUpdateMergesAndLists(t *testing.T) {
	st := New()
	if snap := st.Snapshot("missing"); snap != nil {
		t.Fatalf("unknown device should be nil, got %v", snap)
	}
	st.Update("a", map[string]Sample{"x": {Value: 2}})
	st.Update("b", map[string]Sample{"y": {Value: 3}})

	if got := len(st.Devices()); got != 2 {
		t.Fatalf("Devices()=%d want 2", got)
	}
	if got := len(st.Snapshots()); got != 2 {
		t.Fatalf("Snapshots()=%d want 2", got)
	}

	// a second Update merges with existing fields rather than replacing them.
	st.Update("a", map[string]Sample{"z": {Value: 4}})
	sa := st.Snapshot("a")
	if sa.Len() != 2 {
		t.Fatalf("device a has %d fields want 2", sa.Len())
	}
	if s, ok := sa.Get("x"); !ok || s.Value != 2 {
		t.Fatalf("field x not preserved across update: %+v ok=%v", s, ok)
	}
	if s, ok := sa.Get("z"); !ok || s.Value != 4 {
		t.Fatalf("field z not applied: %+v ok=%v", s, ok)
	}
}

// TestResetDropsOrphanSlots verifies that Reset clears every device slot so a
// config reload that changes the label set (hence the store device key) cannot
// leave an orphan slot keyed by the old labels. Before the fix, the old slot
// lingered and collided with the freshly-written one on render (duplicate series
// → failed collection); after Reset the store holds only the post-reload slot.
func TestResetDropsOrphanSlots(t *testing.T) {
	st := New()
	// Pre-reload: a device slot keyed by the old label set.
	oldKey := "gpu_id=0"
	st.Update(oldKey, map[string]Sample{"gpu_clock": {Value: 1, Labels: map[string]string{"gpu_id": "0"}}})
	if got := len(st.Snapshots()); got != 1 {
		t.Fatalf("pre-reset Snapshots()=%d want 1", got)
	}

	// Config reload clears the store before writers repopulate.
	st.Reset()
	if got := len(st.Snapshots()); got != 0 {
		t.Fatalf("post-reset Snapshots()=%d want 0 (orphan slot not dropped)", got)
	}
	if snap := st.Snapshot(oldKey); snap != nil {
		t.Fatalf("orphan slot %q survived Reset: %v", oldKey, snap)
	}

	// Post-reload: the new label set produces a new key; only it remains.
	newKey := "gpu_id=0\x1fkfd_process_id="
	st.Update(newKey, map[string]Sample{"gpu_clock": {Value: 2, Labels: map[string]string{"gpu_id": "0", "kfd_process_id": ""}}})
	if got := len(st.Snapshots()); got != 1 {
		t.Fatalf("post-reload Snapshots()=%d want 1", got)
	}
	if _, ok := st.Snapshots()[newKey]; !ok {
		t.Fatalf("post-reload slot %q missing", newKey)
	}
}

// TestDropFieldsKeepsSignals verifies the fetch-failure eviction: DropFields
// removes the named telemetry fields per slot, keeps the always-present signals
// (a slot's gpu_health, the node *_total slot), drops a slot left empty, and never
// touches another agent's fields (disjoint names).
func TestDropFieldsKeepsSignals(t *testing.T) {
	st := New()
	st.Update("gpu0", map[string]Sample{
		"gpu_health": {Value: 1},
		"gpu_clock":  {Value: 1500},
		"gpu_temp":   {Value: 60},
	})
	// Index-fanned telemetry-only slot (no signal field) → removed when emptied.
	st.Update("gpu0#hbm0", map[string]Sample{"gpu_hbm_temperature": {Value: 55}})
	st.Update("node", map[string]Sample{"gpu_nodes_total": {Value: 1}})
	// Another agent's slot: its fields are not in the GPU drop set → untouched.
	st.Update("nic0", map[string]Sample{"nic_total": {Value: 2}, "nic_rx": {Value: 99}})

	st.DropFields(map[string]struct{}{"gpu_clock": {}, "gpu_temp": {}, "gpu_hbm_temperature": {}})

	g := st.Snapshot("gpu0")
	if g == nil || g.Len() != 1 {
		t.Fatalf("gpu0 should keep only gpu_health, got %v", g)
	}
	if _, ok := g.Get("gpu_health"); !ok {
		t.Fatalf("gpu_health must be retained")
	}
	if _, ok := g.Get("gpu_clock"); ok {
		t.Fatalf("gpu_clock must be dropped")
	}
	if st.Snapshot("gpu0#hbm0") != nil {
		t.Fatalf("emptied telemetry-only slot must be removed")
	}
	if n := st.Snapshot("node"); n == nil || n.Len() != 1 {
		t.Fatalf("node total slot must be untouched")
	}
	if nc := st.Snapshot("nic0"); nc == nil || nc.Len() != 2 {
		t.Fatalf("other agent's slot must be untouched, got %v", nc)
	}
}

// TestUpdateIfChangedWritesOnce verifies boot/reconfiguration-only semantics for
// node-level totals: UpdateIfChanged swaps the snapshot only when a field value
// changes, so steady-state scrapes never re-store an unchanged count.
func TestUpdateIfChangedWritesOnce(t *testing.T) {
	st := New()
	key := "gpu-node:hostname"
	lbl := prometheus.Labels{"hostname": "node-a"}

	st.UpdateIfChanged(key, map[string]Sample{"gpu_nodes_total": {Value: 0, Labels: lbl}})
	p1 := st.Snapshot(key)
	if p1 == nil {
		t.Fatal("slot must exist after first UpdateIfChanged")
	}

	// Same value again (a steady-state scrape): must NOT swap the snapshot.
	st.UpdateIfChanged(key, map[string]Sample{"gpu_nodes_total": {Value: 0, Labels: lbl}})
	if st.Snapshot(key) != p1 {
		t.Fatal("UpdateIfChanged re-stored an unchanged value (should be a no-op)")
	}

	// Real change (boot/reconfig: 0 -> 8): must swap and carry the new value.
	st.UpdateIfChanged(key, map[string]Sample{"gpu_nodes_total": {Value: 8, Labels: lbl}})
	p2 := st.Snapshot(key)
	if p2 == p1 {
		t.Fatal("UpdateIfChanged did not store a changed value")
	}
	if s, ok := p2.Get("gpu_nodes_total"); !ok || s.Value != 8 {
		t.Fatalf("want gpu_nodes_total=8, got %+v (ok=%v)", s, ok)
	}

	// Same new value again: no further swap.
	st.UpdateIfChanged(key, map[string]Sample{"gpu_nodes_total": {Value: 8, Labels: lbl}})
	if st.Snapshot(key) != p2 {
		t.Fatal("UpdateIfChanged re-stored the unchanged new value")
	}
}
