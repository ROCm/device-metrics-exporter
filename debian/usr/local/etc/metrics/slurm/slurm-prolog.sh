#!/bin/bash

#
#Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

#Licensed under the Apache License, Version 2.0 (the \"License\");
#you may not use this file except in compliance with the License.
#You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
#Unless required by applicable law or agreed to in writing, software
#distributed under the License is distributed on an \"AS IS\" BASIS,
#WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#See the License for the specific language governing permissions and
#limitations under the License.
#

EXPORT_DIR="/var/run/exporter/"
mod128_array() {
    local arr_str="$1"
    local arr result

    # convert string to array using comma as delimiter
    IFS=',' read -ra arr <<< "$arr_str"

    # modulo 128 to each element
    for i in "${!arr[@]}"; do
        arr[i]=$(( ${arr[i]} % 128 ))
    done

    # join array back into a comma-separated string
    result=$(IFS=','; echo "${arr[*]}")

    echo "$result"
}
# json_str escapes a value for a JSON string. The account is chosen by the
# user (-A), so pasted in raw it could add keys such as SLURM_JOB_USER.
json_str() {
    printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' | tr -d '\000-\037'
}
AMDGPU_DEVICES=$(mod128_array "${CUDA_VISIBLE_DEVICES}")
AMD_SLURM_GPUS=$(mod128_array "${SLURM_JOB_GPUS:-${SPUR_JOB_GPUS}}")
# CUDA_VISIBLE_DEVICES is empty on AMD hardware; fall back to SLURM_JOB_GPUS so
# job_id/job_user labels are still attached to the GPU metrics.
[ -z "${AMDGPU_DEVICES}" ] && AMDGPU_DEVICES="${AMD_SLURM_GPUS}"
# Spur exports some fields only under SPUR_* names.
JOB_ID="${SLURM_JOB_ID:-${SPUR_JOB_ID}}"
JOB_USER="${SLURM_JOB_USER:-${SPUR_JOB_USER}}"
JOB_PARTITION="${SLURM_JOB_PARTITION:-${SPUR_JOB_PARTITION}}"
JOB_ACCOUNT="${SLURM_JOB_ACCOUNT:-${SPUR_JOB_ACCOUNT}}"
MSG=$(
	cat <<EOF
    {
    "SLURM_JOB_ID": "$(json_str "${JOB_ID}")",
    "SLURM_JOB_USER": "$(json_str "${JOB_USER}")",
    "SLURM_JOB_PARTITION": "$(json_str "${JOB_PARTITION}")",
    "SLURM_JOB_ACCOUNT": "$(json_str "${JOB_ACCOUNT}")",
    "SLURM_CLUSTER_NAME": "$(json_str "${SLURM_CLUSTER_NAME}")",
    "SLURM_JOB_GPUS": "${AMD_SLURM_GPUS}",
    "CUDA_VISIBLE_DEVICES": "${AMDGPU_DEVICES}",
    "SLURM_SCRIPT_CONTEXT": "$(json_str "${SLURM_SCRIPT_CONTEXT:-${SPUR_SCRIPT_CONTEXT}}")"
   }
EOF
)
[ -d ${EXPORT_DIR} ] || exit 0
GPUS=$(echo ${AMDGPU_DEVICES} | tr "," "\n")
for GPUID in ${GPUS}; do
	echo "${MSG}" >${EXPORT_DIR}/${GPUID}
done
# One file per job, GPU or not, for the workload cgroup metrics. Written via a
# rename so the exporter never reads a partial file.
case "${JOB_ID}" in
'' | *[!0-9A-Za-z_.+-]*) exit 0 ;;
esac
JOBS_DIR="${EXPORT_DIR}/jobs"
mkdir -p "${JOBS_DIR}" || exit 0
echo "${MSG}" >"${JOBS_DIR}/.${JOB_ID}.tmp" && mv -f "${JOBS_DIR}/.${JOB_ID}.tmp" "${JOBS_DIR}/${JOB_ID}"
exit 0
