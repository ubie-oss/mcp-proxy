package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MCPClientConfig is the configuration used in NewMCPClient
type MCPClientConfig struct {
	Command string            `yaml:"command" json:"command"`
	Args    []string          `yaml:"args" json:"args"`
	Env     map[string]string `yaml:"env" json:"env"`
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
		Command: serverCfg.Command,
		Args:    serverCfg.Args,
		Env:     serverCfg.Env,
	}
}
