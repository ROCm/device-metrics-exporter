#!/usr/bin/env bash
dir=/usr/src/github.com/ROCm/device-metrics-exporter
outdir=$dir/build/rocprofilerdeplib

mkdir -p $outdir

ls -al /opt/rocm/lib/libamdhip64.so*
ls -al /opt/rocm/lib/librocprofiler-sdk.so*
ls -al /opt/rocm/lib/librocprofiler-register.so*
ls -al /opt/rocm/lib/libamd_comgr.so*
ls -al /opt/rocm/lib/libhsa-runtime64.so*
ls -al /opt/rocm/lib/libhsa-amd-aqlprofile64.so*
ls -al /usr/lib/x86_64-linux-gnu/libnuma.so*

cp -vr /opt/rocm/lib/libamdhip64.so* $outdir/
cp -vr /opt/rocm/lib/librocprofiler-sdk.so* $outdir/
cp -vr /opt/rocm/lib/librocprofiler-register.so* $outdir/
cp -vr /opt/rocm/lib/libamd_comgr.so* $outdir/
cp -vr /opt/rocm/lib/libhsa-runtime64.so* $outdir/
cp -vr /opt/rocm/lib/libhsa-amd-aqlprofile64.so* $outdir/
cp -vr /usr/lib/x86_64-linux-gnu/libnuma.so* $outdir/

ls -lart $outdir

echo "Successfully rocprofiler dependent libraries"
exit 0
