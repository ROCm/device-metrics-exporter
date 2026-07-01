# Cherry-pick: Slurm doc fixes — container tag, AMD GPU fallback, method-aware commands

- **Date:** 2026-06-30
- **Author:** spraveenio
- **Related PR(s):** #1424 (pensando/device-metrics-exporter), ROCm/device-metrics-exporter#535
- **Related issue(s) / JIRA:** None

## Context

ROCm/device-metrics-exporter#535 landed three doc/script fixes in the ROCm public repo that are equally applicable to the internal repo:

1. The `docker run` example in `docs/integrations/slurm-integration.md` carried a stale `v|version|` placeholder tag and a generic `--name exporter`. Users following the guide were pulling an indeterminate image version and getting a container named differently from the rest of the documentation.
2. On AMD hardware, `CUDA_VISIBLE_DEVICES` is frequently empty in Slurm prolog/epilog hooks. Without a fallback, GPU metrics are published without `job_id`/`job_user` labels, breaking per-job attribution.
3. Troubleshooting log commands (`systemctl`, `journalctl`) were not annotated to indicate they only apply to the host-package installation; container users would hit "unit not found" errors following the same steps.

## Approach

- Cherry-pick commit `8d40d724` from `rocmupstream/main` onto `main`.
- Resolve the single conflict in `slurm-integration.md` by accepting the upstream version (concrete tag `v1.5.0`, full container name `device-metrics-exporter`).
- No logic changes beyond the shell fallback; purely docs + prolog/epilog shell scripts.

### Alternatives considered

- **Backport via patch file** — unnecessary overhead for a single clean commit; cherry-pick is simpler.
- **Templatize version tag** — deferred as follow-up; out of scope for this cherry-pick.

## Scope

- **In scope:** `docs/integrations/slurm-integration.md`, `debian/usr/local/etc/metrics/slurm/slurm-prolog.sh`, `debian/usr/local/etc/metrics/slurm/slurm-epilog.sh`
- **Out of scope:** RPM packaging equivalents, runtime config, any other Slurm integration logic

## Validation

- Unit tests: N/A — documentation and shell-variable fallback only.
- Integration / e2e tests: N/A.
- Manual: diff reviewed against ROCm#535; conflict resolution verified correct by inspecting both sides.

## Risks and rollback

- Known risks: `SLURM_JOB_GPUS` fallback is additive — only activates when `CUDA_VISIBLE_DEVICES` is unset/empty, so existing behaviour on non-AMD or CUDA-visible environments is unchanged.
- Rollback plan: `git revert` the cherry-pick commit; single-commit PR makes this trivial.
