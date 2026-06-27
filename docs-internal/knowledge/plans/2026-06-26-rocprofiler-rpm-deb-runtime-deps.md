# Make rocprofiler GPU_PROF_* metrics work in rpm/deb (7.14)

- **Date:** 2026-06-26
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-923

## Context

In container images the profiler resolves counters against the full `/opt/rocm`
tree. The rpm/deb packages do not ship `/opt/rocm`, so `GPU_PROF_*` metrics
silently failed to initialize when DME was installed from a package: the
rocprofiler-sdk runtime libs and the counter catalog were missing, so
rocprofiler could not enumerate counters and the profiler fields were dropped.

The ROCm 7.14 uplift (libamd_smi.so.26.5.0) added new transitive runtime
dependencies (`librocm_sysdeps_*`, `libLLVM`, `libclang-cpp`, `libatomic`) that
the package builder did not previously bundle, which is what surfaced this gap.

## Approach

- Extend `tools/rocprofiler-libbuilder/entrypoint.sh` to copy the rocprofiler-sdk
  transitive runtime deps (`librocm_sysdeps_*`, `libLLVM`, `libclang-cpp`,
  `libatomic`, `libnuma`) into `build/rocprofilerdeplib/` so they land in the
  package under `/usr/local/metrics/lib`.
- Install `libatomic` in all three builder Dockerfiles (rhel9, ubuntu22,
  ubuntu24) so the lib is present to be bundled.
- Ship the rocprofiler-sdk counter catalog
  (`assets/rocprofiler-sdk/config.yaml` -> `/usr/local/metrics/share/rocprofiler-sdk/`)
  so the profiler can resolve counters without `/opt/rocm`. Replaces the stale
  `counter_defs.yaml` with the current `config.yaml` from the 7.14 tarball.
- Bump rpm/deb ROCm defaults to 7.14 in the Makefile to match `dev.env`.

### Alternatives considered

- Symlink/require a host `/opt/rocm` — rejected: packages must be
  self-contained; a host ROCm install is not guaranteed.
- Build rocprofiler from source in the package — rejected: heavy, and the
  prebuilt runtime libs + catalog are sufficient.

## Scope

- **In scope:** rpm/deb packaging of rocprofiler runtime deps + counter catalog;
  builder Dockerfile libatomic install; ROCm 7.14 default bump.
- **Out of scope:** container-image profiler path (already works via `/opt/rocm`);
  gpuagent/amd_smi asset refresh (done in separate 7.14 commits).

## Validation

- Unit tests: n/a (packaging change).
- Integration / e2e tests: built deb (UBUNTU22) and rpm; confirmed both bundle
  the 47 rocprofiler runtime deps + `share/rocprofiler-sdk/config.yaml`.
- Manual / hardware steps: installed the deb in a GPU-passthrough container on a
  live MI300A (ROCm 7.14, libamd_smi.so.26.5.0), enabled
  `GPUConfig.ProfilerMetrics.all=true`, ran a HIP burn workload, and scraped
  `/metrics`: **35 `amd_gpu_prof_*` series** emitted with non-zero counters
  (e.g. `amd_gpu_prof_cpc_cpc_stat_busy` ~1.3e7). rocprofiler initialized from
  the bundled catalog with no `/opt/rocm` present.

## Risks and rollback

- Known risks: larger package size (deb ~84MB -> ~145MB) from the bundled libs.
- Rollback plan: revert this commit; packages return to the prior (profiler-less
  from-package) behavior. No runtime config or schema change involved.
