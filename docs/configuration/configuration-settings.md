# Default configuration settings

AMD Device Metrics Exporter includes the following default configuration settings that can be modified.

- Metrics endpoint: `http://localhost:5000/metrics`
- Default configuration file: `/etc/metrics/config.json`

See [Kubernetes configuration](configmap.md) for `GPUConfig.HealthThresholds`, including optional `GPU_CPER_MAX_AGE` for fatal CPER age filtering.
