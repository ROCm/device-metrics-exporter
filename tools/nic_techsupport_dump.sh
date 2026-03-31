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

# Collect tech support logs for AMD Device (NIC) Metrics Exporter
# Supports both standalone and operator-managed deployments
# Usage:
#    nic_techsupport_dump.sh -r <helm-release-name> [-k <kubeconfig>] [-o yaml/json] <node-name/all>
#

DEFAULT_RESOURCES="nodes events"

OUTPUT_FORMAT="json"
WIDE=""
clr='\033[0m'

usage() {
    echo -e "$0 [-w] [-o yaml/json] [-k kubeconfig] -r helm-release-name <node-name/all>"
    echo -e "   [-w] wide option"
    echo -e "   [-o yaml/json] output format (default json)"
    echo -e "   [-k kubeconfig] path to kubeconfig (default ~/.kube/config)"
    echo -e "   -r helm-release-name (required)"
    exit 0
}

log() {
    echo -e "[$(date +%F_%T) techsupport]$* ${clr}"
}

die() {
    echo -e "$* ${clr}" && exit 1
}

pod_logs() {
    NS=$1
    FEATURE=$2
    NODE=$3
    PODS=$4

    KNS="${KUBECTL} -n ${NS}"
    mkdir -p ${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}
    for lpod in ${PODS}; do
        pod=$(basename ${lpod})
        # Get pod status
        POD_STATUS=$(${KNS} get pod "${pod}" -o jsonpath='{.status.phase}' 2>/dev/null)
        log "   ${NS}/${pod} (status: ${POD_STATUS})"

        # Always collect describe output for all pods (running, failed, crashloop, etc.)
        ${KNS} describe pod "${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/describe_${NS}_${pod}.txt 2>&1 || \
            echo "Failed to describe pod ${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/describe_${NS}_${pod}.txt

        # pod pending should be skipped for logs
        if [ "${POD_STATUS}" == "Pending" ]; then
            echo "Pod ${pod} is in Pending state, skipping logs collection." >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/${NS}_${pod}_logs_skipped.txt
            continue
        else
            # Collect current logs if available (works for Running, CrashLoopBackOff, Failed pods)
            ${KNS} logs "${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/${NS}_${pod}.txt 2>&1 || \
                echo "Failed to collect current logs for ${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/${NS}_${pod}.txt
        fi

        # Collect previous logs if available (critical for crashloop/failed pods)
        ${KNS} logs -p "${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/${NS}_${pod}_previous.txt 2>&1 || \
            echo "No previous logs available for ${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/${NS}_${pod}_previous.txt

        # For failed/crashloop pods, also collect events
        if [ "${POD_STATUS}" != "Running" ]; then
            ${KNS} get events --field-selector involvedObject.name=${pod} >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/events_${NS}_${pod}.txt 2>&1 || \
                echo "Failed to collect events for ${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/events_${NS}_${pod}.txt
        fi
    done
    echo ${PODS} >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/pods.txt || true
}

while getopts who:k:r: opt; do
    case ${opt} in
    w)
        WIDE="-o wide"
        ;;
    o)
        OUTPUT_FORMAT="${OPTARG}"
        ;;
    k)
        KUBECONFIG="--kubeconfig ${OPTARG}"
        ;;
    r)
        HELM_RELEASENAME="${OPTARG}"
        ;;
    h)
        usage
        ;;
    ?)
        usage
        ;;
    esac
done
shift "$((OPTIND - 1))"
NODES=$@
KUBECTL="kubectl ${KUBECONFIG}"
RELNAME=${HELM_RELEASENAME}

[ -z "${NODES}" ] && die "node-name/all required"
[ -z "${RELNAME}" ] && die "helm-release-name required (use -r flag)"

# Detect helm release namespace
log "Detecting namespace for helm release: ${RELNAME}"
HELM_NS=$(helm list -A ${KUBECONFIG} 2>/dev/null | awk -v name="${RELNAME}" '$1==name {print $2}')
[ -z "${HELM_NS}" ] && die "Unable to find helm release: ${RELNAME} in any namespace"
# Check if multiple namespaces were returned
HELM_NS_COUNT=$(echo "${HELM_NS}" | wc -l)
[ "${HELM_NS_COUNT}" -gt 1 ] && die "Multiple helm releases found with name ${RELNAME} in different namespaces: ${HELM_NS}"
log "Helm release namespace: ${HELM_NS}"

