/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gpuagent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"gotest.tools/assert"

	amdgpu "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/mock_gen"
	"github.com/ROCm/device-metrics-exporter/pkg/events"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
)

// fpClient is a GPU client wired to a gomock GPUWatch client plus the config
// handler and file behind it, so a test can reload periods in place.
type fpClient struct {
	gc      *GPUAgentGPUClient
	watch   *mock_gen.MockGPUWatchSvcClient
	cfg     *config.ConfigHandler
	cfgPath string
}

func newFastPeriodClient(t *testing.T, body string) *fpClient {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	assert.NilError(t, os.WriteFile(cfgPath, []byte(body), 0644))
	cfg := config.NewConfigHandler(cfgPath, config.GPUAgentConfig{GrpcPort: globals.GPUAgentPort})
	hrmh, err := metricsutil.NewMetrics(cfg)
	assert.NilError(t, err)
	hrmh.InitConfig(context.Background())

	ga := NewAgent(hrmh, WithK8sClient(nil), WithK8sSchedulerClient(nil), WithGPUMonitoring(true))
	ga.Init()
	ga.isKubernetes = false
	var gc *GPUAgentGPUClient
	for _, client := range ga.clients {
		if client.GetDeviceType() == globals.GPUDevice {
			gc = client.(*GPUAgentGPUClient)
			break
		}
	}
	assert.Assert(t, gc != nil)
	t.Cleanup(gc.stopWorkloadSource)
	watch := mock_gen.NewMockGPUWatchSvcClient(mockCtl)
	gc.gpuclient = gpuMockCl
	gc.evtclient = eventMockCl
	gc.watchclient = watch
	assert.NilError(t, gc.InitConfigs())
	return &fpClient{gc: gc, watch: watch, cfg: cfg, cfgPath: cfgPath}
}

func (f *fpClient) reload(t *testing.T, body string) {
	t.Helper()
	assert.NilError(t, os.WriteFile(f.cfgPath, []byte(body), 0644))
	assert.NilError(t, f.cfg.RefreshConfig())
}

// existingWatch makes deleteHighResWatch see a watch object, so every delete
// it attempts reaches GPUWatchDelete and can be counted.
var existingWatch = &amdgpu.GPUWatchGetResponse{Response: []*amdgpu.GPUWatch{{}}}

func hrCfg(fields string) string {
	return `{"CommonConfig":{"HighResolution":{"Enable":true,` + fields + `}}}`
}

func createOK(_ context.Context, _ *amdgpu.GPUWatchRequest, _ ...grpc.CallOption) (*amdgpu.GPUWatchResponse, error) {
	return &amdgpu.GPUWatchResponse{ApiStatus: amdgpu.ApiStatus_API_STATUS_OK}, nil
}

func createStatus(st amdgpu.ApiStatus, got *[]*amdgpu.GPUWatchSpec) func(context.Context,
	*amdgpu.GPUWatchRequest, ...grpc.CallOption) (*amdgpu.GPUWatchResponse, error) {
	return func(_ context.Context, in *amdgpu.GPUWatchRequest, _ ...grpc.CallOption) (*amdgpu.GPUWatchResponse, error) {
		// clone: StartHighRes reuses the spec for the fallback retry
		*got = append(*got, proto.Clone(in.GetSpec()[0]).(*amdgpu.GPUWatchSpec))
		return &amdgpu.GPUWatchResponse{ApiStatus: st}, nil
	}
}

var errStopAfterCreate = errors.New("stop after create")

func TestHighResSpecCarriesFastPeriodMs(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	for _, tc := range []struct {
		name     string
		fields   string
		wantMs   uint32
		wantSlow uint32
	}{
		{"ms 10", `"FastPeriodMs":10,"SlowPeriodSecs":30`, 10, 30},
		{"secs 5 only", `"FastPeriodSecs":5,"SlowPeriodSecs":30`, 5000, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFastPeriodClient(t, hrCfg(tc.fields))
			var specs []*amdgpu.GPUWatchSpec
			f.watch.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).
				Return(&amdgpu.GPUWatchGetResponse{}, nil).AnyTimes()
			f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
				DoAndReturn(createStatus(amdgpu.ApiStatus_API_STATUS_OK, &specs)).Times(1)
			f.watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).
				Return(nil, errStopAfterCreate).Times(1)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			assert.Equal(t, f.gc.StartHighRes(ctx), errStopAfterCreate)
			assert.Equal(t, len(specs), 1)
			spec := specs[0]
			assert.Equal(t, spec.GetFastPeriodMs(), tc.wantMs)
			assert.Equal(t, spec.GetSlowPeriodSecs(), tc.wantSlow)
			assert.DeepEqual(t, spec.GetAttribute(), highResWatchAttrs)
			for _, a := range spec.GetAttribute() {
				assert.Assert(t, a != amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_FIRMWARE_TIMESTAMP &&
					a != amdgpu.GPUWatchAttrId_GPU_WATCH_ATTR_ID_SYSTEM_CLOCK_COUNTER,
					"attr %v must not be requested", a)
			}
			assert.Equal(t, f.gc.hrFastMs.Load(), tc.wantMs)
		})
	}
}

