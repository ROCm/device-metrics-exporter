# Gate gpu_nodes_total on GPU_NODES_TOTAL field config in store-serving paths

- **Date:** 2026-10-28
- **Author:** spraveenio
- **Related PR(s):** https://github.com/spraveenio/device-metrics-exporter/pull/30 (fork), upstream PR targeting `release/v1.5.4`
- **Related issue(s) / JIRA:** none (NO-JIRA)

## Context

In the store-serving paths (snapshot-store / high-res), `gpu_nodes_total` was
exported unconditionally regardless of whether `GPU_NODES_TOTAL` was present in
the config `Fields` allowlist. The legacy GaugeVec-only path already honored the
field list, so the two paths disagreed: a `Fields` allowlist that excluded
`GPU_NODES_TOTAL` still leaked the metric via the store collector.

## Approach

- Add `nodeTotalEnabled()` on `GPUAgentGPUClient`, backed by `getExporterFieldState`.
- Gate the node-total store descriptor (`gpuStoreFieldDescs`, `gpuHighResStoreFieldDescs`),
  the seed (`seedGPUNodeTotal`), and the slot write (`writeGPUNodeSlot`) on it.
- `seedGPUNodeTotal` drops any stale slot via `store.DropFields` when the field is
  disabled, so a config reload that newly disables `GPU_NODES_TOTAL` clears it.
- Default behavior (no `Fields` restriction) is unchanged: all fields, including
  `gpu_nodes_total`, are exported.
- Updated several stale comments in `gpuagent_gpu_store.go` and the highres store
  test file that still described `gpu_nodes_total` as unconditionally/always
  present — no functional change, comment-only cleanup to match the new gate.

### Alternatives considered

- Leave `gpu_nodes_total` unconditional (prior behavior) — rejected because it
  diverges from the GaugeVec path's field-gating and from the config contract
  that `Fields` controls what is exported.

## Scope

- **In scope:** `pkg/amdgpu/gpuagent/gpuagent_gpu_store.go` field-gating logic,
  `pkg/amdgpu/gpuagent/gpuagent_gpu_highres_store_test.go` test coverage/comments.
- **Out of scope:** `gpu_health`, which remains always written/exported in
  store-serving paths regardless of config (unchanged by this PR).

## Validation

- Unit tests:
  - `TestHighResStoreConfigDisableBothSurfaces` now asserts the node total is
    absent when `GPU_NODES_TOTAL` is not listed.
  - New `TestHighResStoreNodeTotalFieldGate` asserts it is present when listed.
  - Full package run: `go test -cover ./pkg/amdgpu/gpuagent/...` — PASS,
    62.7% coverage, run inside the project build container.
  - `go build ./...` and `go vet ./pkg/amdgpu/...` — clean.
- Integration / e2e tests: none run (no HW change surface).
- Manual / hardware steps: N/A.

## Risks and rollback

- Known risks: any deployment relying on `gpu_nodes_total` together with a
  `Fields` allowlist must now explicitly list `GPU_NODES_TOTAL`, or the metric
  disappears from `/metrics`.
- Rollback plan: revert the single commit; no schema/data migration involved.
