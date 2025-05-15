/**
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
**/

package types

// TestRunner is interface which can be implemented by different test frameworks like agfhc
type TestRunner interface {
	// GetTestHandler returns test handler with given args
	GetTestHandler(string, TestParams) (TestHandlerInterface, error)
}

// TestHandlerInterface is interface for TestHandler
type TestHandlerInterface interface {
	// StartTest
	StartTest() error

	// StopTest
	StopTest()

	// Status
	Status() CommandStatus

	// GetLogFilePath
	GetLogFilePath() string

	// Result
	Result() []*IterationResult

	// Done signals test completion
	Done() chan struct{}
}

// TestParams exposes the option to configure the CLI
type TestParams struct {
	Iterations    uint
	StopOnFailure bool
	DeviceIDs     []string
	Timeout       uint
	ExtraArgs     []string
}

// CommandStatus is enum for current status of the command
type CommandStatus string

// TestResult is enum for test result
type TestResult string

const (
	// TestRunning represents test is running
	TestRunning CommandStatus = "running"
	// TestCompleted represents test is completed
	TestCompleted CommandStatus = "completed"
	// TestNotStarted represents test is not started yet
	TestNotStarted CommandStatus = "not_started"
	// TestTimedOut represents test timed out
	TestTimedOut CommandStatus = "timed_out"
	// Success represents test passed
	Success TestResult = "success"
	// Failure represents test failed
	Failure TestResult = "failure"
	// Skipped represents test skipped
	Skipped TestResult = "skipped"
	// Cancelled represents test cancelled
	Cancelled TestResult = "cancelled"
	// Timedout represents test timedout
	Timedout TestResult = "timedout"
)

// String convert command status into string
func (cs CommandStatus) String() string {
	return string(cs)
}

// String convert test result to string
func (tr TestResult) String() string {
	return string(tr)
}
