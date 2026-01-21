#include <iostream>
#include <cstdlib>

int main() {
    std::cout << "mock rocpctl cmdline tool" << std::endl;
    // print the environment variable value for ROCPROFILER_DEVICE_LOCK_AT_START
    const char* env_value = std::getenv("ROCPROFILER_DEVICE_LOCK_AT_START");
    if (env_value != nullptr) {
        std::cout << "ROCPROFILER_DEVICE_LOCK_AT_START=" << env_value << std::endl;
    } else {
        std::cout << "ROCPROFILER_DEVICE_LOCK_AT_START is not set" << std::endl;
    }
    
    std::abort(); // This sends a SIGABRT signal
    return 0;
}
