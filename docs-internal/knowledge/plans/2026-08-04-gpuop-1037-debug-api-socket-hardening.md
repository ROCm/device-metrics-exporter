# GPUOP-1037: gate debug endpoints + restrict health socket

- **Date:** 2026-08-04
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** #1497, #1503
- **Related issue(s) / JIRA:** GPUOP-1037

## Context

Two exporter surfaces were open by default:

- The pprof/expvar `/debug/*` endpoints on the metrics HTTP port and the
  error-injection (SetError) gRPC API were always registered, exposing process
  internals with no way to turn them off short of a rebuild.
- The health gRPC unix socket was created world-accessible (chmod 0777).

Both were part of the original v1.5.1 hardening work; the socket hunk was
dropped during the squash-merge of #1497 and is re-applied here. This change
brings both onto `main` as a cherry-pick pair.

## Approach

- **#1497** — add a `CommonConfig.Debug.EnableAPI` proto field (default false).
  `startMetricsServer` registers the `/debug/*` routes only when
  `ConfigHandler.GetEnableAPI()` is true; the value is read live, so it tracks
  the existing 3s config auto-reload (the watcher restarts the server on
  change). Replaces the previous build-time/CLI mechanism, making runtime
  config the single control point. `/metrics`, `/amdgpu-metrics`, and the
  inband-RAS route are unaffected.
- **#1503** — change the health socket chmod from 0777 to 0600 (owner/root
  only). All consumers (metricsclient, testrunner, GPU operator test-runner)
  run as root and share the socket dir, so 0600 is sufficient.

### Alternatives considered

- CLI/build-time flag for the debug API — rejected: requires a redeploy to
  toggle; runtime config already auto-reloads and is the natural control point.
- Leaving the socket world-accessible and relying on container isolation —
  rejected: defense-in-depth, all real consumers are root anyway.

## Scope

- **In scope:** the proto field + gate, the socket permission change, matching
  example configs / docs, and unit + e2e coverage for the gate.
- **Out of scope:** any change to metric collection, the gpuagent, or the
  `/metrics` payload.

## Validation

- Unit tests: `TestExporterDebugAPIDisabled` (endpoints unregistered when
  EnableAPI=false, `/metrics` still reachable) plus metrics_svc coverage.
- Integration / e2e: `test/e2e/exporter_test.go` debug-gate cases.
- Manual / hardware (miramar, MI350 8-GPU, both the main-based and the
  v1.5.1-based builds): socket verified `srw------- root root` (0600); `/debug/*`
  returns 404 with EnableAPI=false and 200 with EnableAPI=true, toggled live via
  the 3s reload in both directions; `/metrics` unaffected (209 amd_ series).

## Risks and rollback

- Known risks: an operator relying on the always-on pprof endpoints must now set
  `Debug.EnableAPI: true` in config.json. Non-root tooling that read the 0777
  socket would break, but no such consumer exists (all run as root).
- Rollback: revert #1497 and #1503; both are self-contained (proto + gate, and a
  one-line chmod).
