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

// Package events is a singleton K8s Warning event emitter, used like logger.Log.
package events

import (
	"context"
	"os"
	"sync"

	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

var (
	mu        sync.RWMutex
	singleton *eventService
)

// Init starts the singleton; a nil client yields a log-only service. cleanup runs
// before the process exits on a Fatal; a nil exitFn defaults to os.Exit.
func Init(k8sClient *k8sclient.K8sClient, cleanup func(), exitFn func(int)) {
	mu.Lock()
	defer mu.Unlock()
	if singleton != nil {
		singleton.stop()
	}
	if exitFn == nil {
		exitFn = os.Exit
	}
	singleton = newEventService(k8sClient)
	singleton.cleanup = cleanup
	singleton.exitFn = exitFn
}

// Stop drains the queue and shuts down the dispatcher.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if singleton == nil {
		return
	}
	singleton.stop()
	singleton = nil
}

// EmitWarning queues a Warning event asynchronously; logs only before Init/after Stop.
func EmitWarning(ctx context.Context, reason EventReason, msg string) {
	mu.RLock()
	s := singleton
	mu.RUnlock()
	if s == nil {
		logger.Log.Printf("event %q (no service): %s", reason, msg)
		return
	}
	s.emitWarning(ctx, reason, msg)
}

// EmitWarningSync emits synchronously (5s timeout) for fatal paths; logs only before Init/after Stop.
func EmitWarningSync(ctx context.Context, reason EventReason, msg string) error {
	mu.RLock()
	s := singleton
	mu.RUnlock()
	if s == nil {
		logger.Log.Printf("event %q (no service): %s", reason, msg)
		return nil
	}
	return s.emitWarningSync(ctx, reason, msg)
}

// Fatal emits a final Warning event synchronously, runs the registered
// cleanup, then exits the process. Idempotent. Before Init it logs and exits.
func Fatal(reason EventReason, msg string) {
	mu.RLock()
	s := singleton
	mu.RUnlock()
	if s == nil {
		logger.Log.Printf("FATAL %q (no service): %s", reason, msg)
		os.Exit(1)
		return
	}
	s.fatal(reason, msg)
}
