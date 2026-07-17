=======================================
SR-IOV Debian Package Install
=======================================

The ``amdgpu-exporter-sriov`` Debian package installs the Device Metrics
Exporter and a matching GPU Agent build on an SR-IOV/GIM hypervisor host for
MI-series platforms. It is versioned independently from the baremetal
``amdgpu-exporter`` package (see :doc:`deb-package`).

System Requirements
===================

- **Operating System**: Ubuntu 22.04 or Ubuntu 24.04
- **Platform**: MI-series GPU configured for SR-IOV/GIM virtualization
- **GIM driver**: version 9.0.0.K or later must already be installed on the
  hypervisor host before installing this package. See the
  `MxGPU-Virtualization releases <https://github.com/amd/MxGPU-Virtualization/releases>`_
  page for GIM driver installation instructions.

Installation
===================

Step 1: Install the APT Prerequisites for Metrics Exporter
-------------------------------------------------------------

1. Update the package list and install necessary tools, keyrings and keys:

   .. code-block:: bash

      sudo apt update
      sudo apt install vim wget gpg

      sudo mkdir --parents --mode=0755 /etc/apt/keyrings

      wget https://repo.radeon.com/rocm/rocm.gpg.key -O - | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg > /dev/null

2. Edit the sources list to add the Device Metrics Exporter repository:

   .. tab-set::

      .. tab-item:: ubuntu 22.04

         .. code-block:: bash

            deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/sriov/1.0.0 jammy main

      .. tab-item:: ubuntu 24.04

         .. code-block:: bash

            deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/device-metrics-exporter/sriov/1.0.0 noble main

3. Update the package list again:

   .. code-block:: bash

      sudo apt update

Step 2: Install the SR-IOV Metrics Exporter
------------------------------------------------------

1. Install the SR-IOV Device Metrics Exporter:

   .. code-block:: bash

      sudo apt install amdgpu-exporter-sriov

   This installs two systemd units:

   - ``gpuagent-sriov.service`` — the SR-IOV/GIM GPU Agent build
   - ``amd-metrics-exporter-sriov.service`` — the metrics exporter (``Wants=``/``After=`` ``gpuagent-sriov.service``)

2. Enable and start both services:

   .. code-block:: bash

      sudo systemctl enable gpuagent-sriov.service amd-metrics-exporter-sriov.service
      sudo systemctl start gpuagent-sriov.service amd-metrics-exporter-sriov.service

3. Check service status:

   .. code-block:: bash

      sudo systemctl status gpuagent-sriov.service
      sudo systemctl status amd-metrics-exporter-sriov.service

Metrics Exporter Default Settings
====================================

- **Configuration file:** ``/etc/metrics/config.json``
- **GPU Agent socket:** ``/var/run/gpuagent.sock`` (Unix Domain Socket)
- **GPU Agent binary:** ``gpuagent-sriov`` (SR-IOV/GIM build, distinct from the baremetal ``gpuagent``)
- Health monitoring is disabled by default in this package's config
- See the **Hypervisor** column in the :doc:`../configuration/metricslist` for the list of metrics supported under this deployment

Removing the SR-IOV Metrics Exporter
------------------------------------------------

1. Stop both services:

   .. code-block:: bash

      sudo systemctl stop amd-metrics-exporter-sriov.service
      sudo systemctl stop gpuagent-sriov.service

2. Remove the package:

   .. code-block:: bash

      sudo dpkg -r amdgpu-exporter-sriov
      sudo apt-get purge amdgpu-exporter-sriov

Troubleshooting
------------------------------------------------

Techsupport Collection
~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: bash

   sudo metrics-exporter-ts.sh

Logs
~~~~

.. code-block:: bash

   sudo journalctl -xu amd-metrics-exporter-sriov
   sudo journalctl -xu gpuagent-sriov

Common Issues
~~~~~~~~~~~~~

1. ``amd-metrics-exporter-sriov.service`` fails to start:

   - Confirm ``gpuagent-sriov.service`` is running first — the exporter
     service ``Wants=``/``After=`` it
   - Verify the GIM driver (9.0.0.K or later) is installed on the host

2. Metric collection issues:

   - Check that the GIM driver version meets the minimum requirement
   - Confirm the host is actually configured for SR-IOV/GIM virtualization
