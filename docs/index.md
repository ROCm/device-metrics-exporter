# AMD Device Metrics Exporter

AMD Device Metrics Exporter enables Prometheus-format metrics collection for AMD GPUs and NICs in HPC and AI environments. It provides detailed telemetry, including temperature, utilization, memory usage, and power consumption. This tool includes the following features:

## Features

- Prometheus-compatible metrics endpoint
- Rich GPU telemetry data including:
  - Temperature monitoring
  - Utilization metrics
  - Memory usage statistics
  - Power consumption data
  - PCIe bandwidth metrics
  - Performance metrics
- Kubernetes integration via Helm chart
- Slurm integration support
- Configurable service ports
- Container-based deployment
- Beta: Kubernetes Dynamic Resource Allocation (DRA) GPU claim support (Kubernetes 1.34+)

## GPU Metrics

### Requirements

- Ubuntu 22.04, 24.04
- Docker (or compatible container runtime)

### Compatibility Matrix

| Rocm Version | Driver Version | Exporter Image Version | Platform     |
|--------------|----------------|------------------------|--------------|
| 6.2.x        | 6.8.5          | v1.0.0                 | MI2xx, MI3xx |
| 6.3.x        | 6.10.5         | v1.2.0                 | MI2xx, MI3xx |
| 6.4.x        | 6.12.12        | v1.3.1                 | MI2xx, MI3xx |
| 7.0.x        | 6.14.14        | v1.4.0.1               | MI2xx, MI3xx |
| 7.1.x        | 6.16.6         | v1.4.2                 | MI2xx, MI3xx |
| TBD          | TBD            | v1.5.0 (dev)           | MI2xx, MI3xx |

### Available Metrics

Device Metrics Exporter provides extensive GPU metrics including:

- Temperature metrics
  - Edge temperature
  - Junction temperature
  - Memory temperature
  - HBM temperature
- Performance metrics
  - GPU utilization
  - Memory utilization
  - Clock speeds
- Power metrics
  - Current power usage
  - Average power usage
  - Energy consumption
- Memory statistics
  - Total VRAM
  - Used VRAM
  - Free VRAM
- PCIe metrics
  - Bandwidth
  - Link speed
  - Error counts

See [GPU Metrics List](./configuration/metricslist.md) for the complete list.

## NIC Metrics

### Requirements

- Ubuntu 22.04, 24.04
- Docker (or compatible container runtime)
- AMD NICs with supported drivers (AINIC)

### Compatibility Matrix

| AINIC Firmware Version | Exporter Image Version | Supported NICs |
|---------------------------------------|------------------------|----------------|
| N/A (host nicctl)      | nic-v1.0.0             | Pollara 400    |
| N/A (host nicctl)      | nic-v1.0.1             | Pollara 400    |
| 1.117.5-a-56           | nic-v1.1.0             | Pollara 400    |
| 1.117.5-a-56<br>1.117.6-a-77          | nic-v1.2.0             | Pollara 400    |

### Available Metrics

Device Metrics Exporter provides extensive NIC metrics including:

- Port statistics
  - Frame counts (RX/TX)
  - Octet counts (RX/TX)
  - Pause and priority frames
  - FCS and other error counts
- LIF statistics
  - Unicast/multicast/broadcast packets
  - DMA errors
  - Drop counts
- Queue Pair (QP) statistics
  - Send Queue requester metrics
  - Receive Queue responder metrics
  - QCN congestion metrics
- RDMA statistics
  - Tx/Rx unicast packets
  - CNP/ECN packets
  - Request/response errors
- Ethtool statistics
  - Packet and byte counts
  - Frame size distribution
  - Per-queue drop counts

See [AINIC Metrics List](./configuration/network-metricslist.md) for the complete list.
