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
  https://github.com/ROCm/device-metrics-exporter/releases/download/v1.3.0/device-metrics-exporter-charts-v1.3.0.tgz \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.interval=15s \
  -n mynamespace --create-namespace
```

This will automatically create a ServiceMonitor resource that Prometheus Operator can discover and use to scrape metrics from the Device Metrics Exporter. The ServiceMonitor will be deployed in the same namespace as the metrics service and daemonset. Alternatively, define a `values.yaml` with the desired options and use it in helm install.

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

## Prometheus Configuration for ServiceMonitor Discovery

After deploying the ServiceMonitor, Prometheus must be configured to discover and scrape it. The Prometheus Operator uses selectors in the Prometheus Custom Resource (CR) to determine which ServiceMonitor objects to monitor. Two key selectors control this behavior:
1. **serviceMonitorNamespaceSelector**: Allows selecting namespaces to search for ServiceMonitor objects. An empty value selects all namespaces.
2. **serviceMonitorSelector**: Specifies which ServiceMonitor objects to select in the chosen namespaces based on labels.

### Example Prometheus Configuration

Here's an example Prometheus CR configuration that will discover the Device Metrics Exporter ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: prometheus
  namespace: monitoring
spec:
  # Select ServiceMonitor objects from specific namespaces
  serviceMonitorNamespaceSelector:
    matchLabels:
      # Select namespaces with this label, or use {} to select all namespaces
      name: mynamespace
  
  # Alternative: Select all namespaces
  # serviceMonitorNamespaceSelector: {}
  
  # Option 1: Use default labels (.Release.Name refers to the metrics exporter helm release name)
  serviceMonitorSelector:
    matchLabels:
      app: <release-name>-amdgpu-metrics-exporter

  # Option 2: Use custom labels (if you specified custom labels in Helm values)
  # serviceMonitorSelector:
  #   matchLabels:
  #     app.kubernetes.io/name: device-metrics-exporter
```

### ServiceMonitor Labels and Discovery

The Helm chart automatically adds default labels to the ServiceMonitor for Prometheus discovery:

- `app: <release-name>-amdgpu-metrics-exporter` (where `<release-name>` is your Helm release name)

#### Using Default Labels

You can rely on the default `app` label for Prometheus discovery without specifying custom labels:

```yaml
serviceMonitor:
  enabled: true
  interval: "15s"
  # No custom labels needed - defaults will be used
```

Your Prometheus CR should then use the `app` label to select the ServiceMonitor:

```yaml
serviceMonitorSelector:
  matchLabels:
    app: <release-name>-amdgpu-metrics-exporter
```

Replace `<release-name>` with the actual name you used when installing the Helm chart.

#### Using Custom Labels

If you need custom labels to match your specific Prometheus configuration, you can override the defaults:

```yaml
serviceMonitor:
  enabled: true
  interval: "15s"
  labels:
    # Custom labels that match your Prometheus serviceMonitorSelector
    app.kubernetes.io/name: device-metrics-exporter
    environment: production
    team: gpu-monitoring
```

### Verifying Prometheus Discovery

After configuring Prometheus, verify the integration by:

1. Accessing the Prometheus UI and navigating to the "Targets" page
2. Your Device Metrics Exporter should appear as a healthy target

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
