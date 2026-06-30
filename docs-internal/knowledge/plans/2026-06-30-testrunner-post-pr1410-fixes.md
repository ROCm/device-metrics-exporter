# Test Runner Post-PR#1410 Follow-up Fixes

- **Date:** 2026-06-30
- **Author:** yan.sun3@amd.com
- **Related PR(s):** follows #1410
- **Related issue(s) / JIRA:** N/A

## Context

After PR #1410 (TheRock tarball + MI350P RVS folder selection) merged, hardware
validation on a real MI350P node revealed additional issues:

1. **hipBLASLt missing from TheRock prune** — RVS `gst_single` and AGFHC `gfx_*`
   tests both use `blas_source: hipblaslt`. The TheRock tarball pruning kept
   `libhipblaslt.so` but omitted `lib/hipblaslt/` (the Tensile kernel data directory).
   Without it, all GEMM-based tests fail with `hipErrorInvalidKernelFile`.

2. **Per-arch kernel subdirs not visible to older binaries** — TheRock 7.14+ stores
   rocblas and hipblaslt kernels in per-arch subdirs (`library/gfx950/`, etc.) but
   the flat `library/` directory expected by older binaries is empty for newer arches.
   Symlinks from flat → subdir fix both RVS and AGFHC lookups.

3. **Review feedback addressed** — Copilot and spraveenio comments from PR #1410:
   - `parseAMDSMILimitOutput` simplified from `(bool, error)` to `bool`
   - `install-rocm-tarball.sh` consolidated (testrunner copy deleted, shared via `--profile`)
   - `RVS_TARBALL_URL` grouped with other URL vars in `Makefile` and added to `dev.env`
   - Base image bumped to UBI9 9.8
   - EPEL double-install fixed in both Dockerfiles

## Changes

Single file change in this PR: `docker/install-rocm-tarball.sh`

- Extended with `--profile exporter|testrunner` argument
- testrunner profile copies `lib/hipblaslt/` alongside `lib/rocblas/`
- After copying, symlinks all files from per-arch subdirs (`gfx950/`, `gfx942/`, etc.)
  to the flat `library/` dir so both old and new rocblas/hipblaslt lookup paths work

## Validation

Tested on MI350P node (3x AMD Instinct MI350P, device 0x75a8):
- RVS `gst_single`: all 6 actions PASS (bf16, fp16, fp8, fp4, bf8, fp6)
- AGFHC `gfx_lvl1`: runs to completion, failures are hardware throttling only
- AGFHC `all_perf`: 4/12 pass, failures are hardware BW/perf targets only
- All other RVS and AGFHC recipes (lvl1–lvl4, dma, hbm, pcie, acf): consistent
  with hardware-level failures, no library loading errors
