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
	"reflect"
	"testing"
)

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root,
		"spur/job_9_0",
		"spur/job_9_2",
		"spur/job_9_1",
		"spur/job_10_0",
		"spur/notajob",
		"system.slice/slurmstepd.scope/job_100",
		"system.slice/slurmstepd.scope/job_100/step_0",
		"system.slice/slurmstepd.scope/system",
		"system.slice/sshd.service",
		"kubepods/pod1",
	)
	writeFile(t, filepath.Join(root, "spur/job_11_0"), "a file, not a cgroup")

	got, shadowed, err := Discover(root)
	if err != nil || len(shadowed) != 0 {
		t.Fatalf("Discover: shadowed %v, err %v; want none, nil", shadowed, err)
	}
	var paths []string
	for _, w := range got {
		paths = append(paths, w.JobID+"="+w.Path)
	}
	want := []string{
		"100=/system.slice/slurmstepd.scope/job_100",
		"10=/spur/job_10_0",
		"9=/spur/job_9_2",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("Discover = %v; want %v", paths, want)
	}
}

func TestDiscoverNoSchedulerCgroups(t *testing.T) {
	got, shadowed, err := Discover(t.TempDir())
	if err != nil || len(got) != 0 || len(shadowed) != 0 {
		t.Errorf("Discover(empty) = %v, %v, %v; want none, none, nil", got, shadowed, err)
	}
}

func TestDiscoverSameJobIDInBothLayouts(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root,
		"spur/job_7_1",
		"system.slice/slurmstepd.scope/job_7",
		"system.slice/slurmstepd.scope/job_8",
	)
	got, shadowed, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Workload{{Path: "/system.slice/slurmstepd.scope/job_8", JobID: "8"}, {Path: "/spur/job_7_1", JobID: "7", attempt: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discover = %v; want %v", got, want)
	}
	wantShadowed := []Workload{{Path: "/system.slice/slurmstepd.scope/job_7", JobID: "7"}}
	if !reflect.DeepEqual(shadowed, wantShadowed) {
		t.Errorf("shadowed = %v; want %v", shadowed, wantShadowed)
	}
}
