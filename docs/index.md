# AMD Device Metrics Exporter

AMD Device Metrics Exporter enables Prometheus-format metrics collection for AMD GPUs in HPC and AI environments. It provides detailed telemetry, including temperature, utilization, memory usage, and power consumption. This tool includes the following features:

- Prometheus-compatible metrics endpoint
- Rich GPU telemetry data
- Kubernetes integration
- Slurm integration support
- Configurable service ports
- Container-based deployment

## Available Metrics

The Device Metrics Exporter provides extensive GPU metrics including:

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

## Troubleshooting

### Logs

View container logs:

```bash
docker logs device-metrics-exporter
```

### Common Issues

1. Port conflicts:
   - Verify port 5000 is available
   - Configure an alternate port through the configuration file

2. Device access:
   - Ensure proper permissions on `/dev/dri` and `/dev/kfd`
   - Verify ROCm is properly installed

3. Metric collection issues:
   - Check GPU driver status
   - Verify ROCm version compatibility
