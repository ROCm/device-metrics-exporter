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

// needed only for v1.4.0
if [ "$DEPLOYMENT" == "container" ]; then
    # Check if tar is installed
    if ! command -v tar &> /dev/null; then
        echo "tar is not installed. Installing tar package..."
        if command -v microdnf &> /dev/null; then
            microdnf update -y  && microdnf install -y tar
            if [ $? -ne 0 ]; then
                echo "Failed to install tar package"
                exit 1
            fi
        else
            echo "Error: microdnf is not available to install tar"
            exit 1
        fi
    fi
fi
# Collect dmesg logs
dmesg > dmesg.log 2>/dev/null
if [ $? -eq 0 ]; then
    LOG_FILES+=("dmesg.log")
fi

# Collect AMD SMI diagnostics if available
if command -v amd-smi &> /dev/null; then
    echo "Collecting AMD SMI diagnostics..."

    # Define AMD SMI commands to collect
    AMD_SMI_COMMANDS=(
        "amd-smi version"
        "amd-smi list"
        "amd-smi static"
        "amd-smi firmware"
        "amd-smi metric"
        "amd-smi topology"
        "amd-smi process"
        "amd-smi xgmi"
        "amd-smi partition"
    )

    # Execute each command and save to separate log files
    for cmd in "${AMD_SMI_COMMANDS[@]}"; do
        # Create filename from command (replace spaces and special chars)
        log_filename=$(echo "$cmd" | tr ' ' '_' | tr -d '-' | tr -d '/')
        log_filename="${log_filename}.log"

        echo "Running: $cmd"
        $cmd > "$log_filename" 2>&1

        # Add to log files array if command succeeded or produced output
        if [ -s "$log_filename" ]; then
            LOG_FILES+=("$log_filename")
        else
            rm -f "$log_filename"
        fi
    done
fi

# Collect metricsclient diagnostics if available
if command -v metricsclient &> /dev/null; then
    echo "Collecting metricsclient diagnostics..."

    # Define metricsclient commands to collect
    METRICSCLIENT_COMMANDS=(
        "metricsclient"
        "metricsclient -gpu"
        "metricsclient -gpuctl"
        "metricsclient -label"
        "metricsclient -npod"
        "metricsclient -pod"
    )

    # Execute each command and save to separate log files
    for cmd in "${METRICSCLIENT_COMMANDS[@]}"; do
        # Create filename from command (replace spaces and special chars)
        log_filename=$(echo "$cmd" | tr ' ' '_' | tr -d '-' | tr -d '/')
        log_filename="${log_filename}.log"

        echo "Running: $cmd"
        $cmd > "$log_filename" 2>&1

        # Add to log files array if command succeeded or produced output
        if [ -s "$log_filename" ]; then
            LOG_FILES+=("$log_filename")
        else
            rm -f "$log_filename"
        fi
    done
fi

# Collect gpuctl diagnostics if available
if command -v gpuctl &> /dev/null; then
    echo "Collecting gpuctl diagnostics..."

    gpuctl_log="gpuctl_show_gpu_all.log"
    echo "Running: gpuctl show gpu all"
    gpuctl show gpu all > "$gpuctl_log" 2>&1

    # Add to log files array if command succeeded or produced output
    if [ -s "$gpuctl_log" ]; then
        LOG_FILES+=("$gpuctl_log")
    else
        rm -f "$gpuctl_log"
    fi
fi

# Collect metrics endpoint output
METRICS_LOG="metrics_endpoint.log"
echo "Collecting metrics from localhost:5000/metrics..."
curl -s localhost:5000/metrics > "$METRICS_LOG" 2>&1
CURL_EXIT_CODE=$?

if [ $CURL_EXIT_CODE -ne 0 ]; then
    echo "Failed to collect metrics from endpoint (exit code: $CURL_EXIT_CODE)" >> "$METRICS_LOG"
    echo "curl command failed with exit code: $CURL_EXIT_CODE" >> "$METRICS_LOG"
    echo "Error: Failed to collect metrics from localhost:5000/metrics"
fi

# Add metrics log to archive regardless of success/failure
if [ -s "$METRICS_LOG" ]; then
    LOG_FILES+=("$METRICS_LOG")
else
    # Create a log file with failure message if empty
    echo "No output received from metrics endpoint" > "$METRICS_LOG"
    LOG_FILES+=("$METRICS_LOG")
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
