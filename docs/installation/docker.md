# Docker installation

This page explains how to install AMD Device Metrics Exporter using Docker.
For SR-IOV/GIM hypervisor hosts, see [SR-IOV Docker installation](docker-sriov.md) instead.

## System requirements

- ROCm 6.2.0 or later
- Ubuntu 22.04 or later
- Docker (or a Docker-compatible container runtime)

## Installation

The Device Metrics Exporter container is hosted on Docker Hub at [rocm/device-metrics-exporter](https://hub.docker.com/r/rocm/device-metrics-exporter).

**Note:** The `/sys` mount is required for in-band RAS error reporting.

- Start the container:

```bash
docker run -d \
  --privileged \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.5.5
```

- Confirm metrics are accessible:

```bash
curl http://localhost:5000/metrics
```

- Review the [Prometheus and Grafana integration guide](../integrations/prometheus-grafana.md).

**Note:** Stop the exporter before you unload or upgrade the GPU driver or change GPU partitions. See [Service management for driver and partition operations](#service-management-for-driver-and-partition-operations).

## Custom metrics

For information about custom metrics, see [Standalone container configuration](../configuration/docker.md).

## Service management for driver and partition operations

The Device Metrics Exporter must be stopped before performing the following operations:

- GPU driver unload or upgrade
- GPU partition configuration changes

### Stopping the service

Stop and remove the Device Metrics Exporter container:

```bash
docker stop device-metrics-exporter
docker rm device-metrics-exporter
```

### Verifying the service is stopped

Confirm the container is no longer running:

```bash
docker ps -a | grep device-metrics-exporter
```

### Restarting after driver or partition operations

After completing driver upgrade or partition operations, restart the container using the same command from the [Installation](#installation) section above.
