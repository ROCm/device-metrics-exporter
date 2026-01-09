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
#include <cstdint>
#include <vector>
#include <iostream>
#include <filesystem>
#include <fstream>
#include <map>
#include <string>

#include "RocpCounterSampler.h"

namespace {
using PtlStateMap = std::map<int, bool>;

PtlStateMap ReadPtlStates() {
    PtlStateMap result;
    namespace fs = std::filesystem;

    const fs::path drm_root{"/sys/class/drm"};
    std::error_code ec;

    if(!fs::exists(drm_root, ec) || !fs::is_directory(drm_root, ec)) {
        return result;
    }

    for(const auto& entry : fs::directory_iterator(drm_root, ec)) {
        if(ec) break;
        if(!entry.is_directory(ec)) continue;

        const auto name = entry.path().filename().string();
        if(name.rfind("card", 0) != 0) continue;  // must start with "card"

        int card_id = -1;
        try {
            card_id = std::stoi(name.substr(4));
        } catch(...) {
            continue;  // non-numeric suffix, ignore
        }

        fs::path ptl_enable = entry.path() / "device" / "ptl" / "ptl_enable";
        if(!fs::exists(ptl_enable, ec) || !fs::is_regular_file(ptl_enable, ec)) {
            continue;  // PTL directory/file not present, safely ignore
        }

        std::ifstream in(ptl_enable);
        if(!in) {
            continue;
        }

        std::string value;
        in >> value;
        bool enabled = (value == "enabled");
        result.emplace(card_id, enabled);
        // std::cout << "PTL state for card" << card_id << ": "
        //           << value << std::endl;
    }

    return result;
}

void RestorePtlStates(const PtlStateMap& states) {
    namespace fs = std::filesystem;
    std::error_code ec;

    for(const auto& [card_id, enabled] : states) {
        fs::path ptl_enable =
            fs::path("/sys/class/drm") / ("card" + std::to_string(card_id)) / "device" / "ptl" /
            "ptl_enable";

        if(!fs::exists(ptl_enable, ec) || !fs::is_regular_file(ptl_enable, ec)) {
            continue;  // PTL directory/file disappeared, ignore
        }

        std::ofstream out(ptl_enable);
        if(!out) {
            continue;
        }

        out << (enabled ? "enabled" : "disabled") << std::endl;

        // std::cout << "Restored PTL state for card" << card_id << ": "
        //           << (enabled ? "enabled" : "disabled") << std::endl;
    }
}

struct PtlStateGuard {
    explicit PtlStateGuard(PtlStateMap states) : states_(std::move(states)) {}
    ~PtlStateGuard() noexcept {
        try {
            RestorePtlStates(states_);
        } catch(...) {
            // best-effort restore; swallow all exceptions
        }
    }

    PtlStateGuard(const PtlStateGuard&) = delete;
    PtlStateGuard& operator=(const PtlStateGuard&) = delete;
    PtlStateGuard(PtlStateGuard&&) = delete;
    PtlStateGuard& operator=(PtlStateGuard&&) = delete;

private:
    PtlStateMap states_;
};
}  // anonymous namespace

#define HIP_CALL(call)                                                                             \
    do                                                                                             \
    {                                                                                              \
        hipError_t err = call;                                                                     \
        if(err != hipSuccess)                                                                      \
        {                                                                                          \
            fprintf(stderr, "%s\n", hipGetErrorString(err));                                       \
            exit(EXIT_FAILURE);                                                                    \
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
    uint64_t duration = 1000;   // Default sampling duration (in microseconds)
    long ndevice = ntotdevice;  // Use actual device count

    if(ndevice > ntotdevice) ndevice = ntotdevice;
    if(ndevice < 1) ndevice = ntotdevice;

    try {
        // Capture current PTL state for all available DRM cards and ensure it is restored
        // on all exit paths (success and error).
        PtlStateGuard ptl_guard(ReadPtlStates());

        // Parse arguments: -d <duration> (uint64), metric names
        for (int i = 1; i < argc; ++i) {
            if (argv[i] == nullptr) continue;
            std::string arg = argv[i];
            if (arg == "-d") {
                if (i + 1 >= argc || argv[i + 1] == nullptr) {
                    std::cerr << "Option -d requires a numeric argument" << std::endl;
                    return -1;
                }
                try {
                    duration = std::stoull(argv[++i]);
                } catch (const std::exception&) {
                    std::cerr << "Invalid value for -d: " << argv[i] << std::endl;
                    return -1;
                }
            } else {                
                metric_fields.push_back(arg);            
            }
        }
        
        int rc = amd::rocp::CounterSampler::runSample(metric_fields, duration);
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
