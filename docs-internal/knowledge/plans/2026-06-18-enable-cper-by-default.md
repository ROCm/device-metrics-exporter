# Enable CPER collection by default

- **Date:** 2026-06-18
- **Author:** bhatturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** ROCM-25398

## Context

CPER (Common Platform Error Record) fetching in DME was made opt-in (env var
`AMD_METRICS_EXPORTER_ENABLE_CPER`, OFF by default) as a guard against ROCM-25398
— a libamd_smi 26.4.0 invalid `free()` in `amdsmi_get_gpu_cper_entries` that crashed
gpuagent (SIGABRT → defunct zombie; exporter stays up but exports zero GPU metrics).

That crash is fixed in ROCm 7.14.0 / libamd_smi 26.5.0 (ROCm/rocm-systems#6677). With the
fix in place the guard is no longer needed, so CPER should be on by default to surface
GPU error records out of the box.

## Approach

- Leave `IsCperEnabled()` in `pkg/exporter/utils/cper_utils.go` **unchanged** — it stays
  opt-in (OFF unless `AMD_METRICS_EXPORTER_ENABLE_CPER` is `1`/`true`/`yes`/`on`). The code
  default is deliberately conservative so a stand-alone binary on an old libamd_smi is safe.
- Turn CPER on **for our shipped artifacts** by exporting `AMD_METRICS_EXPORTER_ENABLE_CPER=1`
  in the deployment environments, where the lib is guaranteed to be 7.14 / 26.5.0:
  - Release container images: `docker/Dockerfile.exporter-release`,
    `docker/Dockerfile.sriov.exporter-release`, `docker/Dockerfile.sriov.ub22.exporter-release`,
    `docker/Dockerfile.azure.linux3.exporter-release` (alongside the existing `ENV` block).
  - Debian package env files consumed by the systemd unit via `EnvironmentFile`:
    `debian/usr/local/etc/metrics/gpuagent.conf` and
    `debian-sriov/usr/local/etc/metrics/gpuagent.conf`.

### Alternatives considered

- Flip the `IsCperEnabled()` default to ON in code — rejected per review; a stand-alone
  binary should not assume a fixed libamd_smi version. Gating via the deployment env keeps
  the code default safe while still enabling CPER everywhere we ship the fixed lib.
- Drive the default from `config.json` instead of an env var — rejected; out of scope and
  larger surface area than warranted for a default flip.

## Scope

- **In scope:** the `AMD_METRICS_EXPORTER_ENABLE_CPER=1` env in release Dockerfiles and the
  Debian `gpuagent.conf` env files.
- **Out of scope:** `IsCperEnabled()` itself (unchanged), the libamd_smi fix (lands via the
  ROCm 7.14 bump), CPER health-check logic, config-schema changes.

## Validation

- Build: `make all` + `gofmt -l` clean in the build container.
- Manual / hardware: validated on 8× R9700S (Navi48/RDNA4) host with the 7.14 build
  (libamd_smi 26.5.0). With CPER enabled, the exporter's 30s refresh goroutine drove
  `amdsmi_get_gpu_cper_entries` for all 8 GPUs 256× over >6 min (past the ~5-min RDNA4
  crash window) — gpuagent survived (0 zombies), 1281 `amd_gpu_*` series kept flowing,
  no invalid `free()`. Confirms ROCM-25398 is fixed and CPER-on is safe.

## Risks and rollback

- Known risks: on a host still running the buggy libamd_smi 26.4.0, enabling CPER would
  re-expose the ROCM-25398 crash. Mitigation: the env is only set in our shipped artifacts
  that bundle the 7.14 / 26.5.0 lib; the code default stays opt-in for stand-alone use.
- Rollback: override `AMD_METRICS_EXPORTER_ENABLE_CPER=0` at runtime, or revert the env
  additions in the Dockerfiles / `gpuagent.conf`.
