# AMD Device Metrics Exporter

AMD Device Metrics Exporter enables real-time collection of telemetry data in Prometheus format from AMD GPUs in HPC and AI environments. It provides comprehensive metrics including temperature, utilization, memory usage, power consumption, and more.

## Quick Start

The Metrics Exporter container is available on Docker Hub:

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -p 5000:5000 \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.3.0
```

## Features

- Prometheus-compatible metrics endpoint
- Rich GPU telemetry data including:
  - Temperature monitoring
  - Utilization metrics
  - Memory usage statistics
  - Power consumption data
  - PCIe bandwidth metrics
  - Performance metrics
- Kubernetes integration via Helm chart
- Slurm integration support
- Configurable service ports
- Container-based deployment

## Requirements

- Ubuntu 22.04, 24.04
- ROCm 6.2.x, 6.3.x, 6.4.x
- Docker (or compatible container runtime)

## Documentation

For detailed documentation including installation guides, configuration options, and metric descriptions, see the [documentation](https://instinct.docs.amd.com/projects/device-metrics-exporter/en/latest/).

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.
