#!/usr/bin/env bash
dir=/usr/src/github.com/ROCm/device-metrics-exporter/rocprofilerclient
outdir=$dir/build/

cd $dir

rm -rf build 2>&1 || true
cmake -B build ./ -DCMAKE_PREFIX_PATH=/opt/rocm -DCMAKE_HIP_COMPILER=/opt/rocm-$ROCM_VERSION/lib/llvm/bin/clang++
cmake --build build --target all

if [ $? -ne 0 ]; then
    echo "Build error"
    exit 1
fi

# come back to root directory
cd $dir

ls -lart $outdir

echo "Successfully Built rocprofiler library"
exit 0
