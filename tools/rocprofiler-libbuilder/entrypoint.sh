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

cp -vr /opt/rocm/lib/libamdhip64.so* $outdir/
cp -vr /opt/rocm/lib/librocm_kpack.so* $outdir/ 2>/dev/null || true
cp -vr /opt/rocm/lib/librocprofiler-sdk.so* $outdir/
cp -vr /opt/rocm/lib/librocprofiler-register.so* $outdir/
cp -vr /opt/rocm/lib/libamd_comgr.so* $outdir/
cp -vr /opt/rocm/lib/libhsa-runtime64.so* $outdir/
cp -vr /opt/rocm/lib/libhsa-amd-aqlprofile64.so* $outdir/

# libnuma: system path, else therock rocm_sysdeps variant.
if ls /usr/lib/x86_64-linux-gnu/libnuma.so* &>/dev/null; then
    cp -vr /usr/lib/x86_64-linux-gnu/libnuma.so* $outdir/
elif ls /usr/lib64/libnuma.so* &>/dev/null; then
    cp -vr /usr/lib64/libnuma.so* $outdir/
elif ls /opt/rocm/lib/rocm_sysdeps/lib/librocm_sysdeps_numa.so* &>/dev/null; then
    echo "libnuma not found in system paths; using therock rocm_sysdeps variant"
    SYSDEPS_NUMA=$(ls /opt/rocm/lib/rocm_sysdeps/lib/librocm_sysdeps_numa.so* | head -1)
    cp -vf "$SYSDEPS_NUMA" $outdir/libnuma.so
    ln -sf libnuma.so $outdir/libnuma.so.1
    ln -sf libnuma.so $outdir/libnuma.so.1.0.0
else
    echo "WARNING: libnuma not found in any expected location; skipping"
fi

# profiler-sdk transitive deps (rpm/deb ship only $outdir, not full /opt/rocm).
if ls /opt/rocm/lib/rocm_sysdeps/lib/librocm_sysdeps_*.so* &>/dev/null; then
    cp -vr /opt/rocm/lib/rocm_sysdeps/lib/librocm_sysdeps_*.so* $outdir/
fi
cp -vr /opt/rocm/lib/llvm/lib/libLLVM.so* $outdir/ 2>/dev/null || true
cp -vr /opt/rocm/lib/llvm/lib/libclang-cpp.so* $outdir/ 2>/dev/null || true

# libatomic.so.1: profiler-sdk runtime dep, from system path.
for atomic in /usr/lib/x86_64-linux-gnu/libatomic.so.1* /usr/lib64/libatomic.so.1* /lib64/libatomic.so.1*; do
    [ -e "$atomic" ] && cp -vr "$atomic" $outdir/
done

ls -lart $outdir

echo "Successfully rocprofiler dependent libraries"
exit 0
