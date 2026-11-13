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

package scheduler

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadJobFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("42", `{"SLURM_JOB_ID": "42", "SLURM_JOB_USER": "alice", "SLURM_JOB_PARTITION": "gpu",
		"SLURM_JOB_ACCOUNT": "research", "SLURM_CLUSTER_NAME": "c1"}`)
	write("43", `{"SLURM_JOB_ID": "99"}`)
	write("44", `not json`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "secret"), []byte(`{"SLURM_JOB_ID": "../secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := ReadJobFile(dir, "42")
	want := JobInfo{Id: "42", User: "alice", Partition: "gpu", Account: "research", Cluster: "c1"}
	if !ok || got != want {
		t.Errorf("ReadJobFile(42) = %+v, %v; want %+v", got, ok, want)
	}
	for _, id := range []string{"43", "44", "45", "", "..", "../secret"} {
		if got, ok := ReadJobFile(dir, id); ok {
			t.Errorf("ReadJobFile(%q) = %+v; want not found", id, got)
		}
	}
}

// runProlog runs the bundled prolog with env, writing under a temp dir instead
// of /var/run/exporter, and returns that dir.
func runProlog(t *testing.T, env []string) string {
	t.Helper()
	src, err := os.ReadFile("../../../debian/usr/local/etc/metrics/slurm/slurm-prolog.sh")
	if err != nil {
		t.Fatal(err)
	}
	const exportDir = `EXPORT_DIR="/var/run/exporter/"`
	if !bytes.Contains(src, []byte(exportDir)) {
		t.Fatalf("prolog no longer sets %s", exportDir)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "prolog.sh")
	src = bytes.Replace(src, []byte(exportDir), []byte(`EXPORT_DIR="`+dir+`/"`), 1)
	if err := os.WriteFile(script, src, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script)
	cmd.Env = append([]string{"PATH=/usr/bin:/bin"}, env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prolog: %v\n%s", err, out)
	}
	return dir
}

func TestPrologEscapesJobFields(t *testing.T) {
	for _, account := range []string{`x","SLURM_JOB_USER":"alice`, `back\slash`, "plain"} {
		dir := runProlog(t, []string{
			"SLURM_JOB_ID=7", "SLURM_JOB_USER=bob", "SLURM_JOB_PARTITION=gpu",
			"SLURM_JOB_ACCOUNT=" + account, "SLURM_JOB_GPUS=1",
		})
		want := JobInfo{Id: "7", User: "bob", Partition: "gpu", Account: account}
		if got, ok := ReadJobFile(filepath.Join(dir, "jobs"), "7"); !ok || got != want {
			t.Errorf("account %q: job file = %+v, %v; want %+v", account, got, ok, want)
		}
		buf, err := os.ReadFile(filepath.Join(dir, "1"))
		if err != nil {
			t.Fatal(err)
		}
		var env map[string]string
		if err := json.Unmarshal(buf, &env); err != nil {
			t.Fatalf("account %q: GPU file is not JSON: %v\n%s", account, err, buf)
		}
		if got := jobInfoFromEnv(env); got != want {
			t.Errorf("account %q: GPU file = %+v; want %+v", account, got, want)
		}
	}
}
