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

package nicagent

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/snapshotstore"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// fakeCmdExec is an in-test cmdexec.CommandExecuter returning canned nicctl JSON
// per exact command string. It also counts invocations (for the singleflight
// test). Independent of the mock-tagged cmdexec.MockCommandExecuter so this test
// runs under the default build.
type fakeCmdExec struct {
	mu        sync.Mutex
	responses map[string]string
	calls     map[string]int
}

func newFakeCmdExec() *fakeCmdExec {
	return &fakeCmdExec{responses: map[string]string{}, calls: map[string]int{}}
}

func (f *fakeCmdExec) set(cmd, resp string) { f.responses[cmd] = resp }

func (f *fakeCmdExec) callCount(cmd string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[cmd]
}

func (f *fakeCmdExec) Run(cmd string) ([]byte, error) {
	return f.RunWithContext(context.Background(), cmd)
}

func (f *fakeCmdExec) RunWithContext(_ context.Context, cmd string) ([]byte, error) {
	f.mu.Lock()
	f.calls[cmd]++
	resp, ok := f.responses[cmd]
	f.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("fakeCmdExec: no response for %q", cmd)
	}
	return []byte(resp), nil
}

func newParityClient(t *testing.T) (*NICAgentClient, *NICCtlClient, *fakeCmdExec) {
	t.Helper()
	logger.Init(true)

	// The store path now derives metric names via mh.GetPrefix(); a default handler
	// (no MetricsFieldPrefix configured) returns "", keeping the store names raw and
	// byte-identical to the unprefixed gauge gather this test compares against.
	mh, err := metricsutil.NewMetrics(config.NewConfigHandler("config.json", config.GPUAgentConfig{}))
	if err != nil {
		t.Fatalf("new metrics handler: %v", err)
	}

	fake := newFakeCmdExec()
	na := &NICAgentClient{
		mh:               mh,
		cmdExec:          fake,
		fetchDeduper:     snapshotstore.NewDeduper(),
		staticHostLabels: map[string]string{},
		nics: map[string]*NIC{
			"nic-1": {
				Index:        "0",
				UUID:         "nic-1",
				SerialNumber: "SN1",
				Ports:        map[string]*Port{"port-1": {Index: "0", UUID: "port-1", Name: "eth1/1"}},
				Lifs:         map[string]*Lif{"lif-1": {Index: "0", UUID: "lif-1", Name: "eth1", PCIeAddress: "0000:01:00.0"}},
			},
		},
	}
	na.initCustomLabels(nil)
	na.initLabelConfigs(nil)
	na.initFieldConfig(nil)
	na.initPrometheusMetrics()
	return na, newNICCtlClient(na), fake
}

func encodeFamilies(t *testing.T, fams []*dto.MetricFamily) string {
	t.Helper()
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range fams {
		if err := enc.Encode(mf); err != nil {
			t.Fatalf("encode family %s: %v", mf.GetName(), err)
		}
	}
	return buf.String()
}

// assertStoreParity populates the store from the client's current GaugeVecs and
// asserts the StoreCollector exposition is identical to the GaugeVec exposition.
// Both sides are gathered live and encoded the same way (no one-sided text
// re-parse, which would normalize HELP whitespace and mask a true match).
// Returns the covered metric-family names.
func assertStoreParity(t *testing.T, na *NICAgentClient) []string {
	t.Helper()

	// Mirror getMetricsAll's node-total write so the gauge path includes
	// nic_total, matching the store path's explicit node slot.
	na.m.nicNodesTotal.With(na.populateNonNICLabels()).Set(float64(len(na.nics)))

	gaugeFams, err := na.gatherNICFamilies()
	if err != nil {
		t.Fatalf("gather gauge families: %v", err)
	}
	if len(gaugeFams) == 0 {
		t.Fatal("no populated NIC gauge families; fixture produced nothing to compare")
	}

	descs, err := na.nicStoreFieldDescs()
	if err != nil {
		t.Fatalf("build store descriptors: %v", err)
	}
	store := snapshotstore.New()
	if err := na.publishNICMetricsToStore(store); err != nil {
		t.Fatalf("publish to store: %v", err)
	}
	sc := snapshotstore.NewStoreCollector(store, descs)
	reg := prometheus.NewRegistry()
	if err := reg.Register(sc); err != nil {
		t.Fatalf("register store collector: %v", err)
	}
	storeFams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather store collector: %v", err)
	}

	gaugeText := encodeFamilies(t, gaugeFams)
	storeText := encodeFamilies(t, storeFams)
	if gaugeText != storeText {
		t.Fatalf("store path not byte-identical to gauge path:\n%s", firstDiff(gaugeText, storeText))
	}

	names := make([]string, 0, len(gaugeFams))
	for _, mf := range gaugeFams {
		names = append(names, mf.GetName())
	}
	return names
}

