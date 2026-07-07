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

package k8sclient

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func TestMain(m *testing.M) {
	logger.Init(true)
	os.Exit(m.Run())
}

// stubEvents records Create calls; embeds the interface so unused methods
// satisfy the type without implementation.
type stubEvents struct {
	corev1.EventInterface
	mu      sync.Mutex
	created []*v1.Event
	delay   time.Duration
	err     error
}

func (s *stubEvents) Create(ctx context.Context, e *v1.Event, _ metav1.CreateOptions) (*v1.Event, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	s.mu.Lock()
	s.created = append(s.created, e)
	s.mu.Unlock()
	return e, nil
}

func (s *stubEvents) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.created)
}

type stubCoreV1 struct {
	corev1.CoreV1Interface
	events *stubEvents
}

func (s *stubCoreV1) Events(string) corev1.EventInterface { return s.events }

type stubClientset struct {
	kubernetes.Interface
	core *stubCoreV1
}

func (s *stubClientset) CoreV1() corev1.CoreV1Interface { return s.core }

func newTestK8sClient(ev *stubEvents) *K8sClient {
	return &K8sClient{
		ctx:          context.Background(),
		clientset:    &stubClientset{core: &stubCoreV1{events: ev}},
		nodeName:     "node1",
		podName:      "pod1",
		podNamespace: "ns1",
	}
}

const testEventReason = "HealthValidationFailed"

func TestEmitWarningEventDirect_EmitsWarning(t *testing.T) {
	ev := &stubEvents{}
	k := newTestK8sClient(ev)
	if err := k.EmitWarningEventDirect(context.Background(), testEventReason, "agent down"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ev.count() != 1 {
		t.Fatalf("expected 1 event, got %d", ev.count())
	}
	e := ev.created[0]
	if e.Type != v1.EventTypeWarning || e.Reason != testEventReason {
		t.Fatalf("unexpected event type=%q reason=%q", e.Type, e.Reason)
	}
	if e.InvolvedObject.Name != "pod1" || e.InvolvedObject.Namespace != "ns1" {
		t.Fatalf("unexpected involvedObject %s/%s", e.InvolvedObject.Namespace, e.InvolvedObject.Name)
	}
}

func TestEmitWarningEventDirect_NoPodMetadata(t *testing.T) {
	ev := &stubEvents{}
	k := newTestK8sClient(ev)
	k.podName = ""
	if err := k.EmitWarningEventDirect(context.Background(), testEventReason, "agent down"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.count() != 0 {
		t.Fatalf("expected no event without pod metadata, got %d", ev.count())
	}
}

func TestEmitWarningEventDirect_DisablesOnForbidden(t *testing.T) {
	ev := &stubEvents{err: apierrors.NewForbidden(
		schema.GroupResource{Resource: "events"}, "", nil)}
	k := newTestK8sClient(ev)

	// First emit hits the Forbidden error, returns it, and trips the disable flag.
	if err := k.EmitWarningEventDirect(context.Background(), testEventReason, "agent down"); err == nil {
		t.Fatal("expected Forbidden error to be returned")
	}
	k.Lock()
	forbidden := k.eventsForbidden
	k.Unlock()
	if !forbidden {
		t.Fatal("expected eventsForbidden to be set after a Forbidden error")
	}

	// Subsequent emits are skipped before reaching the API server. Clear the
	// stub error to prove the short-circuit, not the stub, suppresses it.
	ev.err = nil
	if err := k.EmitWarningEventDirect(context.Background(), testEventReason, "agent down again"); err != nil {
		t.Fatalf("unexpected error after disable: %v", err)
	}
	if ev.count() != 0 {
		t.Fatalf("expected no events after disable, got %d", ev.count())
	}
}

const testInfoEventReason = "K8sWatcherStateChanged"

func TestEmitInfoEventDirect_EmitsNormal(t *testing.T) {
	ev := &stubEvents{}
	k := newTestK8sClient(ev)
	if err := k.EmitInfoEventDirect(context.Background(), testInfoEventReason, "node watcher started"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ev.count() != 1 {
		t.Fatalf("expected 1 event, got %d", ev.count())
	}
	e := ev.created[0]
	if e.Type != v1.EventTypeNormal || e.Reason != testInfoEventReason {
		t.Fatalf("unexpected event type=%q reason=%q", e.Type, e.Reason)
	}
}

func TestEmitWarningEventDirect_CanceledContext(t *testing.T) {
	ev := &stubEvents{delay: time.Second}
	k := newTestK8sClient(ev)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before emit
	if err := k.EmitWarningEventDirect(ctx, testEventReason, "agent down"); err == nil {
		t.Fatal("expected error with a canceled context")
	}
	if ev.count() != 0 {
		t.Fatalf("expected no event with a canceled context, got %d", ev.count())
	}
}

func TestEmitWarningEventDirect_RespectsContextDeadline(t *testing.T) {
	ev := &stubEvents{delay: 10 * time.Second}
	k := newTestK8sClient(ev)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = k.EmitWarningEventDirect(ctx, testEventReason, "agent down")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("EmitWarningEventDirect did not honor the context deadline")
	}
}
