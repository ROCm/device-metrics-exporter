# testrunner: Radeon model detection + case-insensitive GPU model folder lookup

- **Date:** 2026-05-29
- **Author:** yansun1996
- **Related PR(s):** (branch `tr_radeon_devices`)
- **Related issue(s) / JIRA:** —

## Context

The testrunner resolves an RVS test recipe folder by GPU model name (e.g.
`MI300A`, `Nv21`). Two gaps exist today:

1. **Radeon Pro and consumer Radeon GPUs are not mapped.** `GPUDeviceIDToModelName`
   in `pkg/exporter/globals/testrunner.go` only covers MI-series (Instinct)
   device IDs, so on Radeon hardware the testrunner cannot resolve a recipe
   folder and falls back to model-agnostic recipes.
2. **Recipe folder lookup is case-sensitive.** Recipe folders shipped from
   different sources use inconsistent casing (`MI350X`, `mi350x`, `Nv21`).
   On case-sensitive filesystems a single-case `os.Stat` check misses
   otherwise-valid folders, causing a spurious "no recipe" fallback.

## Approach

- Extend `GPUDeviceIDToModelName` with Radeon Pro (W6800–W7900D, AI PRO 9600D
  family) and consumer Radeon (RX 6800–RX 9070) device IDs, mapping each to
  its gfx/nv codename (`nv21`, `nv31`, `nv32`, `R9600D`, `RX9060`, `RX9070`).
- Add a `findGPUModelFolder(baseDir, gpuModel)` helper in
  `pkg/testrunner/testrunner.go`:
  - Fast path: `os.Stat` on the exact path (avoids directory listing in the
    common case).
  - Fallback: `os.ReadDir(baseDir)` + `strings.EqualFold` to match
    case-insensitively, returning the actual on-disk folder name.
  - Distinguishes "base dir missing" from other I/O errors in logs.
- Replace the three existing `os.Stat`-based folder probes in `validateCfg`
  (MI350X alias path, MI355X alias path, final lookup) with the new helper,
  and propagate the actual matched folder name to `testCfgGPUModelName` /
  `gpuModelSubDir` so downstream `testCfgPath` is built against the real path.

### Alternatives considered

- **Lowercase-only folder convention** — would require renaming all existing
  recipe folders shipped by partners, breaking external assumptions. Rejected.
- **Per-call uppercase + lowercase Stat attempts** — three Stat calls per
  lookup, still fragile against mixed case (`Nv21`). The directory-listing
  approach is one ReadDir on miss and exactly matches any casing.

## Scope

- **In scope:** Device-ID table additions for Radeon Pro / Radeon SKUs;
  case-insensitive recipe folder resolution; unit test coverage for the new
  helper.
- **Out of scope:** Recipe content for Radeon models; changes to RVS
  invocation; metric schema or exporter runtime changes.

## Validation

- Unit tests: `TestFindGPUModelFolder` in
  `pkg/testrunner/testrunner_test.go` covers exact match, three casing
  permutations, not-found, missing base dir, and "path is a file" cases.
- Manual / hardware steps: run testrunner on a Radeon-equipped host and
  confirm log line `using test recipe from <folder> folder` with the actual
  on-disk casing.

## Risks and rollback

- **Risk:** `findGPUModelFolder` now reads the base directory on every miss.
  Cost is bounded by the small number of recipe folders and only paid on
  cache miss; the exact-match fast path covers the steady state.
- **Risk:** A new device ID mapped to the wrong codename would silently
  route to an inappropriate recipe folder. Mitigation: codenames are taken
  from the published pci-ids / MxGPU marketing-name sources cited in the
  existing comment block.
- **Rollback:** Revert commit `26a2cac46`. No schema, no on-disk state, no
  config migration.
