# CP PR #692 — GPUGet response cache TTL configurable

- **Date:** 2026-10-08
- **Related PR(s):** ROCm/device-metrics-exporter#692
- **Related issue(s) / JIRA:** N/A (community contribution)

## Context

Cherry-pick of upstream PR #692 into the internal (pensando) repo. The change adds a new environment variable `AMD_GPU_GET_CACHE_TTL` that lets operators tune (or disable) the GPUGet response cache. The default 15 s TTL is unchanged; setting `0s` bypasses the cache entirely, issuing a fresh GPUGet on every scrape.

## Approach

- Straight cherry-pick of the squash-merge commit `54e2071d7f` from `upstream/main` onto a new branch based on `pensando/main`.
- `AMD_GPU_GET_CACHE_TTL` is parsed once in `cmd/exporter/main.go` and threaded through `ExporterOptions` → `GPUAgentClientOptions`.
- A zero-value `GPUAgentClient` literal now defaults to no cache (was 15 s); no existing test relies on that default.

### Alternatives considered

- Rebase instead of cherry-pick — rejected; single squash commit, cherry-pick is cleaner.

## Scope

- **In scope:** env-var parsing, option plumbing, cache TTL override, unit tests.
- **Out of scope:** changing default TTL, docs updates, config.json integration.

## Validation

- Unit tests: `gpuagent_cache_ttl_test.go` (added by upstream PR).
- Integration: existing exporter test coverage in `exporter_test.go`.
- Manual: deploy with `AMD_GPU_GET_CACHE_TTL=0s` and confirm every scrape triggers a fresh GPUGet.

## Risks and rollback

- Known risks: none — additive feature, default behavior unchanged.
- Rollback plan: revert the cherry-pick commit.
