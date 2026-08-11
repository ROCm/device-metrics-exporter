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

if [ "$AINIC" = "1" ]; then
    ln -f "$TOP_DIR/bin/amd-metrics-exporter" "$TOP_DIR/docker/amd-metrics-exporter"
    ln -f "$TOP_DIR/bin/metricsclient" "$TOP_DIR/docker/metricsclient"
    ln -f "$TOP_DIR/tools/techsupport/metrics-exporter-ts.sh" "$TOP_DIR/docker/metrics-exporter-ts.sh"
    cp "$TOP_DIR/LICENSE" "$TOP_DIR/docker/LICENSE"
    exit 0
fi

# gpuagent source model: with GPUAGENT_FROM_SOURCE=1 (default) all
# targets consume the shared producer's binaries from GPUAGENT_BUILD_DIR
# (build/gpuagent/, built once per make invocation via `make gpuagent-build`).
# With GPUAGENT_FROM_SOURCE=0 each target falls back to its committed
# assets/gpuagent_*.bin.gz prebuilt blob (escape hatch).
GPUAGENT_FROM_SOURCE="${GPUAGENT_FROM_SOURCE:-1}"
GPUAGENT_BUILD_DIR="${GPUAGENT_BUILD_DIR:-$TOP_DIR/build/gpuagent}"

# copy all artificats and set proper file permissions
if [ "$MOCK" == "1" ]; then
    if [ "$GPUAGENT_FROM_SOURCE" == "1" ]; then
        echo "Staging gpuagent_mock from source producer ($GPUAGENT_BUILD_DIR)"
        cp -vf "$GPUAGENT_BUILD_DIR/gpuagent_mock" "$TOP_DIR/docker/gpuagent"
    else
        echo "Staging gpuagent_mock from prebuilt asset blob"
        tar -xf "$TOP_DIR/assets/gpuagent_mock.bin.gz" -C "$TOP_DIR/docker/"
    fi
    ln -f $TOP_DIR/bin/rocpctl-mock $TOP_DIR/docker/rocpctl-mock
    chmod +x $TOP_DIR/docker/gpuagent
elif [ "$SRIOV" == "1" ]; then
    if [ "$GPUAGENT_FROM_SOURCE" == "1" ]; then
        # SR-IOV consumes gpuagent_gim from the shared producer.
        # TODO: verify gpuagent_gim is functionally equivalent to
        # today's assets/gpuagent_sriov_static.bin.gz before this becomes the
        # sole SR-IOV path (design "gpuagent_gim equivalence check"). Until then
        # GPUAGENT_FROM_SOURCE=0 restores the proven prebuilt sriov blob.
        echo "Staging gpuagent_gim from source producer ($GPUAGENT_BUILD_DIR)"
        cp -vf "$GPUAGENT_BUILD_DIR/gpuagent_gim" "$TOP_DIR/docker/gpuagent"
    else
        echo "Staging sriov gim driver gpuagent from prebuilt asset blob"
        tar -xf "$TOP_DIR/assets/gpuagent_sriov_static.bin.gz" -C "$TOP_DIR/docker/"
    fi
    chmod +x $TOP_DIR/docker/gpuagent
else
    # default (non-mock, non-sriov) release image
    if [ "$GPUAGENT_FROM_SOURCE" == "1" ]; then
        echo "Staging gpuagent + gpuctl from source producer ($GPUAGENT_BUILD_DIR)"
        cp -vf "$GPUAGENT_BUILD_DIR/gpuagent" "$TOP_DIR/docker/gpuagent"
        cp -vf "$GPUAGENT_BUILD_DIR/gpuctl" "$TOP_DIR/docker/gpuctl"
    else
        echo "Staging gpuagent from prebuilt asset blob; gpuctl from gpuctl.gobin"
        tar -xf "$TOP_DIR/assets/gpuagent_static.bin.gz" -C "$TOP_DIR/docker/"
        ln -f "$TOP_DIR/assets/gpuctl.gobin" "$TOP_DIR/docker/gpuctl"
    fi
    chmod +x $TOP_DIR/docker/gpuagent $TOP_DIR/docker/gpuctl
fi

# Stage amdsmi (header + lib) and gpuagent patches for the gpuagent-build stage
# (release path). The builder stage COPYs these from the docker/ build context.
if [ "$MOCK" != "1" ] && [ "$SRIOV" != "1" ]; then
    cp -vf $TOP_DIR/assets/amd_smi_lib/x86_64/$OS/lib/amdsmi.h $TOP_DIR/docker/amdsmi.h
    rm -rf $TOP_DIR/docker/patch-gpuagent && mkdir -p $TOP_DIR/docker/patch-gpuagent
    if [ -d $TOP_DIR/patch/gpuagent ]; then
        cp -vf $TOP_DIR/patch/gpuagent/*.patch $TOP_DIR/docker/patch-gpuagent/ 2>/dev/null || true
    fi
fi
if [ "$SRIOV" == "1" ]; then
    echo "Copying sriov gim libs to docker"
    cp -vf $TOP_DIR/assets/gim_smi_lib/x86_64/$OS/lib/libgim_amd_smi.so $TOP_DIR/docker/
    cp -vf $TOP_DIR/assets/gim_smi_lib/x86_64/$OS/lib/amdsmi.h $TOP_DIR/docker/

elif [ -d $TOP_DIR/build/assets/$OS/lib ]; then
    # copy built artifacts for the OS else revert to prebuilt files
    echo "Copying newly built amdsmi to docker"
    echo "Note : user to include the built libs on to the container"
    SMI_LIB_DIR=$TOP_DIR/build/assets/$OS/lib
else
    # copy built artifacts for the OS else revert to prebuilt files
    echo "Copying pre built amdsmi to docker"
    echo "Note : user to include the built libs on to the container"
    SMI_LIB_DIR=$TOP_DIR/assets/amd_smi_lib/x86_64/$OS/lib
fi
# stage ONLY the real versioned .so (libamd_smi.so.X.Y.Z); the Dockerfile derives
# the SONAME-major and unversioned symlinks from it. Avoids ADDing a deref'd
# duplicate of the .so.<maj> symlink into an image layer.
if [ -n "$SMI_LIB_DIR" ]; then
    cp -vfL $SMI_LIB_DIR/libamd_smi.so.*.*.* $TOP_DIR/docker/
    # amdsmi 26.5.0 DT_NEEDEDs the rocm_sysdeps netlink libs; stage them too.
    cp -vfL $SMI_LIB_DIR/librocm_sysdeps_*.so* $TOP_DIR/docker/ 2>/dev/null || true
fi

if [ "$SRIOV" != "1" ]; then
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
fi
if [ "$MOCK" == "1" ] || [ "$SRIOV" == "1" ]; then
    ln -f $TOP_DIR/assets/gpuctl.gobin $TOP_DIR/docker/gpuctl
fi
ln -f $TOP_DIR/bin/amd-metrics-exporter $TOP_DIR/docker/amd-metrics-exporter
ln -f $TOP_DIR/bin/metricsclient $TOP_DIR/docker/metricsclient
ln -f $TOP_DIR/bin/amdgpuhealth $TOP_DIR/docker/amdgpuhealth
ln -f $TOP_DIR/tools/techsupport/metrics-exporter-ts.sh $TOP_DIR/docker/metrics-exporter-ts.sh
cp $TOP_DIR/LICENSE $TOP_DIR/docker/LICENSE
