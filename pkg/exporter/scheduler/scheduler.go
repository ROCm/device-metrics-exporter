/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scheduler

type SchedulerType int

const (
	Kubernetes SchedulerType = iota + 1
	Slurm
)

type Workload struct {
	Type SchedulerType
	Info interface{}
}

type SchedulerClient interface {
	// List of JobInfo/PodResourceInfo map
	ListWorkloads() (map[string]Workload, error)
	CheckExportLabels(labels map[string]bool) bool
	Close() error
	Type() SchedulerType
}

type PodResourceInfo struct {
	Pod       string
	Namespace string
	Container string
}

type JobInfo struct {
	Id        string
	User      string
	Partition string
	Cluster   string
}

func (s SchedulerType) String() string {
	return [...]string{"Kubernetes", "Slurm"}[s-1]
}
func GetExportLabels(t SchedulerType) map[string]bool {
	switch t {
	case Kubernetes:
		return KubernetesLabels
	default:
		return SlurmLabels
	}
}