# Set exporter namespace (same as helm namespace)
EXPORTER_NS=${HELM_NS}
KNS="${KUBECTL}"
if [ "${EXPORTER_NS}" != "" ]; then
	KNS="${KUBECTL} -n ${EXPORTER_NS}"
fi

# Find metrics exporter daemonset in the namespace
log "Detecting NIC metrics exporter daemonset..."

# Search for any metrics exporter daemonset (works for both standalone and operator-managed)
METRICS_EXPORTER_DS_ALL=$(${KNS} get daemonset -o name 2>/dev/null | grep '\-metrics-exporter$')
METRICS_EXPORTER_COUNT=$(echo "${METRICS_EXPORTER_DS_ALL}" | grep -c '\-metrics-exporter$' || true)
if [ "${METRICS_EXPORTER_COUNT}" -eq 0 ]; then
	die "Unable to find metrics exporter daemonset in namespace ${EXPORTER_NS}"
elif [ "${METRICS_EXPORTER_COUNT}" -gt 1 ]; then
	echo "Multiple metrics exporter daemonsets found in namespace ${EXPORTER_NS}:"
	echo "${METRICS_EXPORTER_DS_ALL}"
	die "Please ensure only one metrics exporter daemonset exists in the namespace"
fi

METRICS_EXPORTER_DS=$(echo "${METRICS_EXPORTER_DS_ALL}" | head -1)

# Extract app label from daemonset name
EXPORTER_APP_LABEL=$(echo "${METRICS_EXPORTER_DS}" | sed 's|^daemonset.apps/||')
log "Found daemonset: ${EXPORTER_APP_LABEL}"

# Verify this is a NIC exporter by checking container args
MONITOR_NIC_ARG=$(${KNS} get daemonset "${EXPORTER_APP_LABEL}" -o jsonpath='{.spec.template.spec.containers[0].args[*]}' 2>/dev/null | grep -o '\-monitor-nic=true')

if [ -z "${MONITOR_NIC_ARG}" ]; then
	die "Daemonset ${EXPORTER_APP_LABEL} does not appear to be a NIC exporter (no -monitor-nic=true arg found)"
fi

# Get pod selector from daemonset (works universally for both standalone and operator-managed)
POD_SELECTOR_JSON=$(${KNS} get daemonset "${EXPORTER_APP_LABEL}" -o jsonpath='{.spec.selector.matchLabels}' 2>/dev/null)
# Convert JSON to label selector format (e.g., "app=foo,tier=backend")
POD_SELECTOR=$(echo "${POD_SELECTOR_JSON}" | sed 's/[{}"]//g' | sed 's/:/=/g' | sed 's/ //g')
log "POD_SELECTOR: ${POD_SELECTOR}"

# Get server port from Kubernetes service
# Filter by pod selector to get the correct metrics exporter service
# Support both "http" (standalone helm) and "exporter-port" (operator/kube-rbac-proxy) port names
SERVER_PORT=$(${KNS} get service -l "${POD_SELECTOR}" -o jsonpath='{.items[0].spec.ports[?(@.name=="http" || @.name=="exporter-port")].port}' 2>/dev/null | head -1)
[ -z "${SERVER_PORT}" ] && SERVER_PORT="5001"
log "SERVER_PORT: ${SERVER_PORT}"

