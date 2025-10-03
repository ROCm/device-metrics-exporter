#!/usr/bin/env bash
set -euo pipefail
#
# Copyright(C) Advanced Micro Devices, Inc. All rights reserved.
#
# You may not use this software and documentation (if any) (collectively,
# the "Materials") except in compliance with the terms and conditions of
# the Software License Agreement included with the Materials or otherwise as
# set forth in writing and signed by you and an authorized signatory of AMD.
# If you do not have a copy of the Software License Agreement, contact your
# AMD representative for a copy.
#
# You agree that you will not reverse engineer or decompile the Materials,
# in whole or in part, except as allowed by applicable law.
#
# THE MATERIALS ARE DISTRIBUTED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OR
# REPRESENTATIONS OF ANY KIND, EITHER EXPRESS OR IMPLIED.
#
#
# entry point script run on creating a node management container

# default is gpu monitoring enabled
MONITOR_GPU="true"

# Loop through all arguments
for arg in "$@"; do
  case $arg in
    -monitor-gpu=*)
      MONITOR_GPU="${arg#*=}"   # Extract value after '='
      ;;
  esac
done

if [ "$MONITOR_GPU" == "true" ]; then
  LD_PRELOAD=/home/amd/lib/libamd_smi.so.25 /home/amd/bin/gpuagent &
  # sleep before starting prometheus server
  sleep 10
fi

# start prometheus serve
# Run the underlying binary with all arguments passed to the script
/home/amd/bin/server "$@"
