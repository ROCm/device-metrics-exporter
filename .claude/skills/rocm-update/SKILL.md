---
name: rocm-update
description: This skill should be used when updating the Device Metrics Exporter to a new ROCm/therock version (e.g. RC1 → RC2, nightly bump). Orchestrates the tarball-driven update: bump version references, sync amdsmi assets from the therock tarball, rebuild the docker image (gpuagent builds in-image), and smoke-test.
version: 2.0.0
---

# ROCm Version Update Workflow

Updates the Device Metrics Exporter to a new ROCm/therock release.

**All work happens in the DME repo only.** As of the 2026-06-16 pivot
(`docs-internal/knowledge/plans/2026-06-16-gpuagent-source-build-in-docker.md`),
gpuagent is **no longer a git submodule** — it is cloned and compiled inside
the exporter docker build at `GPUAGENT_COMMIT`, linking against the same amdsmi
that ships at runtime. There is no separate gpu-agent repo edit, no host-side
gpuagent build, and no manual amdsmi injection into a vendor tree.

## What actually changes

The update reduces to three artifacts:

1. **Version references** — `Makefile` + `dev.env` (`ROCM_VERSION`, `ROCM_TARBALL_URL`).
2. **amdsmi assets** — `assets/amd_smi_lib/x86_64/RHEL9/lib/{libamd_smi.so*, amdsmi.h, librocm_sysdeps_*.so*}` + `assets/amd_smi_lib/version.txt`, refreshed from the tarball by `make amdsmi-sync-assets`. UBUNTU22/24 are symlinks to RHEL9 — they update automatically.
3. **docker image** — rebuilt by `make docker`; `build_prep_docker.sh` stages the synced `.so` into `docker/` automatically (no manual `cp`).

Optionally bump `GPUAGENT_COMMIT` if the update should also advance gpuagent.

## Inputs Required

- **New ROCm version string** — must match the tarball's embedded version so it
  extracts to `/opt/rocm-<VERSION>/` (e.g. `7.14.0rc2`).
- **Therock tarball URL** — from the JIRA/BKC. Format:
  `https://rocm.prereleases.amd.com/tarball-multi-arch/therock-dist-linux-multiarch-<VERSION>.tar.gz`
- **Old version string** — for grepping stray references (e.g. `7.14.0rc1`).

## Step 1: Verify the tarball

```bash
# Confirm it exists (HTTP 200) and note the libamd_smi.so version
curl -sI "<TARBALL_URL>" | head -1
curl -s "<TARBALL_URL>" | tar -tz 2>/dev/null | grep -E "libamd_smi\.so\.[0-9]|amd_smi/amdsmi\.h" | head
```

Note the `libamd_smi.so.<X.Y.Z>` version. If it differs from the current one
(check `ls assets/amd_smi_lib/x86_64/RHEL9/lib/libamd_smi.so.*.*.*`), the docker
symlink logic still handles it dynamically — no Dockerfile edit needed — but call
it out in the plan file.

## Step 2: Bump version references

Create the branch off `main`:

```bash
cd ~/go/src/github.com/pensando/device-metrics-exporter
git fetch origin main
git checkout -b feature/rocm-<VERSION>-support origin/main
```

