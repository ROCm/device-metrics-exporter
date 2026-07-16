# GPUOP-991: claim Radeon AI support in docs, add missing 1.5.1 release notes

- **Date:** 2026-07-15
- **Author:** praveen
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** https://pensando.atlassian.net/browse/GPUOP-991

## Context

1.5.1 GA shipped Radeon AI (MI350P-class) support, but some user-facing docs
still described the exporter as Instinct-only. Two merged 1.5.1 fixes
(PR #1436, PR #1386) were also missing from `docs/releasenotes.md`. The bug
additionally asked for a pointer to the SR-IOV exporter release, which ships
as a separate docker-only image (`device-metrics-exporter-sriov:1.0.0`) for
MI-series hosts only.

## Approach

- `docs/developerguide.md`: reword the GPU Agent integration blurb from
  "AMD Instinct GPUs" to "AMD Instinct and Radeon AI GPUs".
- `docs/releasenotes.md`: add "Issues Fixed" bullets for #1436 (lazy k8s
  watcher startup + state-change Events) and #1386 (`GPU_CPER_MAX_AGE`),
  referenced without internal ticket names per the bug request; add a "New
  Platform Support" bullet for the `device-metrics-exporter-sriov:1.0.0`
  image.
- `docs/index.md`: add a note that the SR-IOV variant is MI-series only,
  with the image name/tag, a link to the upstream MxGPU-Virtualization
  releases page, and a link to the metrics list for the full
  hypervisor/guest metric support matrix (already documented there via
  per-metric Hypervisor/Baremetal columns).
- `.wordlist.txt`: added `MxGPU` for spellcheck.

### Alternatives considered

- Duplicating the hypervisor-supported metric list in `index.md` — rejected;
  `docs/configuration/metricslist.md` already carries this per-metric via
  Hypervisor/Baremetal columns, so linking avoids drift between two sources
  of truth.

## Scope

- **In scope:** `docs/developerguide.md`, `docs/releasenotes.md`,
  `docs/index.md`, `.wordlist.txt`.
- **Out of scope:** `docs/configuration/agfhc.md` — verified AGFHC recipes
  are MI300x-only in `pkg/testrunner/agfhc.go`, so the existing
  "Instinct GPU models" wording there is accurate and unrelated to this bug.
  No code changes — this is a docs-only bug.

## Validation

- Unit tests: N/A — no code changed.
- Integration / e2e tests: N/A.
- Manual: reviewed diff for accuracy against merged PR history (`git log`,
  `gh pr view`) and existing doc cross-references; `make docs-lint` requires
  the build container (TTY-only), not run in this session.

## Risks and rollback

- Low risk — documentation only, no runtime behavior change.
- Rollback: revert the commits on this branch.
