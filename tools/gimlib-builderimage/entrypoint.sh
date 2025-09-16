#!/usr/bin/env bash
dir=/usr/src/github.com/ROCm/device-metrics-exporter/libgimsmi
exporteroutdir=$dir/build/exporterout/

cd /usr/src/github.com/ROCm/device-metrics-exporter/libgimsmi
git config --global --add safe.directory $dir
if [ -z $BRANCH ]; then
    echo "branch set to $BRANCH"
    git checkout $BRANCH || true
fi
if [ -z $COMMIT ]; then
    echo "commit set to $COMMIT"
    git reset --hard $COMMIT
fi
cd smi-lib
rm -rf build 2>&1 || true
make

if [ $? -ne 0 ]; then
    echo "Build error"
    exit 1
fi

# come back to root directory
cd $dir

# find which os to look for artifacts in specific directories
os=`cat /etc/os-release | grep ^ID= | cut -d'=' -f 2`

#copy all required files for exporter to exporteroutput directory
mkdir -p $exporteroutdir || true


cp -vf $dir/smi-lib/build/amdsmi/Release/libamdsmi.so $exporteroutdir/libgim_amd_smi.so
cp -vf $dir/smi-lib/interface/amdsmi.h $exporteroutdir/amdsmi.h

ls -lart $exporteroutdir

echo "Successfully Build GIM SMI lib $os branch $BRANCH commit $COMMIT"
exit 0
