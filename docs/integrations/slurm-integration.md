# Slurm integration

AMD Device Metrics Exporter integrates with Slurm workload manager to track GPU metrics for Slurm jobs. This topic explains how to set up and configure this integration.

## Prerequisites

- Slurm workload manager installed and configured
- AMD Device Metrics Exporter installed and running
- Root or sudo access on Slurm nodes

## Installation

- Copy the integration script:

```bash
cp ${TOP_DIR}/example/slurm/exporter-prolog.sh /etc/slurm/epilog.d/exporter-prolog.sh
cp ${TOP_DIR}/example/slurm/exporter-epilog.sh /etc/slurm/epilog.d/exporter-epilog.sh
sudo chmod +x /etc/slurm/epilog.d/exporter-prolog.sh
sudo chmod +x /etc/slurm/epilog.d/exporter-epilog.sh
```

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
sudo systemctl restart slurmd     # On compute nodes
```

## Exporter Container Deployment

### Directory Setup

It's recommended to use the following directory structure to store persistent exporter data on the host:

```
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
  -p 5000:5000 --name exporter \
  rocm/device-metrics-exporter:v1.3.1
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
systemctl status gpuagent.service amd-metrics-exporter.service
```

### Logs

View Slurm logs for integration issues:

```bash
sudo tail -f /var/log/slurm/slurmd.log
```

View service logs:

```bash
journalctl -u gpuagent.service -u amd-metrics-exporter.service
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

```
{
  "GPUConfig": {
    "Labels": [
      "GPU_UUID",
      "SERIAL_NUMBER",
      "GPU_ID",
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
