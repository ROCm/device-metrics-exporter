ARG BASE_IMAGE=registry.access.redhat.com/ubi9/ubi-minimal:9.5
FROM ${BASE_IMAGE}

LABEL maintainer="AMD Inc"
LABEL OS="registry.access.redhat.com/ubi9/ubi-minimal:9.5"

ARG ROCM_VERSION=6.4
ARG AMDGPU_VERSION=6.4
ARG AMDGPU_REPO_URL=https://repo.radeon.com

# EPEL repo is required to install yaml-cpp, a dependency of rocm-validation-suite package
RUN microdnf clean all && \
    rpm -ivh https://dl.fedoraproject.org/pub/epel/epel-release-latest-9.noarch.rpm && \
    echo "[amdgpu]" > /etc/yum.repos.d/amdgpu.repo && \
    echo "name=AMDGPU repository" >> /etc/yum.repos.d/amdgpu.repo && \
    echo "baseurl=${AMDGPU_REPO_URL}/amdgpu/${AMDGPU_VERSION}/rhel/9.4/main/x86_64/" >> /etc/yum.repos.d/amdgpu.repo && \
    echo "enabled=1" >> /etc/yum.repos.d/amdgpu.repo && \
    echo "priority=50" >> /etc/yum.repos.d/amdgpu.repo && \
    echo "gpgcheck=1" >> /etc/yum.repos.d/amdgpu.repo && \
    echo "gpgkey=${AMDGPU_REPO_URL}/rocm/rocm.gpg.key" >> /etc/yum.repos.d/amdgpu.repo && \
    echo "[ROCm-${ROCM_VERSION}]" > /etc/yum.repos.d/rocm.repo && \
    echo "name=ROCm${ROCM_VERSION}" >> /etc/yum.repos.d/rocm.repo && \
    echo "baseurl=${AMDGPU_REPO_URL}/rocm/rhel9/${AMDGPU_VERSION}/main" >> /etc/yum.repos.d/rocm.repo && \
    echo "enabled=1" >> /etc/yum.repos.d/rocm.repo && \
    echo "priority=50" >> /etc/yum.repos.d/rocm.repo && \
    echo "gpgcheck=1" >> /etc/yum.repos.d/rocm.repo && \
    echo "gpgkey=${AMDGPU_REPO_URL}/rocm/rocm.gpg.key" >> /etc/yum.repos.d/rocm.repo && \
    microdnf update -y && \
    microdnf install rocrand rocm-validation-suite -y && \
    rm -rf /var/cache/yum && \
    microdnf clean all

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
