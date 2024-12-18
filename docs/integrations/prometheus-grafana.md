# Prometheus and Grafana integration

To ingest metrics into Prometheus, add the AMD Device Metrics Exporter endpoint to your Prometheus configuration:

```yaml
scrape_configs:
  - job_name: 'gpu_metrics'
    static_configs:
      - targets: ['exporter_external_ip:5000']
```

Pre-built Grafana dashboards are available in the `grafana/` directory of the repository:

- [GPU Overview Dashboard](https://raw.githubusercontent.com/ROCm/gpu-operator/refs/heads/main/grafana/dashboard_overview.json)
- [Per-Node Dashboard](https://raw.githubusercontent.com/ROCm/gpu-operator/refs/heads/main/grafana/dashboard_node.json)
- [Job-specific Dashboard](https://raw.githubusercontent.com/ROCm/gpu-operator/refs/heads/main/grafana/dashboard_job.json)

Import these dashboards through the Grafana interface for immediate visualization of your GPU metrics.
