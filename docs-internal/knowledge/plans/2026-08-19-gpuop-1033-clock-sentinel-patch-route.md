# GPUOP-1033 — video/data clock min/max sentinel via gpuagent patch (v1.5.2)

- **Date:** 2026-08-19
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-1033

## Context

On MI210, `gpu_min_clock` / `gpu_max_clock` with `clock_type=video` and
`clock_type=data` are exported as `0`, while `amd-smi` reports N/A for VCLK0/DCLK0
min/max. Root cause is in gpuagent: `amdsmi_get_clock_info` returns
NOT_SUPPORTED and the error branch leaves the zero-initialized `clock_spec`
lo/hi at 0, which the exporter treats as a real value. Fixed on MI210
(suppressed) and verified safe on MI300X (real values kept) — baremetal PF and
VF/GIM.

## Approach

Deliver the gpuagent fix to DME v1.5.2 as an in-tree **patch** rather than a
`GPUAGENT_COMMIT` bump, to avoid landing the fix across multiple gpuagent
branches first.

- Add `patch/gpuagent/0002-gpuop-1033-clock-sentinel.patch` (from the gpuagent
  commit, based on `22f92dc`; applies cleanly against the release pin `9cee401`).
- `GPUAGENT_COMMIT` unchanged (`9cee401`). The docker build's gpuagent-build
  stage applies `patch/gpuagent/*.patch` via `git apply`
  (`docker/build_prep_docker.sh` + `Dockerfile.exporter-release:96-98`).
- No asset repackaging: rpm/deb also build gpuagent from source in-image and
  apply `patch/gpuagent/*.patch` (`GPUAGENT_FROM_SOURCE=1`, the default —
  `Makefile.package:90-93`). The committed `assets/gpuagent_static.bin.gz` is
  used only in the `GPUAGENT_FROM_SOURCE=0` fallback, so the patch alone covers
  every default delivery path.

### Alternatives considered

- `GPUAGENT_COMMIT` bump — rejected: forces updating multiple gpuagent branches.
- Repackage the prebuilt asset too — rejected: unnecessary. rpm/deb build from
  source and apply the patch by default; the asset is a non-default fallback.

## Scope

- **In scope:** patch file only (covers docker + rpm/deb, both from-source).
- **Out of scope:** GPUAGENT_COMMIT bump; asset repackaging; gpuagent branch
  changes; the sibling pensando/sw PR (repo retirement pending gpuagent-owner
  confirmation).

## Validation

- `make docker` — confirm build log applies `0002-gpuop-1033-...patch`.
- Run image on local MI210 (gfx90a): `gpu_{min,max}_clock{clock_type=video|data}`
  rows ABSENT (suppressed); gfx/mem/soc/fclk min/max retained; video/data
  frequency present.
- Asset: repackaged `gpuagent_*.bin.gz` gpuagent contains the sentinel path.

## Risks and rollback

- Low risk: 6-line change, video/data-only, HW-validated on MI210 + MI300X.
- If the pinned gpuagent later advances past where the patch applies, in-image
  `git apply` fails loudly → switch to commit-bump.
- Rollback: remove the patch file and revert the asset.
