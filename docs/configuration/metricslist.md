# List of Available Metrics

The following table contains a full list of GPU Metrics that are available using the Device Metrics Exporter. Descriptions of each metric will be included at a later time.

| Metric                          | Description                                                                 |
|---------------------------------|-----------------------------------------------------------------------------|
| GPU_NODES_TOTAL                 | Number of GPU nodes on the machine                                          |
| GPU_PACKAGE_POWER               | Current socket power in Watts; not available on guest VM                   |
| GPU_AVERAGE_PACKAGE_POWER       | Average socket power in Watts; not available on guest VM                   |
| GPU_EDGE_TEMPERATURE            | Edge temperature value in Celsius                                          |
| GPU_JUNCTION_TEMPERATURE        | Hotspot (aka junction) temperature value in Celsius                        |
| GPU_MEMORY_TEMPERATURE          | Memory temperature value in Celsius                                        |
| GPU_HBM_TEMPERATURE             | List of hbm temperatures in Celsius                                        |
| GPU_GFX_ACTIVITY                | Graphics engine usage percentage (0 - 100)                                 |
| GPU_UMC_ACTIVITY                | Memory engine usage percentage (0 - 100)                                   |
| GPU_MMA_ACTIVITY                | Average multimedia engine usages in percentage (0 - 100)                   |
| GPU_VCN_ACTIVITY                | List of VCN encode/decode engine utilization per AID                       |
| GPU_JPEG_ACTIVITY               | List of JPEG engine activity in percentage (0 - 100)                       |
| GPU_VOLTAGE                     | SoC voltage in mV                                                          |
| GPU_GFX_VOLTAGE                 | gfx voltage in mV                                                          |
| GPU_MEMORY_VOLTAGE              | Mem voltage in mV                                                          |
| PCIE_SPEED                      | Current pcie speed capable in GT/s                                         |
| PCIE_MAX_SPEED                  | Maximum capable pcie speed in GT/s                                         |
| PCIE_BANDWIDTH                  | Current instantaneous bandwidth usage in Mb/s                              |
| GPU_ENERGY_CONSUMED             | Energy consumed by GPU in Micro Jules (uJ)                                 |
| PCIE_REPLAY_COUNT               | Total number of PCIe replays (NAKs)                                        |
| PCIE_RECOVERY_COUNT             | Total number of PCIe replays (NAKs)                                        |
| PCIE_REPLAY_ROLLOVER_COUNT      | PCIe Replay accumulated count                                              |
| PCIE_NACK_SENT_COUNT            | PCIe NAK sent accumulated count                                            |
| PCIE_NAC_RECEIVED_COUNT         | PCIe NAK received accumulated count                                        |
| GPU_CLOCK                       | Clock measure of the GPU in Mhz* ([See note below](#gpu_clock-measurements))|
| GPU_POWER_USAGE                 | GPU power usage in Watts                                                   |
| GPU_TOTAL_VRAM                  | Total VRAM available in MB                                                 |
| GPU_ECC_CORRECT_TOTAL           | Total Correctable ECC error count                                          |
| GPU_ECC_UNCORRECT_TOTAL         | Total Uncorrectable ECC error count                                        |
| GPU_ECC_CORRECT_SDMA            | Correctable ECC error in SDMA                                              |
| GPU_ECC_UNCORRECT_SDMA          | Uncorrectable ECC error in SDMA                                            |
| GPU_ECC_CORRECT_GFX             | Correctable ECC error in GFX                                               |
| GPU_ECC_UNCORRECT_GFX           | Uncorrectable ECC error in GFX                                             |
| GPU_ECC_CORRECT_MMHUB           | Correctable ECC error in MMHUB                                             |
| GPU_ECC_UNCORRECT_MMHUB         | Uncorrectable ECC error in MMHUB                                           |
| GPU_ECC_CORRECT_ATHUB           | Correctable ECC error in ATHUB                                             |
| GPU_ECC_UNCORRECT_ATHUB         | Uncorrectable ECC error in ATHUB                                           |
| GPU_ECC_CORRECT_BIF             | Correctable ECC error in BIF                                               |
| GPU_ECC_UNCORRECT_BIF           | Uncorrectable ECC error in BIF                                             |
| GPU_ECC_CORRECT_HDP             | Correctable ECC error in HDP                                               |
| GPU_ECC_UNCORRECT_HDP           | Uncorrectable ECC error in HDP                                             |
| GPU_ECC_CORRECT_XGMI_WAFL       | Correctable ECC error in XGMI WAFL                                         |
| GPU_ECC_UNCORRECT_XGMI_WAFL     | Uncorrectable ECC error in XGMI WAFL                                       |
| GPU_ECC_CORRECT_DF              | Correctable ECC error in DF                                                |
| GPU_ECC_UNCORRECT_DF            | Uncorrectable ECC error in DF                                              |
| GPU_ECC_CORRECT_SMN             | Correctable ECC error in SMN                                               |
| GPU_ECC_UNCORRECT_SMN           | Uncorrectable ECC error in SMN                                             |
| GPU_ECC_CORRECT_SEM             | Correctable ECC error in SEM                                               |
| GPU_ECC_UNCORRECT_SEM           | Uncorrectable ECC error in SEM                                             |
| GPU_ECC_CORRECT_MP0             | Correctable ECC error in MP0                                               |
| GPU_ECC_UNCORRECT_MP0           | Uncorrectable ECC error in MP0                                             |
| GPU_ECC_CORRECT_MP1             | Correctable ECC error in MP1                                               |
| GPU_ECC_UNCORRECT_MP1           | Uncorrectable ECC error in MP1                                             |
| GPU_ECC_CORRECT_FUSE            | Correctable ECC error in FUSE                                              |
| GPU_ECC_UNCORRECT_FUSE          | Uncorrectable ECC error in FUSE                                            |
| GPU_ECC_CORRECT_UMC             | Correctable ECC error in UMC                                               |
| GPU_ECC_UNCORRECT_UMC           | Uncorrectable ECC error in UMC                                             |
| GPU_XGMI_NBR_0_NOP_TX           | NOPs sent to neighbor 0                                                    |
| GPU_XGMI_NBR_0_REQ_TX           | Outgoing requests to neighbor 0                                            |
| GPU_XGMI_NBR_0_RESP_TX          | Outgoing responses to neighbor 0                                           |
| GPU_XGMI_NBR_0_BEATS_TX         | Data beats sent to neighbor 0; Each beat represents 32 bytes              |
| GPU_XGMI_NBR_1_NOP_TX           | NOPs sent to neighbor 1                                                    |
| GPU_XGMI_NBR_1_REQ_TX           | Outgoing requests to neighbor 1                                            |
| GPU_XGMI_NBR_1_RESP_TX          | Outgoing responses to neighbor 1                                           |
| GPU_XGMI_NBR_1_BEATS_TX         | Data beats sent to neighbor 1; Each beat represents 32 bytes              |
| GPU_XGMI_NBR_0_TX_THRPUT        | Represents the number of outbound beats (each representing 32 bytes) on link 0; Throughput = BEATS/time_running * 10^9  bytes/sec |
| GPU_XGMI_NBR_1_TX_THRPUT        | Represents the number of outbound beats (each representing 32 bytes) on link 1 |
| GPU_XGMI_NBR_2_TX_THRPUT        | Represents the number of outbound beats (each representing 32 bytes) on link 2 |
| GPU_XGMI_NBR_3_TX_THRPUT        | Represents the number of outbound beats (each representing 32 bytes) on link 3 |
| GPU_XGMI_NBR_4_TX_THRPUT        | Represents the number of outbound beats (each representing 32 bytes) on link 4 |
| GPU_XGMI_NBR_5_TX_THRPUT        | Represents the number of outbound beats (each representing 32 bytes) on link 5 |
| GPU_USED_VRAM                   | Total VRAM memory used in MB                                            |
| GPU_FREE_VRAM                   | Total VRAM memory free in MB                                            |
| GPU_TOTAL_VISIBLE_VRAM          | Total available visible VRAM memory in MB                               |
| GPU_USED_VISIBLE_VRAM           | Total used VRAM memory in MB                                            |
| GPU_FREE_VISIBLE_VRAM           | Total free VRAM memory in MB                                            |
| GPU_TOTAL_GTT                   | Total GTT memory in MB                                                     |
| GPU_USED_GTT                    | Current GTT memory usage in MB                                             |
| GPU_FREE_GTT                    | Free GTT memory available in MB                                            |
| GPU_ECC_CORRECT_MCA             | Correctable ECC error in MCA                                               |
| GPU_ECC_UNCORRECT_MCA           | Uncorrectable ECC error in MCA                                             |
| GPU_ECC_CORRECT_VCN             | Correctable ECC error in VCN                                               |
| GPU_ECC_UNCORRECT_VCN           | Uncorrectable ECC error in VCN                                             |
| GPU_ECC_CORRECT_JPEG            | Correctable ECC error in JPEG                                              |
| GPU_ECC_UNCORRECT_JPEG          | Uncorrectable ECC error in JPEG                                            |
| GPU_ECC_CORRECT_IH              | Correctable ECC error in IH                                                |
| GPU_ECC_UNCORRECT_IH            | Uncorrectable ECC error in IH                                              |
| GPU_ECC_CORRECT_MPIO            | Correctable ECC error in MPIO                                              |
| GPU_ECC_UNCORRECT_MPIO          | Uncorrectable ECC error in MPIO                                            |

## GPU_CLOCK measurements

The Device Metrics Exporter `gpu_clock` metric is a common field used for exporting different types of clocks. This metric has a `clock_type` label added to the metric to differentiate the different clock types:

```json
gpu_clock{clock_type="GPU_CLOCK_TYPE_DATA"}
gpu_clock{clock_type="GPU_CLOCK_TYPE_SYSTEM"}
gpu_clock{clock_type="GPU_CLOCK_TYPE_MEMORY"}
gpu_clock{clock_type="GPU_CLOCK_TYPE_VIDEO"}
```

An example of this is shown below:

```json
gpu_clock{card_model="xxxx",clock_index="14",clock_type="GPU_CLOCK_TYPE_DATA",gpu_compute_partition_type="spx",gpu_id="3",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 22
gpu_clock{card_model="xxxx",clock_index="2",clock_type="GPU_CLOCK_TYPE_SYSTEM",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 132
gpu_clock{card_model="xxxx",clock_index="8",clock_type="GPU_CLOCK_TYPE_MEMORY",gpu_compute_partition_type="spx",gpu_id="0",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 900
gpu_clock{card_model="xxxx",clock_index="9",clock_type="GPU_CLOCK_TYPE_VIDEO",gpu_compute_partition_type="spx",gpu_id="5",gpu_partition_id="0",hostname="xxxx",serial_number="xxxx"} 29
```
