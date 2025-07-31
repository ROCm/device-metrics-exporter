# Release Notes

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
