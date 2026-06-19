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
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

const queueSize = 64

// syncEmitTimeout bounds a single K8s event Create; a var so tests can shrink it.
var syncEmitTimeout = 5 * time.Second

// errCh is non-nil for sync emits; the dispatcher returns the delivery error on it.
type eventEnvelope struct {
	ctx    context.Context
	reason EventReason
	msg    string
	errCh  chan error
}

type eventService struct {
	k8sClient    *k8sclient.K8sClient
	createFn     func(ctx context.Context, reason, msg string) error
	queue        chan eventEnvelope
	stopCh       chan struct{}
	wg           sync.WaitGroup
	rbacDisabled atomic.Int32
	cleanup      func()
	exitFn       func(int)
	fatalOnce    sync.Once
}

func newEventService(k8sClient *k8sclient.K8sClient) *eventService {
	s := &eventService{
		k8sClient: k8sClient,
		queue:     make(chan eventEnvelope, queueSize),
		stopCh:    make(chan struct{}),
	}
	if k8sClient != nil {
		s.createFn = k8sClient.EmitWarningEventDirect
	}
	s.wg.Add(1)
	go s.dispatch()
	return s
}

// dispatch is the single queue consumer; drains remaining events on stop.
func (s *eventService) dispatch() {
	defer s.wg.Done()
	for {
		select {
		case env := <-s.queue:
			s.deliver(env)
		case <-s.stopCh:
			for {
				select {
				case env := <-s.queue:
					s.deliver(env)
				default:
					return
				}
			}
		}
	}
}

func (s *eventService) deliver(env eventEnvelope) {
	err := s.emitToK8s(env.ctx, env.reason, env.msg)
	if env.errCh != nil {
		env.errCh <- err
	}
}

// emitToK8s always logs; creates the K8s event unless off-cluster or RBAC-disabled.
func (s *eventService) emitToK8s(ctx context.Context, reason EventReason, msg string) error {
	logger.Log.Printf("event %q: %s", reason, msg)

	if s.k8sClient == nil {
		return nil
	}
	if s.rbacDisabled.Load() != 0 {
		return nil
	}

	// Bound the Create call so a wedged API server cannot stall the dispatcher
	// (and thus Stop()/wg.Wait() on the fatal-exit path) indefinitely.
	ctx, cancel := context.WithTimeout(ctx, syncEmitTimeout)
	defer cancel()

	err := s.createFn(ctx, string(reason), msg)
	if err != nil {
		if apierrors.IsForbidden(err) {
			if s.rbacDisabled.CompareAndSwap(0, 1) {
				logger.Log.Printf("event creation forbidden by RBAC (%v); disabling K8s event "+
					"emission. Grant the exporter ServiceAccount 'create' on events to enable it.", err)
			}
			return nil
		}
		logger.Log.Printf("event %q: K8s emit failed: %v", reason, err)
	}
	return err
}

// emitWarning queues asynchronously; never blocks (drops on full queue).
func (s *eventService) emitWarning(ctx context.Context, reason EventReason, msg string) {
	env := eventEnvelope{ctx: ctx, reason: reason, msg: msg}
	select {
	case s.queue <- env:
	default:
		logger.Log.Printf("event queue full; dropping async event %q: %s", reason, msg)
	}
}

// emitWarningSync waits for delivery, bounded by syncEmitTimeout; delivers directly on full queue.
func (s *eventService) emitWarningSync(ctx context.Context, reason EventReason, msg string) error {
	errCh := make(chan error, 1)
	env := eventEnvelope{ctx: ctx, reason: reason, msg: msg, errCh: errCh}

	select {
	case s.queue <- env:
	default:
		s.deliver(env)
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(syncEmitTimeout):
		return fmt.Errorf("event %q: sync emit timed out after %s", reason, syncEmitTimeout)
	}
}

// fatal emits a final Warning event synchronously, runs cleanup, then exits 1. Idempotent.
func (s *eventService) fatal(reason EventReason, msg string) {
	s.fatalOnce.Do(func() {
		logger.Log.Printf("FATAL: %s: %s", reason, msg)
		_ = s.emitWarningSync(context.Background(), reason, msg)
		if s.cleanup != nil {
			s.cleanup()
		}
		exitFn := s.exitFn
		if exitFn == nil {
			exitFn = os.Exit
		}
		exitFn(1)
	})
}

func (s *eventService) stop() {
	close(s.stopCh)
	s.wg.Wait()
}
