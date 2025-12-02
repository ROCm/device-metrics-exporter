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

package logger

import (
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	Log       *EnhancedLogger // Enhanced logger instance (now the default)
	logdir    = "/var/log/"
	logfile   = "exporter.log"
	logPrefix = "exporter "
	once      sync.Once
)

// SetLogPrefix sets prefix in the log to be exporter or testrunner
func SetLogPrefix(prefix string) {
	logPrefix = prefix
}

// SetLogFile sets the log file name
func SetLogFile(file string) {
	logfile = file
}

// SetLogDir sets the path to the directory of logs
func SetLogDir(dir string) {
	logdir = dir
}

// SetLogFilePath sets the full path to the log file
func SetLogFilePath(path string) {
	logdir = filepath.Dir(path)
	logfile = filepath.Base(path)
}

func GetLogFilePath() string {
	return filepath.Join(logdir, logfile)
}

func initLogger(console bool) {
	// Use enhanced logger as the default (and only) option
	Log = NewEnhancedLogger()
	if Log == nil {
		// This should never happen, but if it does, panic as we can't continue without logging
		panic("Failed to create enhanced logger")
	}

	Log.isConsoleMode = console

	// Configure output for enhanced logger
	if console {
		Log.logrus.SetOutput(os.Stdout)
		Log.Log = log.New(os.Stdout, logPrefix, log.Lmsgprefix)
	} else {
		if os.Getenv("LOGDIR") != "" {
			logdir = os.Getenv("LOGDIR")
		}

		logPath := GetLogFilePath()
		outfile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			// Fallback to stdout if file creation fails
			Log.logrus.SetOutput(os.Stdout)
			Log.Log = log.New(os.Stdout, logPrefix, log.Lmsgprefix)
		} else {
			Log.initLoggerRotation()
			Log.Log = log.New(outfile, logPrefix, log.Lmsgprefix)
		}
	}

	// Set default log level from environment variable if present
	if levelStr := os.Getenv("LOG_LEVEL"); levelStr != "" {
		Log.SetLogLevel(ParseLogLevel(levelStr))
	}

	Log.Log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func Init(console bool) {
	init := func() {
		initLogger(console)
	}
	once.Do(init)
}

// GetLogger returns the enhanced logger instance
func GetLogger() *EnhancedLogger {
	if Log == nil {
		Init(false) // Initialize with file logging by default
	}
	return Log
}

// Convenience functions that use the enhanced logger

// Debug logs a debug message
func Debug(args ...interface{}) {
	if Log != nil {
		Log.Debug(args...)
	}
}

// Debugf logs a formatted debug message
func Debugf(format string, args ...interface{}) {
	if Log != nil {
		Log.Debugf(format, args...)
	}
}

// Info logs an info message
func Info(args ...interface{}) {
	if Log != nil {
		Log.Info(args...)
	}
}

// Infof logs a formatted info message
func Infof(format string, args ...interface{}) {
	if Log != nil {
		Log.Infof(format, args...)
	}
}

// Warn logs a warning message
func Warn(args ...interface{}) {
	if Log != nil {
		Log.Warn(args...)
	}
}

// Warnf logs a formatted warning message
func Warnf(format string, args ...interface{}) {
	if Log != nil {
		Log.Warnf(format, args...)
	}
}

// Error logs an error message
func Error(args ...interface{}) {
	if Log != nil {
		Log.Error(args...)
	}
}

// Errorf logs a formatted error message
func Errorf(format string, args ...interface{}) {
	if Log != nil {
		Log.Errorf(format, args...)
	}
}

// Fatal logs a fatal message and exits
func Fatal(args ...interface{}) {
	if Log != nil {
		Log.Fatal(args...)
	}
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(format string, args ...interface{}) {
	if Log != nil {
		Log.Fatalf(format, args...)
	}
}
