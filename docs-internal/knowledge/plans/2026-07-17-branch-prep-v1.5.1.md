# Branch prep: v1.5.1 release branch cutover

- **Date:** 2026-07-17
- **Author:** praveen
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** none (no-jira, release/branch tooling)

## Context

`main` already carries the v1.5.1 doc/version bump from PR #1425
(`docs: v1.5.1 release notes and version bump`, commit `44671392b`) and the
later `hack/bump-version.sh` addition (commit `8f02b7030`). That earlier pass
covered `docs/conf.py`, `Makefile`, `docs/releasenotes.md`, and the
doc/helm/Dockerfile files listed in `hack/bump-version.sh`'s GPU-track table.

Cutting the actual `release-v1.5.1` branch (mirroring the prior
`release-v1.5.0` cutover, commit `ec0d7e641`) surfaced a few stragglers still
pinned to `v1.5.0` that the earlier docs-only pass didn't touch, plus the
cherry-pick routing config that only makes sense once the release branch
exists:

- `helm-charts/Chart.yaml` and `helm-charts/values.yaml` — chart
  `version`/`appVersion` and image `tag` were still `v1.5.0`.
- `docker/Dockerfile.exporter-release` — image `LABEL version=`/`release=`
  were still `v1.5.0`.
- `docs/integrations/slurm-integration.md` — a `docker run` example still
  pinned `rocm/device-metrics-exporter:v1.5.0`. This file is not in
  `hack/bump-version.sh`'s tracked file list (gap in that script — see
  Risks below), so neither the #1425 pass nor a re-run of the script caught
  it; found by grepping for remaining `v1.5.0` references across `docs/`.
- `branch_policy.yml` — Auto Cherry Picker's `cherry_pick_branches` pointed
  at `collab-2.0.0` and `rocm/device-metrics-exporter:main`, both stale now
  that development has moved past the collab branch and the release branch
  exists. Re-pointed to `main` (so `release-v1.5.1` fixes flow forward) and
  `rocm/device-metrics-exporter:release-v1.5.1` (so they also land on the
  ROCm upstream release branch), matching the pattern from the v1.5.0
  cutover (`ec0d7e641`: `cherry_pick_branches: [] → [main]` at that time).

Re-running `hack/bump-version.sh v1.5.1` against current `main` after this
change confirmed no further diff (idempotent) except the notice that
`docs/releasenotes.md` already has its `## v1.5.1` section.

## Approach

- Manually corrected the four stray `v1.5.0` references (Chart.yaml,
  values.yaml, Dockerfile.exporter-release, slurm-integration.md) to
  `v1.5.1`, matching the anchored-pattern style `hack/bump-version.sh`
  already uses for these exact files (chart `version:`/`appVersion:`, values
  `tag:`, Dockerfile `version=`/`release=` labels).
- Updated `branch_policy.yml`'s `cherry_pick_branches` list to `main` and
  `rocm/device-metrics-exporter:release-v1.5.1`.

### Alternatives considered

- **Extend `hack/bump-version.sh` to cover `slurm-integration.md`**:
  reasonable follow-up, but out of scope here — this PR is a one-off branch
  prep, not a script-maintenance change. Tracked as a known gap instead of
  fixed in place, to keep this diff scoped to version strings only.

## Scope

- **In scope:** `branch_policy.yml`, `docker/Dockerfile.exporter-release`,
  `helm-charts/Chart.yaml`, `helm-charts/values.yaml`,
  `docs/integrations/slurm-integration.md`.
- **Out of scope:** re-running the full `hack/bump-version.sh` file set
  (already applied via #1425 and confirmed idempotent), adding
  `slurm-integration.md` to the script's tracked list, any code/behavior
  changes.

## Validation

- `git diff` reviewed by hand — five files, version-string-only changes.
- `bash hack/bump-version.sh v1.5.1` re-run against the working tree after
  these edits produced no additional diff, confirming no other GPU-track
  file still references `v1.5.0`.
- `grep -rn "v1.5.0" docs/ helm-charts/ docker/ Makefile branch_policy.yml`
  (excluding `docs/_build/` and `docs/releasenotes.md`'s historical `##
  v1.5.0` section, which is intentionally left as-is) returns no remaining
  matches.

## Risks and rollback

- **Known risks:** `hack/bump-version.sh`'s GPU-track file list doesn't
  include `docs/integrations/slurm-integration.md`, so a future version bump
  will silently miss it again unless the script is updated separately.
- **Rollback:** version-string-only edits to tracked files; `git checkout --
  <file>` reverts cleanly, no generated code or migrations involved.
