/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nicagent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/nicagent/cmdexec"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

var (
	nonAMDMetricsEnabled     bool
	nonAMDMetricsEnabledOnce sync.Once
)

// ExecWithContext executes a command with a context timeout
func ExecWithContext(cmd string, cmdExec cmdexec.CommandExecuter) ([]byte, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		ms := float64(elapsed.Milliseconds()) // int64 → float64 for formatting
		if ms > 500 {                         // log only if it takes more than 500 ms
			logger.Log.Printf("ExecWithContext took %.2f ms for cmd: %s", ms, cmd)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return cmdExec.RunWithContext(ctx, cmd)
}

// ExecWithContextTimeout executes a command with a specified context timeout.
// It specifically checks if the command timed out.
func ExecWithContextTimeout(cmd string, timeout time.Duration, cmdExec cmdexec.CommandExecuter) ([]byte, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		if elapsed > 10*time.Second {
			logger.Log.Printf("ExecWithContextTimeout took %.2f seconds for cmd: %s", elapsed.Seconds(), cmd)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	output, err := cmdExec.RunWithContext(ctx, cmd)
	if err != nil {
		// if the context was cancelled due to the timeout, the error will contain
		// the DeadlineExceeded message.
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("command timed out after %s: %w", timeout, ctx.Err())
		}

		// other non-timeout execution errors (e.g., command not found, non-zero exit code)
		return nil, err
	}

	// successful execution
	return output, nil
}

// getVendor retrieves the vendor ID of the RDMA device.
func getVendor(rdmaDev string, cmdExec cmdexec.CommandExecuter) (string, error) {
	cmd := fmt.Sprintf("cat /sys/class/infiniband/%s/device/vendor", rdmaDev)
	data, err := cmdExec.Run(cmd)
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(string(data))), nil
}

// appendLabelsWithoutDuplicates appends newLabels to existingLabels, skipping any that already exist
func appendLabelsWithoutDuplicates(existingLabels []string, newLabels []string) []string {
	labelSet := make(map[string]bool)
	for _, label := range existingLabels {
		labelSet[label] = true
	}

	for _, label := range newLabels {
		if !labelSet[label] {
			existingLabels = append(existingLabels, label)
		}
	}

	return existingLabels
}

// isNonAMDMetricsEnabled checks the EXPORT_NON_AMD_NIC_METRICS environment variable once
// and caches the result. This avoids repeated os.Getenv calls in tight loops.
// NOTE: This is for testing purposes only, not intended for production use.
func isNonAMDMetricsEnabled() bool {
	nonAMDMetricsEnabledOnce.Do(func() {
		exportNonAMD := strings.ToLower(strings.TrimSpace(os.Getenv("EXPORT_NON_AMD_NIC_METRICS")))
		nonAMDMetricsEnabled = exportNonAMD == "true" || exportNonAMD == "1"
	})
	return nonAMDMetricsEnabled
}

// isVendorAllowed checks if a vendor ID is allowed for NIC metrics collection
// Reads EXPORT_NON_AMD_NIC_METRICS environment variable once at first call
// - Not set or empty: AMD only (default, backward compatible)
// - "true" or "1": All vendors allowed (for testing only)
// NOTE: EXPORT_NON_AMD_NIC_METRICS is for testing purposes only, not for production use.
func isVendorAllowed(vendorID string) bool {
	// Always allow AMD
	if vendorID == AMDVendorID {
		return true
	}

	// Check if non-AMD metrics are enabled (cached result)
	// When enabled, allow all vendors for testing purposes
	return isNonAMDMetricsEnabled()
}
