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
monitor:
  resources:
    gpu: true      # Monitor GPU resources (enable for GPU nodes)
    nic: false     # Monitor NIC resources (enable for network metrics)
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

## Enabling DRA (Beta)

Dynamic Resource Allocation (DRA) GPU claim support is available starting with exporter v1.4.1 on Kubernetes 1.34+.
Pod association works natively with both the AMD Kubernetes device plugin (k8s-device-plugin) and the AMD GPU DRA driver (k8s-gpu-dra-driver) without any additional Helm configuration. The exporter first uses device plugin allocations and, if absent, automatically inspects DRA resource claims.

Checklist:

1. Cluster version: Kubernetes 1.34+.
2. DRA GPU driver deployed [AMD GPU DRA driver](https://github.com/ROCm/k8s-gpu-dra-driver).
3. Pods use resource claims referencing the AMD GPU driver.
