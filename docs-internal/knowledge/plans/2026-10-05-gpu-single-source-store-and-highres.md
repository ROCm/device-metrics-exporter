# GPU single-source store + high-resolution GPUWatch feed (single `/metrics` path)

- **Date:** 2026-10-05
- **Author:** AMD Device Metrics Exporter team
- **Related PR(s):** #1663 (supersedes the stacked high-res draft #1658)
- **Related issue(s) / JIRA:** N/A

> Consolidated design for the whole body of work: a single in-memory source of
> truth for GPU, NIC, and IFOE telemetry, written once from the hardware and read
> only by `/metrics`. The high-resolution GPUWatch feed updates that same store.
> Replaces the seven iteration/handoff plan docs that preceded it.

## Context

The exporter serves GPU, NIC, and IFOE telemetry on a single path: one in-memory
snapshot store, written once from the hardware and read only by `/metrics`. The
high-resolution GPUWatch feed (fast writer) and the health-interval slow writer
update that same store; `/metrics` serving the latest snapshot is the sole way to
get data. Hardware measurement showed the store-served `/metrics` round-trip is
fast (p50 1.8–5.1 ms; 20 concurrent scrapes 17–32 ms).

The non-high-res pull path mirrors each scrape's `GPUGet` into the same store, so
both modes share one storage + retrieval mechanism.

## Approach

### Part A — High-resolution GPUWatch feed (store writer)

- **Config:** `CommonConfig.HighResolution { Enable, FastPeriodSecs, SlowPeriodSecs }`
  (`exporterconfig.proto`). `Enable=true` turns on the high-res profile; the
  periods tune the GPUWatch fast/slow sampling tiers.
- **Consumer:** a single-read GPUWatch subscriber (one `amdsmi_get_gpu_metrics`
  per sample) applies each snapshot to the single-source store via
  `applyWatchSnapshot`, which returns the store deltas it wrote. No frame
  building, no publish — store-only.
- **Field coverage (option i):** high-res is **watch-only** — the fields the
  GPUWatch feed carries. Non-watch, GPUGet-only fields are supplied by the slow
  writer (Part B), not by a per-scrape `GPUGet`.
- **Indexed / array-valued attrs:** families such as the violation residency
  counters and per-clock values are fanned out into per-index samples with an
  index label; id-map wiring exists in both directions (aga→proto and proto→aga).

### Part B — Single-source store (Model B), used uniformly

- **Store** (`pkg/exporter/snapshotstore/`): a neutral per-device snapshot store
  owned by `pkg/exporter`, **not** the Prometheus registry. Per-device
  `atomic.Pointer[DeviceSnapshot]`; reads are lock-free; writes clone→apply→swap
  (copy-on-write). The top-level device map is itself an `atomic.Pointer` that is
  clone-swapped, so readers never see a torn map and multiple clients can read
  concurrently while a write is in flight (integrity under concurrent scrape).
- **Clean Go interfaces at the seam:** `StoreWriter { Update; UpdateIfChanged }`
  and `StoreReader { Snapshot; Snapshots; Devices }`. `*Store` satisfies both;
  writers (GPU/NIC/IFOE) take `StoreWriter`, `NewStoreCollector` takes
  `StoreReader`. Only Go interfaces cross the layer boundary.
- **Generic `StoreCollector`:** a `prometheus.Collector` over a `StoreReader` plus
  per-field descriptors (name, help, type, ordered label names), emitting one
  const-metric per (device, field). Reused by GPU, NIC, and IFOE.
- **Writers mirror the already-populated GaugeVecs** via Prometheus
  introspection, so the store is structurally identical (metric names + label
  sets) to the legacy pull output — the parity gate is byte-identical output.
- **Uniform serving:** the snapshot store is the **only** serving path for GPU,
  NIC, and IFOE. One storage + retrieval mechanism keeps the three agents
  consistent.
- **Concurrency win:** `NeedsPerScrapePull()` on each client (`MetricsInterface`)
  + `AnyClientNeedsPerScrapePull()` on the handler. GPU, NIC, and IFOE all report
  `false` (store-served), so `prometheusMiddleware` skips the mutex and the
  reset+`UpdateMetrics`, doing a singleflight store refresh instead → concurrent
  lock-free scrapes.
- **High-res → store (fast writer):** `applyWatchSnapshot` writes the watch
  fields into the store per snapshot. `/metrics` reads that same store — one
  source of truth.
- **Slow writer (non-watch backfill):** on the health-polling interval
  (`GetHealthPollingInterval`, 30s default) the health loop (`StartMonitor`) runs
  one `getMetricsAll` populate and mirrors **only the non-watch** GaugeVec
  families (`gpu_health`, `gpu_energy_consumed`, `gpu_afid_errors`, profiler, …)
  into the store, skipping the watch set. `Store.Update` merges deltas so the fast
  writer's fresh watch values are never clobbered. This gives high-res `/metrics`
  full field parity with the pull path **without** a per-scrape `GPUGet`, so the
  throughput win is preserved.
