# GPUOP-832: Fix Debian package file conflict with libdrm-common (amdgpu.ids)

- **Date:** 2026-05-30
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-832

## Context

Commit `98d0010ba` ("fix(metrics): resolve card_model on consumer GPUs via
amdgpu.ids") added `/usr/share/libdrm/amdgpu.ids` to the Debian packages
(`amdgpu-exporter` and `amdgpu-exporter-sriov`). On Ubuntu hosts where the
amdgpu driver was installed via `amdgpu-install`, that path is already owned by
the `libdrm-common` system package, so `dpkg -i` fails:

```
trying to overwrite '/usr/share/libdrm/amdgpu.ids',
which is also in package libdrm-common 2.4.113-2~ubuntu0.22.04.1
```

This breaks all 7 exporter-debian sanity tests in CI (jobd 31307596) and blocks
standalone `.deb` deploys on bare-metal hosts that have the GPU driver
installed. K8s/OpenShift container deployments are unaffected because they ship
the file inside the image.

The runtime need (libdrm/amdsmi looking up the marketing name for consumer GPUs)
is satisfied as long as `amdgpu.ids` exists at `/usr/share/libdrm/amdgpu.ids` —
the source of the file doesn't have to be our package.

## Approach

- Stop bundling `amdgpu.ids` in `debpkg` and `debpkg-sriov` targets in
  `Makefile.package`. Remove the `mkdir -p .../usr/share/libdrm` +
  `fetch-amdgpu-ids.sh` invocations and the matching cleanup.
- Add `Depends: libdrm-common` to both `debian/DEBIAN/control` and
  `debian-sriov/DEBIAN/control` so apt installs the system package that owns
  the file (Ubuntu 22.04 / 24.04 both have it).
- Container (RPM and OCI image) builds are untouched — they continue to
  bundle `amdgpu.ids` because those targets don't conflict with a host package.

### Alternatives considered

- **`--force-overwrite` workaround** — only viable as a manual escape hatch
  for users, not a default install path. Rejected.
- **`dpkg-divert` / `Replaces: libdrm-common`** — would let our package take
  ownership of a file in a core driver package. Brittle, surprising, and
  ROCm/libdrm-common upgrades could re-trigger the conflict. Rejected.
- **Install to an alternate path (e.g. `/usr/share/amd-metrics-exporter/`)** —
  libdrm/amdsmi look in `/usr/share/libdrm/amdgpu.ids` by default; we'd
  either symlink (still conflicts) or duplicate code paths. Rejected.

## Scope

- **In scope:** Debian + Debian-SRIOV packaging (`debpkg`, `debpkg-sriov`).
- **Out of scope:**
  - RPM packaging — the spec also installs `/usr/share/libdrm/amdgpu.ids`
    and would conflict with the system `libdrm` package on RHEL hosts that
    have it installed. Not reported in GPUOP-832, but a symmetric fix is
    likely warranted as a follow-up.
  - Docker/OCI image — file is correctly bundled there (no host conflict).
  - Code paths in `pkg/amdgpu/gpuagent/gpuagent_gpu_metrics.go` are unchanged.

## Validation

- Re-run `make debpkg` and `make debpkg-sriov`; inspect that the produced
  `.deb` files do not contain `usr/share/libdrm/amdgpu.ids` via
  `dpkg-deb -c bin/amdgpu-exporter_*.deb | grep amdgpu.ids` (expected: no
  output).
- Confirm `Depends:` is set: `dpkg-deb -f bin/amdgpu-exporter_*.deb Depends`.
- Install on the GPUOP-832 reporter's host (MI210, Ubuntu 22.04 with
  amdgpu-install + ROCm 7.2.1):
  - `sudo apt install -y ./amdgpu-exporter_*.deb` — succeeds (no overwrite
    error).
  - `dpkg -L libdrm-common | grep amdgpu.ids` shows the file is still present.
  - Run exporter, confirm `card_model` label resolves for Navi consumer GPUs.
- Re-run the `exporter-debian-pytest-sanity` jobd target — all 7 tests should
  pass.

## Risks and rollback

- **Risk:** A user installing the `.deb` on a system without `libdrm-common`
  available in apt sources would now fail at install time. Mitigation:
  `libdrm-common` is in Ubuntu main and is a transitive dep of any amdgpu
  stack; the exporter on bare metal also requires the amdgpu driver, which
  pulls `libdrm-common` regardless.
- **Risk:** On a host where `libdrm-common` is not installed at all, marketing
  name resolution for consumer GPUs falls back to `CardSeries` (the code path
  added in 98d0010ba already handles this gracefully).
- **Rollback:** Revert this PR; the workaround
  `sudo dpkg -i --force-overwrite amdgpu-exporter_*.deb` remains available.
