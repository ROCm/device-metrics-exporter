# GPUOP-913: double-SIGTERM nil-deref in gRPC graceful shutdown

- **Date:** 2026-06-29
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-913

## Context

DME crashes with a nil-pointer dereference when a second termination
signal arrives during/after the gRPC graceful shutdown that a first
signal already started. The panic backtrace points at a nil
`grpc.Server` receiver:

```
panic: runtime error: invalid memory address or nil pointer dereference
google.golang.org/grpc.(*Server).stop(0x0, 0x1)
google.golang.org/grpc.(*Server).GracefulStop(...)
.../pkg/exporter/svc.(*SvcHandler).Run(...)  svc_handler.go:178
```

Root cause (`pkg/exporter/svc/svc_handler.go`): `Stop()` called
`s.grpc.GracefulStop()` directly, and `Run()`'s `select` also called
`s.grpc.GracefulStop()` directly on signal/error. When shutdown ran twice
the second path dereferenced an already-stopped / nil `s.grpc`.

Real-world trigger: `cmd/exporter/main.go` installs its own SIGINT/SIGTERM
handler goroutine that calls `exporterHandler.Close()` (→ `Stop()`), while
`Run()` is concurrently armed for the same signals — two shutdown paths
race on the single `s.grpc`.

## Approach

Serialize and idempotent-ize shutdown of the shared `s.grpc` handle.

- Add `grpcMu sync.Mutex` guarding `s.grpc`.
- `Stop()` captures the handle and nils `s.grpc` under the mutex, then
  calls `GracefulStop()` only if the captured handle was non-nil — any
  later shutdown is a safe no-op.
- `Run()`'s `select` calls the guarded `s.Stop()` (instead of raw
  `s.grpc.GracefulStop()`) on both the error and the signal branch,
  keeping the subsequent `s.serverWg.Wait()`.

A re-armable mutex (not `sync.Once`) is correct: `Run()` recreates the
server when `s.grpc == nil`, so a one-shot guard would wrongly block a
later legitimate lifecycle stop.

### Alternatives considered

- `sync.Once` around shutdown — rejected; the handler is re-armable
  (`Run()` recreates `s.grpc`), so a one-shot guard breaks a later
  legitimate stop.
- Drop `main.go`'s signal handler and rely solely on `Run()`'s select —
  rejected; larger behavioral change to the shutdown ownership model when
  a small, local guard fully closes the race.

## Scope

- **In scope:** `pkg/exporter/svc/svc_handler.go` — `grpcMu`, guarded
  `Stop()`, `Run()` select using `s.Stop()`.
- **Out of scope:** GPUOP-907 / PR #1408 (C++ gpuagent clock-freq OOB
  SIGSEGV) — a distinct crash; signal-handler ownership refactor in
  `main.go`.

## Validation

- Unit tests: verified locally with a self-exec subprocess repro
  (`TestGPUOP913DoubleSigtermPanic`, kept out of the committed diff) —
  child drives `Run()` → `Stop()` → second SIGTERM; parent asserts a
  clean exit with no nil-deref panic string.
  - WITHOUT fix (main's `svc_handler.go`): child panics with
    `invalid memory address or nil pointer dereference` at
    `grpc.(*Server).stop(0x0, 0x1)` / `svc_handler.go:178` — repro FAILS.
  - WITH fix: `--- PASS` (~2.4s), double-SIGTERM shutdown completes
    cleanly. Run in `golang:1.25-bookworm`, `-mod=vendor`, `CGO_ENABLED=0`
    (`-p 1 GOMAXPROCS=4` to stay under the container thread limit).
- Integration / e2e: n/a.
- Manual / hardware: n/a (signal-race logic, host-independent).

## Risks and rollback

- Known risks: minimal; the change adds a mutex-guarded capture-and-nil
  around an existing call. No API or behavioral change on the
  single-signal happy path.
- Rollback plan: revert this commit; the change is self-contained to
  `svc_handler.go`.
