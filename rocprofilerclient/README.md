# supported ROCM version

6.4.0

## build instruction from roprofiler-builder shell

```bash
cd ..
make rocprofiler-compile
ls build
```

## sample build output

```text
14:35 [device-metrics-exporter]$ make rocprofiler-compile
-- The CXX compiler identification is GNU 11.4.0
-- The HIP compiler identification is Clang 19.0.0
-- Detecting CXX compiler ABI info
-- Detecting CXX compiler ABI info - done
-- Check for working CXX compiler: /usr/bin/c++ - skipped
-- Detecting CXX compile features
-- Detecting CXX compile features - done
-- Detecting HIP compiler ABI info
-- Detecting HIP compiler ABI info - done
-- Check for working HIP compiler: /opt/rocm-6.4.0/lib/llvm/bin/clang++ - skipped
-- Detecting HIP compile features
-- Detecting HIP compile features - done
-- Found PkgConfig: /usr/bin/pkg-config (found version "0.29.2")
-- Checking for module 'libdw'
--   Found libdw, version 0.186
-- Found libdw: /usr/lib/x86_64-linux-gnu/libdw.so
CMake Warning (dev) at /opt/rocm/lib/cmake/hip/hip-config-amd.cmake:98 (message):
   GPU_TARGETS was not set, and system GPU detection was unsuccsesful.

   The amdgpu-arch tool failed:
   Error: 'Failed to get device count'
   Output: ''

   As a result, --offload-arch will not be set for subsuqent
   compilations, and the default architecture
   (gfx906 for dynamic build / gfx942 for static build) will be used

Call Stack (most recent call first):
  /opt/rocm/lib/cmake/hip/hip-config.cmake:149 (include)
  /opt/rocm/lib/cmake/rocprofiler-sdk/rocprofiler-sdk-config.cmake:107 (find_package)
  CMakeLists.txt:145 (find_package)
This warning is for project developers.  Use -Wno-dev to suppress it.

-- Looking for C++ include pthread.h
-- Looking for C++ include pthread.h - found
-- Performing Test CMAKE_HAVE_LIBC_PTHREAD
-- Performing Test CMAKE_HAVE_LIBC_PTHREAD - Success
-- Found Threads: TRUE
-- Found rocprofiler-sdk: /opt/rocm (found version "0.6.0")
-- Configuring done
-- Generating done
-- Build files have been written to: /usr/src/github.com/ROCm/device-metrics-exporter/rocprofilerclient/build
[ 25%] Building CXX object CMakeFiles/rocpclient.dir/rocpclient.cpp.o
[ 50%] Linking CXX shared library librocpclient.so
[ 50%] Built target rocpclient
Scanning dependencies of target rocpctl
[ 75%] Building HIP object CMakeFiles/rocpctl.dir/rocpctl.cpp.o
[100%] Linking HIP executable rocpctl
[100%] Built target rocpctl
total 1360
drwxrwxr-x 3 1001 1001    4096 Apr 17 22:00 ..
-rw-r--r-- 1 root root   20961 Apr 17 22:00 CMakeCache.txt
-rw-r--r-- 1 root root    1722 Apr 17 22:00 cmake_install.cmake
-rw-r--r-- 1 root root    6652 Apr 17 22:00 Makefile
-rwxr-xr-x 1 root root 1297424 Apr 17 22:00 librocpclient.so
-rwxr-xr-x 1 root root   42160 Apr 17 22:00 rocpctl
drwxr-xr-x 3 root root    4096 Apr 17 22:00 .
drwxr-xr-x 6 root root    4096 Apr 17 22:00 CMakeFiles
Successfully Built rocprofiler library

# scratch notes - internal use
```bash
cmake -B build ./ -DCMAKE_PREFIX_PATH=/opt/rocm -DCMAKE_HIP_COMPILER=/opt/rocm-6.4.0/lib/llvm/bin/clang++

cmake --build build --target all
```