// firstDiff returns the first differing line pair (gauge vs store) for a concise
// failure message.
func firstDiff(gauge, store string) string {
	g := strings.Split(gauge, "\n")
	s := strings.Split(store, "\n")
	n := len(g)
	if len(s) < n {
		n = len(s)
	}
	for i := 0; i < n; i++ {
		if g[i] != s[i] {
			return fmt.Sprintf("line %d:\n  gauge: %q\n  store: %q", i+1, g[i], s[i])
		}
	}
	return fmt.Sprintf("line counts differ: gauge=%d store=%d", len(g), len(s))
}

// Port + LIF + LIF-aggregated-QP families, driven through the real NICCtlClient
// parse path on a canned nicctl fixture.
func TestNICStoreParityPortLifQP(t *testing.T) {
	na, nc, fake := newParityClient(t)

	fake.set("nicctl show port statistics -j", `{
		"nic": [{"id": "nic-1", "port": [{
			"spec": {"id": "port-1"},
			"statistics": {"frames_rx_ok": "100", "frames_tx_ok": "50", "octets_rx_ok": "1200"},
			"status": {"number_of_link_down_events": "3"}
		}]}]
	}`)
	fake.set("nicctl show port statistics --rate -j", `{"nic": []}`)
	fake.set("nicctl show lif statistics -j", `{
		"nic": [{"id": "nic-1", "lif": [{
			"spec": {"id": "lif-1"},
			"statistics": {"rx_unicast_packets": "7", "tx_unicast_packets": "9", "rx_dma_errors": "1"}
		}]}]
	}`)
	fake.set("nicctl show rdma queue-pair statistics --card nic-1 -j", `{
		"nic": [{"id": "nic-1", "lif": [{
			"spec": {"id": "lif-1"},
			"queue_pair": [{
				"spec": {"id": "qp-1"},
				"statistics": {
					"send_queue": {
						"requester_tx_statistics": {"num_packet": "4"},
						"requester_rx_statistcs": {},
						"dcqcn_stats": {"num_cnp_rcvd": "2"}
					},
					"receive_queue": {
						"responder_tx_statistics": {},
						"responder_rx_statistics": {"num_packet": "6"}
					}
				}
			}]
		}]}]
	}`)

	ctx := context.Background()
	if err := nc.UpdatePortStats(ctx, nil); err != nil {
		t.Fatalf("UpdatePortStats: %v", err)
	}
	if err := nc.UpdateLifStats(ctx, nil); err != nil {
		t.Fatalf("UpdateLifStats: %v", err)
	}
	if err := nc.UpdateQPStats(ctx, nil); err != nil {
		t.Fatalf("UpdateQPStats: %v", err)
	}

	names := assertStoreParity(t, na)

	// Confirm each targeted family is actually present in the compared set.
	want := map[string]bool{
		"nic_port_stats_frames_rx_ok":       false, // port
		"nic_lif_stats_rx_unicast_packets":  false, // lif
		"lif_qp_sq_req_tx_num_packet_total": false, // lif-aggregated QP
	}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, seen := range want {
		if !seen {
			t.Errorf("expected family %q in parity coverage, not found", n)
		}
	}
	t.Logf("parity covered %d NIC metric families (port/lif/lif-agg-qp)", len(names))
}

