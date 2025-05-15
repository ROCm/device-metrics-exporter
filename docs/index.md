# AMD Device Metrics Exporter

AMD Device Metrics Exporter enables Prometheus-format metrics collection for AMD GPUs in HPC and AI environments. It provides detailed telemetry, including temperature, utilization, memory usage, and power consumption. This tool includes the following features:

- Prometheus-compatible metrics endpoint
- Rich GPU telemetry data
- Kubernetes integration
- Slurm integration support
- Configurable service ports
- Container-based deployment

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
