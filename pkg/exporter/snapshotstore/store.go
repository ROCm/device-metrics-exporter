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
	"sync"
	"sync/atomic"
)

// deviceSlot holds one device's current snapshot plus a writer-only mutex.
// Readers load ptr lock-free; writers serialize on writeMu without blocking
// readers.
type deviceSlot struct {
	writeMu sync.Mutex
	ptr     atomic.Pointer[DeviceSnapshot]
}

// Store is a per-device copy-on-write snapshot store. The device map itself is
// swapped atomically on growth, so lookups stay lock-free and race-free while a
// new device key is being added.
type Store struct {
	growMu  sync.Mutex
	devices atomic.Pointer[map[string]*deviceSlot]
}

func New() *Store {
	s := &Store{}
	m := map[string]*deviceSlot{}
	s.devices.Store(&m)
	return s
}

// StoreWriter is the write side of the store — the seam used by the per-device
// metric writers (GPU/NIC/IFOE) and the high-res feed, so those layers depend on
// the narrow write contract rather than the concrete *Store.
type StoreWriter interface {
	Update(uuid string, deltas map[string]Sample)
	UpdateIfChanged(uuid string, deltas map[string]Sample)
	// DropFields evicts the named telemetry fields on a fetch failure so stale
	// values are not served; the caller passes only its own fields.
	DropFields(drop map[string]struct{})
}

// StoreReader is the read side — the seam used by the StoreCollector that renders
// /metrics from the latest snapshots.
type StoreReader interface {
	Snapshot(uuid string) *DeviceSnapshot
	Snapshots() map[string]*DeviceSnapshot
	Devices() []string
}

// *Store satisfies both seams.
var (
	_ StoreWriter = (*Store)(nil)
	_ StoreReader = (*Store)(nil)
)

// Snapshot returns the current immutable snapshot for uuid, or nil if none is
// published. Lock-free: never blocks on a writer, never allocates.
func (s *Store) Snapshot(uuid string) *DeviceSnapshot {
	slot, ok := (*s.devices.Load())[uuid]
	if !ok {
		return nil
	}
	return slot.ptr.Load()
}

func (s *Store) getOrCreateSlot(uuid string) *deviceSlot {
	if slot, ok := (*s.devices.Load())[uuid]; ok {
		return slot
	}
	s.growMu.Lock()
	defer s.growMu.Unlock()
	old := *s.devices.Load()
	if slot, ok := old[uuid]; ok {
		return slot
	}
	// clone-and-swap the device map so concurrent readers never see a map
	// mutated in place.
	nm := make(map[string]*deviceSlot, len(old)+1)
	for k, v := range old {
		nm[k] = v
	}
	slot := &deviceSlot{}
	nm[uuid] = slot
	s.devices.Store(&nm)
	return slot
}

// acquireSlot returns a slot for uuid with slot.writeMu held, guaranteeing the
// slot is still the one the current device map references. A concurrent
// Reset/DropFields clone-and-swaps the device map; if that happens after
// getOrCreateSlot returns, our slot is detached and a Store into it is a lost
// update (the new map points at a different slot). Re-check membership under the
// write lock and retry from getOrCreateSlot on mismatch. Caller unlocks writeMu.
func (s *Store) acquireSlot(uuid string) *deviceSlot {
	for {
		slot := s.getOrCreateSlot(uuid)
		slot.writeMu.Lock()
		if cur, ok := (*s.devices.Load())[uuid]; ok && cur == slot {
			return slot
		}
		slot.writeMu.Unlock()
	}
}

// Update builds a new snapshot = current + deltas and atomically swaps it in.
// Writers serialize per device on the slot mutex; readers are never blocked and
// only ever observe a complete snapshot.
func (s *Store) Update(uuid string, deltas map[string]Sample) {
	slot := s.acquireSlot(uuid)
	defer slot.writeMu.Unlock()

	var base map[string]Sample
	if cur := slot.ptr.Load(); cur != nil {
		base = cur.fields
	}
	nf := make(map[string]Sample, len(base)+len(deltas))
	for k, v := range base {
		nf[k] = v
	}
	for k, v := range deltas {
		nf[k] = v
	}
	slot.ptr.Store(newDeviceSnapshot(nf))
}

