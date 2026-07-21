# ROCM-26020: Navi48 idle gfx cold-read fix + UBB node power removal

- **Date:** 2026-07-20
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** device-metrics-exporter branch `fix/rocm-26020-gfx-coldread`; ROCm/gpu-agent PR #80
- **Related issue(s) / JIRA:** ROCM-26020, GPUOP-842, GPUOP-826 (idle half)

## Context

On discrete RDNA/Navi (Radeon) GPUs with functional PCIe runtime PM, an idle GPU
runtime-suspends to D3. gpuagent's per-`GpuGet` path called several amdsmi info
APIs that open `/dev/dri/renderD*` (overdrive/perf/power-cap, voltage curve,
enumeration ids, and the UBB `amdsmi_get_node_handle`). The amdgpu DRM `.open`
fop runs `pm_runtime_get_sync()`, forcing a D3->D0 resume just to read static
config; the SMU's first post-resume sample reads a stale-high gfx_activity
(~60-77%) and clock (~1.8GHz) versus the true idle (~0-4% / ~15-50MHz). Because
a metrics exporter is scraped continuously, every scrape woke idle GPUs and
reported the spike. Instinct MI2xx/MI3xx never runtime-suspend, so they are
unaffected.

## Approach

- Move the render-node-opening static reads off the per-`GpuGet` path into
  `smi_gpu_init_immutable_attrs` (read once at GPU create), consolidated into a
  single `smi_gpu_fill_static_config_()` (overdrive/perf/power-cap + voltage
  curve). Enumeration ids likewise moved to create.
- The `GPUUpdate` handler writes a successful set back into the cached `spec_`
  (gated per `upd_mask` bit), so config changes still surface without an agent
  restart. The only unsupported case is out-of-band config change (not via
  gpuagent), which is not a supported flow on a monitored node.
- Per-query path now issues only `amdsmi_get_gpu_metrics_info`, which reads
  gpu_metrics sysfs and returns BUSY (no wake) on a suspended GPU.
- Drop the UBB node power / node-power-cap reads (`amdsmi_get_power_info`
  ubb_power, `amdsmi_get_node_handle`, `amdsmi_get_npm_info`) from both the
  baremetal (amdsmi) and SR-IOV (gimamdsmi) paths: node_handle opens the render
  node (another idle waker) and the UBB metrics were unreliable.
- DME wiring: bump `GPUAGENT_COMMIT` to `875d87be5797` (ROCm main tip incl. GIM
  9.1.0.K #78) and carry `patch-gpuagent/0001` with the gpuagent change until
  ROCm PR #80 merges; repackage `gpuagent_static.bin.gz` +
  `gpuagent_sriov_static.bin.gz` for rpm/deb + SR-IOV image.

### Alternatives considered

- **amdsmi env gate (PR #8024, AMDSMI_SKIP_GPU_METRICS_ON_IDLE):** blanket-masks
  the full field list, sticky boot-order failure. Rejected/deferred.
- **Per-query suspend guard (read runtime_status, skip when suspended):** keeps
  the reads on the hot path, only avoids the wake while suspended. Superseded by
  the boot-time move which removes the render-node open entirely.
- **Gate on a "version ioctl":** impossible — the wake is at `open()`, below any
  ioctl.

## Scope

- **In scope:** gpuagent per-query render-node opens for static config; UBB node
  power reads (both paths); DME gpuagent commit bump + patch + asset repackage.
- **Out of scope:** GPUOP-985 (max_clock frozen under load), GPUOP-826 load-half
  (DVFS instantaneous sampling mismatch), 842-3 active-idle amd-smi-vs-DME
  sampling delta. No-op on Instinct.

## Validation

- Unit tests: gpuagent builds clean (baremetal + gim targets).
- Manual / hardware (Navi48, 8x R9700S, runpm=1): suspended GPUs no longer woken
  by a scrape/GpuGet (was 8/8 -> 0/8); no cold-read spike (gfx 0-6, clock
  ~15-50MHz vs stale 60-77% / ~1800MHz); active-idle and under-load values
  unchanged; no UBB metrics emitted; no crash.
- Pending: SR-IOV/GIM validation on banff (MI300X); DME exporter image e2e.

## Risks and rollback

- Known risks: out-of-band config change (not via gpuagent) not reflected until
  restart — not a supported flow. Patch must apply on the pinned gpuagent base
  (875d87be5797); once PR #80 merges, drop the patch and bump to the merged SHA.
- Rollback: revert the branch; DME returns to the prior `GPUAGENT_COMMIT` and
  committed assets.
