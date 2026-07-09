# GPUOP-880 — SR-IOV / GIM Host-Side Metric Coverage Closure

- **Date:** 2026-07-07
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** DME `feat/gpuop-880-sriov-gim-coverage`; gpu-agent #76
- **Related issue(s) / JIRA:** [GPUOP-880](https://pensando.atlassian.net/browse/GPUOP-880) (Epic), child stories GPUOP-882/895/899/936/937/938

## Context

In the SR-IOV (GIM hypervisor) deployment, the host-side gpuagent path
(`gimamdsmi/smi_api.cc`) surfaced far fewer GPU metrics than bare-metal: no
violation/throttle stats, no `vcn_busy[]`/`jpeg_busy[]`/`gfx_busy_inst[]`, no
`vram_max_bandwidth`. GIM 8.7 lacks the fixed-struct `amdsmi_get_gpu_metrics_info`,
so the host path must decode the variable-length `amdsmi_get_gpu_metrics` stream.
The GPUOP-880 Epic closes the bare-metal vs SR-IOV `smi_api.cc` delta (27-row Gap
Matrix, Phase A GIM 8.7) and uplifts the GIM SMI lib to 9.0.0.K (Phase B).

This plan covers the **DME-side** work of the Epic. The gpuagent stream-walker
implementation lands via gpu-agent PR #76.

## Approach

- Refresh the vendored SR-IOV gpuagent asset (`assets/gpuagent_sriov_static.bin.gz`)
  containing the GIM stream walker (Phase A 8.7 + Phase B 9.0.0.K, prochot/ppt cases).
- **Never derive** metrics the firmware does not emit: GIM carries no per-`*`
  violation percentages, so they stay at the sentinel and DME suppresses them as
  unsupported (no `acc × 100 / counter` fabrication).
- Add a hardware-bound real-GIM e2e (`test/e2e/exporter_sriov_test.go`,
  `TestSRIOVRealGIM`) that runs the prebuilt sriov image against a live GIM host
  and asserts VCN/JPEG/GFX/UMC coverage with no sentinel leak; skips unless
  `SRIOV_EXPORTER_IMAGE` is set. `test/k8s-e2e/sriov/daemonset.yaml` deploys the
  same on a GIM node.
- Documentation accuracy: machine-readable `metrics-support-matrix.yaml` truth
  source, `metricslist.md` Hypervisor column fix, and a CI doc-audit job
  (`.github/workflows/doc-audit.yml`, `tools/scripts/doc-audit.sh`).

### Alternatives considered

- **Derive percentages in the walker / recording rules** — rejected: violates the
  "never fabricate a metric the source doesn't emit" rule; leaves a false signal
  when the GIM firmware genuinely does not report it.
- **Switch the GIM path to a tarball smi-lib install** — rejected for Phase B;
  vendor the prebuilt `.so` + header as assets instead (no runtime install churn).

## Scope

- **In scope:** DME-side SR-IOV/GIM coverage — GIM lib refresh (9.0.0.K), real-GIM
  e2e + k8s-e2e-sriov tests, deb/rpm sriov libamdsmi.so alias, support-matrix
  truth-YAML, metricslist doc fix, CI doc-audit.
- **Out of scope:** the gpuagent C++ stream-walker itself (gpu-agent PR #76); MI2xx
  host-side coverage; guest-VM passthrough; bad-page / ecc_enabled / RAS
  feature/policy Prometheus surfaces (Gap rows 24–27, deferred — not emitted on
  bare-metal either).

## Validation

- **Unit tests:** VCN/JPEG activity + SR-IOV field tests.
- **Integration / e2e:** real-GIM e2e (`exporter_sriov_test.go`, `TestSRIOVRealGIM`)
  and k8s-e2e-sriov (`sriov/daemonset.yaml`) run against the prebuilt sriov image
  on a live GIM host.
- **Doc-audit:** `doc-audit.sh` CI job checks `metricslist.md` against
  `metrics-support-matrix.yaml`.
- **Manual / hardware:**
  - leto MI210 (gfx90a, GIM 9.0.0.K): 222 `amd_` series, no regression, GIM omits
    the throttle block gracefully (sentinel-suppressed, no fabricated percentages).
  - banff-cyxtera-cx58-11 MI300X (gfx942, GIM 9.0.0.K): throttle/violation block
    emits and live-increments; PPT + socket-thermal residency proven non-zero under
    DGEMM load (750 W / 85 °C). See `docs/GPUOP-880-mi300x-*.md`.

## Risks and rollback

- **Known risks:** vendored gpuagent asset is ABI-coupled to the GIM SMI lib —
  append to shared C structs, never mid-insert (ABI skew → SIGSEGV). Percentage
  suppression relies on the sentinel filter (`IsValueApplicable`); a walker that
  writes 0 instead of leaving the sentinel would leak a false 0.
- **Rollback plan:** revert the branch; the prior `gpuagent_sriov_static.bin.gz` is
  the parent-commit blob. No schema/enum removals, so no forward-compat break.
