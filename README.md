# AMD Device Metrics Exporter

AMD Device Metrics Exporter Helm Chart Repository.

## Quick Start
```bash
# Install Helm
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh

# Install Helm Charts
helm repo add exporter https://rocm.github.io/device-metrics-exporter
helm repo update
helm install exporter exporter/device-metrics-exporter-charts --namespace kube-amd-gpu --create-namespace
```
