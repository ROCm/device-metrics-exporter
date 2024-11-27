# Slurm Integration Guide

The AMD GPU Metrics Exporter provides integration with Slurm workload manager to track GPU metrics for Slurm jobs. This guide explains how to set up and configure this integration.

## Prerequisites

- Slurm workload manager installed and configured
- AMD GPU Metrics Exporter installed and running
- Root or sudo access on Slurm nodes

## Installation

- Copy the integration script:

```bash
sudo cp /usr/local/etc/metrics/slurm/slurm-exporter.sh /etc/slurm/
sudo chmod +x /etc/slurm/slurm-exporter.sh
```

- Configure Slurm:

```bash
sudo vi /etc/slurm/slurm.conf

# Add these lines:
prologFlags=Alloc
Prolog=/etc/slurm/slurm-exporter.sh
Epilog=/etc/slurm/slurm-exporter.sh
```

- Restart Slurm services to apply changes:

```bash
sudo systemctl restart slurmctld  # On controller node
sudo systemctl restart slurmd    # On compute nodes
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

## Troubleshooting

### Common Issues

1. Script permissions:
   - Ensure the exporter script is executable
   - Verify proper ownership (should be owned by root or slurm user)

2. Configuration issues:
   - Check Slurm logs for prolog/epilog execution errors
   - Verify paths in slurm.conf are correct

3. Metric collection:
   - Ensure metrics exporter is running
   - Check if job ID labels are being properly set

### Logs

Check Slurm logs for integration issues:

```bash
sudo tail -f /var/log/slurm/slurmd.log
```

## Advanced Configuration

### Custom Script Location

You can place the script in a different location by updating the paths in `slurm.conf`:

```bash
Prolog=/path/to/custom/slurm-exporter.sh
Epilog=/path/to/custom/slurm-exporter.sh
```

### Additional Job Information

The integration script can be modified to include additional job-specific information in the metrics. Edit the script to add custom labels as needed.
