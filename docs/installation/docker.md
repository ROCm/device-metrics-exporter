# Docker

## Prerequisites

- ROCm 6.2.0
- Docker (or a Docker-compatible container runtime)

## Installation

The Metrics Exporter container is hosted on Docker Hub at [rocm/device-metrics-exporter](https://hub.docker.com/r/rocm/device-metrics-exporter).

- Start the container:

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -p 5000:5000 \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.0.0
```

- Confirm metrics are accessible:

```bash
curl http://localhost:5000/metrics
```

- Review the [Prometheus and Grafana Integration Guide](../integrations/prometheus-grafana.md)

## Custom metrics

Please refer to the [Standalone Container](../configuration/docker.md) configuration documentation for instructions.
