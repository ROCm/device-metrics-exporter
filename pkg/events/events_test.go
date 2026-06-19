/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package events

import (
	"context"
	"os"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

func TestMain(m *testing.M) {
	logger.Init(true)
	os.Exit(m.Run())
}

// Calls before Init must not panic and are log-only no-ops.
func TestPackageFuncs_BeforeInit_Noop(t *testing.T) {
	Stop() // ensure no singleton
	EmitWarning(context.Background(), AgentUnreachable, "x")
	if err := EmitWarningSync(context.Background(), AgentUnreachable, "x"); err != nil {
		t.Fatalf("EmitWarningSync before Init: want nil err, got %v", err)
	}
}

// Init with a nil client yields a log-only service that does not panic; Stop
// cleans it up.
func TestInit_NilClient_LogOnly(t *testing.T) {
	Init(nil, nil, func(int) {})
	defer Stop()
	EmitWarning(context.Background(), ProfilerDisabled, "x")
	if err := EmitWarningSync(context.Background(), HealthValidationFailed, "y"); err != nil {
		t.Fatalf("EmitWarningSync nil client: want nil err, got %v", err)
	}
}
