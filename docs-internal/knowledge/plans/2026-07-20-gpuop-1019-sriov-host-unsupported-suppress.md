# GPUOP-1019: suppress host-unsupported engine/link/PCIe metrics under GIM/SR-IOV

- **Date:** 2026-07-20
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** gpuop-1019-sriov-suppress (DME), bhatturu/gpu-agent@gpuop-1019-gim-sentinel
- **Related issue(s) / JIRA:** GPUOP-1019

## Context

On the SR-IOV/GIM hypervisor host (reproduced on banff MI300X; reported on MI350X
device_id 75a0, GIM 9.1.0.K), sriov-dme emits a set of metrics with a misleading
`0` that are not observable from the host. `test_metric_coverage` in
`hypervisor/debian/test_sriov_exporter_debian.py` flags them as untracked.

Root cause: the GIM proto serializer (`gpu_to_proto.hpp`) emits `usage`,
`pcie_stats`, and `xgmi_link_stats` unconditionally, but the GIM walker
(`gimamdsmi/smi_api.cc`) never fills them (no host source for per-VCN/JPEG engine
activity, XGMI link rx/tx, or PCIe rx/tx bytes). The parent `aga_gpu_info_t` is
zero-initialized, so these ship as a genuine `0`, which DME's `IsValueApplicable`
treats as valid and exports. The baremetal path avoids this by sentinel-init'ing
the same structs to `0xff` before fill (`amdsmi/smi_api.cc`); the GIM path only
did so for `violation_stats`.

Affected (7 families removed): amd_gpu_jpeg_activity, amd_gpu_vcn_activity,
amd_gpu_xgmi_link_rx, amd_gpu_xgmi_link_tx, amd_pcie_rx, amd_pcie_tx,
amd_pcie_bidirectional_bandwidth.

Note: the matrix already marks these `hypervisor_host: false` — the docs were
correct; only the emission was wrong. No matrix/metricslist change needed.

## Approach

- **gpuagent** (branch `gpuop-1019-gim-sentinel`, off the v1.5.1 pin 875d87be):
  in `smi_gpu_fill_stats`, `memset(0xff)` the `usage`, `pcie_stats`, and
  `xgmi_link_stats` structs before fill, mirroring the baremetal idiom. Fields the
  GIM stream does populate (gfx/umc/mm activity, PCIe error counters) overwrite the
  sentinel after. Rebased onto ROCm/gpu-agent PR #79 (`a16af7e0d54`, VF VRAM/GTT
  sentinel = GPUOP-1011), so this commit adds only the 3 new memsets and #79 owns
  the vram_usage line.
- **DME** (this branch, off `pensando/v1.5.1`): keep `GPUAGENT_COMMIT` at 875d87be
  and carry the change as `patch/gpuagent/0002-gpuop-1019-gim-sentinel.patch`
  (applied to the from-source standard build via build_prep_docker.sh), plus
  repackage `assets/gpuagent_sriov_static.bin.gz` with the rebuilt `gpuagent_gim` —
  the prebuilt GIM walker is what actually ships in the SR-IOV rpm/deb and
  `make docker-sriov`. Patch applies cleanly alongside the existing rocm-26020
  patch (non-overlapping hunks).
- No DME Go change: existing `IsValueApplicable` / `markUnsupportedFields` logic
  suppresses the sentinel automatically.

## Verification

Built gpuagent_gim from the fixed branch (gpuctl version string confirms
`gpuop-1019-gim-sentinel-aa5b3769a42`), staged into the v1.5.1 sriov image,
deployed on banff MI300X + GIM (idle host).

- **7 families removed, none added** vs the 0.0.1-361 baseline (family diff clean).
- **VRAM/GTT stayed suppressed** (no reappearance): raw `gpuctl statistics` shows
  vram_usage = `18446744073709551615` (0xFFFF...FFFF sentinel, 72 lines / 8 GPUs);
  exporter suppresses used/free vram+gtt, retains `total_vram` (196592 MB).
- **Core hypervisor metrics intact:** gfx_activity, umc_activity, package_power,
  total_vram present; `deployment_mode="hypervisor"`.
- amd-smi/GIM-lib comparison: raw gpuctl (pre-suppression) shows the sentinel →
  exporter shows NA. End-to-end proof the walker emits sentinel, DME suppresses.

## Known caveats (out of scope)

- `amd_gpu_violation_gfx_clock_below_host_limit_{power,thermal}_accumulated` still
  emit (value 0). These live in `violation_stats` (already sentinel-init) but the
  GIM stream populates them with a genuine 0 — a real HW 0, not a leak. Suppressing
  them requires a separate decision on whether they are host-unsupported.

## Risks and rollback

- Low risk — suppression only affects host-unsupported fields; baremetal and guest
  paths keep real values (real 0s from the stream overwrite the sentinel). Rollback:
  revert this commit + the gpuagent commit, restore the prior asset.
