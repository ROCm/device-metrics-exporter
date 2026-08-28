# Bump gpuagent to ROCm/gpu-agent main @1d816bfe2b42, wire staging build parallelism

- **Date:** 2026-08-27
- **Author:** praveen
- **Related PR(s):** (this PR)
- **Related issue(s) / JIRA:** N/A — dependency bump

## Context

Target commit `1d816bfe2b42c4e8d46eee2d85c6a97f86de6640` (ROCm/gpu-agent#92,
"build: parallelize gpuagent build with correct -j dependency graph") fixes the
inner `sw/nic/gpuagent/Makefile` dependency graph so object compilation
correctly waits on generated protos/third-party libs, enabling safe `-j` object
compilation (~449s serial -> ~108s parallel per upstream commit message).

Bumping `GPUAGENT_COMMIT` alone doesn't realize this speedup here: this repo's
`Dockerfile.exporter-release` invoked the inner build without `-j` at all.

## Approach

- Bump `GPUAGENT_COMMIT` (`5b2bf2cc2b94...` -> `1d816bfe2b42...`) in both pin
  locations: `Makefile`, `docker/Dockerfile.exporter-release`.
- `docker/Dockerfile.exporter-release`: add `-j$(nproc)` to the
  `make -C sw/nic/gpuagent all` invocation in the `gpuagent-build` stage, so the
  staging build (`make gpuagent-build` / `make docker`) actually exercises the
  new parallel dependency graph instead of building serially.

### Alternatives considered

- Expose a `GPUAGENT_JOBS` build-arg (mirroring gpu-agent's own top-level
  `Makefile` knob) instead of a bare `nproc`. Skipped for now — no caller here
  needs to override job count; `nproc` matches the container's available
  cores same as gpu-agent's own default.

## Scope

- **In scope:** `GPUAGENT_COMMIT` pin, `-j` on the in-Dockerfile gpuagent build.
- **Out of scope:** amdsmi/ROCm version, vendor patches (none needed —
  `patch/gpuagent/` is currently empty/`.keep`-only).

## Validation

- **Required before merge:** `make gpuagent-build` (or `/builder` skill) builds
  clean at the new commit; confirm build completes faster with `-j` and all
  four binaries (gpuagent, gpuagent_gim, gpuagent_mock, gpuctl) are produced.
- **Done:** `make gpuagent-build` run locally (8-core host, `AMDSMI_FROM_TARBALL=1`).
  Build succeeded; all four binaries (gpuagent, gpuagent_gim, gpuagent_mock,
  gpuctl) extracted to `build/gpuagent/`. The compile+link step (`make -j8 -C
  sw/nic/gpuagent all`, includes `make gopkglist` + `build-libs` + strip) took
  **1117.3s (~18m37s)** under `-j8`. Total wall-clock including the one-time
  ~9GB ROCm tarball download was ~33.5 min. No prior-pin (non-`-j`) baseline
  was captured on this host for a direct before/after comparison.

## Risks and rollback

- **Known risks:** `-j$(nproc)` on an undersized build host could oversubscribe
  memory during third-party lib compilation — upstream's `.NOTPARALLEL` on
  `Makefile.lib` already serializes the six third-party libs across each other
  to mitigate this (each already runs its own internal `-j$(nproc)`).
- **Rollback:** revert this PR; `GPUAGENT_COMMIT` and the Dockerfile `-j` flag
  return to prior values.
