# Fix k8s-e2e ECC injection tests: enable Debug.EnableAPI

- **Date:** 2026-09-28
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** Regression from #1497

## Context

PR #1497 added `CommonConfig.Debug.EnableAPI` as a runtime gate for the
`SetError` gRPC endpoint used by `metricsclient --ecc-file-path`. The regular
`test/e2e` suite was updated to toggle this flag, but the `test/k8s-e2e` suite
was not. As a result, Test007/008/009 (ECC injection and health-label
verification) fail on every real hardware run with `"invalid function error"`.

## Approach

- Add `debugConfig` / `commonConfig` structs to the k8s-e2e `exporterConfig`
  type so the test can set `EnableAPI: true` in the ConfigMap.
- In Test007, update the ConfigMap with `EnableAPI: true` before the first
  `metricsclient` call.
- In Test009, include `EnableAPI: true` alongside the `GPUConfig` update that
  sets health thresholds (the ConfigMap replace would otherwise drop it).
- Wrap `metricsclient` exec calls in `assert.Eventually` (3 min / 10 s for
  Test007, 1 min / 5 s for Test008) to tolerate kubelet ConfigMap propagation
  delay, which can exceed 60 s in k3s.

### Alternatives considered

- Enabling `Debug.EnableAPI` in the Helm chart's default config — rejected
  because it would ship a debug endpoint enabled by default.
- Reducing kubelet sync period — rejected; cluster-wide setting change for a
  test convenience.

## Scope

- **In scope:** `test/k8s-e2e/exporter_test.go` only.
- **Out of scope:** No production code changes. No Helm chart changes.

## Validation

- **Unit tests:** No new unit tests (test-infrastructure fix).
- **k8s-e2e:** 15/15 pass on SMCi MI350X (`smci350-rck-g03-b11-21`) with full
  release image built from upstream/main (v1.5.4).
- **Regression:** Also validated 12/15 on banff MI300X before the fix; same 3
  tests were the only failures.

## Risks and rollback

- **Known risks:** The 3-minute `Eventually` timeout adds wall-clock time to
  Test007 on slow clusters. Acceptable because the alternative is a flaky fail.
- **Rollback plan:** Revert the single commit.