- **Config field-disable** (`GPUConfig.Fields` allowlist) is honored on `/metrics`:
  `applyWatchSnapshot` always applies `getExporterFieldState`, so disabled fields
  are dropped identically to the pull path.
- **Node-level totals** (`gpu_nodes_total`, `nic_total`, `ifoe_total_*`) are
  **unconditional / always-present** — they feed the health service and act as a
  `gpu_health` signal, so they live in a reserved node slot and are exempt from the
  config field-disable gate. They are **boot/reconfiguration-time constants**
  (device counts), so `Store.UpdateIfChanged` stores them only when the count
  changes — never re-written on every scrape.
- **Async pull-path workload labels (GPU + IFOE):** the pull path no longer calls
  the scheduler-blocking `ListWorkloads()` on every scrape — a `pullWorkloadCache`
  refreshes workloads on the 3s cadence off the hot path, so scrapes read the last
  snapshot lock-free (labels ≤3s stale).
- **NPD (`AMDGPUHandlerPrefix` = `/gpumetrics`)** is unchanged behaviorally and
  reads from the same single store — one source of truth, pull-based, simpler.

### Part C — High-res ↔ non-high-res lifecycle (SMI/KFD discipline)

- Resolution/config changes flow **through the MetricsHandler**, which dispatches
  `InitConfigs` to each registered client (GPU, NIC, IFOE) on startup and on every
  3s config reload. No lifecycle or agent logic lives in `pkg/exporter`.
- The composite `GPUAgentClient.InitConfigs`, **after** every sub-client's state is
  set, calls `reconcileHighRes` on the GPU client (optional-interface dispatch):
  - **enable → start** the GPUWatch consumer on a context derived from the agent
    lifetime; gpuagent holds SMI **persistently** while watching.
  - **disable → cancel** the consumer and **delete the GPUWatch object**
    (`GPUWatchDelete`), releasing gpuagent's SMI/KFD hold so it returns to **lazy
    SMI** for the per-scrape `GPUGet` path (`AGA_SMI_LAZY_INIT`).
- `Close()` tears the feed down on shutdown (same cancel + delete). Idempotent.

### Part D — Fetch-failure behavior (serve only the health signals)

On a data-fetch failure the store must not keep serving stale device telemetry —
`/metrics` shows only the always-present signals (the node `*_total` slot and
per-GPU `gpu_health`) until the source recovers, then repopulates. The health
service (polled every 30 s over its own socket, see the health doc) remains the
authoritative health source for the k8s device plugin; `/metrics` mirrors it.

- **Mechanism:** `snapshotstore.Store.DropFields(drop)` (on the `StoreWriter`
  seam) removes the named telemetry fields from every device slot and drops any
  slot left empty. It is field-scoped — a writer passes only its own telemetry
  field names, so a GPU failure never touches NIC/IFOE slots (disjoint names), and
  the signal fields (`gpu_health`, `*_total`) are never in a drop set.
