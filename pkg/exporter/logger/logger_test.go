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

package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	// Clean up before and after tests
	cleanup()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func cleanup() {
	// Reset global variables
	Log = nil
	logdir = "/var/log/"
	logfile = "exporter.log"
	logPrefix = "exporter "
	once = sync.Once{}

	// Clean up environment variables
	os.Unsetenv("LOGDIR")
	os.Unsetenv("LOG_LEVEL")
}

func TestSetLogPrefix(t *testing.T) {
	originalPrefix := logPrefix
	defer func() { logPrefix = originalPrefix }()

	testPrefix := "testrunner "
	SetLogPrefix(testPrefix)

	if logPrefix != testPrefix {
		t.Errorf("Expected log prefix %q, got %q", testPrefix, logPrefix)
	}
}

func TestSetLogFile(t *testing.T) {
	originalFile := logfile
	defer func() { logfile = originalFile }()

	testFile := "test.log"
	SetLogFile(testFile)

	if logfile != testFile {
		t.Errorf("Expected log file %q, got %q", testFile, logfile)
	}
}

func TestSetLogDir(t *testing.T) {
	originalDir := logdir
	defer func() { logdir = originalDir }()

	testDir := "/tmp/logs/"
	SetLogDir(testDir)

	if logdir != testDir {
		t.Errorf("Expected log dir %q, got %q", testDir, logdir)
	}
}

func TestSetLogFilePath(t *testing.T) {
	originalDir := logdir
	originalFile := logfile
	defer func() {
		logdir = originalDir
		logfile = originalFile
	}()

	testPath := "/tmp/logs/test.log"
	SetLogFilePath(testPath)

	expectedDir := "/tmp/logs"
	expectedFile := "test.log"

	if logdir != expectedDir {
		t.Errorf("Expected log dir %q, got %q", expectedDir, logdir)
	}
	if logfile != expectedFile {
		t.Errorf("Expected log file %q, got %q", expectedFile, logfile)
	}
}

func TestInitLoggerConsole(t *testing.T) {
	defer cleanup()

	// Capture stdout
	var buf bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	Init(true)

	// Write a test message
	if Log != nil {
		Log.Print("test message")
	}

	w.Close()
	os.Stdout = originalStdout

	// Read captured output
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}

	if Log == nil {
		t.Error("Expected Log to be initialized")
	}
}

func TestInitLoggerFile(t *testing.T) {
	defer cleanup()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	SetLogDir(tmpDir)
	SetLogFile("test.log")

	Init(false)

	if Log == nil {
		t.Error("Expected Log to be initialized")
	}

	// Test logging to file
	Log.Print("test file message")

	// Check if file was created and contains message
	logPath := filepath.Join(tmpDir, "test.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test file message") {
		t.Errorf("Expected log file to contain 'test file message', got: %s", string(content))
	}
}

func TestInitWithLogDirEnvironment(t *testing.T) {
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("LOGDIR", tmpDir)
	defer os.Unsetenv("LOGDIR")

	Init(false)

	if Log == nil {
		t.Error("Expected Log to be initialized")
	}

	// Test that the log directory was used
	Log.Print("test logdir message")

	logPath := filepath.Join(tmpDir, logfile)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Expected log file to be created at %s", logPath)
	}
}

func TestGetLogger(t *testing.T) {
	defer cleanup()

	// Enhanced logging is now always enabled, so GetLogger should always return a non-nil logger
	logger := GetLogger()
	if logger == nil {
		t.Error("Expected non-nil logger since enhanced logging is always enabled")
	}
}

func TestConvenienceFunctionsLogging(t *testing.T) {
	defer cleanup()

	Init(true) // Enhanced logging to console

	if Log == nil {
		t.Skip("Logger not available, skipping test")
	}

	// Test that all functions work with enhanced logging
	// Note: We can't easily test the actual output without more complex setup,
	// but we can verify the functions don't panic
	Debug("debug message")
	Debugf("formatted debug: %s", "test")
	Info("info message")
	Infof("formatted info: %s", "test")
	Warn("warn message")
	Warnf("formatted warn: %s", "test")
	Error("error message")
	Errorf("formatted error: %s", "test")
}