func TestHighResSubSecondFallback(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	for _, st := range []amdgpu.ApiStatus{
		amdgpu.ApiStatus_API_STATUS_OPERATION_NOT_SUPPORTED,
		amdgpu.ApiStatus_API_STATUS_INVALID_ARG,
	} {
		t.Run(st.String(), func(t *testing.T) {
			f := newFastPeriodClient(t, hrCfg(`"FastPeriodMs":10,"SlowPeriodSecs":30`))
			var specs []*amdgpu.GPUWatchSpec
			f.watch.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).
				Return(existingWatch, nil).AnyTimes()
			gomock.InOrder(
				// pre-create cleanup
				f.watch.EXPECT().GPUWatchDelete(gomock.Any(), gomock.Any()).
					Return(&amdgpu.GPUWatchDeleteResponse{}, nil),
				f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(createStatus(st, &specs)),
				// delete after the rejection, then recreate at 1000 ms
				f.watch.EXPECT().GPUWatchDelete(gomock.Any(), gomock.Any()).
					Return(&amdgpu.GPUWatchDeleteResponse{}, nil),
				f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(createStatus(amdgpu.ApiStatus_API_STATUS_OK, &specs)),
				f.watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).
					Return(nil, errStopAfterCreate),
			)

			buf := captureExporterLog(t, logrus.InfoLevel)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			assert.Equal(t, f.gc.StartHighRes(ctx), errStopAfterCreate)
			assert.Equal(t, len(specs), 2)
			assert.Equal(t, specs[0].GetFastPeriodMs(), uint32(10))
			assert.Equal(t, specs[1].GetFastPeriodMs(), uint32(1000))
			assert.Equal(t, specs[1].GetSlowPeriodSecs(), uint32(30))
			assert.DeepEqual(t, specs[1].GetGPU(), specs[0].GetGPU())
			assert.Equal(t, f.gc.hrFastMs.Load(), uint32(1000))
			assert.Assert(t, strings.Contains(buf.String(), "rejected FastPeriodMs=10"), buf.String())

			// not sticky: the next session asks for the configured period again
			specs = nil
			f.watch.EXPECT().GPUWatchDelete(gomock.Any(), gomock.Any()).
				Return(&amdgpu.GPUWatchDeleteResponse{}, nil).AnyTimes()
			f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
				DoAndReturn(createStatus(amdgpu.ApiStatus_API_STATUS_OK, &specs)).Times(1)
			f.watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).
				Return(nil, errStopAfterCreate).Times(1)
			assert.Equal(t, f.gc.StartHighRes(ctx), errStopAfterCreate)
			assert.Equal(t, len(specs), 1)
			assert.Equal(t, specs[0].GetFastPeriodMs(), uint32(10))
			assert.Equal(t, f.gc.hrFastMs.Load(), uint32(10))
		})
	}
}

