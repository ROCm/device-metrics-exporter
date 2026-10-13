# Implementation plan — high-res GPU profiler streaming (P7)

- **Date:** 2026-10-06
- **Branch:** `user/spraveenio/profiler-highres-stream` (worktree off `single-source-store`@929c279e2)
- **Design:** `2026-10-06-highres-gpu-profiler-streaming.md`
- **Related:** folded into the high-res handoff as packet P7; peer agent owns the
  `single-source-store` store refactor (PR #1663).

## Context

Serve `gpu_prof_*` in high-res mode from an exporter-owned long-lived
`rocpctl --stream` child feeding the snapshot store, instead of today's per-scrape
`rocpctl` exec. Additive: the per-scrape path (pull / non-high-res / PTL) is unchanged.
See the design doc for the full rationale, alternatives, and PTL analysis.

## Global Constraints

Binding on every task and every review:

- **Additive only.** The one-shot `rocpctl` path and `pkg/amdgpu/rocprofiler/rocpclient.go`
  per-scrape behavior must remain byte-for-byte unchanged. No regression to pull /
  non-high-res / PTL deployments.
- **Build discipline.** Never `go build`/`go test` on the host — only inside the build
  container (`${BUILD_CONTAINER}` resolved from the Makefile). Never run `make docker`
  locally. The C++ `rocpctl` (rocprofilerclient/) cannot be built locally — it is
  verified by jobd after push; local "done" for the C++ task means code-complete +
  self-reviewed, not executed.
- **Metric prefix.** Profiler gauges are `amd_gpu_prof_*`. Grep `^amd_gpu_prof_`, never
  `^gpu_prof_`.
- **Mode gate.** The stream manager starts only when high-res mode is on AND profiler is
  enabled AND `ProfilerConfig.PtlDelay == 0`. `PtlDelay > 0` (operator-declared PTL
  platform) keeps the per-scrape path; PTL high-res profiler is unsupported by design.
- **NDJSON contract.** Streaming output is one JSON object per line, each identical in
  shape to today's one-shot `rocpctl` output (the object the existing
  `rocpclient.go getMetrics` unmarshals into `amdgpu.GpuProfiler`). The Go decoder
  unmarshals line-by-line into `amdgpu.GpuProfiler`, reusing that shape.
- **Cadence.** `--interval` = the high-res fast-poll period (`GetHighResPeriods()` fast
  value, seconds → ms), uniform with the watch fields. `-d` stays
  `ProfilerConfig.SamplingInterval` (the per-read integration window), unchanged.
- **Reuse the failure policy.** Supervision reuses the existing
  `IncFailureCount` / `SetFatalFailureState` / `GetDisabledReason` / `SetEventEmitter`
  on `ROCProfilerClient` — do not fork a second failure model.
- **Process hygiene.** Spawn the child with `SysProcAttr{Setpgid: true,
  Pdeathsig: syscall.SIGKILL}`. Shutdown signals the child's process group then waits;
  `cmd.Wait()` reaps it. No orphaned `rocpctl` on `kill -9` of the exporter.
- **Comments.** Technical, bare-minimum — only non-obvious WHY. No internal FQDNs,
  ports, usernames, or registry URLs in committed files.

## Task 1 — Go `ProfilerStreamManager` (pkg/amdgpu/rocprofiler/)

**Goal.** A supervised long-lived profiler stream client that spawns
`rocpctl --stream --interval <ms> -d <us> <fields>`, decodes NDJSON lines into
`amdgpu.GpuProfiler`, and publishes samples through an injected writer interface;
supervises the child (restart-with-backoff, fatal-disable on crash), reconfigures on
demand, and drains gracefully.

**Files.** New `pkg/amdgpu/rocprofiler/profiler_stream_manager.go` and
`pkg/amdgpu/rocprofiler/profiler_stream_manager_test.go`. Do not modify `rocpclient.go`
except to add exported hooks if strictly required (prefer composition).

**Shape.**
- A `StreamSink` interface (defined here) with a method to publish a decoded
  `*amdgpu.GpuProfiler` (per-device field map). The concrete store wiring is Task 3 —
  Task 1 depends only on this interface, enabling isolated testing.
- `ProfilerStreamManager` owns: the `*ROCProfilerClient` (for fields + failure policy),
  the sink, the interval/duration, the child `*exec.Cmd`, and a lifecycle context.
- `Start(ctx)`: spawn child with `Setpgid`+`Pdeathsig`; goroutine scans stdout lines,
  `json.Unmarshal` each into `amdgpu.GpuProfiler`, publish to sink; stderr → logger.
- Supervisor: on child exit, apply failure policy (`IncFailureCount`; on
  dumped/aborted → `SetFatalFailureState`); restart with backoff (base 10s, exponential
  cap) unless disabled.
- `Reconfigure(fields, intervalMs, durationUs)`: kill+respawn with new args.
- `Stop(ctx)`: cancel → SIGTERM the process group → bounded wait → SIGKILL → `Wait()`.

