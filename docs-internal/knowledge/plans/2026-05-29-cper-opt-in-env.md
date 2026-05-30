# Make CPER fetch opt-in via env var

- **Date:** 2026-05-29
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** #1363
- **Related issue(s) / JIRA:** internal CPER instability investigation

## Context

CPER / inband-RAS fetches from `gpuagent` (`GPUCPERGet` RPC) have been a
source of crashes and noisy AFID metrics for users who do not need them.
Today the exporter unconditionally:

- starts a background CPER refresh goroutine per GPU client,
- calls `getGPUCPER` from the `/metrics` path and from
  `QueryInbandRASErrors`, and
- registers the AFID metric family (derived from CPER cache).

We need a low-risk way to disable all of this by default while keeping
the code path available for users who explicitly want it.

## Approach

Add a single feature flag, `AMD_METRICS_EXPORTER_ENABLE_CPER`, that
gates every CPER-touching code path. Default is **OFF**.

- New helper `utils.IsCperEnabled()` in `pkg/exporter/utils/cper_utils.go`
  (separate from `mock_utils.go` since this is production, not mock).
- Env var resolved **once** via `sync.Once` so per-scrape calls do not
  re-parse `os.Getenv`.
- Positive naming (`IsCperEnabled`) matches the env var (`ENABLE_CPER`)
  and avoids double negatives at call sites.

Gated paths:

1. `StartMonitor` — skip starting the background CPER refresh goroutine.
2. `getGPUCPER` — short-circuit to an empty response (no `GPUCPERGet`
   RPC fires; neither `/metrics` nor `QueryInbandRASErrors` reaches the
   wire).
3. `initAfidMetrics` — force `enableAfidMetrics=false`; AFID family
   is not registered.

Health checks read CPER via `cacheCperRead` and naturally see a nil
cache, so CPER-derived health logic is a no-op when disabled.

### Alternatives considered

- **Config-file flag in `exporterconfig.proto`** — rejected: requires
  proto regen + new schema, larger blast radius for what is intended as
  a stop-gap kill switch.
- **Compile-time build tag** — rejected: cannot be toggled at deploy
  time, awkward to ship from a single image.
- **Keep `IsCperDisabled` in `mock_utils.go`** — rejected per PR review;
  helper is production code and belongs in a non-mock file.

## Scope

- **In scope:**
  - Env-gated short-circuit of all CPER fetches and AFID metric
    registration.
  - Helper `utils.IsCperEnabled` with one-time env resolution.
- **Out of scope:**
  - Changing CPER-derived health semantics when CPER is enabled.
  - Removing CPER code paths entirely.
  - Surface in `config.json`/proto (future work if the flag sticks).

## Validation

- **Unit tests:** existing `pkg/exporter/utils` tests pass (28.3%
  coverage). `pkg/amdgpu/gpuagent` failures observed during local
  validation were environmental (`mkdir /var/run/exporter: permission
  denied`) and unrelated to this change — same failure occurs on the
  base commit.
- **Manual verification (default = disabled):**
  - `curl -s localhost:5000/metrics | grep -c '^amd_gpu_afid_errors'`
    → expect `0`.
  - `tcpdump -i lo 'tcp port 50061'` while scraping `/metrics` →
    no `GPUCPERGet` frames.
  - `strace -f -p $(pgrep amd-metrics-exporter)` → no `GPUCPERGet`
    syscalls.
- **Manual verification (opt-in):** set
  `AMD_METRICS_EXPORTER_ENABLE_CPER=1`, restart exporter, repeat the
  three checks above and confirm AFID metrics appear and `GPUCPERGet`
  fires.

## Risks and rollback

- **Risk:** users who relied on AFID / CPER-derived health silently
  lose those signals after upgrade. Mitigation: the env var to re-enable
  is documented in the commit message and this plan; no code rollback
  required to restore old behavior.
- **Risk:** `sync.Once` means the env var is read on first call only;
  changing the env at runtime has no effect until restart. Acceptable
  for a feature flag of this type.
- **Rollback:** revert this commit, or operationally set
  `AMD_METRICS_EXPORTER_ENABLE_CPER=1` to restore prior behavior with
  no code change.
