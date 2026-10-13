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

import "sync"

type call struct {
	wg  sync.WaitGroup
	err error
}

// Deduper collapses concurrent callers sharing an in-flight key into one
// execution of fn. In-package to avoid a dependency: x/sync/singleflight is not
// vendored.
type Deduper struct {
	mu    sync.Mutex
	calls map[string]*call
}

func NewDeduper() *Deduper {
	return &Deduper{calls: map[string]*call{}}
}

// FetchOnce runs fn once for an in-flight key: concurrent callers with the same
// key share that single execution and all receive its error. The key clears
// once fn returns, so the next call re-runs fn.
func (d *Deduper) FetchOnce(key string, fn func() error) error {
	d.mu.Lock()
	if c, ok := d.calls[key]; ok {
		d.mu.Unlock()
		c.wg.Wait()
		return c.err
	}
	c := &call{}
	c.wg.Add(1)
	d.calls[key] = c
	d.mu.Unlock()

	// Clear the key and wake followers via defer so a panic in fn can't strand the
	// key (WaitGroup stuck at 1) and block every later caller forever. A later
	// caller then re-runs fn; followers hold their *call reference and read c.err.
	defer func() {
		d.mu.Lock()
		delete(d.calls, key)
		d.mu.Unlock()
		c.wg.Done()
	}()

	c.err = fn()
	return c.err
}
