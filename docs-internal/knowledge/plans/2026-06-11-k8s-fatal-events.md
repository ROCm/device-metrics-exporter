# K8s warning events on fatal exit paths

- **Date:** 2026-06-11
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** DME server immediately exits with "too many open files"

## Context

DME can exit immediately on startup with "too many open files". Root cause is
host-wide inotify exhaustion (other pods consume `fs.inotify.max_user_instances`);
`inotify_init1` then returns EMFILE and the exporter's `logger.Log.Fatal` killed
the process with no actionable signal. Operators need a clear message so they
know where to look. The agreed direction is to raise K8s events on fatal exit,
while acknowledging those events may also fail under the same exhaustion.

## Approach

- Replace bare `logger.Log.Fatal*` on fatal paths with the package-level
  `events.Fatal(reason, msg)`, which emits a K8s Warning event
  synchronously, runs the registered `cleanup` for orderly resource release,
  then exits. It is idempotent (`sync.Once`) and safe from any goroutine.
- The cleanup (`exporter.Close()`) and `exitFn` are registered once at
  `events.Init(client, cleanup, exitFn)`, so gpuagent monitor-loop exits — which
  previously bypassed the `StartMain` deferred `Close()` via `os.Exit` — now run
  the same orderly cleanup. No `fatalExit` field/handler is threaded through the
  `Exporter` or `GPUAgentClient`.
- Actionable event reasons (`AgentUnreachable`, `ZeroGPUsDetected`,
  `HealthValidationFailed`, `RocpctlFatalExit`, `ConfigWatcherFailed`,
  `SlurmWatcherFailed`, etc.) carry messages naming the likely fix (e.g. raise
  `fs.inotify.max_user_instances`).
- The sync emit delivers the event before exit, bounded by a 5s timeout so a
  failed/slow emit never blocks termination.

## Event service

`pkg/events` owns all emission so callers never guard with `IsKubernetes()`/nil
checks:

- A process-wide singleton `EventService` (buffered queue, single dispatcher
  goroutine) is created by `events.Init(client, cleanup, exitFn)`;
  `events.Stop()` drains it. Package-level `EmitWarning` / `EmitWarningSync` /
  `Fatal` front the singleton; the `cleanup`/`exitFn`/`fatalOnce` fatal-exit
  state lives on the `EventService` itself. The event reason type
  (`events.EventReason`) lives here.
- `events.Init(nil, ...)` yields a log-only service: every emit is logged, no K8s
  event is created. This is the path for bare-metal/Debian and for the disable
  toggle, so non-k8s setups degrade gracefully with no code-path branching at
  the call sites.
- The k8s client exposes a single synchronous `EmitWarningEventDirect(ctx,
  reason string, msg string) error`. RBAC is handled gracefully: the first
  `Forbidden` error trips a one-way `eventsForbidden` latch (logged once) and
  the node/pod watcher reconnect loop stops retrying on a `Forbidden`
  list/watch error. The reason is passed as a plain `string` to avoid an
  import cycle (`pkg/events` imports `pkg/client`).
- The k8s client remains injected for its non-event uses (node labelling, pod
  info, watchers).

## Configurability

- No dedicated event flag. Emission reuses the existing `-enable-k8s` path:
  `NewExporter` only builds the k8s client under `IsKubernetes() &&
  !disableK8sApi`, otherwise `events.Init` receives a nil client and emission is
  log-only. So off-cluster, Debian, or `-enable-k8s=false` all degrade
  gracefully with no separate toggle.

## Scope

- **In scope:** K8s warning events + orderly cleanup on fatal exit paths;
  actionable inotify message; bounded best-effort sync emit; graceful no-op
  off-cluster (reuses `-enable-k8s`).
- **Out of scope:** Working around a bad host (no poll fallback); the
  `pkg/client/k8s.go` informer-recreation theory (verified not the trigger —
  `startWatchers` blocks on `stopCh` and does not loop).

## Validation

- Unit tests: `pkg/events`, `pkg/client`, `pkg/exporter`, and
  `pkg/amdgpu/gpuagent` pass in the RHEL9 build container; `go vet` clean.
- Live K8s (localhost k3s): gpuagent-bypass pod emits the `HealthValidationFailed`
  Warning event on fatal exit; `-enable-k8s=false` suppresses it (log-only, no
  event).
- Live non-k8s (bare host, no `KUBERNETES_SERVICE_HOST`): `events.Init(nil)`
  log-only; fatal path logs + runs `Close()` + exits 1 with no panic.

## Risks and rollback

- Known risks: event emit best-effort and may fail under the same exhaustion
  (accepted); behavior unchanged when not running under K8s.
- Rollback plan: revert the branch commits; prior `logger.Log.Fatal` behavior
  returns.
