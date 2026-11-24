======================================
Standalone Debian package installation
======================================

For SR-IOV/GIM hypervisor hosts, see :doc:`deb-package-sriov` instead.

System requirements
===================

Before you install AMD Device Metrics Exporter, install the AMD GPU driver (``amdgpu``) from the Radeon repository.
Make sure that your system meets the following requirements:

- **Operating System**: Ubuntu 22.04 or Ubuntu 24.04
- **ROCm / AMDGPU Driver**: ROCm 6.2.0 or later (includes the compatible AMDGPU driver)

Installation
===================

Step 1: Install system prerequisites
------------------------------------

1. Install Linux headers and modules:

   .. code-block:: bash

      sudo apt update
      sudo apt install "linux-headers-$(uname -r)" "linux-modules-extra-$(uname -r)"

2. Add user to required groups:

   .. code-block:: bash

      sudo usermod -a -G render,video $LOGNAME 

Step 2: Install the AMD GPU driver
----------------------------------

.. note::
   For the latest information about installing the DKMS driver, see the `ROCm installation quick start <https://rocm.docs.amd.com/projects/install-on-linux/en/latest/install/quick-start.html>`_.

1. Download the driver from the Radeon repository (`repo.radeon.com <https://repo.radeon.com/amdgpu-install>`_) for your operating system. For example, to install ROCm 7.2.3 on Ubuntu 22.04:

   .. code-block:: bash

      wget https://repo.radeon.com/amdgpu-install/7.2.3/ubuntu/jammy/amdgpu-install_7.2.3.70203-1_all.deb
      sudo apt install ./amdgpu-install_7.2.3.70203-1_all.deb
      sudo apt update

   For other ROCm versions, browse `repo.radeon.com/amdgpu-install <https://repo.radeon.com/amdgpu-install>`_ to find the correct ``.deb`` for your OS.

2. Install the driver:

   .. code-block:: bash

      sudo apt install amdgpu-dkms
      sudo reboot

3. Load the driver module:

   .. code-block:: bash

      sudo modprobe amdgpu

Step 3: Install the APT prerequisites for the exporter
------------------------------------------------------

1. Update the package list and install necessary tools, keyrings and keys:

   .. code-block:: bash

      # Install necessary tools  
      sudo apt update
      sudo apt install vim wget gpg

      # Create the keyrings directory with the appropriate permissions:
      sudo mkdir --parents --mode=0755 /etc/apt/keyrings

      # Download the ROCm GPG key and add it to the keyrings:
      wget https://repo.radeon.com/rocm/rocm.gpg.key -O - | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg > /dev/null

2. Add the Device Metrics Exporter repository to the APT sources:

   .. tab-set::

      .. tab-item:: Ubuntu 22.04

         .. code-block:: bash

            echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/apt/1.5.5 jammy main" \
              | sudo tee /etc/apt/sources.list.d/device-metrics-exporter.list

      .. tab-item:: Ubuntu 24.04

         .. code-block:: bash

            echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/apt/1.5.5 noble main" \
              | sudo tee /etc/apt/sources.list.d/device-metrics-exporter.list


3. Update the package list again:

   .. code-block:: bash

      sudo apt update

Step 4: Install the exporter
----------------------------

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

.. note::
   Stop the services before you unload or upgrade the GPU driver or change GPU partitions. See `Service management for driver and partition operations`_.

Default settings
================

- **Metrics endpoint:** ``http://localhost:5000/metrics``
- **Configuration file:** ``/etc/metrics/config.json``
- **GPU Agent socket:** ``/var/run/gpuagent.sock`` (Unix Domain Socket)

To change the HTTP port, set the ``ServerPort`` field in the configuration file.

Custom configuration
====================

Using a different configuration file
------------------------------------

The exporter reads ``/etc/metrics/config.json`` by default. To use a configuration file at a different path:

1. Edit the ``amd-metrics-exporter`` service file:

   .. code-block:: bash

      sudo vi /lib/systemd/system/amd-metrics-exporter.service

2. Update the ``ExecStart`` line to point to your configuration file:

   .. code-block:: bash

      ExecStart=/usr/local/bin/amd-metrics-exporter -amd-metrics-config /path/to/config.json

3. Reload systemd and restart the exporter:

   .. code-block:: bash

      sudo systemctl daemon-reload
      sudo systemctl restart amd-metrics-exporter.service

Changing the GPU Agent socket path (advanced)
---------------------------------------------

By default, GPU Agent uses the Unix domain socket ``/var/run/gpuagent.sock`` for communication with the metrics exporter.

To change the socket path:

1. Edit the GPU Agent service file:

   .. code-block:: bash

      sudo vi /lib/systemd/system/gpuagent.service

2. Update ``ExecStart`` with the custom socket path:

   .. code-block:: bash

      ExecStart=/usr/local/bin/gpuagent -s /path/to/custom.sock

3. Edit the Metrics Exporter service file:

   .. code-block:: bash

      sudo vi /lib/systemd/system/amd-metrics-exporter.service

4. Update ``ExecStart`` to use the same socket path:

   .. code-block:: bash

      ExecStart=/usr/local/bin/amd-metrics-exporter -s /path/to/custom.sock

5. Reload systemd and restart both services:

   .. code-block:: bash

      sudo systemctl daemon-reload
      sudo systemctl restart gpuagent.service
      sudo systemctl restart amd-metrics-exporter.service

Changing the exporter port
--------------------------

1. Edit the configuration file:

   .. code-block:: bash

      sudo vi /etc/metrics/config.json

2. Set ``ServerPort`` to the port you want to use.

3. Restart the exporter service:

   .. code-block:: bash

      sudo systemctl restart amd-metrics-exporter.service

Stopping the exporter
---------------------

To stop the exporter and GPU Agent services, run:

.. code-block:: bash

   sudo systemctl stop amd-metrics-exporter.service
   sudo systemctl stop gpuagent.service

Confirming the exporter is running
----------------------------------

To confirm that the exporter and GPU Agent services are running, run:

.. code-block:: bash

   systemctl status amd-metrics-exporter.service
   systemctl status gpuagent.service

To confirm that the metrics endpoint responds, run:

.. code-block:: bash

   curl http://localhost:5000/metrics

Service management for driver and partition operations
------------------------------------------------------

The exporter and GPU Agent services must be stopped before performing the following operations:

- GPU driver unload or upgrade
- GPU partition configuration changes

**Required Steps:**

1. Stop both services. See `Stopping the exporter`_.
2. Upgrade the driver or change the partitions.
3. Start both services with the enable and start commands from `Step 4: Install the exporter`_.
4. Verify that both services are running. See `Confirming the exporter is running`_.

Removing the exporter and other components
------------------------------------------

To remove the exporter and, optionally, the components it depends on:

1. Uninstall the Metrics Exporter:

   - Remove the package and its configuration files:

     .. code-block:: bash

        sudo apt purge amdgpu-exporter

2. (Optional) Uninstall the AMD GPU driver:

   - Remove the DKMS driver package and the ``amdgpu-install`` repository package:

     .. code-block:: bash

        sudo apt purge amdgpu-dkms amdgpu-install
        sudo apt autoremove

   - Unload the driver module:

     .. code-block:: bash

        sudo modprobe -r amdgpu

3. (Optional) Remove the system prerequisites:

   - Remove Linux header and module packages:

     .. code-block:: bash

        sudo apt remove linux-headers-$(uname -r)
        sudo apt remove linux-modules-extra-$(uname -r)

   - Remove the user from groups:

     .. code-block:: bash

        sudo gpasswd -d $LOGNAME render
        sudo gpasswd -d $LOGNAME video
