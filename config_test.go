package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "mcp-proxy-test")
	if err != nil {
		t.Fatalf("テスト用ディレクトリの作成に失敗: %v", err)
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
			name: "有効なJSONファイル",
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
			name: "有効なYAMLファイル",
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
			name: "環境変数展開のテスト",
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
			name:      "存在しないファイル",
			content:   "",
			extension: ".json",
			wantErr:   true,
		},
		{
			name:      "不正なJSON",
			content:   `{"invalid": json}`,
			extension: ".json",
			wantErr:   true,
		},
		{
			name:      "不正なYAML",
			content:   `invalid: - yaml`,
			extension: ".yaml",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 環境変数の設定
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			var path string
			if tt.content != "" {
				// 一時ファイルの作成
				filename := "config" + tt.extension
				path = filepath.Join(tempDir, filename)
				if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
					t.Fatalf("テストファイルの作成に失敗: %v", err)
				}
			} else {
				// 存在しないファイルのテスト
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

// JSONとYAMLのマーシャル/アンマーシャルのラウンドトリップテスト
func TestMCPClientConfigSerialization(t *testing.T) {
	original := MCPClientConfig{
		Command: "test-command",
		Args:    []string{"arg1", "arg2"},
		Env:     map[string]string{"ENV1": "val1", "ENV2": "val2"},
	}

	// JSONシリアライズ/デシリアライズのテスト
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("JSONマーシャルに失敗: %v", err)
	}

	var jsonDeserialized MCPClientConfig
	if err := json.Unmarshal(jsonData, &jsonDeserialized); err != nil {
		t.Fatalf("JSONアンマーシャルに失敗: %v", err)
	}

	if !reflect.DeepEqual(original, jsonDeserialized) {
		t.Errorf("JSONラウンドトリップ後のデータが一致しません。元: %v, 結果: %v", original, jsonDeserialized)
	}
}
