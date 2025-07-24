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
    if [ -f $TOP_DIR/build/assets/gpuagent ]; then
        echo "Copying newly built gpuagent to docker"
        cp -vf $TOP_DIR/build/assets/gpuagent $TOP_DIR/docker/
    else
        echo "Copying prebuilt gpuagent to docker"
        tar -xf $TOP_DIR/assets/gpuagent_static.bin.gz -C $TOP_DIR/docker/
    fi
fi
chmod +x $TOP_DIR/docker/gpuagent
if [ -d $TOP_DIR/build/assets/$OS/lib ]; then
    # copy built artifacts for the OS else revert to prebuilt files
    echo "Copying newly built amdsmi to docker"
    echo "Note : user to include the built libs on to the container"
    cp -vf $TOP_DIR/build/assets/$OS/lib/libamd_smi.so.* $TOP_DIR/docker/
fi

if [ -f $TOP_DIR/rocprofilerclient/build/librocpclient.so ]; then
	echo "Copying newly built rocprofiler libs and binary"
	cp -vf $TOP_DIR/rocprofilerclient/build/librocpclient.so $TOP_DIR/docker/
	cp -vf $TOP_DIR/rocprofilerclient/build/rocpctl $TOP_DIR/docker/
else
	# copy prebuilt
	echo "Copying prebuilt rocprofiler libs and binary"
	cp -vf $TOP_DIR/assets/rocprofiler/librocpclient.so $TOP_DIR/docker/
	cp -vf $TOP_DIR/assets/rocprofiler/rocpctl $TOP_DIR/docker/
fi

chmod +x $TOP_DIR/docker/rocpctl
ln -f $TOP_DIR/assets/gpuctl.gobin $TOP_DIR/docker/gpuctl
ln -f $TOP_DIR/bin/amd-metrics-exporter $TOP_DIR/docker/amd-metrics-exporter
ln -f $TOP_DIR/bin/metricsclient $TOP_DIR/docker/metricsclient

cp $TOP_DIR/LICENSE $TOP_DIR/docker/LICENSE

# Copy AMD SMI library from assets
if [ -f $TOP_DIR/assets/amd_smi_lib/x86_64/$OS/lib/libamd_smi.so.25.4 ]; then
    echo "Copying AMD SMI library from assets to docker"
    cp -vf $TOP_DIR/assets/amd_smi_lib/x86_64/$OS/lib/libamd_smi.so.25.4 $TOP_DIR/docker/
else
    echo "Warning: AMD SMI library not found at $TOP_DIR/assets/amd_smi_lib/x86_64/RHEL9/lib/libamd_smi.so.25.4"
fi
