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

package cgroup

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
)

func writeCgroup(t testing.TB, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		writeFile(t, filepath.Join(dir, name), body)
	}
}

func fullCgroup() map[string]string {
	return map[string]string{
		"cpu.stat":            "usage_usec 2500000\nuser_usec 2000000\nsystem_usec 500000\nnr_periods 100\nnr_throttled 4\nthrottled_usec 500000\n",
		"memory.current":      "3000\n",
		"memory.max":          "8589934592\n",
		"memory.swap.current": "10\n",
		"memory.stat":         "anon 2000\nfile 900\nactive_file 400\ninactive_file 500\n",
		"memory.events":       "low 0\nhigh 0\nmax 7\noom 2\noom_kill 1\noom_group_kill 0\n",
		"pids.current":        "42\n",
		"io.stat":             "8:0 rbytes=100 wbytes=200 rios=3 wios=4 dbytes=0 dios=0\n259:0 rbytes=1 wbytes=2 rios=0 wios=1 dbytes=99 dios=9\n",
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	writeCgroup(t, dir, fullCgroup())

	got, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Field]float64{
		CPUUsageSeconds:     2.5,
		CPUThrottledSeconds: 0.5,
		CPUPeriods:          100,
		CPUThrottledPeriods: 4,
		MemoryCurrentBytes:  3000,
		MemoryLimitBytes:    8 << 30,
		MemorySwapBytes:     10,
		MemoryRSSBytes:      2000,
		MemoryWorkingSet:    2500,
		MemoryOOMKills:      1,
		Tasks:               42,
		IOReadBytes:         101,
		IOWriteBytes:        202,
		IOReads:             3,
		IOWrites:            5,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Read =\n%v\nwant\n%v", got, want)
	}
}

func TestReadOmitsWhatTheKernelDoesNotReport(t *testing.T) {
	files := fullCgroup()
	files["memory.max"] = "max\n"
	delete(files, "memory.swap.current")
	delete(files, "io.stat")
	delete(files, "memory.events")
	delete(files, "pids.current")
	dir := t.TempDir()
	writeCgroup(t, dir, files)

	got, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Field{MemoryLimitBytes, MemorySwapBytes, MemoryOOMKills, Tasks, IOReadBytes, IOWriteBytes, IOReads, IOWrites} {
		if _, ok := got[f]; ok {
			t.Errorf("Read reported %s = %v; want it absent", f, got[f])
		}
	}
	if got[MemoryCurrentBytes] != 3000 {
		t.Errorf("memory current = %v; want 3000", got[MemoryCurrentBytes])
	}
}

func TestReadWorkingSetClampsAtZero(t *testing.T) {
	files := fullCgroup()
	files["memory.stat"] = "anon 10\ninactive_file 5000\n"
	dir := t.TempDir()
	writeCgroup(t, dir, files)

	got, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got[MemoryWorkingSet] != 0 {
		t.Errorf("working set = %v; want 0", got[MemoryWorkingSet])
	}
}

func TestReadEndedCgroup(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "gone"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read(missing) err = %v; want fs.ErrNotExist", err)
	}
}

func TestReadMalformedValue(t *testing.T) {
	files := fullCgroup()
	files["memory.current"] = "lots\n"
	dir := t.TempDir()
	writeCgroup(t, dir, files)

	if _, err := Read(dir); err == nil {
		t.Error("Read with malformed memory.current: want error")
	}
}

func TestIsV2(t *testing.T) {
	v2, v1 := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(v2, "cgroup.controllers"), "cpu memory io\n")
	writeFile(t, filepath.Join(v1, "cpu", "cpu.shares"), "1024\n")

	if !IsV2(v2) || IsV2(v1) {
		t.Errorf("IsV2(v2) = %v, IsV2(v1) = %v; want true, false", IsV2(v2), IsV2(v1))
	}
}
