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

package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// newHRConfigHandler loads body as the running config of a fresh handler.
func newHRConfigHandler(t *testing.T, body string) *ConfigHandler {
	t.Helper()
	cfgPath := t.TempDir() + "/config.json"
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	ch := NewConfigHandler(cfgPath, GPUAgentConfig{})
	if err := ch.RefreshConfig(); err != nil {
		t.Fatalf("RefreshConfig: %v", err)
	}
	return ch
}

// hrBody builds a config with HighResolution enabled and the given raw fields.
func hrBody(fields string) string {
	if fields != "" {
		fields = "," + fields
	}
	return fmt.Sprintf(`{"ServerPort":5000,"CommonConfig":{"HighResolution":{"Enable":true%s}}}`, fields)
}

// captureLog redirects the exporter logger for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := logger.Log.Out
	logger.Log.SetOutput(&buf)
	t.Cleanup(func() { logger.Log.SetOutput(prev) })
	return &buf
}

func TestHighResFastPeriodMsValid(t *testing.T) {
	valid := []uint32{1, 10, 100, 1000, 2000, 5000, 59000, 60000}
	invalid := []uint32{0, 2, 5, 50, 99, 101, 500, 999, 1001, 1500, 60001, 61000, 4294967000}
	for _, ms := range valid {
		if !highResFastPeriodMsValid(ms) {
			t.Errorf("highResFastPeriodMsValid(%d) = false, want true", ms)
		}
	}
	for _, ms := range invalid {
		if highResFastPeriodMsValid(ms) {
			t.Errorf("highResFastPeriodMsValid(%d) = true, want false", ms)
		}
	}
}

func TestHighResFastPeriodMs(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantMs    uint32
		wantSlow  uint32
		wantSecs  uint32 // GetHighResPeriods fast, floored at 1 s
		wantInval bool   // an "invalid high-res periods" line is logged
	}{
		{"ms 1", hrBody(`"FastPeriodMs":1`), 1, 5, 1, false},
		{"ms 10", hrBody(`"FastPeriodMs":10`), 10, 5, 1, false},
		{"ms 100", hrBody(`"FastPeriodMs":100`), 100, 5, 1, false},
		{"ms 1000", hrBody(`"FastPeriodMs":1000`), 1000, 5, 1, false},
		{"ms 2000", hrBody(`"FastPeriodMs":2000`), 2000, 5, 2, false},
		{"ms 5000 slow 5", hrBody(`"FastPeriodMs":5000,"SlowPeriodSecs":5`), 5000, 5, 5, false},
		{"ms 60000 slow 60", hrBody(`"FastPeriodMs":60000,"SlowPeriodSecs":60`), 60000, 60, 60, false},
		{"ms 100 slow 3600", hrBody(`"FastPeriodMs":100,"SlowPeriodSecs":3600`), 100, 3600, 1, false},
		{"invalid ms 2", hrBody(`"FastPeriodMs":2`), 1000, 5, 1, true},
		{"invalid ms 50", hrBody(`"FastPeriodMs":50`), 1000, 5, 1, true},
		{"invalid ms 99", hrBody(`"FastPeriodMs":99`), 1000, 5, 1, true},
		{"invalid ms 500", hrBody(`"FastPeriodMs":500`), 1000, 5, 1, true},
		{"invalid ms 999", hrBody(`"FastPeriodMs":999`), 1000, 5, 1, true},
		{"invalid ms 1500", hrBody(`"FastPeriodMs":1500`), 1000, 5, 1, true},
		{"invalid ms 60001", hrBody(`"FastPeriodMs":60001,"SlowPeriodSecs":3600`), 1000, 5, 1, true},
		{"invalid ms 61000", hrBody(`"FastPeriodMs":61000,"SlowPeriodSecs":3600`), 1000, 5, 1, true},
		{"slow below fast", hrBody(`"FastPeriodMs":10000,"SlowPeriodSecs":5`), 1000, 5, 1, true},
		{"slow 4", hrBody(`"FastPeriodMs":100,"SlowPeriodSecs":4`), 1000, 5, 1, true},
		{"slow 3601", hrBody(`"FastPeriodMs":100,"SlowPeriodSecs":3601`), 1000, 5, 1, true},
		{"secs 61", hrBody(`"FastPeriodSecs":61,"SlowPeriodSecs":3600`), 1000, 5, 1, true},
		{"huge secs no overflow", hrBody(`"FastPeriodSecs":4294967,"SlowPeriodSecs":3600`), 1000, 5, 1, true},
		{"slow equals fast", hrBody(`"FastPeriodSecs":10,"SlowPeriodSecs":10`), 10000, 10, 10, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLog(t)
			ch := newHRConfigHandler(t, tc.body)
			ms, slow := ch.GetHighResWatchPeriods()
			if ms != tc.wantMs || slow != tc.wantSlow {
				t.Errorf("GetHighResWatchPeriods = %d ms / %d s, want %d / %d", ms, slow, tc.wantMs, tc.wantSlow)
			}
			if tc.wantInval && ms < 1000 {
				t.Errorf("invalid input yielded sub-second period %d ms", ms)
			}
			secs, slow2 := ch.GetHighResPeriods()
			if secs != tc.wantSecs || slow2 != tc.wantSlow {
				t.Errorf("GetHighResPeriods = %d s / %d s, want %d / %d", secs, slow2, tc.wantSecs, tc.wantSlow)
			}
			if got := strings.Contains(buf.String(), "invalid high-res periods"); got != tc.wantInval {
				t.Errorf("invalid log = %v, want %v; log:\n%s", got, tc.wantInval, buf.String())
			}
		})
	}
}

func TestHighResFastPeriodPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMs  uint32
		wantLog bool // "FastPeriodSecs=... ignored" logged
	}{
		{"only secs 5", hrBody(`"FastPeriodSecs":5,"SlowPeriodSecs":30`), 5000, false},
		{"only ms 100", hrBody(`"FastPeriodMs":100`), 100, false},
		{"both agree 2s/2000ms", hrBody(`"FastPeriodSecs":2,"FastPeriodMs":2000`), 2000, false},
		{"both differ 1s/100ms", hrBody(`"FastPeriodSecs":1,"FastPeriodMs":100`), 100, true},
		{"both differ invalid secs", hrBody(`"FastPeriodSecs":99,"FastPeriodMs":100`), 100, true},
		{"neither", hrBody(``), 1000, false},
		{"zeros", hrBody(`"FastPeriodSecs":0,"FastPeriodMs":0,"SlowPeriodSecs":0`), 1000, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLog(t)
			ch := newHRConfigHandler(t, tc.body)
			if ms, _ := ch.GetHighResWatchPeriods(); ms != tc.wantMs {
				t.Errorf("fastMs = %d, want %d", ms, tc.wantMs)
			}
			if got := strings.Count(buf.String(), "ignored"); (got == 1) != tc.wantLog || got > 1 {
				t.Errorf("precedence log count = %d, want logged=%v", got, tc.wantLog)
			}
		})
	}
}

func TestHighResWatchPeriodsDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"no common config", `{"ServerPort":5000}`},
		{"no high resolution", `{"ServerPort":5000,"CommonConfig":{"MetricsFieldPrefix":"amd_"}}`},
		{"disabled ignores periods", `{"ServerPort":5000,"CommonConfig":{"HighResolution":{"Enable":false,"FastPeriodMs":10,"FastPeriodSecs":7,"SlowPeriodSecs":30}}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := newHRConfigHandler(t, tc.body)
			if ms, slow := ch.GetHighResWatchPeriods(); ms != 1000 || slow != 5 {
				t.Errorf("GetHighResWatchPeriods = %d/%d, want 1000/5", ms, slow)
			}
			if fast, slow := ch.GetHighResPeriods(); fast != 1 || slow != 5 {
				t.Errorf("GetHighResPeriods = %d/%d, want 1/5", fast, slow)
			}
		})
	}
	// handler whose running config was never loaded
	ch := NewConfigHandler(t.TempDir()+"/missing.json", GPUAgentConfig{})
	if ms, slow := ch.GetHighResWatchPeriods(); ms != 1000 || slow != 5 {
		t.Errorf("unloaded config: %d/%d, want 1000/5", ms, slow)
	}
}

func TestHighResPeriodLogDedup(t *testing.T) {
	buf := captureLog(t)
	ch := newHRConfigHandler(t, hrBody(`"FastPeriodMs":50`))
	for i := 0; i < 5; i++ {
		ch.GetHighResWatchPeriods()
		ch.GetHighResPeriods()
	}
	if n := strings.Count(buf.String(), "invalid high-res periods"); n != 1 {
		t.Fatalf("invalid log count after repeated reads = %d, want 1", n)
	}

	// a changed tuple logs again, once
	writeAndRefresh(t, ch, hrBody(`"FastPeriodMs":50,"SlowPeriodSecs":30`))
	for i := 0; i < 3; i++ {
		ch.GetHighResWatchPeriods()
	}
	if n := strings.Count(buf.String(), "invalid high-res periods"); n != 2 {
		t.Fatalf("invalid log count after tuple change = %d, want 2", n)
	}

	// the "both set and different" line is deduplicated the same way
	writeAndRefresh(t, ch, hrBody(`"FastPeriodSecs":1,"FastPeriodMs":100`))
	for i := 0; i < 3; i++ {
		ch.GetHighResWatchPeriods()
		ch.GetHighResPeriods()
	}
	if n := strings.Count(buf.String(), "FastPeriodSecs=1 ignored"); n != 1 {
		t.Fatalf("precedence log count = %d, want 1", n)
	}
}

// writeAndRefresh replaces ch's config file content and reloads it.
func writeAndRefresh(t *testing.T, ch *ConfigHandler, body string) {
	t.Helper()
	if err := os.WriteFile(ch.GetMetricsConfigPath(), []byte(body), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := ch.RefreshConfig(); err != nil {
		t.Fatalf("RefreshConfig: %v", err)
	}
}

func TestHighResFastPeriodSecsOnlyUnchanged(t *testing.T) {
	// a ConfigMap written before FastPeriodMs existed keeps its seconds value
	for _, tc := range []struct {
		body           string
		wantMs, wantSl uint32
	}{
		{hrBody(`"FastPeriodSecs":1,"SlowPeriodSecs":5`), 1000, 5},
		{hrBody(`"FastPeriodSecs":5,"SlowPeriodSecs":30`), 5000, 30},
		{hrBody(`"FastPeriodSecs":60,"SlowPeriodSecs":3600`), 60000, 3600},
	} {
		ch := newHRConfigHandler(t, tc.body)
		if ms, slow := ch.GetHighResWatchPeriods(); ms != tc.wantMs || slow != tc.wantSl {
			t.Errorf("%s: %d/%d, want %d/%d", tc.body, ms, slow, tc.wantMs, tc.wantSl)
		}
		if fast, _ := ch.GetHighResPeriods(); fast != tc.wantMs/1000 {
			t.Errorf("%s: GetHighResPeriods fast = %d, want %d", tc.body, fast, tc.wantMs/1000)
		}
	}
}
