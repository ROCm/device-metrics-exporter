# Plan: Multi-Version nicctl Bundling for DME AINIC Docker Image

**Date**: 2026-08-14
**Author**: Yuva Shankar
**Status**: Implemented
**Reference**: [pensando/k8s-network-device-plugin#58](https://github.com/pensando/k8s-network-device-plugin/pull/58)

---

## Context

PR [pensando/k8s-network-device-plugin#58](https://github.com/pensando/k8s-network-device-plugin/pull/58) implemented multi-version nicctl bundling in the NIC device-plugin image. This plan adapts the same pattern to the device-metrics-exporter AINIC Docker image so both images support automatic firmware detection and version selection at container startup.

**Problem**: A single nicctl version is baked into the AINIC image. Clusters with mixed NIC firmware versions need separate images per firmware version.

**Solution**: Bundle 1-5 nicctl versions per image. The bootstrap (latest) is stored uncompressed (~73 MB); older versions are xz-compressed (~8.4 MB each). At startup, a wrapper script detects NIC firmware and selects the correct binary.

**Image Size Impact**:
- 1 version: ~73 MB nicctl overhead
- 2 versions (default): ~81 MB (+8.4 MB)
- 5 versions (max): ~107 MB (+34 MB)

---

## Default Flow

**Root Makefile is the single source of truth** for version defaults, matching the repo's existing pattern for `ROCM_VERSION`, `BASE_IMAGE`, etc.

| Location | AINIC_VERSIONS | BOOTSTRAP_VERSION |
|---|---|---|
| Root `Makefile` (defaults) | `1.117.5-a-77,1.117.5-a-147` | `1.117.5-a-147` |
| `docker/Makefile` | pass-through via `$(AINIC_VERSIONS)` | pass-through via `$(BOOTSTRAP_VERSION)` |
| `Dockerfile` | no default (requires `--build-arg`) | `""` (empty triggers "use last in list" fallback) |

---

## Files Changed

### 1. `docker/nicctl-setup.sh` — NEW FILE

Entrypoint wrapper adapted from k8s-ndp `images/nicctl-setup.sh`. Logic:

1. If `/usr/sbin/nicctl-bootstrap` doesn't exist → single-version build, skip to original entrypoint
2. Read bootstrap version from `/opt/bootstrap-version.txt`
3. Detect firmware via `nicctl-bootstrap show firmware`, parse `Firmware-[AB]` field
4. Match against bundled versions: bootstrap → symlink, compressed → decompress, unknown → fallback to bootstrap with WARNING
5. Verify `nicctl --version`, then `exec /home/amd/tools/entrypoint.sh "$@"`

Key difference from k8s-ndp: chains to `/home/amd/tools/entrypoint.sh` (not `/entrypoint.sh`). Uses AMD copyright header matching `docker/entrypoint.sh`.

### 2. `docker/Dockerfile.ainic.exporter-release` — REWRITE

**ARGs** (top, no hardcoded defaults):
```dockerfile
ARG BASE_IMAGE=registry.access.redhat.com/ubi9/ubi-minimal:9.8
ARG AINIC_VERSIONS
ARG BOOTSTRAP_VERSION=""
```

**Builder stage** → renamed to `nicctlbuilder`, consolidated RUN:
- Parses `AINIC_VERSIONS` as CSV, validates (non-empty, max 5, bootstrap in list)
- Installs dtc, binutils, xz as build deps
- Single version: install + strip → `/export/bin/nicctl`
- Multi-version: bootstrap stripped → `/export/bin/nicctl-bootstrap`, others → `/export/nicctl-versions/nicctl-{ver}.xz`
- Records bootstrap version in `/export/bootstrap-version.txt`
- Copies libpci* and nsenter (DME-specific) to export dirs

**Final stage** changes:
- `ARG AINIC_VERSIONS` (replaces `ARG AINIC_VERSION`)
- `COPY --from=nicctlbuilder /export/bin/nicctl* /usr/sbin/`
- `COPY --from=nicctlbuilder /export/nicctl-versions /opt/nicctl-versions`
- `COPY --from=nicctlbuilder /export/bootstrap-version.txt /opt/`
- Keeps existing libpci, nsenter COPYs
- Adds `ADD ./nicctl-setup.sh /home/amd/tools/nicctl-setup.sh` + `chmod +x`
- Adds `xz` to microdnf install line
- Label: `ainic_bundled_versions="${AINIC_VERSIONS}"` (replaces `ainic_version`)
- Entrypoint: `["/home/amd/tools/nicctl-setup.sh"]`

### 3. Root `Makefile` — lines 158-159, 184-185

```makefile
AINIC_VERSIONS ?= 1.117.5-a-77,1.117.5-a-147
BOOTSTRAP_VERSION ?= 1.117.5-a-147
...
export AINIC_VERSIONS
export BOOTSTRAP_VERSION
```

### 4. `docker/Makefile` — docker-ainic target

```makefile
--build-arg AINIC_VERSIONS=$(AINIC_VERSIONS) \
$(if $(BOOTSTRAP_VERSION),--build-arg BOOTSTRAP_VERSION=$(BOOTSTRAP_VERSION)) \
```

### 5. No changes needed

- `docker/build_prep_docker.sh` — `nicctl-setup.sh` lives in `docker/` (tracked file, already in build context)
- `docker/entrypoint.sh` — unchanged, receives args via `exec` chain
- `pkg/amdnic/nicagent/constants.go` — `NICCtlBinary = "nicctl"` still valid (binary always at `/usr/sbin/nicctl` after setup)
- `docker/build_post_docker.sh` — no staged artifacts to clean up

---

## Backward Compatibility

| Scenario | Behavior |
|----------|----------|
| `make docker-ainic` (default) | Multi-version: `1.117.5-a-77` (compressed) + `1.117.5-a-147` (bootstrap), runtime detection |
| `make docker-ainic AINIC_VERSIONS="1.117.5-a-147"` | Single version, no `nicctl-bootstrap`, setup script skips straight to entrypoint |
| `make docker-ainic AINIC_VERSIONS="a-56,a-77,a-147" BOOTSTRAP_VERSION="a-147"` | 3-version build |
| Go code (`nicctl_client.go`) | Unchanged — `exec.LookPath("nicctl")` finds `/usr/sbin/nicctl` in both modes |

---

## Verification

1. **Default build**: `make docker-ainic` → image builds with 2 versions (`1.117.5-a-77`, `1.117.5-a-147`)
2. **Single-version override**: `make docker-ainic AINIC_VERSIONS="1.117.5-a-147"` → single version, no bootstrap binary
3. **Startup (no NIC)**: `docker run --rm <image> nicctl --version` → uses bootstrap, logs WARNING about no NICs
4. **Image size**: default ~88 MB, single ~80 MB (verify with `docker images`)
5. **nicctl-setup.sh short-circuit**: single-version image should NOT have `/usr/sbin/nicctl-bootstrap`
