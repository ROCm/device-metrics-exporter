# GPUOP-1056 Plan

## Problem
`GPU_HBM_TEMPERATURE` is documented as "deprecated from 6.14.14 driver", but it is
still exported on MI2xx (MI210) and absent on MI300X/MI350X/MI350P on the same
driver family. Availability is GPU-model-specific, not a driver-version cutoff.

## Root cause
DME has no driver-version gate. HBM temp is value-driven
(`gpuagent_gpu_metrics.go:2236`): the exporter mirrors amd-smi and suppresses the
metric when the library returns no value. amd-smi exposes HBM temp on MI2xx but
not on MI300/MI350.

## Evidence
Same container image (`exporter-0.0.1-373`) on both hosts:
- MI210 (leto): `gpu_hbm_temperature` = 4 stacks (51-53°C)
- MI300A (ctr-rack33-mi300a-27): metric absent

## Fix (docs only)
- `docs/configuration/metricslist.md`: remove HBM temp from the "Not Supported on
  Any Platform" list; retag `[Deprecated]` -> `[MI2xx]`; note MI300/MI350 have no
  amd-smi HBM temperature.

## Out of scope
No DME code change (exporter behaves correctly). CI already handled in
gpu-operator PR #1651.
