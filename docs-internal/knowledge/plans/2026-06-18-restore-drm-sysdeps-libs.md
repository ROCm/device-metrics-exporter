# Restore librocm_sysdeps_drm libs for amdsmi runtime dlopen

- **Date:** 2026-06-18
- **Author:** bhatturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** N/A

## Context

amdsmi 26.5.0 (ROCm build #83, 7.14.0a20260618) resolves consumer-GPU
board info (`card_model`, `vbios_version`) through a `dlopen()` of
`librocm_sysdeps_drm_amdgpu.so.1` at runtime. Because that lib is loaded
dynamically and is **not** a `DT_NEEDED` of `libamd_smi.so.26.5.0`, the
asset-sync logic — which only copies the DT_NEEDED netlink sysdeps
(`librocm_sysdeps_mnl`, `nl_3`, `nl_genl_3`) — silently dropped both
`librocm_sysdeps_drm.so.2` and `librocm_sysdeps_drm_amdgpu.so.1` from
`assets/amd_smi_lib/`.

Impact: deb/rpm installs on consumer GPUs (Navi48 / R9700S) report empty
`card_model` and `vbios_version`. The docker image masked the gap because it
also installs ROCm from the S3 tarball, which keeps the drm sysdeps on
`LD_LIBRARY_PATH` (`/opt/rocm-${ROCM_VERSION}/lib/rocm_sysdeps/lib`). The
installed deb/rpm `gpuagent.conf` only sets
`LD_LIBRARY_PATH=/usr/local/metrics/lib`, so it has no such fallback.

## Approach

- Restore the two drm sysdeps libs into all three OS asset dirs:
  `assets/amd_smi_lib/x86_64/{RHEL9,UBUNTU22,UBUNTU24}/lib/`.
- Source the exact build #83 binaries (matching the committed
  `libamd_smi.so.26.5.0` and netlink sysdeps) from the build #83 container
  `dme-build83` at `/opt/rocm/lib/rocm_sysdeps/lib/`. Verified by matching the
  container's `librocm_sysdeps_mnl.so.0` sha256 against the already-committed
  build #83 asset.
  - `librocm_sysdeps_drm.so.2` — 360336 B, `cf203bcf…`
  - `librocm_sysdeps_drm_amdgpu.so.1` — 251416 B, `5485dbf0…`
- The libs are OS-independent (byte-identical across OS dirs, like the other
  rocm_sysdeps libs), so one source populates all three dirs.

### Alternatives considered

- **Strip the drm libs (test/rocm-7.14-pkg-drm direction)** — rejected: the
  Radeon deb A/B test proved this regresses `card_model`/`vbios` on consumer
  GPUs.
- **Rely on plain `libdrm_amdgpu.so.1`** — rejected: amdsmi dlopens the
  `librocm_sysdeps_`-prefixed SONAME specifically; the plain libdrm does not
  satisfy it.
- **Use the stale `docker/rocm-local` (Jun 11) copies** — rejected: different
  tarball than build #83 (mnl sha differs); keep all assets from one tarball.

## Scope

- **In scope:** restoring the 6 drm sysdeps binaries into the asset tree.
- **Out of scope:** changing the asset-sync logic to auto-capture dlopen'd
  (non-DT_NEEDED) libs; any exporter/gpuagent code change.

## Validation

- Manual / hardware (Radeon host 10.7.121.210, 8× Navi48):
  - **Without** the drm libs: deb install → `card_model=""`,
    `vbios_version=""` on all 8 GPUs; gpuagent logs
    "Fail to open librocm_sysdeps_drm_amdgpu.so.1" (soft warning, not fatal).
  - **With** the drm libs copied into `/usr/local/metrics/lib` + restart:
    `card_model="AMD Radeon AI Pro R9700S"` ×8,
    `vbios_version="023.008.000.068.000001"`, warning gone.
  - Confirmed not a race: the concurrently-running container always reported
    `card_model` correctly.
- sha256 of restored libs verified equal to the build #83 container source.

## Risks and rollback

- Known risks: minimal — adds prebuilt binaries only; no code path changes.
  Slightly larger deb/rpm/image footprint (~0.6 MB).
- Rollback plan: revert the commit to remove the 6 libs.
