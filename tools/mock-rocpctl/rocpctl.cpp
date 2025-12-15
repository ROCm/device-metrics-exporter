#include <iostream>
#include <cstdlib>

int main() {
    std::cout << "mock rocpctl cmdline tool" << std::endl;
    std::abort(); // This sends a SIGABRT signal
    return 0;
}
