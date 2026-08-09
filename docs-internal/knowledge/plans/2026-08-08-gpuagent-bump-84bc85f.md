# gpuagent bump to ROCm/gpu-agent main @84bc85f

- **Date:** 2026-08-08
- **Author:** praveen
- **Related PR(s):** (this PR)
- **Related issue(s) / JIRA:** N/A — dependency bump

## Context

Manually ran the bump logic that `.github/workflows/gpuagent-bump-check.yml`
automates, to pick up the latest `ROCm/gpu-agent` main commit in this PR
before the workflow itself has merged and can run on schedule.

`docker/Dockerfile.exporter-release`'s `GPUAGENT_COMMIT` was already at
upstream main HEAD (`84bc85f69e41c9b386dd82afa842987b3e549b00`), but the
`Makefile` default (`GPUAGENT_COMMIT ?=`) had drifted behind it, pinned to
`875d87be5797f4b3d899d5d685fa4862fe8379b5`. Also, gpu-agent's
`sw/nic/gpuagent/go.mod` now pins `go 1.25.12`, one patch ahead of this
repo's `ARG GO_VERSION=1.25.11`.

## Approach

- Bump `GPUAGENT_COMMIT` in `Makefile` from `875d87b...` to `84bc85f...`
  (brings it back in sync with `docker/Dockerfile.exporter-release`, which
  was already current).
- Bump `GO_VERSION` 1.25.11 -> 1.25.12 and `GO_SHA256` to match, in
  `docker/Dockerfile.exporter-release`.

### Alternatives considered

N/A — mechanical dependency bump, no design choice involved.

## Scope

- **In scope:** `GPUAGENT_COMMIT` pin (both locations) + `GO_VERSION`/`GO_SHA256`.
- **Out of scope:** any other asset (amdsmi, ROCm version, vendor patches). If
  this gpuagent commit needs a vendor patch refresh or breaks the build, that
  is a follow-up, not covered here.

## Validation

- **Automated:** none — version pins only, no exporter code changed.
- **Required before merge:** `make docker` (or the `/builder` skill) builds
  clean at the new commit/Go version.

## Risks and rollback

- **Known risks:** a gpuagent commit bump can change metric behavior, add new
  dependencies, or require a vendor patch refresh. Do not merge without a
  successful build.
- **Rollback:** revert this commit; `GPUAGENT_COMMIT` and `GO_VERSION`/`GO_SHA256`
  return to the prior pinned values.
