#!/usr/bin/env bash
# run-e2e.sh — build and run k8s-e2e tests inside a container
#
# MODES:
#   --dme        DME standalone Helm chart tests (Test001-Test200)
#
# EXAMPLES:
#   # Radeon node — DME standalone tests
#   bash test/k8s-e2e/run-e2e.sh --dme \
#     --kubeconfig /tmp/kubeconfig-e2e \
#     --registry rocm/device-metrics-exporter --imagetag v1.5.0-beta.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DME_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
IMAGE="dme-k8s-e2e:latest"

# ---------- Defaults ----------
KUBECONFIG="${HOME}/.kube/config"
HELM_CHART="${DME_DIR}/helm-charts"
REGISTRY="docker.io/rocm/device-metrics-exporter"
IMAGE_TAG="v1.4.1"
NAMESPACE="dme-standalone-test"
MODE=""
REBUILD=false
EXTRA_ARGS=()

usage() {
    grep '^#' "$0" | grep -v '#!/' | sed 's/^# \{0,1\}//'
    exit 0
}

# ---------- Argument parsing ----------
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dme)            MODE="dme";      shift ;;
        --kubeconfig)     KUBECONFIG="$2"; shift 2 ;;
        --helmchart)      HELM_CHART="$2"; shift 2 ;;
        --registry)       REGISTRY="$2";   shift 2 ;;
        --imagetag)       IMAGE_TAG="$2";  shift 2 ;;
        --namespace)      NAMESPACE="$2";  shift 2 ;;
        --rebuild)        REBUILD=true;    shift ;;
        --help|-h)        usage ;;
        *)                EXTRA_ARGS+=("$1"); shift ;;
    esac
done

if [[ -z "${MODE}" ]]; then
    echo "ERROR: specify a mode: --dme" >&2
    echo "Run with --help for usage." >&2
    exit 1
fi

# ---------- Build image ----------
if [[ "${REBUILD}" == "true" ]] || ! docker image inspect "${IMAGE}" &>/dev/null; then
    echo "[run-e2e] Building ${IMAGE} ..."
    docker build -t "${IMAGE}" -f "${SCRIPT_DIR}/Dockerfile.e2e" "${DME_DIR}"
fi

# ---------- Common docker args ----------
DOCKER_BASE=(--rm -v "${KUBECONFIG}:/kubeconfig:ro")

# ---------- Run function ----------
run_dme() {
    echo ""
    echo "=========================================="
    echo " MODE: DME standalone Helm tests"
    echo " namespace: ${NAMESPACE}"
    echo " image: ${REGISTRY}:${IMAGE_TAG}"
    echo "=========================================="
    docker run "${DOCKER_BASE[@]}" \
        -v "${HELM_CHART}:/helm-charts:ro" \
        "${IMAGE}" \
        -kubeconfig /kubeconfig \
        -helmchart /helm-charts \
        -registry "${REGISTRY}" \
        -imagetag "${IMAGE_TAG}" \
        -namespace "${NAMESPACE}" \
        -platform k8s \
        -test.timeout 30m \
        -v \
        "${EXTRA_ARGS[@]}"
}

# ---------- Dispatch ----------
DME_RC=0

case "${MODE}" in
    dme) run_dme || DME_RC=$? ;;
esac

# ---------- Summary ----------
echo ""
echo "=========================================="
echo " SUMMARY"
[[ ${DME_RC} -eq 0 ]] && echo " DME standalone : PASS" || echo " DME standalone : FAIL (rc=${DME_RC})"
echo "=========================================="

exit ${DME_RC}
