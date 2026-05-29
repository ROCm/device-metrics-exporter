#!/bin/bash -e
#
# Fetch latest amdgpu.ids from Mesa libdrm and append unreleased GPU entries.
# Used by Docker, RPM, and Debian builds to provide GPU marketing name resolution.
#
# Usage: tools/fetch-amdgpu-ids.sh <output_path>
#   output_path: where to write the combined amdgpu.ids file
#
# Example: tools/fetch-amdgpu-ids.sh docker/amdgpu.ids

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
EXTRA_IDS="$TOP_DIR/assets/drm/amdgpu.ids.extra"
MESA_URL="https://gitlab.freedesktop.org/mesa/libdrm/-/raw/main/data/amdgpu.ids"

OUTPUT="${1:?Usage: $0 <output_path>}"

echo "Fetching amdgpu.ids from Mesa libdrm..."
wget -q -O "$OUTPUT" "$MESA_URL"

if [ -f "$EXTRA_IDS" ]; then
    echo "Appending unreleased GPU entries from assets/drm/amdgpu.ids.extra..."
    echo "" >> "$OUTPUT"
    grep -v '^#' "$EXTRA_IDS" | grep -v '^$' >> "$OUTPUT"
fi

echo "amdgpu.ids written to $OUTPUT"
