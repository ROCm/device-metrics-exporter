# Parse Flat and Nested RVS Results

- **Date:** 2026-10-15
- **Author:** yan.sun3@amd.com
- **Related PR(s):** [#1683](https://github.com/pensando/device-metrics-exporter/pull/1683)
- **Related issue(s) / JIRA:** N/A

## Context

The test runner currently decodes each RVS action as a flat list of objects
containing `gpu_index` and `pass`. Recent RVS output includes nested per-subtest
records, such as Babel's `results[]`, where `pass` is below the action object.
That causes a completed test whose subtests passed to be reported as having no
results. RVS suite schemas also differ in where they expose GPU identity.

## Approach

- Keep each suite as raw JSON and recursively walk records instead of binding
  every suite to one fixed result struct.
- Inherit GPU identity into nested records and use nested `subtest` or legacy
  `Function` names to retain each result independently.
- Normalize fallback `gpu_id` values through the runner's KFD-ID-to-index map;
  return a parse error if an ID cannot be resolved instead of storing a result
  under a different identifier namespace.
- Treat an empty `pass` string as an absent result while continuing to visit
  nested records.
- Continue accepting flat result records and handle topology suite endpoint IDs.
- Preserve a failure when multiple records map to the same GPU/action.

### Alternatives considered

- Add a Babel-only special case — rejected because RVS result schemas vary by
  suite and future nested layouts would repeat the same parser failure.
- Bind all known suite schemas as structs — rejected because RVS schemas are
  independently versioned and this would make each schema change require code.

## Scope

- **In scope:** Parse flat and nested RVS result records for the test runner's
  configured RVS recipes.
- **Out of scope:** Change RVS output, test execution, or the runner's public
  result API.

## Validation

- Unit tests cover legacy flat records, nested Babel subtests, empty pass
  placeholders, normalized GPU IDs, GPU ID inheritance, unresolved IDs, and
  GPU topology endpoints.
- `go test ./pkg/testrunner` and `make amdtestrunner` pass in both branches.
- `go test ./...` was attempted; unrelated tests need writable
  `/var/run/exporter` and a local kubeconfig, and `go vet` also reports
  non-constant format strings in `tools/metricutil` under the installed Go
  1.25 toolchain.
- Live validation on the MI210 at `10.30.60.190` passed for nested `babel`,
  flat `iet_single`, and `gpup_single`. The runner logged KFD ID `35824` mapped
  to canonical GPU index `0`; all three test Pods exited successfully.

## Risks and rollback

- **Known risks:** RVS suites may add new GPU identity or test-label fields;
  unrecognized records without a GPU identity cannot be associated with a GPU.
- **Rollback plan:** Revert the parser change and restore the prior flat-only
  decoder.
