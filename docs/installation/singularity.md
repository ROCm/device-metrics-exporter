# Singularity installation

You can use Singularity to run Device Metrics Exporter in a cluster environment through Slurm.
  
## Installation

The Device Metrics Exporter container is hosted on Docker Hub at [rocm/device-metrics-exporter](https://hub.docker.com/r/rocm/device-metrics-exporter).

- Convert the Docker image to a Singularity image file (SIF). Create a file named `Singularity.def` with the following content:

  ```text
  bootstrap: docker
  From: rocm/device-metrics-exporter:v1.5.5

  %post
      chmod +x /home/amd/tools/entrypoint.sh

  %startscript
      # This script runs when the container is started with 'singularity run'
      exec /home/amd/tools/entrypoint.sh

  ```

- Build the Singularity image:
  
  ```bash
  sudo singularity build device_metrics_exporter.sif Singularity.def
  ```

- Run the Singularity container:

  ```bash
  sudo singularity instance start \
  --writable-tmpfs \
  --bind ./config:/etc/metrics \
  --bind /var/run/exporter/:/var/run/exporter/ \
  device_metrics_exporter.sif metrics-exporter
  ```

- Confirm metrics are accessible:

```bash
curl http://localhost:5000/metrics
```

- Review the [Prometheus and Grafana integration guide](../integrations/prometheus-grafana.md).

## Key points

- Run the Singularity container with `sudo` to ensure proper device access and logging.
- Bind mount `./config` and `/var/run/exporter` for correct operation.

## Custom metrics

For information about custom metrics, see [Standalone container configuration](../configuration/docker.md).
