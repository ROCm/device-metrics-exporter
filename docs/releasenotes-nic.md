# NIC Exporter Release Notes

## nic-v1.2.0

### New Features

- **LIF-Level Aggregated Queue-Pair Metrics**: Introduced aggregated QP metrics at the LIF level, reducing Prometheus metric cardinality and lowering Exporter and Prometheus CPU/memory overhead compared to enabling per-QP metrics.
- **Ethernet Frame Priority Pause Counters**: Added `ETH_FRAMES_RX_PRIPAUSE` and `ETH_FRAMES_TX_PRIPAUSE` ethtool metrics for monitoring priority-level flow control pause frames.
- **RS-FEC Uncorrectable Word Metric**: Added `NIC_PORT_STATS_RSFEC_UNCORRECTABLE_WORD` metric (from the `RSFEC_UNCORRECTABLE_WORD` port statistic) for monitoring forward error correction failures.
- **NIC Techsupport Script**: Added `nic_techsupport_dump.sh` for collecting NIC diagnostics including nicctl, ethtool, and RDMA stats. Supports both standalone Helm deployments and operator-managed Kubernetes deployments.
- **Helm Chart Support**: Helm charts are now published for the NIC exporter, enabling Kubernetes-native deployment and configuration.

### Improvements

- **Image Size Optimization**: Docker image size reduced by 57% (from 903MB to 386MB) by stripping debug symbols from nicctl binary and Go binaries.
- **Per-QP Metrics Disabled by Default**: Per-QP metrics (metrics prefixed with `QP_*`) are now disabled by default for reduced cardinality.
- **Per-QP Metrics Debug Mode**: Added support for retrieving per-QP metrics via URL parameter `/metrics?debug=qp` without modifying configuration files.
- **CRI API Migration**: Replaced `crictl` with CRI API for container metadata collection, improving reliability and removing external tool dependency.
- **Ethtool Priority Field Naming Standardization**: Priority field names now use underscore format (`PRI_0` through `PRI_7`) for better readability.
  - Affected fields: `ETH_FRAMES_RX_PRI_*`, `ETH_FRAMES_TX_PRI_*`
  - Old format (`PRI0`, `PRI1`, etc.) maintained as deprecated aliases for backward compatibility
  - Existing configurations using old format will continue to work
  - **Migration recommended**: Backward compatibility support for old naming will be removed in a future release

### Bug Fixes

- Fixed duplicate Prometheus label registration crash when workload labels (pod, namespace, container) overlapped with config-defined labels.
- Fixed `intf-alias` label population for NIC interfaces.
- Aligned Helm chart volume mounts with network-operator: removed unused NIC-specific mounts and added missing `/lib/modules`.

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
