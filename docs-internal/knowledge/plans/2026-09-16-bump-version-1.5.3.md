# Bump GPU version to v1.5.3

- **Date:** 2026-09-16
- **Author:** praveen
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** N/A

## Context

Routine GPU-track version bump from v1.5.2 to v1.5.3 via `hack/bump-version.sh`.
While auditing script coverage against all `v1.5.2` references in the repo, two
files were found that carry the same version-tag pattern as files the script
already updates, but were missing from its file lists:

- `docker/testrunner/Dockerfile` — same `LABEL version="..." release="..."`
  pattern as `docker/Dockerfile.exporter-release`, which the script does cover.
- `docs/integrations/slurm-integration.md` — same `rocm/device-metrics-exporter:vX.Y.Z`
  image-tag pattern as the other docs the script globally seds.

A PR review (Copilot) additionally flagged that `helm-charts/README.md` —
generated from `Chart.yaml`/`values.yaml` by `helm-docs` (`make helm-docs`,
`Makefile:833`) — was left stale at `v1.5.0` despite the chart files bumping
to `v1.5.3`.

## Approach

- Extended `hack/bump-version.sh` (GPU track) to also update:
  - `docker/testrunner/Dockerfile` LABEL `version`/`release` fields (same sed as
    `Dockerfile.exporter-release`).
  - `docs/integrations/slurm-integration.md` image tag (added to the existing
    generic `vX.Y.Z` sed loop).
  - `helm-charts/README.md` — version badges and default `image.tag` table
    row patched via the same generic `vX.Y.Z` sed used elsewhere in the
    script. Normally this file is regenerated via `make helm-docs`, but the
    script intentionally never invokes make/build targets, so it patches the
    version strings directly instead.
- Re-ran `hack/bump-version.sh v1.5.3` against the working tree; confirmed via
  `git diff` that only the newly-covered files changed — all already-bumped
  files were no-ops, confirming the script is still idempotent.

### Alternatives considered

- Leave the two files unbumped and fix manually each release — rejected,
  reintroduces the same gap next release.

## Scope

- **In scope:** `hack/bump-version.sh` GPU-track file list/sed additions;
  version string updates across all GPU-track files (docs, Makefile,
  helm-charts, Dockerfiles) from v1.5.2 → v1.5.3, including
  `helm-charts/README.md`'s version badges/table row.
- **Out of scope:** `Makefile:241` `DEBIAN_VERSION := "1.5.2"` fallback (used
  only when `$(RELEASE)` has no `exporter`/`nic`/`v` prefix; comment indicates
  it's an intentional apt-release cap, not a live version tracker — left
  untouched pending confirmation with packaging owner). NIC track unaffected.

## Validation

- Unit tests: N/A (docs/version-string change only, no code logic).
- Integration / e2e tests: N/A.
- Manual: `git diff --stat` reviewed after re-running the script; confirmed
  exactly the two intended files changed with no unintended side effects on
  the previously-applied v1.5.3 bump.

## Risks and rollback

- Known risks: low — string substitution only, scoped to version tags/labels.
- Rollback plan: revert this commit; `hack/bump-version.sh` changes are purely
  additive (new files/sed lines), safe to revert independently of the version
  string changes if needed.
