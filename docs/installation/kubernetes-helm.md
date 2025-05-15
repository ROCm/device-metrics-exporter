# Kubernetes (Helm) installation

This page explains how to install AMD Device Metrics Exporter using Kubernetes.

## System requirements

- ROCm 6.2.0
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
  tag: v1.3.0
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
helm install exporter \
  https://github.com/ROCm/device-metrics-exporter/releases/download/v1.2.1/device-metrics-exporter-charts-v1.2.1.tgz \
  -n mynamespace -f values.yaml --create-namespace
```
