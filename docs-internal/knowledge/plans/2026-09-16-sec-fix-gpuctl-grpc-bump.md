# Bump gpuagent to ROCm/gpu-agent @4ce653089c0 (grpc v1.83.2, CVE-2026-33186)

- **Date:** 2026-09-16
- **Related PR(s):** (this PR)
- **Related issue(s) / JIRA:** NO-JIRA — security scan remediation

## Context

The security scan on build 33692365 (commit 18e3f39dc) flagged gpuctl with
CVE-2026-33186 (CRITICAL, grpc). PR #1611 remediated CVEs in the four DME Go
binaries by bumping grpc to v1.83.2 in DME's `go.mod`, but gpuctl and gpuagent
are built from ROCm/gpu-agent which has its own `go.mod`. At the pinned commit
(`d8e52aa28a58`), gpu-agent replaced grpc to v1.82.1 — still vulnerable.

gpu-agent PR #96 (`4ce653089c0`) bumped grpc to v1.83.2 on `main`, closing the
gap. This PR picks up that fix.

## Approach

- Bump `GPUAGENT_COMMIT` from `d8e52aa28a58c144bfe3d6c779df636670749d2b` to
  `4ce653089c031b53cbc962f5e494e2447e8e3f0c` in
  `docker/Dockerfile.exporter-release`.
- No `GO_VERSION` change needed — already at 1.25.13 in both repos.

### Alternatives considered

- Patch gpu-agent's `go.mod` via the DME patch-gpuagent mechanism — rejected
  because the upstream fix already exists; a pin bump is cleaner.

## Scope

- **In scope:** `GPUAGENT_COMMIT` pin in `docker/Dockerfile.exporter-release`.
- **Out of scope:** DME `go.mod` (already fixed in #1611), amdsmi assets,
  ROCm version pins.

## Validation

- CI build at the new gpuagent commit must succeed (`make docker`).
- Post-build trivy scan of gpuctl should show no grpc CVEs.

## Risks and rollback

- **Known risks:** gpu-agent @4ce653089c0 also includes Dependabot config
  changes (PR #95/#96) — no functional gpuagent code changes beyond the
  `go.mod`/`go.sum` bump.
- **Rollback:** revert this PR to restore `GPUAGENT_COMMIT` to `d8e52aa28a58`.
