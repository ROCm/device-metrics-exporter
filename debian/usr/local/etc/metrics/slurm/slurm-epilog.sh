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
MSG=$(
	cat <<EOF
    {
    "SLURM_JOB_ID": "${SLURM_JOB_ID}",
    "SLURM_JOB_USER": "${SLURM_JOB_USER}",
    "SLURM_JOB_PARTITION": "${SLURM_JOB_PARTITION}",
    "SLURM_CLUSTER_NAME": "${SLURM_CLUSTER_NAME}",
    "SLURM_JOB_GPUS": "${SLURM_JOB_GPUS}",
    "CUDA_VISIBLE_DEVICES": "${CUDA_VISIBLE_DEVICES}",
    "SLURM_SCRIPT_CONTEXT": "${SLURM_SCRIPT_CONTEXT}"
   }
EOF
)
[ -d ${EXPORT_DIR} ] || exit 0
GPUS=$(echo ${CUDA_VISIBLE_DEVICES} | tr "," "\n")
for GPUID in ${GPUS}; do
	rm -f ${EXPORT_DIR}/${GPUID}
done
