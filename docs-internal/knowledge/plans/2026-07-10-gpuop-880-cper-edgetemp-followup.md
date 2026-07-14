# GPUOP-880 — Enable CPER under SR-IOV/GIM + float-sentinel fix

- **Date:** 2026-07-10
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** fix/gpuop-880-cper-edgetemp-followup
- **Related issue(s) / JIRA:** GPUOP-880 (SR-IOV / GIM host-side metric coverage)

## Context

Follow-up hardening on the SR-IOV/GIM host-side exporter path, found while
validating GPUOP-880 on MI300X (banff) and MI210 (leto):

1. **CPER health was skipped under SR-IOV.** `cacheCperRead` /
   `applyCPERHealthChecks` were inside an `if !(enableSriov || IsSimEnabled())`
   block, so the CPER-based health path never ran on a GIM hypervisor host —
   `amd_gpu_health` could not reflect fatal CPER records under SR-IOV.
2. **Float NA sentinel overflowed to a bogus `0`.** GIM reports unsupported
   float fields (e.g. MI300 `edge_temperature`) as `UINT32_MAX`. When converted
   through `convertFloatToUint`, `float32(UINT32_MAX)` rounds up to `2^32` and
   wraps to `0` — a real-looking value that escaped `IsValueApplicable`'s NA
   suppression, so MI300 emitted 8 bogus `amd_gpu_edge_temperature ... 0` rows.
3. **metricslist.md** marked all 39 GPU_ECC rows as unsupported on the
   hypervisor, but the GIM 9.0.0.K walker decodes all 20 ECC blocks — the doc
   was stale.

## Approach

- **gpuagent_health.go:** move `cacheCperRead` / `applyCPERHealthChecks` OUT of
  the `!(enableSriov || IsSimEnabled())` block so CPER health runs under
  SR-IOV/GIM too. CPER *events* stay baremetal-only; the health path is gated
  internally on `IsCperEnabled()`.
- **utils.go `convertFloatToUint`:** clamp `float32 >= MaxUint32` →
  `uint32(MaxUint32)` and `float64 >= MaxUint64` → `uint64(MaxUint64)` before
  the cast, so a float NA sentinel stays at the NA value and
  `IsValueApplicable` can suppress it instead of exporting a bogus `0`.
- **metricslist.md:** flip all 39 GPU_ECC rows (ATHUB/HDP/DF/SMN/SEM/MP0/MP1/
  FUSE/MCA/VCN/JPEG/IH/MPIO × correct/uncorrect/deferred) from Hypervisor=✗ to
  ✓ to match the GIM 9.0.0.K walker output.

### Alternatives considered

- Suppress edge_temp by special-casing the metric name — rejected; the sentinel
  overflow is generic to any float NA field, so the fix belongs in the
  conversion helper, not one metric.
- Add a separate SR-IOV CPER code path — rejected; the existing health logic
  works unchanged once the SR-IOV guard is lifted (gated on `IsCperEnabled()`).

## Scope

- **In scope:** host-side GIM/SR-IOV CPER health, float NA sentinel
  suppression, metricslist doc accuracy for GIM ECC rows.
- **Out of scope:** baremetal CPER events (unchanged); guest-VM passthrough
  coverage (tracked separately); the `gpuctl` asset refresh (built + validated
  this session but intentionally NOT committed).

## Validation

- **Unit tests:** `make unit-test` (utils/health).
- **Manual / hardware (MI300X, banff, GIM 9.0.0.K):** container with
  `AMD_METRICS_EXPORTER_ENABLE_CPER=1`, `--device /dev/gim-smi0`:
  - `edge_temperature` series = **0** (suppressed) vs pre-fix image = **8/10
    bogus 0s** — sentinel fix confirmed.
  - exporter.log: `CPER background refresh goroutine started (interval=30s)`, no
    CPER failures; `amd_gpu_health = 1` for **8/8** GPUs — CPER runs under GIM.
  - 1921 `amd_` series, 480 ECC series / 20 families, **0** `4294967295`
    sentinel leaks, RestartCount=0, no crash.
- **Manual / hardware (MI210, leto, GIM 9.0.0.K):** same fix re-confirmed;
  edge_temperature legitimately present (43°C — MI210 has a real edge sensor),
  CPER refresh running, 1/1 healthy.

## Risks and rollback

- **Known risks:** lifting the SR-IOV guard runs the CPER health path on GIM
  hosts; bounded because it stays gated on `IsCperEnabled()` (env-opt-in) and
  events remain baremetal-only. The float clamp only changes values at/above the
  NA sentinel, which were already invalid.
- **Rollback:** revert commit `9983d9337`; restores the prior SR-IOV guard and
  conversion behavior. No schema/config migration involved.
