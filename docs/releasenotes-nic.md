# NIC Exporter Release Notes

## nic-v1.2.0

### New Features

- **LIF-Level Aggregated Queue-Pair Metrics**: Introduced aggregated QP metrics at the LIF level, reducing Prometheus metric cardinality and lowering Exporter and Prometheus CPU/memory overhead compared to enabling per-QP metrics.

### Improvements

- Image Size Optimization: Docker image size reduced by 57% (from 903MB to 386MB) by stripping debug symbols from nicctl binary and Go binaries
- **Per-QP Metrics Disabled by Default**: Per-QP metrics (metrics prefixed with `QP_*`) are now disabled by default for reduced cardinality.
- **Per-QP Metrics Debug Mode**: Added support for retrieving per-QP metrics via URL parameter `/metrics?debug=qp` without modifying configuration files.

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
