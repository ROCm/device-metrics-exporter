#!/bin/bash
# install-rocm-tarball.sh — download and install ROCm from a TheRock nightly tarball.
#
# Usage: install-rocm-tarball.sh <ROCM_VERSION> <ROCM_TARBALL_URL> [<LIBDRM_SYMLINK_DIR>] [--profile exporter|testrunner]
#
#   ROCM_VERSION       — version string, e.g. "7.14.0rc0"
#   ROCM_TARBALL_URL   — full URL to the .tar.gz tarball
#   LIBDRM_SYMLINK_DIR — directory to create libdrm_amdgpu.so symlinks in
#                        (default: /opt/rocm/lib for exporter, /usr/lib64 for testrunner)
#   --profile          — lib prune profile: exporter (default) or testrunner
#                        exporter:    minimal set for the metrics exporter runtime
#                        testrunner:  extended set for RVS + AGFHC tools
#
# After install:
#   /opt/rocm-<version>/  — extracted and pruned tarball
#   /etc/alternatives/rocm -> /opt/rocm-<version>
#   /opt/rocm             -> /etc/alternatives/rocm
#   <LIBDRM_SYMLINK_DIR>/libdrm_amdgpu.so{,.1} -> rocm_sysdeps/lib/libdrm_amdgpu.so
#   <LIBDRM_SYMLINK_DIR>/libdrm.so{,.2}        -> rocm_sysdeps/lib/libdrm.so  (if present)

set -euo pipefail

ROCM_VERSION="${1:?ROCM_VERSION required}"
ROCM_TARBALL_URL="${2:?ROCM_TARBALL_URL required}"

# Parse optional positional and named args
PROFILE="exporter"
LIBDRM_SYMLINK_DIR=""

shift 2
while [[ $# -gt 0 ]]; do
    case "$1" in
        --profile)
            PROFILE="${2:?--profile requires a value}"
            shift 2
            ;;
        --*)
            echo "Unknown option: $1" >&2; exit 1
            ;;
        *)
            # Positional: libdrm symlink dir
            LIBDRM_SYMLINK_DIR="$1"
            shift
            ;;
    esac
done

# Set default LIBDRM_SYMLINK_DIR based on profile if not provided
if [ -z "${LIBDRM_SYMLINK_DIR}" ]; then
    case "${PROFILE}" in
        testrunner) LIBDRM_SYMLINK_DIR="/usr/lib64" ;;
        *)          LIBDRM_SYMLINK_DIR="/opt/rocm/lib" ;;
    esac
fi

echo "=== ROCm install: TheRock tarball (${ROCM_VERSION}) ==="
echo "    URL: ${ROCM_TARBALL_URL}"
echo "    profile: ${PROFILE}"
echo "    libdrm symlink dir: ${LIBDRM_SYMLINK_DIR}"

mkdir -p "/opt/rocm-${ROCM_VERSION}"
wget -qO- "${ROCM_TARBALL_URL}" | tar -xzf - -C "/opt/rocm-${ROCM_VERSION}"

ROCM_DIR="/opt/rocm-${ROCM_VERSION}"

echo "=== Pruning unreferenced libs from ${ROCM_DIR} (profile=${PROFILE}) ==="
KEEP_DIR=$(mktemp -d)
mkdir -p "${KEEP_DIR}/lib" "${KEEP_DIR}/bin" "${KEEP_DIR}/libexec" "${KEEP_DIR}/share"

# ── Lib prune list — common to both profiles ──────────────────────────────────
COMMON_PATTERNS=(
    "libamdhip64.*"
    "librocm_kpack.*"
    "librocprofiler-sdk.*" "librocprofiler-sdk-roctx.*"
    "librocprofiler-register.*"
    "libamd_comgr.*" "libamd_comgr_loader.*"
    "libhsa-runtime64.*" "libhsa-amd-aqlprofile64.*"
    "librocrand.*"
)

# ── Additional libs for testrunner profile (RVS + AGFHC runtime deps) ─────────
TESTRUNNER_PATTERNS=(
    "libamd_smi.*"
    "librocblas.*"
    "librocm-core.*"
    "libhipblaslt.*"
    "libhiprand.*"
    "libroctx64.*"
    "librocroller.*"
    "libhiprtc.*"
    "librocsolver.*"
)

PATTERNS=("${COMMON_PATTERNS[@]}")
if [ "${PROFILE}" = "testrunner" ]; then
    PATTERNS+=("${TESTRUNNER_PATTERNS[@]}")
fi

for pat in "${PATTERNS[@]}"; do
    for f in "${ROCM_DIR}/lib"/${pat}; do
        if [ -e "$f" ] || [ -L "$f" ]; then
            cp -a "$f" "${KEEP_DIR}/lib/" 2>/dev/null || true
        fi
    done
done

# rocm_sysdeps: libdrm_amdgpu, libdrm, netlink libs (both profiles)
[ -d "${ROCM_DIR}/lib/rocm_sysdeps" ] && \
    cp -a "${ROCM_DIR}/lib/rocm_sysdeps" "${KEEP_DIR}/lib/rocm_sysdeps" || true

# llvm/lib: libclang-cpp and libLLVM required by libamd_comgr
# testrunner also needs libomp.so for RVS modules
if [ -d "${ROCM_DIR}/lib/llvm/lib" ]; then
    mkdir -p "${KEEP_DIR}/lib/llvm/lib"
    LLVM_PATS=("libclang-cpp.so*" "libLLVM.so*" "libLLVM-*.so*")
    if [ "${PROFILE}" = "testrunner" ]; then
        LLVM_PATS=("libomp.so*" "${LLVM_PATS[@]}")
    fi
    for pat in "${LLVM_PATS[@]}"; do
        for f in "${ROCM_DIR}/lib/llvm/lib"/${pat}; do
            [ -e "$f" ] || [ -L "$f" ] && cp -a "$f" "${KEEP_DIR}/lib/llvm/lib/" 2>/dev/null || true
        done
    done
fi

# rocblas kernel library data (Tensile .dat/.co — testrunner only, RVS gst module)
if [ "${PROFILE}" = "testrunner" ] && [ -d "${ROCM_DIR}/lib/rocblas" ]; then
    cp -a "${ROCM_DIR}/lib/rocblas" "${KEEP_DIR}/lib/rocblas"
fi

# Binaries and share
[ -f "${ROCM_DIR}/bin/amd-smi" ]   && cp -a "${ROCM_DIR}/bin/amd-smi"   "${KEEP_DIR}/bin/" || true
[ -f "${ROCM_DIR}/bin/rocprofv3" ] && cp -a "${ROCM_DIR}/bin/rocprofv3" "${KEEP_DIR}/bin/" || true
[ -d "${ROCM_DIR}/libexec/amdsmi_cli" ] && \
    cp -a "${ROCM_DIR}/libexec/amdsmi_cli" "${KEEP_DIR}/libexec/" || true
[ -d "${ROCM_DIR}/share/amd_smi" ] && cp -a "${ROCM_DIR}/share/amd_smi" "${KEEP_DIR}/share/" || true
[ -d "${ROCM_DIR}/share/rocprofiler-sdk" ] && \
    cp -a "${ROCM_DIR}/share/rocprofiler-sdk" "${KEEP_DIR}/share/" || true

# Safety check before rm -rf
if [ -z "${ROCM_DIR}" ]; then
    echo "Refusing to delete empty ROCM_DIR" >&2; exit 1
fi
case "${ROCM_DIR}" in
    /opt/rocm-*) ;;
    *) echo "Refusing to delete unexpected ROCM_DIR: ${ROCM_DIR}" >&2; exit 1 ;;
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
