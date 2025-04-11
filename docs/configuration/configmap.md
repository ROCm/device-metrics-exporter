# Kubernetes configuration

When deploying AMD Device Metrics Exporter on Kubernetes, a `ConfigMap` is deployed in the exporter namespace.

## Configuration parameters

- `ServerPort`: this field is ignored when Device Metrics Exporter is deployed by the [GPU Operator](https://dcgpu.docs.amd.com/projects/gpu-operator/en/latest/) to avoid conflicts with the service node port config.
- `GPUConfig`:
  - Fields: An array of strings specifying what metrics field to be exported.
  - Labels: `CARD_MODEL`, `GPU_UUID` and `SERIAL_NUMBER` are always set and cannot be removed. Labels supported are available in the provided example `configmap.yml`.

## Setting custom values

To use a custom configuration when deploying the Metrics Exporter:

1. Create a `ConfigMap` based on the provided example [configmap.yml](https://github.com/ROCm/device-metrics-exporter/blob/main/example/configmap.yaml)
2. Change the `configMap` property in `values.yaml` to `configmap.yml`
3. Run `helm install`:

```bash
helm repo add exporter https://rocm.github.io/device-metrics-exporter
helm repo update
helm install exporter exporter/device-metrics-exporter-charts --namespace kube-amd-gpu --create-namespace --version=v1.2.1 -f values.yaml
```

Device Metrics Exporter polls for configuration changes every minute, so updates take effect without container restarts.
