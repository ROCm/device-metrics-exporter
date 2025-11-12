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
	"os/exec"
	"time"
)

// ExecWithContext executes a command with a context timeout
func ExecWithContext(cmd string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "/bin/bash", "-c", cmd)
	return command.Output()
}

// ExecWithContextTimeout executes a command with a specified context timeout.
// It specifically checks if the command timed out.
func ExecWithContextTimeout(cmd string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(ctx, "/bin/bash", "-c", cmd)
	output, err := command.Output()
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
