# GPUOP-825: Document unsupported profiler metrics on Radeon GPUs

- **Date:** 2026-05-30
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s):** GPUOP-825, GPUOP-809 (duplicate)

## Context

The `device-metrics-exporter` exposes 52 `GPU_PROF_*` profiler metrics in its
public catalog (`docs/configuration/metricslist.md`). On Radeon GPUs the
majority of these either are silently dropped by gpu-agent (never appear in
`/metrics` even when explicitly enabled in `GPUConfig.Fields` with
`ProfilerMetrics.all = true`) or are exported but stay at `0` even under heavy
GPU load. The user-facing docs did not communicate this, leading to false
expectations and bug reports (GPUOP-825 / GPUOP-809).

## Test methodology

Empirically verified on `10.7.121.210` with:

- Hardware: 8x Radeon V710 (PCI device `0x7551`, gfx1101 / Navi 32-class).
- Image: `amdpsdo/device-metrics-exporter:exporter-0.0.1-330`.
- Container run with `--device /dev/kfd --device /dev/dri --privileged
  --cap-add=CAP_PERFMON`.
- Config: all 52 `GPU_PROF_*` fields listed in `GPUConfig.Fields`,
  `ProfilerMetrics.all = true`.
- Idle measurement: container running, no GPU workload.
- Load measurement: `rocm/pytorch:latest` running a continuous 4096x4096 FP32
  GEMM loop with `HIP_VISIBLE_DEVICES=0` (single GPU under load to avoid
  rocprofiler-sdk contention on the other 7).
- `gpu_gfx_activity` confirmed 90% on the burned GPU during sampling.

## Findings (GPU 0)

### Move under load — supported on Radeon (4 of 52)

| Metric                      | Idle  | Load     |
|-----------------------------|-------|----------|
| `GPU_PROF_GRBM_COUNT`       | 122k  | 2.98M    |
| `GPU_PROF_GRBM_GUI_ACTIVE`  | 53k   | 2.98M    |
| `GPU_PROF_GUI_UTIL_PERCENT` | 44.1  | 100      |
| `GPU_PROF_SQ_WAVES`         | 0     | 986      |

### Exported but always `0` even under load (7 of 52)

These appear in `/metrics` (gpu-agent does not filter them out) but never move
on Radeon hardware — the underlying rocprofiler-sdk counters return `0`:

- `GPU_PROF_FETCH_SIZE`
- `GPU_PROF_SM_ACTIVE` *(GPUOP-825)*
- `GPU_PROF_OCCUPANCY_PERCENT`
- `GPU_PROF_OCCUPANCY_ELAPSED`
- `GPU_PROF_OCCUPANCY_PER_CU` *(GPUOP-825)*
- `GPU_PROF_OCCUPANCY_PER_ACTIVE_CU` *(GPUOP-825)*
- `GPU_PROF_VALU_PIPE_ISSUE_UTIL`

### Silently dropped by gpu-agent — unsupported on Radeon (41 of 52)

These do not appear in `/metrics` at all (gpu-agent disables them at startup
via field filtering and logs `<FIELD> is disabled`):

- All 28 `GPU_PROF_CPC_*` metrics (entire Command-Processor-Compute block)
- All 7 `GPU_PROF_CPF_*` metrics (entire Command-Processor-Fetcher block)
- `GPU_PROF_WRITE_SIZE`
- `GPU_PROF_TOTAL_16_OPS`, `GPU_PROF_TOTAL_32_OPS`, `GPU_PROF_TOTAL_64_OPS`
- `GPU_PROF_TENSOR_ACTIVE_PERCENT`
- `GPU_PROF_SIMD_UTILIZATION`

## Approach (docs change)

- Revert the per-row "Radeon (R9600D, W7900)" column originally proposed
  (rejected in PR #1359 review as wasted real-estate).
- Per the user's guidance, add **a short Radeon support note at the top of each
  profiler subsection** in `docs/configuration/metricslist.md`:
  - Command Processor Metrics: "Not supported on Radeon — all CPC/CPF metrics
    in this section are disabled at startup by gpu-agent and are not
    exported."
  - GPU Core Metrics: list the 4 that are supported and the 1 (`SM_ACTIVE`)
    that is not.
  - Memory & Data Transfer Metrics: "Not supported on Radeon — `FETCH_SIZE`
    is exported but always `0`; `WRITE_SIZE` is not exported."
  - Compute Operation Metrics: "Not supported on Radeon — `TOTAL_16_OPS`,
    `TOTAL_32_OPS`, and `TOTAL_64_OPS` are not exported."
  - Occupancy & Utilization Metrics: "Not supported on Radeon — none of the
    metrics in this section return non-zero values on Radeon (some are not
    exported at all)."
- Use the bare word "Radeon" (not "Radeon GPUs (e.g. R9600D, W7900)") per the
  user's preference.

## Scope

- **In scope:** `docs/configuration/metricslist.md` — profiler metrics section
  only.
- **Out of scope:**
  - Runtime behavior changes (gpu-agent already disables the unsupported
    fields correctly).
  - Test framework changes (`metrics-support.json` updates tracked separately).

## Validation

- Visual review of the updated `metricslist.md` profiler subsections.
- Empirical data from the test above (also captured here under "Findings").

## Risks and rollback

- **Risk:** Minimal — documentation-only change with no runtime impact.
- **Rollback:** Revert the commit; no functional effect.
