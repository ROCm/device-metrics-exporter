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
	"errors"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// node/pod stubs whose List/Watch always fail with the configured error.
type watchStubNodes struct {
	corev1.NodeInterface
	err error
}

func (s *watchStubNodes) List(context.Context, metav1.ListOptions) (*v1.NodeList, error) {
	return nil, s.err
}
func (s *watchStubNodes) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, s.err
}

type watchStubPods struct {
	corev1.PodInterface
	err error
}

func (s *watchStubPods) List(context.Context, metav1.ListOptions) (*v1.PodList, error) {
	return nil, s.err
}
func (s *watchStubPods) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, s.err
}

type watchStubCoreV1 struct {
	corev1.CoreV1Interface
	nodes corev1.NodeInterface
	pods  corev1.PodInterface
}

func (s *watchStubCoreV1) Nodes() corev1.NodeInterface     { return s.nodes }
func (s *watchStubCoreV1) Pods(string) corev1.PodInterface { return s.pods }

type watchStubClientset struct {
	kubernetes.Interface
	core corev1.CoreV1Interface
}

func (s *watchStubClientset) CoreV1() corev1.CoreV1Interface { return s.core }

func newForbiddenClient() *K8sClient {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", nil)
	return &K8sClient{
		ctx:      context.Background(),
		nodeName: "node1",
		stopCh:   make(chan struct{}),
		clientset: &watchStubClientset{core: &watchStubCoreV1{
			nodes: &watchStubNodes{err: forbidden},
			pods:  &watchStubPods{err: forbidden},
		}},
	}
}

// A Forbidden list/watch on nodes must surface as errWatchForbidden so the
// reconnect loop disables the node watcher once instead of retrying forever.
func TestStartNodeWatcher_DisablesOnForbidden(t *testing.T) {
	k := newForbiddenClient()

	done := make(chan error, 1)
	go func() { done <- k.startNodeWatcher(make(chan struct{})) }()

	select {
	case err := <-done:
		if !errors.Is(err, errWatchForbidden) {
			t.Fatalf("startNodeWatcher: want errWatchForbidden, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("startNodeWatcher did not detect RBAC Forbidden on list/watch")
	}
}

// A Forbidden list/watch on pods must surface as errWatchForbidden so the
// reconnect loop disables the pod watcher once instead of retrying forever.
func TestStartPodWatcher_DisablesOnForbidden(t *testing.T) {
	k := newForbiddenClient()

	done := make(chan error, 1)
	go func() { done <- k.startPodWatcher(make(chan struct{})) }()

	select {
	case err := <-done:
		if !errors.Is(err, errWatchForbidden) {
			t.Fatalf("startPodWatcher: want errWatchForbidden, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("startPodWatcher did not detect RBAC Forbidden on list/watch")
	}
}

// SetNodeWatcherEnabled/SetPodWatcherEnabled must be independent and
// idempotent: repeated calls with the same value are no-ops, and disabling
// one watcher must not affect the other. The returned bool reflects whether
// a transition actually happened; callers gate K8s event emission on it.
func TestWatcherEnabled_IndependentAndIdempotent(t *testing.T) {
	k := newForbiddenClient()

	if changed := k.SetNodeWatcherEnabled(true); !changed {
		t.Fatal("expected SetNodeWatcherEnabled(true) to report a transition")
	}
	if changed := k.SetNodeWatcherEnabled(true); changed { // no-op, must not panic on double-start
		t.Fatal("expected SetNodeWatcherEnabled(true) to be a no-op the second time")
	}
	k.Lock()
	if !k.nodeWatcherRunning {
		t.Fatal("expected node watcher running")
	}
	if k.podWatcherRunning {
		t.Fatal("pod watcher should not have started")
	}
	k.Unlock()

	if changed := k.SetPodWatcherEnabled(true); !changed {
		t.Fatal("expected SetPodWatcherEnabled(true) to report a transition")
	}
	k.Lock()
	if !k.podWatcherRunning {
		t.Fatal("expected pod watcher running")
	}
	k.Unlock()

	if changed := k.SetNodeWatcherEnabled(false); !changed {
		t.Fatal("expected SetNodeWatcherEnabled(false) to report a transition")
	}
	if changed := k.SetNodeWatcherEnabled(false); changed { // no-op, must not double-close channel
		t.Fatal("expected SetNodeWatcherEnabled(false) to be a no-op the second time")
	}
	k.Lock()
	if k.nodeWatcherRunning {
		t.Fatal("expected node watcher stopped")
	}
	if !k.podWatcherRunning {
		t.Fatal("pod watcher should still be running")
	}
	k.Unlock()

	if changed := k.SetPodWatcherEnabled(false); !changed {
		t.Fatal("expected SetPodWatcherEnabled(false) to report a transition")
	}
	k.Lock()
	if k.podWatcherRunning {
		t.Fatal("expected pod watcher stopped")
	}
	k.Unlock()
}

// After RBAC forbids the watch, runWatcherWithReconnect must clear
// nodeWatcherRunning/podWatcherRunning so a later Set*WatcherEnabled(true)
// (e.g. once RBAC is fixed) can restart the watcher instead of silently
// no-op'ing forever because the flag was left true.
func TestWatcherEnabled_RecoversAfterForbidden(t *testing.T) {
	k := newForbiddenClient()

	if changed := k.SetNodeWatcherEnabled(true); !changed {
		t.Fatal("expected SetNodeWatcherEnabled(true) to report a transition")
	}
	if changed := k.SetPodWatcherEnabled(true); !changed {
		t.Fatal("expected SetPodWatcherEnabled(true) to report a transition")
	}

	waitForCondition(t, func() bool {
		k.Lock()
		defer k.Unlock()
		return !k.nodeWatcherRunning && !k.podWatcherRunning
	}, "expected node/pod watcherRunning to clear after RBAC Forbidden")

	// Now that RBAC is "fixed" (state cleared), the watcher must be
	// startable again rather than treated as still running.
	if changed := k.SetNodeWatcherEnabled(true); !changed {
		t.Fatal("expected SetNodeWatcherEnabled(true) to report a transition after recovery")
	}
	if changed := k.SetPodWatcherEnabled(true); !changed {
		t.Fatal("expected SetPodWatcherEnabled(true) to report a transition after recovery")
	}
}

func waitForCondition(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatal(msg)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
