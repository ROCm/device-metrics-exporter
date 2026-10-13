# metricsclient Tool - Usage Guide

**Location:** `tools/metricsclient/`

**Purpose:** Command-line utility for querying GPU metrics, health status, and injecting mock ECC errors for testing.

---

## Overview

metricsclient is a Go-based CLI tool that provides:
- GPU health status queries (via DME's gRPC health service)
- Mock ECC error injection for testing
- Direct gpuagent queries
- Kubernetes integration queries
- Device mapping utilities

---

## Installation

Built as part of the exporter build process:

```bash
# Inside docker-shell
make all  # Builds metricsclient along with exporter

# Binary location
./bin/metricsclient
```

---

## Default Socket Paths

**Metrics Service Socket:** `unix:///var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket`
**GPUAgent Socket:** `/var/run/gpuagent.sock`

---

## Commands

### 1. List All GPU Health States (Default)

Query the DME metrics service and list all GPU health states.

```bash
metricsclient

# OR explicitly
metricsclient list

# JSON output
metricsclient --json
metricsclient list --json
```

**Output:**
```
ID         UUID                                     Health     Associated Workload
------------------------------------------------
0          550e8400-e29b-41d4-a716-446655440000     healthy    [pod1/container1]
1          550e8400-e29b-41d4-a716-446655440001     unhealthy  []
------------------------------------------------
```

**JSON Output:**
```json
{
  "GPUState": [
    {
      "ID": "0",
      "UUID": "550e8400-e29b-41d4-a716-446655440000",
      "Health": "healthy",
      "AssociatedWorkload": ["pod1/container1"],
      "Device": "0000:01:00.0"
    }
  ]
}
```

### 2. Get Specific GPU Health State

Query health status of a specific GPU by ID.

```bash
metricsclient get --id 0

# JSON output
metricsclient get --id 0 --json
```

### 3. Set Mock ECC Error (Testing)

Inject mock ECC errors using JSON configuration file.

**CRITICAL:** This uses a **JSON file** for error configuration, not CLI flags.

#### JSON File Format

**File:** `/tmp/ecc-error.json`

```json
{
  "ID": "0",
  "Fields": [
    "GPU_ECC_UNCORRECT_UMC",
    "GPU_ECC_UNCORRECT_SDMA"
  ],
  "Counts": [5, 1]
}
```

**Field Mapping:**
- `ID`: GPU ID (string, e.g., "0", "1")
- `Fields`: Array of ECC error field names (must match proto enum exactly)
- `Counts`: Array of error counts (uint32)
  - Non-zero: Set error count (adds to existing, doesn't override)
  - Zero: Clear error count (reset to 0)

#### Usage

```bash
# Inject errors from JSON file
metricsclient --ecc-file-path /tmp/ecc-error.json

# Custom socket path
metricsclient --socket unix:///custom/path/socket.sock --ecc-file-path /tmp/ecc-error.json
```

#### Valid ECC Error Field Names

**Correctable Errors:**
- `GPU_ECC_CORRECT_SDMA`
- `GPU_ECC_CORRECT_GFX`
- `GPU_ECC_CORRECT_MMHUB`
- `GPU_ECC_CORRECT_ATHUB`
- `GPU_ECC_CORRECT_BIF`
- `GPU_ECC_CORRECT_HDP`
- `GPU_ECC_CORRECT_XGMI_WAFL`
- `GPU_ECC_CORRECT_DF`
- `GPU_ECC_CORRECT_SMN`
- `GPU_ECC_CORRECT_SEM`
- `GPU_ECC_CORRECT_MP0`
- `GPU_ECC_CORRECT_MP1`
- `GPU_ECC_CORRECT_FUSE`
- `GPU_ECC_CORRECT_UMC`
- `GPU_ECC_CORRECT_MCA`
- `GPU_ECC_CORRECT_VCN`
- `GPU_ECC_CORRECT_JPEG`
- `GPU_ECC_CORRECT_IH`
- `GPU_ECC_CORRECT_MPIO`

**Uncorrectable Errors:**
- `GPU_ECC_UNCORRECT_SDMA`
- `GPU_ECC_UNCORRECT_GFX`
- `GPU_ECC_UNCORRECT_MMHUB`
- `GPU_ECC_UNCORRECT_ATHUB`
- `GPU_ECC_UNCORRECT_BIF`
- `GPU_ECC_UNCORRECT_HDP`
- `GPU_ECC_UNCORRECT_XGMI_WAFL`
- `GPU_ECC_UNCORRECT_DF`
- `GPU_ECC_UNCORRECT_SMN`
- `GPU_ECC_UNCORRECT_SEM`
- `GPU_ECC_UNCORRECT_MP0`
- `GPU_ECC_UNCORRECT_MP1`
- `GPU_ECC_UNCORRECT_FUSE`
- `GPU_ECC_UNCORRECT_UMC`
- `GPU_ECC_UNCORRECT_MCA`
- `GPU_ECC_UNCORRECT_VCN`
- `GPU_ECC_UNCORRECT_JPEG`
- `GPU_ECC_UNCORRECT_IH`
- `GPU_ECC_UNCORRECT_MPIO`

#### Example: Inject Uncorrectable UMC Error

**1. Create JSON file:**
```bash
cat > /tmp/ecc-umc-error.json <<EOF
{
  "ID": "0",
  "Fields": ["GPU_ECC_UNCORRECT_UMC"],
  "Counts": [1]
}
EOF
```

**2. Inject error:**
```bash
metricsclient --ecc-file-path /tmp/ecc-umc-error.json
```

**3. Verify:**
```bash
metricsclient list
# GPU 0 should now show "unhealthy" (threshold = 0 for uncorrectable)
```

**4. Clear error:**
```bash
cat > /tmp/ecc-clear.json <<EOF
{
  "ID": "0",
  "Fields": ["GPU_ECC_UNCORRECT_UMC"],
  "Counts": [0]
}
EOF

metricsclient --ecc-file-path /tmp/ecc-clear.json
```

#### Example: Inject Multiple Errors

```bash
cat > /tmp/ecc-multi.json <<EOF
{
  "ID": "0",
  "Fields": [
    "GPU_ECC_UNCORRECT_SDMA",
    "GPU_ECC_UNCORRECT_GFX",
    "GPU_ECC_CORRECT_UMC"
  ],
  "Counts": [1, 2, 100]
}
EOF

metricsclient --ecc-file-path /tmp/ecc-multi.json
```

**Output:**
```json
{
  "ID": "0",
  "Fields": ["GPU_ECC_UNCORRECT_SDMA", "GPU_ECC_UNCORRECT_GFX", "GPU_ECC_CORRECT_UMC"]
}
```

### 4. Query GPUAgent Directly

Connect directly to gpuagent (bypasses DME metrics service).

```bash
# Using socket (default)
metricsclient gpuctl

# Using IP:port
metricsclient gpuctl --port 50061

# Custom socket path
metricsclient gpuctl --socket /custom/path/gpuagent.sock

# JSON output
metricsclient gpuctl --json
```

**Output (non-JSON):**
```
----------------------------------------
Index : 0
Spec  : {"Id":"...","Uuid":"..."}
Status: {"Index":0,"PCIeBusId":"0000:01:00.0"}
Stats : {"EdgeTemperature":45,"PowerUsage":250}
----------------------------------------
```

### 5. Device Mapping

Show logical GPU device map (render IDs to device names).

```bash
metricsclient device-map
```

**Output:**
```
Logical Device Map
Render ID [128] -> Device Name [card0]
Render ID [129] -> Device Name [card1]
```

### 6. Kubernetes Integration

#### Get Pod Resources

Query kubelet for pod resource allocations (requires kubelet socket).

```bash
metricsclient pod-resources
```

**Output:**
```
dev:ns/pod/container [{GPU-UUID}pod-name/namespace/container-name]
```

#### Get Node Pods

List all pods scheduled on current K8s node with labels.

```bash
metricsclient node-pods

# Custom kubeconfig
metricsclient node-pods --kube-config /path/to/kubeconfig
```

**Output:**
```
Pods scheduled on node worker-1:
- default/gpu-pod (Phase: Running)
  Labels:
  app=gpu-workload
  gpu.amd.com/allocated=true
  UID: 12345678-1234-1234-1234-123456789012
```

#### Get Node Labels

Retrieve K8s node labels.

```bash
metricsclient node-labels

# Custom kubeconfig
metricsclient node-labels --kube-config /path/to/kubeconfig
```

### 7. Setup Mock Inband RAS

Create mock inband RAS error_list file for testing.

```bash
# Using socket (default)
metricsclient setup-mock-inbandras

# Using IP:port
metricsclient setup-mock-inbandras --port 50061
```

**Creates:** `/mockdata/inband-ras/error_list`

**Content:**
```json
{
  "cper": [
    {
      "gpu": "550e8400-e29b-41d4-a716-446655440000",
      "afid": []
    }
  ]
}
```

> **Note:** there is no stream subcommand. High-resolution mode has no external
> push stream — the GPUWatch feed updates the in-memory snapshot store and the
> data is retrieved only by scraping `/metrics` (the single data path).

---

## Global Flags

All commands support these persistent flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--socket` | Metrics gRPC socket path | `unix:///var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket` |
| `--json` | Output in JSON format | `false` |
| `--kube-config` | Kubernetes config file path | `""` (auto-detect) |
| `--ecc-file-path` | Path to JSON error config file | `""` |

---

## Proto Service Definition

**File:** [pkg/amdgpu/proto/gpumetricssvc.proto](../../pkg/amdgpu/proto/gpumetricssvc.proto)

```protobuf
message GPUErrorRequest {
    string ID = 1;                  // GPU ID
    repeated string Fields = 2;     // Error field names
    repeated uint32 Counts = 3;     // Error counts (0 = clear)
}

service MetricsService {
    rpc GetGPUState(GPUGetRequest) returns (GPUStateResponse) {}
    rpc List(google.protobuf.Empty) returns (GPUStateResponse) {}
    rpc SetError(GPUErrorRequest) returns (GPUErrorResponse) {}
}
```

---

## Testing Workflow

### Test Health Monitoring with Mock ECC Injection

**1. Verify baseline health:**
```bash
metricsclient list
# All GPUs should be "healthy"
```

**2. Inject uncorrectable error:**
```bash
cat > /tmp/inject-error.json <<EOF
{
  "ID": "0",
  "Fields": ["GPU_ECC_UNCORRECT_UMC"],
  "Counts": [1]
}
EOF

metricsclient --ecc-file-path /tmp/inject-error.json
```

**3. Wait for health monitor cycle (30s default):**
```bash
sleep 35
```

**4. Verify unhealthy state:**
```bash
metricsclient list
# GPU 0 should now be "unhealthy"
```

**5. Verify K8s integration (if deployed in K8s):**
```bash
metricsclient node-labels | grep unhealthy
# Should see: gpu.amd.com/gpu.0.unhealthy=true
```

**6. Clear error and restore healthy:**
```bash
cat > /tmp/clear-error.json <<EOF
{
  "ID": "0",
  "Fields": ["GPU_ECC_UNCORRECT_UMC"],
  "Counts": [0]
}
EOF

metricsclient --ecc-file-path /tmp/clear-error.json
sleep 35
metricsclient list
# GPU 0 should be "healthy" again
```

---

## Important Notes

1. **Mock Errors Only** - metricsclient only injects mock errors into DME's internal state, NOT real hardware errors
2. **Reversible** - Mock errors can be cleared by setting count to 0
3. **No Hardware Impact** - Does not affect GPU hardware, driver, or running workloads
4. **Testing Only** - Use for CI/CD, health service testing, alert system validation
5. **Real Hardware Errors** - For real error injection, use AMDGPURAS tool (see [gpu-metrics-details.md](gpu-metrics-details.md))

---

## Error Scenarios

### Error: Socket Connection Failed

```
failed to connect: dial unix /var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket: connect: no such file or directory
```

**Solution:**
- Verify DME is running and health service enabled
- Check socket path: `ls -la /var/lib/amd-metrics-exporter/`
- Use correct socket path with `--socket` flag

### Error: Invalid Field Name

```
SetError call failed: unknown field name
```

**Solution:**
- Verify field name matches proto enum exactly
- Use correct prefix: `GPU_ECC_CORRECT_*` or `GPU_ECC_UNCORRECT_*`
- Check [Valid ECC Error Field Names](#valid-ecc-error-field-names) section

### Error: JSON Parse Failed

```
err: invalid character '}' looking for beginning of value
```

**Solution:**
- Validate JSON syntax: `jq . /tmp/ecc-error.json`
- Ensure array lengths match: `Fields` and `Counts` must have same length
- Check for trailing commas, missing quotes

---

## Related Documentation

- **GPU Metrics Details:** [gpu-metrics-details.md](gpu-metrics-details.md) - ECC error types, real HW injection
- **Health Monitoring:** [health-monitoring.md](health-monitoring.md) - Health service architecture
- **Troubleshooting:** [troubleshooting.md](troubleshooting.md) - Debugging health issues

---

**Last Updated:** 2026-04-06
