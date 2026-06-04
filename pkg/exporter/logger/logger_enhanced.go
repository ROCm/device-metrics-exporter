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
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// EnhancedLogger wraps the Log and new logrus.Logger for enhanced features
type EnhancedLogger struct {
	Log            *log.Logger        // Standard logger for backward compatibility
	*logrus.Logger                    // Enhanced logger
	logger         *lumberjack.Logger // Lumberjack logger for log rotation
	logFile        *os.File           // Log file handle when rotation is disabled
	currentLevel   logrus.Level       // Current log level
	isConsoleMode  bool               // Whether logging to console
}

// NewEnhancedLogger creates a new enhanced logger instance
func NewEnhancedLogger() *EnhancedLogger {
	e := &EnhancedLogger{
		Logger:       logrus.New(),
		currentLevel: logrus.InfoLevel, // Default level
	}
	e.SetFormatter(&logrus.TextFormatter{
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
	e.SetOutput(e.logger)
}

// logWithCaller returns a logrus.Entry with caller information
// skip is the number of stack frames to skip (typically 2 for logging methods)
func (e *EnhancedLogger) logWithCaller(skip int) *logrus.Entry {
	if _, file, line, ok := runtime.Caller(skip); ok {
		return e.Logger.WithField("func", fmt.Sprintf("%s:%d", filepath.Base(file), line))
	}
	return e.Logger.WithFields(logrus.Fields{})
}

// SetLogLevel sets the logging level at runtime
func (e *EnhancedLogger) SetLogLevel(level logrus.Level) {
	e.currentLevel = level

	e.Infof("Log level set to: %s", level.String())
	e.SetLevel(level)
}

// GetLogLevel returns current log level
func (e *EnhancedLogger) GetLogLevel() string {
	return e.currentLevel.String()
}

// SetLogRotation configures log rotation settings
func (e *EnhancedLogger) SetLogRotation(maxSize, maxBackups, maxAge int, disable bool) error {
	if e.isConsoleMode {
		return fmt.Errorf("log rotation not supported in console mode")
	}

	// Reset to default output first
	e.SetOutput(os.Stdout)

	// Close existing log file if any
	if e.logFile != nil {
		e.logFile.Close()
		e.logFile = nil
	}

	if e.logger != nil {
		e.logger.Close()
		e.logger = nil
	}

	if disable {
		// Disable log rotation by setting output to a file without rotation
		logPath := GetLogFilePath()
		outfile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		e.logFile = outfile
		e.SetOutput(e.logFile)
		e.logger = nil
		e.Info("Log rotation disabled")
		return nil
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
	e.SetOutput(e.logger)

	// For now, just log the rotation settings for future implementation
	e.WithFields(logrus.Fields{
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
	if e.currentLevel <= logrus.DebugLevel {
		e.logWithCaller(3).Debug(args...)
	}
}

// Debugf logs a formatted debug message
func (e *EnhancedLogger) Debugf(format string, args ...interface{}) {
	if e.currentLevel <= logrus.DebugLevel {
		e.logWithCaller(3).Debugf(format, args...)
	}
}

// Info logs an info message
func (e *EnhancedLogger) Info(args ...interface{}) {
	if e.currentLevel <= logrus.InfoLevel {
		e.logWithCaller(3).Info(args...)
	}
}

// Infof logs a formatted info message
func (e *EnhancedLogger) Infof(format string, args ...interface{}) {
	if e.currentLevel <= logrus.InfoLevel {
		e.logWithCaller(3).Infof(format, args...)
	}
}

// Warn logs a warning message
func (e *EnhancedLogger) Warn(args ...interface{}) {
	if e.currentLevel <= logrus.WarnLevel {
		e.logWithCaller(3).Warn(args...)
	}
}

// Warnf logs a formatted warning message
func (e *EnhancedLogger) Warnf(format string, args ...interface{}) {
	if e.currentLevel <= logrus.WarnLevel {
		e.logWithCaller(3).Warnf(format, args...)
	}
}

// Error logs an error message
func (e *EnhancedLogger) Error(args ...interface{}) {
	if e.currentLevel <= logrus.ErrorLevel {
		e.logWithCaller(3).Error(args...)
	}
}

// Errorf logs a formatted error message
func (e *EnhancedLogger) Errorf(format string, args ...interface{}) {
	if e.currentLevel <= logrus.ErrorLevel {
		e.logWithCaller(3).Errorf(format, args...)
	}
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
	if e.logFile != nil {
		err := e.logFile.Close()
		e.logFile = nil
		return err
	}

	if e.logger != nil {
		err := e.logger.Close()
		e.logger = nil
		return err
	}
	return nil
}

// DefaultLogConfig returns a default log configuration
func DefaultLogConfig() *exportermetrics.LoggingConfig {
	return &exportermetrics.LoggingConfig{
		Level:              exportermetrics.LogLevel_INFO.String(),
		MaxFileSizeMB:      10,    // 10MB
		MaxBackups:         3,     // 3 backups
		MaxAgeDays:         7,     // 7 days
		LogRotationDisable: false, // rotation enabled by default
	}
}

// ConfigureFromConfig configures the logger from a LogConfig struct
func (e *EnhancedLogger) ConfigureFromConfig(config *exportermetrics.LoggingConfig) error {
	// Set log level (this method has its own locking)
	SetLogLevelFromString(config.Level)

	// Configure log rotation if not in console mode (this method has its own locking)
	if !e.isConsoleMode {
		return e.SetLogRotation(int(config.MaxFileSizeMB), int(config.MaxBackups), int(config.MaxAgeDays), config.LogRotationDisable)
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
