#!/bin/bash
# install-rvs-tarball.sh — download and install RVS from a pre-built tarball.
#
# Usage: install-rvs-tarball.sh <RVS_TARBALL_URL>
#
#   RVS_TARBALL_URL — full URL to the RVS .tar.gz
#                     e.g. https://repo.amd.com/rocm/rvs/tarball/amdrocm7-rvs-1.4.24-454-Linux.tar.gz
#
# Tarball layout (top-level, no wrapper dir):
#   bin/rvs
#   lib/librvslib.so*
#   lib/rvs/lib*.so*
#   share/rocm-validation-suite/conf/
#
# After install (extracted to /opt/rocm, opt/ subtree excluded):
#   /opt/rocm/bin/rvs
#   /opt/rocm/lib/librvslib.so*
#   /opt/rocm/lib/rvs/lib*.so*
#   /opt/rocm/share/rocm-validation-suite/conf/
#
# The level_1 conf files are patched to remove package-validation actions
# that require a full ROCm package install (not available in a container).

set -euo pipefail

RVS_TARBALL_URL="${1:?RVS_TARBALL_URL required}"

echo "=== RVS install: tarball ==="
echo "    URL: ${RVS_TARBALL_URL}"

mkdir -p /opt/rocm
wget -qO- "${RVS_TARBALL_URL}" | tar -xzf - -C /opt/rocm --exclude="opt/*"

echo "=== Patching level_1 conf files ==="
find /opt/rocm/share/rocm-validation-suite/conf -name '*level_1*' -type f \
    -exec sed -i \
        '/^[[:space:]]*package:/d; /^[[:space:]]*debpackagelist:/d; s/^[[:space:]]*rpmpackagelist:.*/  rpmpackagelist: rocm-core rocminfo comgr rocblas rocrand hsa-rocr hip-runtime-amd/' \
    {} \;

echo "=== RVS installed at /opt/rocm/bin/rvs ==="