// Per-QP family (QP_*): requires QP fields enabled in config, which disables the
// default families, so it is exercised in a dedicated client.
func TestNICStoreParityPerQP(t *testing.T) {
	na, nc, fake := newParityClient(t)

	// Re-init field config to enable per-QP metrics explicitly (listing fields
	// disables the default families, so per-QP is tested in isolation).
	// NIC_TOTAL is included so the gauge path also exposes the always-present
	// node-level nic_total slot for the parity comparison.
	na.initFieldConfig(&exportermetrics.NICMetricConfig{
		Fields: []string{"NIC_TOTAL", "QP_SQ_REQ_TX_NUM_PACKET", "QP_RQ_RSP_RX_NUM_PACKET"},
	})
	na.initPrometheusMetrics()

	fake.set("nicctl show rdma queue-pair statistics --card nic-1 -j", `{
		"nic": [{"id": "nic-1", "lif": [{
			"spec": {"id": "lif-1"},
			"queue_pair": [{
				"spec": {"id": "qp-1"},
				"statistics": {
					"send_queue": {
						"requester_tx_statistics": {"num_packet": "11"},
						"requester_rx_statistcs": {}
					},
					"receive_queue": {
						"responder_tx_statistics": {},
						"responder_rx_statistics": {"num_packet": "22"}
					}
				}
			}]
		}]}]
	}`)

	if err := nc.UpdateQPStats(context.Background(), nil); err != nil {
		t.Fatalf("UpdateQPStats: %v", err)
	}

	names := assertStoreParity(t, na)
	found := false
	for _, n := range names {
		if n == "qp_sq_req_tx_num_packet" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected per-QP family qp_sq_req_tx_num_packet in coverage, got %v", names)
	}
	t.Logf("per-QP parity covered %d families", len(names))
}

// countingNICClient is a fake NICInterface that counts UpdateNICStats executions
// and blocks until released, so the singleflight collapse is deterministic.
type countingNICClient struct {
	mu      sync.Mutex
	count   int
	started chan struct{}
	release chan struct{}
}

func (c *countingNICClient) UpdateNICStats(_ context.Context, _ map[string]scheduler.Workload) error {
	c.mu.Lock()
	c.count++
	first := c.count == 1
	c.mu.Unlock()
	if first {
		close(c.started)
	}
	<-c.release
	return nil
}
func (c *countingNICClient) Init() error           { return nil }
func (c *countingNICClient) GetClientName() string { return "counting" }
func (c *countingNICClient) IsActive() bool        { return true }

// Concurrent scrapes share one underlying fetch (Approach 1 singleflight).
func TestNICFetchMetricsOnceSingleflight(t *testing.T) {
	na, _, _ := newParityClient(t)
	cc := &countingNICClient{started: make(chan struct{}), release: make(chan struct{})}
	na.nicClients = []NICInterface{cc}

	const followers = 8
	ctx := context.Background()

	var leaderWG sync.WaitGroup
	leaderWG.Add(1)
	go func() {
		defer leaderWG.Done()
		if err := na.fetchMetricsOnce(ctx); err != nil {
			t.Errorf("leader fetch: %v", err)
		}
	}()

	<-cc.started // leader is in-flight

	var followerWG sync.WaitGroup
	for i := 0; i < followers; i++ {
		followerWG.Add(1)
		go func() {
			defer followerWG.Done()
			if err := na.fetchMetricsOnce(ctx); err != nil {
				t.Errorf("follower fetch: %v", err)
			}
		}()
	}

	// Let followers attach to the in-flight call before releasing the leader.
	time.Sleep(50 * time.Millisecond)
	close(cc.release)

	leaderWG.Wait()
	followerWG.Wait()

	cc.mu.Lock()
	got := cc.count
	cc.mu.Unlock()
	if got != 1 {
		t.Fatalf("expected 1 underlying fetch for %d concurrent scrapes, got %d", followers+1, got)
	}
}

// Node-level nic_total must be present with value 0 at zero NICs (empty fetch),
// even though no GaugeVec child exists.
func TestNICStoreNodeTotalPresentAtZeroNICs(t *testing.T) {
	na, _, _ := newParityClient(t)
	na.nics = map[string]*NIC{} // zero NICs, no fetch

	descs, err := na.nicStoreFieldDescs()
	if err != nil {
		t.Fatalf("build store descriptors: %v", err)
	}
	store := snapshotstore.New()
	if err := na.publishNICMetricsToStore(store); err != nil {
		t.Fatalf("publish to store: %v", err)
	}
	sc := snapshotstore.NewStoreCollector(store, descs)

	want := `# HELP nic_total Number of NICs in the node
# TYPE nic_total gauge
nic_total{hostname=""} 0
`
	if err := testutil.CollectAndCompare(sc, strings.NewReader(want)); err != nil {
		t.Fatalf("nic_total not present/correct at zero NICs:\n%v", err)
	}
}
