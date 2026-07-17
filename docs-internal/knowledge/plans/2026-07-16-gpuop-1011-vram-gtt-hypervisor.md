# GPUOP-1011: suppress host-unsupported VRAM/GTT metrics under GIM/SR-IOV

- **Date:** 2026-07-16
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** fix/gim-host-unsupported-metrics
- **Related issue(s) / JIRA:** GPUOP-1011 (GPUOP-924 workload evidence)

## Context

On the SR-IOV/GIM hypervisor host (MI300X/MI210), the exporter reports `0` for
per-VF VRAM-usage and GTT metrics even under an active guest GPU workload. The GIM
host SMI lib has **no `amdsmi_get_gpu_memory_usage`/`_total` API** for these types,
so the gpuagent GIM path never fills them and DME emits a misleading `0` (fails the
CI `test_vf_metric_nonzero_in_samples` / `test_vf_used_vram_under_workload`).

Workload evidence (GPUOP-924, leto MI210, 85 s sustained PyTorch matmul): host GIM
telemetry tracks gfx/umc activity, sclk, power, temp correctly, but **`used_vram`
stays 0 host-side** while the guest amd-smi reports real usage — confirming GIM does
not surface per-VF framebuffer usage to the host path. GTT and visible-VRAM have no
host source at all (totals are 0 too).

Decision: **suppress** these 7 metrics under hypervisor mode (report NA, not 0), per
the JIRA interim mitigation. (`amdsmi_get_guest_data().fb_usage` is a possible future
host-side source for `used_vram`, noted for follow-up but not wired here.)

Affected (7): GPU_USED_VRAM, GPU_TOTAL/USED/FREE_VISIBLE_VRAM, GPU_TOTAL/USED/FREE_GTT.

## Approach

Two coordinated changes (gpuagent + DME):

- **gpuagent** (branch `feat/gim-host-unsupported-sentinel`, `gimamdsmi/smi_api.cc`):
  set the 7 unsupported `vram_usage` fields (used_vram, total/used/free visible,
  total/used/free GTT) to `AMDSMI_UINT64_INVALID_VAL` in `smi_gpu_fill_stats` so DME's
  `IsValueApplicable` suppresses them.
- **DME** (this branch, `gpuagent_gpu_metrics.go`): `used_vram` is computed after
  `NormalizeUint64` (which turns the sentinel into 0), so add an explicit
  `IsValueApplicable(vramUsage.UsedVRAM)` gate before emitting it. `free_vram` is
  derived from used (`total - used`), so it is suppressed under the same gate —
  otherwise it would always report `total` when used is unavailable. Baremetal is
  unaffected (real values pass the check). visible/GTT already route through
  `logWithValidateAndExport` and suppress on the sentinel.
- **Docs:** `docs/configuration/metrics-support-matrix.yaml` — flip the 7 entries to
  `hypervisor_host: false`, `class: N`, with GPUOP-1011 notes.
- **Asset:** refresh `assets/gpuagent_sriov_static.bin.gz` with the rebuilt
  `gpuagent_gim` — this prebuilt binary is the GIM walker that ships in the SR-IOV
  rpm/deb packages and the `make docker-sriov` image, so the fix only reaches those
  deliverables once the asset is repackaged.

## Verification

- Build SR-IOV image (new gpuagent_gim + DME server), deploy on banff (GIM, CPX+SPX).
- Assert the 7 metrics are **absent** (NA-suppressed), `total_vram`/`free_vram`
  retained and sane, no sentinel leaks, no regression in GPU-level metrics.
- Baremetal path unchanged (used_vram real → still emitted).
- **Workload-proven (leto MI210, sustained PyTorch load):** guest VF used_vram rose
  14→2979 MB (real allocation) while the host exporter kept all 7 metrics suppressed
  under load; host `gfx_activity` moved to 100% (so the host observes the VF — it
  simply does not surface per-VF VRAM/GTT). Confirms all 7 are genuinely
  host-unsupported under load, not just idle.

## Risks and rollback

- Low risk — suppression only affects host-unsupported fields; baremetal and guest
  paths keep real values. Rollback: revert this commit + the gpuagent commit.
- Follow-up: evaluate `amdsmi_get_guest_data().fb_usage` as a real host-side
  `used_vram` source (separate story; needs nonzero-under-load confirmation).
