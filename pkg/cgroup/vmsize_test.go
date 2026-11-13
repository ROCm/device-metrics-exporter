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
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t testing.TB, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVirtualMemoryBytesSumsSubtree(t *testing.T) {
	cg, proc := t.TempDir(), t.TempDir()
	job := filepath.Join(cg, "system.slice/slurmstepd.scope/job_7")
	writeFile(t, filepath.Join(job, "cgroup.procs"), "")
	writeFile(t, filepath.Join(job, "step_0", "user", "task_0", "cgroup.procs"), "10\n11\n")
	writeFile(t, filepath.Join(job, "step_extern", "cgroup.procs"), "12\n0\n13\n")
	writeFile(t, filepath.Join(proc, "10", "status"), "Name:\ta\nVmPeak:\t 9 kB\nVmSize:\t    100 kB\n")
	writeFile(t, filepath.Join(proc, "11", "status"), "Name:\tb\nVmSize:\t 200 kB\n")
	writeFile(t, filepath.Join(proc, "12", "status"), "Name:\tkthread\n")
	// pid 13 exited between listing and reading.

	got, resolved, err := VirtualMemoryBytes(job, proc)
	if err != nil {
		t.Fatal(err)
	}
	if want := float64(300 * 1024); got != want || resolved != 2 {
		t.Errorf("VirtualMemoryBytes = %v, %d; want %v, 2", got, resolved, want)
	}
}

func TestVirtualMemoryBytesForeignPIDNamespace(t *testing.T) {
	cg, proc := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(cg, "job_1", "cgroup.procs"), "0\n0\n")

	_, resolved, err := VirtualMemoryBytes(filepath.Join(cg, "job_1"), proc)
	if err != nil || resolved != 0 {
		t.Errorf("VirtualMemoryBytes = _, %d, %v; want 0 resolved, nil", resolved, err)
	}
}

func TestVirtualMemoryBytesEndedWorkload(t *testing.T) {
	got, resolved, err := VirtualMemoryBytes(filepath.Join(t.TempDir(), "spur/job_1_0"), t.TempDir())
	if err != nil || got != 0 || resolved != 0 {
		t.Errorf("VirtualMemoryBytes(missing) = %v, %d, %v; want 0, 0, nil", got, resolved, err)
	}
}
