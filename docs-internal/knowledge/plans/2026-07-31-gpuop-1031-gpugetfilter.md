# GPUGetFilter: derive per-attribute skip flags from enabled config

- **Date:** 2026-07-31
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-1031

## Context

The exporter always sent an empty `GPUGetRequest{}` to gpuagent, so every
server-side collector ran each poll even when `config.json` disabled most metric
fields. On nodes with many KFD processes the process walk alone dominates a
GPUGet (~15s / ~1984 rows on an 8-GPU MI350 under ~508 procs), piling requests
against the gRPC thread cap and starving metrics.

gpuagent now accepts an optional `GPUGetFilter` on `GPUGetRequest` with 11
per-attribute `Skip*` flags (unset = fetch everything = prior behavior). This
change wires the exporter to request only what the enabled config needs.

## Approach

- Add `GPUGetFilter` message + `GPUGetRequest.Filter` to the vendored proto and
  regenerate the stubs.
- `gpuGetFilterGroups` maps each skippable attribute group to the fields sourced
  from its collector; `gpuGetFilterGroupPrefixes` covers prefix-keyed groups
  (violation, xgmi). `anyGroupFieldEnabled` reports whether any field in a group
  is enabled.
- `buildGPUGetFilter` derives the filter from the currently enabled
  `Fields`/`Labels` and sets eight `Skip*` flags (clock, xgmi status+stats,
  process, vram usage, violation, pcie stats, activity). It is computed once per
  config load via `initGPUGetFilter` on the existing 3-second reload path and
  cached on `gCache.gpuGetFilter`.
- `getGPUs(filter, cache)` and `cacheRead(filter, cache)` take the filter and
  cache slot as arguments. The Prometheus/health path passes the config-derived
  `gpuGetFilter` + `metricsCache`; the `/gpumetrics` NPD path (`QueryMetrics`)
  passes a `SkipProcessStatus: true` filter + `npdCache`. NPD thus gets the full
  GPU object minus the expensive process walk — which also keeps it under the
  15s GPUGet deadline on nodes with many KFD processes.
- `metricsclient gpuctl` exposes all 11 `GPUGetFilter` fields as `--skip-*` flags
  (unset = nil filter = fetch everything) for manual filter testing.
- The unused `worker.go` (dead `NewWokerRequest`, no callers) is removed.
- The Dockerfile builds gpuagent from ROCm/gpu-agent at the commit carrying this
  support; the previously-carried gpuagent patches are dropped because their
  fixes are already upstream at that commit.
- The prebuilt deb/rpm assets (`assets/gpuagent_static.bin.gz`,
  `assets/gpuagent_sriov_static.bin.gz`, `assets/gpuctl.gobin`) are regenerated
  from that same pinned gpu-agent commit, so the Debian/RPM packages ship the
  filter-aware gpuagent/gpuctl and do not lag the container image.

### Alternatives considered

- **Separate health-only GPUGet call** — rejected. Health validation and metrics
  share one `cacheRead`/GPUGet behind a 15s cache; a second call would double the
  expensive read and break cache coherency. Instead the single shared filter is
  kept safe for health by never skipping the groups health consumes. The NPD
  `/gpumetrics` endpoint is the exception: it needs the full object (minus the
  process list), so it gets its own cache slot (`npdCache`) and a
  `SkipProcessStatus` filter.
- **Env-var / explicit opt-in flag** — rejected. Deriving from the already-present
  enabled-field config needs no new surface and stays correct as config changes.

## Scope

- **In scope:** proto/stub sync; field→group mapping; filter derived once on the
  3s reload and cached; `getGPUs`/`cacheRead` take (filter, cache) args —
  config-derived filter + `metricsCache` for Prometheus/health, `SkipProcessStatus`
  + `npdCache` for NPD; `metricsclient gpuctl --skip-*` flags; worker.go
  removal; docs; unit tests; Dockerfile gpuagent pin + dropping now-upstream
  patches; regenerated deb/rpm gpuagent+gpuctl assets from the pinned commit.
- **Out of scope:** any change to gpuagent itself; a dedicated gRPC client for
  amd-gpu-health (stays on the shared HTTP `/gpumetrics` path).

## Validation

- **Unit tests:** `gpuagent_filter_test.go` — 10 cases covering per-group
  derivation, `KFD_PROCESS_ID` label gating the process walk, xgmi one-flag-both,
  the VRAM-usage-vs-Status trap, and the health-groups-never-skipped invariant.
- **e2e:** `make docker-mock` + `make e2e-test` — 29/29 pass.
- **Release image:** `gpuagent-build` stage compiles clean from the pinned
  ROCm/gpu-agent commit with no patches; built `gpuctl` exposes all 11 `--skip-*`
  flags.
- **Hardware (MI350, ~508-proc load):** config→filter→`/metrics` proven — a full
  config emits every enabled group; a minimal config trims to only the enabled
  field (216 → 8 series); an ECC-only config still emits ECC. GPUGet cost drops
  from ~15s/1984 rows to ~0.86s/1728 rows with the process walk skipped.

## Risks and rollback

- **Health regression** if a health-consumed group were skipped — mitigated:
  ECC and PCIe-status are never expressed as skip flags, covered by a dedicated
  unit test.
- **NPD regression** if `/gpumetrics` returned config-filtered data — mitigated:
  NPD uses its own `npdCache` and a `SkipProcessStatus`-only filter, independent
  of the Prometheus config, so it always returns the full object minus the
  process list. Skipping the process walk also keeps NPD under the 15s deadline
  on high-KFD-process nodes (a fully-unfiltered NPD read timed out there).
- **Config-reload edge:** the filter re-derives reliably on a fresh start; an
  in-place config overwrite was observed not to re-derive within the poll window.
  Metric population reload is unaffected; noted as a follow-up.
- **Rollback:** an unset/empty filter reproduces the prior fetch-everything
  behavior, so reverting the exporter change (or shipping an empty filter) is a
  safe fallback.
