ARG BUILD_BASE_IMAGE=ubuntu:22.04
FROM ${BUILD_BASE_IMAGE}

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

# remove old version of go
RUN rm -rf /usr/local/go

RUN apt-get update && apt-get install -y wget protobuf-compiler \
    curl locales ca-certificates build-essential git vim \
    net-tools sudo cmake build-essential jq && \
    install -m 0755 -d /etc/apt/keyrings && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

RUN localedef -i en_US -f UTF-8 en_US.UTF-8

#download docker
RUN curl -k -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc && \
    chmod a+r /etc/apt/keyrings/docker.asc

RUN echo 'deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu jammy stable' > /etc/apt/sources.list.d/docker.list

RUN apt-get update && apt-get install -y \
  docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
  make python3-sphinx python3-pip \
  git && apt-get clean && rm -rf /var/lib/apt/lists/*

# download go1.24.2
RUN wget https://go.dev/dl/go1.24.2.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.24.2.linux-amd64.tar.gz && \
    rm go1.24.2.linux-amd64.tar.gz

# download and install kubectl 
RUN curl -o /usr/local/bin/kubectl -LO 'https://dl.k8s.io/release/v1.30.4/bin/linux/amd64/kubectl' && \
    chmod +x /usr/local/bin/kubectl

# download and install helm
RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 get_helm.sh && ./get_helm.sh

ARG INSECURE_REGISTRY
RUN echo "INSECURE_REGISTRY is: $INSECURE_REGISTRY" && \
    if [ -n "$INSECURE_REGISTRY" ]; then \
    mkdir -p /etc/docker && \
    echo "{ \
    \"insecure-registries\": [\"$INSECURE_REGISTRY\"] \
    }" > /etc/docker/daemon.json; \
    else \
    echo "INSECURE_REGISTRY is not set"; \
    fi

ADD ./entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
