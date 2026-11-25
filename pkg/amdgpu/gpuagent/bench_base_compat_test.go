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

package gpuagent

// GPUOP-1140 base-compatible benchmark (test plan case 55). Calls base APIs
// only (hr workload-label cache refreshOnce over fake schedulers), so it
// compiles and runs on b2efd1b32 and on the post-change tree. Needs
// oracle_gpuop1140_test.go (fixture helpers) in the same package.
//
//	go test -mod=vendor ./pkg/amdgpu/gpuagent/ -run x \
//	    -bench 'BenchmarkWorkloadLabelRefresh8GPU/legacy' -benchmem -count=10

import (
	"fmt"
	"maps"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

// benchFixture8 places one workload per GPU on an 8-GPU node.
func benchFixture8(k8s bool) oracleFixture {
	if k8s {
		m := map[string]scheduler.Workload{}
		for i := 0; i < 8; i++ {
			m[fmt.Sprintf("0000:0%d:00.0", i+1)] = oraclePod(i)
		}
		return oracleFixture{name: "bench-k8s", k8s: m}
	}
	m := map[string]scheduler.Workload{}
	for i := 0; i < 8; i++ {
		m[fmt.Sprint(i)] = oracleJob(fmt.Sprint(100+i), fmt.Sprintf("user%d", i))
	}
	return oracleFixture{name: "bench-slurm", slurm: m}
}

// BenchmarkWorkloadLabelRefresh8GPU/legacy: one hr label-cache refresh with
// unchanged slurm workloads (base: ListWorkloads copy + full label rebuild +
// swap every refresh). legacy-k8s: same over a fake k8s scheduler (no gRPC
// decode; the fake returns a copied map).
//
// GPUOP-1142 (post-change only, not base-compatible): unchanged / unchanged-k8s
// / changed measure one full refresh interval on the shared source, i.e. a
// source refresh plus the hr label-cache refresh: unchanged on a slurm node
// (ChangeNotifier, version memo), unchanged on k8s (the fake List copy is the
// only work) and changed (one GPU's job changes every refresh).
func BenchmarkWorkloadLabelRefresh8GPU(b *testing.B) {
	benchWorkloadLabelRefreshLegacy(b)
	for _, tc := range []struct {
		name    string
		k8s     bool
		changed bool
	}{{"unchanged", false, false}, {"unchanged-k8s", true, false}, {"changed", false, true}} {
		b.Run(tc.name, func(b *testing.B) {
			fx := benchFixture8(tc.k8s)
			e := newOracleEnv(b, oracleHRConfig, fx)
			var slurm *notifySched
			if !tc.k8s {
				slurm = newNotifySched(fx.slurm)
				e.ga.slurmScheduler = slurm
			}
			_, pm, err := e.gpu.getGPUs(e.gpu.gCache.gpuGetFilter, &e.gpu.gCache.metricsCache)
			if err != nil {
				b.Fatal(err)
			}
			gpus := oracleGPUResp(8).Response
			e.gpu.initGPUMetadata(gpus)
			e.gpu.hrSnapshot.Store(&hrGPUSnap{gpus: gpus, partitionMap: pm})
			src := manualSource(e) // no goroutine: the loop drives the refresh
			c := newHRWorkloadLabelCache(e.gpu)
			c.refreshOnce()
			var alt [2]map[string]scheduler.Workload
			if tc.changed {
				alt = [2]map[string]scheduler.Workload{fx.slurm, maps.Clone(fx.slurm)}
				alt[1]["3"] = oracleJob("999", "other")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if tc.changed {
					// the job event itself (slurm watcher work) is not part
					// of the refresh, as in the base benchmark
					b.StopTimer()
					slurm.set(alt[(i+1)%2])
					b.StartTimer()
				}
				src.refresh(true)
				c.refreshOnce()
			}
		})
	}
}

func benchWorkloadLabelRefreshLegacy(b *testing.B) {
	for _, tc := range []struct {
		name string
		k8s  bool
	}{{"legacy", false}, {"legacy-k8s", true}} {
		b.Run(tc.name, func(b *testing.B) {
			e := newOracleEnv(b, oracleHRConfig, benchFixture8(tc.k8s))
			_, pm, err := e.gpu.getGPUs(e.gpu.gCache.gpuGetFilter, &e.gpu.gCache.metricsCache)
			if err != nil {
				b.Fatal(err)
			}
			gpus := oracleGPUResp(8).Response
			e.gpu.initGPUMetadata(gpus)
			e.gpu.hrSnapshot.Store(&hrGPUSnap{gpus: gpus, partitionMap: pm})
			c := newHRWorkloadLabelCache(e.gpu)
			c.refreshOnce()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.refreshOnce()
			}
		})
	}
}
