ARG BASE_IMAGE=registry.access.redhat.com/ubi9/ubi:9.6
FROM ${BASE_IMAGE}

LABEL maintainer="AMD Inc"
LABEL OS="registry.access.redhat.com/ubi9/ubi:9.6"

# EPEL repo is required to install yaml-cpp, a dependency of rocm-validation-suite package
RUN dnf clean all && \
    rpm -ivh https://dl.fedoraproject.org/pub/epel/epel-release-latest-9.noarch.rpm && \
    dnf update -y && \
    dnf install -y wget yum && \
    yum install -y https://artifactory-cdn.amd.com/artifactory/list/amdgpu-rpm/rhel/amdgpu-install-internal-7.0_9-1.noarch.rpm && \
    amdgpu-repo --rocm-build=compute-rocm-rel-7.0/24 && \
    dnf install rocrand rocm-validation-suite -y && \
    ln /usr/lib64/libdrm_amdgpu.so.1 /usr/lib64/libdrm_amdgpu.so && \
    rm -rf /var/cache/yum /var/cache/dnf && \
    dnf clean all

WORKDIR /

# Security patch for multiple CVEs
RUN dnf update -y libarchive libxml2 pam python3-requests glibc && \
    rm -rf /var/cache/yum /var/cache/dnf && \
    dnf clean all

ADD amd-test-runner /amd-test-runner
ADD LICENSE /licenses/LICENSE
RUN chmod +x /amd-test-runner

# Set LD_LIBRARY_PATH for ROCm
ENV LD_LIBRARY_PATH=/opt/rocm/lib:$LD_LIBRARY_PATH

LABEL name="amd-device-test-runner" \ 
    maintainer="yan.sun3@amd.com,udaybhaskar.biluri@amd.com,shrey.ajmera@amd.com" \
    vendor="Advanced Micro Devices, Inc." \
    version="v1.4.0" \
    release="v1.4.0" \
    summary="AMD Device Test Runner performs troubleshooting and benchmarking tests on AMD GPUs across Kubernetes clusters." \
    description="AMD Device Test Runner ensures that GPUs are functioning correctly and efficiently by running a series of diagnostic tests and performance benchmarks. It helps identify issues, optimize GPU performance, and validate the readiness of GPU resources by running Rocm Validation Suite (RVS) tests."

ENTRYPOINT ["/amd-test-runner"]
