# Kubernetes (Helm) installation

This page explains how to install AMD Device Metrics Exporter using Kubernetes.

## System requirements

- ROCm 6.3.x
- Ubuntu 22.04 or later
- Kubernetes cluster v1.29.0 or later
- Helm v3.2.0 or later
- `kubectl` command-line tool configured with access to the cluster

## Installation

For Kubernetes environments, a Helm chart is provided for easy deployment.

- Prepare a `values.yaml` file:

```yaml
platform: k8s
nodeSelector: {} # Optional: Add custom nodeSelector
image:
  repository: docker.io/rocm/device-metrics-exporter
  tag: v1.3.1
  pullPolicy: Always
service:
  type: ClusterIP  # or NodePort
  ClusterIP:
    port: 5000
# ServiceMonitor configuration for Prometheus Operator integration
serviceMonitor:
  enabled: false
  interval: "30s"
  honorLabels: true
  honorTimestamps: true
  labels: {}
  relabelings: []
```

- Install using Helm:

```bash
# Install Helm
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh

# Install Helm Charts
helm repo add exporter https://rocm.github.io/device-metrics-exporter
helm repo update
helm install exporter \
  https://github.com/ROCm/device-metrics-exporter/releases/download/v1.3.1/device-metrics-exporter-charts-v1.3.1.tgz \
  -n mynamespace -f values.yaml --create-namespace
```
