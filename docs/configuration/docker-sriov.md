# SR-IOV standalone container configuration

To use a custom configuration with the SR-IOV Device Metrics Exporter
container:

1. Create a config file based on the provided example [config.json](../../example/config.json)
2. Save `config.json` in the `config/` folder
3. Mount the `config/` folder when starting the container:

**_NOTE:_** : `/sys` mount is required for inband-ras

```bash
docker run -d \
  --privileged \
  --device=/dev/gim-smi0 \
  -v /sys:/sys:ro \
  -p 5000:5000 \
  -v ./config:/etc/metrics \
  --name device-metrics-exporter-sriov \
  rocm/device-metrics-exporter:sriov-v1.0.0
```

`GPUConfig.ExtraPodLabels`, `GPUConfig.Fields`, `GPUConfig.ProfilerMetrics`,
and `NICConfig` are not applicable under SR-IOV and are omitted
from the shipped default config; `CommonConfig.HealthService.Enable` also
defaults to `false`. See the **Hypervisor** column in the
[GPU Metrics List](metricslist.md) for the list of metrics supported under
this deployment.

The exporter polls for configuration changes every minute, so updates take effect without container restarts.
