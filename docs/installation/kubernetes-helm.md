# Kubernetes (Helm) installation

This page explains how to install AMD Device Metrics Exporter using Kubernetes.

## System requirements

- ROCm 6.3.x
- Ubuntu 22.04 or later
- Kubernetes cluster v1.29.0 or later
- Helm v3.2.0 or later
- `kubectl` command-line tool configured with access to the cluster

**Note:** GPU and NIC monitoring cannot be enabled simultaneously in a single Helm deployment.
Choose the appropriate configuration based on your monitoring needs:

- **GPU Monitoring**: For monitoring AMD GPUs (requires ROCm 6.2.0+)
- **NIC Monitoring**: For monitoring AMD NICs (requires compatible AMD NIC hardware)

## Installation

For Kubernetes environments, a Helm chart is provided for easy deployment. The chart comes with default values pre-configured - you only need to customize them if your environment requires specific settings.

### Install Helm (if not installed)

```bash
# Install Helm
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh
```

### Deploy the exporter using Helm

::::{tab-set}

:::{tab-item} GPU

```bash
# Install Helm Charts for GPU monitoring
helm repo add exporter https://rocm.github.io/device-metrics-exporter
helm repo update
helm install exporter exporter/device-metrics-exporter-charts \
  --version v1.4.1 \
  --namespace kube-amd-gpu \
  --create-namespace
```

#### Verifying the GPU installation

After installation, verify that the exporter pods are running:

```bash
kubectl get pods -l app.kubernetes.io/name=metrics-exporter -n kube-amd-gpu  # For GPU monitoring
```

#### Customizing the GPU installation

You can customize the installation using one of two methods:

##### Option 1: Using `--set` flags

Override individual values on the command line using `--set key=value`:

###### Example: Enable Prometheus monitoring and adjust resource limits

```bash
helm install exporter exporter/device-metrics-exporter-charts \
  --version v1.4.1 \
  --namespace kube-amd-gpu \
  --create-namespace \
  --set serviceMonitor.enabled=true \
  --set resources.limits.cpu=4 \
  --set resources.limits.memory=8Gi
```

###### Example: Add custom tolerations for tainted nodes

```bash
helm install exporter exporter/device-metrics-exporter-charts \
  --version v1.4.1 \
  --namespace kube-amd-gpu \
  --create-namespace \
  --set tolerations[0].key=example.com/foo \
  --set tolerations[0].operator=Exists \
  --set tolerations[0].effect=NoSchedule
```

##### Option 2: Using a custom `values.yaml` file

For more extensive customization, download and modify the default values.yaml:

```bash
# Download the default values.yaml for GPU monitoring
helm show values exporter/device-metrics-exporter-charts --version v1.4.1 > gpu-values.yaml
```

Edit `gpu-values.yaml` as needed (example below), then install using the custom file:

```yaml
tolerations:
  - key: example.com/foo
    operator: Exists
    effect: NoSchedule
```

```bash
helm install exporter exporter/device-metrics-exporter-charts \
  --version v1.4.1 \
  --namespace kube-amd-gpu \
  --create-namespace \
  -f gpu-values.yaml
```

#### Debugging GPU installation

Use the `--debug` flag with helm install or helm template to see the fully rendered Kubernetes manifests with all values applied. This is useful for verifying that overrides are taking effect before deploying:

```bash
helm template exporter exporter/device-metrics-exporter-charts \
  --debug \
  -f gpu-values.yaml
```

Use `--dry-run` with helm install to simulate the installation without applying any resources to the cluster. Unlike helm template, a dry run communicates with the Kubernetes API server to validate the manifests against the cluster:

```bash
helm install exporter exporter/device-metrics-exporter-charts \
  --version v1.4.1 \
  --namespace kube-amd-gpu \
  --create-namespace \
  --dry-run \
  -f gpu-values.yaml
```

:::

:::{tab-item} NIC

```bash
# Install Helm Charts for NIC monitoring
helm repo add exporter https://rocm.github.io/device-metrics-exporter
helm repo update
helm install exporter exporter/nic-device-metrics-exporter-charts \
  --version v1.2.0 \
  --namespace kube-amd-network \
  --create-namespace
```

