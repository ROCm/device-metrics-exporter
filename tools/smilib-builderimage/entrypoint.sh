#!/usr/bin/env bash
# Build libamd_smi.so from ROCm/rocm-systems (release/therock-7.12) projects/amdsmi.
# Previously used the libamdsmi/ submodule (ROCm/amdsmi@release/rocm-rel-7.2).
#
# Environment variables (set by amdsmi-compile make target):
#   REPO   - git repo URL (default: https://github.com/ROCm/rocm-systems.git)
#   BRANCH - branch to checkout (default: release/therock-7.12)
#   COMMIT - specific commit to pin, or HEAD (default: HEAD)
#   SUBDIR - subdirectory containing CMakeLists.txt (default: projects/amdsmi)

REPO="${REPO:-https://github.com/ROCm/rocm-systems.git}"
BRANCH="${BRANCH:-release/therock-7.12}"
COMMIT="${COMMIT:-HEAD}"
SUBDIR="${SUBDIR:-projects/amdsmi}"

WORKROOT=/usr/src/github.com/ROCm/device-metrics-exporter
CLONEDIR="${WORKROOT}/build/amdsmi-therock/src"
exporteroutdir="${WORKROOT}/libamdsmi/build/exporterout"

echo "=== amdsmi build ==="
echo "  Repo:   ${REPO}"
echo "  Branch: ${BRANCH}"
echo "  Commit: ${COMMIT}"
echo "  Subdir: ${SUBDIR}"

# Sparse-clone only the required subdirectory to avoid downloading the full
# rocm-systems monorepo (~1 GB with full history).
rm -rf "${CLONEDIR}"
mkdir -p "${CLONEDIR}"
git clone --filter=blob:none --sparse --depth=1 --branch "${BRANCH}" \
    "${REPO}" "${CLONEDIR}"
cd "${CLONEDIR}"
git sparse-checkout set "${SUBDIR}"

if [ "${COMMIT}" != "HEAD" ]; then
    echo "Pinning to commit ${COMMIT}"
    if ! git fetch --depth=1 origin "${COMMIT}"; then
        echo "ERROR: Failed to fetch commit ${COMMIT} from origin"
        exit 1
    fi
    if ! git reset --hard "${COMMIT}"; then
        echo "ERROR: Failed to reset to commit ${COMMIT}"
        exit 1
    fi
fi

SRCDIR="${CLONEDIR}/${SUBDIR}"
if [ ! -f "${SRCDIR}/CMakeLists.txt" ]; then
    echo "ERROR: CMakeLists.txt not found at ${SRCDIR}"
    echo "Check that SUBDIR=${SUBDIR} is correct for branch ${BRANCH}"
    exit 1
fi

patchdir="${WORKROOT}/patch/amdsmi"
if [ -d "${patchdir}" ]; then
    echo "Applying patches from ${patchdir}"
    for patch in "${patchdir}"/*.patch; do
        if [ -f "${patch}" ]; then
            echo "Applying patch: ${patch}"
            if git -C "${SRCDIR}" apply --check "${patch}" 2>/dev/null; then
                if ! git -C "${SRCDIR}" apply "${patch}"; then
                    echo "ERROR: Failed to apply patch: ${patch}"
                    exit 1
                fi
            else
                echo "Patch cannot be applied cleanly (may already be applied or conflicts), skipping: ${patch}"
            fi
        fi
    done
else
    echo "No patch directory found at ${patchdir}, skipping patches"
fi

# cmake writes generated files (rust-interface, rocm_smi64Config.h, etc.) into
# the source tree, so copy to a writable build directory.
BUILDSRC="${WORKROOT}/build/amdsmi-therock/build-src"
rm -rf "${BUILDSRC}"
cp -r "${SRCDIR}" "${BUILDSRC}"

BUILDDIR="${BUILDSRC}/build"
mkdir -p "${BUILDDIR}"
cd "${BUILDDIR}"

cmake -DCMAKE_C_COMPILER=gcc \
      -DCMAKE_CXX_COMPILER=g++ \
      -DENABLE_ESMI_LIB=OFF \
      -DCMAKE_BUILD_TYPE=Release \
      ..

make -j "$(nproc)" amd_smi

if [ $? -ne 0 ]; then
    echo "Build error"
    exit 1
fi

# Strip and collect output artifacts
strip --strip-unneeded "${BUILDDIR}"/src/libamd_smi.so.* 2>/dev/null || true
mkdir -p "${exporteroutdir}" || true

# find which os for platform-specific library paths
os=$(grep ^ID= /etc/os-release | cut -d'=' -f2 | tr -d '"')

if [ "${os}" = "ubuntu" ]; then
    echo "Copying UBUNTU library..."
    cp -vr "${BUILDDIR}"/src/libamd_smi.so* "${exporteroutdir}"/
    cp -vr "${BUILDSRC}"/include/amd_smi/amdsmi.h "${exporteroutdir}"/
    cp -vr /usr/lib/x86_64-linux-gnu/libdrm_amdgpu.so* "${exporteroutdir}"/
    cp -vr /usr/lib/x86_64-linux-gnu/libdrm.so* "${exporteroutdir}"/
else
    echo "Copying ${os} library..."
    cp -vr "${BUILDDIR}"/src/libamd_smi.so* "${exporteroutdir}"/
    cp -vr "${BUILDSRC}"/include/amd_smi/amdsmi.h "${exporteroutdir}"/
    cp -vr /usr/lib64/libdrm_amdgpu.so* "${exporteroutdir}"/
    cp -vr /usr/lib64/libdrm.so* "${exporteroutdir}"/
fi

ls -lart "${exporteroutdir}"
echo "Successfully built AMD SMI lib ${os} branch ${BRANCH} commit ${COMMIT}"
exit 0
