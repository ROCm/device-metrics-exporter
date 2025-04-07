#!/usr/bin/env bash
set -x
#set -eou pipefail
dir=/usr/src/github.com/ROCm/gpu-agent

cd $dir/sw/nic

make -C gpuagent
if [ $? -ne 0 ]; then
    echo "Build error"
    exit 1
fi

ls build/x86_64/sim/bin/gpuagent

echo "gpuagent build successfull"
exit 0
