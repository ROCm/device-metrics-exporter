#!/bin/bash -e



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

# copy all artificates and set proper file permissions
if [ "$MOCK" == "1" ]; then
    tar -xf $TOP_DIR/assets/gpuagent_mock.bin.gz -C $TOP_DIR/docker/
else
    tar -xf $TOP_DIR/assets/gpuagent_static.bin.gz -C $TOP_DIR/docker/
fi
chmod +x $TOP_DIR/docker/gpuagent
cp $TOP_DIR/assets/patch/libamd_smi.so.24.7.60300 $TOP_DIR/docker/
ln -f $TOP_DIR/assets/gpuctl.gobin $TOP_DIR/docker/gpuctl
ln -f $TOP_DIR/bin/amd-metrics-exporter $TOP_DIR/docker/amd-metrics-exporter
cp $TOP_DIR/LICENSE $TOP_DIR/docker/LICENSE
