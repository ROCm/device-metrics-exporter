# Standalone container configuration

To use a custom configuration with the AMD Network Device Metrics Exporter container:

1. Create a config file based on the provided example [config.json](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/example/config.json)
2. Save `config.json` in the `config/` folder
3. Mount the `config/` folder  and `nicctl` binary from AINIC installation,  when starting the container:

```bash
docker run -d  \
--privileged   \
--network=host \
-v ./config:/etc/metrics \
-v /usr/sbin/nicctl:/usr/sbin/nicctl  -v ./config:/etc/metrics  \
--name network-device-metrics-exporter \
rocm/device-metrics-exporter:v|version| -monitor-nic=true -monitor-gpu=false
```

The exporter polls for configuration changes every minute, so updates take effect without container restarts.