Edit **`Makefile`** and **`dev.env`** — set `ROCM_VERSION` and `ROCM_TARBALL_URL`
to the new values in both. In `dev.env`, also update the provenance comment above
`ROCM_VERSION` (build #, rockrel run, date).

```bash
# Verify no stray old-version references remain in the version-bearing files
grep -rn "<OLD_VERSION>" Makefile dev.env
```

> `Makefile` line ~117 has `AMDSMI_BRANCH ?= therock-7.13` and
> `assets/version.yaml` still references `7.13` — these are **stale metadata**,
> not part of the tarball-driven build path, and are not required for the update.
> Leave them unless the JIRA explicitly asks to reconcile them.

## Step 3: Sync amdsmi assets from the tarball

This is the core step. `amdsmi-sync-assets` extracts `amdsmi.h`,
`libamd_smi.so*`, and the tracked `librocm_sysdeps_*.so*` from the tarball into
`assets/amd_smi_lib/x86_64/RHEL9/lib/` and rewrites `version.txt`. Use
`AMDSMI_TARBALL_FORCE=1` to bypass the staging cache and re-extract.

```bash
make amdsmi-sync-assets AMDSMI_TARBALL_FORCE=1
```

Verify the assets updated:

```bash
git status assets/amd_smi_lib/
cat assets/amd_smi_lib/version.txt          # → therock-<VERSION>
ls -l assets/amd_smi_lib/x86_64/RHEL9/lib/libamd_smi.so.*.*.*
```

Only the 5 already-tracked sysdeps are copied — the sync will **not** introduce
new sysdeps files. If a new runtime sysdep is genuinely needed, add it
explicitly (`git add`) and note it in the plan file.

## Step 4: Rebuild the docker image

Prefer the builder skill: `/builder exporter docker`. It runs the containerized
build with the correct args. Under the hood:

```bash
make docker          # AMDSMI_FROM_TARBALL=0 (default): uses the synced assets/
```

**Default path (`AMDSMI_FROM_TARBALL=0`)** consumes the committed
`assets/amd_smi_lib/` you just synced — no 10 GB re-download. gpuagent is cloned
at `GPUAGENT_COMMIT` and compiled in-image against that same amdsmi. This is the
correct path for a version update whose assets are already synced in Step 3.

Only pass `AMDSMI_FROM_TARBALL=1 ROCM_TARBALL_URL=<URL>` if you want the docker
build itself to re-extract amdsmi from the tarball (redundant right after Step 3).

## Step 5: Smoke test

### Mock (no hardware)

```bash
make -C docker docker-mock TOP_DIR=$(pwd)    # build the mock image (or /builder docker mock)
curl -s localhost:<PORT>/metrics | grep -E "^(amd|gpu)_" | head
# gpu_* prefix for bare docker run; amd_* only with MetricsFieldPrefix in config.json
```

### Real hardware (dev GPU host)

Deploy the image and confirm live metrics:

```bash
docker exec <container> curl -s localhost:<PORT>/metrics | grep gpu_average_package_power
# Expect non-zero power/clock/VRAM values, and no ErrZeroGPUs / undefined-symbol in logs
```

Acceptance (from GPUOP-970): `assets/` reflects the new amdsmi (no old artifacts),
`make docker` succeeds, and basic `amd_gpu_*` / `gpu_*` metrics emit correctly.

## Step 6: Commit + plan file

```bash
git add Makefile dev.env assets/amd_smi_lib/
git commit -m "<JIRA-ID>: sync amdsmi assets for ROCm <VERSION>"
```

Every PR to `main` requires a plan file in
`docs-internal/knowledge/plans/YYYY-MM-DD-*.md` (see
`2026-07-07-rocm-7.14rc1-amdsmi-sync.md` for the template). Then open the PR with
`/pr-create`.

## Key Rules

- **No submodule, no host-side gpuagent build.** gpuagent compiles inside the
  docker build. Do not look for a `gpuagent/` submodule or run
  `gpuagent-asset-copy` / `amdsmi-inject-gpuagent` — those targets were retired.
- **`make amdsmi-sync-assets` is the single amdsmi update mechanism.** Do not
  hand-copy `.so`/header files into `assets/` or `docker/`.
- **Runtime `.so` staging into `docker/` is automatic** via
  `build_prep_docker.sh` — no manual `cp` step.
- **RHEL9 is the source of truth**; UBUNTU22/24 lib dirs are symlinks to it.
- **A `.so` version bump needs no Dockerfile edit** — both the builder and
  runtime stages derive the major/symlink names dynamically from the staged file.
- **DCM changes are separate** — not part of this workflow.

## Reused Skills

- `/builder exporter` — build the DME binary
- `/builder docker` — build the release docker image
- `/builder docker mock` — build the mock image for smoke testing
- `/pr-create` — open/update the PR with the required format

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Tarball URL 404 | Wrong version/URL | Re-check the JIRA/BKC for the exact tarball path |
| `amdsmi.h / libamd_smi.so not found in tarball` | Wrong URL or version | Verify with the Step 1 `tar -tz` listing |
| `assets/` unchanged after sync | Staging cache hit | Re-run with `AMDSMI_TARBALL_FORCE=1` |
| `undefined symbol` from libamdsmi at runtime | Header/binary skew | Ensure Step 3 synced both `.so` and `amdsmi.h`; rebuild docker (gpuagent relinks in-image) |
| `librocm_sysdeps_*.so: not found` | Sysdeps missing from assets | Confirm the tarball carries them; re-run sync with `AMDSMI_TARBALL_FORCE=1` |
| `GLIBC_2.38` symbols in `.so` | Ubuntu-built `.so` in tarball | Use the multiarch/RHEL9-compatible tarball |
