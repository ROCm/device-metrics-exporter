# SR-IOV gpuagent asset refresh — gpu-agent@b19cd57

- **Date:** 2026-06-04
- **Author:** praveen
- **Related PR(s):** #1376
- **Related issue(s) / JIRA:** —
- **Predecessor:** #1372 (asset at gpu-agent@075ca40)

## Context

`assets/gpuagent_sriov_static.bin.gz` embeds the GIM-build (`gpuagent_gim`)
from the `gpu-agent` repo. The asset on `main` after #1372 was at `075ca40`;
this refresh moves it to `b19cd57`, which is the next commit on the
`feature/gimamdsmi-smi-session` branch and contains the same `smi_session`
RAII change plus minor Makefile / `.gitignore` updates (no gpuagent source
diff).

The key behavioral change present in this binary (carried over from `075ca40`):

- **`smi_session` RAII helper:** GIM amdsmi backend switches from a
  persistent init-at-startup model to an on-demand per-request session that
  calls `amdsmi_init` / `amdsmi_shut_down` around each gRPC handler, releasing
  `/dev/gim-smi0` between scrapes. Activated by `AGA_SMI_LAZY_INIT=1`
  (already set in `Dockerfile.sriov.exporter-release`).

## Scope

- **In scope:** `assets/gpuagent_sriov_static.bin.gz` (binary-only).
- **Also included:** `.claude/hooks/make-in-container.sh` + `settings.json`
  wiring (Claude Code tooling, no runtime impact).
- **Out of scope:** Source code, other asset variants
  (`gpuagent_static.bin.gz`, `gpuagent_mock.bin.gz`), Dockerfiles, configs.

## Repack procedure

```bash
GIM_BIN=~/go/src/github.com/ROCm/gpu-agent/sw/nic/build/x86_64/sim/bin/gpuagent_gim
cp "$GIM_BIN" assets/gpuagent
strip assets/gpuagent
( cd assets && tar czf gpuagent_sriov_static.bin.gz gpuagent )
rm assets/gpuagent
```

## Validation

HW stress on RHEL9 SR-IOV/GIM host (1 GPU, hypervisor passthrough):

**Startup checks:**
- Container: `running`, restarts=0
- `AGA_SMI_LAZY_INIT=1` confirmed via `docker exec … env`
- `/var/run/gpuagent.sock` present; `gpu-agent.log` shows IPC messages
- 298 `gpu_*`/`pcie_*` metric lines per scrape, `deployment_mode="hypervisor"`
  and `gpu_partition_id` labels present

**Load test (concurrent metrics + topology):**

| Test | Result |
|---|---|
| `gpuctl show device topology` × 15 (every 2 s, concurrent with scrapes) | 15/15 ok |
| `/metrics` sequential (30 req, 1 req/s) | 30/30 ok, 0 fail |
| `/metrics` parallel (4 concurrent × 10) | 40/40 ok, 0 fail |
| Container state at end | `running`, restarts=0 |

## Risks and rollback

- **Known risks:** Binary-only asset change; bug surface is the gpu-agent
  commit baked into the binary. Mitigated by the HW stress run above.
- **Rollback:** Revert `assets/gpuagent_sriov_static.bin.gz` to the previous
  binary (restores #1372 state).
