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

package exporter

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
)

// storeFakeClient stands in for a GPU client in store-serving mode: it serves via
// the snapshot store (NeedsPerScrapePull false), and its UpdateMetricsStats is a
// singleflight-wrapped fetch with a counter so a test can assert concurrent
// scrapes collapse to one underlying fetch. When leaderIn is non-nil the leader
// blocks inside the fetch until release closes, forcing followers to attach.
type storeFakeClient struct {
	store      *snapshotstore.Store
	deduper    *snapshotstore.Deduper
	field      string
	fetchRuns  int32
	resetCalls int32
	leaderIn   chan struct{}
	release    chan struct{}
}

func (c *storeFakeClient) UpdateStaticMetrics(context.Context) error        { return nil }
func (c *storeFakeClient) InitConfigs() error                               { return nil }
func (c *storeFakeClient) StopServices()                                    {}
func (c *storeFakeClient) QueryMetrics() (interface{}, error)               { return nil, nil }
func (c *storeFakeClient) GetDeviceType() globals.DeviceType                { return globals.GPUDevice }
func (c *storeFakeClient) QueryInbandRASErrors(string) (interface{}, error) { return nil, nil }
func (c *storeFakeClient) NeedsPerScrapePull() bool                         { return false }

func (c *storeFakeClient) ResetMetrics() error {
	atomic.AddInt32(&c.resetCalls, 1)
	return nil
}

func (c *storeFakeClient) UpdateMetricsStats(context.Context) error {
	return c.deduper.FetchOnce("k", func() error {
		atomic.AddInt32(&c.fetchRuns, 1)
		if c.leaderIn != nil {
			c.leaderIn <- struct{}{}
			<-c.release
		}
		c.store.Update("dev", map[string]snapshotstore.Sample{
			c.field: {Value: 1, Labels: prometheus.Labels{"hostname": "h"}},
		})
		return nil
	})
}

// legacyFakeClient stands in for a reset+refill client (NIC/IFOE/GPU-legacy):
// NeedsPerScrapePull true, recording the order the middleware drives it.
type legacyFakeClient struct {
	mu    sync.Mutex
	calls []string
}

func (c *legacyFakeClient) UpdateStaticMetrics(context.Context) error        { return nil }
func (c *legacyFakeClient) InitConfigs() error                               { return nil }
func (c *legacyFakeClient) StopServices()                                    {}
func (c *legacyFakeClient) QueryMetrics() (interface{}, error)               { return nil, nil }
func (c *legacyFakeClient) GetDeviceType() globals.DeviceType                { return globals.GPUDevice }
func (c *legacyFakeClient) QueryInbandRASErrors(string) (interface{}, error) { return nil, nil }
func (c *legacyFakeClient) NeedsPerScrapePull() bool                         { return true }

func (c *legacyFakeClient) ResetMetrics() error {
	c.mu.Lock()
	c.calls = append(c.calls, "reset")
	c.mu.Unlock()
	return nil
}

func (c *legacyFakeClient) UpdateMetricsStats(context.Context) error {
	c.mu.Lock()
	c.calls = append(c.calls, "stats")
	c.mu.Unlock()
	return nil
}

// newStoreServingHandler wires the package-global mh with the given client,
// registers a store collector for field "fake_metric", and returns the
// middleware-wrapped /metrics handler.
func newStoreServingHandler(t *testing.T, client metricsutil.MetricsInterface, store *snapshotstore.Store) func() string {
	t.Helper()
	logger.Init(false)
	configPath, cleanup := setupTestConfig(t, 0)
	t.Cleanup(cleanup)

	runConf = config.NewConfigHandler(configPath, config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort})
	var err error
	mh, err = metricsutil.NewMetrics(runConf)
	assert.NilError(t, err)
	mh.RegisterMetricsClient(client)
	mh.InitConfig(context.Background())

	if store != nil {
		err = mh.GetRegistry().Register(snapshotstore.NewStoreCollector(store,
			[]snapshotstore.FieldDesc{{
				Field: "fake_metric", Name: "amd_fake_metric", Help: "fake",
				ValueType: prometheus.GaugeValue, LabelNames: []string{"hostname"},
			}}))
		assert.NilError(t, err)
	}

	reg := mh.GetRegistry()
	h := prometheusMiddleware(promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	return func() string {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", globals.MetricsHandlerPrefix, nil))
		assert.Equal(t, rr.Code, 200)
		return rr.Body.String()
	}
}

// Store-serving + non-high-res (flag-on GPU-only) scrape must serve from the
// store via singleflight and must NOT run the reset-based UpdateMetrics path.
func TestMiddlewareLockFreeServesStoreWithoutReset(t *testing.T) {
	store := snapshotstore.New()
	fake := &storeFakeClient{store: store, deduper: snapshotstore.NewDeduper(), field: "fake_metric"}
	scrape := newStoreServingHandler(t, fake, store)

	body := scrape()
	assert.Assert(t, strings.Contains(body, `amd_fake_metric{hostname="h"} 1`),
		"store metric should be served, got:\n%s", body)
	assert.Equal(t, atomic.LoadInt32(&fake.fetchRuns), int32(1), "exactly one store fetch")
	assert.Equal(t, atomic.LoadInt32(&fake.resetCalls), int32(0), "lock-free path must not reset")
}

// When a client still needs per-scrape pull, the middleware keeps today's
// behavior: take the mutex and drive reset+refill via UpdateMetrics.
func TestMiddlewareLegacyPathResetsAndRefills(t *testing.T) {
	legacy := &legacyFakeClient{}
	scrape := newStoreServingHandler(t, legacy, nil)

	_ = scrape()
	legacy.mu.Lock()
	defer legacy.mu.Unlock()
	assert.DeepEqual(t, legacy.calls, []string{"reset", "stats"})
}

// N concurrent scrapes in the lock-free path must dedupe to <=1 underlying fetch
// and must not serialize on the scrape mutex. The leader blocks inside the fetch
// until all followers attach; if the middleware wrongly held the mutex the
// followers could not reach the singleflight and the deadlines below would fire.
func TestMiddlewareConcurrentScrapesSingleFetch(t *testing.T) {
	store := snapshotstore.New()
	fake := &storeFakeClient{
		store:    store,
		deduper:  snapshotstore.NewDeduper(),
		field:    "fake_metric",
		leaderIn: make(chan struct{}, 1),
		release:  make(chan struct{}),
	}
	scrape := newStoreServingHandler(t, fake, store)

	const n = 16
	var started, finished sync.WaitGroup
	started.Add(n)
	finished.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer finished.Done()
			started.Done()
			_ = scrape()
		}()
	}

	started.Wait()
	select {
	case <-fake.leaderIn:
	case <-time.After(5 * time.Second):
		t.Fatal("no scrape reached the singleflight fetch (serialized on mutex or deadlocked)")
	}
	time.Sleep(100 * time.Millisecond) // let followers attach to the in-flight call
	close(fake.release)

	done := make(chan struct{})
	go func() { finished.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent scrapes did not complete (deadlock)")
	}

	if got := atomic.LoadInt32(&fake.fetchRuns); got != 1 {
		t.Fatalf("underlying fetch ran %d times under %d concurrent scrapes, want 1", got, n)
	}
}
