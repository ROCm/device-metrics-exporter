# Critical Metrics
- The below list of metrics are critical to evaluate the workload is running as expected on the GPU, these metric values change as per the workload exercised on the GPU

> **Note:** few metrics are applicable for unpartitioned GPU, some fields are platform specific, please refer [link](../docs/configuration/metricslist.md) for more details

  - Temperature Metrics
    1. GPU_EDGE_TEMPERATURE
    2. GPU_JUNCTION_TEMPERATURE
    3. GPU_MEMORY_TEMPERATURE
    4. GPU_HBM_TEMPERATURE

  - Power Metrics
    1. GPU_PACKAGE_POWER
    2. GPU_AVERAGE_PACKAGE_POWER

  - Activity Metrics
    1. GPU_GFX_ACTIVITY
    2. GPU_UMC_ACTIVITY
    3. GPU_GFX_BUSY_INSTANTANEOUS
    4. GPU_VCN_BUSY_INSTANTANEOUS

  - VRAM Metrics
    1. GPU_TOTAL_VRAM
    2. GPU_USED_VRAM
    3. GPU_FREE_VRAM

  - Profiler Metrics
    1. GPU_PROF_SM_ACTIVE
    2. GPU_PROF_TENSOR_ACTIVE_PERCENT
    3. GPU_PROF_OCCUPANCY_PER_CU
    4. GPU_PROF_OCCUPANCY_PER_ACTIVE_CU
    5. GPU_PROF_SIMD_UTILIZATION
    6. GPU_PROF_GUI_UTIL_PERCENT


# Internal Mapping of Field on each service

Platform if specified only applies to that specific model, else applies to all

