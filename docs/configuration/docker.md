# Standalone container configuration

To use a custom configuration with the AMD Device Metrics Exporter container:

1. Create a config file based on the provided example [config.json](../../example/config.json)
2. Save `config.json` in the `config/` folder
3. Mount the `config/` folder when starting the container:

**_NOTE:_** : `/sys` mount is required for inband-ras

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.5.0
```

**_NOTE:_** If profiler metrics (`gpu_prof_*`) are enabled in the config, add
`--privileged` to the `docker run` command. The `rocpctl` profiler binary requires
`CAP_PERFMON` to access hardware performance counters. Without it, profiler metrics
will not appear and permission errors will be logged.

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --privileged \
  --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.5.0
```

The exporter polls for configuration changes every minute, so updates take effect without container restarts.
