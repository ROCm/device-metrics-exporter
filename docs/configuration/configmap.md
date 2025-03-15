# Kubernetes configuration

When deploying AMD Device Metrics Exporter on Kubernetes, a `ConfigMap` is deployed in the exporter namespace.

## Configuration parameters

- `ServerPort`: this field is ignored when Device Metrics Exporter is deployed by the [GPU Operator](https://dcgpu.docs.amd.com/projects/gpu-operator/en/latest/) to avoid conflicts with the service node port config.
- `GPUConfig`:
  - Fields: An array of strings specifying what metrics field to be exported.
  - Labels: `GPU_ID`, `SERIAL_NUMBER`, `CARD_MODEL`, `HOSTNAME`, `GPU_PARTITION_ID` and `GPU_COMPUTE_PARTITION_TYPE` are always set and cannot be removed. Labels supported are available in the provided example `configmap.yml`.
  - HealthThresholds: These values dictates the threshold of the ECC field counters to mark a GPU as unhealthy. Default is 0 if not specified, the GPU will be marked unhealthy once the value goes above the set threshold limit.

## Setting custom values

To use a custom configuration when deploying the Metrics Exporter:

1. Create a `ConfigMap` based on the provided example [configmap.yml](https://github.com/ROCm/device-metrics-exporter/blob/main/example/configmap.yaml)
2. Change the `configMap` property in `values.yaml` to `configmap.yml`
3. Run `helm install`:

```bash
helm install exporter https://github.com/ROCm/device-metrics-exporter/releases/download/v1.2.0/device-metrics-exporter-charts-v1.2.0.tgz -n metrics-exporter -f values.yaml --create-namespace
```

Device Metrics Exporter polls for configuration changes every minute, so updates take effect without container restarts.
