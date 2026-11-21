# Prometheus and Grafana integration

The provided Grafana dashboards visualize GPU metrics collected from AMD Device Metrics Exporter via Prometheus.
Pre-built Grafana dashboards are available in the [`grafana`](https://github.com/ROCm/device-metrics-exporter/tree/main/grafana) directory of the repository:

- [High-level GPU cluster overview](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/grafana/dashboard_overview.json)
- [Detailed per-GPU metrics](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/grafana/dashboard_gpu.json)
- [Host-level GPU usage](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/grafana/dashboard_node.json)
- [GPU usage by job (Slurm and Kubernetes)](https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/grafana/dashboard_job.json)

Import these dashboards through the Grafana interface for immediate visualization of your GPU metrics.

## Grafana dashboard setup

- Variables can be configured at any time in each dashboard's **Settings > Variables** section.

  `g_metrics_prefix`: Prefix added to the metric names in queries (for example, `gpu_gfx_activity` becomes `amd_gpu_gfx_activity`).

- Prefix can be set using the dropdown menu in the top left corner of each dashboard.

## Ingesting metrics into Prometheus

### Method 1: Direct Prometheus configuration

#### Run Prometheus for testing

Run this command from the root of a clone of this repository, which provides `example/prometheus.yml`:

```bash
docker run -p 9090:9090 -v ./example/prometheus.yml:/etc/prometheus/prometheus.yml -v prometheus-data:/prometheus prom/prometheus
```

#### Install Grafana for testing

Follow the [Grafana Debian installation guide](https://grafana.com/docs/grafana/latest/setup-grafana/installation/debian/).

Start the Grafana server:

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

### Method 2: Using the Prometheus Operator in Kubernetes

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
   helm repo add exporter https://rocm.github.io/device-metrics-exporter
   helm repo update
   helm install metrics-exporter exporter/device-metrics-exporter-charts \
     --version v1.5.5 \
     --set serviceMonitor.enabled=true \
     --set serviceMonitor.interval=15s \
     -n mynamespace --create-namespace
   ```

For ServiceMonitor configuration options and troubleshooting, see [Prometheus ServiceMonitor integration](./prometheus-servicemonitor.md).
