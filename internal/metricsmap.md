# Internal Mapping of Field on each service

Platform if specified only applies to that specific model, else applies to all

|  Exporter Metric                                           | GPU Agent                               |          amd-smi                            |    Platform     |
|------------------------------------------------------------|---------------------------------------|-----------------------------------------------|--------------|
| GPU_NODES_TOTAL                                            |                                       |                                               |              |
| GPU_PACKAGE_POWER                                          | stats.PackagePower                      |  power_info.current_socket_power            |             |               
| GPU_AVERAGE_PACKAGE_POWER                                  | stats.AvgPackagePower                   |  power_info.average_socket_power            |        |               
| GPU_EDGE_TEMPERATURE                                       | stats.temperature.edge_temperature      |  temp.edge                                  |             Mi2xx |
| GPU_JUNCTION_TEMPERATURE                                   | stats.temperature.junction_temperature  | temp.junction/hostspot                      |  Mi3xx      |
| GPU_MEMORY_TEMPERATURE                                     | stats.temperature.memory_temperature    |   temp.memory                               |        |
| GPU_HBM_TEMPERATURE                                        | stats.temperature.hbm_temperature[i]    |   temp.hbm[i]                               |             |
| GPU_GFX_ACTIVITY (Applicable for unpartitioned GPU)        | stats.usage.gfx_activity                |  usage.gfx_activity                         |     |
| GPU_UMC_ACTIVITY                                           | stats.usage.umc_activity                | usage.umc_activity           |        |
| GPU_MMA_ACTIVITY                                           | stats.usage.mm_activity                 | usage.mm_activity           |        |
| GPU_VCN_ACTIVITY                                           | stats.usage.vcn_activity[i]             | metrics_info.vcn_activity [i]          |        |
| GPU_JPEG_ACTIVITY                                          | stats.usage.jpeg_activity[i]            | metrics_info.jpeg_activity[i]           |        |
| GPU_VOLTAGE                                                | stats.voltage.voltage                   | power_info.soc_voltage          |             |
| GPU_GFX_VOLTAGE                                            | stats.voltage.gfx_voltage               | power_info.gfx_voltage           |             |
| GPU_MEMORY_VOLTAGE                                         | stats.voltage.memory_voltage            | power_info.mem_voltage           |             |
| PCIE_SPEED                                                 | status.pcie_status->speed               |  pcie_metric.pcie_speed/1000          |             |        |
| PCIE_MAX_SPEED                                             | status.pcie_status->max_speed            | pcie_static.max_pcie_speed/1000           |             |        |
| PCIE_BANDWIDTH                                             | status.pcie_status->bandwidth            | pcie_metric.pcie_bandwidth           |             |        |
| GPU_ENERGY_CONSUMED                                        | stats.energy_consumed                    | energy.total_energy_consumption           |             |        |
| PCIE_REPLAY_COUNT                                          | stats->pcie_stats.replay_count           | pcie_info.pcie_metric.pcie_replay_count           |             |        |
| PCIE_RECOVERY_COUNT                                        | stats->pcie_stats.recovery_count         | pcie_info.pcie_metric.pcie_l0_to_recovery_count           |             |        |
| PCIE_REPLAY_ROLLOVER_COUNT                                 | stats->pcie_stats.replay_rollover_count  | pcie_info.pcie_metric.pcie_replay_roll_over_count           |             |        |
| PCIE_NACK_SENT_COUNT                                       | stats->pcie_stats.nack_sent_count        |  pcie_info.pcie_metric.pcie_nak_sent_count          |             |        |
| PCIE_NAC_RECEIVED_COUNT                                    | stats->pcie_stats.nack_received_count    | pcie_info.pcie_metric.pcie_nak_received_count           |             |        |
| GPU_CLOCK                                                  | status.clock_status[i] SYSTEM            |  metrics_info->current_gfxclks[i]          |             |        |
|                                                            | status.clock_status[i] MEMORY            | metrics_info->current_uclk           |             |        |
|                                                            | status.clock_status[i] VIDEO             |  metrics_info->current_vclk0s[i]          |             |        |
|                                                            | status.clock_status[i] DATA              |  metrics_info->current_dclk0s[i]          |             |        |
| GPU_POWER_USAGE                                            | stats.power_usage                        | gpu_metrics.current_socket_power           | MI3xx            |        |
|                                                            | stats.power_usage                        | gpu_metrics.average_socket_power           | MI2xx            |        |
| GPU_TOTAL_VRAM                                             | status.vramstatus.size                   | mem_usage.total_vram           |             |        |
| GPU_ECC_CORRECT_TOTAL                                      |             |            |             |        
| GPU_ECC_UNCORRECT_TOTAL                                    |             |            |             |        
| GPU_ECC_CORRECT_SDMA                                       |             |            |             |        
| GPU_ECC_UNCORRECT_SDMA                                     |             |            |             |        
| GPU_ECC_CORRECT_GFX                                        |             |            |             |        
| GPU_ECC_UNCORRECT_GFX                                      |             |            |             |        
| GPU_ECC_CORRECT_MMHUB                                      |             |            |             |        
| GPU_ECC_UNCORRECT_MMHUB                                    |             |            |             |        
| GPU_ECC_CORRECT_ATHUB                                      |             |            |             |        
| GPU_ECC_UNCORRECT_ATHUB                                    |             |            |             |        
| GPU_ECC_CORRECT_BIF                                        |             |            |             |        
| GPU_ECC_UNCORRECT_BIF                                      |             |            |             |        
| GPU_ECC_CORRECT_HDP                                        |             |            |             |        
| GPU_ECC_UNCORRECT_HDP                                      |             |            |             |        
| GPU_ECC_CORRECT_XGMI_WAFL                                  |             |            |             |        
| GPU_ECC_UNCORRECT_XGMI_WAFL                                |             |            |             |        
| GPU_ECC_CORRECT_DF                                         |             |            |             |        
| GPU_ECC_UNCORRECT_DF                                       |             |            |             |        
| GPU_ECC_CORRECT_SMN                                        |             |            |             |        
| GPU_ECC_UNCORRECT_SMN                                      |             |            |             |        
| GPU_ECC_CORRECT_SEM                                        |             |            |             |        
| GPU_ECC_UNCORRECT_SEM                                      |             |            |             |        
| GPU_ECC_CORRECT_MP0                                        |             |            |             |        
| GPU_ECC_UNCORRECT_MP0                                      |             |            |             |        
| GPU_ECC_CORRECT_MP1                                        |             |            |             |        
| GPU_ECC_UNCORRECT_MP1                                      |             |            |             |        
| GPU_ECC_CORRECT_FUSE                                       |             |            |             |        
| GPU_ECC_UNCORRECT_FUSE                                     |             |            |             |        
| GPU_ECC_CORRECT_UMC                                        |             |            |             |        
| GPU_ECC_UNCORRECT_UMC                                      |             |            |             |        
| GPU_XGMI_NBR_0_NOP_TX                                      |             |            |             |        
| GPU_XGMI_NBR_0_REQ_TX                                      |             |            |             |        
| GPU_XGMI_NBR_0_RESP_TX                                     |             |            |             |        
| GPU_XGMI_NBR_0_BEATS_TX                                    |             |            |             |        
| GPU_XGMI_NBR_1_NOP_TX                                      |             |            |             |        
| GPU_XGMI_NBR_1_REQ_TX                                      |             |            |             |        
| GPU_XGMI_NBR_1_RESP_TX                                     |             |            |             |        
| GPU_XGMI_NBR_1_BEATS_TX                                    |             |            |             |        
| GPU_XGMI_NBR_0_TX_THRPUT                                   |             |            |             |        
| GPU_XGMI_NBR_1_TX_THRPUT                                   |             |            |             |        
| GPU_XGMI_NBR_2_TX_THRPUT                                   |             |            |             |        
| GPU_XGMI_NBR_3_TX_THRPUT                                   |             |            |             |        
| GPU_XGMI_NBR_4_TX_THRPUT                                   |             |            |             |        
| GPU_XGMI_NBR_5_TX_THRPUT                                   |             |            |             |        
| GPU_XGMI_LINK_RX                                           |             |            |             |        
| GPU_XGMI_LINK_TX                                           |             |            |             |        
| GPU_USED_VRAM                                              | driver file read (smi-bug)            |            |             |
| GPU_FREE_VRAM                                              | sub(total - used)            |            |             |
| GPU_TOTAL_VISIBLE_VRAM                                     | stats.vram_usage.total_visible_vram            | mem_usage.total_visible_vram           |        |
| GPU_USED_VISIBLE_VRAM                                      | stats.vram_usage.used_visible_vram            |  mem_usage.used_visible_vram          |        |
| GPU_FREE_VISIBLE_VRAM                                      | stats.vram_usage.free_visible_vram            |  mem_usage.free_visible_vram          |        |
| GPU_TOTAL_GTT                                              | stats.vram_usage.total_gtt            | mem_usage.total_gtt           |             |
| GPU_USED_GTT                                               | stats.vram_usage.used_gtt            | mem_usage.used_gtt           |             |
| GPU_FREE_GTT                                               | stats.vram_usage.free_gtt            | mem_usage.free_gtt           |             |
| GPU_ECC_CORRECT_MCA                                        |             |            |             |
| GPU_ECC_UNCORRECT_MCA                                      |             |            |             |
| GPU_ECC_CORRECT_VCN                                        |             |            |             |
| GPU_ECC_UNCORRECT_VCN                                      |             |            |             |
| GPU_ECC_CORRECT_JPEG                                       |             |            |             |
| GPU_ECC_UNCORRECT_JPEG                                     |             |            |             |
| GPU_ECC_CORRECT_IH                                         |             |            |             |
| GPU_ECC_UNCORRECT_IH                                       |             |            |             |
| GPU_ECC_CORRECT_MPIO                                       |             |            |             |
| GPU_ECC_UNCORRECT_MPIO                                     |             |            |             |
| GPU_CURRENT_ACCUMULATED_COUNTER                            | stats->violation_stats.current_accumulated_counter | metrics_info.accumulation_counter             |            |             
| GPU_VIOLATION_PROCESSOR_HOT_RESIDENCY_ACCUMULATE           | stats->violation_stats.processor_hot_residency_accumulated | metrics_info.prochot_residency_acc            |            |             
| GPU_VIOLATION_PPT_RESIDENCY_ACCUMULATED                    | stats->violation_stats.ppt_residency_accumulated | metrics_info.ppt_residency_acc            |            |             
| GPU_VIOLATION_SOCKET_THERMAL_RESIDENCY_ACCUMULAT           | stats->violation_stats.socket_thermal_residency_accumulated | metrics_info.socket_thm_residency_acc            |            |             
| GPU_VIOLATION_VR_THERMAL_RESIDENCY_ACCUMULATED             | stats->violation_stats.vr_thermal_residency_accumulated |metrics_info.vr_thm_residency_acc            |            |             
| GPU_VIOLATION_HBM_THERMAL_RESIDENCY_ACCUMULATED            | stats->violation_stats.hbm_thermal_residency_accumulated | metrics_info.hbm_thm_residency_acc            |            |             
| GPU_GFX_BUSY_INSTANTANEOUS                                 | stats.usage.gfx_busy_inst |  usage.gfx_busy_inst.xcp_[partition_id]
| GPU_VCN_BUSY_INSTANTANEOUS                                 | stats.usage.vcn_busy_inst |  usage.vcn_busy_inst.xcp_[partition_id]
| GPU_JPEG_BUSY_INSTANTANEOUS                                | stats.usage.jpeg_busy_inst |  usage.jpeg_busy_inst.xcp_[partition_id]
|                                                            |             |            |             |

node_id of a gpu:
```bash
 amd-smi list -e -g 0 | grep -i node_id
     NODE_ID: 8
```

used_vram file : /sys/class/kfd/kfd/topology/nodes", nodeid, "mem_banks/0/used_memory
