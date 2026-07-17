# SR-IOV debian package versioned independently at 1.0.0-X

- **Date:** 2026-07-15
- **Author:** praveen
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** https://pensando.atlassian.net/browse/GPUOP-991

## Context

Split out from the GPUOP-991 docs bug to keep that PR docs-only. The
`amdgpu-exporter-sriov` Debian package (`debpkg-sriov` target) previously
shared `DEBIAN_VERSION`/`BUILD_VER_ENV` with the baremetal `amdgpu-exporter`
package, both derived from the CI `RELEASE` tag (e.g. `v1.5.1-3` or
`exporter-0.0.1-3` → `1.5.1-3` / `0.0.1-3`). The SR-IOV package should be
versioned independently on its own `1.0.0` line, while preserving the
release-label suffix (`-X`) CI appends for build tracking.

## Approach

- Added `DEBIAN_SRIOV_VERSION` to `Makefile`, derived from `DEBIAN_VERSION`
  by replacing the leading `X.Y.Z` version number with `1.0.0` via
  `sed -E 's/^[0-9]+\.[0-9]+\.[0-9]+/1.0.0/'` — this keeps whatever release
  label suffix `DEBIAN_VERSION` already carries (e.g. `1.5.1-3` → `1.0.0-3`,
  `0.0.1-3` → `1.0.0-3`, and a plain `1.5.1` → `1.0.0` with no suffix).
- Reusing `DEBIAN_VERSION`'s own derivation (instead of re-parsing `RELEASE`
  from scratch) means both `v1.5.1-X` and `exporter-0.0.1-X` CI formats are
  handled automatically, since `DEBIAN_VERSION` already normalizes both to
  `X.Y.Z[-label]`.
- Added `BUILD_SRIOV_VER_ENV` in `Makefile.package` (`DEBIAN_SRIOV_VERSION` +
  `~<ubuntu-version>`, mirroring `BUILD_VER_ENV`) and used it in
  `debpkg-sriov`'s control-file substitution instead of `BUILD_VER_ENV`.
- Added two new install docs (`docker-sriov.md`, `deb-package-sriov.rst`)
  documenting the SR-IOV docker image (`rocm/device-metrics-exporter-sriov`,
  now pinned `1.0.0`) and the `amdgpu-exporter-sriov` Debian package, wired
  into the Sphinx TOC, with cross-links from the existing baremetal
  `docker.md` / `deb-package.rst` pages.
- Both SR-IOV systemd units (`gpuagent-sriov.service`,
  `amd-metrics-exporter-sriov.service`) spelled out explicitly with correct
  enable/start/stop order; GIM driver (9.0.0.K+) reworded as a
  pre-installed prerequisite (not a platform config note); `--privileged`
  rationale documented for the docker image.
- Fixed the SR-IOV apt repo URL to
  `.../device-metrics-exporter/sriov/1.0.0` (was incorrectly copy-pasted
  from the non-SR-IOV `.../apt/1.0.0` path).
- Replaced the deb-package-sriov.rst config-omission line
  (`ExtraPodLabels`/`NICConfig`/`IFOEConfig`) with a pointer to the
  Hypervisor column in `configuration/metricslist.md`, the single source
  of truth for SR-IOV metric support.
- Added a Troubleshooting section to both SR-IOV install docs, and a new
  `docs/configuration/docker-sriov.md` (SR-IOV standalone container
  config), mirroring existing baremetal doc structure/patterns.
- Discovered and fixed `docs/sphinx/_toc.yml.in` had drifted out of sync
  with `_toc.yml` (missing entries) — likely a leftover template file from
  history; kept both in sync.
- Added `--privileged` to the baremetal `docs/installation/docker.md` /
  `docs/configuration/docker.md` `docker run` examples — it is mandatory,
  not just conditionally required for profiler metrics as previously
  documented.
- `docs/developerguide.md` referenced several Makefile targets removed by
  the gpuagent-submodule-removal work
  (`docs-internal/knowledge/plans/2026-06-16-gpuagent-source-build-in-docker.md`):
  `gpuagent-build`, `gpuagent-compile`, `gpuagent-compile-full`,
  `amdsmi-build`, `amdsmi-compile`. Rewrote those sections to reflect that
  gpuagent is cloned and compiled in-image by `make docker` (no host-side
  build step) and that amdsmi uses the current
  `amdsmi-compile-{rhel,ub22,ub24,all}` targets; also fixed the git
  submodule section (only `libgimsmi` remains a submodule) and a dead
  `git@github.com:ROCm/amdsmi.git` link.

### Alternatives considered

- A separate `sriov-v*` `RELEASE` tag format (previous attempt, reverted) —
  rejected per user direction: CI's actual `RELEASE` format is `v1.5.1-X` /
  `exporter-0.0.1-X`, with no `sriov-` prefix; the SR-IOV version must be
  derived from that same tag, not a new tag format.

## Scope

- **In scope:** `Makefile`, `Makefile.package`, `docs/installation/docker-sriov.md`,
  `docs/installation/deb-package-sriov.rst`, `docs/installation/docker.md`,
  `docs/installation/deb-package.rst`, `docs/configuration/docker.md`,
  `docs/configuration/docker-sriov.md`, `docs/developerguide.md`,
  `docs/sphinx/_toc.yml`, `docs/sphinx/_toc.yml.in`.
- **Out of scope:** GPUOP-991 docs fixes (Radeon claims, release notes) — kept
  on a separate branch/PR to stay docs-only.

## Validation

- Manual: verified `DEBIAN_VERSION`/`DEBIAN_SRIOV_VERSION`/`BUILD_VER_ENV`/
  `BUILD_SRIOV_VER_ENV` resolution with a standalone `make print-vars`
  harness across `RELEASE` unset, `v1.5.1-3`, `exporter-0.0.1-3`, `v1.5.1`
  (no label), and `nic-v1.2.0` — all resolve correctly and independently.
- `make docs-lint` and an actual `debpkg-sriov` build require the build
  container's TTY session; not run here.

## Risks and rollback

- Low risk — versioning-only Makefile change plus new docs pages; no
  runtime behavior change.
- Rollback: revert the commit(s) on this branch.
