# Cherry-pick GPU_CPER_MAX_AGE CPER staleness filter (PR #507)

- **Date:** 2026-06-10
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** cherry-pick of ROCm/device-metrics-exporter#507
- **Related issue(s) / JIRA:** open-source sync (public -> internal)

## Context

ROCm public PR #507 adds a `GPU_CPER_MAX_AGE` GPU health-threshold field.
When set, only CPER (Common Platform Error Record) entries newer than the
configured age window can mark a GPU unhealthy, so stale fatal records left
over from a previous boot or a long-past event no longer keep a healthy GPU
flagged as unhealthy.

This change is wanted in the internal `pensando` tree, so it is cherry-picked
from the public repo. The PR was 16 commits upstream; we bring it in as a
single squashed commit to keep the internal history clean.

## Approach

- Squashed cherry-pick of PR #507 (`<merge-base>..pr-507`) onto `pensando/main`.
  Zero merge conflicts; 13 files staged.
- Preserve original authorship: commit author set to the upstream PR author
  (Aryan <paryan@digitalocean.com>) with a `Co-authored-by:` trailer; committer
  is the local engineer.
- Config plumbing: `GPU_CPER_MAX_AGE` added to `GPUHealthThresholds` in
  `exporterconfig.proto` (field 20), `make gen` regenerates the `.pb.go`.
  `config_handler.GetGPUCperMaxAge()` parses the duration string and the
  gpuagent CPER health path honors the age window.

### Integration fixup (CI vet failure)

The upstream test `pkg/exporter/config/config_handler_test.go` was written
against an older `NewConfigHandler(path string, port int)` signature. The
internal tree's signature is
`NewConfigHandler(path string, agentConfig GPUAgentConfig)`, so the cherry-pick
broke `make vet`:

```
vet: pkg/exporter/config/config_handler_test.go:32:45: cannot use
globals.GPUAgentPort (untyped int constant 50061) as GPUAgentConfig value
```

Fix: pass `GPUAgentConfig{GrpcPort: globals.GPUAgentPort}`, matching the
existing convention used across `exporter_test.go` and `init_test.go`.

## Scope

- **In scope:** the PR #507 feature (proto field, generated code, config
  handler, gpuagent CPER age filtering, tests) and the `NewConfigHandler`
  signature fixup.
- **Out of scope:** gpuagent/libgimsmi submodule changes; any new feature work
  beyond what PR #507 contained.

## Validation

- `go vet ./pkg/exporter/config/...` passes in the build container (was the
  CI failure).
- `make all` builds the exporter binary cleanly on this branch.
- Live metrics verified flowing from 4x MI300A (`/metrics` HTTP 200).
- Note: `TestCPERFatalSeveritySetsGPUUnhealthy` fails locally on both this
  branch and the unmodified upstream `pr-507` branch — a pre-existing
  timezone/`cperTimestampLayout` environment issue, not a cherry-pick
  regression.

## Risks and rollback

- **Risk:** low — additive health-config field; default (unset) preserves
  prior behavior.
- **Rollback:** revert the single squashed cherry-pick commit.