- **GPU non-high-res:** on a `getMetricsAll` (GPUGet) error, `fetchMetricsOnce`
  calls `dropStaleGPUTelemetry` (GaugeVec family names minus the two signals). The
  next successful scrape merges the full set back (the client reconnects via
  `getMetricsAll`'s `InitClients`/`isActive`).
- **GPU high-res:** the feed runs in `runHighResFeed`, a reconnect loop — it
  (re)subscribes, consumes until the stream fails, then `DropFields(highResWatch
  FieldSet)` evicts the watch telemetry (keeping `gpu_health` from the slow writer
  and `gpu_nodes_total`) and reconnects after a short backoff; `applyWatchSnapshot`
  repopulates on the next frame. The loop is the sole owner of the high-res
  connection state; teardown cancels its context and waits on a done channel
  before any new session starts (no two feeds overlap), and the workload-label
  cache is cancelled/replaced per (re)connect so it never leaks.
- **NIC / IFOE:** same policy for consistency — on a fetch failure each drops its
  own telemetry fields, keeping `nic_total` / `ifoe_total_*`.

### Alternatives considered

- **Model A (Prometheus registry as the source of truth)** — rejected: the
  registry's single-writer reset model fights concurrent lock-free reads and
  couples storage to the exposition format.
- **High-res option (ii): GPUGet backfill of non-watch fields on scrape** —
  rejected: re-introduces a per-scrape `GPUGet` and loses the throughput win.
- **GPUWatch field-pruning by config** — deferred (see Future work).

## Scope

- **In scope:** uniform GPU/NIC/IFOE snapshot store + writers + `StoreCollector`;
  the high-res GPUWatch feed into the store; slow-writer non-watch backfill;
  config field-disable on `/metrics`; high-res↔non-high-res lifecycle (delete
  watch → lazy SMI); NPD served from the same store.
- **Out of scope:** the 10ms north-star cadence (1s delivered now); GPUWatch
  attribute pruning (Future work).

## Validation

- **Unit tests (build container `v1.17`, `-race`):** store parity (store serving
  == legacy GaugeVec output, byte-identical), singleflight (≤1 fetch under N
  concurrent scrapes), high-res slow-writer (non-watch field set absent before the
  slow tick, equals the pull-path set after, watch fields + `gpu_nodes_total`
  preserved), applyWatchSnapshot store-delta assertions, cutover test. `gpuagent`,
  `nicagent`, `exporter/config`, `snapshotstore` all pass (only the pre-existing
  `pkg/exporter/logger` race is skipped).
- **Development hardware evidence (store vs legacy serving):** the store serving
  path matched legacy output and delivered the throughput win:
  - **leto (single-GPU):** non-high-res parity identical (338≡338 signatures);
    high-res store-served ⊇ legacy pull-path (`only-in-legacy = 0`),
    `amd_gpu_health` + `amd_gpu_energy_consumed` present (slow-writer), plus the 12
    GPUWatch-only attrs (`package_power` + `violation_*` residency). Throughput
    ~5× concurrent (20 scrapes 213ms→43ms); high-res p50 10.6ms→6.0ms.
  - **MI350P (galena-466c):** non-high-res identical (128≡128); high-res store ==
    legacy pull (128≡128); 20 concurrent scrapes 18.6ms, p50 1.9ms. Config
    field-disable honored (pull 231→18). Caveat: host enumerated 1 GPU (shared
    worker state), so multi-GPU breadth was not exercised.
- **Final unified verification:** re-verify on leto + MI350P: `/metrics` serves the
  fast snapshot; GPU/NIC/IFOE all store-served; labels 1-1 with the prior pull
  output; config field-disable drops fields on `/metrics`; NPD `/gpumetrics` 200;
  **high-res→non-high-res toggle deletes the GPUWatch object and returns gpuagent
  to lazy SMI** (watch gone, KFD hold released).

## Risks and rollback

- **Rollback:** revert the single commit — serving returns to the legacy GaugeVec
  path. No runtime state depends on the store.
- **Known risks:**
  - High-res field coverage is bounded by the watch set + slow-writer backfill; a
    field neither watched nor populated by `getMetricsAll` is absent until gpuagent
    adds the attribute. Accepted.
  - Sub-interval samples on `/metrics` are overwritten before a scrape sees them
    (pull is scrape-cadence). By design — `/metrics` is the single path.

## Future work

- [ ] **Prune the GPUWatch subscription by config field-disable (TODO / when needed).**
  Today `highResWatchAttrs` subscribes the *full* attribute set and `applyWatchSnapshot`
  masks config-disabled fields after the fact (`getExporterFieldState`), so gpuagent still
  samples everything — unlike the pull path's `gpuGetFilter`, which is built from config.
  Build the `GPUWatchSpec.Attribute` set from the enabled fields instead, so gpuagent can
  skip work for disabled attributes.
  - Win is on the **slow-tier** reads (ECC/RAS, memory usage) — separate amd-smi reads that
    gpuagent could skip when a whole family is disabled; also trims exporter CPU per tick.
    **No fast-tier savings** — the fast set comes from one `amdsmi_get_gpu_metrics()` read
    regardless. (Confirm gpuagent honors the subscribed attr list to skip slow reads.)
  - Cost / why deferred: GPUWatch periods+attributes are **create-time only**, so pruning
    means **delete+recreate the watch object** whenever a field is toggled in
    `config.json`. The current subscribe-all + mask design handles config reload with zero
    watch churn. Implement the recreate-on-change path when the slow-read savings are
    needed. (The lifecycle delete/recreate plumbing from Part C is the foundation.)

- [ ] **Stream `gpu_energy_consumed` in the high-res watch — recommended fast tier.**
  Energy is currently a pull-only/GPUGet field (non-watch), so in high-res mode it rides the
  pull-snapshot cadence, not the fast stream. It is a monotonic accumulator that advances every
  tick — measured **~145–157 W idle** on 8× MI325X (ΔE ≈ 150 J/s) — so 1 s sampling gives real
  power-integration resolution.
  - **Recommended tier: fast**, sourced from the existing bulk `amdsmi_get_gpu_metrics()` read
    (the one call that already feeds the fast set). `amd-smi metric` exposes
    `ENERGY.TOTAL_ENERGY_CONSUMPTION` + `ACCUMULATION_COUNTER` from gpu_metrics, so extracting it
    there is **free** — no added amd-smi call, no fast-tick cost.
  - **Do NOT** use a dedicated `amdsmi_get_energy_count()`: measured **~105 ms/GPU (~840 ms for
    8 GPUs)** — prohibitive for a 1 s tick. If `energy_accumulator` is absent from the watcher's
    `amdsmi_get_gpu_metrics()` result on some ASIC/driver, fall back to **slow tier** rather than
    paying the per-fast-tick dedicated read.
  - Wiring (add-a-high-res-attr checklist): new `AGA_GPU_WATCH_ATTR_ID_ENERGY_CONSUMED`, add to
    `highResWatchMetrics` (Go) + C++ fast extraction from the bulk metrics map, and wire **both**
    id-map directions (aga→proto and proto→aga) or the attr silently drops from the stream path.
    Validate on real HW (SPX 1 s + CPX) that energy appears in the stream and matches the pull value.
