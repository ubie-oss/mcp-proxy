package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Commands string            `yaml:"commands"`
	Args     []string          `yaml:"args"`
	Env      map[string]string `yaml:"env"`
}

func loadConfig(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &cfg, nil
}

func main() {
	// Handle command line arguments
	configPath := flag.String("config", "config.yml", "path to config file")
	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	// Load config file
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize MCP client
	mcpClient, err := NewMCPClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer mcpClient.Close()

	// Start server
	server := NewServer(mcpClient)
	if err := server.Start(*port); err != nil {
		log.Fatal(err)
	}
}
