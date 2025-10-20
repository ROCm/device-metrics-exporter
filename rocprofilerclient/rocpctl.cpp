// MIT License
//
// Copyright (c) 2023 Advanced Micro Devices, Inc. All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

#include <hip/hip_runtime.h>

#include <unistd.h>
#include <vector>
#include <iostream>

#include "RocpCounterSampler.h"

#define HIP_CALL(call)                                                                             \
    do                                                                                             \
    {                                                                                              \
        hipError_t err = call;                                                                     \
        if(err != hipSuccess)                                                                      \
        {                                                                                          \
            fprintf(stderr, "%s\n", hipGetErrorString(err));                                       \
            abort();                                                                               \
        }                                                                                          \
    } while(0)


// specify list of metrics in arguments to collect
int
main(int argc, char** argv)
{
    int ntotdevice = 0;
    HIP_CALL(hipGetDeviceCount(&ntotdevice));

    // Check if any GPU devices are available
    if(ntotdevice == 0) {
        std::cerr << "No GPU devices found. Exiting." << std::endl;
        return -1;
    }

    std::vector<std::string> metric_fields;
    long ndevice = ntotdevice;  // Use actual device count

    if(ndevice > ntotdevice) ndevice = ntotdevice;
    if(ndevice < 1) ndevice = ntotdevice;

    try {
        // Build the metrics vector argument
        for (int i = 1; i < argc; ++i) {
            if(argv[i] != nullptr) {  // Add null check for safety
                metric_fields.push_back(argv[i]);
            }
        }
        
        int rc = amd::rocp::CounterSampler::runSample(metric_fields);
        if (rc != 0) {
            std::cerr << "run sample err: " << rc << "\n"; 
            return -1;
        }
    } catch (const std::exception& e) {
        std::cerr << "Exception caught: " << e.what() << std::endl;
        return -1;
    } catch (...) {
        std::cerr << "Unknown exception caught" << std::endl;
        return -1;
    }
    return 0;
}
