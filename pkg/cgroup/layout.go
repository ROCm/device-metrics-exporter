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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// Workload is one job cgroup, Path relative to the cgroup root.
type Workload struct {
	Path    string
	JobID   string
	attempt int
}

type layout struct {
	dir string
	re  *regexp.Regexp
}

// layouts are the job cgroups Spur and Slurm (cgroup/v2 plugin) create. Only
// the job level is read: cgroup v2 charges a job's steps to it already.
var layouts = []layout{
	{"spur", regexp.MustCompile(`^job_(\d+)_(\d+)$`)},
	{"system.slice/slurmstepd.scope", regexp.MustCompile(`^job_(\d+)$`)},
}

// Discover lists the job cgroups under root, one per job id, newest job first.
// A requeued Spur job keeps its highest attempt, as an earlier attempt's cgroup
// can linger. A job id already taken by an earlier layout comes back in shadowed:
// two series with the same labels would fail the whole scrape.
func Discover(root string) (ws, shadowed []Workload, err error) {
	byJob := map[string]Workload{}
	for _, l := range layouts {
		found, err := discoverLayout(root, l)
		if err != nil {
			return nil, nil, err
		}
		for id, w := range found {
			if _, taken := byJob[id]; taken {
				shadowed = append(shadowed, w)
				continue
			}
			byJob[id] = w
		}
	}
	ws = make([]Workload, 0, len(byJob))
	for _, w := range byJob {
		ws = append(ws, w)
	}
	sort.Slice(ws, func(i, j int) bool { return newerJob(ws[i].JobID, ws[j].JobID) })
	sort.Slice(shadowed, func(i, j int) bool { return shadowed[i].Path < shadowed[j].Path })
	return ws, shadowed, nil
}

func discoverLayout(root string, l layout) (map[string]Workload, error) {
	entries, err := os.ReadDir(filepath.Join(root, l.dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	byJob := map[string]Workload{}
	for _, e := range entries {
		m := l.re.FindStringSubmatch(e.Name())
		if !e.IsDir() || m == nil {
			continue
		}
		w := Workload{Path: "/" + l.dir + "/" + e.Name(), JobID: m[1]}
		if len(m) > 2 {
			w.attempt, _ = strconv.Atoi(m[2])
		}
		if cur, ok := byJob[w.JobID]; !ok || w.attempt > cur.attempt {
			byJob[w.JobID] = w
		}
	}
	return byJob, nil
}

// newerJob orders job ids numerically, highest (newest) first.
func newerJob(a, b string) bool {
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return a > b
}
