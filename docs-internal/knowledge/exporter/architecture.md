# Architecture - AMD Device Metrics Exporter

## Overview

AMD Device Metrics Exporter (DME) is a Prometheus metrics exporter for AMD GPU, NIC, and UAL devices. It follows a three-tier architecture with modular clients for different device types.

---

## Three-Tier Architecture

```
HTTP /metrics → MetricsHandler
                      ↓
      ┌───────────────┼───────────────┐
      ↓               ↓               ↓
  GPUAgent Lib    NIC Agent      (scheduler
  (GPU Client,     (CLI:          clients:
   UAL Client)   nicctl/ethtool/  K8s/SLURM)
      ↓            rdma)
   gpuagent
   (gRPC)
      ↓
  ┌───┴────┐
amd-smi   UAL driver
library   (direct)
  ↓          ↓
amdgpu    UAL
driver    hardware
```

**Key:** Both GPU and UAL clients in GPUAgent Library communicate with same gpuagent instance via gRPC.

---

## Component Details

### 1. Prometheus HTTP Server

**Port:** 5000 (GPU), 5001 (NIC)

**Endpoints:**
- `/metrics` - Prometheus metrics (filtered by config.json)
- `/gpumetrics` - Full GPU metrics (JSON payload, unfiltered)
- `/inbandraserrors` - In-band RAS errors (JSON payload)

**File:** [pkg/exporter/exporter.go](../../pkg/exporter/exporter.go), [pkg/exporter/npd.go](../../pkg/exporter/npd.go)

### 2. MetricsHandler (Orchestrator)

**Responsibilities:**
- Coordinates metrics collection from all clients
- Manages Prometheus registry
- Handles config reload (3-second debounce)
- Routes requests to appropriate clients

**File:** [pkg/exporter/exporter.go](../../pkg/exporter/exporter.go)

### 3. GPUAgent Library

**Structure:**
- Contains **two sub-clients**:
  - **GPU Client** - GPU metrics and management
  - **UAL/IFOE Client** - UAL metrics and management (codebase may still reference IFOE)
- Both communicate with **same gpuagent instance** via gRPC

**Files:**
- [pkg/amdgpu/gpuagent/gpuagent.go](../../pkg/amdgpu/gpuagent/gpuagent.go) - Main client & monitor
- [pkg/amdgpu/gpuagent/gpuagent_gpu.go](../../pkg/amdgpu/gpuagent/gpuagent_gpu.go) - GPU metrics client
- [pkg/amdgpu/gpuagent/gpuagent_ifoe.go](../../pkg/amdgpu/gpuagent/gpuagent_ifoe.go) - UAL metrics client
- [pkg/amdgpu/gpuagent/gpuagent_health.go](../../pkg/amdgpu/gpuagent/gpuagent_health.go) - Health logic

### 4. NIC Agent Client

**CLI-Based Architecture:**
- Uses system tools instead of direct system calls
- Three independent clients: NICCtl, Ethtool, RDMA Stats

**Files:**
- [pkg/amdnic/nicagent/nicagent.go](../../pkg/amdnic/nicagent/nicagent.go) - Main coordinator
- [pkg/amdnic/nicagent/nicctl_client.go](../../pkg/amdnic/nicagent/nicctl_client.go) - NICCtl client
- [pkg/amdnic/nicagent/ethtool_client.go](../../pkg/amdnic/nicagent/ethtool_client.go) - Ethtool client
- [pkg/amdnic/nicagent/rdma_stats_client.go](../../pkg/amdnic/nicagent/rdma_stats_client.go) - RDMA client

### 5. Scheduler Clients (Optional)

**Kubernetes:**
- gRPC to kubelet's PodResources endpoint
- Socket: `/var/lib/kubelet/pod-resources/kubelet.sock`
- File: [pkg/exporter/scheduler/k8s.go](../../pkg/exporter/scheduler/k8s.go)

**SLURM:**
- File system watcher on `/var/run/exporter/`
- File: [pkg/exporter/scheduler/slurm.go](../../pkg/exporter/scheduler/slurm.go)

### 6. gpuagent (gRPC Service)

**Purpose:** Core GPU telemetry service (git submodule)

**Communication:**
- Unix socket: `/var/run/gpuagent.sock` (default)
- TCP/IP: `localhost:50061` (via `-agent-grpc-port` flag)

