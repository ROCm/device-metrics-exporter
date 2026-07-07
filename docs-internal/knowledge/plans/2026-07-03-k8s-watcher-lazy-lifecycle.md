# Plan: Lazy/Reconfigurable Node & Pod Watchers

## Problem

`K8sClient` (`pkg/client/k8s.go`) always creates and runs **both** the node
informer and the pod informer as soon as `Watch()` is called
(`startWatchers()`, lines 336-424), which happens unconditionally at startup
from `exporter.go:472-481` whenever `-enable-k8s` + `-enable-k8s-scl` are set.

In reality:
- **Node watcher** is only useful when `HealthService.Enable` is true — its
  only real consumer path is the health-label update flow
  (`sendNodeLabelUpdate` → `UpdateHealthLabel`, which does a direct
  `clientset.Get/Update`, not the cache — but conceptually node watching is a
  health-service concern). The informer itself currently has no functional
  consumer (`GetNode()`/`GetNodeLabel()` have zero callers) besides logging.
- **Pod watcher** is only useful when `ExtraPodLabels` is non-empty for at
  least one of GPU/NIC/IFOE config (`podInfoEnabled` flag, consumed via
  `FetchPodInfoForNode()` → `k8sApiClient.GetAllPods()` → `ListPods()` →
  `podInformer`).

Both `HealthService.Enable` and `ExtraPodLabels` are **runtime-reloadable**
config (read every reload cycle via `ConfigHandler`/`MetricsHandler` getters),
but watcher creation today is a one-shot boot-time decision. There is no
existing config-change notification mechanism (config reload is currently
pull-style getters + a full HTTP-server restart on file change — see
`foreverWatcher()` in `exporter.go`). The overall `-enable-k8s` flag remains
the global on/off switch and is unaffected by this change.

## Goal

When `enableK8s` is true, dynamically start/stop the node watcher based on
`HealthService.Enable`, and the pod watcher based on whether any
`ExtraPodLabels` config is set — independently of each other — and react to
runtime config changes without a process restart.

## Design

### 1. Split node/pod watcher lifecycle in `K8sClient`

Replace the single `Watch()` → `startWatchers()` (which always builds both
informers in one function) with independently controlled watchers:

```go
func (k *K8sClient) SetNodeWatcherEnabled(enabled bool)
func (k *K8sClient) SetPodWatcherEnabled(enabled bool)
```

Internally:
- Each watcher gets its own `stopCh`, `started bool`, and its own
  `runWithReconnect` goroutine (reuse the existing reconnect/backoff and
  RBAC-forbidden detection logic, just parameterized per-resource instead of
  hardcoded to run both).
- `SetNodeWatcherEnabled(true)` starts the node informer goroutine if not
  already running; `SetNodeWatcherEnabled(false)` closes its stop channel and
  clears `k.nodeInformer`. Same for pod.
- Calls must be idempotent and safe to call repeatedly with the same value
  (no-op if already in that state) — this matters because the config-reload
  path will call these on every reload tick regardless of whether the value
  actually changed.
- Guard with the existing `sync.Mutex` on `K8sClient`.
- `Stop()` (full shutdown) disables both.

This keeps `K8sClient`'s public API additive — existing `Watch()`/`Stop()`
callers (if any remain) can be removed since `exporter.go` is the only
caller.

### 2. Config plumbing: add getters for the two gating conditions

- `HealthService.Enable` already has `ConfigHandler.GetHealthServiceState()`
  — reuse as-is.
- `ExtraPodLabels` needs a single combined check across GPU/NIC/IFOE configs
  since the pod watcher is shared. Add to `MetricsHandler`
  (`pkg/exporter/metricsutil/metrics.go`), since that's where
  `GetGPUMetricsConfig()`/`GetNICMetricsConfig()`/`GetIFOEMetricsConfig()`
  already live:

```go
func (mh *MetricsHandler) AnyExtraPodLabelsConfigured() bool {
    if len(mh.GetGPUMetricsConfig().GetExtraPodLabels()) > 0 { return true }
    if len(mh.GetNICMetricsConfig().GetExtraPodLabels()) > 0 { return true }
    if len(mh.GetIFOEMetricsConfig().GetExtraPodLabels()) > 0 { return true }
    return false
}
```

(Guard nil configs the same way existing getters do.)

### 3. Reconciliation point: hook into the existing reload path

`foreverWatcher()`'s debounced fsnotify handler already does
`stopServer()` / `startServer()` on every config change, and `startServer()`
calls `mh.InitConfig(e.ctx)` which calls `runConf.RefreshConfig()`. This is
the natural place to reconcile watcher state — no new Subscribe/observer
mechanism needed, since this callback already fires exactly when config
changes land.

