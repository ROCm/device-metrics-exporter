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
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// EnhancedLogger wraps the Log and new logrus.Logger for enhanced features
type EnhancedLogger struct {
	Log           *log.Logger        // Standard logger for backward compatibility
	logrus        *logrus.Logger     // Enhanced logger
	logger        *lumberjack.Logger // Lumberjack logger for log rotation
	currentLevel  logrus.Level       // Current log level
	mu            sync.RWMutex       // Mutex for thread-safe operations
	isConsoleMode bool               // Whether logging to console
}

// NewEnhancedLogger creates a new enhanced logger instance
func NewEnhancedLogger() *EnhancedLogger {
	e := &EnhancedLogger{
		logrus:       logrus.New(),
		currentLevel: logrus.InfoLevel, // Default level
	}
	// Set JSON formatter for structured logging
	e.logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: time.RFC3339,
		FullTimestamp:   true,
	})
	return e

}

func (e *EnhancedLogger) initLoggerRotation() {
	logRotator := &lumberjack.Logger{
		Filename:   GetLogFilePath(), // Path to your log file
		MaxSize:    10,               // Max size in MB before rotation
		MaxBackups: 3,                // Max number of old log files to retain
		MaxAge:     7,                // Max number of days to retain old files
		Compress:   true,             // Whether to compress old log files
		LocalTime:  true,             // Use local time for timestamps in rotated filenames
	}
	e.logger = logRotator
	e.logrus.SetOutput(e.logger)
}

// SetOutput sets the output destination for the logger - only for unit test
func (e *EnhancedLogger) SetOutput(output io.Writer) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logrus.SetOutput(output)
}

// SetLogLevel sets the logging level at runtime
func (e *EnhancedLogger) SetLogLevel(level logrus.Level) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.currentLevel = level

	e.logrus.Infof("Log level set to: %s", level.String())
	e.logrus.SetLevel(level)
}

// GetLogLevel returns current log level
func (e *EnhancedLogger) GetLogLevel() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentLevel.String()
}

// SetLogRotation configures log rotation settings
func (e *EnhancedLogger) SetLogRotation(maxSize, maxBackups, maxAge int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isConsoleMode {
		return fmt.Errorf("log rotation not supported in console mode")
	}

	logRotator := &lumberjack.Logger{
		Filename:   GetLogFilePath(), // Path to your log file
		MaxSize:    maxSize,          // Max size in MB before rotation
		MaxBackups: maxBackups,       // Max number of old log files to retain
		MaxAge:     maxAge,           // Max number of days to retain old files
		Compress:   true,             // Whether to compress old log files
		LocalTime:  true,             // Use local time for timestamps in rotated filenames
	}
	// apply new log rotator
	e.logger = logRotator
	e.logrus.SetOutput(e.logger)

	// For now, just log the rotation settings for future implementation
	e.logrus.WithFields(logrus.Fields{
		"level":      e.currentLevel.String(),
		"maxSize":    maxSize,
		"maxBackups": maxBackups,
		"maxAge":     maxAge,
		"compress":   true,
	}).Info("Log rotation settings configured")

	return nil
}

// Debug logs a debug message
func (e *EnhancedLogger) Debug(args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.DebugLevel {
		e.logrus.Debug(args...)
	}
}

// Debugf logs a formatted debug message
func (e *EnhancedLogger) Debugf(format string, args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.DebugLevel {
		e.logrus.Debugf(format, args...)
	}
}

// Info logs an info message
func (e *EnhancedLogger) Info(args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.InfoLevel {
		e.logrus.Info(args...)
	}
}

// Infof logs a formatted info message
func (e *EnhancedLogger) Infof(format string, args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.InfoLevel {
		e.logrus.Infof(format, args...)
	}
}

// Warn logs a warning message
func (e *EnhancedLogger) Warn(args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.WarnLevel {
		e.logrus.Warn(args...)
	}
}

// Warnf logs a formatted warning message
func (e *EnhancedLogger) Warnf(format string, args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.WarnLevel {
		e.logrus.Warnf(format, args...)
	}
}

// Error logs an error message
func (e *EnhancedLogger) Error(args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.ErrorLevel {
		e.logrus.Error(args...)
	}
}

// Errorf logs a formatted error message
func (e *EnhancedLogger) Errorf(format string, args ...interface{}) {
	e.mu.RLock()
	currentLevel := e.currentLevel
	e.mu.RUnlock()

	if currentLevel <= logrus.ErrorLevel {
		e.logrus.Errorf(format, args...)
	}
}

// Fatal logs a fatal message and exits
func (e *EnhancedLogger) Fatal(args ...interface{}) {
	e.logrus.Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits
func (e *EnhancedLogger) Fatalf(format string, args ...interface{}) {
	e.logrus.Fatalf(format, args...)
}

// WithField adds a field to the log entry
func (e *EnhancedLogger) WithField(key string, value interface{}) *logrus.Entry {
	return e.logrus.WithField(key, value)
}

// WithFields adds multiple fields to the log entry
func (e *EnhancedLogger) WithFields(fields logrus.Fields) *logrus.Entry {
	return e.logrus.WithFields(fields)
}

// Backward compatibility methods to maintain existing Log interface
// Printf logs a formatted message (for backward compatibility)
func (e *EnhancedLogger) Printf(format string, args ...interface{}) {
	e.Infof(format, args...)
}

// Print logs a message (for backward compatibility)
func (e *EnhancedLogger) Print(args ...interface{}) {
	e.Info(args...)
}

// Println logs a message with newline (for backward compatibility)
func (e *EnhancedLogger) Println(args ...interface{}) {
	e.Info(args...)
}

// Close closes the logger and any open file handles
func (e *EnhancedLogger) Close() error {
	// nolint:errcheck
	// no resources to close currently
	return nil
}

// DefaultLogConfig returns a default log configuration
func DefaultLogConfig() *exportermetrics.LoggingConfig {
	return &exportermetrics.LoggingConfig{
		Level:         exportermetrics.LogLevel_INFO.String(),
		MaxFileSizeMB: 10, // 10MB
		MaxBackups:    3,
		MaxAgeDays:    7, // 7 days
	}
}

// ConfigureFromConfig configures the logger from a LogConfig struct
func (e *EnhancedLogger) ConfigureFromConfig(config *exportermetrics.LoggingConfig) error {
	// Set log level (this method has its own locking)
	SetLogLevelFromString(config.Level)

	// Configure log rotation if not in console mode (this method has its own locking)
	if !e.isConsoleMode {
		return e.SetLogRotation(int(config.MaxFileSizeMB), int(config.MaxBackups), int(config.MaxAgeDays))
	}

	return nil
}

// ParseLogLevel parses a string to LogLevel (logrus compatible)
func ParseLogLevel(level string) logrus.Level {
	switch strings.ToLower(level) {
	case "DEBUG", "debug":
		return logrus.DebugLevel
	case "INFO", "info":
		return logrus.InfoLevel
	case "WARN", "warn", "WARNING", "warning":
		return logrus.WarnLevel
	case "ERROR", "error":
		return logrus.ErrorLevel
	case "FATAL", "fatal":
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}

// SetLogLevelFromString sets log level from string (global convenience function)
func SetLogLevelFromString(level string) {
	if Log != nil {
		Log.SetLogLevel(ParseLogLevel(level))
	}
}