# Detect if deployment is operator-managed or standalone
MANAGED_BY_LABEL=$(${KNS} get daemonset "${EXPORTER_APP_LABEL}" -o jsonpath='{.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null)
HAS_OWNER_REF=$(${KNS} get daemonset "${EXPORTER_APP_LABEL}" -o jsonpath='{.metadata.ownerReferences[0].kind}' 2>/dev/null)

if [ ! -z "${HAS_OWNER_REF}" ]; then
	DEPLOYMENT_TYPE="operator-managed"
elif [ ! -z "${MANAGED_BY_LABEL}" ]; then
	DEPLOYMENT_TYPE="standalone"
fi
log "DEPLOYMENT_TYPE: ${DEPLOYMENT_TYPE}"

# Set tech support filename
TECH_SUPPORT_FILE=techsupport-nic-$(date "+%F_%T" | sed -e 's/:/-/g')

rm -rf ${TECH_SUPPORT_FILE}
mkdir -p ${TECH_SUPPORT_FILE}
${KUBECTL} version >${TECH_SUPPORT_FILE}/kubectl.txt || die "${KUBECTL} failed"

# Verify pods exist by checking daemonset status
DESIRED_PODS=$(${KNS} get daemonset "${EXPORTER_APP_LABEL}" -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null)
READY_PODS=$(${KNS} get daemonset "${EXPORTER_APP_LABEL}" -o jsonpath='{.status.numberReady}' 2>/dev/null)
[ -z "${DESIRED_PODS}" ] || [ "${DESIRED_PODS}" -eq 0 ] && die "No exporter pods scheduled for daemonset ${EXPORTER_APP_LABEL} in namespace ${EXPORTER_NS}"

echo -e "EXPORTER_NAMESPACE:$EXPORTER_NS" >${TECH_SUPPORT_FILE}/namespace.txt
echo -e "EXPORTER_TYPE:nic" >>${TECH_SUPPORT_FILE}/namespace.txt
echo -e "DEPLOYMENT_TYPE:$DEPLOYMENT_TYPE" >>${TECH_SUPPORT_FILE}/namespace.txt
echo -e "DAEMONSET_NAME:$EXPORTER_APP_LABEL" >>${TECH_SUPPORT_FILE}/namespace.txt
log "EXPORTER_NAMESPACE:$EXPORTER_NS (${DESIRED_PODS} desired, ${READY_PODS} ready)\n"

# default namespace
for resource in ${DEFAULT_RESOURCES}; do
    ${KUBECTL} get -A ${resource} ${WIDE} >${TECH_SUPPORT_FILE}/${resource}.txt 2>&1
    ${KUBECTL} describe -A ${resource} >>${TECH_SUPPORT_FILE}/${resource}.txt 2>&1
    ${KUBECTL} get -A ${resource} -o ${OUTPUT_FORMAT} >${TECH_SUPPORT_FILE}/${resource}.${OUTPUT_FORMAT} 2>&1
done


CONTROL_PLANE=$(${KUBECTL} get nodes -l node-role.kubernetes.io/control-plane | grep -w Ready | awk '{print $1}')
# logs
if [ "${NODES}" == "all" ]; then
    NODES=$(${KUBECTL} get nodes | grep -w Ready | awk '{print $1}')
else
    NODES=$(echo "${NODES} ${CONTROL_PLANE}" | tr ' ' '\n' | sort -u)
fi

log "logs:"
for node in ${NODES}; do
    log " ${node}:"
    ${KUBECTL} get nodes ${node} | grep -w Ready >/dev/null || continue
    mkdir -p ${TECH_SUPPORT_FILE}/${node}
    ${KUBECTL} describe nodes ${node} >${TECH_SUPPORT_FILE}/${node}/${node}.txt

    EXPORTER_PODS=$(${KNS} get pods -o name --field-selector spec.nodeName=${node} -l "${POD_SELECTOR}")
    pod_logs $EXPORTER_NS "metrics-exporter" $node $EXPORTER_PODS

    # Prefer a fully Running pod for exec; fall back to Terminating (still alive
    # during grace period when the node is tainted NoExecute).
    RUNNING_POD=""
    TERMINATING_POD=""
    for expod in ${EXPORTER_PODS}; do
        pod=$(basename ${expod})
        POD_STATUS=$(${KNS} get pod "${pod}" -o jsonpath='{.status.phase}' 2>/dev/null)
        DELETION_TS=$(${KNS} get pod "${pod}" -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null)
        if [ "${POD_STATUS}" == "Running" ] && [ -z "${DELETION_TS}" ]; then
            RUNNING_POD="${pod}"
            break
        elif [ -n "${DELETION_TS}" ] && [ -z "${TERMINATING_POD}" ]; then
            TERMINATING_POD="${pod}"
        fi
    done
    EXEC_POD="${RUNNING_POD:-${TERMINATING_POD}}"

    if [ -z "${EXEC_POD}" ]; then
        log "   No Running or Terminating pod found for node ${node}, skipping exec commands"
        cat >${TECH_SUPPORT_FILE}/${node}/missing-data-reason.txt <<REASON
No Running or Terminating exporter pod found on node ${node} at collection time.
This typically occurs when the node is tainted (e.g. during network maintenance),
causing the DaemonSet pod to be evicted before techsupport was collected.
REASON
    else
        # Common exporter commands
        log "   exporter version"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "server -version" >${TECH_SUPPORT_FILE}/${node}/exporterversion.txt || true
        log "   exporter health"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "metricsclient list" >${TECH_SUPPORT_FILE}/${node}/exporterhealth.txt || true
        log "   exporter config"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "cat /etc/metrics/config.json" >${TECH_SUPPORT_FILE}/${node}/exporterconfig.json || true
        log "   exporter pod details"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "metricsclient pod-resources" >${TECH_SUPPORT_FILE}/${node}/exporterpod.json || true
        log "   exporter node details"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "metricsclient node-pods" >${TECH_SUPPORT_FILE}/${node}/exporternode.txt || true

        # NIC-specific commands
        log "   metrics endpoint"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "curl -fsS http://localhost:${SERVER_PORT}/metrics" >${TECH_SUPPORT_FILE}/${node}/metrics.txt 2>&1 || true

        log "   RDMA statistics"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "rdma statistic -j" >${TECH_SUPPORT_FILE}/${node}/rdma-stats.json || true

        log "   ethtool statistics for AMD interfaces"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "for iface in \$(ls /sys/class/net/); do VENDOR_ID=\$(cat /sys/class/net/\${iface}/device/vendor 2>/dev/null); if [ \"\$VENDOR_ID\" == \"0x1dd8\" ]; then echo \"Interface: \$iface\"; ethtool -S \$iface 2>/dev/null; echo \"\"; fi; done" >${TECH_SUPPORT_FILE}/${node}/ethtool-stats.txt || true

        # Check if nicctl is available before running nicctl commands
        NICCTL_AVAILABLE=$(${KNS} exec -i ${EXEC_POD} -- sh -c "command -v nicctl >/dev/null 2>&1 && echo 'yes' || echo 'no'")
        if [ "${NICCTL_AVAILABLE}" == "yes" ]; then
            log "   nicctl statistics"
            mkdir -p ${TECH_SUPPORT_FILE}/${node}/nicctl
            ${KNS} exec -i ${EXEC_POD} -- sh -c "nicctl show port statistics -j" >${TECH_SUPPORT_FILE}/${node}/nicctl/port-stats.json || true
            ${KNS} exec -i ${EXEC_POD} -- sh -c "nicctl show lif statistics -j" >${TECH_SUPPORT_FILE}/${node}/nicctl/lif-stats.json || true
            ${KNS} exec -i ${EXEC_POD} -- sh -c "nicctl show rdma queue-pair" >${TECH_SUPPORT_FILE}/${node}/nicctl/rdma-qp.txt || true
            ${KNS} exec -i ${EXEC_POD} -- sh -c "nicctl show rdma queue-pair statistics -j" >${TECH_SUPPORT_FILE}/${node}/nicctl/rdma-qp-stats.json || true
        else
            log "   nicctl not available, skipping nicctl statistics"
        fi

        log "   NIC device information"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "ls -la /sys/class/infiniband/" >${TECH_SUPPORT_FILE}/${node}/nic-infiniband-devices.txt || true
        ${KNS} exec -i ${EXEC_POD} -- sh -c "ls -la /sys/class/net/" >${TECH_SUPPORT_FILE}/${node}/nic-net-devices.txt || true

        log "   goroutine dump"
        ${KNS} exec -i ${EXEC_POD} -- sh -c "curl -X GET localhost:${SERVER_PORT}/debug/pprof/goroutine -o /tmp/goroutine.pprof 2>/dev/null && cat /tmp/goroutine.pprof && rm -f /tmp/goroutine.pprof" >${TECH_SUPPORT_FILE}/${node}/goroutine.pprof 2>/dev/null || true
    fi

    ${KUBECTL} get nodes -l "node-role.kubernetes.io/control-plane=NoSchedule" 2>/dev/null | grep ${node} && continue # skip master nodes
done

tar cfz ${TECH_SUPPORT_FILE}.tgz ${TECH_SUPPORT_FILE} && rm -rf ${TECH_SUPPORT_FILE} && log "${TECH_SUPPORT_FILE}.tgz is ready"
