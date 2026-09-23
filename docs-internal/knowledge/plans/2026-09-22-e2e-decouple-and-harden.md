# Decouple e2e test from mock-image build, harden Test022

- **Date:** 2026-09-22
- **Author:** —
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** NO-JIRA (CI reliability)

## Context

The `e2e-build-mock-exporter` jobd build runs `make e2e` which both
builds the mock Docker image AND runs the full e2e test suite.  When
the e2e suite flakes, jobd marks the build as failed and refuses to
copy the mock tarball artifact — blocking `device-metrics-sanity` and
all downstream consumers even though the artifact was produced
successfully.

Test022 (`Test022LoggerConfigUpdate`) is the primary flake source.
It asserts on `"loading new config on"` in the container log after a
config-reload cycle.  Two independent CI runs on different runners
failed identically: the exporter log timestamps never advanced past
initial startup — the fsnotify event for the second `WriteConfig` was
never delivered.  Root cause: inotify on Docker bind-mounts under
nested docker with the vfs storage driver intermittently drops events.

## Approach

**1. `.job.yml` — split build from test:**
- `e2e-build-mock-exporter` (build): `make docker-mock` only; artifact always publishes.
- New `e2e` (target): `make e2e` (build + test); runs independently, failure does not gate `device-metrics-sanity`.
- Follows the gpu-operator `e2e-sim` pattern.

**2. Harden Test022:**
- Check for `"level=debug"` (written post-reload by the new lumberjack) instead of `"loading new config on"` (written pre-reload, may be in a rotated-away segment when MaxSize shrinks from 10→1 MB).
- Re-write the config file on each failed poll inside `assert.Eventually` so a missed fsnotify event is retried on the next write.
- Widen all `CheckExporterLogForString` assertions from 10s/1s to 30s/2s (`docker cp` is slow under vfs).

### Alternatives considered

- Increase timeout only — insufficient; if fsnotify never delivers the event, no timeout helps.
- Add a polling fallback to the exporter's config watcher — correct long-term fix but larger scope; out of band for this CI-reliability fix.

## Scope

- **In scope:** `.job.yml` target split, Test022/Test020 assertion hardening.
- **Out of scope:** Exporter-side config watcher changes, other e2e tests.

## Validation

- Local e2e: 30/30 passed in Docker-in-Docker with vfs storage driver (same as CI).
- Test022 matched on `"level=debug"` (confirmed in log output).
- `.job.yml` validated as correct YAML; structural invariants verified (build produces artifact, target runs tests independently, sanity depends on build not test).

## Risks and rollback

- Known risks: the `e2e` target runs `make e2e` which rebuilds the mock image redundantly (once in the build, once in the target).  Acceptable cost for decoupling; can be optimized later with artifact sharing.
- Rollback plan: revert the commit; no runtime code changes.