| Exporter Metric                                     | GPU Agent                                                   | amd-smi                                           | Platform                   |
|-----------------------------------------------------|-------------------------------------------------------------|---------------------------------------------------|----------------------------|
| GPU_NODES_TOTAL                                     |                                                             |                                                   |                            |
| GPU_PACKAGE_POWER                                   | stats.PackagePower                                          | power_info.current_socket_power                   | MI3xx                      |
| GPU_AVERAGE_PACKAGE_POWER                           | stats.AvgPackagePower                                       | power_info.average_socket_power                   | MI2xx                      |
| GPU_EDGE_TEMPERATURE                                | stats.temperature.edge_temperature                          | temp.edge                                         | MI2xx                      |
| GPU_JUNCTION_TEMPERATURE                            | stats.temperature.junction_temperature                      | temp.junction/hotspot                             | MI3xx                      |
| GPU_MEMORY_TEMPERATURE                              | stats.temperature.memory_temperature                        | temp.memory                                       |                            |
| GPU_HBM_TEMPERATURE                                 | stats.temperature.hbm_temperature[i]                        | temp.hbm[i]                                       |             Depricated from 6.14.14 driver |
| GPU_GFX_ACTIVITY (Applicable for unpartitioned GPU) | stats.usage.gfx_activity                                    | usage.gfx_activity                                |                            |
| GPU_UMC_ACTIVITY                                    | stats.usage.umc_activity                                    | usage.umc_activity                                |                            |
| GPU_MMA_ACTIVITY                                    | stats.usage.mm_activity                                     | usage.mm_activity                                 | depricated on all platform |
| GPU_VCN_ACTIVITY                                    | stats.usage.vcn_activity[i]                                 | metrics_info.vcn_activity [i]                     | depricated on all platform |
| GPU_JPEG_ACTIVITY                                   | stats.usage.jpeg_activity[i]                                | metrics_info.jpeg_activity[i]                     | depricated on all platform |
| GPU_VOLTAGE                                         | stats.voltage.voltage                                       | power_info.soc_voltage                            | depricated on all platform |
| GPU_GFX_VOLTAGE                                     | stats.voltage.gfx_voltage                                   | power_info.gfx_voltage                            | depricated on all platform |
| GPU_MEMORY_VOLTAGE                                  | stats.voltage.memory_voltage                                | power_info.mem_voltage                            | depricated on all platform |
| PCIE_SPEED                                          | status.pcie_status->speed                                   | pcie_metric.pcie_speed/1000                       |                            |
| PCIE_MAX_SPEED                                      | status.pcie_status->max_speed                               | pcie_static.max_pcie_speed/1000                   |                            |
| PCIE_BANDWIDTH                                      | status.pcie_status->bandwidth                               | pcie_metric.pcie_bandwidth                        | MI3xx                      |
| GPU_ENERGY_CONSUMED                                 | stats.energy_consumed                                       | energy.total_energy_consumption                   |                            |
| PCIE_REPLAY_COUNT                                   | stats->pcie_stats.replay_count                              | pcie_info.pcie_metric.pcie_replay_count           | MI3xx                      |
| PCIE_RECOVERY_COUNT                                 | stats->pcie_stats.recovery_count                            | pcie_info.pcie_metric.pcie_l0_to_recovery_count   | MI3xx                      |
| PCIE_REPLAY_ROLLOVER_COUNT                          | stats->pcie_stats.replay_rollover_count                     | pcie_info.pcie_metric.pcie_replay_roll_over_count | MI3xx                      |
| PCIE_NACK_SENT_COUNT                                | stats->pcie_stats.nack_sent_count                           | pcie_info.pcie_metric.pcie_nak_sent_count         | MI3xx                      |
| PCIE_NAC_RECEIVED_COUNT                             | stats->pcie_stats.nack_received_count                       | pcie_info.pcie_metric.pcie_nak_received_count     | MI3xx                      |
| PCIE_RX                                             | stats->pcie_stats.rx_bytes                                  | pcie_info.pcie_metric.CURRENT_BANDWIDTH_SENT      | (upcoming feature)         |
| PCIE_TX                                             | stats->pcie_stats.tx_bytes                                  | pcie_info.pcie_metric.CURRENT_BANDWIDTH_RECEIVED  | (upcoming feature)         |
| PCIE_BIDIRECTIONAL_BANDWIDTH                        | stats->pcie_stats.bidir_bandwidth                           | pcie_info.pcie_metric.pcie_bandwidth_acc          |  MI3xx api only (grep for pcie_bandwidth_acc in  `rocm-smi --showmetrics`)                           |
| GPU_CLOCK                                           | status.clock_status[i] SYSTEM                               | metrics_info->current_gfxclks[i]                  |                            |
|                                                     | status.clock_status[i] MEMORY                               | metrics_info->current_uclk                        |                            |
|                                                     | status.clock_status[i] VIDEO                                | metrics_info->current_vclk0s[i]                   |                            |
|                                                     | status.clock_status[i] DATA                                 | metrics_info->current_dclk0s[i]                   |                            |
| GPU_POWER_USAGE                                     | stats.power_usage                                           | gpu_metrics.current_socket_power                  | MI3xx                      |
|                                                     | stats.power_usage                                           | gpu_metrics.average_socket_power                  | MI2xx                      |
| GPU_TOTAL_VRAM                                      | status.vramstatus.size                                      | mem_usage.total_vram                              |                            |
| GPU_ECC_CORRECT_TOTAL                               |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_TOTAL                             |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_SDMA                                |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_SDMA                              |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_GFX                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_GFX                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_MMHUB                               |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_MMHUB                             |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_ATHUB                               |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_ATHUB                             |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_BIF                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_BIF                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_HDP                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_HDP                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_XGMI_WAFL                           |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_XGMI_WAFL                         |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_DF                                  |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_DF                                |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_SMN                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_SMN                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_SEM                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_SEM                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_MP0                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_MP0                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_MP1                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_MP1                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_FUSE                                |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_FUSE                              |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_UMC                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_UMC                               |                                                             |                                                   |                            |
| GPU_XGMI_NBR_0_NOP_TX                               |                                                             |                                                   |                            |
| GPU_XGMI_NBR_0_REQ_TX                               |                                                             |                                                   |                            |
| GPU_XGMI_NBR_0_RESP_TX                              |                                                             |                                                   |                            |
| GPU_XGMI_NBR_0_BEATS_TX                             |                                                             |                                                   |                            |
| GPU_XGMI_NBR_1_NOP_TX                               |                                                             |                                                   |                            |
| GPU_XGMI_NBR_1_REQ_TX                               |                                                             |                                                   |                            |
| GPU_XGMI_NBR_1_RESP_TX                              |                                                             |                                                   |                            |
| GPU_XGMI_NBR_1_BEATS_TX                             |                                                             |                                                   |                            |
| GPU_XGMI_NBR_0_TX_THRPUT                            |                                                             |                                                   |                            |
| GPU_XGMI_NBR_1_TX_THRPUT                            |                                                             |                                                   |                            |
| GPU_XGMI_NBR_2_TX_THRPUT                            |                                                             |                                                   |                            |
| GPU_XGMI_NBR_3_TX_THRPUT                            |                                                             |                                                   |                            |
| GPU_XGMI_NBR_4_TX_THRPUT                            |                                                             |                                                   |                            |
| GPU_XGMI_NBR_5_TX_THRPUT                            |                                                             |                                                   |                            |
| GPU_XGMI_LINK_RX                                    |                                                             |                                                   |                            |
| GPU_XGMI_LINK_TX                                    |                                                             |                                                   |                            |
| GPU_USED_VRAM                                       | stats.vram_usage.used_vram                                  | mem_usage.used_vram                               |                            |
| GPU_FREE_VRAM                                       | stats.vram_usage.free_vram                                  | mem_usage.free_vram                               |                            |
| GPU_TOTAL_VISIBLE_VRAM                              | stats.vram_usage.total_visible_vram                         | mem_usage.total_visible_vram                      |                            |
| GPU_USED_VISIBLE_VRAM                               | stats.vram_usage.used_visible_vram                          | mem_usage.used_visible_vram                       |                            |
| GPU_FREE_VISIBLE_VRAM                               | stats.vram_usage.free_visible_vram                          | mem_usage.free_visible_vram                       |                            |
| GPU_TOTAL_GTT                                       | stats.vram_usage.total_gtt                                  | mem_usage.total_gtt                               |                            |
| GPU_USED_GTT                                        | stats.vram_usage.used_gtt                                   | mem_usage.used_gtt                                |                            |
| GPU_FREE_GTT                                        | stats.vram_usage.free_gtt                                   | mem_usage.free_gtt                                |                            |
| GPU_ECC_CORRECT_MCA                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_MCA                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_VCN                                 |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_VCN                               |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_JPEG                                |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_JPEG                              |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_IH                                  |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_IH                                |                                                             |                                                   |                            |
| GPU_ECC_CORRECT_MPIO                                |                                                             |                                                   |                            |
| GPU_ECC_UNCORRECT_MPIO                              |                                                             |                                                   |                            |
| GPU_CURRENT_ACCUMULATED_COUNTER                     | stats->violation_stats.current_accumulated_counter          | metrics_info.accumulation_counter                 | MI3xx                      |
| GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_ACCUMULATE    | stats->violation_stats.processor_hot_residency_accumulated  | metrics_info.prochot_residency_acc                | MI3xx                      |
| GPU_VIOLATION_PPT_RESIDENCY_ACCUMULATED             | stats->violation_stats.ppt_residency_accumulated            | metrics_info.ppt_residency_acc                    | MI3xx                      |
| GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACCUMULAT    | stats->violation_stats.socket_thermal_residency_accumulated | metrics_info.socket_thm_residency_acc             | MI3xx                      |
| GPU_VIOLATION_VR_THERMAL_RESIDENCY_ACCUMULATED      | stats->violation_stats.vr_thermal_residency_accumulated     | metrics_info.vr_thm_residency_acc                 | MI3xx                      |
| GPU_VIOLATION_HBM_THERMAL_RESIDENCY_ACCUMULATED     | stats->violation_stats.hbm_thermal_residency_accumulated    | metrics_info.hbm_thm_residency_acc                | MI3xx                      |
| GPU_GFX_BUSY_INSTANTANEOUS                          | stats.usage.gfx_busy_inst                                   | usage.gfx_busy_inst.xcp_[partition_id]            | MI3xx                      |
| GPU_VCN_BUSY_INSTANTANEOUS                          | stats.usage.vcn_busy_inst                                   | usage.vcn_busy_inst.xcp_[partition_id]            | MI3xx                      |
| GPU_JPEG_BUSY_INSTANTANEOUS                         | stats.usage.jpeg_busy_inst                                  | usage.jpeg_busy_inst.xcp_[partition_id]           | MI3xx                      |
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------

