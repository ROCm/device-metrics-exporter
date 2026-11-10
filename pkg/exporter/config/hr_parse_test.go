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
	"encoding/json"
	"os"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

func init() {
	logger.Init(true)
}

func TestHighResParse(t *testing.T) {
	js := `{"ServerPort":5055,"CommonConfig":{"MetricsFieldPrefix":"amd_","HighResolution":{"Enable":true},"HealthService":{"Enable":false}}}`
	var c exportermetrics.MetricConfig
	if err := json.Unmarshal([]byte(js), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hr := c.GetCommonConfig().GetHighResolution()
	t.Logf("CommonConfig!=nil=%v HighResolution!=nil=%v", c.GetCommonConfig() != nil, hr != nil)
	if hr == nil {
		t.Fatalf("HighResolution is nil after parse")
	}
	t.Logf("Enable=%v", hr.GetEnable())
	if !hr.GetEnable() {
		t.Fatalf("Enable=false, expected true")
	}
}

func TestHighResExampleConfigs(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		wantEnable bool
		wantFast   uint32
		wantSlow   uint32
	}{
		{
			name:       "disabled with defaults",
			json:       `{"ServerPort":5000,"CommonConfig":{"HighResolution":{"Enable":false,"FastPeriodSecs":1,"SlowPeriodSecs":5}}}`,
			wantEnable: false,
			wantFast:   1,
			wantSlow:   5,
		},
		{
			name:       "enabled with custom periods",
			json:       `{"ServerPort":5000,"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":2,"SlowPeriodSecs":10}}}`,
			wantEnable: true,
			wantFast:   2,
			wantSlow:   10,
		},
		{
			name:       "omitted entirely",
			json:       `{"ServerPort":5000,"CommonConfig":{"MetricsFieldPrefix":"amd_"}}`,
			wantEnable: false,
			wantFast:   1,
			wantSlow:   5,
		},
		{
			name:       "invalid periods fallback to defaults",
			json:       `{"ServerPort":5000,"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":999,"SlowPeriodSecs":1}}}`,
			wantEnable: true,
			wantFast:   1,
			wantSlow:   5,
		},
		{
			name:       "slow less than fast fallback",
			json:       `{"ServerPort":5000,"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":10,"SlowPeriodSecs":5}}}`,
			wantEnable: true,
			wantFast:   1,
			wantSlow:   5,
		},
		{
			name:       "zero periods use defaults",
			json:       `{"ServerPort":5000,"CommonConfig":{"HighResolution":{"Enable":true,"FastPeriodSecs":0,"SlowPeriodSecs":0}}}`,
			wantEnable: true,
			wantFast:   1,
			wantSlow:   5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			cfgPath := tmp + "/config.json"
			if err := os.WriteFile(cfgPath, []byte(tc.json), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			ch := NewConfigHandler(cfgPath, GPUAgentConfig{})
			if err := ch.RefreshConfig(); err != nil {
				t.Fatalf("RefreshConfig: %v", err)
			}

			gotEnable := ch.GetHighResolutionState()
			if gotEnable != tc.wantEnable {
				t.Errorf("Enable = %v, want %v", gotEnable, tc.wantEnable)
			}

			gotFast, gotSlow := ch.GetHighResPeriods()
			if gotFast != tc.wantFast {
				t.Errorf("FastPeriodSecs = %d, want %d", gotFast, tc.wantFast)
			}
			if gotSlow != tc.wantSlow {
				t.Errorf("SlowPeriodSecs = %d, want %d", gotSlow, tc.wantSlow)
			}
		})
	}
}

func TestHighResExampleFiles(t *testing.T) {
	repoRoot := "../../../"
	files := []string{
		repoRoot + "example/config.json",
		repoRoot + "example/config-gpu.json",
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Skipf("example file not found: %v", err)
			}
			var c exportermetrics.MetricConfig
			if err := json.Unmarshal(data, &c); err != nil {
				t.Fatalf("unmarshal %s: %v", f, err)
			}
			hr := c.GetCommonConfig().GetHighResolution()
			if hr == nil {
				t.Fatalf("HighResolution is nil in %s", f)
			}
			if hr.GetEnable() {
				t.Errorf("expected Enable=false (disabled by default) in %s", f)
			}
			if hr.GetFastPeriodSecs() != 1 {
				t.Errorf("FastPeriodSecs = %d, want 1 in %s", hr.GetFastPeriodSecs(), f)
			}
			if hr.GetSlowPeriodSecs() != 5 {
				t.Errorf("SlowPeriodSecs = %d, want 5 in %s", hr.GetSlowPeriodSecs(), f)
			}

			tmp := t.TempDir()
			cfgPath := tmp + "/config.json"
			if err := os.WriteFile(cfgPath, data, 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
			ch := NewConfigHandler(cfgPath, GPUAgentConfig{})
			if err := ch.RefreshConfig(); err != nil {
				t.Fatalf("RefreshConfig: %v", err)
			}
			if ch.GetHighResolutionState() {
				t.Errorf("expected disabled in example file")
			}
			fast, slow := ch.GetHighResPeriods()
			if fast != 1 || slow != 5 {
				t.Errorf("periods = %d/%d, want 1/5", fast, slow)
			}
		})
	}
}
