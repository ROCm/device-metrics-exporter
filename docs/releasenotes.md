# Release Notes

## v1.3.1

### Release Highlights

- ** New Fields **
  - GPU_GFX_BUSY_INSTANTANEOUS, GPU_VC_BUSY_INSTANTANEOUS,
    GPU_JPEG_BUSY_INSTANTANEOUS are added to represent partition activities at
    more granuler level.
  - GPU_GFX_ACTIVITY is only applicable for unpartitioned systems, user must
    rely on the new BUSY_INSTANTANEOUS fields on partitioned systems.

- ** Health Service Config **
  - Health services can be disabled through configmap

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
