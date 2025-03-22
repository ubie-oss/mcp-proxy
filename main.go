package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

// ServerConfig はMCPサーバー設定を表す構造体です
type ServerConfig struct {
	Command string            `yaml:"command" json:"command"`
	Args    []string          `yaml:"args" json:"args"`
	Env     map[string]string `yaml:"env" json:"env"`
}

// Config はアプリケーション全体の設定を表す構造体です
type Config struct {
	MCPServers map[string]ServerConfig `yaml:"mcpServers" json:"mcpServers"`
}

func main() {
	// Load dotenv if it exists
	if err := godotenv.Load(); err != nil {
		log.Printf("Notice: .env file not found, continuing without it")
	}

	// Handle command line arguments
	configPath := flag.String("config", "", "path to config file (required)")
	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	// Check if config file path is provided
	if *configPath == "" {
		log.Fatal("Error: config file path is required. Use -config flag to specify it.")
	}

	// Load config file
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize MCP clients
	mcpClients := make(map[string]*MCPClient)
	for name, serverCfg := range cfg.MCPServers {
		mcpCfg := ConvertToMCPClientConfig(serverCfg)
		client, err := NewMCPClient(mcpCfg)
		if err != nil {
			log.Fatal(err)
		}
		mcpClients[name] = client
		log.Printf("Info: MCP Server '%s' initialized successfully", name)
	}
	defer func() {
		for _, client := range mcpClients {
			client.Close()
		}
	}()

	// Create context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server
	server := NewServer(mcpClients)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(*port)
	}()

	// Wait for interrupt signal or server error
	select {
	case <-ctx.Done():
		log.Println("Shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}
}
