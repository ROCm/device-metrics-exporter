#!/bin/bash
# install-rocm-tarball.sh — download and install ROCm from an S3 therock nightly tarball.
#
# Usage: install-rocm-tarball.sh <ROCM_VERSION> <ROCM_TARBALL_URL> [<LIBDRM_SYMLINK_DIR>]
#
#   ROCM_VERSION       — version string, e.g. "7.12"
#   ROCM_TARBALL_URL   — full URL to the .tar.gz tarball
#   LIBDRM_SYMLINK_DIR — directory to create libdrm_amdgpu.so.1 symlink in
#                        (default: /opt/rocm/lib)
#
# Integrity: TheRock nightly tarballs are served over HTTPS from S3 and do not
# have companion checksum files. The tarball URL encodes the build date (e.g.
# 7.12.0a20260225), providing implicit version pinning.
#
# After install:
#   /opt/rocm-<version>/  — extracted tarball
#   /etc/alternatives/rocm -> /opt/rocm-<version>
#   /opt/rocm             -> /etc/alternatives/rocm
#   <LIBDRM_SYMLINK_DIR>/libdrm_amdgpu.so{,.1} -> rocm_sysdeps/lib/libdrm_amdgpu.so
#   <LIBDRM_SYMLINK_DIR>/libdrm.so{,.2}        -> rocm_sysdeps/lib/libdrm.so  (if present)

set -euo pipefail

ROCM_VERSION="${1:?ROCM_VERSION required}"
ROCM_TARBALL_URL="${2:?ROCM_TARBALL_URL required}"
LIBDRM_SYMLINK_DIR="${3:-/opt/rocm/lib}"

echo "=== ROCm install: S3 tarball (${ROCM_VERSION}) ==="
echo "    URL: ${ROCM_TARBALL_URL}"
echo "    libdrm symlink dir: ${LIBDRM_SYMLINK_DIR}"

mkdir -p "/opt/rocm-${ROCM_VERSION}"
wget -qO- "${ROCM_TARBALL_URL}" | tar -xzf - -C "/opt/rocm-${ROCM_VERSION}"

ROCM_DIR="/opt/rocm-${ROCM_VERSION}"

echo "=== Pruning unreferenced libs from ${ROCM_DIR} ==="
KEEP_DIR=$(mktemp -d)
mkdir -p "${KEEP_DIR}/lib" "${KEEP_DIR}/bin" "${KEEP_DIR}/libexec" "${KEEP_DIR}/share"
for pat in \
    "libamdhip64.*" "librocm_kpack.*" "librocprofiler-sdk.*" "librocprofiler-sdk-roctx.*" \
    "librocprofiler-register.*" "libamd_comgr.*" "libamd_comgr_loader.*" \
    "libhsa-runtime64.*" "libhsa-amd-aqlprofile64.*" "librocrand.*"; do
    for f in "${ROCM_DIR}/lib"/${pat}; do
        if [ -e "$f" ] || [ -L "$f" ]; then
            cp -a "$f" "${KEEP_DIR}/lib/" 2>/dev/null || true
        fi
    done
done
[ -d "${ROCM_DIR}/lib/rocm_sysdeps" ] && \
    cp -a "${ROCM_DIR}/lib/rocm_sysdeps" "${KEEP_DIR}/lib/rocm_sysdeps" || true
# libclang-cpp and libLLVM are required by libamd_comgr; copy .so only (llvm/lib/ has 2GB+ of .a files)
if [ -d "${ROCM_DIR}/lib/llvm/lib" ]; then
    mkdir -p "${KEEP_DIR}/lib/llvm/lib"
    for pat in "libclang-cpp.so*" "libLLVM.so*" "libLLVM-*.so*"; do
        for f in "${ROCM_DIR}/lib/llvm/lib"/${pat}; do
            [ -e "$f" ] || [ -L "$f" ] && cp -a "$f" "${KEEP_DIR}/lib/llvm/lib/" 2>/dev/null || true
        done
    done
fi
[ -f "${ROCM_DIR}/bin/amd-smi" ] && cp -a "${ROCM_DIR}/bin/amd-smi" "${KEEP_DIR}/bin/" || true
[ -f "${ROCM_DIR}/bin/rocprofv3" ] && cp -a "${ROCM_DIR}/bin/rocprofv3" "${KEEP_DIR}/bin/" || true
[ -d "${ROCM_DIR}/libexec/amdsmi_cli" ] && \
    cp -a "${ROCM_DIR}/libexec/amdsmi_cli" "${KEEP_DIR}/libexec/" || true
[ -d "${ROCM_DIR}/share/amd_smi" ] && cp -a "${ROCM_DIR}/share/amd_smi" "${KEEP_DIR}/share/" || true
[ -d "${ROCM_DIR}/share/rocprofiler-sdk" ] && \
    cp -a "${ROCM_DIR}/share/rocprofiler-sdk" "${KEEP_DIR}/share/" || true
# Wipe entire ROCM_DIR and rebuild minimal structure
if [ -z "${ROCM_DIR}" ]; then
    echo "Refusing to delete empty ROCM_DIR" >&2
    exit 1
fi
case "${ROCM_DIR}" in
    /opt/rocm-*) ;;
    *)
        echo "Refusing to delete unexpected ROCM_DIR: ${ROCM_DIR}" >&2
        exit 1
        ;;
esac
rm -rf "${ROCM_DIR}"
mkdir -p "${ROCM_DIR}"
cp -a "${KEEP_DIR}/." "${ROCM_DIR}/"
rm -rf "${KEEP_DIR}"

echo "=== Pruning done ==="

mkdir -p /etc/alternatives
ln -sf "/opt/rocm-${ROCM_VERSION}" /etc/alternatives/rocm
ln -sf /etc/alternatives/rocm /opt/rocm

SYSDEPS="/opt/rocm-${ROCM_VERSION}/lib/rocm_sysdeps/lib"
mkdir -p "${LIBDRM_SYMLINK_DIR}"

if [ -f "${SYSDEPS}/libdrm_amdgpu.so" ]; then
    ln -sf "${SYSDEPS}/libdrm_amdgpu.so" "${LIBDRM_SYMLINK_DIR}/libdrm_amdgpu.so.1"
    ln -sf "${SYSDEPS}/libdrm_amdgpu.so" "${LIBDRM_SYMLINK_DIR}/libdrm_amdgpu.so"
fi

if [ -f "${SYSDEPS}/libdrm.so" ]; then
    ln -sf "${SYSDEPS}/libdrm.so" "${LIBDRM_SYMLINK_DIR}/libdrm.so.2"
    ln -sf "${SYSDEPS}/libdrm.so" "${LIBDRM_SYMLINK_DIR}/libdrm.so"
fi

echo "=== ROCm ${ROCM_VERSION} installed at /opt/rocm-${ROCM_VERSION} ==="