Add one call after config refresh, in `startServer()` right after
`mh.InitConfig(e.ctx)`:

```go
e.reconcileK8sWatchers()
```

```go
func (e *Exporter) reconcileK8sWatchers() {
    if e.k8sApiClient == nil {
        return
    }
    e.k8sApiClient.SetNodeWatcherEnabled(runConf.GetHealthServiceState())
    e.k8sApiClient.SetPodWatcherEnabled(mh.AnyExtraPodLabelsConfigured())
}
```

Call this once at initial startup too (replacing the current unconditional
`e.startWatchers()` call at line 480) so first boot honors the same rule
instead of always starting both.

This satisfies "handle config changes in runtime gracefully" using the
mechanism the codebase already has (debounced file-watch → reload → re-derive
derived state), rather than inventing a new push-notification config system —
consistent with "surgical changes."

### 4. `enableK8s` flag interaction

No change needed to `WithK8sApiClient`/`disableK8sApi` — that continues to
gate whether `K8sClient` exists at all. `reconcileK8sWatchers()` naturally
no-ops when `e.k8sApiClient == nil` (i.e., `-enable-k8s=false`).

## Files touched

| File | Change |
|---|---|
| `pkg/client/k8s.go` | Split `Watch()`/`startWatchers()` into independent `SetNodeWatcherEnabled`/`SetPodWatcherEnabled` with per-watcher stop channel, started flag, reconnect loop |
| `pkg/client/k8s_watch_test.go` | Update/extend existing RBAC-forbidden test for the new per-watcher entry points; add tests for independent start/stop and idempotency |
| `pkg/exporter/metricsutil/metrics.go` | Add `AnyExtraPodLabelsConfigured()` |
| `pkg/exporter/exporter.go` | Replace `startWatchers()` unconditional call with `reconcileK8sWatchers()`; invoke it from `startServer()` after `mh.InitConfig()` and once at initial `StartMain` k8s-scl setup block |

## Test plan

- Unit: `K8sClient` — start node only, start pod only, start both, toggle
  off/on repeatedly, verify idempotency (no duplicate goroutines/informers),
  verify `Stop()` tears down both.
- Unit: `MetricsHandler.AnyExtraPodLabelsConfigured()` — nil configs, empty
  maps, one non-empty map among the three.
- Integration/manual: on a k8s test host, toggle `HealthService.Enable` and
  `ExtraPodLabels` in `config.json`, wait for the 3s debounce, confirm via
  logs that only the relevant watcher starts/stops (`kubectl logs` grep for
  the existing "k8s watchers started/stopped" log lines, extended per-watcher).

## Follow-up: debuggability (info logs + k8s events)

Added after initial implementation, same PR/branch:

- `SetNodeWatcherEnabled`/`SetPodWatcherEnabled` now return whether a
  transition actually happened, and log both no-op (debug level) and real
  transitions (info level, "was X, now Y").
- `reconcileK8sWatchers()` logs the desired state of both watchers plus the
  config knob that drove it on every reconcile pass.
- New `K8sWatcherStateChanged` Normal k8s Event is emitted (via
  `K8sClient.EmitInfoEventDirect` / `events.EmitInfo`) whenever a watcher's
  running state actually flips, so `kubectl get events` / `kubectl describe
  pod` shows watcher lifecycle without needing to grep pod logs.

## Validation — unit tests (build container)

```
go build ./...                                                  # PASS
go vet ./...                                                     # PASS
go test ./pkg/client/... ./pkg/events/... ./pkg/exporter/...     # PASS (ok, all packages)
```

Covers: independent start/stop and idempotency of both watchers
(`TestWatcherEnabled_IndependentAndIdempotent`), RBAC-forbidden handling per
watcher (`TestStartNodeWatcher_DisablesOnForbidden`,
`TestStartPodWatcher_DisablesOnForbidden`), and the new
`EmitInfoEventDirect` Normal-event path (`TestEmitInfoEventDirect_EmitsNormal`).

## Validation — hardware (lab test host, gpu-operator-managed k8s)

