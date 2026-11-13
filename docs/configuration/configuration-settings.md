# Default configuration settings

AMD Device Metrics Exporter includes the following default configuration settings that can be modified.

- Metrics endpoint: `http://localhost:5000/metrics`
- Default configuration file: `/etc/metrics/config.json`

See [Kubernetes configuration](configmap.md) for `GPUConfig.HealthThresholds`, including optional `GPU_CPER_MAX_AGE` for fatal CPER age filtering.

See [Kubernetes configuration](configmap.md) for `CommonConfig.HighResolution`, the high-resolution sampling profile for GPU and workload cgroup metrics (`Enable`, `FastPeriodSecs`, `SlowPeriodSecs`).

See [Workload cgroup metrics](workload-cgroup-metrics.md) for per-job CPU, memory and disk I/O metrics (`--monitor-cgroup`), which are not supported on Kubernetes.
