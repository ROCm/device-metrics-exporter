# Cgroup workload metrics: per-job CPU, memory and disk I/O

- **Date:** 2026-10-30
- **Author:** AMD Device Metrics Exporter team
- **Related PR(s):** #1706
- **Related issue(s) / JIRA:** SPUR-363

## Context

Batch schedulers need per-job CPU, memory and disk I/O on the same Prometheus
endpoint as GPU metrics, labelled with the job's identity. DME exports no host
resource metrics today, and Slurm job labels exist only for GPU jobs.
node_exporter cannot attribute usage to a job.

## Approach

### Cgroup reader

- Read the cgroup v2 interface files directly (`pkg/cgroup`): `cpu.stat`,
  `memory.current`, `memory.max`, `memory.swap.current`, `memory.stat`,
  `memory.events` (OOM kills), `pids.current` (tasks) and `io.stat`. No new
  dependencies.
- A file the kernel does not provide (controller off for the subtree, swap
  accounting off) leaves its fields out for that job instead of reporting 0.
  `memory.max` of `max` leaves the limit out.
- cgroup v2 only. `cgroup.controllers` at the root identifies v2; on v1 or
  hybrid hosts the agent logs that v2 is required and exports nothing.
- Virtual memory sums `VmSize` from `/proc/<pid>/status` over the job
  subtree's `cgroup.procs`. Exported only when at least one PID resolves: from
  another PID namespace the kernel lists every PID as 0.

### Which cgroups

- Two fixed layouts, no configuration:
  - Spur: `/sys/fs/cgroup/spur/job_<id>_<attempt>`
  - Slurm `cgroup/v2`: `/sys/fs/cgroup/system.slice/slurmstepd.scope/job_<id>`
- Only the job level is read; v2 charges steps to it.
- One series per job id: a requeued Spur job keeps its highest attempt.
- At most 1024 jobs per node, keeping the highest (newest) job ids.

### Labels

- `hostname`, `job_id`, `job_user`, `job_partition`, `job_account`, fixed.
- User, partition and account come from `/var/run/exporter/jobs/<job_id>`,
  read on every publish (no cache to go stale when a requeue reuses the id).
- The prolog writes that file for every job (atomic rename) and the epilog
  removes it. Per-GPU files are unchanged, so CPU-only jobs become visible
  without touching the GPU path. The scripts fall back to `SPUR_*` variables;
  the job account reaches Spur hooks with a companion Spur change.
- New `JOB_ACCOUNT` label (`MetricLabel` = 15), mandatory on GPU metrics like
  the other job labels.

### Store and publishing

- New `MetricsInterface` client (`pkg/cgroup/cgroupagent`), store-served
  (`NeedsPerScrapePull() == false`), device type `CGROUP`.
- One store slot per job, key `cgroup:<path>`.
- New `Store.ReplaceOwned(prefix, slots)`: one clone-and-swap of the device
  map that sets the owner's slots to exactly the new set (fields set, not
  merged), drops the owner's other slots, and keeps everyone else's. A scrape
  sees the old set or the new one, never a mix; ended jobs and dropped fields
  disappear without the agent tracking what it published, so a reload's
  `Store.Reset` cannot leave that bookkeeping stale.
- Pull mode (default): one read per scrape, concurrent scrapes deduped.
- High-res mode (`CommonConfig.HighResolution.Enable`): a feed publishes every
  `FastPeriodSecs` and scrapes serve the store. The mode flag is an
  `atomic.Bool`, so a scrape never waits on a publish.

### Configuration

- CLI `-monitor-cgroup` (default false). `main.go` accepts it as the only
  enabled monitor, so CPU-only nodes work.
- No config section: layouts, fields and the job cap are fixed.
- Deployment: Debian/systemd and standalone Docker are supported. Kubernetes
  is not (no Helm toggle). Singularity is untested.

### Alternatives considered

- **cAdvisor library (`cadvisor/lib`):** prototyped first. Rejected: about
  26k vendored lines (cadvisor, godbus, opencontainers, coreos) and a grpc
  bump for a handful of small files; `manager.Stop` does not stop its per-cgroup
  goroutines, so it could never be restarted on reload; it logs through klog.
- **Configurable path patterns and include/exclude filters:** more general,
  but Spur and Slurm are the only consumers; add when a third layout appears.
- **node_exporter textfile plus a writer daemon:** extra component.
- **Scheduler exports the metrics:** per-scheduler duplication.

## Scope

- **In scope:** reader, pull and high-res modes, Spur and Slurm layouts,
  scheduler labels, `JOB_ACCOUNT`, CPU-only startup, docs, tests.
- **Out of scope (later):** Kubernetes, cgroup v1, per-device I/O, process
  counts (`cgroup_tasks` counts threads), PSI, other `memory.events`, a
  collector health metric (v1 is logged at startup, and it would not catch a
  container without the host cgroup namespace), per-job disk space.
- **TODO:** GPU and cgroup high-res feeds share one store, and the GPU
  high-res collector scans every cgroup slot per scrape; measure the combined
  cost before tightening `FastPeriodSecs`.

## Test plan

Unit tests run in CI. Live tests run on a cgroup v2 node with the exporter
built from the PR branch.

| Area | Test | Pass when |
|---|---|---|
| Regression | `-monitor-cgroup=false` (default), diff GPU `/metrics` against main | Only difference is the new `job_account` label |
| Regression | Slurm/Spur GPU job with an account | GPU series carry `job_account=<account>` |
| Correctness | Running job; compare each metric with its cgroup file | Equal for gauges at the same instant; counters within one scrape interval of the file |
| Correctness | Job ends | Its series are gone on the next scrape (pull) or tick (high-res) |
| Correctness | Requeued Spur job, two attempts' cgroups present | One series per metric, from the highest attempt |
| Correctness | Job without `memory.max` limit / swap accounting off | Limit / swap series absent, other metrics present |
| Reload | Toggle high-res, change `MetricsFieldPrefix` (there are no cgroup fields, patterns or caps to change) | No duplicate or stale series; names follow the new prefix (unit: `TestPrefixChangeOnReload`, `TestFeedScrapesAndReloadTogether`) |
| Deployment | Debian package with the systemd drop-in | Metrics present with labels |
| Deployment | Docker with the documented flags; without `--pid=host` | All metrics; virtual memory absent without `--pid=host` |
| Deployment | cgroup v1 host | Exporter keeps running, logs `cgroup v2 is required`, no `cgroup_*` series |
| Concurrency | `go test -race`: feed, scrapes and reloads together (`TestFeedScrapesAndReloadTogether`, `TestReplaceOwnedIsAtomicForReaders`) | No race reported, final scrape has exactly the live jobs |
| High-res | Injected ticker (`TestHighResolutionFeedOwnsTheStore`) | Feed updates the store; a scrape does not read cgroups |
| Naming | Unit tests assert `amd_cgroup_*` names | Pass |
| Performance | `BenchmarkPublish` (1/10/100/1000 jobs); on-node scrape latency, exporter CPU and RSS with and without `-monitor-cgroup` | Proposed: pull-mode scrape p99 added at most 100 ms at 100 jobs and 1 s at 1000 jobs; exporter CPU added at most 5% of one core at 100 jobs in high-res at 1 s; RSS added at most 50 MB at 1000 jobs |

## Risks and rollback

- Publish cost grows linearly with jobs (one directory listing plus eight small
  reads per job, and a `/proc` walk for virtual memory). In the build
  container's temp filesystem it was about 0.3 ms per job; real cgroupfs is
  in-memory. Bounded by the 1024 cap.
- Rollback: disabled by default; revert the PR.