func TestHighResCreateRejectedErrors(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	for _, tc := range []struct {
		name       string
		fields     string
		statuses   []amdgpu.ApiStatus // per create call, in order
		wantPeriod []uint32           // FastPeriodMs per create call
		transport  error              // returned by the single create call
	}{
		{
			// non-OK at 1000 ms: no fallback, error for reconnect/backoff
			"non-OK at 1000", `"FastPeriodMs":1000`,
			[]amdgpu.ApiStatus{amdgpu.ApiStatus_API_STATUS_INVALID_ARG},
			[]uint32{1000}, nil,
		},
		{
			"non-OK at 5000 from secs", `"FastPeriodSecs":5,"SlowPeriodSecs":30`,
			[]amdgpu.ApiStatus{amdgpu.ApiStatus_API_STATUS_OPERATION_NOT_SUPPORTED},
			[]uint32{5000}, nil,
		},
		{
			// the fallback retry itself is rejected
			"fallback rejected", `"FastPeriodMs":100`,
			[]amdgpu.ApiStatus{amdgpu.ApiStatus_API_STATUS_INVALID_ARG, amdgpu.ApiStatus_API_STATUS_INVALID_ARG},
			[]uint32{100, 1000}, nil,
		},
		{
			// a status that is not a period rejection is not retried
			"other status at 100", `"FastPeriodMs":100`,
			[]amdgpu.ApiStatus{amdgpu.ApiStatus_API_STATUS_OUT_OF_MEM},
			[]uint32{100}, nil,
		},
		{
			// a transport error is returned as is, with no retry
			"transport error at 10", `"FastPeriodMs":10`,
			[]amdgpu.ApiStatus{amdgpu.ApiStatus_API_STATUS_OK},
			[]uint32{10}, errors.New("transport down"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFastPeriodClient(t, hrCfg(tc.fields))
			var specs []*amdgpu.GPUWatchSpec
			call := 0
			f.watch.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).
				Return(&amdgpu.GPUWatchGetResponse{}, nil).AnyTimes()
			f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, in *amdgpu.GPUWatchRequest,
					_ ...grpc.CallOption) (*amdgpu.GPUWatchResponse, error) {
					specs = append(specs, proto.Clone(in.GetSpec()[0]).(*amdgpu.GPUWatchSpec))
					if tc.transport != nil {
						return nil, tc.transport
					}
					st := tc.statuses[call]
					call++
					return &amdgpu.GPUWatchResponse{ApiStatus: st}, nil
				}).Times(len(tc.statuses))
			f.watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).Times(0)

			err := f.gc.StartHighRes(context.Background())
			if tc.transport != nil {
				assert.Equal(t, err, tc.transport)
			} else {
				last := tc.statuses[len(tc.statuses)-1]
				assert.Error(t, err, "GPUWatchCreate: "+last.String())
			}
			assert.Equal(t, len(specs), len(tc.wantPeriod))
			for i, ms := range tc.wantPeriod {
				assert.Equal(t, specs[i].GetFastPeriodMs(), ms)
			}
		})
	}
}

// blockingStream yields the queued frames, then blocks until cancelled;
// drained is closed once every frame was applied.
type blockingStream struct {
	grpc.ClientStream
	ctx     context.Context
	frames  []*amdgpu.GPUWatch
	drained chan struct{}
	once    sync.Once
}

