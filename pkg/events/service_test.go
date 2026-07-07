/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package events

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
)

// Nil-client service logs only: async and sync both succeed without a backend.
func TestEventService_NilClient(t *testing.T) {
	s := newEventService(nil)
	defer s.stop()

	s.emitWarning(context.Background(), ProfilerDisabled, "async")
	if err := s.emitWarningSync(context.Background(), AgentUnreachable, "sync"); err != nil {
		t.Fatalf("emitWarningSync nil client: want nil, got %v", err)
	}
}

// A Forbidden (RBAC) error must be swallowed (returns nil) and trip the
// rbacDisabled latch so later emits short-circuit before the API call — no log
// flood, no repeated Create attempts.
func TestEventService_DisablesOnForbidden(t *testing.T) {
	var calls atomic.Int32
	s := &eventService{
		k8sClient: &k8sclient.K8sClient{}, // non-nil so the createFn path is reached
		queue:     make(chan eventEnvelope, 1),
		stopCh:    make(chan struct{}),
		createFn: func(ctx context.Context, reason, msg string) error {
			calls.Add(1)
			return apierrors.NewForbidden(schema.GroupResource{Resource: "events"}, "", nil)
		},
	}

	if err := s.emitToK8s(context.Background(), false, AgentUnreachable, "first"); err != nil {
		t.Fatalf("forbidden should be swallowed: want nil, got %v", err)
	}
	if s.rbacDisabled.Load() == 0 {
		t.Fatal("expected rbacDisabled latch to be set after a Forbidden error")
	}

	// Latch set: subsequent emits short-circuit before createFn.
	if err := s.emitToK8s(context.Background(), false, AgentUnreachable, "second"); err != nil {
		t.Fatalf("post-disable emit: want nil, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("createFn calls: want 1 (only before disable), got %d", got)
	}
}

// A full queue must not block async emits (event is dropped + logged).
func TestEventService_AsyncDropsOnFullQueue(t *testing.T) {
	s := &eventService{
		k8sClient: nil,
		queue:     make(chan eventEnvelope, 1),
		stopCh:    make(chan struct{}),
	}
	// No dispatcher running; fill the single slot, next emit must drop, not block.
	s.queue <- eventEnvelope{reason: ProfilerDisabled, msg: "fill"}
	done := make(chan struct{})
	go func() {
		s.emitWarning(context.Background(), ProfilerDisabled, "dropped")
		close(done)
	}()
	<-done // would deadlock if emitWarning blocked
}

// EmitWarningSync falls back to direct delivery when the queue is full.
func TestEventService_SyncFallbackOnFullQueue(t *testing.T) {
	s := &eventService{
		k8sClient: nil,
		queue:     make(chan eventEnvelope, 1),
		stopCh:    make(chan struct{}),
	}
	s.queue <- eventEnvelope{reason: ProfilerDisabled, msg: "fill"}
	if err := s.emitWarningSync(context.Background(), AgentUnreachable, "direct"); err != nil {
		t.Fatalf("sync fallback: want nil, got %v", err)
	}
}

// Stop drains queued events before returning.
func TestEventService_StopDrains(t *testing.T) {
	s := newEventService(nil)
	for i := 0; i < 5; i++ {
		s.emitWarning(context.Background(), ProfilerDisabled, "q")
	}
	s.stop() // must return without hanging
}

// A wedged Create must not stall delivery forever: emitToK8s bounds it with
// syncEmitTimeout so the fatal-exit path (Stop()/wg.Wait()) cannot hang.
func TestEventService_DeliverHonorsTimeout(t *testing.T) {
	orig := syncEmitTimeout
	syncEmitTimeout = 100 * time.Millisecond
	defer func() { syncEmitTimeout = orig }()

	s := &eventService{
		k8sClient: &k8sclient.K8sClient{}, // non-nil so createFn path is reached
		queue:     make(chan eventEnvelope, 1),
		stopCh:    make(chan struct{}),
		createFn: func(ctx context.Context, reason, msg string) error {
			<-ctx.Done() // simulate a wedged API server
			return ctx.Err()
		},
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.emitToK8s(context.Background(), false, AgentUnreachable, "wedged") }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("emitToK8s: want context deadline error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("emitToK8s did not return; Create was not bounded by syncEmitTimeout")
	}
}
