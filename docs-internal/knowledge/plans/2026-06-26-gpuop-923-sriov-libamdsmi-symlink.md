# GPUOP-923 — ship libamdsmi.so symlink for SR-IOV gpuagent

- **Date:** 2026-06-26
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** fix/gpuop-923-sriov-libamdsmi
- **Related issue(s) / JIRA:** GPUOP-923

## Context

`gpuagent-sriov` links the gim SMI library, which has soname `libamdsmi.so`
(recorded as a `DT_NEEDED` entry in the binary). The deb and rpm packages,
however, only shipped the file under its real name `libgim_amd_smi.so`. The
dynamic loader resolves `DT_NEEDED` by filename, so at startup it found no
`libamdsmi.so` on the library path and the gpuagent-sriov service crash-looped
with exit code 127 (unresolved shared library).

## Approach

Create a `libamdsmi.so -> libgim_amd_smi.so` symlink in
`/usr/local/metrics/lib` (already on the `gpuagent.conf` `LD_LIBRARY_PATH`) for
both packaging paths:

- `Makefile.package` — `ln -sf libgim_amd_smi.so $(PKG_SRIOV_LIB_PATH)/lib/libamdsmi.so` in the deb staging step.
- `rpmbuild/amdgpu-exporter-sriov.spec` — `ln -sf libgim_amd_smi.so $RPM_BUILD_ROOT%{DEST_LIB}/libamdsmi.so` in `%install`.

The container image (`Dockerfile.sriov.exporter-release`) is unaffected by this
change — it already creates its own `libamdsmi.so` symlink under `/home/amd/lib`
and runs `ldconfig`, so the loader was already satisfied there.

### Alternatives considered

- **Rename the shipped file to `libamdsmi.so`** — rejected; keeps the real gim
  name visible for provenance/debugging and matches the container layout, and a
  symlink is the minimal change.
- **Patch the gpuagent `DT_NEEDED` to `libgim_amd_smi.so`** — rejected; the
  binary is a prebuilt asset, and rewriting its dynamic deps is fragile vs. a
  one-line packaging symlink.

## Scope

- **In scope:** deb + rpm SR-IOV packaging symlink.
- **Out of scope:** container image (already correct), non-SR-IOV packages,
  gpuagent source changes.

## Validation

- **Packaging contents:**
  - deb: `dpkg-deb -c amdgpu-exporter-sriov_22.04_amd64.deb | grep metrics/lib` shows `libamdsmi.so -> libgim_amd_smi.so`.
  - rpm: `rpm -qlvp amdgpu-exporter-sriov-rhel9.x86_64.rpm | grep metrics/lib` shows the same symlink.
- **Loader (decisive):** `ldd /home/amd/bin/gpuagent` resolves
  `libamdsmi.so => .../libamdsmi.so` instead of "not found"; A/B test confirmed
  exit 127 without the symlink → exit 0 with it.
- **Built + verified on two hosts:** deb, rpm, and the SR-IOV container image
  all carry the symlink and pass the loader check.

## Risks and rollback

- **Known risks:** low — adds a single symlink to a directory already on the
  library path; no binary or runtime-config change.
- **Rollback plan:** revert the two-line diff in `Makefile.package` and
  `rpmbuild/amdgpu-exporter-sriov.spec`; packages return to prior (broken)
  state.
