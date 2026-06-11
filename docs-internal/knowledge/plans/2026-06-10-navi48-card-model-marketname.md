# Navi48 card_model market_name fallback + drop amdgpu.ids plumbing

- **Date:** 2026-06-10
- **Author:** Bhanu Kiran Atturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-832 / GPUOP-843 (amdgpu.ids / card_model marketing name)

## Context

amd-smi **26.4.0** returns an empty `board_info.model_number` (N/A) for
consumer Navi48/RDNA4 parts, where 26.2.1 used to populate it. As a result
`card_model` came back empty on those GPUs. `asic_info.market_name` *is*
populated on 26.4.0 (e.g. "AMD Radeon AI Pro R9700S").

An earlier mitigation shipped an `amdgpu.ids` file into the image/packages and
resolved the marketing name in the exporter by looking up the PCI
device/revision. That approach added build-time fetch steps and a dpkg file
conflict (GPUOP-832), and is now redundant: gpuagent resolves the marketing
name natively.

This change is split out so the card_model fix can merge independently of the
still-undecided gfx_activity cold-read debounce work.

## Approach

- Rebuild the `gpuagent_static` asset so `card_model` falls back to
  `asic_info.market_name` when `board_info.model_number` is empty.
- Remove the now-unneeded `amdgpu.ids` plumbing across all package/image paths:
  - docker: drop the `ADD amdgpu.ids` layer and the `build_prep_docker.sh`
    fetch step.
  - rpm: drop the fetch + install + `%files` + `DEST_DRM` entries from both
    `amdgpu-exporter.spec` and `amdgpu-exporter-sriov.spec`.
  - `Makefile.package`: drop the rpm `data/` fetch and debian `libdrm` cleanup.
  - delete `tools/fetch-amdgpu-ids.sh` and `assets/drm/amdgpu.ids.extra`.

### Alternatives considered

- Keep exporter-side `amdgpu.ids` lookup — rejected: extra build fetch,
  dpkg conflict, and obsolete now that gpuagent returns market_name.
- Bundle with the cold-read debounce commit — rejected: that design is still
  undecided and would block this ready, independent fix.

## Scope

- **In scope:** card_model market_name fallback (asset rebuild) and full
  removal of amdgpu.ids build/package plumbing.
- **Out of scope:** gfx_activity cold-read suppression (tracked separately).

## Validation

- Unit tests: `make unit-test` in the build container.
- Build: `make all` (binary) + image build via the builder skill.
- Manual / hardware steps: localhost build + run; on Navi48 hosts (e.g.
  10.7.121.210, 8x Navi48) `card_model` reports "AMD Radeon AI Pro R9700S"
  for all GPUs (previously validated 8/8 on 7.13/26.4.0).

## Risks and rollback

- Known risks: any consumer that relied on `/usr/share/libdrm/amdgpu.ids`
  shipped by these packages will no longer get it from us (the distro
  `libdrm`/`libdrm-amdgpu` package still provides its own copy).
- Rollback plan: revert the single commit; the previous asset and amdgpu.ids
  plumbing are restored together.