// UpdateIfChanged merges deltas like Update but skips any field whose Value already
// equals the current snapshot's, swapping only when something actually changed. Use
// it for boot/reconfiguration-time constants (the node-level *_total device counts)
// so they are stored once and never re-written on every scrape or stream tick. The
// compare+swap runs under the slot mutex, so concurrent writers stay consistent.
func (s *Store) UpdateIfChanged(uuid string, deltas map[string]Sample) {
	slot := s.acquireSlot(uuid)
	defer slot.writeMu.Unlock()

	var base map[string]Sample
	if cur := slot.ptr.Load(); cur != nil {
		base = cur.fields
	}
	changed := false
	nf := make(map[string]Sample, len(base)+len(deltas))
	for k, v := range base {
		nf[k] = v
	}
	for k, v := range deltas {
		if old, ok := nf[k]; ok && old.Value == v.Value {
			continue
		}
		nf[k] = v
		changed = true
	}
	if changed {
		slot.ptr.Store(newDeviceSnapshot(nf))
	}
}

// Reset drops every device slot, swapping in a fresh empty device map. Readers
// see either the old or the new map atomically (never a torn one). Called on
// config reload so a changed label set does not leave orphan slots keyed by the
// old labels: such orphans render against the new descriptor with the removed
// label defaulted to "", colliding with the freshly-written slot and producing
// duplicate series that fail promhttp collection. Writers repopulate on the next
// scrape / feed tick.
func (s *Store) Reset() {
	s.growMu.Lock()
	defer s.growMu.Unlock()
	m := map[string]*deviceSlot{}
	s.devices.Store(&m)
}

// DropFields removes the named fields from every device slot and drops any slot
// left with no fields. Used on a fetch failure (GPUGet / GPUWatch stream error)
// to evict stale device telemetry so /metrics serves only the always-present
// signals (the node *_total slot and gpu_health, which are not in drop), instead
// of last-known clocks/temps. Scoped by field name: a writer passes only its own
// telemetry fields, so a GPU failure never touches NIC/IFOE slots (their field
// names are disjoint). Writers repopulate on the next successful scrape / feed
// tick (Update merges the full set back under the same key). No-op if drop empty.
func (s *Store) DropFields(drop map[string]struct{}) {
	if len(drop) == 0 {
		return
	}
	s.growMu.Lock()
	defer s.growMu.Unlock()
	old := *s.devices.Load()
	nm := make(map[string]*deviceSlot, len(old))
	for key, slot := range old {
		slot.writeMu.Lock()
		cur := slot.ptr.Load()
		if cur == nil {
			nm[key] = slot
			slot.writeMu.Unlock()
			continue
		}
		nf := make(map[string]Sample, len(cur.fields))
		for f, v := range cur.fields {
			if _, d := drop[f]; d {
				continue
			}
			nf[f] = v
		}
		switch {
		case len(nf) == len(cur.fields):
			// Nothing dropped from this slot (e.g. a NIC/IFOE slot, or the node
			// slot): keep it untouched.
			nm[key] = slot
		case len(nf) == 0:
			// Slot had only dropped fields (e.g. an index-fanned telemetry row):
			// omit it from the new map so it stops rendering.
			// (slot intentionally not carried over)
		default:
			slot.ptr.Store(newDeviceSnapshot(nf))
			nm[key] = slot
		}
		slot.writeMu.Unlock()
	}
	s.devices.Store(&nm)
}

// Devices returns the known device UUIDs (unordered).
func (s *Store) Devices() []string {
	m := *s.devices.Load()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Snapshots returns the current snapshot for every device that has one, for a
// full scrape.
func (s *Store) Snapshots() map[string]*DeviceSnapshot {
	m := *s.devices.Load()
	out := make(map[string]*DeviceSnapshot, len(m))
	for k, slot := range m {
		if snap := slot.ptr.Load(); snap != nil {
			out[k] = snap
		}
	}
	return out
}
