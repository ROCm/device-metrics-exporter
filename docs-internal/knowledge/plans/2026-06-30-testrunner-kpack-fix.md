# Test Runner: Fix minihpl/GEMM Failures via .kpack Directory

- **Date:** 2026-06-30
- **Author:** yan.sun3@amd.com
- **Related PR(s):** follows #1410, #1421
- **Related issue(s) / JIRA:** GPUOP-948

## Context

After PR #1421 (hipblaslt library data + per-arch symlinks), minihpl and
AGFHC `all_lvl5`/`single_pass` recipes continued failing with
`hipErrorInvalidKernelFile` in the TheRock tarball-based test runner image.

Root cause investigation by comparing the TheRock tarball image against a
native nightly package install (`amdrocm7.14-gfx950` from
`rocm.nightlies.amd.com`) where minihpl PASSES:

- Native package ships `.kpack/blas_lib_gfx950.kpack` (34MB) alongside the
  rocblas library
- Our tarball prune copies `librocm_kpack.so` (the extraction library) but
  never copies the `.kpack/` directory from the tarball root
- `librocm_kpack.so` calls into the `.kpack` archive at runtime to extract
  the correct GPU kernel variant — without the archive, every GEMM call
  fails with `hipErrorInvalidKernelFile`

The TheRock multiarch tarball ships per-arch `.kpack` files for all supported
GPU architectures: `blas_lib_gfx950.kpack`, `blas_lib_gfx942.kpack`,
`blas_lib_gfx90a.kpack`, etc. Copying the entire `.kpack/` directory covers
all GPU targets, not just MI350P.

## Approach

Add `.kpack/` directory copy to the testrunner profile prune in
`docker/install-rocm-tarball.sh`, alongside the existing `rocblas/` and
`hipblaslt/` library data copies.

## Validation

Tested on MI350P node (3× AMD Instinct MI350P, device 0x75a8,
TheRock 7.14.0rc0, AGFHC 1.32.0):
- `minihpl` standalone: **PASS**
- `single_pass`: in progress
- `all_lvl5`: in progress