func (s *blockingStream) Recv() (*amdgpu.GPUWatch, error) {
	if len(s.frames) > 0 {
		w := s.frames[0]
		s.frames = s.frames[1:]
		return w, nil
	}
	if s.drained != nil {
		s.once.Do(func() { close(s.drained) })
	}
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

func TestHighResPeriodChangeRestartsFeed(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	f := newFastPeriodClient(t, hrCfg(`"FastPeriodMs":100`))
	created := make(chan *amdgpu.GPUWatchSpec, 16)
	f.watch.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).
		Return(&amdgpu.GPUWatchGetResponse{}, nil).AnyTimes()
	f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, in *amdgpu.GPUWatchRequest,
			opts ...grpc.CallOption) (*amdgpu.GPUWatchResponse, error) {
			created <- in.GetSpec()[0]
			return createOK(ctx, in, opts...)
		}).AnyTimes()
	f.watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ *amdgpu.GPUWatchSubscribeRequest,
			_ ...grpc.CallOption) (amdgpu.GPUWatchSvc_GPUWatchSubscribeClient, error) {
			return &blockingStream{ctx: ctx}, nil
		}).AnyTimes()

	agentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.gc.gpuHandler.ctx = agentCtx
	defer f.gc.teardownHighRes()

	expectCreate := func(wantMs, wantSlow uint32) {
		t.Helper()
		select {
		case spec := <-created:
			assert.Equal(t, spec.GetFastPeriodMs(), wantMs)
			assert.Equal(t, spec.GetSlowPeriodSecs(), wantSlow)
		case <-time.After(5 * time.Second):
			t.Fatalf("no GPUWatchCreate for %d ms / %d s", wantMs, wantSlow)
		}
	}
	expectNoCreate := func() {
		t.Helper()
		select {
		case spec := <-created:
			t.Fatalf("unexpected restart, create with %d ms / %d s",
				spec.GetFastPeriodMs(), spec.GetSlowPeriodSecs())
		case <-time.After(200 * time.Millisecond):
		}
	}
	running := func() [2]uint32 {
		f.gc.hrMu.Lock()
		defer f.gc.hrMu.Unlock()
		assert.Assert(t, f.gc.hrCancel != nil, "feed not running")
		return f.gc.hrPeriodsRunning
	}

	buf := captureExporterLog(t, logrus.InfoLevel)
	f.gc.reconcileHighRes()
	expectCreate(100, 5)
	assert.Equal(t, running(), [2]uint32{100, 5})

	// same config: reconcile is a no-op
	f.gc.reconcileHighRes()
	expectNoCreate()

	// FastPeriodMs change restarts with the new spec
	f.reload(t, hrCfg(`"FastPeriodMs":1000`))
	f.gc.reconcileHighRes()
	expectCreate(1000, 5)
	assert.Equal(t, running(), [2]uint32{1000, 5})
	assert.Assert(t, strings.Contains(buf.String(), "watch periods changed 100ms/5s -> 1000ms/5s"), buf.String())

	// same effective period expressed in seconds: no restart
	f.reload(t, hrCfg(`"FastPeriodSecs":1`))
	f.gc.reconcileHighRes()
	expectNoCreate()

	// SlowPeriodSecs change restarts
	f.reload(t, hrCfg(`"FastPeriodSecs":1,"SlowPeriodSecs":30`))
	f.gc.reconcileHighRes()
	expectCreate(1000, 30)

	// FastPeriodSecs change of the effective value restarts
	f.reload(t, hrCfg(`"FastPeriodSecs":2,"SlowPeriodSecs":30`))
	f.gc.reconcileHighRes()
	expectCreate(2000, 30)
	assert.Equal(t, running(), [2]uint32{2000, 30})

	// disabling stops the feed and does not recreate
	f.reload(t, `{"CommonConfig":{"HighResolution":{"Enable":false,"FastPeriodMs":10}}}`)
	f.gc.reconcileHighRes()
	expectNoCreate()
	f.gc.hrMu.Lock()
	stopped := f.gc.hrCancel == nil
	f.gc.hrMu.Unlock()
	assert.Assert(t, stopped)
}

