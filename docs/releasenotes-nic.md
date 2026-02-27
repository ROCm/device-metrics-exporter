# NIC Exporter Release Notes

## nic-v1.1.0

- nicctl Bundling: `nicctl` is now bundled within the NIC exporter Docker image, eliminating the need to mount nicctl from host

## nic-v1.0.1

- Improved RDMA statistics collection, reducing the previously observed latency by several folds compared to the earlier release `nic-v1.0.0`

## nic-v1.0.0

- **NIC Metrics Exporter for Prometheus**
  - Real-time metrics exporter for AMD NICs.
  - Supports both Docker and Debian installations.
  - Collects metrics using nicctl, rdma, and ethtool, and works across hypervisor, VM, and bare-metal environments.
  - Optimized RDMA stats, reducing the previously observed latency by multiple folds compared to the beta release.
