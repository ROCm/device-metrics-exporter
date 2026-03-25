# Docker installation

This page explains how to install AMD Device Metrics Exporter using Docker.

## System requirements

- ROCm 6.2.0 or later
- Ubuntu 22.04 or later
- Docker (or a Docker-compatible container runtime)

## Installation

The Device Metrics Exporter container is hosted on Docker Hub at [rocm/device-metrics-exporter](https://hub.docker.com/r/rocm/device-metrics-exporter).

**_NOTE:_** : `/sys` mount is required for inband-ras

- Start the container:

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.5.0
```

- Confirm metrics are accessible:

```bash
curl http://localhost:5000/metrics
```

- Review the [Prometheus and Grafana Integration Guide](../integrations/prometheus-grafana.md).

**_NOTE:_** Before performing GPU driver unload/upgrade or partition operations, services must be stopped. See [Troubleshooting - Service Management](#service-management-for-driver-and-partition-operations) for detailed instructions.

## Custom metrics

For information about custom metrics, see [Standalone Container](../configuration/docker.md) for instructions.

## Service Management for Driver and Partition Operations

The Device Metrics Exporter must be stopped before performing the following operations:

- GPU driver unload or upgrade
- GPU partition configuration changes

### Stopping the Service

Stop and remove the Device Metrics Exporter container:

```bash
docker stop device-metrics-exporter
docker rm device-metrics-exporter
```

### Verifying the Service is Stopped

Confirm the container is no longer running:

```bash
docker ps -a | grep device-metrics-exporter
```

### Restarting After Driver or Partition Operations

After completing driver upgrade or partition operations, restart the container using the same command from the [Installation](#installation) section above.
