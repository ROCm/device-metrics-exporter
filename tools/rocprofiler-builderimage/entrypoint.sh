#!/usr/bin/env bash
set -euo pipefail

dir=/usr/src/github.com/ROCm/device-metrics-exporter/rocprofilerclient
outdir="$dir/build/"

cd "$dir"

# Prefer hipcc; fall back to amdclang++ if needed
HIPCC_BIN="${CMAKE_HIP_COMPILER:-}"
if [[ -z "${HIPCC_BIN}" ]]; then
    if [[ -x "/opt/rocm/llvm/bin/amdclang++" ]]; then
        HIPCC_BIN="/opt/rocm/llvm/bin/amdclang++"
    else
        echo "ERROR: Could not find hipcc or amdclang++ in expected locations." >&2
        exit 1
    fi
fi

# Set a HIP architecture list if one is not provided. This avoids CMake failing
# to auto-detect arch in containers without GPU access.
# Override via CMAKE_HIP_ARCHITECTURES or AMDGPU_TARGETS env vars.
HIP_ARCHS="${CMAKE_HIP_ARCHITECTURES:-}"
if [[ -z "${HIP_ARCHS}" ]]; then
    HIP_ARCHS="${AMDGPU_TARGETS:-}"
fi
if [[ -z "${HIP_ARCHS}" ]]; then
    # Sensible default; adjust as needed or override via env
    HIP_ARCHS="gfx942,gfx950"
fi

echo "Using HIP compiler: ${HIPCC_BIN}"
echo "Target HIP architectures: ${HIP_ARCHS}"

rm -rf build || true
cmake -B build ./ \
    -DCMAKE_PREFIX_PATH=/opt/rocm \
    -DCMAKE_HIP_COMPILER="${HIPCC_BIN}" \
    -DCMAKE_HIP_ARCHITECTURES="${HIP_ARCHS}"

cmake --build build --target all

# come back to root directory
cd "$dir"

ls -lart "$outdir"

echo "Successfully Built rocprofiler library"
exit 0
