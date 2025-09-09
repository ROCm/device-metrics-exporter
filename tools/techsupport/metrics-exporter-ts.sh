#!/bin/bash
#
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
#limitations under the License.
#

# collect tech support on a non k8s container or debian deployment
# usage:
#    metrics-exporter-ts.sh

DEPLOYMENT="baremetal"
SYSTEM_SERVICE_NAME="amd-metrics-exporter.service"

if [ ! -f "/etc/systemd/system/$SYSTEM_SERVICE_NAME" ] && [ ! -f "/lib/systemd/system/$SYSTEM_SERVICE_NAME" ]; then
    DEPLOYMENT="container"
fi

echo "Deployment type: $DEPLOYMENT"
# Initialize log files array for archiving
LOG_FILES=()
ARICHVE_DIR="/var/log/"

if [ "$DEPLOYMENT" == "baremetal" ]; then
    # Check for sudo access
    if ! sudo -n true 2>/dev/null; then
        echo "Warning: This script requires sudo access for systemd operations"
        echo "Please run with sudo or ensure passwordless sudo is configured"
        exit 1
    fi
    # Collect systemd journal logs
    journalctl -xu amd-metrics-exporter > amd-metrics-exporter.log
    journalctl -xu gpuagent > amd-gpu-agent.log
    
    # Add log files to archive list
    LOG_FILES+=("amd-metrics-exporter.log")
    LOG_FILES+=("amd-gpu-agent.log")
    
    
fi

# Add existing log files if they exist
[ -f "/var/log/exporter.log" ] && LOG_FILES+=("/var/log/exporter.log")
for gpu_log in /var/log/gpu-agent*.log; do
    [ -f "$gpu_log" ] && LOG_FILES+=("$gpu_log")
done

# Add configuration file if it exists
[ -f "/etc/metrics/config.json" ] && LOG_FILES+=("/etc/metrics/config.json")

# Archive log files
ARCHIVE_NAME="amd-metrics-exporter-techsupport-$(date +%Y%m%d-%H%M%S).tar.gz"
tar -czf "$ARICHVE_DIR/$ARCHIVE_NAME" "${LOG_FILES[@]}" > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "Failed to create tech support tarball"
    exit 1
fi
echo "Tech support tarball created: $ARICHVE_DIR/$ARCHIVE_NAME"
echo "Please provide the tech support tarball to AMD for further analysis"
exit 0