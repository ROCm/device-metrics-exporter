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

// Package cgroup reads per-workload resource usage from the cgroup v2
// hierarchy.
package cgroup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const usPerSecond = 1e6

var errUnlimited = errors.New("unlimited")

// IsV2 reports whether root is a cgroup v2 (unified) mount.
func IsV2(root string) bool {
	_, err := os.Stat(filepath.Join(root, "cgroup.controllers"))
	return err == nil
}

// Read returns the fields of the cgroup at dir. A file the kernel does not
// provide, e.g. a controller not enabled for the subtree or swap accounting
// off, leaves its fields out. It returns fs.ErrNotExist when the cgroup is gone.
func Read(dir string) (map[Field]float64, error) {
	cpu, err := readKeyed(filepath.Join(dir, "cpu.stat"))
	if err != nil {
		return nil, err
	}
	out := map[Field]float64{}
	setScaled(out, cpu, "usage_usec", CPUUsageSeconds, usPerSecond)
	setScaled(out, cpu, "throttled_usec", CPUThrottledSeconds, usPerSecond)
	setScaled(out, cpu, "nr_periods", CPUPeriods, 1)
	setScaled(out, cpu, "nr_throttled", CPUThrottledPeriods, 1)

	if err := readMemory(dir, out); err != nil {
		return nil, err
	}

	// pids.current counts threads, not processes.
	tasks, err := readValue(filepath.Join(dir, "pids.current"))
	switch {
	case err == nil:
		out[Tasks] = tasks
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	rd, wr, rios, wios, err := readIO(filepath.Join(dir, "io.stat"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		out[IOReadBytes], out[IOWriteBytes], out[IOReads], out[IOWrites] = rd, wr, rios, wios
	}
	return out, nil
}

func readMemory(dir string, out map[Field]float64) error {
	current, err := readValue(filepath.Join(dir, "memory.current"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	out[MemoryCurrentBytes] = current

	limit, err := readValue(filepath.Join(dir, "memory.max"))
	switch {
	case err == nil:
		out[MemoryLimitBytes] = limit
	case !errors.Is(err, errUnlimited) && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	swap, err := readValue(filepath.Join(dir, "memory.swap.current"))
	switch {
	case err == nil:
		out[MemorySwapBytes] = swap
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	// memory.events is hierarchical, so OOM kills in job steps count too.
	events, err := readKeyed(filepath.Join(dir, "memory.events"))
	switch {
	case err == nil:
		setScaled(out, events, "oom_kill", MemoryOOMKills, 1)
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	stat, err := readKeyed(filepath.Join(dir, "memory.stat"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if anon, ok := stat["anon"]; ok {
		out[MemoryRSSBytes] = float64(anon)
	}
	if inactive, ok := stat["inactive_file"]; ok {
		out[MemoryWorkingSet] = max(current-float64(inactive), 0)
	}
	return nil
}

// readValue reads a single-value file; "max" yields errUnlimited.
func readValue(path string) (float64, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(buf))
	if s == "max" {
		return 0, errUnlimited
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return float64(v), nil
}

// readKeyed reads a flat keyed file of "key value" lines (cpu.stat, memory.stat).
func readKeyed(path string) (map[string]uint64, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]uint64{}
	sc := bufio.NewScanner(bytes.NewReader(buf))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		if v, err := strconv.ParseUint(f[1], 10, 64); err == nil {
			out[f[0]] = v
		}
	}
	return out, sc.Err()
}

// readIO sums io.stat over devices so each workload stays one series. Lines
// look like "8:0 rbytes=1 wbytes=2 rios=3 wios=4 dbytes=0 dios=0".
func readIO(path string) (rd, wr, rios, wios float64, err error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	sc := bufio.NewScanner(bytes.NewReader(buf))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		for _, kv := range f[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			n, perr := strconv.ParseUint(v, 10, 64)
			if perr != nil {
				continue
			}
			switch k {
			case "rbytes":
				rd += float64(n)
			case "wbytes":
				wr += float64(n)
			case "rios":
				rios += float64(n)
			case "wios":
				wios += float64(n)
			}
		}
	}
	return rd, wr, rios, wios, sc.Err()
}

func setScaled(out map[Field]float64, kv map[string]uint64, key string, f Field, div float64) {
	if v, ok := kv[key]; ok {
		out[f] = float64(v) / div
	}
}
