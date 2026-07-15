//go:build mock
// +build mock

/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
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

	err := nc.UpdatePortStats(context.Background(), nil)
	assert.Assert(t, err == nil, "expected no error from UpdatePortStats: %v", err)

	count := testutil.CollectAndCount(&nc.na.m.nicPortStatusLinkDownCount)
	assert.Equal(t, 0, count, "expected no link-down series when status is absent")
}
