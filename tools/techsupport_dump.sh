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
set -e

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
		log "   ${NS}/${pod}"
		${KNS} logs "${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/${NS}_${pod}.txt
		${KNS} describe pod "${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/describe_${NS}_${pod}.txt
		${KNS} logs -p "${pod}" --tail 1 >/dev/null 2>&1 && ${KNS} logs -p "${pod}" >${TECH_SUPPORT_FILE}/${NODE}/${FEATURE}/${NS}_${pod}_previous.txt
	done
	echo ${PODS} >${TECH_SUPPORT_FILE}/${node}/${FEATURE}/pods.txt
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

log "logs:"
for node in ${NODES}; do
	log " ${node}:"
	${KUBECTL} get nodes ${node} | grep -w Ready >/dev/null || continue
	mkdir -p ${TECH_SUPPORT_FILE}/${node}
	${KUBECTL} describe nodes ${node} >${TECH_SUPPORT_FILE}/${node}/${node}.txt

	KNS="${KUBECTL} -n ${EXPORTER_NS}"
	EXPORTER_PODS=$(${KNS} get pods -o name --field-selector spec.nodeName=${node} -l "app=${RELNAME}-amdgpu-metrics-exporter")
	pod_logs $EXPORTER_NS "metrics-exporter" $node $EXPORTER_PODS
	# gpuagent logs
	GPUAGENT_LOGS="gpu-agent.log gpu-agent-api.log gpu-agent-err.log"
	mkdir -p ${TECH_SUPPORT_FILE}/${node}/gpu-agent
	for l in ${GPUAGENT_LOGS}; do
		for expod in ${EXPORTER_PODS}; do
			pod=$(basename ${expod})
			${KUBECTL} cp ${EXPORTER_NS}/${pod}:"/run/$l" ${TECH_SUPPORT_FILE}/${node}/gpu-agent/$l >/dev/null || true
		done
	done
	#exporter version 
	log "   exporter version"
	${KUBECTL} exec -it ${EXPORTER_PODS} -- sh -c "/home/amd/bin/server -version" >${TECH_SUPPORT_FILE}/${node}/exporterversion.txt

	${KUBECTL} get nodes -l "node-role.kubernetes.io/control-plane=NoSchedule" 2>/dev/null | grep ${node} && continue # skip master nodes
done

tar cfz ${TECH_SUPPORT_FILE}.tgz ${TECH_SUPPORT_FILE} && rm -rf ${TECH_SUPPORT_FILE} && log "${TECH_SUPPORT_FILE}.tgz is ready"
