# Release Notes

## v1.5.0

- **Kubevirt**
  - Exporter now supports Kubevirt deployments
    - New exporter with SR-IOV support for hypervisor environments is now available
      - Legacy exporter remains applicable for existing deployments:
        1. Baremetal passthrough
        2. Guest VM

- **Slinky**
  - Slinky job reporting is now supported, with labels providing both Kubernetes and Slurm job information

- **New Label**
  - `KFD_PROCESS_ID` label will  now report the process ID using the
  	respective GPU. This enables baremetal debian deployments to have job
  	information where no scheduler is used.

### Platform Support
ROCm 7.0.rc1 MI2xx, MI3xx

## v1.4.0

- **MI35x Platfform Support**
  - Exporter now supports MI35x platform with parity with latest supported
    fields.

- **Mask Unsupported Fields**
  - Platform-specific unsupported fields (amd-smi marked as N/A) will not be exported.
    Boot logs will indicate which fields are supported by the platform (logged once during startup).

- **New Profiler Fields**
  - New fields are added for better understanding of the application

### Platform Support
ROCm 7.0.rc1 MI2xx, MI3xx


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