**Interfaces:**
- **amd-smi library** → amdgpu driver (GPU metrics)
- **UAL driver** directly (UAL metrics)

**Location:** `gpuagent/` submodule

---

## GPUAgent Library Internals

### Multi-Layer Architecture

**Reference:** [gpuagent/developerguide.md](../../gpuagent/developerguide.md)

1. **API Layer** (North Bound)
   - API definitions: `gpuagent/sw/nic/gpuagent/protos/`
   - Internal models: `gpuagent/sw/nic/gpuagent/api/`

2. **Service Layer**
   - gRPC services: `gpuagent/sw/nic/gpuagent/svc/`
   - **Two Sub-Services:**
     - **GPU Service** - GPU metrics via `GPUSvcClient`
     - **UAL Service** - UAL metrics via `UALSvcClient`

3. **Data Abstraction Layer (SMI)**
   - Layer: `gpuagent/sw/nic/gpuagent/api/smi/`
   - Translates internal models to protobuf payloads
   - Data client: amdsmi (`gpuagent/sw/nic/gpuagent/api/smi/amdsmi/smi_api.cc`)

### Request Flow

```
DME (user/client)
  ↓ gRPC request
gpuagent
  ↓ data marshal
svc (service layer)
  ↓ smi call
smi (data abstraction)
  ↓ amdsmi call
libamdsmi
  ↓ driver query
AMD GPU HW (via amdgpu driver)
  ↓ driver response
libamdsmi
  ↓ amdsmi response
smi
  ↓ smi response
svc
  ↓ data unmarshal
gpuagent
  ↓ gRPC response
DME (user/client)
```

---

## Request Flows

### GPU Metrics Collection (/metrics)

```
HTTP GET /metrics
  ↓
Prometheus Middleware
  ↓
MetricsHandler.UpdateMetrics()
  ↓
GPUAgent Library → GPU Client
  ↓ gRPC (unix:///var/run/gpuagent.sock)
gpuagent.GPUGet()
  ↓
amd-smi library
  ↓
amdgpu driver
  ↓
AMD GPU Hardware
  ↓
[Response propagates back]
  ↓
Prometheus Registry Updated (filtered by config.json)
  ↓
HTTP 200 /metrics response (Prometheus format)
```

> The `GPUGet` → GaugeVec box above is an **internal intermediate**: the populated
> GaugeVecs are mirrored into the single-source store, and `/metrics` is rendered
> from the store. `/metrics` is the **only** way to retrieve data — there is no
> push stream.

### Single-source snapshot store (the only data path)

All metric storage and retrieval goes through **one in-memory source of truth**
(`pkg/exporter/snapshotstore`). Every client (GPU, NIC, IFOE) writes it; `/metrics`
renders it. Hardware data is written once and read once, uniformly.

- **Store (Model B):** per-device immutable `DeviceSnapshot`s behind an
  `atomic.Pointer`; reads are lock-free, writes clone→apply→swap (copy-on-write).
  The device map itself is an `atomic.Pointer`, clone-swapped, so a scrape never
  sees a torn map and many scrapes read concurrently while a write is in flight.
- **Clean interface seams:** writers depend on `snapshotstore.StoreWriter`
  (`Update`, `UpdateIfChanged`); the collector depends on `snapshotstore.StoreReader`
  (`Snapshot`, `Snapshots`, `Devices`). `*Store` satisfies both. The high-res feed
  is wired through the `HighResSource` interface (`SupportsHighRes`, `StartHighRes`).
- **Generic `StoreCollector`** (a `prometheus.Collector`) renders the store to
  exposition text, reused by GPU/NIC/IFOE. Writers mirror the already-populated
  GaugeVecs via introspection, so output is structurally identical to the legacy
  GaugeVec exposition. The GaugeVecs remain the source of truth for metric
  name/help/label derivation; they are never registered in the live registry.
