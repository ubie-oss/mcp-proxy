package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Commands string            `yaml:"commands"`
	Args     []string          `yaml:"args"`
	Env      map[string]string `yaml:"env"`
}

type Config struct {
	Servers map[string]ServerConfig `yaml:"servers"`
}

// MCPClientConfig is used for NewMCPClient
type MCPClientConfig struct {
	Commands string            `yaml:"commands"`
	Args     []string          `yaml:"args"`
	Env      map[string]string `yaml:"env"`
}

func loadConfig(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// To enable embed environment variables
	expanded := os.ExpandEnv(string(buf))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &cfg, nil
}

func main() {
	// Load dotenv
	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}

	// Handle command line arguments
	configPath := flag.String("config", "config.yml", "path to config file")
	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	// Load config file
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize MCP clients
	mcpClients := make(map[string]*MCPClient)
	for name, serverCfg := range cfg.Servers {
		mcpCfg := &MCPClientConfig{
			Commands: serverCfg.Commands,
			Args:     serverCfg.Args,
			Env:      serverCfg.Env,
		}
		client, err := NewMCPClient(mcpCfg)
		if err != nil {
			log.Fatal(err)
		}
		mcpClients[name] = client
	}
	defer func() {
		for _, client := range mcpClients {
			client.Close()
		}
	}()

	// Start server
	server := NewServer(mcpClients)
	if err := server.Start(*port); err != nil {
		log.Fatal(err)
	}
}
