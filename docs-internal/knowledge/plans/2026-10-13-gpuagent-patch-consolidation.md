# Consolidate the gpuagent patch set and re-sync it with aicc-dev PR #5405

- **Date:** 2026-10-13
- **Author:** praveen
- **Related PR(s):** this PR; aicc-dev #5405
- **Related issue(s) / JIRA:** NO-JIRA

## Context

PR #1663 merged the single-source store / high-res GPUWatch work along with a
five-file gpuagent patch set. That patch set and aicc-dev PR #5405 — the
upstream home of the same GPUWatch change — had drifted apart in both
directions:

- The patch set tracked aicc-dev commit `3569a4fd7` (Oct 6). The last five PR
  commits, including `2b3b3d587` "address GPUWatch review comments" (Oct 8),
  were never back-ported, so the review fixes were missing from the DME build.
- The patch set carried DME-only work — the energy attr, a GPUGet-from-watcher
  cache, and amd-smi call reduction — that was never upstreamed, so the fix
  keeping amd-smi stable at 64 partitions was absent from the PR under review.

It had also fragmented into five files patching the same feature and largely
the same handful of source files.

## Approach

One patch file, `patch/gpuagent/0001-gpuagent-highres-gpuwatch.patch`, holding
the union of the old `0001`/`0002`/`0004`/`0005` plus the back-ported review
fixes. No build plumbing changes: `Makefile`, `docker/build_prep_docker.sh`,
and `docker/Dockerfile.exporter-release` all glob `*.patch`.

Dropped from the set entirely:

- **Old `0003`** (serve GPUGet from the watcher's gpu_metrics cache). Redundant
  — all four exporter `getGPUs` sites are gated off in high-res
  (`UpdateMetricsStats`, `UpdateStaticMetrics`, `QueryMetrics`, and
  `processHealthValidation` → `processEccHealthFromStore`), leaving only the
  subscribe-time call in `StartHighRes` while the watcher is not streaming.
  There is no concurrent amd-smi caller for it to guard against. It was also
  still labelled `TEST(option#3)`.

Back-ported from PR #5405 into the patch:

- `aga_gpu_watch_reset_fast_fields()`, called after the `slow_cache_` seed in
  both SMI backends. `slow_cache_` is written only on slow-refresh ticks and
  captures the whole record including fast fields, so without this a failed
  fast read republishes a value up to `SlowPeriodSecs` old.
- `ecc_read_succeeded`: ECC correct/uncorrect/deferred totals report
  `UINT64_MAX` when every per-block read failed, instead of a real-looking `0`.
- The full units table (MB / C / Watts / % / MHz / KB / GB/s / GT/s) replacing
  the `// TODO add units for other ids` stub.
- Input validation in `gpu_watch_to_spec.hpp` and `gpu_watch_svc.hpp`: checked
  `aga_obj_key_proto_to_api_spec()` returns, zero-GPU / zero-attr rejection,
  an `AGA_MAX_GPU_WATCH` bound on subscribe, and a NUL-terminate after the
  `strncpy` into `req.client`.
- `AGA_GPU_WATCH_MAX_INDEXED_VALS` 24 → 40, so JPEG arrays are no longer
  clamped.
- C++11 style: `std::make_unique` → `std::unique_ptr<T>(new T())` at the four
  patch-introduced sites (pre-existing base uses are left alone).
- The gpuctl indexed-attribute printer, with the stream table's attribute
  column now sized to the longest label rather than a fixed `%-30s`.

Pushed the other way, into aicc-dev PR #5405 (staged locally, not pushed):

- The live half of old `0005` — `MEM_CLOCK` from bulk `current_uclk` and
  `GPU_TEMP` from bulk `temperature_edge`, with discrete fallbacks.
- The `ENERGY_CONSUMED` attr (id 139), so it ships on both sides.
- The same gpuctl alignment fix.

After this, the only intended differences between the two trees are the
gimamdsmi lazy-init / per-tick UUID→handle resolve (DME-only, SR-IOV), the
license headers, and the Go import paths.

### Alternatives considered

- **Keep five patch files.** Rejected: they all patch one feature and the same
  files; splitting made the drift harder to see and the set harder to review.
- **Also reset slow-tier fields to NA on a failed slow read.** Rejected: it is
  untested behavior, and slow counters dropping out mid-run would break the
  "consistent metric count" soak criterion. Carry-forward is retained.
- **Move `AMDSMI_COUNTER_RESOLUTION` to a shared header** so the watcher could
  use the named constant. Rejected: it adds two files to a PR already under
  review for a cosmetic gain. Both sides keep the literal fallback.
- **Port 0005's `if (refresh_slow)` guards** around `PCIE_BANDWIDTH` and
  `GPU_MEMORY_USAGE`. Rejected: both ids are already in `k_slow_attrs` and the
  watch loop skips slow attrs on fast ticks, so the guards are unreachable.

## Scope

- **In scope:** the consolidated patch file; the two watcher-tier handoff docs
  that named the old filenames and the superseded "add a `0002-*.patch`"
  convention.
- **Out of scope:** exporter Go code (untouched); the dead `if (refresh_slow)`
  guards; the duplicated `15.3` literal; upstreaming the gimamdsmi lazy-init
  path.

## Validation

- **Patch applies:** `git apply --check` then `git apply` against a fresh
  `sw/nic/gpuagent` export of the pinned base (`bcae57f9`, `Makefile`
  `GPUAGENT_COMMIT`), plus a simulation of the Dockerfile's
  `for p in /patches/*.patch` loop. Both clean — `git apply` has no fuzz
  tolerance, so this is the gate that matters.
- **No content lost in the merge:** the tree produced by the consolidated patch
  is byte-identical (`diff -r`) to the tree built from the four old patches plus
  the new edits.
- **Convergence:** per-file comparison of every GPUWatch source file against the
  aicc-dev PR head, with comments, license headers, and import paths filtered —
  zero differing lines in all of them except `gpu_watch_api.cc`, whose single
  difference (`walk` vs `read_walk`) is an aicc-dev-only db API.
- **Go:** `gofmt -l` and `gofmt -e` clean on both copies of `gpu_watch.go`.
- **Unit tests:** not run — no exporter Go code changed.
- **Hardware:** still to run. The one behavior delta vs the #1663 soak is the
  fast-tier NA reset. Re-run the CPX-64 soak on the 8-GPU MI325X: 30 min at 1s
  high-res, no restart/segfault, ECC/violation/XGMI counters steady. Expect a
  fast-tier metric to intermittently drop out of `/metrics` on a failed read —
  that is the reset working, not instability, and the count check needs to
  distinguish the two.

## Risks and rollback

- **Fast-tier NA reset changes `/metrics` shape.** A failed fast read now drops
  the metric for that scrape instead of emitting a stale value. Correct, but it
  is the one change that can move the metric count during a soak.
- **gpuagent is not compiled locally** — it builds inside the image build, which
  runs in CI. A compile error in the patched C++ surfaces there, not here.
- **Rollback:** revert this commit; the five original patch files return intact
  and the gpuagent build goes back to the #1663 behavior.

## Update 2026-10-19 — patch dropped, commit bumped

The consolidated patch content landed upstream: `ROCm/gpu-agent#106`
("gpuagent: consolidate high-res GPUWatch patch, back-port review fixes"),
merged into `ROCm/gpu-agent` `main` at `e888c802b85efc44cc6657297f6cf95e70b5a27a`.
That merge also folded in a follow-up upstream commit fixing repo coding-style
violations (declaration ordering, enum/proto alignment) in the patch content.

- `Makefile` `GPUAGENT_COMMIT` moved from `bcae57f9b39` to `2c967f0209c6` —
  upstream `main` tip, two commits past the PR #106 merge.
- `patch/gpuagent/0001-gpuagent-highres-gpuwatch.patch` removed; `patch/gpuagent/`
  is now empty again (`.keep` retained).
- Verified equivalence: applied the (now-removed) patch to a fresh export of
  the old pinned base `bcae57f9` and diffed every touched file against
  `e888c802`; the only differences are the upstream style-cleanup commit's
  local-variable reordering and enum/proto column realignment — no functional
  change.
- Supersedes the "pushed the other way, into aicc-dev PR #5405 (staged
  locally, not pushed)" line above — the consolidated change went upstream as
  `ROCm/gpu-agent#106` instead.
- Also picks up `ROCm/gpu-agent#108` ("security: bump x/net to v0.60.0 and pin
  CI Actions to commit SHAs"), merged immediately after #106: fixes 5 Trivy
  CVEs in the vendored `sw/nic/gpuagent/go.mod` (`golang.org/x/net` v0.58.0 ->
  v0.60.0 — CVE-2026-78669 high; CVE-2026-78663/97032/78659/78660 medium) plus
  Dockerfile Trivy/zizmor findings in the gpu-agent repo's own CI. Confirmed
  the #106->#108 delta touches only `go.mod`/`go.sum`, no overlap with the
  GPUWatch files.
  - **Risk:** #108 bumps the vendored `go.mod` `go` directive to `1.27.2`; our
    build container's Go toolchain (`tools/base-image/Dockerfile`) is pinned
    at `1.25.13`. The in-image `gpuagent-build` stage will need `GOTOOLCHAIN=auto`
    to fetch `1.27.2` on the fly (requires network access at that build step),
    or the builder image's Go version needs bumping. Not addressed here —
    first CI run after this PR is the gate.
