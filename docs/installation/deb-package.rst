=================================
Standalone Debian Package Install
=================================

System Requirements
===================

Before installing the AMD GPU Metrics Exporter, you need to install the following:

- **Operating System**: Ubuntu 22.04 or Ubuntu 24.04
- **ROCm Version**: 6.4.1 (specific to each .deb pkg)

Each Debian package release of the Standalone Metrics Exporter is dependent on a specific version of the ROCm amdgpu driver. Please see table below for more information:

.. list-table::
   :header-rows: 1

   * - Metrics Exporter Debian Version
     - ROCm Version
     - AMDGPU Driver Version
   * - amdgpu-exporter-1.3.0
     - ROCm 6.4.1
     - 6.12.12

Installation
===================

Step 1: Install System Prerequisites
------------------------------------

1. Update the system:

   .. code-block:: bash

      sudo apt update
      sudo apt install "linux-headers-$(uname -r)" "linux-modules-extra-$(uname -r)"

2. Add user to required groups:

   .. code-block:: bash

      sudo usermod -a -G render,video $LOGNAME 

Step 2: Install AMDGPU Driver
------------------------------

.. note::
   For the most up-to-date information on installing dkms drivers please see the `ROCm Install Quick Start <https://rocm.docs.amd.com/projects/install-on-linux/en/latest/install/quick-start.html>`_ page. The below instructions are the most current instructions as of ROCm 6.4.1.

1. Download the driver from the Radeon repository (`repo.radeon.com <https://repo.radeon.com/amdgpu-install>`_) for your operating system. For example if you want to get the latest ROCm 6.4.1 drivers for Ubuntu 22.04 you would run the following command:

   .. code-block:: bash

      wget https://repo.radeon.com/amdgpu-install/6.4.1/ubuntu/jammy/amdgpu-install_6.4.60401-1_all.deb

   Please note that the above url will be different depending on what version of the drivers you will be installing and type of Operating System you are using.

2. Install the driver:

   .. code-block:: bash

      sudo apt install ./amdgpu-install_6.4.60401-1_all.deb
      sudo apt update 
      amdgpu-install --usecase=dkms 

3. Load the driver module:

   .. code-block:: bash

      sudo modprobe amdgpu

Step 3: Install the APT Prerequisites for Metrics Exporter
-----------------------------------------------------------

1. Update the package list and install necessary tools, keyrings and keys:

   .. code-block:: bash

      # Install necessary tools  
      sudo apt update
      sudo apt install vim wget gpg

      # Create the keyrings directory with the appropriate permissions:
      sudo mkdir --parents --mode=0755 /etc/apt/keyrings

      # Download the ROCm GPG key and add it to the keyrings:
      wget https://repo.radeon.com/rocm/rocm.gpg.key -O - | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg > /dev/null

2. Edit the sources list to add the Device Metrics Exporter repository:

   **For Ubuntu 22.04**, add the following line:

   .. code-block:: bash

      deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/apt/1.3.0 jammy main

   **For Ubuntu 24.04**, add the following line:

   .. code-block:: bash

      deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/apt/1.3.0 noble main

3. Update the package list again:

   .. code-block:: bash

      sudo apt update

Step 4: Install the Prerequisites for Metrics Exporter
------------------------------------------------------

1. Install the Device Metrics Exporter:

   .. code-block:: bash

      sudo apt install amdgpu-exporter

2. Enable and start services:

   .. code-block:: bash

      sudo systemctl enable amd-metrics-exporter.service
      sudo systemctl start amd-metrics-exporter.service

3. Check service status:

   .. code-block:: bash

      sudo systemctl status amd-metrics-exporter.service

Metrics Exporter Default Settings
====================================

- **Metrics endpoint:** ``http://localhost:5000/metrics``
- **Configuration file:** ``/etc/metrics/config.json``
- **GPU Agent port (default):** ``50061``

The Exporter HTTP port is configurable via the `ServerPort` field in the configuration file.

Metrics Exporter Custom Configuration
======================================

Make changes to config.json
---------------------------

If you need to customize ports or settings:

1. Installation creates a default configuration under `/etc/metrics/config.json`. Note that you can change the path to save the config.json file into a different direct. Just be sure to also update the path in the server ExecStart command in step 3.

   .. code-block:: bash

      wget -O /etc/metrics/config.json https://raw.githubusercontent.com/ROCm/device-metrics-exporter/refs/heads/main/example/config.json

2. Make any required changes to your config.json file and ensure the port number you want to use is correct. Example of the first few lines of the config.json shown below:

   .. code-block:: bash

      {
      "ServerPort": 5000,
      "GPUConfig": {
          "Fields": [
          "GPU_NODES_TOTAL",
          "GPU_PACKAGE_POWER",
      ...
      ...

3. Edit the amd-metrics-exporter service file:

   .. code-block:: bash

      sudo vi /lib/systemd/system/amd-metrics-exporter.service

4. Update the `ExecStart` line to read in the config.json file:

   .. code-block:: bash

      ExecStart=/usr/local/bin/amd-metrics-exporter -amd-metrics-config /etc/metrics/config.json

5. Reload systemd:

   .. code-block:: bash

      sudo systemctl daemon-reload

Custom Port Configuration - Change GPU Agent Port
-------------------------------------------------

1. Edit the GPU Agent service file:

   .. code-block:: bash

      sudo vi /lib/systemd/system/gpuagent.service

2. Update `ExecStart` with desired port:

   .. code-block:: bash

      ExecStart=/usr/local/bin/gpuagent -p <port_number>

Change Metrics Exporter Port
----------------------------

1. Edit the configuration file:

   .. code-block:: bash

      sudo vi /etc/metrics/config.json

2. Update `ServerPort` to your desired port.

Removing Metrics Exporter and other components
------------------------------------------------

To remove this application, follow these commands in reverse order:

1. Uninstall the Metrics Exporter:

   - Ensure the .deb package is removed:

     .. code-block:: bash

        sudo dpkg -r amdgpu-exporter
        sudo apt-get purge amdgpu-exporter

2. (Optional) If you would also like to uninstall the AMDGPU Driver:

   - Uninstall any associated DKMS packages:

     .. code-block:: bash

        sudo dpkg -r amdgpu-install

   - Unload the driver module:

     .. code-block:: bash

        sudo modprobe -r amdgpu

3. (Optional) If you would also like to remove the system prerequisites that were installed:

   - Remove Linux header and module packages:

     .. code-block:: bash

        sudo apt remove linux-headers-$(uname -r)
        sudo apt remove linux-modules-extra-$(uname -r)

   - Remove the user from groups:

     .. code-block:: bash

        sudo gpasswd -d $LOGNAME render
        sudo gpasswd -d $LOGNAME video
