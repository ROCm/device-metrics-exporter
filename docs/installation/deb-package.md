# Standalone Debian Package Install

## System Requirements

- **Operating System**: Ubuntu 22.04 LTS
- **ROCm Version**: 6.3.x (specific to each .deb pkg)
 
 Each Debian package release of the Standalone Metrics Exporter is dependent on a specific version of the ROCm amdgpu driver. Please see table below for more information:

| Metrics Exporter Debian Version | ROCm Version | AMDGPU Driver Version |
|---------------------------------|--------------|-----------------------|
| amdgpu-exporter-v1.2.0-13-1.2.0 | ROCm 6.3.x   | 6.10.5                |

## Installation

Before installing the AMD GPU Metrics Exporter, you need to install the following:

- **AMDGPU DKMS Driver**: Installed on the system - (see instructions below)
- **RDC (ROCm Data Center Tool)**: Already installed and running - (see instructions below)

### Step 1: Install System Prerequisites

Update the system:

```bash
sudo apt update
sudo apt install "linux-headers-$(uname -r)" "linux-modules-extra-$(uname -r)"
```

Add user to required groups:

```bash
sudo usermod -a -G render,video $LOGNAME 
```


### Step 2: Install AMDGPU Driver

```{info}
For the most up-to-date information on installing dkms drivers please see the [ROCm Install Quick Start](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/install/quick-start.html) page. The below instructions are the most current instructions as of ROCm 6.2.4.
```

1. Download the driver from the Radeon repository ([repo.radeon.com]( https://repo.radeon.com/amdgpu-install)) for your operating system. For example if you want to get the latest ROCm 6.3.4 drivers for Ubuntu 22.04 you would run the following command:

    ```bash
    wget https://repo.radeon.com/amdgpu-install/6.3.4/ubuntu/jammy/amdgpu-install_6.3.60304-1_all.deb
    ```

    ```{info}
    Please note that the above url will be different depending on what the latest version of the drivers are and your Operating System
    ```

2. Install the driver:

    ```bash
    sudo apt install ./amdgpu-install_6.3.60304-1_all.deb
    sudo apt update 
    amdgpu-install --usecase=dkms 
    ```

3. Load the driver module:

    ```bash
    sudo modprobe amdgpu
    ```

### Step 3: Install RDC (ROCm Data Center Tool)

1. Install RDC package:

    ```bash
    sudo apt-get install rdc
    ```

2. Update your PATH environment variable:

    ```bash
    echo 'export PATH=$PATH:/opt/rocm-6.2.0/bin' >> ~/.bashrc
    source ~/.bashrc
    ```

3. Start the RDC service:

    ```bash
    sudo systemctl start rdc.service
    ```

### Step 4: Install Metrics Exporter

1. Once you have the .deb package (obtained via AMD representative):

    ```bash
    sudo dpkg -i amdgpu-exporter_0.1_amd64.deb
    ```

2. Enable and start services:

    ```bash
    sudo systemctl enable amd-metrics-exporter.service
    sudo systemctl start amd-metrics-exporter.service
    ```

3. Check service status:

    ```bash
    sudo systemctl status amd-metrics-exporter.service
    ```

## Metrics Exporter Default Settings

- Metrics endpoint: `http://localhost:5000/metrics`
- Configuration file: `/etc/metrics/config.json`
- GPU Agent port (default): 50061

The Exporter HTTP port is configurable via the `ServerPort` field in the configuration file.

## Metrics Exporter Custom Configuration

### Using a custom config.json

If you need to customize ports or settings:

1. Download a copy of the default [config.json](https://github.com/ROCm/device-metrics-exporter/blob/main/example/config.json) from the Metrics Exporter Repo. Note that you can change the path to save the config.json file into a different direct. Just be sure to also update the path in the server ExecStart command in step 3.

    ```bash
    wget -O /etc/metrics/config.json https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/example/config.json
    ```

2. Make any required changes to your config.json file and ensure the port number you want to use is correct. Example of the first few lines of the config.json shown below:

    ```json
    {
    "ServerPort": 5000,
    "GPUConfig": {
        "Fields": [
        "GPU_NODES_TOTAL",
        "GPU_PACKAGE_POWER",
    ...
    ...
    ```

3. Edit the amd-metrics-exporter service file:

    ```bash
    sudo vi /lib/systemd/system/amd-metrics-exporter.service
    ```

4. Update the `ExecStart` line to read in the config.json file:

    ```plaintext
    ExecStart=/usr/local/bin/amd-metrics-exporter -amd-metrics-config /etc/metrics/config.json
    ```

5. Reload systemd:

    ```bash
    sudo systemctl daemon-reload
    ```

### Custom Port Configuration - Change GPU Agent Port

1. Edit the GPU Agent service file:

    ```bash
    sudo vi /lib/systemd/system/gpuagent.service
    ```

2. Update `ExecStart` with desired port:

    ```plaintext
    ExecStart=/usr/local/bin/gpuagent -p <port_number>
    ```

### Change Metrics Exporter Port

1. Edit the configuration file:

    ```bash
    sudo vi /etc/metrics/config.json
    ```

2. Update `ServerPort` to your desired port.

### Removing Metrics Exporter and other components

To remove this application, follow these commands in reverse order:

1. Uninstall the Metrics Exporter:
    - Ensure the .deb package is removed:

        ```bash
        sudo dpkg -r amdgpu-exporter
        sudo apt-get purge amdgpu-exporter
        
        ```

2. Uninstall RDC:

    - Ensure the .deb package is removed:

        ```bash
       sudo dpkg -r rdc 
        sudo apt-get purge rdc
        ```

3. (Optional) If you would also like to uninstall the AMDGPU Driver:

    - Uninstall any associated DKMS packages:

        ```bash
        sudo dpkg -r amdgpu-install
        ```

    - Unload the driver module:

        ```bash
        sudo modprobe -r amdgpu
        ```

4. (optional) If you would also like to remove the system prerequisites that were installed:

    - Remove Linux header and module packages:

        ```bash
        sudo apt remove linux-headers-$(uname -r)
        sudo apt remove linux-modules-extra-$(uname -r)
        ```

    - Remove the user from groups:

        ```bash
        sudo gpasswd -d $LOGNAME render
        sudo gpasswd -d $LOGNAME video
        ```
