# GPUOP-1005: Scope GPU_VRAM_MAX_BANDWIDTH to MI3xx in metrics docs

- **Date:** 2026-07-16
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** #1467
- **Related issue(s) / JIRA:** GPUOP-1005 (same pattern as GPUOP-825)

## Context

`device-metrics-exporter` documents `GPU_VRAM_MAX_BANDWIDTH`
(`gpu_vram_max_bandwidth`) as supported on `[MI2xx, MI3xx]` in
`docs/configuration/metricslist.md`. On the AMD Instinct MI210 (gfx90a) the
metric is not exported: gpu-agent reports the field as unsupported for that
platform at startup, so it never appears in `/metrics`. The catalog did not
reflect this, leading to a system-test failure
(`test_exporter_all_supported_metrics`, job 32282018, MI210 gpu-operator
sanity on OpenShift) and the GPUOP-1005 bug. This mirrors GPUOP-825, which
added platform scoping for profiler metrics unsupported on Radeon.

## Approach

- In `docs/configuration/metricslist.md`, change the platform tag on the
  `GPU_VRAM_MAX_BANDWIDTH` row from `[MI2xx, MI3xx]` to `[MI3xx]`.
- Add `GPU_VRAM_MAX_BANDWIDTH` to the "MI3xx-Only Metrics" list in the
  Platform Support Summary, keeping the doc self-consistent.
- Per reviewer (spraveenio) guidance: drop the MI2xx tag rather than adding a
  verbose per-row "not supported on MI210" note.

### Alternatives considered

- Verbose per-row note (`**_Not supported on MI210 (gfx90a)_**: ...`) —
  rejected in review as too wordy; tag scoping is the established convention.
- Flip the Baremetal column to `&cross;` — wrong: the metric is still
  supported on MI3xx baremetal. Only the platform scope is narrowed.

## Scope

- **In scope:** `docs/configuration/metricslist.md` — the
  `GPU_VRAM_MAX_BANDWIDTH` row tag and the MI3xx-Only summary list.
- **Out of scope:**
  - Runtime behavior (gpu-agent already excludes the field on MI210).
  - `metrics-support-matrix.yaml` (the doc-audit gate only compares the
    Hypervisor/Baremetal booleans, which are unchanged; platform granularity
    lives in the `[...]` tag / summary prose).

## Validation

- `make doc-audit` (`tools/scripts/doc-audit.sh`): checked=142, errors=0.
- Jira evidence: gpu-agent logs
  `GPU 0 Platform doesn't support field name: GPU_VRAM_MAX_BANDWIDTH`
  (`gpuagent_utils.go:66`) on MI210 (env: MI210, gfx90a, OpenShift,
  exporter-0.0.1-356 baremetal image).

## Risks and rollback

- **Risk:** Minimal — documentation-only change, no runtime impact.
- **Rollback:** Revert the commit; no functional effect.
