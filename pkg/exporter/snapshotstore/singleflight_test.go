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
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// N concurrent callers on one key share a single fn execution and all get its
// result. The leader blocks inside fn until released, guaranteeing every
// follower attaches to the in-flight call before it completes.
func TestFetchOnceSharesInFlight(t *testing.T) {
	d := NewDeduper()
	var runs int64
	sentinel := errors.New("done")
	leaderIn := make(chan struct{}, 1)
	release := make(chan struct{})
	fn := func() error {
		atomic.AddInt64(&runs, 1)
		leaderIn <- struct{}{}
		<-release
		return sentinel
	}

	const n = 20
	var started, finished sync.WaitGroup
	started.Add(n)
	finished.Add(n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer finished.Done()
			started.Done()
			errs[i] = d.FetchOnce("k", fn)
		}()
	}

	started.Wait()                    // all goroutines are running
	<-leaderIn                        // the single leader is inside fn
	time.Sleep(50 * time.Millisecond) // let the 19 followers reach wg.Wait
	close(release)
	finished.Wait()

	if got := atomic.LoadInt64(&runs); got != 1 {
		t.Fatalf("fn ran %d times, want 1", got)
	}
	for i, e := range errs {
		if !errors.Is(e, sentinel) {
			t.Fatalf("caller %d got %v, want sentinel", i, e)
		}
	}
}

// Distinct keys run fn once per key. Each key's leader blocks until release so
// its followers attach first; exactly one run per key.
func TestFetchOnceDifferentKeys(t *testing.T) {
	d := NewDeduper()
	var runs int64
	keys := []string{"a", "b", "c"}
	const perKey = 5
	leaderIn := make(chan struct{}, len(keys))
	release := make(chan struct{})
	fn := func() error {
		atomic.AddInt64(&runs, 1)
		leaderIn <- struct{}{}
		<-release
		return nil
	}

	var started, finished sync.WaitGroup
	started.Add(len(keys) * perKey)
	finished.Add(len(keys) * perKey)
	for _, k := range keys {
		k := k
		for i := 0; i < perKey; i++ {
			go func() {
				defer finished.Done()
				started.Done()
				_ = d.FetchOnce(k, fn)
			}()
		}
	}

	started.Wait()
	for range keys {
		<-leaderIn // one leader per key entered fn
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	finished.Wait()

	if got := atomic.LoadInt64(&runs); got != int64(len(keys)) {
		t.Fatalf("fn ran %d times, want %d (one per key)", got, len(keys))
	}
}

// A call issued after the previous one fully returns re-runs fn (key cleared).
func TestFetchOnceReRunsAfterCompletion(t *testing.T) {
	d := NewDeduper()
	var runs int64
	fn := func() error { atomic.AddInt64(&runs, 1); return nil }

	_ = d.FetchOnce("k", fn)
	_ = d.FetchOnce("k", fn)

	if got := atomic.LoadInt64(&runs); got != 2 {
		t.Fatalf("sequential calls ran fn %d times, want 2", got)
	}
}
