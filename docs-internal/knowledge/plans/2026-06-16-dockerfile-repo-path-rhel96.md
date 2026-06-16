# Fix Dockerfile amdgpu/graphics repo path: rhel/9.8 -> rhel/9.6

## Problem

PR #1395 (`2f71284a8`) bumped the container base image from RHEL 9.6 to 9.8.
In addition to `BASE_IMAGE` / `LABEL OS`, it also changed the `repo.radeon.com`
`baseurl` directory to `rhel/9.8`. repo.radeon.com does **not** publish a
`rhel/9.8` tree for these versions, so `microdnf` gets a 404 at build time.

Confirmed via HTTP HEAD on repo.radeon.com:

| Path | Status |
|---|---|
| `amdgpu/6.4.1/rhel/9.6/main/x86_64/` | 200 |
| `amdgpu/6.4.1/rhel/9.8/main/x86_64/` | 404 |
| `graphics/7.2/rhel/9.6/main/x86_64/` | 200 |
| `graphics/7.2/rhel/9.8/main/x86_64/` | 404 |

Impact:
- **SR-IOV exporter** (`Dockerfile.sriov.exporter-release`) has no ROCm-tarball
  bypass, so it always hits the amdgpu repo -> 404 -> `docker-sriov` CI fails.
- **Standard exporter** (`Dockerfile.exporter-release`) is masked because the
  Makefile sets `ROCM_TARBALL_URL` (tarball branch skips repo.radeon.com), but
  the `else` branch still carries the 9.8 landmine.

## Decision

Keep `BASE_IMAGE` / `LABEL OS` at `9.8` (the UBI minimal 9.8 image is valid),
but make the repo config correct for each image.

## Changes

- `docker/Dockerfile.sriov.exporter-release`: **remove the amdgpu + rocm repo
  config entirely.** The sriov image installs only base UBI/RHEL packages and
  gets amd-smi from the `ADD`'d `libgim_amd_smi.so` — it never installs an
  amdgpu/rocm package, so the repos were dead weight whose only effect was a
  `microdnf update` 404. This makes the sriov build independent of which
  `rhel/9.x` tree repo.radeon.com publishes.
- `docker/Dockerfile.exporter-release`: `graphics/.../rhel/9.8` -> `rhel/9.6`
  (the non-tarball `else` branch — the standard exporter genuinely installs
  amd-smi-lib / libdrm-amdgpu / rocprofiler-sdk from these repos).

## Validation

- `grep -rn 'rhel/9\.8' docker/` returns no matches after the change.
- Built the real sriov Dockerfile: the `microdnf` layer completes, installing
  all base packages from `ubi-9-baseos`/`ubi-9-appstream` with no
  repo.radeon.com fetch and no 404.
