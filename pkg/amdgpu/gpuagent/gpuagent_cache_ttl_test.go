/*
*
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
*
*/
package gpuagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

func TestParseGPUGetCacheTTL(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
		bad   bool
	}{
		{"", DefaultGPUGetCacheTTL, false},
		{"15s", 15 * time.Second, false},
		{"500ms", 500 * time.Millisecond, false},
		{" 0s ", 0, false},
		{"0", 0, false},
		{"-1s", 0, true},
		{"1", 0, true},
		{"abc", 0, true},
		{"999999999999999999999s", 0, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, err := ParseGPUGetCacheTTL(tc.value)
			if (err != nil) != tc.bad || got != tc.want {
				t.Fatalf("%q: got %v, %v; want %v, error=%v", tc.value, got, err, tc.want, tc.bad)
			}
		})
	}
}

func newCacheTTLClient(t *testing.T, ttl time.Duration) (*GPUAgentGPUClient, *mock_gen.MockGPUSvcClient) {
	t.Helper()
	logger.Init(true)
	ga, err := NewGPUAgentGPUClient(&GPUAgentClient{ctx: context.Background(), gpuGetCacheTTL: ttl})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ga.Close)
	client := mock_gen.NewMockGPUSvcClient(gomock.NewController(t))
	ga.gpuclient = client
	return ga, client
}

func TestCacheReadHonorsGPUGetCacheTTL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		age     time.Duration
		refresh bool
	}{
		{"default keeps a 2s-old response", DefaultGPUGetCacheTTL, 2 * time.Second, false},
		{"1s TTL refreshes a 2s-old response", time.Second, 2 * time.Second, true},
		{"0 refreshes even a fresh response", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ga, client := newCacheTTLClient(t, tc.ttl)
			cached, fresh := &amdgpu.GPUGetResponse{}, &amdgpu.GPUGetResponse{}
			slot := &ga.gCache.metricsCache
			slot.resp, slot.ts = cached, time.Now().Add(-tc.age)
			filter := &amdgpu.GPUGetFilter{SkipProcessStatus: true}
			want := cached
			if tc.refresh {
				client.EXPECT().GPUGet(gomock.Any(), &amdgpu.GPUGetRequest{Filter: filter}).Return(fresh, nil).Times(1)
				want = fresh
			}
			got, err := ga.cacheRead(filter, slot)
			if err != nil || got != want || slot.resp != want {
				t.Fatalf("cacheRead = %p, %v; want %p", got, err, want)
			}
		})
	}
}

func TestCacheReadErrorClearsSlot(t *testing.T) {
	ga, client := newCacheTTLClient(t, time.Second)
	slot := &ga.gCache.metricsCache
	slot.resp, slot.ts = &amdgpu.GPUGetResponse{}, time.Now().Add(-2*time.Second)
	readErr := errors.New("GPUGet failed")
	fresh := &amdgpu.GPUGetResponse{}
	gomock.InOrder(
		client.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(nil, readErr),
		client.EXPECT().GPUGet(gomock.Any(), gomock.Any()).Return(fresh, nil),
	)
	if got, err := ga.cacheRead(nil, slot); !errors.Is(err, readErr) || got != nil || slot.resp != nil {
		t.Fatalf("failed read retained a response: %v, %v", got, err)
	}
	if got, err := ga.cacheRead(nil, slot); err != nil || got != fresh {
		t.Fatalf("retry = %v, %v", got, err)
	}
}

func TestCacheReadConcurrentTTL(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ttl   time.Duration
		calls int
	}{
		{"default reuses one read", DefaultGPUGetCacheTTL, 1},
		{"0 reads once per caller", 0, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ga, client := newCacheTTLClient(t, tc.ttl)
			fresh := &amdgpu.GPUGetResponse{}
			client.EXPECT().GPUGet(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *amdgpu.GPUGetRequest, ...grpc.CallOption) (*amdgpu.GPUGetResponse, error) {
				time.Sleep(10 * time.Millisecond)
				return fresh, nil
			}).Times(tc.calls)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					if got, err := ga.cacheRead(nil, &ga.gCache.metricsCache); err != nil || got != fresh {
						t.Errorf("concurrent read = %v, %v", got, err)
					}
				}()
			}
			close(start)
			wg.Wait()
		})
	}
}

func TestNewAgentGPUGetCacheTTL(t *testing.T) {
	logger.Init(true)
	handler, err := metricsutil.NewMetrics(config.NewConfigHandler("config.json", config.GPUAgentConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts []GPUAgentClientOptions
		want time.Duration
	}{
		{"no option keeps the upstream default", nil, DefaultGPUGetCacheTTL},
		{"option disables reuse", []GPUAgentClientOptions{WithGPUGetCacheTTL(0)}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ga := NewAgent(handler, tc.opts...)
			if ga == nil || len(ga.clients) == 0 {
				t.Fatal("NewAgent returned no GPU client")
			}
			gpu, ok := ga.clients[0].(*GPUAgentGPUClient)
			if !ok {
				t.Fatalf("first client is %T, want *GPUAgentGPUClient", ga.clients[0])
			}
			defer gpu.Close()
			if gpu.gpuGetCacheTTL != tc.want {
				t.Fatalf("GPU client TTL = %v, want %v", gpu.gpuGetCacheTTL, tc.want)
			}
		})
	}
}
