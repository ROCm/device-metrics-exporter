# Slurm integration

AMD Device Metrics Exporter integrates with Slurm workload manager to track GPU metrics for Slurm jobs. This topic explains how to set up and configure this integration.

## Prerequisites

- Slurm workload manager installed and configured
- AMD Device Metrics Exporter installed and running
- Root or sudo access on Slurm nodes

## Installation

- Copy the integration script:
  - [exporter-epilog.sh](../../debian/usr/local/etc/metrics/slurm/slurm-epilog.sh)
  - [exporter-prolog.sh](../../debian/usr/local/etc/metrics/slurm/slurm-prolog.sh)

```bash
cp ${TOP_DIR}/example/slurm/exporter-prolog.sh /etc/slurm/epilog.d/exporter-prolog.sh
cp ${TOP_DIR}/example/slurm/exporter-epilog.sh /etc/slurm/epilog.d/exporter-epilog.sh
sudo chmod +x /etc/slurm/epilog.d/exporter-prolog.sh
sudo chmod +x /etc/slurm/epilog.d/exporter-epilog.sh
```

> **Note (AMD hardware):** `CUDA_VISIBLE_DEVICES` is empty on AMD GPUs, so the bundled scripts fall back to `SLURM_JOB_GPUS` (`[ -z "${AMDGPU_DEVICES}" ] && AMDGPU_DEVICES="${AMD_SLURM_GPUS}"`). Keep this fallback if you maintain your own copy, otherwise metrics are not tagged with `job_id`/`job_user`.

- Configure Slurm:

```bash
sudo vi /etc/slurm/slurm.conf

# Add these lines:
prologFlags=Alloc
Prolog="/etc/slurm/prolog.d/*"
Epilog="/etc/slurm/epilog.d/*"
```

- Restart Slurm services to apply changes:

```bash
sudo systemctl restart slurmd     # On compute nodes if slurm.conf is updated
```

## Exporter Container Deployment

### Directory Setup

It's recommended to use the following directory structure to store persistent exporter data on the host:

```bash
$ tree -d exporter/
     exporter/
       - config/
         - config.json
```

Create the directory required for tracking Slurm jobs:

```bash
mkdir -p /var/run/exporter
```

### Start Exporter Container

Once the directory structure is ready, start the exporter container:

```bash
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v ./config:/etc/metrics \
  -v /var/run/exporter/:/var/run/exporter/ \
  -p 5000:5000 --name device-metrics-exporter \
  rocm/device-metrics-exporter:v1.5.0
```

## Verification

- Submit a test job:

```bash
srun --gpus=1 amd-smi monitor
```

- Check metrics endpoint:

```bash
curl http://localhost:5000/metrics | grep job_id
```

You should see metrics tagged with the Slurm job ID.

## Metrics

When Slurm integration is enabled, the following job-specific labels are added to metrics:

- `job_id`: Slurm job ID
- `job_user`: Username of job owner
- `job_partition`: Slurm partition name
- `cluster_name`: Slurm cluster name

## Troubleshooting

### Common Issues

1. Script permissions:
   - Ensure the exporter script is executable
   - Verify proper ownership (should be owned by `root` or `slurm` user)

2. Configuration issues:
   - Check Slurm logs for prolog/epilog execution errors
   - Verify paths in slurm.conf are correct

3. Metric collection:
   - Ensure metrics exporter is running
   - Check if job ID labels are being properly set

4. Check service status:

```bash
# Host package (apt):
systemctl status gpuagent.service amd-metrics-exporter.service
# Container (Docker):
docker ps --filter name=device-metrics-exporter
```

### Logs

View Slurm logs for integration issues:

```bash
sudo tail -f /var/log/slurm/slurmd.log
```

View service logs:

```bash
# Host package (apt):
journalctl -u gpuagent.service -u amd-metrics-exporter.service
# Container (Docker): logs are written to /var/log/exporter.log inside the container
docker exec device-metrics-exporter tail -f /var/log/exporter.log
```

## Advanced Configuration

### Custom Script Location

You can place the script in a different location by updating the paths in `slurm.conf`:

```bash
Prolog=/path/to/custom/slurm-prolog.sh
Epilog=/path/to/custom/slurm-epilog.sh
```

### Additional Job Information

The integration script can be modified to include additional job-specific information in the metrics. Edit the script to add custom labels as needed.

Slurm labels are disabled by default. To enable Slurm labels, add the following to your `config.json`:

```json
{
  "GPUConfig": {
    "Labels": [
      "GPU_UUID",
      "SERIAL_NUMBER",
      "GPU_ID",
      "POD",
      "POD_UUID",
      "NAMESPACE",
      "CONTAINER",
      "JOB_ID",
      "JOB_USER",
      "JOB_PARTITION",
      "CLUSTER_NAME",
      "CARD_SERIES",
      "CARD_MODEL",
      "CARD_VENDOR",
      "DRIVER_VERSION",
      "VBIOS_VERSION",
      "HOSTNAME"
    ]
  }
}
```
