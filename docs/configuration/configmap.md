# Kubernetes configuration

When deploying AMD Device Metrics Exporter on Kubernetes, a `ConfigMap` is deployed in the exporter namespace.

## Configuration parameters

- `ServerPort`: this field is ignored when Device Metrics Exporter is deployed by the [GPU Operator](https://instinct.docs.amd.com/projects/gpu-operator/en/latest/) or the [Network Operator](https://instinct.docs.amd.com/projects/network-operator/en/latest/) to avoid conflicts with the service port config. Use `.spec.metricsExporter.port` in the operator CR instead.
- `GPUConfig`:
  - `Fields`: An array of strings specifying which metric fields are exported. The exporter derives a request filter from the enabled `Fields` (and `Labels`) so that `gpuagent` skips the server-side collectors no enabled metric needs (for example the KFD process walk or the PCIe throughput sampler), reducing per-poll cost on large or many-GPU nodes. This is automatic and requires no extra setting: disabling a field group (clock, XGMI, VRAM usage, violation, PCIe stats, activity, process) stops it from being collected; enabling any field in a group restores it. ECC and PCIe-status collectors are always fetched because the health service depends on them. The filter re-derives on the standard config auto-reload, so changes take effect without a restart. An empty `Fields` (the default) exports everything and skips nothing.
  - `Labels`: `SERIAL_NUMBER`, `GPU_ID`, `POD`, `NAMESPACE`, `CONTAINER`, `JOB_ID`, `JOB_USER`, `JOB_PARTITION`, `JOB_ACCOUNT`, `CARD_MODEL`, `HOSTNAME`, `GPU_PARTITION_ID`, `GPU_COMPUTE_PARTITION_TYPE`, `GPU_MEMORY_PARTITION_TYPE` and `DEPLOYMENT_MODE` are always set and cannot be removed. The `POD_UUID` label is fetched from the Kubernetes API server and provides the unique identifier (UID) of the pod. Optional labels such as `KFD_PROCESS_ID` (process IDs using the GPU) and `GPU_UUID` can be enabled by adding them to the Labels array. The optional `PRODUCT_NAME` label (disabled by default) reports the GPU product, for example `AMD_Instinct_MI350P`, so metrics can be filtered by product in clusters with mixed GPUs. It matches the `amd.com/gpu.product-name` label set by the Kubernetes node labeller: the board product name, or the device ID based market name reported by AMD SMI when the board product name is blank, with spaces and `/` changed to `_` and parentheses dropped. Unlike the node labeller, repeated `_` are collapsed to one. Partitioned GPUs report the product name of their physical GPU. Labels supported are available in the provided example `configmap.yaml`.
  - `CustomLabels`: A map of user-defined labels and their values. Users can set up to 10 custom labels. From the `GPUMetricLabel` list, only `CLUSTER_NAME` is allowed to be set in `CustomLabels`. Any other labels from this list cannot be set. Users can define other custom labels outside of this restriction. These labels will be exported with every metric, ensuring consistent metadata across all metrics.
  - `ExtraPodLabels`: This defines a map that links Prometheus label names to Kubernetes pod labels. Each key is the Prometheus label that will be exposed in metrics, and the value is the pod label to pull the data from. This lets you expose pod metadata as Prometheus labels for easier filtering and querying. For example, with the entry `"WORKLOAD_ID": "amd-workload-id"`, metrics carry a `WORKLOAD_ID` label whose value is the value of the pod's `amd-workload-id` label.
  - `HealthThresholds`: Map of GPU health check thresholds used by the exporter health service.
    - ECC fields (`GPU_ECC_UNCORRECT_*`): Unsigned integer counters. A GPU is marked unhealthy when the corresponding ECC metric exceeds the configured threshold.
    - `GPU_CPER_MAX_AGE`: Go duration string (for example `"1h"`, `"30m"`). Maximum age of the latest fatal CPER record that can mark a GPU unhealthy. Empty or unset preserves legacy behavior (any latest fatal CPER marks the GPU unhealthy). Set an explicit duration to ignore older fatal CPER records. Set to `"0"` to explicitly disable the age filter (same as empty).
  - `ProfilerMetrics`: A map of toggle to enable Profiler Metrics either for `all` nodes or a specific hostname with desired state. A key with a specific hostname (`$HOSTNAME`) takes precedence over the `all` key. This setting only controls the profiler metrics, which have the `GPU_PROF_` prefix in the metrics list.
- `CommonConfig`:
  - `MetricsFieldPrefix`: A prefix added to the names of all exported fields. The prefix must be valid in a [Prometheus metric name](https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels). An invalid prefix is replaced with an empty prefix so that the fields are still exported.
  - `HealthService`: Health service configuration for the exporter.
    - `Enable`: Set to `false` to disable the health service. Enabled by default.
    - `PollingRate`: Health polling rate in duration format. Default is `30s` (30 seconds). Minimum allowed value is `30s` (30 seconds), maximum is `24h` (1 day). Supports duration formats like `30s`, `1h`, `1d`, or `23h10m15s`. Values below minimum default to 30s, values above maximum default to 24h.
  - `Logging`: Logger configurations for the exporter.
    - `Level`: Log level for the exporter. Supported levels are `DEBUG`, `INFO`, `WARN`, `ERROR`. Default is `INFO`.
    - `MaxSizeMB`: Maximum size in megabytes of the log file before it gets rotated. Default is `10` MB.
    - `MaxBackups`: Maximum number of old log files to retain. Default is `3`.
    - `MaxAgeDays`: Maximum number of days to retain old log files. Default is `7` days.
    - `LogRotationDisable`: Boolean flag to disable log rotation. If set to `true`, log rotation is disabled and logs will be written to a single file without rotation. Default is `false`.
  - `ProfilerConfig`: Configuration for Profiler metrics.
    - `SamplingInterval`: Specifies the duration, in microseconds, of the sampling window used by the profiler to collect metrics for each query request. The default value is 1000 µs (1 millisecond), which is also the minimum allowed value. Excessively high values may result in delayed or timeout errors during metric collection.
    - `PtlDelay`: Delay in milliseconds to wait after setting PTL states before collecting metrics. Default is `0` (no delay). This setting is useful for platforms that support Peaks Top Limiter (PTL) mode to ensure that the PTL states are properly applied before metrics collection begins.
  - `Debug`: Configuration for optional debug interfaces.
    - `EnableAPI`: Boolean flag to expose the debug interfaces (the pprof/expvar profiling endpoints on the metrics port and the error-injection gRPC API). Default is `false`, meaning these interfaces are disabled. The value is read at runtime and applied on config reload.
  - `HighResolution`: High-resolution sampling profile for GPU and [workload cgroup](workload-cgroup-metrics.md) metrics. When enabled, these metrics are refreshed faster than the `/metrics` scrape interval, and `/metrics` always serves the most recent sample. When disabled (the default), metrics are refreshed once per scrape.
    - `Enable`: Boolean flag to enable the high-resolution profile. Default is `false`.
    - `FastPeriodMs` (preview): Fast sampling period in milliseconds: `1`, `10`, `100`, or a multiple of `1000` up to `60000`. Overrides `FastPeriodSecs` when set. Periods below 1 s apply to power metrics only; other metrics refresh at most once per second.
    - `FastPeriodSecs`: Sampling period, in seconds, for fast-moving attributes. Valid range `1`–`60`. `0` selects the default (`1`).
    - `SlowPeriodSecs`: Sampling period, in seconds, for slow-moving attributes (ECC/XGMI counters, PCIe bandwidth, memory usage, clocks). Must be ≥ the fast period. Valid range `5`–`3600`. `0` selects the default (`5`).

    Sub-second periods are opt-in. `10` ms and `1` ms are for short diagnostic runs, and `1` ms is experimental; below 100 ms the exporter logs a warning and, on Kubernetes, emits a `HighResDiagnosticTier` event. An invalid or unsupported period falls back to 1 s. Set the CPU and memory in [High-resolution resource guidelines](#high-resolution-resource-guidelines) for the period you use.

    While high-resolution mode is enabled, the following are not available: in-band-RAS/CPER-based health checks (basic ECC-based health is still reported), the `/gpumetrics` live query (a cached snapshot is served instead), the `KFD_PROCESS_ID` label, and the `gpu_afid_errors` metric.

    **Example** (`config.json`):

    ```json
    {
      "CommonConfig": {
        "HighResolution": {
          "Enable": true,
          "FastPeriodSecs": 1,
          "SlowPeriodSecs": 5
        }
      }
    }
    ```

    **Example** (`config.json`, 100 ms power metrics):

    ```json
    {
      "CommonConfig": {
        "HighResolution": {
          "Enable": true,
          "FastPeriodMs": 100
        }
      }
    }
    ```

    See [High-Resolution Sampling Tiers](metricslist.md) for which metrics land in the fast and slow tiers.

- `NICConfig`:
  - `Fields`: An array of strings specifying which metric fields are exported. For the list of fields, see the [AINIC metrics list](network-metricslist.md).
  - `Labels`: `NIC_SERIAL_NUMBER`, `NIC_UUID`, `NIC_HOSTNAME` are always set and cannot be removed. Workload related labels such as `NIC_POD`, `NIC_NAMESPACE`, and `NIC_CONTAINER` are dynamically added to the LIF when there is an associated workload. The `POD_UUID` label is fetched from the Kubernetes API server and provides the unique identifier (UID) of the pod. Labels supported are available in the provided example `configmap.yaml`.
  - `CustomLabels`: A map of user-defined labels and their values. Users can set up to 10 custom labels. `CLUSTER_NAME` is the only label that is exported by default. Users can define other custom labels outside of this restriction. These labels will be exported with every metric, ensuring consistent metadata across all metrics.
  - `HealthCheckConfig`: List of the configs that determine the health check behavior for NICs. This includes settings such as whether interfaces that are down should be reported as unhealthy (`InterfaceAdminDownAsUnhealthy`). These configurations help define how NIC health metrics are evaluated and exported.
- `IFOEConfig`:
  - `Fields`: An array of strings specifying what IFOE metrics fields to be exported. If no fields are specified, all IFOE metrics are exported by default.
  - `Labels`: `HOSTNAME`, `GPU_UUID`, `IFOE_STATION_UUID`, `IFOE_PORT_NAME` are mandatory labels that are always set and cannot be removed. These labels provide identification of IFOE components at the host, GPU, station, port, and device levels. Labels supported are available in the provided example `configmap.yaml`.
  - `CustomLabels`: A map of user-defined labels and their values. Users can set up to 10 custom labels. These labels will be exported with every IFOE metric, ensuring consistent metadata across all metrics. Custom labels allow you to add deployment-specific information such as cluster identifiers, data center locations, or other organizational metadata.
  - `ExtraPodLabels`: Similar to GPUConfig, this defines a map that links Prometheus label names to Kubernetes pod labels for IFOE metrics. This allows you to expose pod metadata as Prometheus labels for easier correlation between IFOE network metrics and workload information.

## High-resolution resource guidelines

Stable performance at each fast period requires at least this CPU and memory for the exporter and gpuagent together, on a node with up to 8 GPUs:

| Fast period | CPU | Memory | Memory with profiler metrics |
| --- | --- | --- | --- |
| 1 s (default) | 0.5 | 256Mi | 1.5Gi |
| 100 ms | 1 | 256Mi | 1.5Gi |
| 10 ms | 3 | 256Mi | 1.5Gi |
| 1 ms | 4 | 256Mi | 1.5Gi |

- **Kubernetes (Helm):** set the top-level `resources` value, for example for 10 ms: `--set resources.limits.cpu=3 --set resources.limits.memory=256Mi`.
- **Debian package:** `gpuagent.service` and `amd-metrics-exporter.service` limit CPU with `CPUQuota`. Raise it with `sudo systemctl edit <service>` (for example `[Service]` and `CPUQuota=300%` for 3 CPUs), then `sudo systemctl restart <service>`.

## CLI flags

The following flags can be passed to the exporter binary via the `args` field in `values.yaml`:

- `--exit-on-rocpctl-error`: Exit DME when `rocpctl` is auto-disabled after 3 consecutive failures or a crash (default: `false`). When enabled, DME exits and relies on the pod restart policy to recover. This is useful in Kubernetes environments where a `CrashLoopBackOff` and the associated K8s Warning event are preferred over a silently degraded metrics stream.

  **When to enable:** Use this flag if profiler metrics (`gpu_prof_*`) are critical to your monitoring and you want the pod to restart automatically when `rocpctl` becomes unresponsive or crashes.

  **Operator guidance:**
  - If `rocpctl` failures are persistent (for example, due to a ROCm driver issue), the pod will enter `CrashLoopBackOff`. In that case, disable profiler metrics via the ConfigMap (`"ProfilerMetrics": {"all": false}`) before restarting the pod, then investigate the root cause.
  - This flag has no effect when `ProfilerMetrics` is disabled.

  **Helm example** (`values.yaml`):

  ```yaml
  args:
    - "--exit-on-rocpctl-error=true"
  ```

- `--exit-on-agent-down`: Exit DME when the `gpuagent` process is unreachable after 3 consecutive health-poll failures (default: `false`).

## Setting custom values

To use a custom configuration when deploying the Metrics Exporter:

1. Create a `ConfigMap` based on the example [configmap.yaml](https://github.com/ROCm/device-metrics-exporter/blob/main/example/configmap.yaml). Set `metadata.namespace` to the namespace you install the exporter in (`metrics-exporter` in this example), then create the namespace and apply the file:

   ```bash
   kubectl create namespace metrics-exporter
   kubectl apply -f configmap.yaml
   ```

2. In `values.yaml`, set `configMap` to the name of the `ConfigMap` (`gpu-config` in the example).
3. Install the chart with your `values.yaml`:

   ```bash
   helm repo add exporter https://rocm.github.io/device-metrics-exporter
   helm repo update
   helm install exporter exporter/device-metrics-exporter-charts \
     --version v1.5.5 \
     -n metrics-exporter \
     -f values.yaml
   ```

When you update the `ConfigMap`, the exporter reloads the configuration after the kubelet updates the mounted file, so changes take effect without a pod restart.
