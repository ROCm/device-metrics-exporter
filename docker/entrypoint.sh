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
# start rdcd and gpuagent processes
# start rdcd in the background
# WORKAROUND FIX : rdcd logs are overflowing on stdin move it to null for now
LD_LIBRARY_PATH=/opt/rocm-6.2.0/lib:/opt/rocm-6.2.0/lib/rdc /opt/rocm-6.2.0/bin/rdcd -u 1>/dev/null 2>&1 &
# sleep before starting gpuagent
sleep 10
# start gpuagent process in the background
LD_LIBRARY_PATH=/home/amd/lib/ /home/amd/bin/gpuagent  &

# sleep before starting promethesu server
sleep 10
# start prometheus server
/home/amd/bin/server
