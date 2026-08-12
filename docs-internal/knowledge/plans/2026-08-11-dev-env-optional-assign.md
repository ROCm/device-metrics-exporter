# Make all dev.env build variables overridable with ?=

- **Date:** 2026-08-11
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** NO-JIRA

## Context

`dev.env` holds the build-time configuration (builder tags, base images,
registry) consumed by the Makefiles. Most entries used recursive `=`
assignment, which unconditionally sets the value and silently ignores any
value already present in the environment. Only `DOCKER_REGISTRY` and the
`ROCM_*` / `RVS_*` entries used `?=` (conditional assignment).

The inconsistency means a developer or CI job cannot override, say,
`DOCKER_BUILDER_TAG` or `BUILD_BASE_IMAGE` by exporting it before invoking
`make` — the `=` assignment in `dev.env` clobbers it. This blocks
per-developer and per-pipeline overrides of builder images and tags without
editing the tracked file.

## Approach

Convert the remaining recursive (`=`) assignments in `dev.env` to conditional
(`?=`) so every build variable can be overridden from the environment,
matching the already-optional `DOCKER_REGISTRY` / `ROCM_*` entries.

- 14 assignments changed from `=` to `?=`.
- Values and `$(VAR)` references are unchanged — this is a pure
  assignment-operator change.
- Variables that reference others (e.g. `INSECURE_REGISTRY ?= $(DOCKER_REGISTRY)`)
  keep their reference; `?=` still expands the referenced variable when no
  environment override is present.

### Alternatives considered

- **Command-line overrides (`make VAR=... target`)** — rejected: does not
  compose across the multi-step build targets, must be repeated on every
  invocation, and does not help tooling that only exports environment
  variables.
- **Wrapper script that exports each variable** — rejected: duplicates the
  defaults that already live in `dev.env`, creating a second source of truth
  that drifts.

## Scope

- **In scope:** `dev.env` assignment operators only.
- **Out of scope:** Makefile default logic, any change to variable *values*,
  and the `ROCM_*` defaults (those remain authoritative in the Makefile as
  noted in `dev.env`).

## Validation

- Unit tests: n/a (build configuration change).
- Integration / e2e tests: n/a.
- Manual: `git show` confirms the diff is limited to `=` → `?=` with no value
  changes; an environment override (e.g.
  `DOCKER_BUILDER_TAG=custom make docker-shell`) is now honored, whereas a
  default invocation still resolves to the in-file value.

## Risks and rollback

- Known risks: negligible — conditional assignment falls back to the same
  default when no override is set, so existing builds are unaffected.
- Rollback plan: revert the commit.
