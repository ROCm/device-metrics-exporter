# Prometheus ServiceMonitor Integration

The Device Metrics Exporter Helm chart supports integration with Prometheus Operator through ServiceMonitor Custom Resource Definition (CRD). This enables automated service discovery and scraping of metrics by Prometheus instances managed by the Prometheus Operator.

## Prerequisites

Before using the ServiceMonitor feature, ensure that:

1. You have a Kubernetes cluster with Prometheus Operator installed
2. The ServiceMonitor CRD is available in your cluster (`servicemonitors.monitoring.coreos.com`)

## Enabling ServiceMonitor

To deploy the Device Metrics Exporter with a ServiceMonitor resource, use the following Helm command:

```bash
helm install metrics-exporter \
  https://github.com/ROCm/device-metrics-exporter/releases/download/v1.2.1/device-metrics-exporter-charts-v1.2.1.tgz \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.interval=15s \
  -n mynamespace --create-namespace
```

This will automatically create a ServiceMonitor resource that Prometheus Operator can discover and use to scrape metrics from the Device Metrics Exporter. The ServiceMonitor will be deployed in the same namespace as the metrics service and daemonset. Additional configuration in Prometheus is necessary to select the metrics namespace and the ServiceMonitor. Aternatively, define a `values.yaml` with the desired options and use it in helm install.

```bash
helm install metrics-exporter \
  https://github.com/ROCm/device-metrics-exporter/releases/download/v1.2.1/device-metrics-exporter-charts-v1.2.1.tgz \
  -n mynamespace -f values.yaml --create-namespace
```

## Configuration Options

The following options can be customized for the ServiceMonitor via Helm values `values.yaml`:

```yaml
serviceMonitor:
  # -- Whether to create a ServiceMonitor resource for Prometheus Operator
  enabled: false
  # -- Scrape interval for the ServiceMonitor
  interval: "30s"
  # -- Honor labels configuration for ServiceMonitor
  honorLabels: true
  # -- Honor timestamps configuration for ServiceMonitor
  honorTimestamps: true
  # -- Additional labels for the ServiceMonitor object (to match the Prometheus Operator instance selectors)
  labels: {}
  # -- RelabelConfigs to apply to targets before scraping
  relabelings: []
```

## Verifying ServiceMonitor Deployment

After installation, you can verify that the ServiceMonitor was created correctly:

```bash
kubectl get servicemonitor -n mynamespace
```

Here, `mynamespace` refers to the metrics namespace where the service and daemonset are deployed. You should see a ServiceMonitor resource with the name pattern `<release-name>-amd-metrics-exporter`.

## Troubleshooting

If Prometheus is not scraping metrics from the Device Metrics Exporter:

1. Verify the ServiceMonitor exists and is correctly configured:
   ```bash
   kubectl describe servicemonitor <release-name>-amd-metrics-exporter -n mynamespace
   ```

2. Check if the labels on the ServiceMonitor match the Prometheus Operator's serviceMonitorSelector:
   ```bash
   kubectl get prometheus -n monitoring -o yaml
   ```

3. Ensure that the service endpoints are available and ready:
   ```bash
   kubectl get endpoints -l app=<release-name>-amdgpu-metrics-exporter -n mynamespace
   ```
