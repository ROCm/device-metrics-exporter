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

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// DefaultTestTimeout is default test timeout
const DefaultTestTimeout = 600 // 10 min

// DefaultTestIteration is the default number of iteration to run test
const DefaultTestIteration = 1

// TestResults is map where test name is key and value is result
type TestResults map[string]TestResult

// IterationResult contains the result for each test iteration
type IterationResult struct {
	Number       uint32                 `json:"number,omitempty"`
	Stdout       string                 `json:"stdout,omitempty"`
	Stderr       string                 `json:"stderr,omitempty"`
	SuitesResult map[string]TestResults `json:"suitesResult,omitempty"`
	Status       CommandStatus          `json:"status,omitempty"`
}

// ResultParser is a generic function which can be used to develop parser for different test frameworks
type ResultParser func(val string) (map[string]TestResults, error)

// TOption fills the optional params for Test Handler
type TOption func(*TestHandler)

// TestWithTimeout passes a timeout to the Test handler
func TestWithTimeout(timeout uint) TOption {
	return func(th *TestHandler) {
		th.timeout = timeout
	}
}

// TestWithLogFilePath sets the log file for the current test execution
func TestWithLogFilePath(logFilePath string) TOption {
	return func(th *TestHandler) {
		th.logFilePath = logFilePath
	}
}

// TestWithResultParser sets the Result parser
func TestWithResultParser(parser ResultParser) TOption {
	return func(th *TestHandler) {
		th.parser = parser
	}
}

// TestWithIteration sets the iterations count
func TestWithIteration(iterations uint32) TOption {
	return func(th *TestHandler) {
		th.iterations = iterations
	}
}

// TestWithStopOnFailure sets the stop on failure option
func TestWithStopOnFailure(stopOnFailure bool) TOption {
	return func(th *TestHandler) {
		th.stopOnFailure = stopOnFailure
	}
}

// TestHandler runs a given test CLI
type TestHandler struct {
	testname      string
	args          []string
	process       *exec.Cmd
	stdout        bytes.Buffer
	stderr        bytes.Buffer
	cancelFunc    context.CancelFunc
	wg            sync.WaitGroup
	logger        *log.Logger
	timeout       uint
	logFilePath   string
	status        CommandStatus
	rwLock        sync.RWMutex
	result        map[string]TestResults // gpuid -> test1 - pass, test2- fail
	doneChan      chan struct{}
	parser        ResultParser
	iterations    uint32
	IterResults   []*IterationResult
	stopTest      bool
	stopOnFailure bool
}

// NewTestHandler returns instance of TestHandler
func NewTestHandler(testname string, logger *log.Logger, args []string, opts ...TOption) TestHandlerInterface {
	hldr := &TestHandler{
		testname:   testname,
		args:       args,
		wg:         sync.WaitGroup{},
		logger:     logger,
		iterations: DefaultTestIteration,
		timeout:    DefaultTestTimeout,
		status:     TestNotStarted,
		rwLock:     sync.RWMutex{},
		doneChan:   make(chan struct{}),
		parser:     defaultParser,
	}

	for _, o := range opts {
		o(hldr)
	}

	return hldr
}

// StartTest starts the CLI execution
func (th *TestHandler) StartTest() error {
	if th.iterations == 0 {
		return fmt.Errorf("iterations must be greater than 0")
	}
	th.wg.Add(1)
	go th.runTest()
	return nil
}

