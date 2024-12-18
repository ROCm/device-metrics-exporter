# AMD Device Metrics Exporter

AMD Device Metrics Exporter enables Prometheus-format metrics collection for AMD GPUs in HPC and AI environments. It provides detailed telemetry, including temperature, utilization, memory usage, and power consumption.

## Features

- Prometheus-compatible metrics endpoint
- Rich GPU telemetry data
- Kubernetes integration
- Slurm integration support
- Configurable service ports
- Container-based deployment



## Configuration

### Default Settings

- Metrics endpoint: `http://localhost:5000/metrics`
- Default configuration file: `/etc/metrics/config.json`

### Custom Configuration

To use a custom configuration:

1. Create your config file based on the [example config](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/example/config.json)
2. Mount it when starting the container:

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.0.0
```

The exporter polls for configuration changes every minute, so updates take effect without container restarts.

## Available Metrics

The exporter provides extensive GPU metrics including:

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
