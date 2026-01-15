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
#limitations under the License.
#

# collect tech support logs
# usage:
#    techsupport_dump.sh node-name/all
#

TECH_SUPPORT_FILE=techsupport-$(date "+%F_%T" | sed -e 's/:/-/g')
DEFAULT_RESOURCES="nodes events"
EXPORTER_RESOURCES="pods daemonsets deployments configmap"

OUTPUT_FORMAT="json"
WIDE=""
clr='\033[0m'

usage() {
	echo -e "$0 [-w] [-o yaml/json] [-k kubeconfig] <node-name/all>"
	echo -e "   [-w] wide option "
	echo -e "   [-o yaml/json] output format (default json)"
	echo -e "   [-k kubeconfig] path to kubeconfig(default ~/.kube/config)"
	echo -e "   [-r helm-release-name] helm release name"
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
[ -z "${RELNAME}" ] && die "helm-release-name required"


rm -rf ${TECH_SUPPORT_FILE}
mkdir -p ${TECH_SUPPORT_FILE}
${KUBECTL} version >${TECH_SUPPORT_FILE}/kubectl.txt || die "${KUBECTL} failed"

EXPORTER_NS=$(${KUBECTL} get pods --no-headers -A -l app=${RELNAME}-amdgpu-metrics-exporter | awk '{ print $1 }' | sort -u | head -n1)

echo -e "EXPORTER_NAMESPACE:$EXPORTER_NS" >${TECH_SUPPORT_FILE}/namespace.txt
log "EXPORTER_NAMESPACE:$EXPORTER_NS \n"

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

KNS="${KUBECTL}"
if [ "${EXPORTER_NS}" != "" ]; then
	KNS="${KUBECTL} -n ${EXPORTER_NS}"
fi

log "logs:"
for node in ${NODES}; do
	log " ${node}:"
	${KUBECTL} get nodes ${node} | grep -w Ready >/dev/null || continue
	mkdir -p ${TECH_SUPPORT_FILE}/${node}
	${KUBECTL} describe nodes ${node} >${TECH_SUPPORT_FILE}/${node}/${node}.txt
	
	EXPORTER_PODS=$(${KNS} get pods -o name --field-selector spec.nodeName=${node} -l "app=${RELNAME}-amdgpu-metrics-exporter")
	pod_logs $EXPORTER_NS "metrics-exporter" $node $EXPORTER_PODS
	
	# Check if there are any running pods before attempting exec commands
	RUNNING_POD=""
	for expod in ${EXPORTER_PODS}; do
		pod=$(basename ${expod})
		POD_STATUS=$(${KNS} get pod "${pod}" -o jsonpath='{.status.phase}' 2>/dev/null)
		if [ "${POD_STATUS}" == "Running" ]; then
			RUNNING_POD="${pod}"
			break
		fi
	done
	
	# Skip exec commands if no running pod found
	if [ -z "${RUNNING_POD}" ]; then
		log "   No running pods found for node ${node}, skipping exec commands"
	else
		# gpuagent logs
		GPUAGENT_LOGS="gpu-agent.log gpu-agent-api.log gpu-agent-err.log"
		mkdir -p ${TECH_SUPPORT_FILE}/${node}/gpu-agent
		for l in ${GPUAGENT_LOGS}; do
			${KNS} exec -it ${RUNNING_POD} -- sh -c "cat /var/log/$l" > ${TECH_SUPPORT_FILE}/${node}/gpu-agent/$l || true
		done
		#exporter version 
		log "   exporter version"
		${KNS} exec -it ${RUNNING_POD} -- sh -c "server -version" >${TECH_SUPPORT_FILE}/${node}/exporterversion.txt || true
		log "   exporter health"
		${KNS} exec -it ${RUNNING_POD} -- sh -c "metricsclient" >${TECH_SUPPORT_FILE}/${node}/exporterhealth.txt || true
		log "   exporter config"
		${KNS} exec -it ${RUNNING_POD} -- sh -c "cat /etc/metrics/config.json" >${TECH_SUPPORT_FILE}/${node}/exporterconfig.json || true
		log "   exporter pod details"
		${KNS} exec -it ${RUNNING_POD} -- sh -c "metricsclient -pod -json" >${TECH_SUPPORT_FILE}/${node}/exporterpod.json || true
		log "   exporter node details"
		${KNS} exec -it ${RUNNING_POD} -- sh -c "metricsclient -npod" >${TECH_SUPPORT_FILE}/${node}/exporternode.txt || true
		log "   amd-smi output"
		${KNS} exec -it ${RUNNING_POD} -- sh -c "amd-smi list" >${TECH_SUPPORT_FILE}/${node}/amd-smi-list.txt || true
		${KNS} exec -it ${RUNNING_POD} -- sh -c "amd-smi metric" >${TECH_SUPPORT_FILE}/${node}/amd-smi-metric.txt || true
		${KNS} exec -it ${RUNNING_POD} -- sh -c "amd-smi static " >${TECH_SUPPORT_FILE}/${node}/amd-smi-static.txt || true
		${KNS} exec -it ${RUNNING_POD} -- sh -c "amd-smi firmware" >${TECH_SUPPORT_FILE}/${node}/amd-smi-firmware.txt || true
		${KNS} exec -it ${RUNNING_POD} -- sh -c "amd-smi partition" >${TECH_SUPPORT_FILE}/${node}/amd-smi-partition.txt || true
		${KNS} exec -it ${RUNNING_POD} -- sh -c "amd-smi xgmi" >${TECH_SUPPORT_FILE}/${node}/amd-smi-xgmi.txt || true
	fi

	${KUBECTL} get nodes -l "node-role.kubernetes.io/control-plane=NoSchedule" 2>/dev/null | grep ${node} && continue # skip master nodes
done

tar cfz ${TECH_SUPPORT_FILE}.tgz ${TECH_SUPPORT_FILE} && rm -rf ${TECH_SUPPORT_FILE} && log "${TECH_SUPPORT_FILE}.tgz is ready"
