# Kubernetes configuration

When deploying AMD Device Metrics Exporter on Kubernetes, a `ConfigMap` is deployed in the exporter namespace.

## Configuration parameters

- `ServerPort`: this field is ignored when Device Metrics Exporter is deployed by the [GPU Operator](https://instinct.docs.amd.com/projects/gpu-operator/en/latest/) to avoid conflicts with the service node port config.
- `GPUConfig`:
  - `Fields`: An array of strings specifying what metrics field to be exported.
  - Labels: `SERIAL_NUMBER`, `GPU_ID`, `POD`, `NAMESPACE`, `CONTAINER`, `JOB_ID`, `JOB_USER`, `JOB_PARTITION`, `CARD_MODEL`, `HOSTNAME`, `GPU_PARTITION_ID`, `GPU_COMPUTE_PARTITION_TYPE`, `GPU_MEMORY_PARTITION_TYPE`, `KFD_PROCESS_ID` and `DEPLOYMENT_MODE` are always set and cannot be removed. Labels supported are available in the provided example `configmap.yml`.
  - CustomLabels: A map of user-defined labels and their values. Users can set up to 10 custom labels. From the `GPUMetricLabel` list, only `CLUSTER_NAME` is allowed to be set in `CustomLabels`. Any other labels from this list cannot be set. Users can define other custom labels outside of this restriction. These labels will be exported with every metric, ensuring consistent metadata across all metrics.
  - `ExtraPodLabels`: This defines a map that links Prometheus label names to Kubernetes pod labels. Each key is the Prometheus label that will be exposed in metrics, and the value is the pod label to pull the data from. This lets you expose pod metadata as Prometheus labels for easier filtering and querying.<br>(e.g. Considering an entry like `"WORKLOAD_ID"   : "amd-workload-id"`, where `WORKLOAD_ID` is a label visible in metrics and its value is the pod label value of a pod label key set as `amd-workload-id`).
  - `ProfilerMetrics`: A map of toggle to enable Profiler Metrics either for `all` nodes or a specific hostname with desired state. Key with specific hostname `$HOSTNAME` takes precedense over a `all` key. This only controls the Profiler Metrics which has prefix of `GPU_PROF_` from the metrics list.
- `CommonConfig`:
  - `MetricsFieldPrefix`: Add prefix string for all the fields exporter. [Premetheus Metric Label formatted](https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels) string prefix will be accepted, on any invalid prefix will default to empty prefix to allow exporting of the fields.
  - `HealthService` : Health Service configurations for the exporter.
    - `Enable` : false to disable, otherwise enabled by default
    - `PollingRate`: Health polling rate in duration format. Default is `30s` (30 seconds). Minimum allowed value is `30s` (30 seconds), maximum is `24h` (1 day). Supports duration formats like `30s`, `1h`, `1d`, or `23h10m15s`. Values below minimum default to 30s, values above maximum default to 24h.
  - `LoggerConfig`: Logger configurations for the exporter.
    - `Level`: Log level for the exporter. Supported levels are `DEBUG`, `INFO`, `WARN`, `ERROR`. Default is `INFO`.
    - `MaxSizeMB`: Maximum size in megabytes of the log file before it gets rotated. Default is `10` MB.
    - `MaxBackups`: Maximum number of old log files to retain. Default is `3`.
    - `MaxAgeDays`: Maximum number of days to retain old log files. Default is `7` days.
    - `LogRotationDisable`: Boolean flag to disable log rotation. If set to `true`, log rotation is disabled and logs will be written to a single file without rotation. Default is `false`.
  - `ProfilerConfig`: Configuration for Profiler metrics.
    - `SamplingInterval`: Specifies the duration, in microseconds, of the sampling window used by the profiler to collect metrics for each query request. The default value is 1000 µs (1 millisecond), which is also the minimum allowed value. Excessively high values may result in delayed or timeout errors during metric collection.
    - `PtlDelay` : Delay in milliseconds to wait after setting PTL states before collecting metrics. Default is `0` ms no delay. This setting is useful for platform supporting Peaks Top Limiter (PTL) mode to ensure that the PTL states are properly applied before metrics collection begins.
- `NICConfig`:
  - `Fields`: An array of strings specifying what metrics field to be exported. Detailed list of fields can be found [here](metricslist.md)
  - `Labels`: `NIC_SERIAL_NUMBER`, `NIC_UUID`, `NIC_HOSTNAME` are always set and cannot be removed. Workload related labels such as `NIC_POD`, `NIC_NAMESPACE`, and `NIC_CONTAINER` are dynamically added to the LIF when there is an associated workload.  Labels supported are available in the provided example `configmap.yml`.
  - `CustomLabels`: A map of user-defined labels and their values. Users can set up to 10 custom labels. `CLUSTER_NAME` is the only label that is exported by default. Users can define other custom labels outside of this restriction. These labels will be exported with every metric, ensuring consistent metadata across all metrics.
  - `HealthCheckConfig`: List of the configs that determine the health check behavior for NICs. This includes settings such as whether interfaces that are down should be reported as unhealthy (`InterfaceAdminDownAsUnhealthy`). These configurations help define how NIC health metrics are evaluated and exported.
- `IFOEConfig`:
  - `Fields`: An array of strings specifying what IFOE metrics fields to be exported. Detailed list of fields can be found [here](ifoe-metricslist.md). If no fields are specified, all IFOE metrics are exported by default.
  - `Labels`: `HOSTNAME` and `IFOE_UUID` are mandatory labels that are always set and cannot be removed. Additional optional labels include `IFOE_STATION_UUID`, `IFOE_PORT_NAME`, and `IFOE_DEVICE_UUID` which provide more granular identification of IFOE components. Labels supported are available in the provided example `configmap.yml`.
  - `CustomLabels`: A map of user-defined labels and their values. Users can set up to 10 custom labels. These labels will be exported with every IFOE metric, ensuring consistent metadata across all metrics. Custom labels allow you to add deployment-specific information such as cluster identifiers, data center locations, or other organizational metadata.
  - `ExtraPodLabels`: Similar to GPUConfig, this defines a map that links Prometheus label names to Kubernetes pod labels for IFOE metrics. This allows you to expose pod metadata as Prometheus labels for easier correlation between IFOE network metrics and workload information.

## Setting custom values

To use a custom configuration when deploying the Metrics Exporter:

1. Create a `ConfigMap` based on the provided example [configmap.yml](../examples/configmap.yml) file.
2. Change the `configMap` property in `values.yaml` to `configmap.yml`
3. Run `helm install`:

```bash
helm install exporter https://github.com/ROCm/device-metrics-exporter/releases/download/v1.5.0/device-metrics-exporter-charts-v1.5.0.tgz -n metrics-exporter -f values.yaml --create-namespace
```

Device Metrics Exporter polls for configuration changes every minute, so updates take effect without container restarts.
