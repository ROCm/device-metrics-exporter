# SR-IOV Docker installation

This page explains how to install AMD Device Metrics Exporter using Docker on
an SR-IOV/GIM hypervisor host for MI-series platforms. This image is
docker-only — there is no Helm chart or GPU Operator support for the SR-IOV
variant. For the baremetal container, see [Docker installation](docker.md).

## System requirements

- MI-series platform configured for SR-IOV/GIM virtualization
- GIM driver version 9.0.0.K or later must already be installed on the
  hypervisor host before running this container. See the
  [MxGPU-Virtualization releases](https://github.com/amd/MxGPU-Virtualization/releases)
  page for GIM driver installation instructions
- Ubuntu 22.04 or later
- Docker (or a Docker-compatible container runtime)

## Installation

The SR-IOV Device Metrics Exporter container is hosted on Docker Hub at
[rocm/device-metrics-exporter-sriov](https://hub.docker.com/r/rocm/device-metrics-exporter-sriov).

The container runs both `gpuagent` (SR-IOV/GIM build) and the exporter
together, and requires access to the GIM SR-IOV management device.

**_NOTE:_** `--privileged` is required for the container to access the GIM
SR-IOV management character device (`/dev/gim-smi0`) on the hypervisor host.

- Start the container:

```bash
docker run -d \
  --privileged \
  --device=/dev/gim-smi0 \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  --name device-metrics-exporter-sriov \
  rocm/device-metrics-exporter-sriov:v1.0.0
```

- Confirm metrics are accessible:

```bash
curl http://localhost:5000/metrics
```

## Service Management for Driver and Partition Operations

The SR-IOV Device Metrics Exporter must be stopped before performing GPU
driver unload/upgrade or partition operations:

```bash
docker stop device-metrics-exporter-sriov
docker rm device-metrics-exporter-sriov
```

After completing driver upgrade or partition operations, restart the
container using the same command from the [Installation](#installation)
section above.

## Troubleshooting

### Techsupport Collection

```bash
docker exec -it device-metrics-exporter-sriov metrics-exporter-ts.sh
docker cp device-metrics-exporter-sriov:/var/log/amd-metrics-exporter-techsupport-<timestamp>.tar.gz .
```

### Logs

```bash
docker logs device-metrics-exporter-sriov
```

### Common Issues

1. Container fails to start or exits immediately:
   - Confirm `--privileged` and `--device=/dev/gim-smi0` are present in the
     `docker run` command — both are mandatory, not optional
   - Verify the GIM driver (9.0.0.K or later) is installed and
     `/dev/gim-smi0` exists on the host

2. Metric collection issues:
   - Check that the GIM driver version meets the minimum requirement
   - Confirm the host is actually configured for SR-IOV/GIM virtualization
