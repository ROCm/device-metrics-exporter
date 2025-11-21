# Standalone NIC Monitoring Debian Package Install

This guide explains how to install and manage the AMD NIC Metrics Exporter using the Debian package artifacts published with each release.

## System Requirements

Make sure the target node meets the following requirements:

- **Operating System**: Ubuntu 22.04 LTS or Ubuntu 24.04 LTS (amd64)
- **Privileges**: Root access (or sudo privileges) to install packages and manage systemd services
- **Networking Tools**: `ethtool` and `iproute2` (automatically pulled in by the package dependencies)

## Prepare the Host

Update the package metadata and install any pending security fixes before adding the exporter:

```bash
sudo apt update
sudo apt upgrade -y
```

**Note**: For air-gapped environments, ensure ethtool and iproute2 are available locally, or mirror them in your repository so .deb dependencies can be resolved.

## Install the NIC Metrics Exporter

### 1. Install via APT

#### Install Prerequisites
1. Update the package list and install necessary tools, keyrings and keys:
    ```bash
    # Install necessary tools
    sudo apt update
    sudo apt install vim wget gpg

    # Create the keyrings directory with the appropriate permissions:
    sudo mkdir --parents --mode=0755 /etc/apt/keyrings

    # Download the ROCm GPG key and add it to the keyrings:
    wget https://repo.radeon.com/rocm/rocm.gpg.key -O - | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg > /dev/null
    ```

2. Edit or create the sources list `/etc/apt/sources.list.d/amdnic-exporter.list` to add the Device Metrics Exporter repository:

    On Ubuntu 22.04:
    ```bash
    deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg]  https://repo.radeon.com/device-metrics-exporter/nic/apt/1.0.0 jammy main
    ```

    On Ubuntu 24.04:
    ```bash
    deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg]  https://repo.radeon.com/device-metrics-exporter/nic/apt/1.0.0 noble main
    ```

3. Update Package List and Install NIC Metrics Exporter

    ```bash
    # Update packages list
    sudo apt update

    # Install NIC metrics exporter
    sudo apt install amdnic-exporter
    ```

### 2. Install using a downloaded .deb file

Replace the filename with the specific Ubuntu version you are targeting (for example, `amdnic-exporter_24.04_amd64.deb` for Ubuntu 24.04):

```bash
sudo apt install ./amdnic-exporter_<ubuntu-version>_amd64.deb
```

Using `apt` ensures the `ethtool` and `iproute2` dependencies are pulled in automatically. If you prefer to use `dpkg`, run `sudo dpkg -i ./amdnic-exporter_<ubuntu-version>_amd64.deb` followed by `sudo apt install -f` to resolve dependencies.

### 3. Enable and Start the Service

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now amd-nic-metrics-exporter.service
```

Verify that the service is running:

```bash
systemctl status amd-nic-metrics-exporter.service
```

Check logs if you need to troubleshoot startup:

```bash
journalctl -u amd-nic-metrics-exporter.service -f
```

### 4. Validate Metrics Collection

- Scrape the metrics endpoint locally:

  ```bash
  curl http://127.0.0.1:5001/metrics | head
  ```

## Metrics Exporter Default Settings

- **Metrics endpoint:** ``http://localhost:5001/metrics``
- **Configuration file:** ``/etc/metrics/config-nic.json``
- **Log file:** ``/var/log/amd-nic-metrics-exporter.log``
- **Server port:** ``5001``

The Exporter HTTP port is configurable via the ServerPort field in the configuration file.

## Metrics Exporter Custom Configuration

### Update the configuration

Edit the NIC configuration file to adjust scrape settings (for example, to change the port or enable additional metrics):

```bash
sudo vi /etc/metrics/config-nic.json
```

The exporter watches this file and automatically reloads the new settings when it changes. For reference, see [example/config-nic.json](./../../example/config-nic.json).

### Change the log file path

1. Open the systemd unit:

```bash
sudo vi /usr/lib/systemd/system/amd-nic-metrics-exporter.service
```

2. Update the `--log-file-path` flag on the `ExecStart` line.

```bash
ExecStart=/usr/local/bin/amd-nic-metrics-exporter --monitor-nic=true --monitor-gpu=false \
    --amd-metrics-config=/etc/metrics/config-nic.json \
    --log-file-path=/var/log/amd-nic-metrics-exporter.log
```

3. Reload systemd and restart the service:

```bash
sudo systemctl daemon-reload
sudo systemctl restart amd-nic-metrics-exporter.service
```

## Uninstall the Package

```bash
sudo apt remove amdnic-exporter
```