## AMD-SMI Command Line Reference

```bash
# amd-smi --help
```

### GPU List

```bash
#  amd-smi list
GPU: 0
    BDF: 0000:83:00.0
    UUID: 9fff74b9-0000-1000-8044-4a2fb30adeb0
    KFD_ID: 43855
    NODE_ID: 9
    PARTITION_ID: 0

GPU: 1
    BDF: 0000:8b:00.0
    UUID: b7ff74b9-0000-1000-80d1-81671a192e45
    KFD_ID: 27976
    NODE_ID: 8
    PARTITION_ID: 0

---- snipped ----

```

### GPU List Enumerated

```bash
# amd-smi list -e

GPU: 0
    BDF: 0000:83:00.0
    UUID: 9fff74b9-0000-1000-8044-4a2fb30adeb0
    KFD_ID: 43855
    NODE_ID: 9
    PARTITION_ID: 0
    RENDER: renderD185
    CARD: card57
    HSA_ID: 9
    HIP_ID: 7
    HIP_UUID: GPU-9f444a2fb30adeb0

GPU: 1
    BDF: 0000:8b:00.0
    UUID: b7ff74b9-0000-1000-80d1-81671a192e45
    KFD_ID: 27976
    NODE_ID: 8
    PARTITION_ID: 0
    RENDER: renderD177
    CARD: card49
    HSA_ID: 8
    HIP_ID: 6
    HIP_UUID: GPU-b7d181671a192e45

---- snipped ----

```


