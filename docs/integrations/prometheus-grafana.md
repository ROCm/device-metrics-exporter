# Prometheus and Grafana integration

Grafana dashboards provided visualize GPU metrics collected from AMD Device Metrics Exporter via Prometheus. Dashboard files are located in the grafana directory:

- `dashboard_overview.json`: High-level GPU cluster overview.

- `dashboard_gpu.json`: Detailed per-GPU metrics.

- `dashboard_job.json`: GPU usage by job (Slurm and Kubernetes).

- `dashboard_node.json`: Host-level GPU usage.

### Run Prometheus (for Testing)

```bash
docker run -p 9090:9090 -v ./example/prometheus.yml:/etc/prometheus/prometheus.yml -v prometheus-data:/prometheus prom/prometheus
```

### Installing Grafana (for Testing)

Follow the official [Grafana Debian Installation guide](https://grafana.com/docs/grafana/latest/setup-grafana/installation/debian/).

Start Grafana Server:

```bash
sudo systemctl daemon-reload
sudo systemctl start grafana-server
sudo systemctl status grafana-server
```

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
