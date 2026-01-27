# Kubernetes (Helm) installation

This page explains how to install AMD Device Metrics Exporter using Kubernetes.

## System requirements

- ROCm 6.2.0 or later
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
podAnnotations: {} # Optional: Add custom pod annotations
kubelet:
  podResourceAPISocketPath: /var/lib/kubelet/pod-resources
image:
  repository: docker.io/rocm/device-metrics-exporter
  tag: v1.4.1.1
  pullPolicy: Always
configMap: "" # Optional: Add custom configuration
# Resource requests and limits for the exporter pod
resources:
  limits:
    cpu: "2"
    memory: "4Gi"
  requests:
    cpu: "500m"
    memory: "512M"
service:
  type: ClusterIP  # or NodePort
  annotations: {} # Optional: Add custom service annotations
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
helm install exporter exporter/device-metrics-exporter-charts --version v1.4.1 --namespace kube-amd-gpu --create-namespace -f values.yaml
```

## Enabling DRA (Beta)

Dynamic Resource Allocation (DRA) GPU claim support is available starting with exporter v1.4.1 on Kubernetes 1.34+.
Pod association works natively with both the AMD Kubernetes device plugin (k8s-device-plugin) and the AMD GPU DRA driver (k8s-gpu-dra-driver) without any additional Helm configuration. The exporter first uses device plugin allocations and, if absent, automatically inspects DRA resource claims.

Checklist:
1. Cluster version: Kubernetes 1.34+.
2. DRA GPU driver deployed [AMD GPU DRA driver](https://github.com/ROCm/k8s-gpu-dra-driver).
3. Pods use resource claims referencing the AMD GPU driver.
