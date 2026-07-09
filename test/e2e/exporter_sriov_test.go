/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package e2e

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	testutils "github.com/ROCm/device-metrics-exporter/test/utils"
)

// Real-GIM SR-IOV e2e (GPUOP-880). Runs the sriov exporter image (real
// libgim_amd_smi.so) against a live GIM SR-IOV host and asserts that the
// gpu-agent stream walker (smi_walk_gpu_metrics) populates the host-side
// metric families from real firmware.
//
// Hardware-bound: it only runs when SRIOV_EXPORTER_IMAGE is set, so the
// generic mock e2e suite is unaffected. On a GIM host (e.g. banff):
//
//	SRIOV_EXPORTER_IMAGE=local/device-metrics-exporter-sriov:latest \
//	  go test -run TestSRIOVRealGIM -v
//
// Values are not asserted against fixed carry-values (real firmware varies);
// instead we assert the families are present, carry the expected index
// labels, and never leak the amdsmi UINT32_MAX sentinel.

const (
	sriovContainerName = "test_exporter_sriov"
	sriovHostPort      = 5004
	// amdsmi UINT32_MAX sentinel for an unpopulated metric slot; the walker
	// must decode absent rows to this and DME must suppress them, never emit.
	sriovSentinel = 4294967295.0
)

func sriovImageURL() string {
	return os.Getenv("SRIOV_EXPORTER_IMAGE")
}

func scrapeSRIOV(client *http.Client) (string, error) {
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/metrics", sriovHostPort))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// TestSRIOVRealGIM verifies real GIM host-side metric coverage:
//   - VCN/JPEG activity present with per-engine index labels
//   - GFX/UMC activity present
//   - no series leaks the UINT32_MAX sentinel
func TestSRIOVRealGIM(t *testing.T) {
	image := sriovImageURL()
	if image == "" {
		t.Skip("SRIOV_EXPORTER_IMAGE not set; skipping real-GIM SR-IOV e2e")
	}
	t.Logf("real-GIM sriov image: %s", image)

	sriov := NewMockExporter(sriovContainerName, image)
	if sriov == nil {
		t.Fatal("failed to create sriov exporter")
	}
	sriov.SetPortMap(map[int]int{sriovHostPort: 5000})
	sriov.SkipConfigMount()

	_ = sriov.Stop()
	time.Sleep(2 * time.Second)
	_ = sriov.Cleanup()
	time.Sleep(1 * time.Second)

	if err := sriov.Start(); err != nil {
		t.Fatalf("failed to start sriov exporter: %v", err)
	}
	defer func() {
		_ = sriov.Stop()
		time.Sleep(2 * time.Second)
		_ = sriov.Cleanup()
	}()

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		Timeout:   10 * time.Second,
	}

	var all []testutils.MetricSeries
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		body, err := scrapeSRIOV(client)
		if err == nil && body != "" {
			all = testutils.ParseAllMetricSeries(body)
			if len(all) > 0 {
				break
			}
		}
		time.Sleep(3 * time.Second)
	}
	if len(all) == 0 {
		t.Fatal("no metrics scraped from sriov exporter")
	}

	// families the GIM stream walker must populate on a real host
	requireFamily := func(name, indexLabel string) {
		series := testutils.FilterSeries(all, name)
		if len(series) == 0 {
			t.Errorf("%s: no series present (walker did not populate from GIM stream)", name)
			return
		}
		for _, s := range series {
			v, err := strconv.ParseFloat(s.Value, 64)
			if err != nil {
				t.Errorf("%s: unparseable value %q", name, s.Value)
				continue
			}
			if v == sriovSentinel {
				t.Errorf("%s: sentinel leaked for %s=%s", name, indexLabel, s.Labels[indexLabel])
			}
			if indexLabel != "" {
				if _, ok := s.Labels[indexLabel]; !ok {
					t.Errorf("%s: missing index label %q", name, indexLabel)
				}
			}
		}
		t.Logf("%s: %d series present, no sentinel", name, len(series))
	}

	requireFamily("gpu_vcn_activity", "vcn_index")
	requireFamily("gpu_jpeg_activity", "jpeg_index")
	requireFamily("gpu_gfx_activity", "")
	requireFamily("gpu_umc_activity", "")
}
