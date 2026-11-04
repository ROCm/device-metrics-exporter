# Bump docs-build pip floors: certifi 2026.10.18, gitpython 3.2.1

- **Date:** 2026-11-04
- **Author:** Nitish Bhat
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** NO-JIRA (security remediation)

## Context

A trivy source scan (`trivy fs`, HIGH/CRITICAL, fixable only) of the repo
flagged two findings in `docs/sphinx/requirements.txt`, the pip-compiled
lockfile used to build the docs site:

- `certifi` 2024.8.30 — GHSA-6q48-6c7w-9gw2 (CRITICAL), fixed in 2026.10.18.
- `gitpython` 3.2.0 — CVE-2026-114145 (HIGH), fixed in 3.2.1.

Both are transitive dependencies of `rocm-docs-core` (via `requests` and
directly). They affect only the docs build; neither is in any shipped binary.

## Approach

- Add a `certifi==2026.10.18` floor and bump the existing `gitpython` floor
  to `3.2.1` in the "Security floors" block of `docs/sphinx/requirements.in`,
  matching the repo's existing rule of pinning to the lowest patched release.
- Regenerate `docs/sphinx/requirements.txt` with `pip-compile
  --output-file=requirements.txt requirements.in` under Python 3.10 (matching
  `.readthedocs.yaml`), without `--upgrade`, so every other pin is unchanged.
  The resulting diff is limited to the `certifi` and `gitpython` pins and the
  `# via` provenance lines for `certifi`. The lockfile is not hand-edited.

### Alternatives considered

- Hand-editing only the two pin lines — rejected; the lockfile is generated and
  prior changes regenerated it with `pip-compile`.
- Waiting for `rocm-docs-core` to pull patched versions — it does not yet, and
  the floors block exists for exactly this.

## Scope

- **In scope:** the two docs lockfile findings above.
- **Out of scope:** Go toolchain/module CVEs (separate PR #1711), container
  image scanning, and any other pip pins.

## Validation

- `pip-compile` under Python 3.10 (python:3.10 container); diff against the
  original lockfile contains only the `certifi` and `gitpython` changes.
- Docs build in a clean python:3.10 container (writable copy of the repo):
  `pip install -r docs/sphinx/requirements.txt`, then `sphinx-build -b html
  docs`. `pip check` reports no broken requirements; the build succeeds
  (exit 0, 21 warnings). A baseline build using `main`'s original lockfile
  (certifi 2024.8.30, GitPython 3.2.0) produces the identical warning set, so
  no new warnings come from this change.

## Risks and rollback

- Known risks: a newer `certifi` / `gitpython` could in principle break the
  docs build or `rocm-docs-core`; the clean-container docs build above guards
  against that.
- Rollback plan: `git revert` the commit.
