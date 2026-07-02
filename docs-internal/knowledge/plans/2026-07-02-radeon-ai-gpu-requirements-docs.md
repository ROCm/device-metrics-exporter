# Add Radeon AI platform to GPU requirements in docs main page

- **Date:** 2026-07-02
- **Author:** praveen
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** N/A

## Context

The v1.5.1 release added support for MI350P and Radeon AI platforms (ROCm 7.13+),
but the docs main page (`docs/index.md`) still listed only MI2xx/MI3xx under GPU
requirements with a single ROCm 6.2.0 line. This caused the requirements section
to be misleading: Radeon AI users need ROCm 7.13 or later, not 6.2.

The release notes already document the split requirements correctly; the index page
needed to match.

## Approach

Split the single ROCm requirement line into two bullet points, one per platform tier:

- MI2xx or MI3xx platform: ROCm 6.2.0 or later
- MI350P or Radeon AI platform: ROCm 7.13 or later

This preserves existing MI2xx/MI3xx guidance while surfacing the Radeon AI entry
point for new users landing on the main docs page.

### Alternatives considered

- Add a compatibility table (similar to NIC section) — rejected as over-engineering
  for two tiers; bullet points are sufficient and consistent with the current style.

## Scope

- **In scope:** `docs/index.md` requirements section under GPU Metrics.
- **Out of scope:** Metrics list pages, configuration docs, NIC docs, release notes
  (already correct).

## Validation

- Visual review of rendered Markdown output.
- No code or config changes; no unit/integration tests required.

## Risks and rollback

- Known risks: None — documentation-only change.
- Rollback plan: Revert commit.
