# Width-correct NA sentinels in gpuagent; respective-max-only guard in exporter

- **Date:** 2026-09-28
- **Author:** praveen
- **Related PR(s):** TBD (PR-A gpuagent, PR-B exporter)
- **Related issue(s) / JIRA:** ROCm/device-metrics-exporter#662

## Context

amd-smi (and the amdgpu driver that fills `gpu_metrics`) reports an unavailable
field as the all-Fs value of the field's own width: `uint8 -> 0xFF`,
`uint16 -> 0xFFFF`, `uint32 -> 0xFFFFFFFF`, `uint64 -> UINT64_MAX`. gpuagent
widens many narrow source fields into wider aga/proto fields (e.g. the `uint16`
`current_socket_power` into a `uint64` proto field) without widening the
sentinel, so a sub-width all-Fs lands in a wide field. To cope, the exporter's
`IsValueApplicable` / `NormalizeUint64` rejected *every* type-max
(`MaxUint8|16|32`, `MaxInt32`, `MaxUint64`) for *any* value regardless of width.

That over-broad check is the #662 bug: a real 255 W power reading (a valid
`uint16`) equals `MaxUint8`, so it was classified unsupported and — because the
field logger caches an unsupported field during startup — suppressed for the
process lifetime. The same false-rejection risk applies to any field whose real
value can equal a lower-width max (voltage, fan, small counters).

## Approach

Establish the invariant that **whenever a value is widened, its NA sentinel is
widened with it**, so every value the exporter sees is either real or the all-Fs
of its own datatype. Then the exporter only checks the respective-datatype max.

### PR-A — gpuagent (ROCm/gpu-agent)

- New header `sw/nic/gpuagent/api/smi/smi_na.hpp` with two pure templated
  helpers: `aga_widen_na<D,S>(val, gim=false)` (source-width all-Fs, and with
  `gim` also `INT32_MAX`, map to destination all-Fs) and `aga_widen_na_float<S>`
  (source-width all-Fs maps to the float NA sentinel `UINT32_MAX`).
- Helpers are function-like macros (`AGA_WIDEN_NA`, `AGA_WIDEN_NA_GIM`,
  `AGA_WIDEN_NA_FLOAT`) taking the source and destination all-Fs maxes
  explicitly, rather than C++ templates.
- Apply at every widening site in the `amdsmi` and `gimamdsmi` backends. A full
  field-by-field audit of the amdsmi source vs aga destination widths (prompted
  by a HW-caught `gpu_clock 65535` regression) found these `uint16 -> wider`
  sites, all now widened:
  - amd-smi -> aga (`smi_api.cc`, amdsmi): power, voltage, fan; the `gfx/umc/mm`
    activity assignments; **clock frequencies** (`current_gfxclks/uclk/vclk0s/
    dclk0s/socclks`, u16->u32); **xgmi** `link_width`/`link_speed` (u16->u64);
    **pcie** `max_pcie_width`/`pcie_width` (u16->u32); temperature via the float
    macro.
  - gim (`smi_api.cc`, gimamdsmi): activity copies use `AGA_WIDEN_NA_GIM` for the
    `INT32_MAX` sentinel. GIM power/voltage are `uint64` source and destination
    (no widening needed). GIM clocks come from `amdsmi_get_clock_info` (`uint32`,
    already full-width).
  - aga -> proto (`gpu_to_proto.hpp`): `power_usage` (u32->u64) and the
    `vcn/jpeg` activity/busy setters (u16->u32).

### PR-B — exporter (this repo)

- Reduce `IsValueApplicable` / `NormalizeUint64` to reject only the value's own
  datatype max (`uint64->MaxUint64`, `uint32->MaxUint32`, ...). This fixes #662.
- Replace six export call sites that passed `float64(math.MaxUint32)` as a
  control-flow "skip export" sentinel (relying on the old over-broad rejection)
  with the explicit `markUnsupportedFields(...)` already used by their sibling
  branches. Without this, a widened sentinel would export with mismatched labels
  and panic `GaugeVec.With`.
- Carry PR-A as `patch/gpuagent/0001-gpuagent-na-sentinel-width-translation.patch`
  (applied by the gpuagent-build stage via `git apply`) so ci-sanity-build
  validates the combined change on real hardware before PR-A merges. The pinned
  `GPUAGENT_COMMIT` bump replaces the patch once PR-A lands.

### Alternatives considered

- Drop only the stranded `MaxUint8` check, keep the rest — rejected: the user
  wants the full respective-max-only cleanup, and widening in gpuagent is the
  correct home for the fix (the exporter cannot recover a field's origin width).
- Centralized post-fill normalization pass in gpuagent — rejected: it cannot see
  each field's origin width and would hardcode/stale; per-assignment widening via
  a template that deduces the source type is robust.

## Scope

- Backends: `amdsmi` (baremetal) and `gimamdsmi` (SR-IOV). Legacy `rocmsmi` is not
  compiled into this stack, so it is out of scope.
- `MaxInt32` (GIM smi-lib sentinel) is normalized in gpuagent for the engine
  activity fields (the documented carriers), letting the exporter drop its
  `MaxInt32` special-case. If other GIM fields surface with `INT32_MAX`, they need
  the same `gim=true` treatment.

## Testing

- gpuagent helper: table-driven red-green unit test over all width pairs +
  `INT32_MAX` + the float helper (compiled standalone; gpuagent has no C++ unit
  harness, so full-build validation is via ci-sanity-build on real hardware).
- exporter: `pkg/exporter/utils` guard tests rewritten to per-type inputs,
  asserting a real 255 (uint64) is applicable (#662) and only the respective max
  is NA; `pkg/amdgpu/gpuagent` package tests pass (the sentinel-site panic is
  resolved).
- Patch verified to `git apply --check` cleanly on the pinned gpuagent SHA.
- HW: CI image (jobc target `33989295`) loaded on a baremetal MI210 host. First
  pass exposed `gpu_clock 65535` (u16 NA leaking) which drove the field audit and
  the clock/xgmi/pcie fixes above. Image identity confirmed via
  `server --version` (`GitCommit d6b65d220`) and `gpuctl show version`
  (`HEAD-26807f9`). Re-validation runs on the rebuilt image.

## Risks / limitations

- gpuagent C++ changes are compile-validated only by the docker build /
  ci-sanity-build, not locally.
- The exporter simplification is only safe because gpuagent now widens every
  guarded field (including the temperature float path); the patch and the SHA
  bump must land together with PR-B, never a bare exporter change against an old
  agent.
