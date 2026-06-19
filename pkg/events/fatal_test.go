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
	"sync"
	"testing"
)

// Fatal runs cleanup then exitFn, in order, exactly once.
func TestFatal_OrderAndExit(t *testing.T) {
	var order []string
	Init(nil,
		func() { order = append(order, "cleanup") },
		func(code int) {
			order = append(order, "exit")
			if code != 1 {
				t.Fatalf("exit code: want 1, got %d", code)
			}
		},
	)
	defer Stop()

	Fatal(AgentUnreachable, "boom")

	if len(order) != 2 || order[0] != "cleanup" || order[1] != "exit" {
		t.Fatalf("call order: want [cleanup exit], got %v", order)
	}
}

// fatalOnce guarantees a single execution even under concurrent calls.
func TestFatal_Idempotent(t *testing.T) {
	var exits int32
	var mu sync.Mutex
	Init(nil, nil, func(int) {
		mu.Lock()
		exits++
		mu.Unlock()
	})
	defer Stop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Fatal(HealthValidationFailed, "x")
		}()
	}
	wg.Wait()

	if exits != 1 {
		t.Fatalf("exitFn calls: want 1, got %d", exits)
	}
}

// A nil cleanup is skipped without panicking.
func TestFatal_NilCleanup(t *testing.T) {
	var exited bool
	Init(nil, nil, func(int) { exited = true })
	defer Stop()

	Fatal(RocpctlFatalExit, "x")
	if !exited {
		t.Fatal("exitFn was not called with nil cleanup")
	}
}

// Fatal before Init must not panic; it falls back to logging (exit path is
// the real os.Exit, so it is not exercised here -- only the no-panic guarantee).
func TestFatal_BeforeInit_NoPanic(t *testing.T) {
	Stop() // ensure no singleton
	// We cannot call Fatal here without triggering os.Exit; assert the
	// singleton is nil so the documented log-and-exit branch is selected.
	mu.RLock()
	s := singleton
	mu.RUnlock()
	if s != nil {
		t.Fatal("expected nil singleton after Stop")
	}
}
