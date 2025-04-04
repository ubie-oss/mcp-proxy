package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MCPClientConfig is the configuration used in NewMCPClient
type MCPClientConfig struct {
	Command    string            `yaml:"command" json:"command"`
	Args       []string          `yaml:"args" json:"args"`
	Env        map[string]string `yaml:"env" json:"env"`
	Extensions *Extensions       `yaml:"_extensions" json:"_extensions"`
}

// LoadConfig loads the configuration file from the specified path
func LoadConfig(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// Enable environment variable expansion
	expanded := os.ExpandEnv(string(buf))

	var cfg Config

	// Determine format from file extension
	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		if err := json.Unmarshal([]byte(expanded), &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	default:
		// If no extension or unrecognized, try as YAML
		if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
			// If YAML fails, try as JSON
			if jsonErr := json.Unmarshal([]byte(expanded), &cfg); jsonErr != nil {
				return nil, fmt.Errorf("failed to parse config file (tried both YAML and JSON): %w", err)
			}
		}
	}

	return &cfg, nil
}

// ConvertToMCPClientConfig converts ServerConfig to MCPClientConfig
func ConvertToMCPClientConfig(serverCfg ServerConfig) *MCPClientConfig {
	return &MCPClientConfig{
		Command:    serverCfg.Command,
		Args:       serverCfg.Args,
		Env:        serverCfg.Env,
		Extensions: serverCfg.Extensions,
	}
}

// LogSafeEnvChecksum calculates the SHA-256 checksum of environment variable values
// without exposing the actual values in logs
func LogConfig(logger *slog.Logger, serverName string, env map[string]string) {
	for key, value := range env {
		// Calculate SHA-256 checksum of the value
		hash := sha256.Sum256([]byte(value))
		checksum := hex.EncodeToString(hash[:])
		// Log the key and value checksum (not the actual value)
		logger.Debug("Environment variable checksum",
			"server", serverName,
			"key", key,
			"value_checksum", checksum[:8]) // Only log first 8 chars of checksum for brevity
	}
}
