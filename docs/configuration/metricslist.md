# List of Available Metrics

The following document contains a full list of GPU Metrics that are available using the Device Metrics Exporter.

## Platform Support Summary

### MI2xx-Only Metrics
- GPU_AVERAGE_PACKAGE_POWER
- GPU_EDGE_TEMPERATURE

### MI3xx-Only Metrics
- GPU_PACKAGE_POWER
- GPU_JUNCTION_TEMPERATURE
- PCIE_BANDWIDTH
- PCIE_REPLAY_COUNT
- PCIE_RECOVERY_COUNT
- PCIE_REPLAY_ROLLOVER_COUNT
- PCIE_NACK_SENT_COUNT
- PCIE_NACK_RECEIVED_COUNT
- PCIE_BIDIRECTIONAL_BANDWIDTH
- GPU_CURRENT_ACCUMULATED_COUNTER
- GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_ACCUMULATED
- GPU_VIOLATION_PPT_RESIDENCY_ACCUMULATED
- GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACCUMULATED
- GPU_VIOLATION_VR_THERMAL_RESIDENCY_ACCUMULATED
- GPU_VIOLATION_HBM_THERMAL_RESIDENCY_ACCUMULATED
- GPU_GFX_BUSY_INSTANTANEOUS
- GPU_VC_BUSY_INSTANTANEOUS
- GPU_JPEG_BUSY_INSTANTANEOUS
- GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_PERCENTAGE
- GPU_VIOLATION_PPT_RESIDENCY_PERCENTAGE
- GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_PERCENTAGE
- GPU_VIOLATION_VR_THERMAL_RESIDENCY_PERCENTAGE
- GPU_VIOLATION_HBM_THERMAL_RESIDENCY_PERCENTAGE
- GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_POWER_ACCUMULATED
- GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_POWER_PERCENTAGE
- GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_THERMAL_ACCUMULATED
- GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_THERMAL_PERCENTAGE
- GPU_VIOLATION_GFX_CLOCK_LOW_UTILIZATION_ACCUMULATED
- GPU_VIOLATION_GFX_CLOCK_LOW_UTILIZATION_PERCENTAGE
- GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_TOTAL_ACCUMULATED
- GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_TOTAL_PERCENTAGE

### Deprecated Metrics (Not Supported on Any Platform)
- GPU_MMA_ACTIVITY
- GPU_VCN_ACTIVITY
- GPU_JPEG_ACTIVITY
- GPU_VOLTAGE
- GPU_GFX_VOLTAGE
- GPU_MEMORY_VOLTAGE
- PCIE_RX (upcoming feature)
- PCIE_TX (upcoming feature)
- GPU_HBM_TEMPERATURE (deprecated from 6.14.14 driver)

---

## Cluster Management Metrics

### System Information

| Hypervisor | Baremetal | Metric                           | Description                        |
|------------|-----------|----------------------------------|------------------------------------|
| &check;    | &check;   | GPU_NODES_TOTAL `[MI2xx, MI3xx]` | Number of GPU nodes on the machine |

### Temperature Metrics

| Hypervisor | Baremetal | Metric                                  | Description                                                          |
|------------|-----------|-----------------------------------------|----------------------------------------------------------------------|
| &check;    | &check;   | GPU_EDGE_TEMPERATURE `[MI2xx]`          | Edge temperature value in Celsius (MI2XX platforms only)             |
| &check;    | &check;   | GPU_JUNCTION_TEMPERATURE `[MI3xx]`      | Hotspot (aka junction) temperature value in Celsius                  |
| &check;    | &check;   | GPU_MEMORY_TEMPERATURE `[MI2xx, MI3xx]` | Memory temperature value in Celsius                                  |
| &cross;    | &check;   | GPU_HBM_TEMPERATURE `[Deprecated]`      | List of hbm temperatures in Celsius (Deprecated from 6.14.14 driver) |

### Power Metrics

| Hypervisor | Baremetal | Metric                               | Description                                                |
|------------|-----------|--------------------------------------|------------------------------------------------------------|
| &check;    | &check;   | GPU_PACKAGE_POWER `[MI3xx]`          | Current socket power in Watts; not available on guest VM   |
| &cross;    | &check;   | GPU_AVERAGE_PACKAGE_POWER `[MI2xx]`  | Average socket power in Watts; not available on guest VM   |
| &check;    | &check;   | GPU_POWER_USAGE `[MI2xx, MI3xx]`     | GPU power usage in Watts                                   |
| &check;    | &check;   | GPU_ENERGY_CONSUMED `[MI2xx, MI3xx]` | Accumulated energy consumed by the GPU in Micro Jules (uJ) |

