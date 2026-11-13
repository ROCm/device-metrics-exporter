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

import "github.com/prometheus/client_golang/prometheus"

// Field is a metric field name; it doubles as the snapshot-store field key.
type Field string

const (
	CPUUsageSeconds     Field = "cgroup_cpu_usage_seconds_total"
	CPUThrottledSeconds Field = "cgroup_cpu_throttled_seconds_total"
	CPUPeriods          Field = "cgroup_cpu_periods_total"
	CPUThrottledPeriods Field = "cgroup_cpu_throttled_periods_total"
	MemoryCurrentBytes  Field = "cgroup_memory_current_bytes"
	MemoryLimitBytes    Field = "cgroup_memory_limit_bytes"
	MemoryRSSBytes      Field = "cgroup_memory_rss_bytes"
	MemorySwapBytes     Field = "cgroup_memory_swap_bytes"
	MemoryWorkingSet    Field = "cgroup_memory_working_set_bytes"
	MemoryVirtualBytes  Field = "cgroup_memory_virtual_bytes"
	MemoryOOMKills      Field = "cgroup_memory_oom_kills_total"
	Tasks               Field = "cgroup_tasks"
	IOReadBytes         Field = "cgroup_io_read_bytes_total"
	IOWriteBytes        Field = "cgroup_io_write_bytes_total"
	IOReads             Field = "cgroup_io_reads_total"
	IOWrites            Field = "cgroup_io_writes_total"
)

type FieldInfo struct {
	Field     Field
	Help      string
	ValueType prometheus.ValueType
}

// AllFields lists every field in export order.
var AllFields = []FieldInfo{
	{CPUUsageSeconds, "Total CPU time consumed by the workload cgroup", prometheus.CounterValue},
	{CPUThrottledSeconds, "Total time the workload cgroup was CPU throttled", prometheus.CounterValue},
	{CPUPeriods, "Number of CPU bandwidth enforcement periods", prometheus.CounterValue},
	{CPUThrottledPeriods, "Number of CPU bandwidth periods that were throttled", prometheus.CounterValue},
	{MemoryCurrentBytes, "Memory currently charged to the workload cgroup", prometheus.GaugeValue},
	{MemoryLimitBytes, "Hard memory limit of the workload cgroup; absent when unlimited", prometheus.GaugeValue},
	{MemoryRSSBytes, "Anonymous (resident) memory of the workload cgroup", prometheus.GaugeValue},
	{MemorySwapBytes, "Swap used by the workload cgroup; absent when swap accounting is off", prometheus.GaugeValue},
	{MemoryWorkingSet, "Memory charged to the workload cgroup minus inactive file cache", prometheus.GaugeValue},
	{MemoryVirtualBytes, "Sum of virtual memory (VmSize) of the workload's processes; needs the host PID namespace", prometheus.GaugeValue},
	{MemoryOOMKills, "Processes in the workload cgroup killed by the OOM killer", prometheus.CounterValue},
	{Tasks, "Tasks (threads) in the workload cgroup", prometheus.GaugeValue},
	{IOReadBytes, "Bytes read from block devices by the workload cgroup", prometheus.CounterValue},
	{IOWriteBytes, "Bytes written to block devices by the workload cgroup", prometheus.CounterValue},
	{IOReads, "Read operations on block devices by the workload cgroup", prometheus.CounterValue},
	{IOWrites, "Write operations on block devices by the workload cgroup", prometheus.CounterValue},
}
