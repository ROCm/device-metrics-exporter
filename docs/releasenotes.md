# Release Notes

## v1.5.0

- **Profiler Configuration Enhancement**
  - `SamplingInterval` to set sampling window for the profiler metrics
  - Default value is 1000 microseconds (1 millisecond)

- **Configurable Health Polling Rate**
  - Added `PollingRate` field to `HealthService` configuration in `CommonConfig`
  - Supports duration formats: 30s, 5m, 1h, 1d, 23h10m15s
  - Default: 30 seconds, Min: 30 seconds, Max: 24 hours

- **Kubevirt**
  - Exporter now supports Kubevirt deployments
    - New exporter with SR-IOV support for hypervisor environments is now available
      - Legacy exporter remains applicable for existing deployments:
        1. Baremetal passthrough
        2. Guest VM

- **Slinky**
  - Slinky job reporting is now supported, with labels providing both Kubernetes and Slurm job information

### Platform Support

ROCm 7.0 MI2xx, MI3xx

## v1.4.2

- **New Label**
  - `KFD_PROCESS_ID` label will  now report the process ID using the
    respective GPU. This enables baremetal debian deployments to have job
    information where no scheduler is used.
  - `DEPLOYMENT_MODE` label to specify the GPU operating environment
  
- **New Field**
  - `GPU_AFID_ERRORS` field added to report RAS events associated AMD Field Identifier (AFID) list, More details can be found [here](configuration/metricslist.md#afid-error-metrics)
    - More Info on AMD Field ID and next steps are https://docs.amd.com/r/en-US/AMD_Field_ID_70122_v1.0/AFID-Event-List
  - Violation Metrics 1.8 version fields
    - Current percentage and Per Compute Core violation metrics available for
      - `GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_PERCENTAGE`
      - `GPU_VIOLATION_PPT_RESIDENCY_PERCENTAGE`
      - `GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_PERCENTAGE`
      - `GPU_VIOLATION_VR_THERMAL_RESIDENCY_PERCENTAGE`
      - `GPU_VIOLATION_HBM_THERMAL_RESIDENCY_PERCENTAGE`
      - `GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_POWER_ACCUMULATED`
      - `GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_THERMAL_ACCUMULATED`
      - `GPU_VIOLATION_LOW_UTILIZATION_ACCUMULATED`
      - `GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_TOTAL_ACCUMULATED`
      - `GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_POWER_PERCENTAGE`
      - `GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_THERMAL_PERCENTAGE`
      - `GPU_VIOLATION_LOW_UTILIZATION_PERCENTAGE`
      - `GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_TOTAL_PERCENTAGE`

  - Clock Metrics Added `GPU_MIN_CLOCK`, `GPU_MAX_CLOCK`

- **Label Value Change**
  - Clock type was not normalized in previous releases, now clock type label values
      are normalized without prefixes of `GPU_CLOCK_TYPE_`. More details can be found
      [here](./configuration/metricslist.md#clock-metrics)

### Platform Support

ROCm X.X.X MI2xx, MI3xx

## v1.4.1.2

### Issues Fixed

- **Security Vulnerability Fix**
  - `gpuagent.service` internal service port was hosted on all interfaces instead of only localhost
  - internal service is now only hosted on localcalhost and can be configurable through service files, more details (here)[installation/deb-package.rst]

- **Workaround for prior version**
  - Add firewall rules in front of the service to block inbound  connections, allowing only traffic originating from localhost (127.0.0.1).

## nic-v1.0.0

- **NIC Metrics Exporter for Prometheus**
  - Real-time metrics exporter for AMD NICs.
  - Supports both Docker and Debian installations.
  - Collects metrics using nicctl, rdma, and ethtool, and works across hypervisor, VM, and bare-metal environments.
  - Optimized RDMA stats, reducing the previously observed latency by multiple folds compared to the beta release.

## v1.4.1.2

### Issues Fixed

- **Security Vulnerability Fix**
  - `gpuagent.service` internal service port was hosted on all interfaces instead of only localhost
  - internal service is now only hosted on localhost and can be configurable through service files, more details [here](installation/deb-package.rst)

- **Workaround for prior version**
  - Add firewall rules in front of the service to block inbound  connections, allowing only traffic originating from localhost (127.0.0.1).

## v1.4.0.1

### Issues Fixed

- **Security Vulnerability Fix**
  - `gpuagent.service` internal service port was hosted on all interfaces instead of only localhost
  - internal service is now only hosted on localhost and can be configurable through service files, more details [here](installation/deb-package.rst)

- **Workaround for prior version**
  - Add firewall rules in front of the service to block inbound  connections, allowing only traffic originating from localhost (127.0.0.1).

## v1.4.1

- **Configurable Resource Limits**
  - Exporter Pod resource limits can now be configured through the Helm chart
  - Default limits are set if not specified in the Helm chart
  - Debian systemd service file is now set with default resource limits
  - Beta: DRA (Dynamic Resource Allocation) device support — exporter can detect and report DRA-allocated GPU devices from the device plugin/pod resources.

- **Profiler Failure Handling**
  - **Profiler Failure Handling**
    - Profiler is hardware sensitive for failures. To better handle potential system disruption from coredumps or profiler failures, the exporter will automatically disable profiler metrics to maintain critical exporter functionality and server stability:
      - **Coredump**: Profiler will be disabled immediately upon detection
      - **Non-crash errors**: If the profiler encounters 3 consecutive non-crash errors, it will be automatically disabled
    - **Re-enabling**: Profiler can be re-enabled by restarting/deleting the exporter pod on that node/server

- **_Note_** Profiler metrics are prefixed by `gpu_prof_` in [metrics list](./configuration/metricslist.md)

### Platform Support

ROCm 7.1.1 MI2xx, MI3xx; Kubernetes 1.34+ (required only for DRA beta feature)

## v1.4.0.1

### Issues Fixed

- **Security Vulnerability Fix**
  - `gpuagent.service` internal service port was hosted on all interfaces instead of only localhost
  - internal service is now only hosted on localcalhost and can be configurable through service files, more details (here)[installation/deb-package.rst]

## v1.4.0

- **MI35x Platform Support**
  - Exporter now supports MI35x platform with parity with latest supported
    fields.

- **Mask Unsupported Fields**
  - Platform-specific unsupported fields (amd-smi marked as N/A) will not be exported.
    Boot logs will indicate which fields are supported by the platform (logged once during startup).

- **New Profiler Fields**
  - New fields are added for better understanding of the application

- **Deprecated Fields Notice**
  - Following fields are deprecated from 6.14.14 driver onwards
    - GPU_MMA_ACTIVITY
    - GPU_JPEG_ACTIVITY
    - GPU_VCN_ACTIVITY

  - These fields are replaced by following fields
    - GPU_VCN_BUSY_INSTANTANEOUS
    - GPU_JPEG_BUSY_INSTANTANEOUS

### Platform Support

ROCm 7.0 MI2xx, MI3xx

### Issues Fixed

- fixed metric naming discrepancies between config field and exported field. The
  following prometheus fields name are being changed.
- one config field is being renamed which would require updating the
  config.json from the released branch for `pcie_nac_received_count` ->
  `pcie_nack_received_count`

  | S.No | Old Field Name                                  | New Field Name                                     |
  |------|-------------------------------------------------|----------------------------------------------------|
  | 1    | xgmi_neighbor_0_nop_tx                          | gpu_xgmi_nbr_0_nop_tx                              |
  | 2    | xgmi_neighbor_1_nop_tx                          | gpu_xgmi_nbr_1_nop_tx                              |
  | 3    | xgmi_neighbor_0_request_tx                      | gpu_xgmi_nbr_0_req_tx                              |
  | 4    | xgmi_neighbor_0_response_tx                     | gpu_xgmi_nbr_0_resp_tx                             |
  | 5    | xgmi_neighbor_1_response_tx                     | gpu_xgmi_nbr_1_resp_tx                             |
  | 6    | xgmi_neighbor_0_beats_tx                        | gpu_xgmi_nbr_0_beats_tx                            |
  | 7    | xgmi_neighbor_1_beats_tx                        | gpu_xgmi_nbr_1_beats_tx                            |
  | 8    | xgmi_neighbor_0_tx_throughput                   | gpu_xgmi_nbr_0_tx_thrput                           |
  | 9    | xgmi_neighbor_1_tx_throughput                   | gpu_xgmi_nbr_1_tx_thrput                           |
  | 10   | xgmi_neighbor_2_tx_throughput                   | gpu_xgmi_nbr_2_tx_thrput                           |
  | 11   | xgmi_neighbor_3_tx_throughput                   | gpu_xgmi_nbr_3_tx_thrput                           |
  | 12   | xgmi_neighbor_4_tx_throughput                   | gpu_xgmi_nbr_4_tx_thrput                           |
  | 13   | xgmi_neighbor_5_tx_throughput                   | gpu_xgmi_nbr_5_tx_thrput                           |
  | 14   | gpu_violation_vr_thermal_tracking_accumulated   | gpu_violation_vr_thermal_residency_accumulated     |
  | 15   | pcie_nac_received_count                         | pcie_nack_received_count                           |
  | 16   | gpu_violation_proc_hot_residency_accumulated    | gpu_violation_processor_hot_residency_accumulated  |
  | 17   | gpu_violation_soc_thermal_residency_accumulated | gpu_violation_socket_thermal_residency_accumulated |

## v1.3.1

### Release Highlights

- **New Metric Fields**
  - GPU_GFX_BUSY_INSTANTANEOUS, GPU_VC_BUSY_INSTANTANEOUS,
    GPU_JPEG_BUSY_INSTANTANEOUS are added to represent partition activities at
    more granular level.
  - GPU_GFX_ACTIVITY is only applicable for unpartitioned systems, user must
    rely on the new BUSY_INSTANTANEOUS fields on partitioned systems.

- **Health Service Config**
  - Health services can be disabled through configmap

- **Profiler Metrics Default Config Change**
  - The previous release of exporter i.e. v1.3.0's ConfigMap present under
    example directory had Profiler Metrics enabled by default. Now, this is
    set to be disabled by default from v1.3.1 onwards, because profiling is
    generally needed only by application developers. If needed, please enable
    it through the ConfigMap and make sure that there is no other Exporter
    instance or another tool running ROCm profiler at the same time.

- **Notice: Exporter Handling of Unsupported Platform Fields (Upcoming Major Release)**
  - Current Behavior: The exporter sets unsupported platform-specific field metrics to 0.
  - Upcoming Change: In the next major release, the exporter will omit unsupported fields
    (e.g., those marked as N/A in amd-smi) instead of exporting them as 0.
  - Logging: Detailed logs will indicate which fields are unsupported, allowing users to verify platform compatibility.

## v1.3.0

### Release Highlights

- **K8s Extra Pod Labels**
  - Adds more granular Pod level details as labels meta data through configmap
    `ExtraPodLabels`
- **Support for Singularity Installation**
  - Exporter can now be deployed on HPC systems through singularity.
- **Performance Metrics**
  - Adds more profiler related metrics on supported platforms, with toggle
    functionality through configmap `ProfilerMetrics`
- **Custom Prefix for Exporter**
  - Adds more flexibility to add custom prefix to better identify AMD GPU on
    multi cluster deployment, through configmap `CommonConfig`

### Platform Support

ROCm 6.4.x MI3xx

## v1.2.1

### Release Highlights

- **Prometheus Service Monitor**
  - Easy integration with Prometheus Operator
- **K8s Toleration and Selector**
  - Added capability to add tolerations and nodeSelector during helm install

### Platform Support

ROCm 6.3.x

## v1.2.0

### Release Highlights

- **GPU Health Monitoring**
  - Real-time health checks via **metrics exporter**
  - With **Kubernetes Device Plugin** for automatic removal of unhealthy GPUs from compute node schedulable resources
  - Customizable health thresholds via K8s ConfigMaps

### Platform Support

ROCm 6.3.x

## v1.1.0

### Platform Support

ROCm 6.3.x

## v1.0.0

### Release Highlights

- **GPU Metrics Exporter for Prometheus**
  - Real-time metrics exporter for GPU MI platforms.

### Platform Support

ROCm 6.2.x