### Activity Metrics

| Hypervisor | Baremetal | Metric                                | Description                                                                               |
|------------|-----------|---------------------------------------|-------------------------------------------------------------------------------------------|
| &check;    | &check;   | GPU_GFX_ACTIVITY `[MI2xx, MI3xx]`     | Graphics engine usage percentage (0 - 100) - Applicable for unpartitioned GPU             |
| &check;    | &check;   | GPU_UMC_ACTIVITY `[MI2xx, MI3xx]`     | Memory engine usage percentage (0 - 100)                                                  |
| &cross;    | &cross;   | GPU_MMA_ACTIVITY `[Deprecated]`       | Average multimedia engine usages in percentage (0 - 100) - Deprecated from 6.14.14 driver |
| &cross;    | &cross;   | GPU_VCN_ACTIVITY `[Deprecated]`       | List of VCN encode/decode engine utilization per AID - Deprecated from 6.14.14 driver     |
| &cross;    | &cross;   | GPU_JPEG_ACTIVITY `[Deprecated]`      | List of JPEG engine activity in percentage (0 - 100) - Deprecated from 6.14.14 driver     |
| &cross;    | &check;   | GPU_GFX_BUSY_INSTANTANEOUS `[MI3xx]`  | GFX Busy Instantaneous Activity Per Accelerator Compute Processor Per Compute Core        |
| &cross;    | &check;   | GPU_VC_BUSY_INSTANTANEOUS `[MI3xx]`   | VCN Busy Instantaneous Activity Per Accelerator Compute Processor Per Compute Core        |
| &cross;    | &check;   | GPU_JPEG_BUSY_INSTANTANEOUS `[MI3xx]` | JPEG Busy Instantaneous Activity Per Accelerator Compute Processor Per Compute Core       |

### Voltage Metrics (Deprecated)

| Hypervisor | Baremetal | Metric                            | Description                                     |
|------------|-----------|-----------------------------------|-------------------------------------------------|
| &cross;    | &check;   | GPU_VOLTAGE `[Deprecated]`        | SoC voltage in mV - Deprecated on all platforms |
| &cross;    | &check;   | GPU_GFX_VOLTAGE `[Deprecated]`    | gfx voltage in mV - Deprecated on all platforms |
| &cross;    | &check;   | GPU_MEMORY_VOLTAGE `[Deprecated]` | Mem voltage in mV - Deprecated on all platforms |

### PCIe Metrics

| Hypervisor | Baremetal | Metric                                 | Description                                    |
|------------|-----------|----------------------------------------|------------------------------------------------|
| &check;    | &check;   | PCIE_SPEED `[MI2xx, MI3xx]`            | Current pcie speed capable in GT/s             |
| &check;    | &check;   | PCIE_MAX_SPEED `[MI2xx, MI3xx]`        | Maximum capable pcie speed in GT/s             |
| &check;    | &check;   | PCIE_BANDWIDTH `[MI3xx]`               | Current instantaneous bandwidth usage in Mb/s  |
| &check;    | &check;   | PCIE_REPLAY_COUNT `[MI3xx]`            | Total number of PCIe replays (NAKs)            |
| &check;    | &check;   | PCIE_RECOVERY_COUNT `[MI3xx]`          | Total number of PCIe replays (NAKs)            |
| &check;    | &check;   | PCIE_REPLAY_ROLLOVER_COUNT `[MI3xx]`   | PCIe Replay accumulated count                  |
| &check;    | &check;   | PCIE_NACK_SENT_COUNT `[MI3xx]`         | PCIe NACK sent accumulated count               |
| &check;    | &check;   | PCIE_NACK_RECEIVED_COUNT `[MI3xx]`     | PCIe NACK received accumulated count           |
| &cross;    | &cross;   | PCIE_RX `[Upcoming]`                   | Accumulated bytes received from the PCIe link  |
| &cross;    | &cross;   | PCIE_TX `[Upcoming]`                   | Accumulated bytes transmitted to the PCIe link |
| &cross;    | &check;   | PCIE_BIDIRECTIONAL_BANDWIDTH `[MI3xx]` | Accumulated bandwidth on PCIe link in GB/sec   |

### Clock Metrics

