# Standalone container configuration

To use a custom configuration with the AMD Device Metrics Exporter container:
(for the SR-IOV/GIM hypervisor image, see [SR-IOV standalone container configuration](docker-sriov.md) instead)

1. Create a config file based on the provided example [config.json](../../example/config.json)
2. Save `config.json` in the `config/` folder
3. Mount the `config/` folder when starting the container:

**_NOTE:_** : `/sys` mount is required for inband-ras

```bash
docker run -d \
  --privileged \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.5.1
```

**_NOTE:_** `--privileged` is required. If profiler metrics (`gpu_prof_*`) are
enabled in the config, the `rocpctl` profiler binary additionally requires
`CAP_PERFMON` to access hardware performance counters, which `--privileged`
also grants. Without it, profiler metrics will not appear and permission
errors will be logged.

The exporter polls for configuration changes every minute, so updates take effect without container restarts.
