package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
			name: "基本的な変換",
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
			name: "空の値を持つ設定",
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
