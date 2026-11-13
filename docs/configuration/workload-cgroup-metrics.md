# Workload cgroup metrics

Device Metrics Exporter can export CPU, memory and disk I/O usage for each Spur or Slurm job running on a node, read directly from the job's cgroup v2 files. CPU-only jobs are included; no GPU is needed.

## Support matrix

| | Supported |
| --- | --- |
| cgroup v2 (unified hierarchy) | Yes |
| cgroup v1 or hybrid | No. The exporter logs `cgroup v2 is required` and exports no workload cgroup metrics; GPU, NIC and IFOE metrics are unaffected. |
| Spur | Yes |
| Slurm with the `cgroup/v2` plugin | Yes |
| Debian package (systemd) | Yes |
| Standalone Docker container | Yes |
| Kubernetes (Helm) | No |
| Singularity | Not tested |

## Enabling

Pass `-monitor-cgroup=true`. It can run alone (`-monitor-gpu=false`) on CPU-only nodes.

### Debian package (systemd)

The exporter runs on the host, so it already sees the cgroup hierarchy and every process. Add the flag with a drop-in:

```bash
sudo systemctl edit amd-metrics-exporter
```

```ini
[Service]
ExecStart=
ExecStart=/usr/local/bin/amd-metrics-exporter -monitor-cgroup=true
```

```bash
sudo systemctl restart amd-metrics-exporter
```

### Standalone Docker container

```bash
docker run -d \
  --pid=host \
  -e NODE_NAME="$(hostname)" \
  -v /sys/fs/cgroup:/sys/fs/cgroup:ro \
  -v /var/run/exporter/:/var/run/exporter/ \
  -v ./config:/etc/metrics \
  -p 5000:5000 --name device-metrics-exporter \
  rocm/device-metrics-exporter:<tag> -monitor-gpu=false -monitor-cgroup=true
```

- The `/sys/fs/cgroup` mount makes the host cgroup hierarchy visible. `--cgroupns=host` works instead of the mount. With neither, the exporter sees only its own container and exports no job metrics.
- `NODE_NAME` sets the `hostname` label; without it the label is the container ID.
- `/var/run/exporter` carries the per-job files that provide the scheduler labels.
- `--pid=host` is needed only for `cgroup_memory_virtual_bytes`; without it that metric is not exported.

To collect GPU metrics in the same container, start from the command in [Standalone container configuration](docker.md) and add the options above, dropping `-monitor-gpu=false`.

## Which cgroups are read

| Path under `/sys/fs/cgroup` | Scheduler |
| --- | --- |
| `spur/job_<id>_<attempt>` | Spur |
| `system.slice/slurmstepd.scope/job_<id>` | Slurm with the `cgroup/v2` plugin |

Only the job-level cgroup is read; cgroup v2 already charges each job step to it. When a requeued Spur job has more than one attempt's cgroup present, only the highest attempt is exported. At most 1024 jobs are exported per node, keeping the newest job IDs. If the same job ID has a cgroup under both `spur/` and `system.slice/slurmstepd.scope/`, only the Spur one is exported and the exporter logs the other. Warnings about job cgroups, such as a cgroup that cannot be read, are logged once when they first appear rather than on every sample.

With `CommonConfig.HighResolution.Enable`, jobs are sampled every `FastPeriodSecs` and `/metrics` serves the latest sample. Otherwise each scrape reads the cgroups once; concurrent scrapes share that read.

## Metrics

| Metric | Type | Source | Description |
| --- | --- | --- | --- |
| `cgroup_cpu_usage_seconds_total` | counter | `cpu.stat` `usage_usec` | CPU time consumed |
| `cgroup_cpu_throttled_seconds_total` | counter | `cpu.stat` `throttled_usec` | Time spent throttled by the CPU quota |
| `cgroup_cpu_periods_total` | counter | `cpu.stat` `nr_periods` | CPU bandwidth enforcement periods |
| `cgroup_cpu_throttled_periods_total` | counter | `cpu.stat` `nr_throttled` | Periods in which the job was throttled |
| `cgroup_memory_current_bytes` | gauge | `memory.current` | Memory charged to the cgroup |
| `cgroup_memory_limit_bytes` | gauge | `memory.max` | Hard memory limit; absent when unlimited |
| `cgroup_memory_rss_bytes` | gauge | `memory.stat` `anon` | Anonymous (resident) memory |
| `cgroup_memory_swap_bytes` | gauge | `memory.swap.current` | Swap in use; absent when swap accounting is off |
| `cgroup_memory_working_set_bytes` | gauge | `memory.current` − `memory.stat` `inactive_file` | Working set |
| `cgroup_memory_virtual_bytes` | gauge | `VmSize` in `/proc/<pid>/status` | Sum of the job's processes' virtual memory; absent when no process is visible |
| `cgroup_memory_oom_kills_total` | counter | `memory.events` `oom_kill` | Processes killed by the OOM killer, including in job steps |
| `cgroup_tasks` | gauge | `pids.current` | Tasks (threads, not processes) in the job |
| `cgroup_io_read_bytes_total` | counter | `io.stat` `rbytes` | Bytes read from block devices, summed over devices |
| `cgroup_io_write_bytes_total` | counter | `io.stat` `wbytes` | Bytes written to block devices, summed over devices |
| `cgroup_io_reads_total` | counter | `io.stat` `rios` | Read operations, summed over devices |
| `cgroup_io_writes_total` | counter | `io.stat` `wios` | Write operations, summed over devices |

`CommonConfig.MetricsFieldPrefix` is prepended to every name.

A metric whose source file the kernel does not provide for a job, for example because a controller is not enabled for that subtree, is left out for that job rather than reported as `0`.

`cgroup_memory_virtual_bytes` needs the host PID namespace: from another PID namespace the kernel lists the job's processes as PID 0, so the metric is not exported. ROCm processes reserve very large virtual address ranges, so expect large values.

Disk I/O counts only block-device traffic. Network and parallel filesystems (NFS, Lustre, GPFS) are not included.

## Labels

| Label | Source |
| --- | --- |
| `hostname` | `NODE_NAME` if set, otherwise the host name |
| `job_id` | The cgroup path |
| `job_user`, `job_partition`, `job_account` | The per-job file written by the Slurm integration prolog |

The scheduler labels are filled when `/var/run/exporter/jobs/<job_id>` exists; otherwise they are empty. The bundled [Slurm integration](../integrations/slurm-integration.md) prolog writes that file for every job, including CPU-only jobs, and the epilog removes it. Spur passes job details to the prolog under `SPUR_*` names, which the bundled scripts also read. `job_account` is empty unless the scheduler exports `SLURM_JOB_ACCOUNT` or `SPUR_JOB_ACCOUNT` to the prolog. With `-enable-slurm-scl=false` the exporter does not read that file: the scheduler labels are still present on every series but always empty.

## Limitations

- Process counts are not exported; `cgroup_tasks` counts threads.
- Disk I/O is summed per job, not reported per device.
- Disk I/O metrics need the `io` controller enabled for the job cgroups, in the parent's `cgroup.subtree_control`.
- The kernel charges buffered writes to the cgroup that owns the file, so rewriting a file another job created can be counted against that job.
- Spur ends a job on its first OOM kill, so a scrape rarely sees `cgroup_memory_oom_kills_total` above 0 for a Spur job.
