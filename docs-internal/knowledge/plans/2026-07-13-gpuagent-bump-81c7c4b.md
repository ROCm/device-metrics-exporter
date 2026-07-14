# Bump gpuagent to ROCm/gpu-agent main @81c7c4b + refresh SR-IOV/gpuctl assets

- **Date:** 2026-07-13
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** fix/gpuop-880-gpuagent-81c7c4b
- **Related issue(s) / JIRA:** GPUOP-880, GPUOP-907

## Context

The pinned gpuagent commit (`ad0f7f10`) predates the MI350P clock-freq OOB
SIGSEGV fix. ROCm/gpu-agent `main` is now at `81c7c4bbd90` — "GPUOP-907: fix
gpuagent SIGSEGV on MI350P from clock freq OOB read (#75)", on top of the
SR-IOV/GIM coverage (#76) and the CVE go-toolchain bump (#77). This bump pulls
the latest main so the exporter ships the MI350P fix and current CVE-patched
deps.

## Approach

- Bump `GPUAGENT_COMMIT` to `81c7c4bbd90fe08f6b7effb8dfb4912d71b94570` in both
  places it is pinned:
  - `Makefile:127` (default var)
  - `docker/Dockerfile.exporter-release:17` (release image builds gpuagent
    from-source at this SHA)
- Drop the local `patch/gpuagent/0002-gpuop-907-clock-freq-oob.patch` — the
  GPUOP-907 clock-freq OOB fix is now upstream in `81c7c4bbd90`
  (`current_frequency_hz()` helper + `find_low_high_frequency()` clamp), so the
  patch no longer applies and is redundant (same pattern as the abseil patch
  dropped in #1447).
- Refresh the two **prebuilt** assets that are NOT built by the release
  Dockerfile, from a local build of the same commit:
  - `assets/gpuagent_sriov_static.bin.gz` — the GIM `gpuagent_gim` binary
    (stripped, tar+gzip, single entry named `gpuagent`).
  - `assets/gpuctl.gobin` — the `gpuctl` CLI (pure-Go, CVE-bumped deps).

### Alternatives considered

- Switch the SR-IOV image to build gpuagent from-source in-Dockerfile like the
  default image — rejected for this change; the SR-IOV path intentionally ships
  a prebuilt GIM static asset. Kept the prebuilt-asset model, just refreshed it.

## Scope

- **In scope:** gpuagent commit bump (both pins) + the two prebuilt assets.
- **Out of scope:** GIM smi-lib version (still 9.0.0.K); the CPER/edge-temp
  functional fix (separate branch `fix/gpuop-880-cper-edgetemp-followup`).

## Validation

- **Local build:** gpu-agent built clean at `81c7c4b` in the RHEL9 builder
  container (grpc/abseil rebuilt fully to avoid stale-lib link errors).
  - `gpuctl` version string → `main-81c7c4bbd90`.
  - GIM `gpuagent` version string → `build/gpuagent-81c7c4b-81c7c4bbd90`.
- **Image:** SR-IOV image rebuilt with the refreshed assets.
- **Hardware (leto MI210, GIM 9.0.0.K):** e2e — exporter comes up
  `deployment_mode="hypervisor"`, `amd_` series present, gpuctl talks to the
  live GIM agent, no sentinel leaks, container stable.

## Risks and rollback

- **Known risks:** a gpuagent commit bump can shift metric behavior; mitigated
  by the same GIM 9.0.0.K smi-lib and hardware e2e on leto.
- **Rollback:** revert this commit to restore the `ad0f7f10` pin and the prior
  assets. No config/schema migration involved.
