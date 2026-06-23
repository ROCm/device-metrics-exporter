# GPUOP-907: gpuagent SIGSEGV on MI350P from clock-freq OOB read

- **Date:** 2026-06-23
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-907

## Context

gpuagent crashes with a deterministic SIGSEGV on MI350P (gfx950, PCI
`1002:75a8`) on every `gpuctl show gpu`. Faulting thread is
`grpcpp_sync_ser` at IP `0x116384f`; backtrace runs through
`aga::smi_gpu_fill_status` → `gpu_entry::fill_status_` → `gpu_entry::read`
→ `GPUSvcImpl::GPUGet`.

Root cause: `smi_fill_clock_status_` did `freq.frequency[freq.current]`
with no bounds check. `amdsmi_frequencies_t.frequency[]` is a fixed-size
array (`AMDSMI_MAX_NUM_FREQUENCIES` = 33). On gfx950, amdsmi returns
`AMDSMI_STATUS_SUCCESS` for the DF (DATA) clock type but with a garbage
`current` index (and garbage `num_supported`). The old code trusted
SUCCESS and indexed past the array → out-of-bounds read → SIGSEGV.

Why MI350P only: gfx942 (MI300) returns a valid in-bounds `current` /
`num_supported`, so the OOB never fires there. Same gpuagent code path;
the difference is amdsmi's return metadata, not a per-ASIC branch. This is
why the crash cannot be reproduced on MI300A or with the mock gpuagent.

## Approach

Defensive bounds-clamping in gpuagent before indexing the frequency array.

- `smi_utils.hpp`: add `current_frequency_mhz(amdsmi_frequencies_t*)`
  helper that clamps `current` to a valid index (`current < num_supported`
  AND `current < AMDSMI_MAX_NUM_FREQUENCIES`, else 0) before indexing.
- `smi_utils.hpp`: `find_low_high_frequency` clamps `num_supported` to
  `AMDSMI_MAX_NUM_FREQUENCIES` before building the supported-freq vector.
- `smi_api.cc`: the three clock-type fills (FABRIC/DCE/PCIE) use
  `clock_status->frequency = current_frequency_mhz(&freq);`.

The fix lives in the gpu-agent source and is delivered to this repo via
two paths:

- **Docker image** builds gpuagent from source in-image and applies
  `patch/gpuagent/0002-gpuop-907-clock-freq-oob.patch`.
  `docker/build_prep_docker.sh` stages `patch/gpuagent/*.patch` into
  `docker/patch-gpuagent/` (a build artifact, git-ignored), and
  `Dockerfile.exporter-release` `git apply`s each patch in the build.
- **RPM/DEB packages** consume the prebuilt
  `assets/gpuagent_static.bin.gz` (rebuilt with the fix) via
  `Makefile.package` (`tar -xf`).

### Alternatives considered

- Trust amdsmi `current`/`num_supported` and skip clamping — rejected;
  this is exactly the upstream metadata that is garbage on gfx950.
- Per-ASIC branch (special-case gfx950) — rejected; the unbounded index
  is wrong on any ASIC. A bounds check is the correct general fix.
- Fix only in the docker (source-build) path — rejected; the RPM/DEB
  package path ships the static asset, so it must carry the fix too.

## Scope

- **In scope:** clock-frequency OOB read in `smi_fill_clock_status_`;
  source-of-truth patch `patch/gpuagent/0002`; rebuilt
  `assets/gpuagent_static.bin.gz`; `.gitignore` for the generated
  `docker/patch-gpuagent/` staging dir.
- **Out of scope:** unrelated amdsmi hardening (overdrive, kfd-pid,
  bad-page, CPER bounds/OOM) held for separate review; gpu.proto /
  gpu_to_proto.hpp are untouched.

## Validation

- Unit tests: n/a (gpuagent C++ change; not covered by DME Go unit tests).
- Integration / e2e tests: mock e2e is regression-only — the mock
  gpuagent serves canned data and does not exercise the real amdsmi
  clock-fill path, so it cannot validate this fix.
- Manual / hardware (MI350P, gfx950):
  - WITHOUT fix (baseline binary): `gpuctl show gpu` → server EOF; gdb
    caught `grpcpp_sync_ser` SIGSEGV @ `0x116384f` via
    `smi_gpu_fill_status` — the exact documented signature.
  - WITH fix (UBI9-built binary): `gpuctl show gpu` run 4× all exit 0,
    "No. of gpus : 3", gpuagent survives; clock path logs DCE/PCIe errors
    gracefully instead of crashing.
  - Docker delivery image (`device-metrics-exporter:dev-gpuop907-714-rhel9`,
    ROCm 7.14.0a20260618) validated on MI350P hardware.

## Risks and rollback

- Known risks: the static asset fix is not symbol-verifiable
  (`current_frequency_mhz` is `static inline` and the shipped ELF is
  stripped); trust rests on the verified patch + fresh rebuild + MI350P
  hardware validation of the docker image.
- Rollback plan: revert this commit; the patch and the rebuilt asset are
  self-contained and the change is additive (a clamp), so reverting
  restores prior behavior.