func TestInitOnlyOnce(t *testing.T) {
	defer cleanup()

	// Initialize first time
	Init(true)
	firstLogger := Log

	// Initialize second time (should be ignored due to sync.Once)
	Init(false)
	secondLogger := Log

	if firstLogger != secondLogger {
		t.Error("Expected Init to only run once due to sync.Once")
	}
}

func TestLogFileCreationFailure(t *testing.T) {
	defer cleanup()

	// Set an invalid directory (no permissions)
	SetLogDir("/root/nonexistent/")

	// Should fall back to stdout without panicking
	Init(false)

	if Log == nil {
		t.Error("Expected Log to be initialized even when file creation fails")
	}
}

func BenchmarkBasicLogging(b *testing.B) {
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "logger_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	SetLogDir(tmpDir)
	Init(false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("benchmark message")
	}
}

func BenchmarkEnhancedLogging(b *testing.B) {
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "logger_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	SetLogDir(tmpDir)
	Init(false)

	// Enhanced logging is now always available

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("benchmark message")
	}
}

func TestConcurrentLoggingAndLevelChange(t *testing.T) {
	defer cleanup()

	Init(true)

	if Log == nil {
		t.Skip("Logger not available, skipping test")
	}

	var wg sync.WaitGroup
	done := make(chan bool, 1)

	// Start goroutines that log messages continuously
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					Debugf("debug message from goroutine %d", id)
					Infof("info message from goroutine %d", id)
					Warnf("warn message from goroutine %d", id)
					Errorf("error message from goroutine %d", id)
				}
			}
		}(i)
	}

	// Start goroutines that change log level continuously
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			levels := []logrus.Level{logrus.DebugLevel, logrus.InfoLevel, logrus.WarnLevel, logrus.ErrorLevel}
			for {
				select {
				case <-done:
					return
				default:
					for _, level := range levels {
						Log.SetLogLevel(level)
						_ = Log.GetLogLevel()
					}
				}
			}
		}()
	}

	// Let them run for a short time
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()
}
func TestLogRotation(t *testing.T) {
	defer cleanup()

	// Create temporary directory for log files
	tmpDir, err := os.MkdirTemp("", "logger_rotation_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	defer func() {
		// list all files in tmpDir for debugging
		files, _ := os.ReadDir(tmpDir)
		for _, f := range files {
			t.Logf("File: %s", f.Name())
		}
	}()

	SetLogDir(tmpDir)
	SetLogFile("rotation_test.log")

	// Initialize with file logging
	Init(false)

	if Log == nil {
		t.Error("Expected Log to be initialized")
	}

	// Write some log messages
	for i := 0; i < 10; i++ {
		Log.Infof("Log message before rotation %d", i)
	}

	// Verify that the main log file exists and contains messages
	logPath := GetLogFilePath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("Expected log file to exist at %s before rotation", logPath)
	}

	// Check that the log file contains pre-rotation messages
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file before rotation: %v", err)
	}

	if !strings.Contains(string(content), "Log message before rotation") {
		t.Error("Expected log file to contain pre-rotation messages")
	}

	// Force rotation to create archive file
	err = Log.logger.Rotate()
	if err != nil {
		t.Errorf("Failed to rotate log: %v", err)
	}

	// Write more log messages after rotation
	for i := 0; i < 5; i++ {
		Log.Infof("Log message after rotation %d", i)
	}

	// Check that log files exist
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Expected main log file to exist at %s", logPath)
	}

	// Check for archived log file (lumberjack creates *.log files)
	// find files with regex rotation_test.*log in director tmpDir
	entries, _ := os.ReadDir(tmpDir)
	re := regexp.MustCompile(`rotation_test.*log`)
	files := []string{}

	for _, e := range entries {
		if !e.IsDir() && re.MatchString(e.Name()) {
			files = append(files, filepath.Join(tmpDir, e.Name()))
		}
	}

	if len(files) == 0 {
		t.Error("Expected at least one archived log file to be created after rotation")
	}

	t.Logf("list file : %v", files)

	// Verify the main log file contains post-rotation messages
	content, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "Log message after rotation") {
		t.Error("Expected main log file to contain post-rotation messages")
	}
}
