# NIC standalone container configuration

To use a custom configuration with the AMD Network Device Metrics Exporter container:

1. Create a config file based on the provided example [config-nic.json](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/example/config-nic.json)
2. Save the file as `config.json` in the `config/` folder
3. Mount the `config/` folder when starting the container

```bash
docker run -d  \
  --privileged   \
  --network=host \
  -v ./config:/etc/metrics \
  --name network-device-metrics-exporter \
  rocm/device-metrics-exporter:nic-v1.1.0 -monitor-nic=true -monitor-gpu=false
```

Use the image tag that matches your AINIC firmware version. See the [compatibility matrix](../index.md#compatibility-matrix).

The exporter watches the configuration directory and reloads the configuration when the file changes, so updates take effect without a container restart.
