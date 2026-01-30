# Standalone container configuration

To use a custom configuration with the AMD Device Metrics Exporter container:

1. Create a config file based on the provided example [config.json](../../example/config.json)
2. Save `config.json` in the `config/` folder
3. Mount the `config/` folder when starting the container
4. Security priviledge `SYS_ADMIN` required for profiler metrics if enabled

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  --cap-add=SYS_ADMIN \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.4.1.2
```

The exporter polls for configuration changes every minute, so updates take effect without container restarts.
