ARG BASE_IMAGE=registry.access.redhat.com/ubi9/ubi:9.4
FROM ${BASE_IMAGE}

LABEL maintainer="AMD Inc"
LABEL OS="registry.access.redhat.com/ubi9/ubi:9.4"

RUN dnf install https://dl.fedoraproject.org/pub/epel/epel-release-latest-9.noarch.rpm -y && \
    crb enable && \
    echo -e "[baseos] \n\
name=Red Hat Enterprise Linux 9.2 Partners - x86_64 \n\
baseurl=https://artifactory-cdn.amd.com/artifactory/list/rhel-remote/9.2/x86_64/os/BaseOS \n\
enabled=1 \n\
gpgcheck=0 \n\
\n\
[appstream] \n\
name=Red Hat Enterprise Linux 9.2 Partners (AppStream) - x86_64 \n\
baseurl=https://artifactory-cdn.amd.com/artifactory/list/rhel-remote/9.2/x86_64/os/AppStream \n\
enabled=1 \n\
gpgcheck=0 \n\
\n\
[crb] \n\
name=Red Hat Enterprise Linux 9.2 Partners (CRB) - x86_64 \n\
baseurl=https://artifactory-cdn.amd.com/artifactory/list/rhel-remote/9.2/x86_64/os/CRB \n\
enabled=1 \n\
gpgcheck=0"> /etc/yum.repos.d/internal-rhel.repo && \
    dnf install https://artifactory-cdn.amd.com:8443/artifactory/list/amdgpu-rpm-remote/rhel/amd-nonfree-radeon-9-1.noarch.rpm -y && \
    amdgpu-repo --amdgpu-build=2096344 --rocm-build=compute-rocm-dkms-no-npi-hipclang/15302 && \
    dnf clean all

RUN dnf install yaml-cpp rocm-smi-lib rocrand -y && \
    rm -rf /var/cache/yum && \
    rm -rf /var/cache/dnf && \
    dnf clean all

RUN dnf install rocm-validation-suite -y && \
    rm -rf /var/cache/yum && \
    rm -rf /var/cache/dnf && \
    dnf clean all

ADD amd-test-runner /amd-test-runner
RUN chmod +x /amd-test-runner

ENTRYPOINT ["/amd-test-runner"]
