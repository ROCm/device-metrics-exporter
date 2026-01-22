# Building Device Metrics Exporter with GPU Agent

This guide covers building the device-metrics-exporter including custom-built agent-agent binaries.

## Prerequisites

- Docker installed and running
- Git
- Make

## Step 1: Build GPU Agent

Clone and build the GPU agent:

```bash
git clone https://github.com/ROCm/gpu-agent.git
cd gpu-agent
git checkout <branch-or-commit>
git submodule update --init --recursive -f
```

Verify submodules were initialized successfully:

```bash
git submodule status
```

All submodules should show a commit hash without a `-` prefix (which indicates uninitialized).

Build using the containerized environment:

```bash
make build-container
GPUAGENT_BLD_CONTAINER_IMAGE=gpuagent-builder-rhel:9 make docker-shell
```

This starts a container with an attached bash shell.

Inside the container, build the binaries:

```bash
make gopkglist
cd sw/nic/gpuagent/
go mod vendor
make
```

A successful build should show output similar to:

```bash
make gpuctl
make[1]: Entering directory '/usr/src/github.com/ROCm/gpu-agent/sw/nic/gpuagent'
building gpuctl
CGO_ENABLED=0 go build -C cli -o /usr/src/github.com/ROCm/gpu-agent/sw/nic/build/x86_64/sim/bin/gpuctl
make[1]: Leaving directory '/usr/src/github.com/ROCm/gpu-agent/sw/nic/gpuagent'
```

Artifacts location:

- `gpuagent` binary: `${TOP_DIR}/sw/nic/build/x86_64/sim/bin/gpuagent`
- `gpuctl` binary: `${TOP_DIR}/sw/nic/build/x86_64/sim/bin/gpuctl`

Prepare artifacts to use in device metrics exporter:

```bash
cd /usr/src/github.com/ROCm/gpu-agent/sw/nic/build/x86_64/sim/bin/
tar -zcvf gpuagent_static.tar.gz gpuagent
cp gpuctl gpuctl.gobin
```

## Step 2: Build Device Metrics Exporter

Clone the exporter repository:

```bash
git clone https://github.com/ROCm/device-metrics-exporter
cd device-metrics-exporter
git checkout <branch-or-tag>
```

Copy the GPU agent binaries from Step 1:

```bash
cp ../gpu-agent/sw/nic/build/x86_64/sim/bin/gpuagent_static.tar.gz assets/gpuagent_static.bin.gz
cp ../gpu-agent/sw/nic/build/x86_64/sim/bin/gpuctl.gobin assets/gpuctl.gobin
```

Build the exporter:

```bash
make build-dev-container
make docker-shell
```

This starts a container with an attached bash shell.

Inside the container, build the Docker image:

```bash
make docker
```

A successful build should show output similar to:

```bash
make -C docker docker-save TOP_DIR=/usr/src/github.com/ROCm/device-metrics-exporter
make[1]: Entering directory '/usr/src/github.com/ROCm/device-metrics-exporter/docker'
saving docker image to device-metrics-exporter-latest.tar.gz
make[1]: Leaving directory '/usr/src/github.com/ROCm/device-metrics-exporter/docker'
```

The build produces a Docker image containing the device-metrics-exporter with your custom GPU agent binaries.

Exit the container shell and find the built image at:

```bash
./docker/device-metrics-exporter-latest.tar.gz
```

## Loading the Docker Image

Load the image using:

```bash
docker load -i <path-to-image.tar.gz>
```

Example: `docker load -i ./docker/device-metrics-exporter-latest.tar.gz`

## Running and Verifying

Run the container:

```bash
docker run -d --device=/dev/dri --device=/dev/kfd -p 5050:5000 --name device-metrics-exporter rocm/device-metrics-exporter:latest
```

Verify the container is running:

```bash
docker ps
```

The container named `device-metrics-exporter` should appear with status `Up`.

Test the metrics endpoint:

```bash
curl localhost:5050/metrics
```

This should return the metrics output, confirming the exporter is working correctly.
