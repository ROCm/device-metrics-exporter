#!/bin/bash
#
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# Standalone version bump for release-branch creation. Rewrites version
# strings across docs/ and helm-charts/ for a GPU or NIC release. Does not
# touch git (no branch/commit/tag) — intended for CICD bootstrap use.
#
# usage:
#   hack/bump-version.sh v1.5.2        # GPU release
#   hack/bump-version.sh 1.5.2         # GPU release (v optional)
#   hack/bump-version.sh nic-v1.2.1    # NIC release
#   hack/bump-version.sh nic-1.2.1     # NIC release (v optional)

set -euo pipefail

usage() {
	echo "usage: $0 <version>" >&2
	echo "  GPU release: $0 v1.5.2  (or 1.5.2)" >&2
	echo "  NIC release: $0 nic-v1.2.1  (or nic-1.2.1)" >&2
	exit 1
}

[ $# -eq 1 ] || usage

RAW_VERSION="$1"

if [[ "$RAW_VERSION" =~ ^nic-v?([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
	TRACK="nic"
	VERSION_NUM="${BASH_REMATCH[1]}"
	VERSION_TAG="nic-v${VERSION_NUM}"
elif [[ "$RAW_VERSION" =~ ^v?([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
	TRACK="gpu"
	VERSION_NUM="${BASH_REMATCH[1]}"
	VERSION_TAG="v${VERSION_NUM}"
else
	echo "error: invalid version '$RAW_VERSION' (expected vX.Y.Z, X.Y.Z, nic-vX.Y.Z, or nic-X.Y.Z)" >&2
	usage
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

CHANGED_FILES=()

require_file() {
	if [ ! -f "$1" ]; then
		echo "error: expected file not found: $1" >&2
		exit 1
	fi
}

# Replace within a byte range of a file (1-based inclusive line numbers),
# used to scope edits to one {tab-item} block in kubernetes-helm.md without
# touching the sibling GPU/NIC block.
sed_range() {
	local file="$1" start="$2" end="$3" pattern="$4"
	sed -i "${start},${end}{${pattern}}" "$file"
}

track_change() {
	CHANGED_FILES+=("$1")
}

# Insert an idempotent placeholder release-notes section for $ver right
# after the file's top-level heading line, unless that section already
# exists. Matches on heading text (not line number) so a preamble before
# the heading doesn't misplace the insert.
insert_releasenotes_placeholder() {
	local file="$1" heading="$2" ver="$3"
	if grep -q "^## ${ver}$" "$file"; then
		echo "notice: ${file} already has a '## ${ver}' section — skipping insert"
		return
	fi
	local tmp_file
	tmp_file="$(mktemp)"
	awk -v ver="$ver" -v heading="$heading" '
		$0 == heading { print; print ""; print "## " ver; print ""; \
			print "- **New Features**"; print "  - TODO"; print ""; \
			print "### Issues Fixed"; print ""; print "- TODO"; print ""; \
			print "### Known Issues"; print ""; print "- TODO"; next }
		{ print }
	' "$file" > "$tmp_file"
	mv "$tmp_file" "$file"
	track_change "$file"
	echo "notice: inserted placeholder '## ${ver}' section into ${file} — fill in before merging"
}

if [ "$TRACK" = "gpu" ]; then
	CONF_PY="docs/conf.py"
	HELM_MD="docs/installation/kubernetes-helm.md"
	VALUES_YAML="helm-charts/values.yaml"
	CHART_YAML="helm-charts/Chart.yaml"
	DOCKERFILE="docker/Dockerfile.exporter-release"
	TESTRUNNER_DOCKERFILE="docker/testrunner/Dockerfile"
	DOCKER_MD="docs/installation/docker.md"
	SINGULARITY_MD="docs/installation/singularity.md"
	CONFIGMAP_MD="docs/configuration/configmap.md"
	CONFIG_DOCKER_MD="docs/configuration/docker.md"
	PROM_GRAFANA_MD="docs/integrations/prometheus-grafana.md"
	SLURM_MD="docs/integrations/slurm-integration.md"
	DEB_PACKAGE_RST="docs/installation/deb-package.rst"
	DEVGUIDE_MD="docs/developerguide.md"
	MAKEFILE="Makefile"
	RELEASENOTES_MD="docs/releasenotes.md"

	for f in "$CONF_PY" "$HELM_MD" "$VALUES_YAML" "$CHART_YAML" "$DOCKERFILE" \
		"$TESTRUNNER_DOCKERFILE" "$DOCKER_MD" "$SINGULARITY_MD" "$CONFIGMAP_MD" \
		"$CONFIG_DOCKER_MD" "$PROM_GRAFANA_MD" "$SLURM_MD" "$DEB_PACKAGE_RST" \
		"$DEVGUIDE_MD" "$MAKEFILE" "$RELEASENOTES_MD"; do
		require_file "$f"
	done

	sed -i -e "s|^version = .*|version = \"${VERSION_NUM}\"|" \
		-e "s|^debian_version = .*|debian_version = \"${VERSION_NUM}\"|" \
		"$CONF_PY"
	track_change "$CONF_PY"

	# kubernetes-helm.md: only the GPU {tab-item} blocks (lines 34-135 in the
	# reference layout). Locate them dynamically by tab-item markers so this
	# stays correct if the doc is reordered.
	GPU_START=$(grep -n '^:::{tab-item} GPU' "$HELM_MD" | head -1 | cut -d: -f1)
	GPU_END=$(awk -v s="$GPU_START" 'NR>s && /^:::$/{print NR; exit}' "$HELM_MD")
	sed_range "$HELM_MD" "$GPU_START" "$GPU_END" \
		's|--version v[0-9]\+\.[0-9]\+\.[0-9]\+|--version '"${VERSION_TAG}"'|g'
	track_change "$HELM_MD"

	sed -i -E "s|^(\s*)tag:.*|\1tag: ${VERSION_TAG}|" "$VALUES_YAML"
	track_change "$VALUES_YAML"

	sed -i -e "s|^version:.*|version: ${VERSION_TAG}|" \
		-e "s|^appVersion:.*|appVersion: ${VERSION_TAG}|" \
		"$CHART_YAML"
	track_change "$CHART_YAML"

	sed -i -e "s|version=\"[^\"]*\"|version=\"${VERSION_TAG}\"|" \
		-e "s|release=\"[^\"]*\"|release=\"${VERSION_TAG}\"|" \
		"$DOCKERFILE"
	track_change "$DOCKERFILE"

	sed -i -e "s|version=\"[^\"]*\"|version=\"${VERSION_TAG}\"|" \
		-e "s|release=\"[^\"]*\"|release=\"${VERSION_TAG}\"|" \
		"$TESTRUNNER_DOCKERFILE"
	track_change "$TESTRUNNER_DOCKERFILE"

	for f in "$DOCKER_MD" "$SINGULARITY_MD" "$CONFIGMAP_MD" "$CONFIG_DOCKER_MD" \
		"$PROM_GRAFANA_MD" "$SLURM_MD" "$DEVGUIDE_MD"; do
		sed -i "s#v[0-9]\+\.[0-9]\+\.[0-9]\+#${VERSION_TAG}#g" "$f"
		track_change "$f"
	done

	sed -i -E "s|(https://repo\.radeon\.com/device-metrics-exporter/apt/)[0-9]+\.[0-9]+\.[0-9]+|\1${VERSION_NUM}|g" \
		"$DEB_PACKAGE_RST"
	track_change "$DEB_PACKAGE_RST"

	sed -i -e "s|^PROJECT_VERSION ?= .*|PROJECT_VERSION ?= ${VERSION_TAG}|" "$MAKEFILE"
	track_change "$MAKEFILE"

	# helm-charts/README.md is normally regenerated from Chart.yaml/values.yaml
	# by `make helm-docs`, but this script only ever touches version strings —
	# no make/build targets — so patch the version badges + default image.tag
	# in place instead of invoking helm-docs.
	HELM_README="helm-charts/README.md"
	require_file "$HELM_README"
	sed -i "s#v[0-9]\+\.[0-9]\+\.[0-9]\+#${VERSION_TAG}#g" "$HELM_README"
	track_change "$HELM_README"

	insert_releasenotes_placeholder "$RELEASENOTES_MD" "# Release Notes" "$VERSION_TAG"

else
	NIC_DEB_MD="docs/installation/nic-debian-package.md"
	NIC_DOCKER_MD="docs/configuration/network-exporter-docker.md"
	HELM_MD="docs/installation/kubernetes-helm.md"
	RELEASENOTES_NIC_MD="docs/releasenotes-nic.md"

	for f in "$NIC_DEB_MD" "$NIC_DOCKER_MD" "$HELM_MD" "$RELEASENOTES_NIC_MD"; do
		require_file "$f"
	done

	sed -i -E "s|(https://repo\.radeon\.com/device-metrics-exporter/nic/apt/)[0-9]+\.[0-9]+\.[0-9]+|\1${VERSION_NUM}|g" \
		"$NIC_DEB_MD"
	track_change "$NIC_DEB_MD"

	sed -i "s#rocm/device-metrics-exporter:nic-v[0-9]\+\.[0-9]\+\.[0-9]\+#rocm/device-metrics-exporter:${VERSION_TAG}#g" \
		"$NIC_DOCKER_MD"
	track_change "$NIC_DOCKER_MD"

	# kubernetes-helm.md: only the NIC {tab-item} blocks. NIC helm charts are
	# tagged with a bare vX.Y.Z (no nic- prefix) — only image references
	# elsewhere use the nic-v form.
	NIC_START=$(grep -n '^:::{tab-item} NIC' "$HELM_MD" | head -1 | cut -d: -f1)
	NIC_END=$(awk -v s="$NIC_START" 'NR>s && /^:::$/{print NR; exit}' "$HELM_MD")
	sed_range "$HELM_MD" "$NIC_START" "$NIC_END" \
		's|--version v[0-9]\+\.[0-9]\+\.[0-9]\+|--version v'"${VERSION_NUM}"'|g'
	track_change "$HELM_MD"

	insert_releasenotes_placeholder "$RELEASENOTES_NIC_MD" "# NIC Exporter Release Notes" "$VERSION_TAG"
fi

echo ""
echo "Bumped ${TRACK} version to ${VERSION_TAG} in:"
printf '  %s\n' "${CHANGED_FILES[@]}" | sort -u
