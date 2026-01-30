=================================
Standalone Debian Package Install
=================================

System Requirements
===================

Before installing the AMD GPU Metrics Exporter, you need to install the following:

- **Operating System**: Ubuntu 22.04 or Ubuntu 24.04

Each Debian package release of the Standalone Metrics Exporter is dependent on a specific version of the ROCm amdgpu driver. Please see table below for more information:

.. list-table::
   :header-rows: 1

   * - Metrics Exporter Debian Version
     - ROCm Version
     - AMDGPU Driver Version
   * - amdgpu-exporter-1.2.0
     - ROCm 6.3.x
     - 6.10.5
   * - amdgpu-exporter-1.3.1
     - ROCm 6.4.x
     - 6.12.12
   * - amdgpu-exporter-1.4.0.1
     - ROCm 7.0.x
     - 6.14.x

Installation
===================

Step 1: Install System Prerequisites
------------------------------------

1. Install Linux headers and modules:

   .. code-block:: bash

      sudo apt update
      sudo apt install "linux-headers-$(uname -r)" "linux-modules-extra-$(uname -r)"

2. Add user to required groups:

   .. code-block:: bash

      sudo usermod -a -G render,video $LOGNAME 

Step 2: Install AMDGPU Driver
------------------------------

.. note::
   For the most up-to-date information on installing dkms drivers please see the `ROCm Install Quick Start <https://rocm.docs.amd.com/projects/install-on-linux/en/latest/install/quick-start.html>`_ page. The below instructions are the most current instructions as of ROCm 7.0.rc1.

1. Download the driver from the Radeon repository (`repo.radeon.com <https://repo.radeon.com/amdgpu-install>`_) for your operating system. For example if you want to get the latest ROCm 7.0.0 drivers for Ubuntu 22.04 you would run the following command:

   .. code-block:: bash

      wget https://repo.radeon.com/amdgpu-install/7.0/ubuntu/jammy/amdgpu-install_7.0.70000-1_all.deb
      sudo apt install ./amdgpu-install_7.0.70000-1_all.deb
      sudo apt update

   Please note that the above url will be different depending on what version of the drivers you will be installing and type of Operating System you are using.

2. Install the driver:

   .. code-block:: bash

      sudo apt install amdgpu-dkms
      sudo reboot

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

   .. tab-set::

      .. tab-item:: ubuntu 22.04

         .. code-block:: bash

            deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/apt/1.4.0.1 jammy main

      .. tab-item:: ubuntu 24.04

         .. code-block:: bash

            deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/apt/1.4.0.1 noble main


3. Update the package list again:

   .. code-block:: bash

      sudo apt update

Step 4: Install Metrics Exporter
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

Changing configuration config.json
----------------------------------

If you need to customize ports or settings:


1. Edit the amd-metrics-exporter service file:

   .. code-block:: bash

      sudo vi /lib/systemd/system/amd-metrics-exporter.service

2. Update the `ExecStart` line to read in the config.json file:

   .. code-block:: bash

      ExecStart=/usr/local/bin/amd-metrics-exporter -amd-metrics-config /etc/metrics/config.json

3. Reload systemd:

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


3. Restart GPU Agent service:

   .. code-block:: bash

      sudo systemctl restart gpuagent.service
      sudo systemctl daemon-reload

Change Metrics Exporter Port
----------------------------

1. Edit the configuration file:

   .. code-block:: bash

      sudo vi /etc/metrics/config.json

2. Update `ServerPort` to your desired port.

Change Metrics Exporter Port Connecting to GPU Agent
---------------------------------------------------

1. Edit the Metrics Exporter service file:
   .. code-block:: bash

      sudo vi /lib/systemd/system/amd-metrics-exporter.service

2. Update `ExecStart` with desired port:

   .. code-block:: bash

      ExecStart=/usr/local/bin/amd-metrics-exporter -agent-grpc-port <port_number>

3. Restart Metrics Exporter service:

   .. code-block:: bash

      sudo systemctl restart amd-metrics-exporter.service
      sudo systemctl daemon-reload

Stop Metrics Exporter
---------------------

To stop the Metrics Exporter service, run:
   .. code-block:: bash

      sudo systemctl stop amd-metrics-exporter.service
      sudo systemctl stop gpuagent.service 
      sudo systemctl daemon-reload

Confirm Metrics Exporter is Running
------------------------------------

To confirm that the Metrics Exporter is running and accessible, you can use the following command:

   .. code-block:: bash

      systemctl status amd-metrics-exporter.service
      systemctl status gpuagent.service


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
