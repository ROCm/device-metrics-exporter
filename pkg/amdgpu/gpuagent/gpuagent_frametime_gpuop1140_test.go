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

// GPUOP-1140 case 58 measurement. Calls base APIs only (oracle harness
// startHR, hr cache start, applyPowerFrame), so the same file runs on the base
// copy of b2efd1b32 for the before/after comparison. Gated by
// GPUOP1140_FRAMETIME=<duration>.

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"
)

// TestHotFrameTimeSteady applies a partial power frame every 1 ms for the
// gated duration with the hr label cache ticking every 3 s and unchanged
// workloads. It records the frame-time median/p99/max, the hot-cache rebuilds
// (hrHot pointer changes) and the worst frame within 5 ms after a rebuild.
func TestHotFrameTimeSteady(t *testing.T) {
	dur, err := time.ParseDuration(os.Getenv("GPUOP1140_FRAMETIME"))
	if err != nil || dur <= 0 {
		t.Skip("set GPUOP1140_FRAMETIME=<duration> to run")
	}
	var fx oracleFixture
	for _, f := range oracleFixtures() {
		if f.name == "slurm" {
			fx = f
		}
	}
	e := newOracleEnv(t, oracleHRConfig, fx)
	hc := e.startHR(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hc.start(ctx, 3*time.Second)
	e.gpu.hrFastMs.Store(10)
	var ids [][]byte
	for _, g := range oracleGPUResp(oracleGPUs).Response {
		ids = append(ids, g.Spec.Id)
	}
	frame := hrPartial(ids, hrPowerTick(100)...)
	e.gpu.applyPowerFrame(frame) // warm
	var d []time.Duration
	rebuilds := 0
	var worstAfter time.Duration
	var lastRebuild time.Time
	start := time.Now()
	for time.Since(start) < dur {
		hot := e.gpu.hrHot
		t0 := time.Now()
		e.gpu.applyPowerFrame(frame)
		el := time.Since(t0)
		d = append(d, el)
		if e.gpu.hrHot != hot {
			rebuilds++
			lastRebuild = t0
		}
		if !lastRebuild.IsZero() && t0.Sub(lastRebuild) < 5*time.Millisecond && el > worstAfter {
			worstAfter = el
		}
		time.Sleep(time.Millisecond)
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	n := len(d)
	t.Logf("CASE58 frames=%d over %v: median=%v p99=%v p99.9=%v max=%v; hot rebuilds=%d; "+
		"worst frame within 5ms of a rebuild=%v", n, dur, d[n/2], d[n*99/100], d[n*999/1000], d[n-1],
		rebuilds, worstAfter)
}