### GPU Static information

```bash
# amd-smi static -g 0
GPU: 0
    ASIC:
        MARKET_NAME: AMD Instinct Mi325X VF
        VENDOR_ID: 0x1002
        VENDOR_NAME: Advanced Micro Devices Inc. [AMD/ATI]
        SUBVENDOR_ID: 0x1002
        DEVICE_ID: 0x74b9
        SUBSYSTEM_ID: 0x74a5
        REV_ID: 0x00
        ASIC_SERIAL: 0x9F444A2FB30ADEB0
        OAM_ID: 6
        NUM_COMPUTE_UNITS: 304
        TARGET_GRAPHICS_VERSION: gfx942
    BUS:
        BDF: 0000:83:00.0
        MAX_PCIE_WIDTH: 16
        MAX_PCIE_SPEED: 32 GT/s
        PCIE_INTERFACE_VERSION: Gen 5
        SLOT_TYPE: OAM

---- snipped ----

```

### GPU Metrics

```bash
# amd-smi metric -g 0

            DEEP_SLEEP: ENABLED
        GFX_7:
            CLK: 131 MHz
            MIN_CLK: 500 MHz
            MAX_CLK: 2100 MHz
            CLK_LOCKED: DISABLED
            DEEP_SLEEP: ENABLED
        MEM_0:
            CLK: 1198 MHz
            MIN_CLK: 900 MHz
            MAX_CLK: 1500 MHz
            DEEP_SLEEP: DISABLED
        VCLK_0:
            CLK: 40 MHz

---- snipped ----

```

### GPU Partition Information

```bash
# amd-smi partition -g 0
CURRENT_PARTITION:
GPU_ID  MEMORY  ACCELERATOR_TYPE  ACCELERATOR_PROFILE_INDEX  PARTITION_ID
0       NPS1    SPX               0                          0

MEMORY_PARTITION:
GPU_ID  MEMORY_PARTITION_CAPS  CURRENT_MEMORY_PARTITION
0       N/A                    NPS1

ACCELERATOR_PARTITION_PROFILES:
GPU_ID  PROFILE_INDEX  MEMORY_PARTITION_CAPS  ACCELERATOR_TYPE  PARTITION_ID     NUM_PARTITIONS  NUM_RESOURCES  RESOURCE_INDEX  RESOURCE_TYPE  RESOURCE_INSTANCES  RESOURCES_SHARED
0       0              N/A                    SPX*              0                1               4              0               XCC            8                   1
                                                                                                                1               DECODER        4                   1
                                                                                                                2               DMA            16                  1
                                                                                                                3               JPEG           32                  1
        1              N/A                    DPX               N/A              2               4              4               XCC            4                   1
                                                                                                                5               DECODER        2                   1
                                                                                                                6               DMA            8                   1
                                                                                                                7               JPEG           16                  1
        2              N/A                    QPX               N/A              4               4              8               XCC            2                   1
                                                                                                                9               DECODER        1                   1
                                                                                                                10              DMA            4                   1
                                                                                                                11              JPEG           8                   1
        3              N/A                    CPX               N/A              8               4              12              XCC            1                   1
                                                                                                                13              DECODER        1                   2
                                                                                                                14              DMA            2                   1
                                                                                                                15              JPEG           8                   2

ACCELERATOR_PARTITION_RESOURCES:
RESOURCE_INDEX  RESOURCE_TYPE  RESOURCE_INSTANCES  RESOURCES_SHARED
0               XCC            8                   1
1               DECODER        4                   1
2               DMA            16                  1
3               JPEG           32                  1
4               XCC            4                   1
5               DECODER        2                   1
6               DMA            8                   1
7               JPEG           16                  1
8               XCC            2                   1
9               DECODER        1                   1
10              DMA            4                   1
11              JPEG           8                   1
12              XCC            1                   1
13              DECODER        1                   2
14              DMA            2                   1
15              JPEG           8                   2


Legend:
  * = Current mode

```

### GPU CPER Read

```bash
$ sudo amd-smi ras --cper
WARNING:No CPER files will be dumped unless --folder=<folder_name> is specified and cper entries exist.
timestamp            gpu_id  severity
2025/10/07 09:58:04  0       FATAL
2025/10/09 05:09:13  0       FATAL
```

