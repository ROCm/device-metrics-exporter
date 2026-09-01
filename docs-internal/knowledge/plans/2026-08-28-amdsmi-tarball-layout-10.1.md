# amdsmi tarball layout tolerance for ROCm 10.1

- **Date:** 2026-08-28
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-1067

## Context

The nightly CDN moved from ROCm 10.0.0 GA to 10.1.0, which relocates
`libamd_smi.so` from `share/amd_smi/amdsmi/` to `lib/`. The DME runtime image
build breaks on the new layout.

Two runtime-stage consumers assume the old `share/` layout:

1. `docker/Dockerfile.exporter-release` — the tarball-mode runtime override
   hardcodes `src=/opt/rocm-${ROCM_VERSION}/share/amd_smi/amdsmi/libamd_smi.so`.
   On 10.1 that path is gone → `cp` fails → build dies.
2. `docker/install-rocm-tarball.sh` — the exporter prune profile keeps
   `share/amd_smi` wholesale but drops `lib/libamd_smi.*` (it was only in the
   testrunner keep-list). On 10.1 the real `.so` is in `lib/`, so the prune
   removes it entirely.

The gpuagent-build stage was already layout-tolerant (it extracts by filename
via `tar --no-anchored 'libamd_smi.so*'`), which is why the earlier ROCm-10.1
amdsmi API-migration A/B (which only exercised the build stage) did not catch
this. This is a packaging-layout regression in the runtime stage, distinct from
that API break.

The ROCm `share/ → lib/` move is still landing upstream (open PR), so both
layouts are in flight depending on tarball date.

## Approach

Make both consumers layout-tolerant rather than flipping the path:

- **Dockerfile runtime override:** resolve the source `.so` from `lib/` first,
  fall back to `share/amd_smi/amdsmi/`; hard-error if neither exists. Use
  `cp -aL` to dereference the `lib/` symlink into a real file at the dest.
- **install-rocm-tarball.sh:** move `libamd_smi.*` from `TESTRUNNER_PATTERNS`
  into `COMMON_PATTERNS` so every profile keeps the `lib/` copy through the
  prune.

### Alternatives considered

- Flip the hardcoded path to `lib/` only — rejected: breaks 10.0.0 GA and any
  older tarball still in use while the ROCm PR is open.

## Scope

- **In scope:** exporter runtime image tarball path handling (Dockerfile +
  install-rocm-tarball.sh).
- **Out of scope:** the gpuagent amdsmi API migration (already merged); bumping
  the repo's default `ROCM_VERSION`/`ROCM_TARBALL_URL` pin.

## Validation

- Manual: build the exporter image with a 10.1 nightly tarball
  (`AMDSMI_FROM_TARBALL`/`ROCM_TARBALL_URL`), confirm the override resolves
  `libamd_smi.so` from `lib/`, image builds, container comes up and serves
  metrics.
- Regression: confirm 10.0.0 GA tarball still builds via the `share/` fallback.

## Risks and rollback

- Known risks: low — the fallback preserves old-layout behavior; the added
  hard-error surfaces a missing lib clearly instead of a confusing `cp` failure.
- Rollback plan: revert the two-file diff.
