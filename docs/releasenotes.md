# Release Notes

## v1.4.1

- **Configurable Resource Limits**
  - Exporter Pod resource limits can now be configured through the Helm chart
  - Default limits are set if not specified in the Helm chart
  - Debian systemd service file is now set with default resource limits

### Platform Support
ROCm 7.0 MI2xx, MI3xx

## v1.4.0

- **MI35x Platfform Support**
  - Exporter now supports MI35x platform with parity with latest supported
    fields.

- **Mask Unsupported Fields**
  - Platform-specific unsupported fields (amd-smi marked as N/A) will not be exported.
    Boot logs will indicate which fields are supported by the platform (logged once during startup).

- **New Profiler Fields**
  - New fields are added for better understanding of the application

- **Depricated Fields Notice**
  - Following fields are depricated from 6.14.14 driver onwards
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

### Known Issues
 - violation metrics are broken on rocm 7.0.0 api change, they are not being exported. This is a known issue and will be fixed in the next release.

  - [Issue 245/246]  enabling exporter profiling metrics can cause the exporter over time to consume more memory and cpu. This is a known issue and will be fixed in the next release.
    - https://github.com/ROCm/device-metrics-exporter/issues/245
    - https://github.com/ROCm/device-metrics-exporter/issues/246

  _Workaround_ is to disable the profiling metrics in the exporter configmap or restart the pod or service at regular intervals.

 *Issues* https://github.com/ROCm/device-metrics-exporter/issues


## v1.3.1

### Release Highlights

- **New Metric Fields**
  - GPU_GFX_BUSY_INSTANTANEOUS, GPU_VC_BUSY_INSTANTANEOUS,
    GPU_JPEG_BUSY_INSTANTANEOUS are added to represent partition activities at
    more granuler level.
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
  - Adds more flexibility to add custome prefix to better identify AMD GPU on
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
