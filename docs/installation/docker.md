# Docker installation

This page explains how to install AMD Device Metrics Exporter using Docker.

## System requirements

- ROCm 6.2.0 or later
- Ubuntu 22.04 or later
- Docker (or a Docker-compatible container runtime)

## Installation

The Device Metrics Exporter container is hosted on Docker Hub at [rocm/device-metrics-exporter](https://hub.docker.com/r/rocm/device-metrics-exporter).

- Start the container:

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -p 5000:5000 \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.2.1
```

- Confirm metrics are accessible:

```bash
curl http://localhost:5000/metrics
```

- Review the [Prometheus and Grafana Integration Guide](../integrations/prometheus-grafana.md).

## Custom metrics

For information about custom metrics, see [Standalone Container](../configuration/docker.md) for instructions.
