# GPUOP-985 — GFX max clock TTL refresh via gpuagent patch (v1.5.2)

- **Date:** 2026-08-21
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-985

## Context

On RDNA (W7900/gfx1100, R9700S/gfx1201), `gpu_max_clock{clock_type=system}`
reads stale under GPU load and fails the `GPU_MAX_CLOCK` accuracy check. Root
cause is in gpuagent: `smi_fill_gpu_clock_frequency_spec_` reads the GFX
`max_clk` once from `smi_gpu_init_immutable_attrs` and the per-GpuGet status
path only copies the cached value. On RDNA the boost ceiling is mutable and
rises above the nominal DPM max under load, so the init snapshot goes stale
versus the live `amd-smi` value. CDNA (MI2xx/MI3xx) has a fixed DPM range so it
never drifts, which is why only Radeon fails. RCA + repro:
`docs-internal/knowledge/rca/GPUOP-985-rca.md`.

## Approach

Deliver the gpuagent fix to DME v1.5.2 as an in-tree **patch** rather than a
`GPUAGENT_COMMIT` bump (same route as GPUOP-1033 / patch 0002).

- Add `patch/gpuagent/0003-gpuop-985-gfx-maxclock-ttl.patch` (from the gpuagent
  commit based on `9cee401`, the release pin; applies cleanly after 0001+0002).
- The fix re-reads `amdsmi_get_clock_info(GFX)` in `smi_fill_clock_status_` at
  most once per 5s TTL per GPU (new `g_gfx_maxclk` cache of {lo,hi,timestamp}),
  re-applying the cached ceiling onto the per-get spec copy every GpuGet
  (`fill_spec_` memcpys the stale init spec each call, so re-apply is required).
  Bounds staleness to the TTL while keeping the render-node hot path off a
  per-call amdsmi query (KUBE-50 constraint); GFX-only, inside the existing
  `skip_clock_status` filter gate.
- **RDNA-only** (per gpuagent-owner feedback): the drift is RDNA-specific, so the
  refresh is gated to RDNA GPUs. RDNA is recorded once at init
  (`target_graphics_version` top nibble == 1, i.e. gfx10/11/12) in a new
  `g_rdna_gpus` set; CDNA GPUs skip the refresh entirely, leaving the MI get path
  unchanged (their fixed DPM range makes the init snapshot authoritative).
- The `amdsmi_get_clock_info` refresh runs **outside** `g_gfx_maxclk_mutex`
  (double-checked): one GPU's refresh never blocks the others' clock-status fills.
- `GPUAGENT_COMMIT` unchanged (`9cee401`). Docker gpuagent-build stage applies
  `patch/gpuagent/*.patch` via `git apply`
  (`docker/build_prep_docker.sh` + `Dockerfile.exporter-release:96-98`).
- No asset repackaging: rpm/deb also build gpuagent from source and apply
  `patch/gpuagent/*.patch` (`GPUAGENT_FROM_SOURCE=1`, default). The committed
  `assets/gpuagent_static.bin.gz` is only the `GPUAGENT_FROM_SOURCE=0` fallback.

### Alternatives considered

- `GPUAGENT_COMMIT` bump — rejected: forces updating gpuagent branches first.
- DME-side synthesis — rejected: violates the "no derived values" rule; DME is a
  faithful pass-through of `clock.HighFrequency`.
- Un-cache GFX max entirely (re-read every GpuGet) — rejected: reintroduces the
  per-get amdsmi call KUBE-50's immutable-attrs cache was built to avoid.

## Scope

- **In scope:** patch file only (covers docker + rpm/deb, both from-source).
- **Out of scope:** GPUAGENT_COMMIT bump; asset repackaging; gpuagent branch
  changes; the TTL-value tuning decision (see open question).

## Validation

- Patch chain applies cleanly in order (0001 -> 0002 -> 0003) against the release
  pin `9cee401` via `git apply`.
- Built patched gpuagent (gpuagent-builder-rhel:9). A/B on W7900 (10.7.40.16):
  idle both GPUs = 1760 (match amd-smi); under load patched DME emits >1760
  (tracked the boost, e.g. 1787/1839) then decays to 1760 within the ~5s TTL;
  stock stays frozen at the init-latched value. No false positives on a
  100%-busy GPU whose workload stayed <=1760.
- Same-instant DME-vs-amd-smi (amd-smi read from a stock ROCm container, not the
  so.27 overlay): exact match at idle; under load both track the boost within
  the pytest +/-5% band. Note amd-smi's own max_clk is a noisy high-water mark
  that swings read-to-read, so DME == amd-smi at each TTL refresh instant but
  can differ by the amd-smi jitter between refreshes.
- RDNA-gated build (full docker pipeline, real image) re-validated end-to-end:
  W7900 (RDNA/gfx1100) under load still tracks the boost (DME emitted 1953 vs
  amd-smi 1869-2067) then decays -- so the RDNA branch provably runs. MI350X
  (CDNA/gfx950, SMCi) stays at a stable 2200 under gets with no regression; the
  refresh is gated off (is_rdna false for gfx950). KUBE-50-scale latency A/B
  (626 KFD procs, MI350X) already showed no measurable GpuGet delta stock vs
  patched, and the CDNA gate now removes even the per-get set-lookup cost path
  from firing an amdsmi call.

## Open question

TTL value (currently 5s) trades staleness against amdsmi call frequency, and
freezes a jittery source. If the accuracy test wants tighter agreement, shorten
the TTL (more amdsmi calls) — confirm intended `GPU_MAX_CLOCK` semantics (live
ceiling vs stable nameplate) with the gpuagent owner before finalizing.

## Risks and rollback

- Low risk: ~38-line change, GFX-system-clock-only, HW-validated on W7900 +
  CDNA control (MI210). Mutex-guarded cache; no change to other clock domains.
- If the pinned gpuagent later advances past where the patch applies, in-image
  `git apply` fails loudly -> switch to commit-bump.
- Rollback: remove the patch file.
