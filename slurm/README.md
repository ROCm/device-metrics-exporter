# Slurm exporter setup

Exporter requires the epilog and prolog scripts to be placed in the
slurm configuration path respectively

## Host/Node Setup

This is to be performed on all the nodes on the cluster where the exporter is to be deployed.

Below is a smaple `slurm.conf` entry which points to the epilog and prolog directories.

```
# Epilogs and Prologs
Epilog="/etc/slurm/epilog.d/*"
Prolog="/etc/slurm/prolog.d/*"
```

Copy the files to the path
```
cp exporter-epilog.sh /etc/slurm/epilog.d/exporter-epilog.sh
cp exporter-prolog.sh /etc/slurm/epilog.d/exporter-prolog.sh
```

Once all the necessary configurations are done restart slurmd

```
systemctl restart slurmd.service
```

## Exporter Container Deployment

### directory setup

 1. It is recommended to create following heirary to keep the exporter data in persistant files on the host

   ```
     $ tree -d exporter/
     exporter/
       - config/
         - config.json
   ```

  2. `/var/run/exporter` directory must be created on the host, as this is used by prolog and epilog scripts to tracking the slurm jobs

   ```
   mkdir -p /var/run/exporter
   ```

### label configuration
By default the exporter labels are not enabled for slurm labels. Use the below content for config.json file.

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

### start exporter

Once we have all the above steps done we can start the exporter

```
docker run -d \
  --device=/dev/dri \
  --device=/dev/kfd \
  -v ./config:/etc/metrics \
  -v /var/run/exporter/:/var/run/exporter/ \
  -p 5000:5000 --name exporter \
  rocm/device-metrics-exporter:v1.0.0
```
