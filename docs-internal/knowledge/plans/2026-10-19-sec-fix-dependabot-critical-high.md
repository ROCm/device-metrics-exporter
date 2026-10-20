# Fix critical/high Dependabot alerts via go-helm-client bump

- **Date:** 2026-10-19
- **Author:** praveen
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** NO-JIRA (security remediation)

## Context

`https://github.com/pensando/device-metrics-exporter/security/dependabot` listed 7 open
**critical/high** alerts (confirmed via `gh api repos/pensando/device-metrics-exporter/dependabot/alerts`).
All 7 are Go-module CVEs reached transitively through one direct dependency,
`github.com/mittwald/go-helm-client v0.12.14` — used only in `test/k8s-e2e/clients/helm.go`
(the k8s e2e test harness, not a shipped binary):

| Pkg | Installed | Fixed | CVE |
|---|---|---|---|
| containerd/containerd | v1.7.12 | 1.7.33 | CVE-2026-53488 |
| containerd/containerd | (same pkg) | 1.7.29 | CVE-2024-25621 |
| docker/cli | v25.0.1 | 29.2.0 | CVE-2025-15558 |
| docker/docker | v25.0.6 | none published | CVE-2026-42306 |
| docker/docker | (same pkg) | none published | CVE-2026-41567 |
| moby/spdystream | v0.5.0 | 0.5.1 | CVE-2026-35469 |
| helm.sh/helm/v3 | v3.16.1 | 3.17.4 | CVE-2025-53547 |

Neither track in `.claude/agents/sec-fix-agent.md` fit: this is a third-party Go module bump
(not stdlib-via-trivy, not the docs pip lockfile), so it was handled directly.

## Approach

Investigated the dependency chain rather than bumping `helm.sh/helm/v3` by hand: `go-helm-client`
is an actively maintained wrapper that had already moved its own `helm.sh/helm/v3` pin forward
across several releases before jumping to `helm.sh/helm/v4` at v0.13.0. The last v3-based release,
**v0.12.19**, is a patch-level bump within the same 0.12 minor line as the current v0.12.14 —
verified the exact API surface used by `test/k8s-e2e/clients/helm.go` (`NewClientFromRestConf`,
the `Client` interface's `InstallChart`/`UninstallReleaseByName`/`AddOrUpdateChartRepo`/
`ListDeployedReleases`, plus `ChartSpec`/`Options`/`RestConfClientOptions`) is byte-for-byte
unchanged between v0.12.14 and v0.12.19 — zero source changes needed.

Bumping to v0.12.19 also drops `docker/cli` and `docker/docker` from the dependency graph
entirely (go-helm-client refactored away from the Docker/containerd registry client), closing
3 of the 7 alerts as a side effect — including both "no fix published" `docker/docker` CVEs,
since the vulnerable package is no longer reachable from this module's build graph.

Remaining gap after the v0.12.19 bump: `containerd` landed at v1.7.28 (still short of the 1.7.33
fix) and `moby/spdystream` stayed at v0.5.0 (still short of 0.5.1) — both closed with one more
explicit indirect `go get` each, small patch bumps within lines already in the build.

Rejected alternatives:
- **Bump `helm.sh/helm/v3` directly to latest (v3.22.x):** unnecessary — go-helm-client's own
  v0.12.19 gets everything fixable without touching `helm.sh/helm/v3` by hand or risking a
  6-minor-version helm core jump.
- **Bump `go-helm-client` to v0.13.x:** moves to `helm.sh/helm/v4`, a breaking API change for
  this wrapper library's `Client`/`ChartSpec` shape — far riskier for no additional CVE coverage.

## Scope

- **In scope:** `go.mod`, `go.sum`, `vendor/**` (regenerated via `make mod`, never hand-edited).
- **Out of scope:** the 2 `docker/docker` CVEs with no upstream fix — now unreachable from the
  dependency graph (package absent from `go.mod`/`go.sum` entirely), so there's no residual
  exposure to track, but no fixed version exists if the package were ever reintroduced by a
  future dependency.

## Steps taken

All `go get`/`go mod` work ran inside `${BUILD_CONTAINER}` (`registry.test.pensando.io:5000/device-metrics-exporter-build:v1.18`),
never against the host toolchain:

1. `go get github.com/mittwald/go-helm-client@v0.12.19`
2. `go get github.com/containerd/containerd@v1.7.33`
3. `go get github.com/moby/spdystream@v0.5.1`
4. `make mod` (repo's tidy+vendor target; re-pins the `go` directive and reapplies the existing
   CVE `replace` directives already in the `mod:` target)

## Validation

- `go build ./...` — succeeds inside the container, confirms `test/k8s-e2e/clients/helm.go`
  still compiles against the bumped `go-helm-client` (this package isn't covered by
  `make unit-test`, which only runs `./pkg/...`).
- `make unit-test` — all 14 packages pass (run as root inside `${BUILD_CONTAINER}`, matching
  the `device-metrics-sanity` job target in `.job.yml`; running as a non-root UID-mapped user,
  as `make docker-shell` does for interactive dev, fails unrelated to this change because
  `/var/run/exporter` isn't writable by a non-root UID — the same pre-existing
  container/test-harness gap documented in `2026-10-16-sec-fix-go1.27.2-xnet-cve.md`).
- `go list -m all` / `grep` on `go.mod` confirms: `containerd/containerd v1.7.33`,
  `moby/spdystream v0.5.1`, `helm.sh/helm/v3 v3.19.0`, `mittwald/go-helm-client v0.12.19`, and
  `docker/cli`/`docker/docker` absent entirely.
- `git diff --stat go.mod go.sum` and the large expected `vendor/` diff reviewed; no
  application source changes required.

## Risks and rollback

- Known risk: `go-helm-client` v0.12.19 also pulls in unrelated transitive bumps (k8s.io/* to
  v0.34.0, spf13/cobra, etc., already present elsewhere in the dependency graph at compatible
  versions) — covered by the full `go build ./...` + `make unit-test` pass above.
- Rollback plan: `git revert` this commit; re-run `make mod` to regenerate `go.sum`/`vendor/`
  back to the prior `go-helm-client v0.12.14` pin.
