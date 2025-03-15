# Standalone container configuration

To use a custom configuration with the AMD Device Metrics Exporter container:

1. Create a config file based on the provided example [config.json](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/example/config.json)
2. Save `config.json` in the `config/` folder
3. Mount the `config/` folder when starting the container:

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.2.0
```

The exporter polls for configuration changes every minute, so updates take effect without container restarts.
