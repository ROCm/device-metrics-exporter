# Build profiler (rocpctl / librocpclient / counter XMLs) from source

- **Date:** 2026-08-28
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-985 follow-up

## Context

The profiler client (`rocpctl` + `librocpclient.so`) and the ROCProfiler counter
definition XMLs (`basic_counters.xml`, `derived_counters.xml`, `config.yaml`) were
shipped as committed binary blobs under `assets/rocprofiler{,-sdk}/`. This is the
same anti-pattern the gpuagent producer already retired: the blobs drift from the
ROCm tarball they must match, bloat the repo, and hide which ROCm build actually
produced them. The docker image already sourced the XMLs from the tarball; deb/rpm
and the client binaries did not.

## Approach

- Add a rocprofiler producer, mirroring the gpuagent model: `rocpctl` +
  `librocpclient.so` compile once per make invocation from `rocprofilerclient/`
  source against the tarball ROCm, output to `build/rocprofiler/`, gated by a stamp
  dep wired into `docker-image` / `docker-pkgs`. Always from source — no knob, no
  prebuilt-blob fallback.
- The rocprofiler builder image extracts the counter XMLs from the ROCm tarball and
  the producer stages them next to the client binaries, so deb/rpm consume them from
  `build/rocprofiler/` with no separate host-side extraction.
- `git rm` the committed `assets/rocprofiler{,-sdk}` binaries and XMLs and drop their
  `version.yaml` entries (including `profiler_lib`).
- Flip `docker-azure` off the `GPUAGENT_FROM_SOURCE=0` pin to the source producers.

### Alternatives considered

- Keep committed blobs, refresh by hand — rejected: the drift/bloat/opacity problem
  is exactly what this removes.
- Bundle the gpuctl-from-source change in the same commit — rejected: gpuctl is a
  separate concern; split out so this branch is profiler-only.

## Scope

- **In scope:** rocpctl / librocpclient / counter-XML build-from-source producers;
  removal of the committed profiler blobs; docker-azure default flip.
- **Out of scope:** the sriov `gpuctl` build-from-source change (dropped from this
  branch); any exporter runtime / metric-schema change.

## Validation

- Unit tests: unaffected (build-system only).
- Container image: full release image built entirely from source
  (`dme-profiler-test:local`); provenance verified inside — `rocpctl` 7457f37d,
  `librocpclient` a26e61f4, counter XMLs from the tarball at
  `/opt/rocm-*/share/rocprofiler-sdk/`.
- rocprofiler producer: built the builder image and ran the producer — `rocpctl`
  (305cd367), `librocpclient.so` (a26e61f4), and the counter XMLs (basic 0f654642,
  derived f03ec9f6) staged to `build/rocprofiler/` by the builder entrypoint, XMLs
  byte-identical to the prior host-side tarball extraction.
- Debian/RPM: exercised the deb XML-staging step against the producer output — the
  `*_counters.xml` + `config.yaml` land in `usr/local/metrics/share/rocprofiler-sdk/`
  (byte-identical), and the glob excludes the client binaries in the same dir.
  `make -n rpmpkg`/`debpkg` confirm both source XMLs from `build/rocprofiler/`.
- Integration / e2e tests: mock `build_prep` skips the rocprofiler producer (CI fix)
  so `make e2e` (mock image) is unaffected.
- Manual / hardware steps: built the release image from this commit end-to-end
  (gpuagent + gpuctl from source at the rebased pin 1d816bfe — `gpuctl` bf76e58d;
  `rocpctl` 305cd367; counter XMLs at `/opt/rocm-10.0.0rc4/share/rocprofiler-sdk/`)
  and ran it on banff MI300X (baremetal amdgpu, `ProfilerMetrics.all=true`). Verified
  the running container's binaries by md5, then `/metrics` exposed 280
  `amd_gpu_prof_*` families across the 8 GPUs with live counter values. A prior
  session additionally showed idle→load movement under a HIP VALU load (`sq_waves`
  0→24192, `simd_utilization` 0→0.993, satisfying the CI `variation>=1` assertion)
  on the pre-rebase image; also validated on MI210 (gfx90a).

## Risks and rollback

- Known risks: the profiler client is now always built from source — a build
  environment without the ROCm tarball / rocprofiler builder image cannot produce
  it. There is no prebuilt-blob fallback (the committed `assets/rocprofiler*` were
  removed).
- Rollback plan: revert the single commit to restore the committed blobs.
