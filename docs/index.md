# AMD Device Metrics Exporter

AMD Device Metrics Exporter enables Prometheus-format metrics collection for AMD GPUs in HPC and AI environments. It provides detailed telemetry, including temperature, utilization, memory usage, and power consumption. This tool includes the following features:

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
- Docker (or compatible container runtime)

| Rocm Version | Driver Version | Exporter Image Version | Platform     |
|--------------|----------------|------------------------|--------------|
| 6.2.x        | 6.8.5          | v1.0.0                 | MI2xx, MI3xx |
| 6.3.x        | 6.10.5         | v1.1.0, v1.2.0         | MI2xx, MI3xx |
| 6.4.x        | 6.12.12        | v1.3.0                 | MI3xx        |
| 6.4.x        | 6.12.12        | v1.3.0.1, v1.3.1       | MI2xx, MI3xx |
| 7.0          | 6.14.14        | v1.4.0                 | MI2xx, MI3xx |

## Available Metrics

Device Metrics Exporter provides extensive GPU metrics including:

- Temperature metrics
  - Edge temperature
  - Junction temperature
  - Memory temperature
  - HBM temperature
- Performance metrics
  - GPU utilization
  - Memory utilization
  - Clock speeds
- Power metrics
  - Current power usage
  - Average power usage
  - Energy consumption
- Memory statistics
  - Total VRAM
  - Used VRAM
  - Free VRAM
- PCIe metrics
  - Bandwidth
  - Link speed
  - Error counts

For a full list of available metrics see [this page](./configuration/metricslist.md).