#### Verifying the NIC installation

After installation, verify that the exporter pods are running:

```bash
kubectl get pods -l app.kubernetes.io/name=metrics-exporter -n kube-amd-network  # For NIC monitoring
```

#### Customizing the NIC installation

You can customize the installation using one of two methods:

**Note:** Ensure the Metrics Exporter image `image.tag` matches the AINIC firmware version installed on your nodes. Refer to the [Compatibility Matrix](../index.md) for the correct image version to use and configure the correct `image.tag` value in the values file.

##### Option 1: Using `--set` flags

Override individual values on the command line using `--set key=value`:

###### Example: Enable Prometheus monitoring and adjust resource limits

```bash
helm install exporter exporter/nic-device-metrics-exporter-charts \
  --version v1.2.0 \
  --namespace kube-amd-network \
  --create-namespace \
  --set serviceMonitor.enabled=true \
  --set resources.limits.cpu=4 \
  --set resources.limits.memory=8Gi
```

###### Example: Add custom tolerations for tainted nodes

```bash
helm install exporter exporter/nic-device-metrics-exporter-charts \
  --version v1.2.0 \
  --namespace kube-amd-network \
  --create-namespace \
  --set tolerations[0].key=example.com/foo \
  --set tolerations[0].operator=Exists \
  --set tolerations[0].effect=NoSchedule
```

##### Option 2: Using a custom `values.yaml` file

For more extensive customization, download and modify the default values.yaml:

```bash
# Download the default values.yaml for NIC monitoring
helm show values exporter/nic-device-metrics-exporter-charts --version v1.2.0 > nic-values.yaml
```

Edit `nic-values.yaml` as needed (example below), then install using the custom file:

```yaml
tolerations:
  - key: example.com/foo
    operator: Exists
    effect: NoSchedule
```

```bash
helm install exporter exporter/nic-device-metrics-exporter-charts \
  --version v1.2.0 \
  --namespace kube-amd-network \
  --create-namespace \
  -f nic-values.yaml
```

#### Debugging NIC installation

Use the `--debug` flag with helm install or helm template to see the fully rendered Kubernetes manifests with all values applied. This is useful for verifying that overrides are taking effect before deploying:

```bash
helm template exporter exporter/nic-device-metrics-exporter-charts \
  --debug \
  -f nic-values.yaml
```

Use `--dry-run` with helm install to simulate the installation without applying any resources to the cluster. Unlike helm template, a dry run communicates with the Kubernetes API server to validate the manifests against the cluster:

```bash
helm install exporter exporter/nic-device-metrics-exporter-charts \
  --version v1.2.0 \
  --namespace kube-amd-network \
  --create-namespace \
  --dry-run \
  -f nic-values.yaml
```

:::

::::

## Testing the installation

Verify that the exporter is running and exposing metrics:

::::{tab-set}

:::{tab-item} GPU

```bash
curl http://<exporter-service-ip>:5000/metrics
```

:::

:::{tab-item} NIC

```bash
curl http://<exporter-service-ip>:5001/metrics
```

:::

::::

## Uninstallation

To uninstall the exporter, run:

::::{tab-set}

:::{tab-item} GPU

```bash
helm uninstall exporter -n kube-amd-gpu # For GPU monitoring
```

:::

:::{tab-item} NIC

```bash
helm uninstall exporter -n kube-amd-network  # For NIC monitoring
```

:::

::::

## Enabling DRA (Beta) for GPU monitoring

Dynamic Resource Allocation (DRA) GPU claim support is available starting with exporter v1.4.1 on Kubernetes 1.34+.
Pod association works natively with both the AMD Kubernetes device plugin (k8s-device-plugin) and the AMD GPU DRA driver (k8s-gpu-dra-driver) without any additional Helm configuration. The exporter first uses device plugin allocations and, if absent, automatically inspects DRA resource claims.

Checklist:

1. Cluster version: Kubernetes 1.34+.
2. DRA GPU driver deployed [AMD GPU DRA driver](https://github.com/ROCm/k8s-gpu-dra-driver).
3. Pods use resource claims referencing the AMD GPU driver.
