//go:build mock
// +build mock

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

package cmdexec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

const (
	// DefaultMockDataDir is the default directory for mock data files
	DefaultMockDataDir = "/mockdata/nic"
	// EnvMockDataDir is the environment variable to specify the mock data directory
	EnvMockDataDir = "MOCKDATA_DIR"
	// CommandMappingsFile is the JSON file that maps commands to mock data files
	CommandMappingsFile = "command_mappings.json"
)

// MockCommandExecuter is a mock implementation that returns responses from files
// for specific commands. For unknown commands, it returns an error.
type MockCommandExecuter struct {
	mockDataDir string
	responses   map[string]string
}

// NewExecuter creates and returns a new MockCommandExecuter instance.
// It uses the MOCKDATA_DIR environment variable if set, otherwise uses /mockdata/nic as default.
func NewExecuter() CommandExecuter {
	mockDir := os.Getenv(EnvMockDataDir)
	if mockDir == "" {
		mockDir = DefaultMockDataDir
	}

	executer := &MockCommandExecuter{
		mockDataDir: mockDir,
		responses:   make(map[string]string),
	}

	// Load responses from mock data files
	if err := executer.loadResponses(); err != nil {
		// If loading fails, log the error but continue with empty responses
		// This allows the executer to still work with AddResponse
		fmt.Fprintf(os.Stderr, "Warning: failed to load responses: %v\n", err)
	}

	return executer
}

// loadCommandMappings loads the command-to-file mappings from the JSON configuration file
// and returns the map.
func (m *MockCommandExecuter) loadCommandMappings() (map[string]string, error) {
	configPath := filepath.Join(m.mockDataDir, CommandMappingsFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read command mappings file %s: %w", configPath, err)
	}

	var mappings map[string]string
	if err := json.Unmarshal(data, &mappings); err != nil {
		return nil, fmt.Errorf("failed to parse command mappings JSON: %w", err)
	}

	return mappings, nil
}

// loadResponses loads command mappings and reads all mock data files,
// populating the responses map with the file contents.
func (m *MockCommandExecuter) loadResponses() error {
	// Load command to file mappings
	mappings, err := m.loadCommandMappings()
	if err != nil {
		return err
	}

	// Read each file and populate responses
	for cmd, filename := range mappings {
		filePath := filepath.Join(m.mockDataDir, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Log.Infof("Failed to read mock data file %s for command '%s': %v", filePath, cmd, err)
			continue
		}
		m.responses[cmd] = string(data)
	}

	return nil
}

// Run executes a command and returns output from the corresponding mock data file.
// Returns an error for unknown commands or if the file cannot be read.
func (m *MockCommandExecuter) Run(cmd string) ([]byte, error) {
	// Remove /bin/bash -c if present
	cmd = strings.TrimPrefix(cmd, "/bin/bash -c ")
	// Trim whitespace
	cmd = strings.TrimSpace(cmd)

	// Check if there's a response for this command
	if response, ok := m.responses[cmd]; ok {
		return []byte(response), nil
	}

	return nil, fmt.Errorf("mock executer: command not found: %s", cmd)
}

// RunWithContext executes a command with context and returns output from mock data files.
// Returns an error for unknown commands or if the file cannot be read.
func (m *MockCommandExecuter) RunWithContext(ctx context.Context, cmd string) ([]byte, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return m.Run(cmd)
}

// AddResponse adds a custom response for a specific command.
// This allows dynamic configuration of mock responses during tests.
// This overrides file-based responses for the given command.
func (m *MockCommandExecuter) AddResponse(cmd string, response string) {
	m.responses[cmd] = response
}