**Tests (TDD, container).** Drive with a mock `rocpctl` (see `tools/mock-rocpctl/`,
extend as needed) emitting scripted NDJSON / exit codes: decode → sink receives expected
samples; child-exit → restart within backoff; dumped/aborted → fatal-disable (no
restart); `Stop` drains with no leaked process; `Reconfigure` respawns with new args.
Use a fake clock / short backoff in tests. Table-driven where natural.

**Done when.** Container `go vet` + `go test ./pkg/amdgpu/rocprofiler/` pass; `gofmt`
clean.

## Task 2 — C++ `rocpctl --stream` mode (rocprofilerclient/)

**Goal.** Add a long-lived streaming mode to `rocpctl` that initializes the sampler once
and emits NDJSON continuously, leaving the one-shot path unchanged.

**Files.** `rocprofilerclient/rocpctl.cpp` (arg parsing + stream loop), and
`rocprofilerclient/RocpCounterSampler.{h,cpp}` only if a reusable init/sample split is
needed (prefer adding a `runStream()` beside `runSample()`).

**Shape.**
- Add `-s/--stream` and `-i/--interval <ms>` flags. Absent `--stream`, behavior is
  identical to today (`runSample()` → print → exit).
- Stream mode: initialize samplers once; loop — `sample_counters_with_packing(...)` →
  serialize the same JSON object the one-shot path prints → write as one line to stdout
  → flush → sleep `interval`. Honor SIGTERM/SIGINT for clean loop exit.
- `-d <duration_us>` (integration window) and `-p <ptl_delay_ms>` keep their current
  meaning. On the high-res stream path the caller passes `-p 0` (PTL gate guarantees
  `PtlDelay == 0`).

**Verification.** Cannot build locally. Code-complete + self-review; actual build via
jobd `build-device-metrics-exporter-gpu` after push; runtime via P5 HW items 11–12.

**Done when.** Diff is self-consistent, matches the NDJSON contract exactly (the Go
decoder in Task 1 must parse it), one-shot path provably untouched, and the self-review
records the deferred jobd verification.

## Task 3 — store wiring + mode gate (gpuagent) — base-dependent

**Base.** The peer's `single-source-store` refactor is now pushed. Rebase this branch
onto the peer's confirmed-green FINAL SHA before implementing (candidate `215ffb7b8`,
pending the peer's e2e-green confirmation — do NOT rebase onto an unconfirmed SHA). That
refactor: `snapshotstore.StoreWriter`/`StoreReader` are the layer seam (`*Store`
satisfies both); GPU/NIC/IFOE all store-serve uniformly;
`applyWatchSnapshot` is store-only; the high-res feed is reconciled in
`GPUAgentClient.InitConfigs` (enable starts the consumer, disable deletes the GPUWatch
object); `snapshotstore.Store.Reset()` is called on config reload (writers repopulate
next cycle).

**StoreWriter API the StreamSink writes through** (`pkg/exporter/snapshotstore/store.go`):
- `Update(uuid string, deltas map[string]snapshotstore.Sample)` — merges; never clobbers
  other writers' fields.
- `UpdateIfChanged(uuid string, deltas map[string]snapshotstore.Sample)`.
Mirror how `applyWatchSnapshot` builds its deltas in `gpuagent_gpu_highres.go`.

**Wire:**
- `highResProfilerFieldSet` (parallel to `highResWatchFieldSet`), as one delimited block.
- Profiler fields added to the slow-writer skip-mask so the writers don't clobber.
- A `StreamSink` implementation (satisfying Task 1's interface) that maps a decoded
  `*amdgpu.GpuProfiler` to per-UUID `map[string]snapshotstore.Sample` deltas and calls
  `Update`. Re-publishes on its next sample after a `Store.Reset()`.
- Failure-path clearing: when the profiler stream is disabled/failed (manager
  `IsDisabledOnFailure()` / child down), CLEAR the profiler fields this sink owns rather
  than serve them stale. Mechanism (from the peer): `StoreWriter.DropFields(drop
  map[string]struct{})` — pass the set of profiler store field keys the StreamSink owns
  (reuse `highResProfilerFieldSet`). It removes those fields from every device slot, drops
  slots left empty, and leaves the node `*_total` slot + `gpu_health` + all non-profiler
  fields untouched — uniform with the peer's `dropStaleGPUTelemetry`
  (`gpuagent_gpu_store.go`) / `dropStaleHighResTelemetry` (`gpuagent_gpu_highres.go`).
  Reconnect/re-enable re-publishes via `Update()` and the fields merge back.
- Mode gate + lifecycle: start/stop the `ProfilerStreamManager` alongside the GPUWatch
  feed reconciliation in `InitConfigs` (enable→Start, disable→Stop) when the gate passes:
  high-res AND profiler enabled AND `ProfilerConfig.PtlDelay == 0`; `--interval` from
  `GetHighResPeriods()` (fast period, s→ms). On shutdown, pass the manager a `Stop` ctx
  ≥ its `stopTimeout` (carries the Task 1 deferred-minor note).

**Done when.** Container gate passes on the rebased tree; profiler fields served from the
store in high-res; per-scrape path unchanged when gated off (`PtlDelay>0` / non-high-res).
