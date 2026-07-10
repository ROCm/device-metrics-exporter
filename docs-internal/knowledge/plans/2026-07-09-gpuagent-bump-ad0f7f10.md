# gpuagent: bump to main HEAD ad0f7f10, drop upstreamed abseil patch

- **Date:** 2026-07-09
- **Author:** praveen
- **Related PR(s):** #1447
- **Related issue(s) / JIRA:** ROCm/gpu-agent#74, ROCm/gpu-agent#77

## Context

GPUAGENT_COMMIT was pinned at `9af0bf5b` (2026-06-12). Two commits have landed
on upstream main since then:

- `22ec8211` — abseil `--start-group`/`--end-group` linker fix (#74)
- `ad0f7f10` — CVE fix: go 1.25.11→1.25.12, `x/net` v0.55.0, `x/sys` v0.44.0 (#77),
  closing 10 trivy findings (4 HIGH, 3 MEDIUM, 3 UNKNOWN) in `gpuctl`

`patch/0001-abseil-link-start-group.patch` was introduced locally because the RHEL9
linker required explicit archive grouping for gRPC/abseil static libs. That fix is now
native in upstream (`22ec8211`), making the local patch redundant.

## Approach

- Advance `GPUAGENT_COMMIT` from `9af0bf5b` → `ad0f7f10` in `Makefile` and
  `docker/Dockerfile.exporter-release`.
- Delete `patch/gpuagent/0001-abseil-link-start-group.patch`.
- Retain `patch/gpuagent/0002-gpuop-907-clock-freq-oob.patch` — ROCm/gpu-agent#75
  is still open.

### Alternatives considered

- Stay on `9af0bf5b` — leaves 10 CVEs open in `gpuctl` and requires maintaining
  a patch that is already upstream.

## Scope

- **In scope:** commit pin bump, patch removal.
- **Out of scope:** amdsmi update, SRIOV/mock asset changes, gpuagent API changes.

## Validation

- CI `gpuagent-build` Docker stage: shallow clone at `ad0f7f10`, `0002` patch
  applies cleanly, binaries build and strip without error.
- No Go source changes; unit test suite unaffected.

## Risks and rollback

- Both new upstream commits are build/CVE-only with no gpuagent API changes — low risk.
- Rollback: revert `GPUAGENT_COMMIT` to `9af0bf5b` and restore `0001` patch.
- If `0002` (clock-freq OOB) merges upstream before this PR lands, a follow-up
  removes it.
