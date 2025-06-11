# Prometheus and Grafana integration

Grafana dashboards provided visualize GPU metrics collected from AMD Device Metrics Exporter via Prometheus. Dashboard files are located in the grafana directory:

- `dashboard_overview.json`: High-level GPU cluster overview.

- `dashboard_gpu.json`: Detailed per-GPU metrics.

- `dashboard_job.json`: GPU usage by job (Slurm and Kubernetes).

- `dashboard_node.json`: Host-level GPU usage.

To ingest metrics into Prometheus, you can use one of the following methods:

### Method 1: Direct Prometheus Configuration

#### Run Prometheus (for Testing)

```bash
docker run -p 9090:9090 -v ./example/prometheus.yml:/etc/prometheus/prometheus.yml -v prometheus-data:/prometheus prom/prometheus
```

#### Installing Grafana (for Testing)

Follow the official [Grafana Debian Installation guide](https://grafana.com/docs/grafana/latest/setup-grafana/installation/debian/).

Start Grafana Server:

```bash
sudo systemctl daemon-reload
sudo systemctl start grafana-server
sudo systemctl status grafana-server
```
#### Configure Prometheus

Add the AMD Device Metrics Exporter endpoint to your Prometheus configuration:

```yaml
scrape_configs:
  - job_name: 'gpu_metrics'
    static_configs:
      - targets: ['exporter_external_ip:5000']
```

### Method 2: Using Prometheus Operator in Kubernetes

If you're using Kubernetes, you can install Prometheus and Grafana using the Prometheus Operator:

1. Add the Prometheus Community Helm repository:
```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
```

2. Install the kube-prometheus-stack (includes Prometheus, Alertmanager, and Grafana):
```bash
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set grafana.enabled=true
```

3. Deploy Device Metrics Exporter with ServiceMonitor enabled:
```bash
helm install metrics-exporter \
  https://github.com/ROCm/device-metrics-exporter/releases/download/v1.3.1/device-metrics-exporter-charts-v1.3.1.tgz \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.interval=15s \
  -n mynamespace --create-namespace
```

For detailed ServiceMonitor configuration options and troubleshooting, please refer to the [Prometheus ServiceMonitor Integration](./prometheus-servicemonitor.md) documentation.

Pre-built Grafana dashboards are available in the `grafana/` directory of the repository:

- [GPU Overview Dashboard](https://raw.githubusercontent.com/ROCm/gpu-operator/refs/heads/main/grafana/dashboard_overview.json)
- [Per-Node Dashboard](https://raw.githubusercontent.com/ROCm/gpu-operator/refs/heads/main/grafana/dashboard_node.json)
- [Job-specific Dashboard](https://raw.githubusercontent.com/ROCm/gpu-operator/refs/heads/main/grafana/dashboard_job.json)

Import these dashboards through the Grafana interface for immediate visualization of your GPU metrics.
