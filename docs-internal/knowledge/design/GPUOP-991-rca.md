# Root-Cause Analysis: GPUOP-991

## Bug Summary
1.5.1 GA release documentation should claim Radeon AI Pro support wherever it
currently claims Instinct-only, and release notes should reference the fixes
shipped in 1.5.1 without internal ticket names.

## Environment
- Docs are Sphinx-based, under `docs/`, published via `docs/conf.py` /
  `docs/sphinx/_toc.yml`.
- Current released version: 1.5.1 (`docs/conf.py:9-10`).

## Symptom
Documentation classification: **conditional** (docs-only, no code path).
Reporter observed that some user-facing pages still describe the exporter as
Instinct-only even though Radeon AI (MI350P-class) support shipped in 1.5.1,
and that `docs/releasenotes.md` doesn't mention two additional 1.5.1 code
changes (PR #1436, PR #1386).

## Prior Investigation
None — first triage of this bug.

## Root Cause

### What / Where
1. `docs/developerguide.md:206` — "GPU Agent... to configure and monitor AMD
   Instinct GPUs" is stale; Radeon AI is supported per `docs/index.md:20` and
   `docs/releasenotes.md:6`.
2. `docs/releasenotes.md` v1.5.1 section is missing two merged fixes:
   - PR #1436 (`580d9f037`, merged to main): k8s node/pod watchers now start
     lazily based on `HealthService.Enable` / `ExtraPodLabels`, plus new
     `K8sWatcherStateChanged` / `ProfilerStateChanged` k8s Events.
   - PR #1386 (`fbe2cd8ce`, merged to main): new `GPU_CPER_MAX_AGE` health
     threshold to filter stale fatal CPER records (already documented in
     `docs/configuration/configmap.md:15` and
     `docs/configuration/configuration-settings.md:8`, but not called out in
     release notes).
   PR #1452 (CVE docs metadata) is already reflected in `docs/releasenotes.md`
   — no gap there.
3. SR-IOV: `docs/knownissues.md` and `docs/configuration/metricslist.md`
   already scope SR-IOV/GIM/`vm_vf` behavior correctly; there is no
   Instinct-only miswording to fix. Per the bug description, add an explicit
   note that the SR-IOV exporter variant is released only for MI-series
   (Instinct) platforms, pointing to the upstream release page.

### Why / How
These are stale/incomplete docs, not a code defect — writer omissions when
each PR landed (PR #1436 and #1386 didn't touch `docs/releasenotes.md`; the
Radeon platform-support blurb in `developerguide.md` predates Radeon support
and was never revisited).

### Checked and ruled out
- `docs/installation/gpu-operator.md`, `docs/configuration/configmap.md`
  references to `instinct.docs.amd.com` — a URL domain, not a support claim.
- `docs/configuration/agfhc.md:101` "Instinct GPU models" — verified against
  `pkg/testrunner/agfhc.go`; AGFHC recipes are MI300x-only in code, so this
  wording is accurate and out of scope.
- AGFHC/testrunner code path — no Radeon-related string or logic found; no
  code change needed, docs-only bug.

## Proposed Fix

### Changes Required
| File | Change | Reason |
|------|--------|--------|
| `docs/developerguide.md` | Reword line 206 to "AMD Instinct and Radeon AI GPUs" | Stale Instinct-only claim |
| `docs/releasenotes.md` | Add bullets under v1.5.1 "Issues Fixed" for #1436 (lazy k8s watchers + profiler/watcher state Events) and #1386 (`GPU_CPER_MAX_AGE`) | Missing from release notes, no internal ticket names per bug request |
| `docs/index.md` | Add a short note under GPU Metrics requirements that the SR-IOV exporter variant is MI-series only, linking `https://github.com/amd/MxGPU-Virtualization/releases` | Requested in bug description |

### Risk Assessment
Low — documentation-only change, no code/build impact.

### Dependencies
None.

## Test Plan
- Manual review: `make docs` (or Sphinx build) if available, otherwise visual
  diff review.
- Confirm no other "Instinct"-only claims remain that contradict Radeon
  support (grep sweep already done in RCA).