func (th *TestHandler) runTest() {
	defer th.wg.Done()
	iterationDoneChan := make(chan struct{}) // Separate channel for iteration signaling

	closeFunc := func() {
		close(iterationDoneChan)
		th.doneChan <- struct{}{}
	}

	th.setStatus(TestRunning)
	defer th.setStatus(TestCompleted)
	for i := uint32(1); i <= th.iterations; i++ {
		th.rwLock.Lock()
		logger.Log.Printf("Starting iteration %d of %d for test: %v", i, th.iterations, th.testname)
		logger.Log.Printf("cmd %v args: %+v \n", th.args[0], th.args[1:])
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(th.timeout)*time.Second)
		th.process = exec.CommandContext(ctx, th.args[0], th.args[1:]...)
		th.process.Stdout = &th.stdout
		th.process.Stderr = &th.stderr
		th.cancelFunc = cancel
		th.rwLock.Unlock()

		if err := th.process.Start(); err != nil {
			logger.Log.Printf("test %v [iteration=%d] failed to start: %v", th.testname, i, err)
			continue
		}

		th.setStatus(TestRunning)
		logger.Log.Printf("test %v [iteration=%d, pid=%v] started running \n", th.testname, i, th.process.Process.Pid)
		th.wg.Add(1)

		go func(iter uint32) {
			defer th.wg.Done()
			err := th.process.Wait()
			if err != nil {
				logger.Log.Printf("cmd %v [iteration=%d, pid=%v] exited with error: %v \n", th.testname, iter, th.process.Process.Pid, err)
			} else {
				logger.Log.Printf("cmd %v [iteration=%d, pid=%v] completed successfully \n", th.testname, iter, th.process.Process.Pid)
			}
			iterationDoneChan <- struct{}{}
			th.cancelCtx()
		}(i)

		// Wait for the command to finish or timeout
		select {
		case <-iterationDoneChan:
			// Process completed successfully or with an error
			logger.Log.Printf("writing logs %v [iteration=%d]: \n", th.testname, i)
			err := th.logResults(i, TestCompleted)
			if err != nil {
				logger.Log.Printf("err: %v", err)
				closeFunc()
				return
			}
		case <-ctx.Done():
			// Timeout occurred
			logger.Log.Printf("cmd %v [iteration=%d] timed out", th.testname, i)
			// wait for the command routine to complete
			<-iterationDoneChan
			err := th.logResults(i, TestTimedOut)
			if err != nil {
				logger.Log.Printf("err: %v", err)
				closeFunc()
				return
			}
			if th.stopTest {
				closeFunc()
				return
			}
		}

		// Reset buffers for the next iteration
		th.stdout.Reset()
		th.stderr.Reset()
	}
	// Close the iteration signaling channel
	closeFunc()
}

func (th *TestHandler) logResults(i uint32, status CommandStatus) error {
	res, err := th.parser(th.stdout.String())
	if err != nil {
		logger.Log.Printf("error parsing test logs for %v err: %v", th.testname, err)
		res = make(map[string]TestResults)
	}
	th.result = res
	obj := &IterationResult{
		Stdout:       th.stdout.String(),
		Stderr:       th.stderr.String(),
		Number:       i,
		SuitesResult: res,
		Status:       status,
	}
	th.rwLock.Lock()
	th.IterResults = append(th.IterResults, obj)
	th.rwLock.Unlock()
	if th.stopOnFailure && (checkFailure(res) || status == TestTimedOut) {
		return fmt.Errorf("stopping test on failure")
	}
	return nil
}

// StopTest stops the current test execution
func (th *TestHandler) StopTest() {
	logger.Log.Printf("stop test called for %v [pid=%v]", th.testname, th.process.Process.Pid)
	th.stopTest = true
	th.cancelCtx()
	<-th.doneChan
	th.wg.Wait()
}

// cancelCtx calls the cancel function
func (th *TestHandler) cancelCtx() {
	if th.cancelFunc == nil {
		return
	}
	th.cancelFunc()
	th.cancelFunc = nil
}

// GetLogFilePath return log file path of the test command
func (th *TestHandler) GetLogFilePath() string {
	return th.logFilePath
}

// Status returns status of the test command
func (th *TestHandler) Status() CommandStatus {
	th.rwLock.RLock()
	defer th.rwLock.RUnlock()
	return th.status
}

// Result for the test
func (th *TestHandler) Result() []*IterationResult {
	th.rwLock.RLock()
	defer th.rwLock.RUnlock()
	return th.IterResults
}

// Done is used to signal completion of the test
func (th *TestHandler) Done() chan struct{} {
	return th.doneChan
}

func (th *TestHandler) setStatus(status CommandStatus) {
	th.rwLock.Lock()
	defer th.rwLock.Unlock()
	th.status = status
}

// defaultParser is default parser for test handler
func defaultParser(val string) (map[string]TestResults, error) {
	res := make(map[string]TestResults)
	return res, nil
}

// checkFailure checks if any test case failed or not
func checkFailure(res map[string]TestResults) bool {
	if len(res) == 0 {
		logger.Log.Printf("No test results found during checking for StopOnFailure, marking test as failed")
		// If no results, either test command failed to run or test parser failed
		return true
	}
	for gpu, val := range res {
		if val == nil {
			continue
		}
		for test, testResult := range val {
			if testResult == Failure {
				logger.Log.Printf("Found failure for test %v, GPU %v", test, gpu)
				return true
			}
		}
	}
	return false
}
