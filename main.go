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

// ServerConfig represents the MCP server configuration structure
type ServerConfig struct {
	Command string            `yaml:"command" json:"command"`
	Args    []string          `yaml:"args" json:"args"`
	Env     map[string]string `yaml:"env" json:"env"`
}

// Config represents the application's global configuration structure
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

	// Create empty MCP clients map and start server immediately
	server := NewServer(make(map[string]*MCPClient))

	// Create context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(*port)
	}()

	// Initialize MCP clients asynchronously
	go func() {
		log.Println("Starting MCP client initialization...")
		tempMcpClients := make(map[string]*MCPClient)
		for name, serverCfg := range cfg.MCPServers {
			mcpCfg := ConvertToMCPClientConfig(serverCfg)
			client, err := NewMCPClient(mcpCfg)
			if err != nil {
				log.Printf("Error initializing MCP client '%s': %v", name, err)
				continue
			}
			tempMcpClients[name] = client
			log.Printf("Info: MCP Server '%s' initialized successfully", name)
		}

		// Update server's mcpClients map atomically with initMu lock
		server.initMu.Lock()
		server.mcpClients = tempMcpClients
		server.initMu.Unlock()
		log.Println("MCP client initialization completed; Now its ready")
	}()

	// Add cleanup for MCP clients on shutdown
	defer func() {
		server.initMu.RLock()
		mcpClients := server.mcpClients
		server.initMu.RUnlock()
		for _, client := range mcpClients {
			client.Close()
		}
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
