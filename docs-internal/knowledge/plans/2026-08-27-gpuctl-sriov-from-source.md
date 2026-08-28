# Stage SR-IOV gpuctl from the shared source producer

- **Date:** 2026-08-27
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** N/A

## Context

The SR-IOV build paths staged `gpuctl` from the committed `assets/gpuctl.gobin`
blob, while every non-SR-IOV path and the SR-IOV `.deb` path already build it
from the shared gpuagent source producer (`build/gpuagent/`). This left the
SR-IOV `.rpm` and docker paths inconsistent: they shipped a prebuilt binary that
could drift from the pinned gpuagent source, and required carrying a large binary
blob in the tree.

## Approach

Gate SR-IOV gpuctl staging on `GPUAGENT_FROM_SOURCE` so the default path uses the
producer output, matching the existing `.deb` behavior; remove the committed
blob.

- `Makefile.package` (rpm-sriov): copy `gpuctl` from `$(GPUAGENT_BUILD_DIR)` when
  `GPUAGENT_FROM_SOURCE=1`, else fall back to `gpuctl.gobin`.
- `docker/build_prep_docker.sh` (docker sriov path): stage `gpuctl` from the
  producer when `GPUAGENT_FROM_SOURCE=1`, else the blob. Mock reuses its staged
  mock gpuagent as the gpuctl stand-in (mock skips the ROCm tarball and never
  exercises gpuctl).
- `Makefile`: flip `docker-sriov-ub22` off the `GPUAGENT_FROM_SOURCE=0` pin to the
  shared producer, consistent with `docker-sriov`.
- Remove `assets/gpuctl.gobin` and its `assets/version.yaml` entry.

The `GPUAGENT_FROM_SOURCE=0` fallbacks remain so the collab branch, which re-adds
its own prebuilt blob, keeps working.

### Alternatives considered

- Keep the committed blob for SR-IOV only — rejected: perpetuates drift between
  the shipped gpuctl and the pinned gpuagent source, and keeps a large binary in
  the tree.

## Scope

- **In scope:** SR-IOV gpuctl staging (rpm + docker + ub22), removal of the
  committed blob.
- **Out of scope:** gpuagent source pin, ROCm version, any runtime/exporter
  behavior. gpuctl output is unchanged; only its build provenance changes.

## Validation

- Build: SR-IOV image built with `GPUAGENT_FROM_SOURCE=1`; build log shows
  `Staging sriov gpuctl from source producer`. gpuctl inside the image is
  byte-identical to the producer output (`build/gpuagent/gpuctl`).
- Hardware e2e: on an 8x MI300X GIM host, ran the from-source gpuctl against a
  live gpuagent over a VF. `show gpu --summary` reports 8 GPUs; `show gpu all`
  and `show gpu statistics` return full firmware/clock/PCIe/power data. Host-side
  GIM sentinels (VRAM/voltage/edge-temp) present as expected and suppressed to NA
  by the exporter.

## Risks and rollback

- Known risks: a build that sets `GPUAGENT_FROM_SOURCE=0` without re-supplying a
  prebuilt gpuctl would have no SR-IOV gpuctl; the collab branch supplies its own
  blob, so this only affects that configuration.
- Rollback: revert the commit to restore `gpuctl.gobin` and the unconditional
  blob-staging path.
