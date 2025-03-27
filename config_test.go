package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "mcp-proxy-test")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name      string
		content   string
		extension string
		envVars   map[string]string
		want      *Config
		wantErr   bool
	}{
		{
			name: "Valid JSON file",
			content: `{
				"mcpServers": {
					"test-service": {
						"command": "echo",
						"args": ["hello", "world"],
						"env": {
							"TEST_ENV": "test-value"
						}
					}
				}
			}`,
			extension: ".json",
			want: &Config{
				MCPServers: map[string]ServerConfig{
					"test-service": {
						Command: "echo",
						Args:    []string{"hello", "world"},
						Env:     map[string]string{"TEST_ENV": "test-value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Valid YAML file",
			content: `mcpServers:
  test-service:
    command: echo
    args:
      - hello
      - world
    env:
      TEST_ENV: test-value`,
			extension: ".yaml",
			want: &Config{
				MCPServers: map[string]ServerConfig{
					"test-service": {
						Command: "echo",
						Args:    []string{"hello", "world"},
						Env:     map[string]string{"TEST_ENV": "test-value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Environment variable expansion test",
			content: `mcpServers:
  test-service:
    command: echo
    args:
      - hello
      - $TEST_MESSAGE
    env:
      TEST_ENV: test-value`,
			extension: ".yaml",
			envVars: map[string]string{
				"TEST_MESSAGE": "expanded-value",
			},
			want: &Config{
				MCPServers: map[string]ServerConfig{
					"test-service": {
						Command: "echo",
						Args:    []string{"hello", "expanded-value"},
						Env:     map[string]string{"TEST_ENV": "test-value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "Non-existent file",
			content:   "",
			extension: ".json",
			wantErr:   true,
		},
		{
			name:      "Invalid JSON",
			content:   `{"invalid": json}`,
			extension: ".json",
			wantErr:   true,
		},
		{
			name:      "Invalid YAML",
			content:   `invalid: - yaml`,
			extension: ".yaml",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			var path string
			if tt.content != "" {
				// Create temporary file
				filename := "config" + tt.extension
				path = filepath.Join(tempDir, filename)
				if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			} else {
				// Test for non-existent file
				path = filepath.Join(tempDir, "non-existent-file"+tt.extension)
			}

			got, err := LoadConfig(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertToMCPClientConfig(t *testing.T) {
	tests := []struct {
		name      string
		serverCfg ServerConfig
		want      *MCPClientConfig
	}{
		{
			name: "Basic conversion",
			serverCfg: ServerConfig{
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
				Env:     map[string]string{"ENV1": "val1", "ENV2": "val2"},
			},
			want: &MCPClientConfig{
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
				Env:     map[string]string{"ENV1": "val1", "ENV2": "val2"},
			},
		},
		{
			name: "Empty values",
			serverCfg: ServerConfig{
				Command: "",
				Args:    []string{},
				Env:     map[string]string{},
			},
			want: &MCPClientConfig{
				Command: "",
				Args:    []string{},
				Env:     map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertToMCPClientConfig(tt.serverCfg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertToMCPClientConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

// JSON and YAML marshalling/unmarshalling round-trip test
func TestMCPClientConfigSerialization(t *testing.T) {
	original := MCPClientConfig{
		Command: "test-command",
		Args:    []string{"arg1", "arg2"},
		Env:     map[string]string{"ENV1": "val1", "ENV2": "val2"},
	}

	// JSON serialization/deserialization test
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	var jsonDeserialized MCPClientConfig
	if err := json.Unmarshal(jsonData, &jsonDeserialized); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if !reflect.DeepEqual(original, jsonDeserialized) {
		t.Errorf("Data does not match after JSON round-trip. Original: %v, Result: %v", original, jsonDeserialized)
	}
}

// TestLogConfig tests secure logging functionality for environment variables
func TestLogConfig(t *testing.T) {
	// Create test environment variables map
	testEnv := map[string]string{
		"API_KEY":      "secret-api-key-123",
		"DATABASE_URL": "postgres://user:password@localhost:5432/db",
		"DEBUG":        "true",
	}

	// Prepare buffer to capture log output
	var logBuffer strings.Builder
	testHandler := slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	testLogger := slog.New(testHandler)

	// Execute the function under test
	LogConfig(testLogger, "test-server", testEnv)

	// Verify log output
	logOutput := logBuffer.String()

	// Verify each environment variable key is present in logs
	for key := range testEnv {
		if !strings.Contains(logOutput, key) {
			t.Errorf("Log output doesn't contain key %s", key)
		}
	}

	// Verify actual values (sensitive info) are not present in logs
	for _, value := range testEnv {
		if strings.Contains(logOutput, value) {
			t.Errorf("Log output contains raw sensitive value: %s", value)
		}
	}

	// Verify checksum format (generic check since implementation dependent)
	if !strings.Contains(logOutput, "value_checksum=") {
		t.Errorf("Log output doesn't contain checksums")
	}

	// Verify server name is correctly included in logs
	if !strings.Contains(logOutput, "test-server") {
		t.Errorf("Log output doesn't contain server name")
	}
}

// TestLogConfigConsistency tests the consistency of checksum calculation
func TestLogConfigConsistency(t *testing.T) {
	// Capture and compare multiple log outputs for the same value
	testValue := "very-secret-password"
	testEnv := map[string]string{
		"TEST_SECRET": testValue,
	}

	// First log output
	var logBuffer1 strings.Builder
	testHandler1 := slog.NewTextHandler(&logBuffer1, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	testLogger1 := slog.New(testHandler1)
	LogConfig(testLogger1, "test-server", testEnv)
	logOutput1 := logBuffer1.String()

	// Second log output
	var logBuffer2 strings.Builder
	testHandler2 := slog.NewTextHandler(&logBuffer2, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	testLogger2 := slog.New(testHandler2)
	LogConfig(testLogger2, "test-server", testEnv)
	logOutput2 := logBuffer2.String()

	// Extract checksum from log outputs
	checksum1 := extractChecksum(logOutput1)
	checksum2 := extractChecksum(logOutput2)

	// Verify same checksum is generated for same value
	if checksum1 == "" || checksum2 == "" {
		t.Errorf("Failed to extract checksums from log output")
	} else if checksum1 != checksum2 {
		t.Errorf("Checksum calculation is not consistent: %s != %s", checksum1, checksum2)
	}

	// Verify different checksum is generated when value changes
	testEnvModified := map[string]string{
		"TEST_SECRET": testValue + "-modified",
	}

	var logBuffer3 strings.Builder
	testHandler3 := slog.NewTextHandler(&logBuffer3, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	testLogger3 := slog.New(testHandler3)
	LogConfig(testLogger3, "test-server", testEnvModified)
	logOutput3 := logBuffer3.String()

	// Extract checksum from modified value log
	checksum3 := extractChecksum(logOutput3)

	// Verify different values produce different checksums
	if checksum1 == "" || checksum3 == "" {
		t.Errorf("Failed to extract checksums from log output")
	} else if checksum1 == checksum3 {
		t.Errorf("Different values produced the same checksum: %s", checksum1)
	}
}

// extractChecksum extracts the checksum value from log output
func extractChecksum(logOutput string) string {
	// Find the value_checksum field in the log output
	checksumIndex := strings.Index(logOutput, "value_checksum=")
	if checksumIndex == -1 {
		return ""
	}

	// Extract the checksum value (assuming it's 8 chars as specified in the implementation)
	checksumStart := checksumIndex + len("value_checksum=")
	if checksumStart >= len(logOutput) {
		return ""
	}

	// Find the end of the checksum (either space, newline or end of string)
	checksumEnd := checksumStart + 8
	if checksumEnd > len(logOutput) {
		checksumEnd = len(logOutput)
	}

	return logOutput[checksumStart:checksumEnd]
}
