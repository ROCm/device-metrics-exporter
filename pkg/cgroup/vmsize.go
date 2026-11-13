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
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// VirtualMemoryBytes sums VmSize over every process in the cgroup subtree at
// dir and reports how many processes it resolved. cgroup v2 keeps processes
// only in leaf cgroups, hence the walk. Outside the host PID namespace
// cgroup.procs lists every process as 0, so nothing resolves.
func VirtualMemoryBytes(dir, procRoot string) (float64, int, error) {
	var total float64
	resolved := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// The workload, or one of its steps, can end mid-walk.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		procs, err := os.ReadFile(filepath.Join(path, "cgroup.procs"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		for _, pid := range strings.Fields(string(procs)) {
			if pid == "0" {
				continue
			}
			if kb, ok := vmSizeKiB(filepath.Join(procRoot, pid, "status")); ok {
				total += kb * 1024
				resolved++
			}
		}
		return nil
	})
	return total, resolved, err
}

// vmSizeKiB returns false for processes that exited or have no address space
// (kernel threads).
func vmSizeKiB(statusPath string) (float64, bool) {
	buf, err := os.ReadFile(statusPath)
	if err != nil {
		return 0, false
	}
	sc := bufio.NewScanner(bytes.NewReader(buf))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmSize:") {
			continue
		}
		f := strings.Fields(strings.TrimPrefix(line, "VmSize:"))
		if len(f) == 0 {
			return 0, false
		}
		v, err := strconv.ParseFloat(f[0], 64)
		return v, err == nil
	}
	return 0, false
}