- **Serving (uniform, no flag):** `prometheusMiddleware` refreshes each
  store-serving client once per scrape via `RefreshStoreServingClients` (each
  client's singleflight fetch coalesces concurrent scrapes onto one hardware pull),
  then renders lock-free from the store collectors. No `prometheusMiddlewareMu`,
  no reset+refill. `NeedsPerScrapePull()` is `false` for GPU/NIC/IFOE; it is only
  ever true with zero clients registered (startup guard).
- **Node-level totals** (`gpu_nodes_total`, `nic_total`, `ifoe_total_*`) live in a
  reserved node slot, are always-present (health signal), and are written only on
  change via `Store.UpdateIfChanged` — boot/reconfiguration-time constants, never
  re-stored per scrape.
- **Async pull labels (GPU):** the scheduler `ListWorkloads()` is served from a 3s
  async cache (`pullWorkloadCache`) off the scrape hot path.

### High-resolution mode (GPUWatch → store)

When `HighResolution.Enable` is set, a GPUWatch consumer samples the GPU at a fast
cadence (`FastPeriodSecs`, 1s first, 10ms north-star) and drives the store directly,
decoupling hardware polling from HTTP scrapes. `/metrics` serves the latest snapshot:

```
gpuagent GPUWatch feed (fast)                     health loop (slow, health interval)
  ↓ applyWatchSnapshot (fast writer)                ↓ refreshNonWatchStore (slow writer)
  watch fields → store                              non-watch fields (health, afid,
   (incl. energy_consumed)                            GPUGet-only) → store
                     └──────────→ store ←────────────────┘
                                    ↓
                     /metrics (pull, StoreCollector) — the only output
```

- **Fast writer** (`applyWatchSnapshot`) writes watch fields to the store per
  snapshot (no frame, no publish). **Slow writer** backfills the non-watch
  GPUGet-only fields on the health interval, so high-res `/metrics` has full field
  parity **without** a per-scrape `GPUGet` (fast scrapes, p50 ≈ 2–5 ms).
- **Profiler stream writer** (high-res, non-PTL) is a third store feeder: when the
  profiler stream is active it owns `gpu_prof_*` and writes them to the store at the
  fast cadence, and the slow writer skips them. With the stream off (PTL delay set or
  profiler disabled) the slow writer backfills `gpu_prof_*` instead — so `profiler`
  in the diagram above is slow-writer-fed only in that fallback.
- **Config field-disable** is honored on `/metrics` (the single output).
- Labels are 1-1 with the pull path: the same `populateLabelsFromGPU` set (static
  GPU metadata + partition + pod/slurm workload labels from the async cache),
  including empty-valued mandatory keys.
- `HighResolution.StreamPort` was removed (no stream server). NIC/IFOE have no
  high-res feed (`HighResSource` is GPU-only).

### Full GPU Metrics (/gpumetrics)

```
HTTP GET /gpumetrics
  ↓
NPD Handler (pkg/exporter/npd.go)
  ↓
GPUAgent Library → GPU Client
  ↓ gRPC (unix:///var/run/gpuagent.sock)
gpuagent.GPUGet()
  ↓
amd-smi library
  ↓
amdgpu driver
  ↓
AMD GPU Hardware
  ↓
[Response propagates back]
  ↓
HTTP 200 /gpumetrics response (JSON payload, unfiltered)
```

### In-Band RAS Errors (/inbandraserrors)

```
HTTP GET /inbandraserrors
  ↓
NPD Handler (pkg/exporter/npd.go)
  ↓
GPUAgent Library → GPU Client
  ↓ gRPC (unix:///var/run/gpuagent.sock)
gpuagent.GPUCPERGet()
  ↓
amd-smi library
  ↓
amdgpu driver
  ↓
AMD GPU Hardware (RAS error records)
  ↓
[Response propagates back]
  ↓
HTTP 200 /inbandraserrors response (JSON payload)
```

### UAL/IFOE Metrics Collection

```
HTTP GET /metrics
  ↓
Prometheus Middleware
  ↓
MetricsHandler.UpdateMetrics()
  ↓
GPUAgent Library → UAL/IFOE Client
  ↓ gRPC (unix:///var/run/gpuagent.sock)
gpuagent.UALGet()
  ↓
UAL driver (direct)
  ↓
UAL Hardware
  ↓
[Response propagates back]
  ↓
Prometheus Registry Updated
  ↓
HTTP 200 /metrics response (Prometheus format)
```

### NIC Metrics Collection

```
HTTP GET /metrics
  ↓
Prometheus Middleware
  ↓
MetricsHandler.UpdateMetrics()
  ↓
NIC Agent Client
  ↓
┌────────────┬──────────────┬────────────────┐
│            │              │                │
NICCtl      Ethtool       RDMA Stats
Client      Client         Client
  ↓            ↓              ↓
nicctl      ethtool        rdma
show nic    -S <intf>      statistic -j
  ↓            ↓              ↓
NIC Hardware → Ethernet → RoCE Protocol
  ↓
[Responses aggregated]
  ↓
Prometheus Registry Updated
  ↓
HTTP 200 /metrics response (Prometheus format)
```

### Health Monitoring (gRPC Service)

```
Health Monitor Loop (30s interval by default)
  ↓
GPUAgent.UpdateStaticMetrics()
  ↓ gRPC
gpuagent.GPUGet()
  ↓
GPU metrics retrieved
  ↓
Health Evaluation (ECC threshold checks)
  ↓
Update health status in gRPC service
  ↓
External client (device-plugin, testrunner)
  ↓ gRPC (unix:///var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket)
MetricsService.GetGPUState() / List()
  ↓
GPUStateResponse (healthy/unhealthy)
```

**Note:** DME is the server; device-plugin and testrunner are clients querying health status.

---

## Deployment Detection & Startup

### Entry Point

**File:** [cmd/exporter/main.go](../../cmd/exporter/main.go)

### Startup Sequence

1. **Parse Command-Line Flags**
   - Relaxed flag parsing: `AMD_EXPORTER_RELAXED_FLAGS_PARSING` env var
   - Key flags:
     - `-s <socket_path>` - gpuagent socket path
     - `-agent-grpc-port <port>` - gpuagent TCP port (default: 50061)
     - `-exit-on-agent-down` - Auto-restart on agent failures
     - `-sriov-enable` - SR-IOV mode
     - `-amd-metrics-config <path>` - Config file path

2. **Detect Deployment Type**
   - **Kubernetes:** Check `KUBERNETES_SERVICE_HOST` env var or kubelet socket
   - **Debian Package:** Check `/usr/lib/systemd/system/amd-metrics-exporter*.service`
   - **Container:** Default if neither detected

3. **Initialize Components**
   - **Logger:** File-based with rotation (LoggingConfig) or console
   - **Config Handler:** Load `/etc/metrics/config.json` or custom path
   - **Metrics Handler:** Initialize Prometheus registry

4. **Create Exporter (Builder Pattern)**
   ```go
   exporter := NewExporter(
       WithGPUMonitoring(),        // Enable GPU metrics
       WithNICMonitoring(),        // Enable NIC metrics
       WithIFOEMonitoring(),       // Enable UAL metrics
       WithK8sApiClient(),         // K8s pod metadata
       WithK8sSchedulerClient(),   // K8s PodResources
       WithSlurmClient(),          // SLURM job files
       WithSocketConnection(),     // gpuagent socket
       WithExitOnAgentDown(),      // Auto-restart on failures
   )
   ```

5. **Start File Watcher**
   - Monitor config file directory for changes
   - 3-second debounce on file modifications
   - Dynamic reload: Stop server → Reload config → Reinit registry → Start server
   - No service restart required

6. **Launch HTTP Metrics Server**
   - **Port:** 5000 (GPU), 5001 (NIC)
   - **Endpoints:**
     - `/metrics` - Prometheus format (filtered by config.json)
     - `/gpumetrics` - JSON payload (full GPU metrics, unfiltered)
     - `/inbandraserrors` - JSON payload (in-band RAS errors)
   - Bind to address (default: all interfaces)

7. **Handle Graceful Shutdown**
   - Catch SIGINT/SIGTERM signals
   - Stop HTTP servers
   - Cleanup resources (close connections, stop monitors)
   - Exit cleanly

---

## gpuagent Operating Modes

### 1. amd-smi Mode (Standard)

**Environment:** Bare metal/host with amdgpu driver

**Hardware Access:** Direct via KFD (Kernel Fusion Driver)

**Library:** AMD SMI from `gpuagent/sw/nic/third-party/rocm/amd_smi_lib`

**Detection:** KFD device (`/dev/kfd`) present, amdgpu driver loaded

**Usage:** Default mode on bare metal AMD GPU systems

### 2. GIM amd-smi Mode (SR-IOV)

**Environment:** Virtual machine with gim driver (driver name: gim)

**Hardware Access:** SR-IOV VF/PF passthrough

**Library:** GIM SMI from `gpuagent/sw/nic/third-party/rocm/gim_amd_smi_lib`

**Activation:** DME flag `-sriov-enable`

**Limitations:**
- Profiler disabled on SR-IOV hosts
- Limited metrics compared to bare metal

**Usage:** VMs with SR-IOV GPU passthrough

### 3. Mock Mode (Testing)

**Environment:** CI/CD without real hardware

**Location:** `pkg/amdgpu/mock/`, `pkg/amdgpu/mock_gen/`

**Purpose:** Generate synthetic GPU data for testing

**Usage:** 
- `test/e2e` for mock exporter container images
- CI/CD sanity tests
- Development without GPU hardware

**Mode Selection:** gpuagent auto-detects hardware (KFD presence, GPU device files, hypervisor detection). DME receives metrics via unified gRPC interface regardless of mode.

---

## gRPC Service Clients

### GPU Service (amdgpu.GPUSvcClient)

**Proto:** [pkg/amdgpu/proto/gpu.proto](../../pkg/amdgpu/proto/gpu.proto)

**Methods:**
- `GPUGet()` - Fetch GPU metrics, status, specs (primary metrics collection)
- `GPUUpdate()` - Update GPU configuration (power limits, clocks)
- `GPUReset()` - Reset GPU state
- `GPUComputePartitionSet/Get()` - Manage compute partitions
- `GPUMemoryPartitionSet/Get()` - Manage memory partitions
- `GPUCPERGet()` - Fetch correctable/uncorrectable error records (for /inbandraserrors)

### UAL Service (amdgpu.UALSvcClient)

**Proto:** [pkg/amdgpu/proto/ual.proto](../../pkg/amdgpu/proto/ual.proto)

**Methods:**
- UAL network port management APIs
- Station device management APIs
- Used for UAL/IFOE device link health monitoring

**Note:** Codebase may still reference this as IFOE; treat as UAL.

### Events Service (amdgpu.EventSvcClient)

**Proto:** [pkg/amdgpu/proto/events.proto](../../pkg/amdgpu/proto/events.proto)

**Methods:**
- Stream ECC error events
- RAS (Reliability, Availability & Serviceability) events

**IMPORTANT - Currently Disabled:**
- Disabled by default via ENV variable
- **Reason:** Keeps `/dev/kfd` open permanently, preventing:
  - amdgpu driver removal
  - Driver reinstall
  - GPU partitioning operations
- **Future Consideration:** Enabling requires careful decision on driver operation impact
- **Enablement Risk:** Feature-breaking change, must document impact before enabling

---

## Scheduler Integrations

### Kubernetes Scheduler Client

**File:** [pkg/exporter/scheduler/k8s.go](../../pkg/exporter/scheduler/k8s.go)

**Connection:** gRPC to kubelet's PodResources endpoint

**Socket Path:** `/var/lib/kubelet/pod-resources/kubelet.sock`

**Resource Detection:**
- **Device Plugin:** Looks for `amd.com/*` resource names (prefix match)
- **DRA (Dynamic Resource Allocation):** Looks for `gpu.amd.com` driver name claims

**ListWorkloads():**
- **Input:** None (queries kubelet)
- **Output:** Map of Device ID → `PodResourceInfo{Pod, Namespace, Container}`

**Export Labels:** POD, NAMESPACE, CONTAINER, POD_UUID

**Connection Resilience:** Auto-reconnect on connection failures

### Slurm Scheduler Client

**File:** [pkg/exporter/scheduler/slurm.go](../../pkg/exporter/scheduler/slurm.go)

**Connection:** File system watcher on `/var/run/exporter/` directory

**Job Files:** JSON files created by SLURM prolog scripts

**File Format:**
```json
{
    "CUDA_VISIBLE_DEVICES": "0,1",
    "SLURM_JOB_ID": "12345",
    "SLURM_JOB_USER": "username",
    "SLURM_JOB_PARTITION": "gpu",
    "SLURM_CLUSTER_NAME": "cluster1"
}
```

**ListWorkloads():**
- **Input:** File system events
- **Output:** Map of GPU ID → `JobInfo{Id, User, Partition, Cluster}`

**Export Labels:** JOB_ID, JOB_USER, JOB_PARTITION, CLUSTER_NAME

**File Lifecycle:** Created by prolog, deleted by epilog

---

## Common Infrastructure

### Client Package

**File:** [pkg/client/k8s.go](../../pkg/client/k8s.go)

**Purpose:** Kubernetes API client for pod metadata and label extraction

**Functions:**
- `WatchPods()` - Watch pod changes and maintain cache
- `GetPodLabels()` - Extract pod labels for ExtraPodLabels feature
- `GetPodInfo()` - Get pod metadata (namespace, UID, etc.)

**Required For:** K8s deployments with ExtraPodLabels configuration

### Exporter Core

**File:** [pkg/exporter/exporter.go](../../pkg/exporter/exporter.go)

**Exporter Struct:** Central orchestrator for all components

**ExporterOptions (Builder Pattern):**
```go
type ExporterOptions struct {
    WithGPUMonitoring()        // Enable GPU metrics collection
    WithNICMonitoring()        // Enable NIC metrics collection
    WithIFOEMonitoring()       // Enable UAL metrics collection
    WithK8sApiClient()         // Enable K8s API client
    WithK8sSchedulerClient()   // Enable K8s PodResources client
    WithSlurmClient()          // Enable SLURM job file watcher
    WithSocketConnection()     // Set gpuagent socket path
    WithBindAddr()             // Set HTTP server bind address
    WithExitOnAgentDown()      // Enable auto-restart on agent failures
}
```

**Key Methods:**
- `StartMain()` - Initialize all clients, start metric collection
- `foreverWatcher()` - Continuous config file monitoring with reload
- `prometheusMiddleware()` - HTTP handler for /metrics endpoint

### NPD Integration

**File:** [pkg/exporter/npd.go](../../pkg/exporter/npd.go)

**Purpose:** Node Problem Detector integration with JSON endpoints

**Endpoints:**
- `/gpumetrics` - Full GPU metrics in JSON format (unfiltered by config)
- `/inbandraserrors` - In-band RAS errors in JSON format

**Note:** These endpoints return complete data independent of config.json metric filtering

### Metrics Handler

**Directory:** [pkg/exporter/metricsutil/](../../pkg/exporter/metricsutil/)

**Purpose:**
- Manage Prometheus registry (create, register, unregister metrics)
- Coordinate metric updates from all clients
- Handle GPU agent communication
- Handle NIC agent communication

**Key Functions:**
- `UpdateMetrics()` - Main entry point for metrics collection
- `RegisterMetrics()` - Register Prometheus metrics
- `UnregisterMetrics()` - Cleanup on config reload

### Services

**Directory:** [pkg/exporter/svc/](../../pkg/exporter/svc/)

**Purpose:**
- Health service: gRPC server on Unix socket for health queries
- Evaluate GPU/NIC health at configurable intervals
- Register clients and evaluate thresholds
- Serve health status to external clients (device-plugin, testrunner)

**Key Files:**
- Health service implementation
- Client registration and management
- Threshold evaluation logic

### Globals

**File:** [pkg/exporter/globals/constants.go](../../pkg/exporter/globals/constants.go)

**Port Assignments:**
- Metrics HTTP: 5000 (GPU), 5001 (NIC)
- GPU Agent gRPC: 50061 (TCP mode)

**Socket Paths:**
- GPU Agent: `/var/run/gpuagent.sock`
- Metrics Health gRPC: `/var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket`
- Kubelet PodResources: `/var/lib/kubelet/pod-resources/kubelet.sock`
- SLURM jobs: `/var/run/exporter/`

**K8s Constants:**
- Device plugin prefix: `amd.com/`
- DRA driver: `gpu.amd.com`
- Max custom labels: 10
- Max ExtraPodLabels: 20

---

## Design Principles

### Graceful Degradation

**Principle:** If a feature is unsupported, mark as unsupported during boot (don't fail)

**Examples:**
- Profiler metrics unavailable → mark as unsupported, continue exporting other metrics
- No K8s scheduler → skip pod labels, continue with basic metrics
- gpuagent unreachable → mark GPUs unhealthy, continue serving health service

**Implementation:** Feature detection during initialization, soft failures logged

### Self-Recovery

**Principle:** Built-in recovery from transient failures

**Mechanisms:**
- Auto-restart on gpuagent connection failures
- Reconnection logic with exponential backoff
- Failure counter (3 consecutive failures → action)
- Exit-on-agent-down: DME exits → K8s restarts pod

**Examples:**
- Driver load race: gpuagent starts before driver → DME retries → reconnects when driver ready
- gpuagent crash: DME detects connection loss → reconnects → resumes metrics

### Security

**Principle:** No external network exposure, Unix socket communication

**Security Features:**
- gRPC over Unix sockets (no TCP ports exposed by default)
- No external network access required
- Safe profiler execution with timeout and failure handling
- Health service only accessible via local Unix socket

**Network Isolation:** Even in K8s, communication is via hostPath-mounted sockets

### Extensibility

**Principle:** Modular design allows easy addition of new device types

**Extensibility Points:**
- Modular client architecture (GPU, NIC, UAL)
- Builder pattern for flexible exporter configuration
- Proto-based metric definitions (easy to extend)
- Can be vendored into other projects (e.g., ROCm gpu-operator)

**Adding New Devices:** Create new client implementing metric interface, add to builder options

### Runtime Configurability

**Principle:** Dynamic changes without service restart

**Configuration Features:**
- Dynamic config reload without service restart (3-second debounce)
- Per-node profiler control (enable/disable per hostname)
- Flexible metric and label selection
- Runtime health threshold updates

**Implementation:** File watcher → detect changes → stop server → reload config → reinit → start server

---

## Documentation Structure

### User-Facing Documentation (`/docs/`)

The project uses Sphinx to generate user documentation from Markdown sources:

**Configuration Guides** (`/docs/configuration/`):
- `configuration-settings.md` - Complete config.json reference with all available fields
- `metricslist.md` - GPU metrics catalog (categories, descriptions, platform support)
- `network-metricslist.md` - NIC metrics catalog
- `ifoe-metricslist.md` - UAL/IFOE metrics catalog
- `configmap.md` - Kubernetes ConfigMap examples
- `docker.md` - Docker-specific configuration
- `troubleshooting.md` - User troubleshooting guide
- `agfhc.md` - AGFHC-specific configuration

**Installation Guides** (`/docs/installation/`):
- `kubernetes-helm.md` - Helm chart installation and configuration
- `docker.md` - Docker deployment options
- `gpu-operator.md` - ROCm GPU operator integration
- `nic-debian-package.md` - NIC exporter Debian package installation
- `singularity.md` - Singularity container deployment

**Integration Guides** (`/docs/integrations/`):
- `prometheus-grafana.md` - Prometheus/Grafana setup and dashboards
- `prometheus-servicemonitor.md` - Kubernetes ServiceMonitor configuration
- `node-health-monitoring.md` - Node health monitoring integration
- `slurm-integration.md` - SLURM scheduler integration

**Release Information**:
- `index.md` - Main entry point with compatibility matrix (ROCm/Driver/Exporter versions)
- `releasenotes.md` - GPU exporter release notes
- `releasenotes-nic.md` - NIC exporter release notes
- `knownissues.md` - Known issues and limitations

### Developer Documentation

**Build and Architecture**:
- `developerguide.md` - Build instructions, architecture diagrams, development workflow
- `full-build-instructions.md` - Complete component build instructions

**Metrics Reference**:
- `internal/metricsmap.md` - Critical metrics mapping table:
  - Maps: Exporter Metric → GPU Agent Field → amd-smi Field → Platform
  - Platform applicability (MI2xx, MI3xx)
  - Critical metrics for workload evaluation

**Component Architecture**:
- `gpuagent/developerguide.md` - GPUAgent library architecture (API, Service, Data layers)

**Project Overview**:
- `README.md` - Project overview and quick start

### Sphinx Configuration

**Build System** (`/docs/sphinx/`):
- `_toc.yml` - Table of contents structure
- `requirements.txt` - Python dependencies for documentation build
- `conf.py` - Sphinx configuration

**Build Command**:
```bash
make docs  # Builds HTML documentation
make docs-lint  # Lints documentation
```

---

## Related Documentation

For detailed information on specific components, see:

- **GPU Exporter Details:** [gpu-exporter.md](gpu-exporter.md)
- **NIC Exporter Details:** [nic-exporter.md](nic-exporter.md)
- **ROCProfiler Integration:** [rocprofiler.md](rocprofiler.md)
- **Health Monitoring:** [health-monitoring.md](health-monitoring.md)
- **Configuration System:** [configuration.md](configuration.md)
- **Build System:** [build-system.md](build-system.md)
- **Deployment Modes:** [deployment.md](deployment.md)
- **CI/CD Pipeline:** [ci-cd.md](ci-cd.md)
- **Troubleshooting Guide:** [troubleshooting.md](troubleshooting.md)
- **Adding New Metrics:** [adding-metrics.md](adding-metrics.md)
- **Critical File Paths:** [critical-paths.md](critical-paths.md)

---

**Last Updated:** 2026-04-05
