//go:build mock
// +build mock

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

package nicagent

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/nicagent/cmdexec"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// newTestNICCtlClient builds a NICCtlClient wired to a MockCommandExecuter,
// with Prometheus metrics initialized the same way InitConfigs() would (minus
// the metricsutil.MetricsHandler dependency, which isn't needed to exercise
// UpdatePortStats directly).
func newTestNICCtlClient(t *testing.T) (*NICCtlClient, *cmdexec.MockCommandExecuter) {
	t.Helper()

	logger.Init(true)

	mockExec, ok := cmdexec.NewExecuter().(*cmdexec.MockCommandExecuter)
	assert.Assert(t, ok, "expected mock build to provide a MockCommandExecuter")

	na := &NICAgentClient{
		cmdExec: mockExec,
		nics: map[string]*NIC{
			"nic-1": {
				Index: "0",
				UUID:  "nic-1",
				Ports: map[string]*Port{
					"port-1": {Index: "0", UUID: "port-1", Name: "eth1/1"},
				},
			},
		},
	}
	na.initCustomLabels(nil)
	na.initLabelConfigs(nil)
	na.initFieldConfig(nil)
	na.initPrometheusMetrics()

	return newNICCtlClient(na), mockExec
}

func TestUpdatePortStatsParsesLinkDownCountFromStatus(t *testing.T) {
	nc, mockExec := newTestNICCtlClient(t)

	mockExec.AddResponse("nicctl show port statistics -j", `{
		"nic": [
			{
				"id": "nic-1",
				"port": [
					{
						"spec": {"id": "port-1"},
						"statistics": {"frames_rx_ok": "100"},
						"status": {"number_of_link_down_events": "7"}
					}
				]
			}
		]
	}`)
	mockExec.AddResponse("nicctl show port statistics --rate -j", `{"nic": []}`)

	err := nc.UpdatePortStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no error from UpdatePortStats: %v", err)

	assert.Equal(t, float64(7), testutil.ToFloat64(&nc.na.m.nicPortStatusLinkDownCount))
	assert.Equal(t, float64(100), testutil.ToFloat64(&nc.na.m.nicPortStatsFramesRxOk))
}

// TestUpdatePortStatsHandlesMissingStatus guards against nicctl output that
// predates the "status" section: it must not panic, and the link-down gauge
// should simply have no recorded series rather than a false zero value.
func TestUpdatePortStatsHandlesMissingStatus(t *testing.T) {
	nc, mockExec := newTestNICCtlClient(t)

	mockExec.AddResponse("nicctl show port statistics -j", `{
		"nic": [
			{
				"id": "nic-1",
				"port": [
					{
						"spec": {"id": "port-1"},
						"statistics": {"frames_rx_ok": "100"}
					}
				]
			}
		]
	}`)
	mockExec.AddResponse("nicctl show port statistics --rate -j", `{"nic": []}`)

	err := nc.UpdatePortStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no error from UpdatePortStats: %v", err)

	count := testutil.CollectAndCount(&nc.na.m.nicPortStatusLinkDownCount)
	assert.Equal(t, 0, count, "expected no link-down series when status is absent")
}

func TestUpdateEnvironmentStatsParsesPowerMetrics(t *testing.T) {
	nc, mockExec := newTestNICCtlClient(t)

	mockExec.AddResponse("nicctl show environment -j", `{
		"nic": [
			{
				"id": "nic-1",
				"power": {
					"total_power_drawn": "27.156",
					"core_power": "11.296",
					"arm_power": "0.109"
				}
			}
		]
	}`)

	err := nc.UpdateEnvironmentStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no error: %v", err)

	assert.Equal(t, 27.156, testutil.ToFloat64(&nc.na.m.nicTotalPowerDrawn))
	assert.Equal(t, 11.296, testutil.ToFloat64(&nc.na.m.nicCorePower))
	assert.Equal(t, 0.109, testutil.ToFloat64(&nc.na.m.nicArmPower))
}

func TestUpdateEnvironmentStatsMultipleNICs(t *testing.T) {
	nc, mockExec := newTestNICCtlClient(t)

	nc.na.nics["nic-2"] = &NIC{
		Index: "1",
		UUID:  "nic-2",
	}

	mockExec.AddResponse("nicctl show environment -j", `{
		"nic": [
			{
				"id": "nic-1",
				"power": {
					"total_power_drawn": "27.0",
					"core_power": "11.0",
					"arm_power": "0.1"
				}
			},
			{
				"id": "nic-2",
				"power": {
					"total_power_drawn": "25.0",
					"core_power": "10.0",
					"arm_power": "0.2"
				}
			}
		]
	}`)

	err := nc.UpdateEnvironmentStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no error: %v", err)

	count := testutil.CollectAndCount(&nc.na.m.nicTotalPowerDrawn)
	assert.Equal(t, 2, count, "expected 2 NIC power series")
}

func TestUpdateEnvironmentStatsHandlesParseError(t *testing.T) {
	nc, mockExec := newTestNICCtlClient(t)

	mockExec.AddResponse("nicctl show environment -j", `{
		"nic": [
			{
				"id": "nic-1",
				"power": {
					"total_power_drawn": "invalid",
					"core_power": "11.0",
					"arm_power": "0.1"
				}
			}
		]
	}`)

	err := nc.UpdateEnvironmentStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no error even with parse failure")

	assert.Equal(t, 11.0, testutil.ToFloat64(&nc.na.m.nicCorePower))
	assert.Equal(t, 0.1, testutil.ToFloat64(&nc.na.m.nicArmPower))
}

func TestUpdateEnvironmentStatsCommandFailure(t *testing.T) {
	nc, _ := newTestNICCtlClient(t)

	err := nc.UpdateEnvironmentStats(context.Background(), nil)
	assert.Assert(t, err != nil, "expected error when nicctl command not found")
}

func TestUpdateEnvironmentStatsDisabledByConfig(t *testing.T) {
	nc, _ := newTestNICCtlClient(t)

	fetchEnvironmentMetrics = false
	defer func() { fetchEnvironmentMetrics = true }()

	err := nc.UpdateEnvironmentStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no error when disabled")
}

func TestUpdateEnvironmentStatsUnknownNICDoesNotPanic(t *testing.T) {
	nc, mockExec := newTestNICCtlClient(t)

	mockExec.AddResponse("nicctl show environment -j", `{
		"nic": [
			{
				"id": "unknown-nic",
				"power": {
					"total_power_drawn": "20.0",
					"core_power": "8.0",
					"arm_power": "0.05"
				}
			}
		]
	}`)

	err := nc.UpdateEnvironmentStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no panic or error for unknown NIC")

	assert.Equal(t, 20.0, testutil.ToFloat64(&nc.na.m.nicTotalPowerDrawn))
}
