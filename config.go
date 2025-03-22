package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MCPClientConfig はNewMCPClientで使用される設定です
type MCPClientConfig struct {
	Command string            `yaml:"command" json:"command"`
	Args    []string          `yaml:"args" json:"args"`
	Env     map[string]string `yaml:"env" json:"env"`
}

// LoadConfig は指定されたパスから設定ファイルを読み込みます
func LoadConfig(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// 環境変数の展開を有効にする
	expanded := os.ExpandEnv(string(buf))

	var cfg Config

	// ファイル拡張子から形式を判断
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
		// 拡張子がない場合や認識できない場合は、YAMLとして試行
		if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
			// YAMLとして失敗した場合はJSONとして試行
			if jsonErr := json.Unmarshal([]byte(expanded), &cfg); jsonErr != nil {
				return nil, fmt.Errorf("failed to parse config file (tried both YAML and JSON): %w", err)
			}
		}
	}

	return &cfg, nil
}

// ConvertToMCPClientConfig はServerConfigからMCPClientConfigへの変換を行います
func ConvertToMCPClientConfig(serverCfg ServerConfig) *MCPClientConfig {
	return &MCPClientConfig{
		Command: serverCfg.Command,
		Args:    serverCfg.Args,
		Env:     serverCfg.Env,
	}
}
