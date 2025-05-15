# Singularity installation

Singularity can be used to run device-metrics-exporter in a cluster environment through SLURM. The steps to achieve
this are detailed below.
  
## Installation

The Device Metrics Exporter container is hosted on Docker Hub at [rocm/device-metrics-exporter](https://hub.docker.com/r/rocm/device-metrics-exporter).

- Convert Docker Image to Singularity SIF:
  Create a file named Singularity.def with the following content:

```bash
bootstrap: docker
From: rocm/device-metrics-exporter:v|version|

%post
    chmod +x /home/amd/tools/entrypoint.sh

%startscript
    # This script runs when the container is started with 'singularity run'
    exec /home/amd/tools/entrypoint.sh

```
- Build the Singularity Image:
  
```bash
sudo singularity build device_metrics_exporter.sif Singularity.def
```
- Run the Singularity Container:

```bash
sudo singularity instance start \
--writable-tmpfs \
--bind ./config:/etc/metrics \
--bind /var/run/exporter/:/var/run/exporter/ \
device-metrics-exporter-numa-fixed.sif metrics-exporter
```
- Confirm metrics are accessible:

```bash
curl http://localhost:5000/metrics
```

- Review the [Prometheus and Grafana Integration Guide](../integrations/prometheus-grafana.md).

## Keypoints
- Run the singularity container with `sudo` to ensure proper device access and logging.
- Bind mount `/config`, `/var/run/exporter`, for correct operation.

## Custom metrics

For information about custom metrics, see [Standalone Container](../configuration/docker.md) for instructions.
