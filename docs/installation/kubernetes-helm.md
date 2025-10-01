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
tolerations: []  # Optional: Add custom tolerations
kubelet:
  podResourceAPISocketPath: /var/lib/kubelet/pod-resources
image:
  repository: docker.io/rocm/device-metrics-exporter
  tag: v1.5.0
  pullPolicy: Always
configMap: "" # Optional: Add custom configuration
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
helm repo add exporter https://rocm.github.io/device-metrics-exporter
helm repo update
helm install exporter exporter/device-metrics-exporter-charts --namespace kube-amd-gpu --create-namespace -f values.yaml
```