Deployed a binary built from this branch (overlaid onto the cluster's
existing `claude-1.5.0-v2` base image) via a `DeviceConfig` patch
(`metricsExporter.image` + `metricsExporter.config.name` pointing at a test
ConfigMap), following the `MetricsExporterSpec.Config` mount documented in
the [gpu-operator full DeviceConfig reference](https://instinct.docs.amd.com/projects/gpu-operator/en/latest/fulldeviceconfig.html#full-deviceconfig).

Sequence (each step is a `kubectl apply` on the test ConfigMap, followed by
waiting out kubelet's ConfigMap-to-volume sync + the exporter's 3s debounce):

1. **Startup**, `HealthService.Enable=true` + `ExtraPodLabels` set:
   ```
   reconciling k8s watchers: node watcher wanted=true (HealthService.Enable), pod watcher wanted=true (ExtraPodLabels configured)
   k8s node watcher starting (was running=false, now running=true)
   k8s pod watcher starting (was running=false, now running=true)
   event "K8sWatcherStateChanged": k8s node watcher started (HealthService.Enable=true)
   emitted K8s Normal event "K8sWatcherStateChanged" on pod kube-amd-gpu/gpu-operator-metrics-exporter-lqnrr: k8s node watcher started (HealthService.Enable=true)
   event "K8sWatcherStateChanged": k8s pod watcher started (ExtraPodLabels configured=true)
   emitted K8s Normal event "K8sWatcherStateChanged" on pod kube-amd-gpu/gpu-operator-metrics-exporter-lqnrr: k8s pod watcher started (ExtraPodLabels configured=true)
   ```
   `kubectl get events` confirms both `K8sWatcherStateChanged` Normal events.

2. **ConfigMap update**, `HealthService.Enable=false` (labels kept):
   ```
   loading new config on /etc/metrics/config.json
   reconciling k8s watchers: node watcher wanted=false (HealthService.Enable), pod watcher wanted=true (ExtraPodLabels configured)
   k8s node watcher stopped (was running=true, now running=false)
   event "K8sWatcherStateChanged": k8s node watcher stopped (HealthService.Enable=false)
   emitted K8s Normal event "K8sWatcherStateChanged" ...: k8s node watcher stopped (HealthService.Enable=false)
   ```
   Pod watcher untouched (no pod-watcher log/event this cycle). Confirmed in
   `kubectl get events`.

3. **ConfigMap update**, `HealthService.Enable=true` + `ExtraPodLabels` removed:
   ```
   reconciling k8s watchers: node watcher wanted=true (HealthService.Enable), pod watcher wanted=false (ExtraPodLabels configured)
   k8s node watcher starting (was running=false, now running=true)
   k8s pod watcher stopped (was running=true, now running=false)
   event "K8sWatcherStateChanged": k8s node watcher started (HealthService.Enable=true)
   emitted K8s Normal event "K8sWatcherStateChanged" ...: k8s node watcher started (HealthService.Enable=true)
   event "K8sWatcherStateChanged": k8s pod watcher stopped (ExtraPodLabels configured=false)
   emitted K8s Normal event "K8sWatcherStateChanged" ...: k8s pod watcher stopped (ExtraPodLabels configured=false)
   ```

Final `kubectl get events -n kube-amd-gpu --field-selector involvedObject.name=<pod>,reason=K8sWatcherStateChanged --sort-by=.lastTimestamp` showed all
5 transitions across the sequence, matching the reconcile logs exactly:

```
LAST SEEN   TYPE     REASON                   OBJECT                                    MESSAGE
4m16s       Normal   K8sWatcherStateChanged   pod/gpu-operator-metrics-exporter-lqnrr   k8s pod watcher started (ExtraPodLabels configured=true)
4m16s       Normal   K8sWatcherStateChanged   pod/gpu-operator-metrics-exporter-lqnrr   k8s node watcher started (HealthService.Enable=true)
2m57s       Normal   K8sWatcherStateChanged   pod/gpu-operator-metrics-exporter-lqnrr   k8s node watcher stopped (HealthService.Enable=false)
26s         Normal   K8sWatcherStateChanged   pod/gpu-operator-metrics-exporter-lqnrr   k8s pod watcher stopped (ExtraPodLabels configured=false)
26s         Normal   K8sWatcherStateChanged   pod/gpu-operator-metrics-exporter-lqnrr   k8s node watcher started (HealthService.Enable=true)
```

`/metrics` continued serving correctly throughout. After validation, the
`DeviceConfig` was reverted to the original `claude-1.5.0-v2` image with no
test ConfigMap, and the test ConfigMap was deleted — cluster restored to its
pre-test state.

## Follow-up: drop transition tracking, extend to profiler state, dedupe watcher code

Same PR/branch, after the initial hardware validation above:

- `reconcileK8sWatchers()` and `GPUAgentGPUClient.initProfilerMetrics()` no
  longer track prev-state and only emit on a flip. They now emit their
  respective `K8sWatcherStateChanged` / new `ProfilerStateChanged` k8s Event
  unconditionally on every pass (every ~3s config-reload tick), reporting the
  current desired state. Simpler, at the cost of noisier `kubectl get
  events` — acceptable since Events naturally dedupe/aggregate identical
  reason+object+message in the k8s events API (`count` increments instead of
  new entries).
- Removed the now-unnecessary `profileMetricsStateKnown` bool from
  `GPUAgentGPUClient` and the `SetNodeWatcherEnabled`/`SetPodWatcherEnabled`
  "did it change" bookkeeping is no longer read by `reconcileK8sWatchers()`
  (the functions still return a bool for their own idempotency logging, but
  callers don't gate on it anymore).
- New `events.ProfilerStateChanged` reason (`pkg/events/reasons.go`) —
  emitted from `initProfilerMetrics` in
  `pkg/amdgpu/gpuagent/gpuagent_gpu_metrics.go` with message `"profiler
  metrics <enabled|disabled> on <hostname> (ProfilerMetrics config)"`.
- Deduplicated `runNodeWatcherWithReconnect`/`runPodWatcherWithReconnect`
  into a single `runWatcherWithReconnect(name, stopCh, start)` (`pkg/client/k8s.go`),
  and the informer-run/cache-sync/select boilerplate shared between
  `startNodeWatcher`/`startPodWatcher` into `runInformerAndWait(informer,
  stopCh)`. `startNodeWatcher`/`startPodWatcher` remain separate functions
  (still called directly by `k8s_watch_test.go`) but now only build their
  resource-specific informer + event handlers before delegating.

### Validation — unit tests (host toolchain, quick check)

```
go build ./pkg/client/... ./pkg/exporter/... ./pkg/amdgpu/gpuagent/...   # PASS
go test ./pkg/client/...                                                 # ok
```

### Validation — hardware (lab test host, gpu-operator-managed k8s)

Built `bin/amd-metrics-exporter` via `make docker-compile` (CI=1, non-TTY)
in the project's build container, then `make docker` to produce a
`${DOCKER_REGISTRY}/device-metrics-exporter:dev-k8swatch` dev image, pushed
to the lab registry.

Created a test ConfigMap (`exporter-watcher-test-cfgmap`) with:
```json
{
  "CommonConfig": { "HealthService": { "Enable": true } },
  "GPUConfig": {
    "ExtraPodLabels": { "WORKLOAD_ID": "amd-workload-id" },
    "ProfilerMetrics": { "all": true }
  }
}
```
and patched the `DeviceConfig`'s `metricsExporter.image` +
`metricsExporter.config.name` to point at the test image/ConfigMap.

New pod (`gpu-operator-metrics-exporter-hbzbx`) came up and, on the very
first reconcile pass, emitted all three target events:

```
reconciling k8s watchers: node watcher wanted=true (HealthService.Enable), pod watcher wanted=true (ExtraPodLabels configured)
k8s node watcher starting (was running=false, now running=true)
k8s pod watcher starting (was running=false, now running=true)
event "K8sWatcherStateChanged": k8s node watcher started (HealthService.Enable=true)
emitted K8s Normal event "K8sWatcherStateChanged" on pod kube-amd-gpu/gpu-operator-metrics-exporter-hbzbx: k8s node watcher started (HealthService.Enable=true)
event "K8sWatcherStateChanged": k8s pod watcher started (ExtraPodLabels configured=true)
emitted K8s Normal event "K8sWatcherStateChanged" on pod kube-amd-gpu/gpu-operator-metrics-exporter-hbzbx: k8s pod watcher started (ExtraPodLabels configured=true)
profiler metric state set for leto -> true
event "ProfilerStateChanged": profiler metrics enabled on leto (ProfilerMetrics config)
emitted K8s Normal event "ProfilerStateChanged" on pod kube-amd-gpu/gpu-operator-metrics-exporter-hbzbx: profiler metrics enabled on leto (ProfilerMetrics config)
```

`kubectl get events -n kube-amd-gpu --field-selector involvedObject.name=gpu-operator-metrics-exporter-hbzbx --sort-by=.lastTimestamp` confirmed all
three event reasons landed as Normal k8s Events, and repeated on each
subsequent ~3s reconcile tick (expected now that transition-tracking was
removed — the k8s events API deduped identical reason+object+message into
repeat `LAST SEEN` bumps rather than spamming new entries):

```
LAST SEEN   TYPE      REASON                   OBJECT                                    MESSAGE
7s          Normal    K8sWatcherStateChanged   pod/gpu-operator-metrics-exporter-hbzbx   k8s node watcher started (HealthService.Enable=true)
7s          Normal    ProfilerStateChanged     pod/gpu-operator-metrics-exporter-hbzbx   profiler metrics enabled on leto (ProfilerMetrics config)
7s          Normal    K8sWatcherStateChanged   pod/gpu-operator-metrics-exporter-hbzbx   k8s pod watcher started (ExtraPodLabels configured=true)
6s          Warning   ProfilerDisabled         pod/gpu-operator-metrics-exporter-hbzbx   GPU profiler metrics (gpu_prof_*) disabled: rocpctl process core dumped/aborted. Restart the pod to re-enable.
```

Note: the `ProfilerDisabled` warning is pre-existing, unrelated behavior —
this sim/container GPU (`leto`) has no `/dev/kfd` for PC sampling, so
`rocpctl` aborts and the existing 3-failure auto-disable kicks in
immediately after `ProfilerStateChanged` fires. This confirms
`ProfilerStateChanged` correctly reports the *config-driven* intended state
(`ProfilerMetrics.all=true` → enabled) independently of the
runtime-capability auto-disable path, which is the intended distinction.

`/metrics` continued serving correctly (`workload_id` label populated per
`ExtraPodLabels` config, confirming the pod watcher's label data reached the
GPU metrics path). After validation, the `DeviceConfig` was reverted to the
original `claude-1.5.0-v2` image with empty `config.name`, and the test
ConfigMap was deleted — cluster restored to its pre-test state.

## Follow-up: PR review fixes (Copilot review on PR #1436)

Same branch, addressed before merge. All four review comments were valid:

1. **RBAC-forbidden left watchers permanently "running".**
   `runWatcherWithReconnect` returned on `errWatchForbidden` without
   resetting `nodeWatcherRunning`/`podWatcherRunning`, so a later
   `Set*WatcherEnabled(true)` — e.g. after the ServiceAccount is granted
   `list/watch` — was a silent no-op forever (`enabled == k.*WatcherRunning`
   short-circuits). Fixed by passing an `onForbidden` callback into
   `runWatcherWithReconnect` that clears the running flag/stop-channel/
   informer under the lock, guarded by an identity check on the stop channel
   so a stale goroutine can't clobber a state a newer `Set*WatcherEnabled`
   call already established. Added `TestWatcherEnabled_RecoversAfterForbidden`.
2. **Data race on `nodeInformer`/`podInformer`.** These fields are now
   mutated at runtime (previously set once at boot), but `GetNode()`/
   `ListPods()` read them without holding `k.Lock()`. Fixed by snapshotting
   the informer pointer under the lock before use in both functions.
3. **Misleading doc comments on `EmitWarning`/`EmitInfo`** ("logs only
   before Init/after Stop") — `emitToK8s` actually logs on every call.
   Corrected the comments to describe the real behavior.
4. **Stale test comment** claiming `reconcileK8sWatchers` gates event
   emission on the `Set*WatcherEnabled` return value — it emits
   unconditionally per the earlier "no transition tracking" change.
   Corrected the comment.

### Validation — unit tests (build container, race detector)

```
go test -race ./pkg/client/... ./pkg/events/... ./pkg/exporter ./pkg/amdgpu/gpuagent/...   # PASS
```

(A pre-existing, unrelated race in `pkg/exporter/logger` — untouched by this
PR — was excluded; confirmed via `git log` that the failing test file predates
this branch.)

### Branch history

The branch was squashed to a single commit before merge (was two commits:
the initial feature + a follow-up fixing the four review comments above).
`git log --oneline main..HEAD` shows one commit ahead of `main`.

### Validation — hardware (lab test host, gpu-operator-managed k8s), post-squash

Re-ran the full hardware validation from the squashed commit to confirm no
regressions from the review fixes: rebuilt `bin/amd-metrics-exporter` and the
dev docker image, redeployed via the same `DeviceConfig`/test-ConfigMap
pattern as before (`HealthService.Enable=true`, `ExtraPodLabels` set,
`ProfilerMetrics.all=true`), and confirmed:
- `K8sWatcherStateChanged` (node + pod) and `ProfilerStateChanged` Normal
  k8s Events fire on startup and each reconcile tick, matching the earlier
  run.
- `/metrics` served correctly with `workload_id` populated.
- No data-race warnings surfaced in pod logs (exporter is built without
  `-race` for production images, so this is a smoke check, not a substitute
  for the `go test -race` run above).

`DeviceConfig` was reverted to the original image with empty `config.name`,
and the test ConfigMap was deleted — cluster restored to its pre-test state.
