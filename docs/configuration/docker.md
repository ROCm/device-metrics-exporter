# Standalone container configuration

For the SR-IOV/GIM hypervisor image, see [SR-IOV standalone container configuration](docker-sriov.md) instead.

To use a custom configuration with the AMD Device Metrics Exporter container:

1. Create a config file based on the provided example [config.json](https://github.com/ROCm/device-metrics-exporter/blob/main/example/config.json)
2. Save `config.json` in the `config/` folder
3. Mount the `config/` folder when starting the container:

**Note:** The `/sys` mount is required for in-band RAS error reporting.

```bash
docker run -d \
  --privileged \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.5.5
```

**Note:** `--privileged` is required. If profiler metrics (`gpu_prof_*`) are
enabled in the config, the `rocpctl` profiler binary additionally requires
`CAP_PERFMON` to access hardware performance counters, which `--privileged`
also grants. Without it, profiler metrics will not appear and permission
errors will be logged.

The exporter watches the configuration directory and reloads the configuration when the file changes, so updates take effect without a container restart.