func TestHighResMissedTicksLoggedOncePerMinute(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	f := newFastPeriodClient(t, hrCfg(`"FastPeriodMs":10`))
	frame := func(missed uint64) *amdgpu.GPUWatch {
		return &amdgpu.GPUWatch{Status: &amdgpu.GPUWatchStatus{MissedTicks: missed}}
	}
	buf := captureExporterLog(t, logrus.DebugLevel)
	ctx, cancel := context.WithCancel(context.Background())
	s := &blockingStream{ctx: ctx, frames: []*amdgpu.GPUWatch{
		frame(0), frame(3), frame(3), frame(7), {Spec: &amdgpu.GPUWatchSpec{}}, frame(9),
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.gc.consumeWatchStream(ctx, s)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(buf.String(), "missed ticks") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	// growth 0 -> 3 -> 7 -> 9 within one minute logs exactly once (the first)
	assert.Equal(t, strings.Count(buf.String(), "missed ticks"), 1, buf.String())
	assert.Assert(t, strings.Contains(buf.String(), "missed ticks 3 (+3)"), buf.String())
}

// captureExporterLog redirects the exporter logger at the given level for the
// rest of the test.
func captureExporterLog(t *testing.T, level logrus.Level) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prevOut, prevLevel := logger.Log.Out, logger.Log.GetLevel()
	logger.Log.SetOutput(buf)
	logger.Log.SetLevel(level)
	t.Cleanup(func() {
		logger.Log.SetOutput(prevOut)
		logger.Log.SetLevel(prevLevel)
	})
	return buf
}

// syncBuffer is a bytes.Buffer safe for the feed goroutine to write while the
// test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestHighResDiagnosticTierSignal(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	f := newFastPeriodClient(t, hrCfg(`"FastPeriodMs":10`))
	rec := &events.RecordingEmitter{}
	f.gc.hrEvents = rec
	subscribed := make(chan uint32, 16)
	f.watch.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).
		Return(&amdgpu.GPUWatchGetResponse{}, nil).AnyTimes()
	f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(createOK).AnyTimes()
	f.watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ *amdgpu.GPUWatchSubscribeRequest,
			_ ...grpc.CallOption) (amdgpu.GPUWatchSvc_GPUWatchSubscribeClient, error) {
			// the tier is signalled before subscribing, so this orders the
			// recorder writes before the test reads them
			subscribed <- f.gc.hrFastMs.Load()
			return &blockingStream{ctx: ctx}, nil
		}).AnyTimes()

	agentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.gc.gpuHandler.ctx = agentCtx
	defer f.gc.teardownHighRes()

	apply := func(body string, wantMs uint32) {
		t.Helper()
		if body != "" {
			f.reload(t, body)
		}
		f.gc.reconcileHighRes()
		if wantMs == 0 {
			return
		}
		select {
		case ms := <-subscribed:
			assert.Equal(t, ms, wantMs)
		case <-time.After(5 * time.Second):
			t.Fatalf("feed not started at %d ms", wantMs)
		}
	}
	buf := captureExporterLog(t, logrus.InfoLevel)

	apply("", 10)
	assert.Equal(t, len(rec.Events), 1)
	assert.Equal(t, rec.Events[0].Reason, events.HighResDiagnosticTier)
	assert.Assert(t, strings.Contains(rec.Events[0].Msg, "10 ms is a diagnostic tier"), rec.Events[0].Msg)
	assert.Assert(t, !strings.Contains(rec.Events[0].Msg, "experimental"), rec.Events[0].Msg)
	assert.Equal(t, strings.Count(buf.String(), "level=warning"), 1, buf.String())

	// a reload at the same period neither restarts nor signals again
	f.reload(t, hrCfg(`"FastPeriodMs":10,"FastPeriodSecs":0`))
	apply("", 0)
	assert.Equal(t, len(rec.Events), 1)

	// 1 ms is announced again, as experimental
	apply(hrCfg(`"FastPeriodMs":1`), 1)
	assert.Equal(t, len(rec.Events), 2)
	assert.Assert(t, strings.Contains(rec.Events[1].Msg, "1 ms is experimental"), rec.Events[1].Msg)

	// 100 ms ends the tier: logged, no event
	apply(hrCfg(`"FastPeriodMs":100`), 100)
	assert.Equal(t, len(rec.Events), 2)
	assert.Assert(t, strings.Contains(buf.String(), "diagnostic tier 1 ms ended (fast period now 100 ms)"), buf.String())

	// disabling ends a running diagnostic tier
	apply(hrCfg(`"FastPeriodMs":10`), 10)
	assert.Equal(t, len(rec.Events), 3)
	apply(`{"CommonConfig":{"HighResolution":{"Enable":false,"FastPeriodMs":10}}}`, 0)
	assert.Assert(t, strings.Contains(buf.String(), "diagnostic tier 10 ms ended (high-resolution disabled)"), buf.String())
	assert.Equal(t, len(rec.Events), 3)
}

func TestHighResDiagnosticTierNotSignalledOnFallback(t *testing.T) {
	tcase := setupTest(t)
	defer tcase(t)

	f := newFastPeriodClient(t, hrCfg(`"FastPeriodMs":10`))
	rec := &events.RecordingEmitter{}
	f.gc.hrEvents = rec
	var specs []*amdgpu.GPUWatchSpec
	f.watch.EXPECT().GPUWatchGet(gomock.Any(), gomock.Any()).
		Return(&amdgpu.GPUWatchGetResponse{}, nil).AnyTimes()
	gomock.InOrder(
		f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
			DoAndReturn(createStatus(amdgpu.ApiStatus_API_STATUS_OPERATION_NOT_SUPPORTED, &specs)),
		f.watch.EXPECT().GPUWatchCreate(gomock.Any(), gomock.Any()).
			DoAndReturn(createStatus(amdgpu.ApiStatus_API_STATUS_OK, &specs)),
	)
	f.watch.EXPECT().GPUWatchSubscribe(gomock.Any(), gomock.Any()).
		Return(nil, errStopAfterCreate)

	buf := captureExporterLog(t, logrus.InfoLevel)
	assert.Equal(t, f.gc.StartHighRes(context.Background()), errStopAfterCreate)
	assert.Equal(t, f.gc.hrFastMs.Load(), uint32(1000))
	assert.Equal(t, len(rec.Events), 0)
	assert.Assert(t, !strings.Contains(buf.String(), "diagnostic tier"), buf.String())
}
