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

// A Forbidden list/watch on nodes/pods must surface as errWatchForbidden so the
// reconnect loop disables the watchers once instead of retrying forever.
func TestStartWatchers_DisablesOnForbidden(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", nil)
	k := &K8sClient{
		ctx:      context.Background(),
		nodeName: "node1",
		stopCh:   make(chan struct{}),
		clientset: &watchStubClientset{core: &watchStubCoreV1{
			nodes: &watchStubNodes{err: forbidden},
			pods:  &watchStubPods{err: forbidden},
		}},
	}

	done := make(chan error, 1)
	go func() { done <- k.startWatchers() }()

	select {
	case err := <-done:
		if !errors.Is(err, errWatchForbidden) {
			t.Fatalf("startWatchers: want errWatchForbidden, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("startWatchers did not detect RBAC Forbidden on list/watch")
	}
}