| Hypervisor | Baremetal | Metric                         | Description                                                                  |
|------------|-----------|--------------------------------|------------------------------------------------------------------------------|
| &check;    | &check;   | GPU_CLOCK `[MI2xx, MI3xx]`     | Clock measure of the GPU in Mhz* ([See note below](#gpu_clock-measurements)) |
| &cross;    | &check;   | GPU_MIN_CLOCK `[MI2xx, MI3xx]` | Minimum Clock measure of the GPU in Mhz                                      |
| &cross;    | &check;   | GPU_MAX_CLOCK `[MI2xx, MI3xx]` | Maximum Clock measure of the GPU in Mhz                                      |

### Memory (VRAM) Metrics

| Hypervisor | Baremetal | Metric                                  | Description                               |
|------------|-----------|-----------------------------------------|-------------------------------------------|
| &check;    | &check;   | GPU_TOTAL_VRAM `[MI2xx, MI3xx]`         | Total VRAM available in MB                |
| &cross;    | &check;   | GPU_USED_VRAM `[MI2xx, MI3xx]`          | Total VRAM memory used in MB              |
| &cross;    | &check;   | GPU_FREE_VRAM `[MI2xx, MI3xx]`          | Total VRAM memory free in MB              |
| &cross;    | &check;   | GPU_TOTAL_VISIBLE_VRAM `[MI2xx, MI3xx]` | Total available visible VRAM memory in MB |
| &cross;    | &check;   | GPU_USED_VISIBLE_VRAM `[MI2xx, MI3xx]`  | Total used VRAM memory in MB              |
| &cross;    | &check;   | GPU_FREE_VISIBLE_VRAM `[MI2xx, MI3xx]`  | Total free VRAM memory in MB              |

### GTT Memory Metrics

| Hypervisor | Baremetal | Metric                         | Description                     |
|------------|-----------|--------------------------------|---------------------------------|
| &cross;    | &check;   | GPU_TOTAL_GTT `[MI2xx, MI3xx]` | Total GTT memory in MB          |
| &cross;    | &check;   | GPU_USED_GTT `[MI2xx, MI3xx]`  | Current GTT memory usage in MB  |
| &cross;    | &check;   | GPU_FREE_GTT `[MI2xx, MI3xx]`  | Free GTT memory available in MB |

### ECC Error Metrics

| Hypervisor | Baremetal | Metric                                       | Description                          |
|------------|-----------|----------------------------------------------|--------------------------------------|
| &check;    | &check;   | GPU_ECC_CORRECT_TOTAL `[MI2xx, MI3xx]`       | Total Correctable ECC error count    |
| &check;    | &check;   | GPU_ECC_UNCORRECT_TOTAL `[MI2xx, MI3xx]`     | Total Uncorrectable ECC error count  |
| &check;    | &check;   | GPU_ECC_CORRECT_SDMA `[MI2xx, MI3xx]`        | Correctable ECC error in SDMA        |
| &check;    | &check;   | GPU_ECC_UNCORRECT_SDMA `[MI2xx, MI3xx]`      | Uncorrectable ECC error in SDMA      |
| &check;    | &check;   | GPU_ECC_CORRECT_GFX `[MI2xx, MI3xx]`         | Correctable ECC error in GFX         |
| &check;    | &check;   | GPU_ECC_UNCORRECT_GFX `[MI2xx, MI3xx]`       | Uncorrectable ECC error in GFX       |
| &check;    | &check;   | GPU_ECC_CORRECT_MMHUB `[MI2xx, MI3xx]`       | Correctable ECC error in MMHUB       |
| &check;    | &check;   | GPU_ECC_UNCORRECT_MMHUB `[MI2xx, MI3xx]`     | Uncorrectable ECC error in MMHUB     |
| &cross;    | &check;   | GPU_ECC_CORRECT_ATHUB `[MI2xx, MI3xx]`       | Correctable ECC error in ATHUB       |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_ATHUB `[MI2xx, MI3xx]`     | Uncorrectable ECC error in ATHUB     |
| &check;    | &check;   | GPU_ECC_CORRECT_BIF `[MI2xx, MI3xx]`         | Correctable ECC error in BIF         |
| &check;    | &check;   | GPU_ECC_UNCORRECT_BIF `[MI2xx, MI3xx]`       | Uncorrectable ECC error in BIF       |
| &cross;    | &check;   | GPU_ECC_CORRECT_HDP `[MI2xx, MI3xx]`         | Correctable ECC error in HDP         |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_HDP `[MI2xx, MI3xx]`       | Uncorrectable ECC error in HDP       |
| &check;    | &check;   | GPU_ECC_CORRECT_XGMI_WAFL `[MI2xx, MI3xx]`   | Correctable ECC error in XGMI WAFL   |
| &check;    | &check;   | GPU_ECC_UNCORRECT_XGMI_WAFL `[MI2xx, MI3xx]` | Uncorrectable ECC error in XGMI WAFL |
| &cross;    | &check;   | GPU_ECC_CORRECT_DF `[MI2xx, MI3xx]`          | Correctable ECC error in DF          |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_DF `[MI2xx, MI3xx]`        | Uncorrectable ECC error in DF        |
| &cross;    | &check;   | GPU_ECC_CORRECT_SMN `[MI2xx, MI3xx]`         | Correctable ECC error in SMN         |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_SMN `[MI2xx, MI3xx]`       | Uncorrectable ECC error in SMN       |
| &cross;    | &check;   | GPU_ECC_CORRECT_SEM `[MI2xx, MI3xx]`         | Correctable ECC error in SEM         |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_SEM `[MI2xx, MI3xx]`       | Uncorrectable ECC error in SEM       |
| &cross;    | &check;   | GPU_ECC_CORRECT_MP0 `[MI2xx, MI3xx]`         | Correctable ECC error in MP0         |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_MP0 `[MI2xx, MI3xx]`       | Uncorrectable ECC error in MP0       |
| &cross;    | &check;   | GPU_ECC_CORRECT_MP1 `[MI2xx, MI3xx]`         | Correctable ECC error in MP1         |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_MP1 `[MI2xx, MI3xx]`       | Uncorrectable ECC error in MP1       |
| &cross;    | &check;   | GPU_ECC_CORRECT_FUSE `[MI2xx, MI3xx]`        | Correctable ECC error in FUSE        |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_FUSE `[MI2xx, MI3xx]`      | Uncorrectable ECC error in FUSE      |
| &check;    | &check;   | GPU_ECC_CORRECT_UMC `[MI2xx, MI3xx]`         | Correctable ECC error in UMC         |
| &check;    | &check;   | GPU_ECC_UNCORRECT_UMC `[MI2xx, MI3xx]`       | Uncorrectable ECC error in UMC       |
| &cross;    | &check;   | GPU_ECC_CORRECT_MCA `[MI2xx, MI3xx]`         | Correctable ECC error in MCA         |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_MCA `[MI2xx, MI3xx]`       | Uncorrectable ECC error in MCA       |
| &cross;    | &check;   | GPU_ECC_CORRECT_VCN `[MI2xx, MI3xx]`         | Correctable ECC error in VCN         |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_VCN `[MI2xx, MI3xx]`       | Uncorrectable ECC error in VCN       |
| &cross;    | &check;   | GPU_ECC_CORRECT_JPEG `[MI2xx, MI3xx]`        | Correctable ECC error in JPEG        |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_JPEG `[MI2xx, MI3xx]`      | Uncorrectable ECC error in JPEG      |
| &cross;    | &check;   | GPU_ECC_CORRECT_IH `[MI2xx, MI3xx]`          | Correctable ECC error in IH          |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_IH `[MI2xx, MI3xx]`        | Uncorrectable ECC error in IH        |
| &cross;    | &check;   | GPU_ECC_CORRECT_MPIO `[MI2xx, MI3xx]`        | Correctable ECC error in MPIO        |
| &cross;    | &check;   | GPU_ECC_UNCORRECT_MPIO `[MI2xx, MI3xx]`      | Uncorrectable ECC error in MPIO      |

### XGMI Link Metrics

| Hypervisor | Baremetal | Metric                                    | Description                                                                                                                      |
|------------|-----------|-------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------|
| &cross;    | &check;   | GPU_XGMI_NBR_0_NOP_TX `[MI2xx, MI3xx]`    | NOPs sent to neighbor 0                                                                                                          |
| &cross;    | &check;   | GPU_XGMI_NBR_0_REQ_TX `[MI2xx, MI3xx]`    | Outgoing requests to neighbor 0                                                                                                  |
| &cross;    | &check;   | GPU_XGMI_NBR_0_RESP_TX `[MI2xx, MI3xx]`   | Outgoing responses to neighbor 0                                                                                                 |
| &cross;    | &check;   | GPU_XGMI_NBR_0_BEATS_TX `[MI2xx, MI3xx]`  | Data beats sent to neighbor 0; Each beat represents 32 bytes                                                                     |
| &cross;    | &check;   | GPU_XGMI_NBR_1_NOP_TX `[MI2xx, MI3xx]`    | NOPs sent to neighbor 1                                                                                                          |
| &cross;    | &check;   | GPU_XGMI_NBR_1_REQ_TX `[MI2xx, MI3xx]`    | Outgoing requests to neighbor 1                                                                                                  |
| &cross;    | &check;   | GPU_XGMI_NBR_1_RESP_TX `[MI2xx, MI3xx]`   | Outgoing responses to neighbor 1                                                                                                 |
| &cross;    | &check;   | GPU_XGMI_NBR_1_BEATS_TX `[MI2xx, MI3xx]`  | Data beats sent to neighbor 1; Each beat represents 32 bytes                                                                     |
| &cross;    | &check;   | GPU_XGMI_NBR_0_TX_THRPUT `[MI2xx, MI3xx]` | Represents the number of outbound beats (each representing 32 bytes) on link 0; Throughput = BEATS/time_running * 10^9 bytes/sec |
| &cross;    | &check;   | GPU_XGMI_NBR_1_TX_THRPUT `[MI2xx, MI3xx]` | Represents the number of outbound beats (each representing 32 bytes) on link 1                                                   |
| &cross;    | &check;   | GPU_XGMI_NBR_2_TX_THRPUT `[MI2xx, MI3xx]` | Represents the number of outbound beats (each representing 32 bytes) on link 2                                                   |
| &cross;    | &check;   | GPU_XGMI_NBR_3_TX_THRPUT `[MI2xx, MI3xx]` | Represents the number of outbound beats (each representing 32 bytes) on link 3                                                   |
| &cross;    | &check;   | GPU_XGMI_NBR_4_TX_THRPUT `[MI2xx, MI3xx]` | Represents the number of outbound beats (each representing 32 bytes) on link 4                                                   |
| &cross;    | &check;   | GPU_XGMI_NBR_5_TX_THRPUT `[MI2xx, MI3xx]` | Represents the number of outbound beats (each representing 32 bytes) on link 5                                                   |
| &cross;    | &check;   | GPU_XGMI_LINK_RX `[MI2xx, MI3xx]`         | Accumulated XGMI Link Data Read in KB**                                                                                          |
| &cross;    | &check;   | GPU_XGMI_LINK_TX `[MI2xx, MI3xx]`         | Accumulated XGMI Link Data Write in KB**                                                                                         |

### Throttling & Violation Metrics (MI3xx Only)

| Hypervisor | Baremetal | Metric                                                                 | Description                                                                     |
|------------|-----------|------------------------------------------------------------------------|---------------------------------------------------------------------------------|
| &cross;    | &check;   | GPU_CURRENT_ACCUMULATED_COUNTER `[MI3xx]`                              | Current Accumulated Violation Counter                                           |
| &cross;    | &check;   | GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_ACCUMULATED `[MI3xx]`            | Process Hot Residency Accumulated Violation Counter                             |
| &cross;    | &check;   | GPU_VIOLATION_PPT_RESIDENCY_ACCUMULATED `[MI3xx]`                      | Package Power Tracking Accumulated Violation Counter                            |
| &cross;    | &check;   | GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACCUMULATED `[MI3xx]`           | Socket Thermal accumulated Violation Counter                                    |
| &cross;    | &check;   | GPU_VIOLATION_VR_THERMAL_RESIDENCY_ACCUMULATED `[MI3xx]`               | Voltage Rail accumulated Violation Counter                                      |
| &cross;    | &check;   | GPU_VIOLATION_HBM_THERMAL_RESIDENCY_ACCUMULATED `[MI3xx]`              | HBM Accumulated Violation Counter                                               |
| &cross;    | &check;   | GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_PERCENTAGE `[MI3xx]`             | Process Hot Residency Percentage Violation Counter                              |
| &cross;    | &check;   | GPU_VIOLATION_PPT_RESIDENCY_PERCENTAGE `[MI3xx]`                       | Package Power Tracking Percentage Violation Counter                             |
| &cross;    | &check;   | GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_PERCENTAGE `[MI3xx]`            | Socket Thermal Percentage Violation Counter                                     |
| &cross;    | &check;   | GPU_VIOLATION_VR_THERMAL_RESIDENCY_PERCENTAGE `[MI3xx]`                | Voltage Rail Percentage Violation Counter                                       |
| &cross;    | &check;   | GPU_VIOLATION_HBM_THERMAL_RESIDENCY_PERCENTAGE `[MI3xx]`               | HBM Percentage Violation Counter                                                |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_POWER_ACCUMULATED `[MI3xx]`   | GFX Clock Below Host Limit Power Accumulated Violation Counter Per Compute Core |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_POWER_PERCENTAGE `[MI3xx]`    | GFX Clock Below Host Limit Power Percentage Violation Counter Per Compute Core  |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_THERMAL_ACCUMULATED `[MI3xx]` | GFX Clock Below Host Limit Power Accumulated Violation Counter Per Compute Core |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_THERMAL_PERCENTAGE `[MI3xx]`  | GFX Clock Below Host Limit Power Percentage Violation Counter Per Compute Core  |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_LOW_UTILIZATION_ACCUMULATED `[MI3xx]`          | GFX Clock Low Utilization Accumulated Violation Counter Per Compute Core        |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_LOW_UTILIZATION_PERCENTAGE `[MI3xx]`           | GFX Clock Low Utilization Percentage Violation Counter Per Compute Core         |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_TOTAL_ACCUMULATED `[MI3xx]`   | GFX Clock Below Host Limit Total Accumulated Violation Counter Per Compute Core |
| &cross;    | &check;   | GPU_VIOLATION_GFX_CLOCK_BELOW_HOST_LIMIT_TOTAL_PERCENTAGE `[MI3xx]`    | GFX Clock Below Host Limit Total Percentage Violation Counter Per Compute Core  |

### RAS & Error Reporting

| Hypervisor | Baremetal | Metric                           | Description                                                                                                                                    |
|------------|-----------|----------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| &cross;    | &check;   | GPU_AFID_ERRORS `[MI2xx, MI3xx]` | Last Occured RAS Event associated AMD Field Identifier list, More Info in https://docs.amd.com/r/en-US/AMD_Field_ID_70122_v1.0/AFID-Event-List |

---

## Performance Metrics Focused For Application Development

The Device Metrics Exporter now supports a whole list of Performance metrics to better facilitate the developers to understand more about the application performance on the GPUs.

The `ProfilerMetrics` should be turned off in case of application level profiling is run as the current hardware limits a single profiler instance to be run at any given time.

By default the performance metrics are disabled and they can be enabled through the ConfigMap `ProfilerMetrics` to enable or disable per host (higher precedence) or `all` key to specify for cluster wide toggle.

The list comprises of all well known metrics supported by MI 200 & 300 platforms. Some fields which are not supported by platforms though enabled would not be exported. The full list of supported Fields and Registers are available at [Performance Counters](https://rocm.docs.amd.com/en/latest/conceptual/gpu-arch/mi300-mi200-performance-counters.html).

**_Note_**: Disabling Performance metrics (prefixed with `GPU_PROF_`) doesn't stop non performance metrics from being exported.

### Command Processor Metrics

| Metric                                      | Description                                                                                    |
|---------------------------------------------|------------------------------------------------------------------------------------------------|
| GPU_PROF_CPC_CPC_STAT_BUSY                  | Number of cycles command processor-compute is busy                                             |
| GPU_PROF_CPC_CPC_STAT_IDLE                  | Number of cycles command processor-compute is idle                                             |
| GPU_PROF_CPC_CPC_STAT_STALL                 | Number of cycles command processor-compute is stalled                                          |
| GPU_PROF_CPC_CPC_TCIU_BUSY                  | Number of cycles command processor-compute texture cache interface unit interface is busy      |
| GPU_PROF_CPC_CPC_TCIU_IDLE                  | Number of cycles command processor-compute texture cache interface unit interface is idle      |
| GPU_PROF_CPC_CPC_UTCL2IU_BUSY               | Number of cycles command processor-compute unified translation cache (L2) interface is busy    |
| GPU_PROF_CPC_CPC_UTCL2IU_IDLE               | Number of cycles command processor-compute unified translation cache (L2) interface is idle    |
| GPU_PROF_CPC_CPC_UTCL2IU_STALL              | Number of cycles command processor-compute unified translation cache (L2) interface is stalled |
| GPU_PROF_CPC_ME1_BUSY_FOR_PACKET_DECODE     | Number of cycles command processor-compute micro engine is busy decoding packets               |
| GPU_PROF_CPC_ME1_DC0_SPI_BUSY               | Number of cycles command processor-compute micro engine processor is busy                      |
| GPU_PROF_CPC_UTCL1_STALL_ON_TRANSLATION     | Number of cycles one of the unified translation caches (L1) is stalled waiting on translation  |
| GPU_PROF_CPC_ALWAYS_COUNT                   | CPC Always Count                                                                               |
| GPU_PROF_CPC_ADC_VALID_CHUNK_NOT_AVAIL      | CPC ADC valid chunk not available when dispatch walking is in progress at multi-xcc mode       |
| GPU_PROF_CPC_ADC_DISPATCH_ALLOC_DONE        | CPC ADC dispatch allocation done                                                               |
| GPU_PROF_CPC_ADC_VALID_CHUNK_END            | CPC ADC crawler valid chunk end at multi-xcc mode                                              |
| GPU_PROF_CPC_SYNC_FIFO_FULL_LEVEL           | CPC SYNC FIFO full last cycles                                                                 |
| GPU_PROF_CPC_SYNC_FIFO_FULL                 | CPC SYNC FIFO full times                                                                       |
| GPU_PROF_CPC_GD_BUSY                        | CPC ADC busy                                                                                   |
| GPU_PROF_CPC_TG_SEND                        | CPC ADC thread group send                                                                      |
| GPU_PROF_CPC_WALK_NEXT_CHUNK                | CPC ADC walking next valid chunk at multi-xcc mode                                             |
| GPU_PROF_CPC_STALLED_BY_SE0_SPI             | CPC ADC csdata stalled by SE0SPI                                                               |
| GPU_PROF_CPC_STALLED_BY_SE1_SPI             | CPC ADC csdata stalled by SE1SPI                                                               |
| GPU_PROF_CPC_STALLED_BY_SE2_SPI             | CPC ADC csdata stalled by SE2SPI                                                               |
| GPU_PROF_CPC_STALLED_BY_SE3_SPI             | CPC ADC csdata stalled by SE3SPI                                                               |
| GPU_PROF_CPC_LTE_ALL                        | CPC Sync counter LteAll, only Master XCD cares LteAll                                          |
| GPU_PROF_CPC_SYNC_WRREQ_FIFO_BUSY           | CPC Sync Counter Request Fifo is not empty                                                     |
| GPU_PROF_CPC_CANE_BUSY                      | CPC CANE bus busy, means there are inflight sync counter requests                              |
| GPU_PROF_CPC_CANE_STALL                     | CPC Sync counter sending is stalled by CANE                                                    |
| GPU_PROF_CPF_CMP_UTCL1_STALL_ON_TRANSLATION | One of the Compute UTCL1s is stalled waiting on translation, XNACK or PENDING response         |
| GPU_PROF_CPF_CPF_STAT_BUSY                  | CPF Busy                                                                                       |
| GPU_PROF_CPF_CPF_STAT_IDLE                  | CPF Idle                                                                                       |
| GPU_PROF_CPF_CPF_STAT_STALL                 | CPF Stalled                                                                                    |
| GPU_PROF_CPF_CPF_TCIU_BUSY                  | CPF TCIU interface Busy                                                                        |
| GPU_PROF_CPF_CPF_TCIU_IDLE                  | CPF TCIU interface Idle                                                                        |
| GPU_PROF_CPF_CPF_TCIU_STALL                 | CPF TCIU interface Stalled waiting on Free, Tags                                               |

### GPU Core Metrics

| Metric                    | Description                                                                                              |
|---------------------------|----------------------------------------------------------------------------------------------------------|
| GPU_PROF_GRBM_GUI_ACTIVE  | Number of GPU active cycles                                                                              |
| GPU_PROF_SQ_WAVES         | Number of wavefronts dispatched to sequencers, including both new and restored wavefronts                |
| GPU_PROF_GRBM_COUNT       | Number of free-running GPU cycles                                                                        |
| GPU_PROF_GUI_UTIL_PERCENT | Percentage of the time that GUI is active                                                                |
| GPU_PROF_SM_ACTIVE        | The percentage of GPUTime vector ALU instructions are processed. Value range: 0% (bad) to 100% (optimal) |

### Memory & Data Transfer Metrics

| Metric              | Description                                                                                                                                   |
|---------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| GPU_PROF_FETCH_SIZE | The total kilobytes fetched from the video memory. This is measured with all extra fetches and any cache or memory effects taken into account |
| GPU_PROF_WRITE_SIZE | The total kilobytes written to the video memory. This is measured with all extra fetches and any cache or memory effects taken into account   |

### Compute Operation Metrics

| Metric                | Description                        |
|-----------------------|------------------------------------|
| GPU_PROF_TOTAL_16_OPS | The number of 16 bits OPS executed |
| GPU_PROF_TOTAL_32_OPS | The number of 32 bits OPS executed |
| GPU_PROF_TOTAL_64_OPS | The number of 64 bits OPS executed |

### Occupancy & Utilization Metrics

| Metric                           | Description                                         |
|----------------------------------|-----------------------------------------------------|
| GPU_PROF_OCCUPANCY_PERCENT       | GPU Occupancy as Percentage of maximum              |
| GPU_PROF_TENSOR_ACTIVE_PERCENT   | MFMA Utilization Unit percent                       |
| GPU_PROF_VALU_PIPE_ISSUE_UTIL    | Percentage of the time that GUI is active           |
| GPU_PROF_OCCUPANCY_ELAPSED       | Number of GPU active cycles                         |
| GPU_PROF_OCCUPANCY_PER_ACTIVE_CU | Mean occupancy per active compute unit              |
| GPU_PROF_OCCUPANCY_PER_CU        | Mean occupancy per compute unit                     |
| GPU_PROF_SIMD_UTILIZATION        | Fraction of time the SIMDs are being utilized [0,1] |

---

## Array-Based Field Descriptions

The following sections describe metrics that return arrays of values, where each element is distinguished by specific label indices to represent different components or links within the GPU hardware.

### GPU_CLOCK measurements

The Device Metrics Exporter `gpu_clock` metric is a common field used for exporting different types of clocks. This metric has a `clock_type` label added to the metric to differentiate the different clock types:

```json
gpu_clock{clock_type="data"}
gpu_clock{clock_type="system"}
gpu_clock{clock_type="memory"}
gpu_clock{clock_type="video"}
gpu_clock{clock_type="soc"}
```

An example of this is shown below:

```json
gpu_clock{card_model="xxxx",clock_index="14",clock_type="data",gpu_compute_partition_type="spx",gpu_id="3",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 22
gpu_clock{card_model="xxxx",clock_index="2",clock_type="system",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 132
gpu_clock{card_model="xxxx",clock_index="8",clock_type="memory",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 900
gpu_clock{card_model="xxxx",clock_index="9",clock_type="video",gpu_compute_partition_type="spx",gpu_id="5",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 29
gpu_clock{card_model="xxxx",clock_index="9",clock_type="soc",gpu_compute_partition_type="spx",gpu_id="5",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 29
```

### XGMI Link Read and Write measurements

The Device Metrics Exporter `gpu_xgmi_link_rx` and `gpu_xgmi_link_tx` metrics consist of an array field used for exporting the transfer metrics for each xgmi link connected to a GPU. These metric have a `link_index` label added to the metric to differentiate the different links (usually 8 in an MI300X system):

```json
gpu_xgmi_link_rx{link_index="0"}
gpu_xgmi_link_rx{link_index="1"}
gpu_xgmi_link_rx{link_index="2"}
gpu_xgmi_link_rx{link_index="3"}

gpu_xgmi_link_tx{link_index="0"}
gpu_xgmi_link_tx{link_index="1"}
gpu_xgmi_link_tx{link_index="2"}
gpu_xgmi_link_tx{link_index="3"}
```

An example of this is shown below:

```json
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="0",serial_number="xxxx"} 0
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="1",serial_number="xxxx"} 2.776148269e+09
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="2",serial_number="xxxx"} 2.914491813e+09
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="3",serial_number="xxxx"} 2.853215723e+09
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="4",serial_number="xxxx"} 2.857943554e+09
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="5",serial_number="xxxx"} 2.859773597e+09
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="6",serial_number="xxxx"} 2.852296682e+09
gpu_xgmi_link_rx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="7",serial_number="xxxx"} 2.757052542e+09

gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="0",serial_number="xxxx"} 0
gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="1",serial_number="xxxx"} 3.539423344e+09
gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="2",serial_number="xxxx"} 3.708415141e+09
gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="3",serial_number="xxxx"} 3.639806555e+09
gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="4",serial_number="xxxx"} 3.675549728e+09
gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="5",serial_number="xxxx"} 3.657430314e+09
gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="6",serial_number="xxxx"} 3.646094607e+09
gpu_xgmi_link_tx{card_model="xxxx",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",link_index="7",serial_number="xxxx"} 3.545990503e+0
```

### AFID Error metrics

The Device Metrics Exporter `gpu_afid_errors` metric consists of an array field used for exporting the AFID list each indexed with label `afid_index`. This metric has a `severity` label added to the metric to differentiate the different severity levels:

`severity` label can have one of the following values:
- fatal
- non_fatal_uncorrected
- non_fatal_corrected

```json
# HELP gpu_afid_errors Last Occured RAS Event associated AMD Field Identifier list
# TYPE gpu_afid_errors gauge
gpu_afid_errors{afid_index="0", severity="fatal", gpu_id="0", ...} 30
gpu_afid_errors{afid_index="1", severity="fatal", gpu_id="0", ...} 25
