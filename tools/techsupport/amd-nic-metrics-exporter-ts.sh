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
# limitations under the License.
#

# collect tech support on a non k8s container or debian deployment
# usage:
#    amd-nic-metrics-exporter-ts.sh

capture_time() {
    local start
    start=$(date +%s%3N)

    "$@"
    local exit_code=$?

    local end
    end=$(date +%s%3N)

    printf "%s: %s, time taken: %d ms (exit: %d)\n" \
        "$(date '+%Y-%m-%d %H:%M:%S')" \
        "$*" \
        "$(( end - start ))" \
        "$exit_code" >> "$TIME_LOGS"

    return $exit_code
}

DEPLOYMENT="baremetal"
SYSTEM_SERVICE_NAME="amd-nic-metrics-exporter.service"

if [ ! -f "/etc/systemd/system/$SYSTEM_SERVICE_NAME" ] && [ ! -f "/usr/lib/systemd/system/$SYSTEM_SERVICE_NAME" ]; then
    DEPLOYMENT="container"
fi


echo "Deployment type: $DEPLOYMENT"
# Initialize log files array for archiving
LOG_FILES=()
ARCHIVE_DIR="/var/log"

if [ "$DEPLOYMENT" == "baremetal" ]; then
    # Check for sudo access
    if ! sudo -n true 2>/dev/null; then
        echo "Warning: This script requires sudo access for systemd operations"
        echo "Please run with sudo or ensure passwordless sudo is configured"
        exit 1
    fi
    # Collect systemd journal logs
    journalctl -xu amd-nic-metrics-exporter > amd-nic-metrics-exporter-journalctl.log
    
    # Add log files to archive list
    LOG_FILES+=("amd-nic-metrics-exporter-journalctl.log")
fi

# Collect dmesg logs
dmesg > dmesg.log 2>/dev/null
if [ $? -eq 0 ]; then
    LOG_FILES+=("dmesg.log")
fi

TIME_LOGS="time_logs.log"
echo "Collecting timing information..." > "$TIME_LOGS"
echo "Hostname: $(hostname)" >> "$TIME_LOGS"
LOG_FILES+=("$TIME_LOGS")

# Add existing log files if they exist
# move it to a file directly instead of copying to /var/log
[ -f "/var/log/amd-nic-metrics-exporter.log" ] && LOG_FILES+=("/var/log/amd-nic-metrics-exporter.log")

# Add configuration file if it exists
[ -f "/etc/metrics/config-nic.json" ] && LOG_FILES+=("/etc/metrics/config-nic.json")

# Determine server port from config file or use default
SERVER_PORT=5001
if [ -f "/etc/metrics/config-nic.json" ]; then
    PORT_FROM_CONFIG=$(grep -oP '(?<="ServerPort": )\d+' /etc/metrics/config-nic.json)
    if [ -n "$PORT_FROM_CONFIG" ]; then
        SERVER_PORT=$PORT_FROM_CONFIG
    fi
fi

# Collect metrics endpoint output
METRICS_LOG="metrics_endpoint.log"
echo "Collecting metrics from localhost:$SERVER_PORT/metrics..."
# Use curl with -f to fail on HTTP errors and -S to show errors
if capture_time curl -fsS http://localhost:$SERVER_PORT/metrics > "$METRICS_LOG" 2>&1; then
    echo "Metrics successfully collected into $METRICS_LOG"
    CURL_EXIT_CODE=0
else
    CURL_EXIT_CODE=$?
    echo "Failed to collect metrics (exit code $CURL_EXIT_CODE). See $METRICS_LOG for details."
fi

# Add metrics log to archive regardless of success/failure
if [ -s "$METRICS_LOG" ]; then
    LOG_FILES+=("$METRICS_LOG")
else
    # Create a log file with failure message if empty
    echo "No output received from metrics endpoint" > "$METRICS_LOG"
    LOG_FILES+=("$METRICS_LOG")
fi

# Add rdma stats output
RDMA_LOG="rdma_stats.log"
if command -v rdma >/dev/null 2>&1; then
    capture_time rdma statistic -j > "$RDMA_LOG" 2>/dev/null
    if [ $? -eq 0 ]; then
        LOG_FILES+=("$RDMA_LOG")
    else
        echo "Failed to collect RDMA statistics" > "$RDMA_LOG"
        LOG_FILES+=("$RDMA_LOG")
    fi
fi

# Add ethtool stats for all interfaces and add check for vendor AMD (vendor: 1dd8)
ETHTOOL_LOG="ethtool_stats.log"
if command -v ethtool >/dev/null 2>&1; then
    for iface in $(ls /sys/class/net/); do
        VENDOR_ID=$(cat /sys/class/net/"${iface}"/device/vendor 2>/dev/null)
        if [ "$VENDOR_ID" == "0x1dd8" ]; then
            echo "Collecting ethtool stats for interface: $iface" >> "$ETHTOOL_LOG"
            capture_time ethtool -S "$iface" >> "$ETHTOOL_LOG" 2>/dev/null
            echo "" >> "$ETHTOOL_LOG" # add a new line for better readability
        fi
    done
    if [ -s "$ETHTOOL_LOG" ]; then
        LOG_FILES+=("$ETHTOOL_LOG")
    else
        echo "No AMD interfaces found for ethtool statistics" > "$ETHTOOL_LOG"
        LOG_FILES+=("$ETHTOOL_LOG")
    fi
fi

# Add nicctl port, lif and queue pair stats
NICCTL_LOG="nicctl_stats.log"
if command -v nicctl >/dev/null 2>&1; then
    capture_time nicctl show port statistics -j >> "$NICCTL_LOG" 2>/dev/null
    echo  >> "$NICCTL_LOG" # add a new line for better readability
    capture_time nicctl show lif statistics -j >> "$NICCTL_LOG" 2>/dev/null
    echo "" >> "$NICCTL_LOG"
    capture_time nicctl show rdma queue-pair statistics -j >> "$NICCTL_LOG" 2>/dev/null
    if [ $? -eq 0 ]; then
        LOG_FILES+=("$NICCTL_LOG")
    else
        echo "Failed to collect nicctl statistics" > "$NICCTL_LOG"
        LOG_FILES+=("$NICCTL_LOG")
    fi
fi

# Archive log files
ARCHIVE_NAME="amd-nic-metrics-exporter-techsupport-$(date +%Y%m%d-%H%M%S).tar.gz"
tar -czf "$ARCHIVE_DIR/$ARCHIVE_NAME" "${LOG_FILES[@]}" > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "Failed to create tech support tarball"
    exit 1
fi
echo "Tech support tarball created: $ARCHIVE_DIR/$ARCHIVE_NAME"
echo "Please provide the tech support tarball to AMD for further analysis"

# remove all the temporary log files created except /var/log/amd-nic-metrics-exporter.log and /etc/metrics/config-nic.json
for file in "${LOG_FILES[@]}"; do
    if [ "$file" != "/var/log/amd-nic-metrics-exporter.log" ] && [ "$file" != "/etc/metrics/config-nic.json" ]; then
        rm -f "$file"
    fi
done

exit 0