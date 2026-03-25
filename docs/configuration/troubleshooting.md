# Troubleshooting Device Metrics Exporter

This topic provides an overview of troubleshooting options for Device Metrics Exporter.

## Techsupport Collection

### K8s Techsupport Collection

Two specialized techsupport scripts are available for collecting diagnostics:

#### GPU Exporter Techsupport

The [techsupport_dump.sh](https://github.com/ROCm/device-metrics-exporter/blob/main/tools/techsupport_dump.sh) script collects GPU exporter diagnostics.

```bash
./techsupport_dump.sh -r <helm-release-name> [-k kubeconfig] [-o yaml/json] [-w] <node-name/all>
```

Options:

- `-r helm-release-name`: helm release name (required)
- `-k kubeconfig`: path to kubeconfig (default: ~/.kube/config)
- `-o yaml/json`: output format (default: json)
- `-w`: wide option

Example:

```bash
# Collect GPU exporter diagnostics for all nodes
./techsupport_dump.sh -r amd-gpu-operator -k ~/.kube/config all

# Collect for a specific node
./techsupport_dump.sh -r amd-gpu-operator node1
```

Collected diagnostics include:

- Exporter version, health, and configuration
- Pod and container logs (current and previous)
- amd-smi output (list, metric, static, firmware, partition, xgmi)
- GPU agent logs
- Kubernetes resources (nodes, events, pods, daemonsets)

#### NIC Exporter Techsupport

The [nic_techsupport_dump.sh](https://github.com/ROCm/device-metrics-exporter/blob/main/tools/nic_techsupport_dump.sh) script collects NIC exporter diagnostics. It supports both standalone helm deployments and operator-managed deployments.

```bash
./nic_techsupport_dump.sh -r <helm-release-name> [-k kubeconfig] [-o yaml/json] [-w] <node-name/all>
```

Options:

- `-r helm-release-name`: helm release name (required)
  - For standalone: use the exporter helm release name (e.g., `amd-ainic-exporter`)
  - For operator-managed: use the operator helm release name (e.g., `amd-network-operator`)
- `-k kubeconfig`: path to kubeconfig (default: ~/.kube/config)
- `-o yaml/json`: output format (default: json)
- `-w`: wide option

Examples:

```bash
# Standalone deployment
./nic_techsupport_dump.sh -r amd-ainic-exporter -k ~/.kube/config genoa3

# Operator-managed deployment
./nic_techsupport_dump.sh -r amd-network-operator -k ~/.kube/config all
```

Collected diagnostics include:

- Exporter version, health, and configuration
- Pod and container logs (current and previous)
- Metrics endpoint (`/metrics`)
- RDMA statistics (`rdma statistic -j`)
- ethtool statistics for AMD interfaces
- nicctl statistics (if available):
  - Port statistics
  - LIF statistics
  - RDMA queue-pair information and statistics
- NIC device information (infiniband, network interfaces)
- Goroutine dump for debugging
- Kubernetes resources (nodes, events, pods, daemonsets)

The script automatically:

1. Detects the namespace from the helm release
2. Finds the metrics exporter daemonset (works for both deployment types)
3. Verifies it's a NIC exporter by checking container arguments
4. Detects deployment type (standalone vs operator-managed)
5. Generates a tarball named `techsupport-nic-<timestamp>.tgz`

### Docker Techsupport Collection

```bash
docker exec -it device-metrics-exporter metrics-exporter-ts.sh
docker cp device-metrics-exporter:/var/log/amd-metrics-exporter-techsupport-<timestamp>.tar.gz .
```

### Debian Techsupport Collection

```bash
sudo metrics-exporter-ts.sh
```

Please file an issue with collected techsupport bundle on our [GitHub Issues](https://github.com/ROCm/device-metrics-exporter/issues) page

## Logs

You can view the container logs by executing the following command:

### K8s deployment

```bash
kubectl logs -n <namespace> <exporter-container-on-node>
```

### Docker deployment

```bash
docker logs device-metrics-exporter
```

### Debian deployment

```bash
sudo journalctl -xu amd-metrics-exporter
```

## Service Management for Driver and Partition Operations

When performing GPU driver unload/upgrade or partition operations, the AMD Device Metrics Exporter services must be stopped first. The specific steps vary depending on your deployment method:

- **Docker Deployment**: See [Docker Installation - Service Management](../installation/docker.md#service-management-for-driver-and-partition-operations)
- **Debian Package Deployment**: See [Debian Package Installation - Troubleshooting](../installation/deb-package.rst#troubleshooting)

**Why is this necessary?**

The AMD Device Metrics Exporter maintains active connections to GPU devices and drivers.

## Common Issues

This section describes common issues with AMD Device Metrics Exporter

1. Port conflicts:
   - Verify port 5000 is available
   - Configure an alternate port through the configuration file

2. Device access:
   - Ensure proper permissions on `/dev/dri` and `/dev/kfd`
   - Verify ROCm is properly installed

3. Metric collection issues:
   - Check GPU driver status
   - Verify ROCm version compatibility

4. App Armor blocking Profiler:

```bash
# dmesg  | grep -3 rocpctl
root@genoa3:~/praveen# dmesg | grep -3 rocpctl
[97478.776746] cni0: port 10(veth9ec08a32) entered forwarding state
[113647.022518] audit: type=1400 audit(1765338835.280:130): apparmor="DENIED" operation="open" class="file" profile="ubuntu_pro_apt_news" name="/opt/rocm-7.1.1/lib/" pid=801116 comm="python3" requested_mask="r" denied_mask="r" fsuid=0 ouid=0
[113647.029634] audit: type=1400 audit(1765338835.287:131): apparmor="DENIED" operation="open" class="file" profile="ubuntu_pro_esm_cache" name="/opt/rocm-7.1.1/lib/" pid=801117 comm="python3" requested_mask="r" denied_mask="r" fsuid=0 ouid=0
[172955.500614] rocpctl[1200455]: segfault at 736d ip 00007279ae77f98e sp 00007ffea7539590 error 4 in librocprofiler-sdk.so.1.0.0[7279ae22a000+69e000] likely on CPU 42 (core 82, socket 0)
[172955.500630] Code: 40 31 d2 48 8b 5d 38 4c 8b 00 4c 89 c0 48 f7 f6 4c 8d 2c d3 49 89 d3 4d 8b 55 00 4d 85 d2 0f 84 9d 00 00 00 49 8b 02 4d 89 d1 <48> 8b 48 08 4c 39 c1 74 28 48 8b 38 48 85 ff 0f 84 82 00 00 00 48
[172955.598083] amdgpu: Freeing queue vital buffer 0x727888200000, queue evicted
[172955.598090] amdgpu: Freeing queue vital buffer 0x727890200000, queue evicted
```

  **Solution** : Disable App Armor or create custom profile to allow `rocpctl` access to /opt/rocm-7.1.1/lib/
