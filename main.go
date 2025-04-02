package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

// ServerConfig represents the MCP server configuration structure
type ServerConfig struct {
	Command    string            `yaml:"command" json:"command"`
	Args       []string          `yaml:"args" json:"args"`
	Env        map[string]string `yaml:"env" json:"env"`
	Extensions *Extensions       `yaml:"_extensions" json:"_extensions"`
}

// ToolsExtensions contains tool allow/deny lists
type ToolsExtensions struct {
	Allow []string `yaml:"allow" json:"allow"`
	Deny  []string `yaml:"deny" json:"deny"`
}

// Extensions contains various extension configurations
type Extensions struct {
	Tools ToolsExtensions `yaml:"tools" json:"tools"`
}

// Config represents the application's global configuration structure
type Config struct {
	MCPServers map[string]ServerConfig `yaml:"mcpServers" json:"mcpServers"`
}

func main() {
	// Handle command line arguments
	configPath := flag.String("config", "", "path to config file (required)")
	port := flag.String("port", "8080", "port to listen on")
	debug := flag.Bool("debug", false, "enable debug mode")
	initTimeoutSec := flag.Int("init-timeout", 60, "timeout in seconds for each MCP client initialization")
	flag.Parse()

	// Initialize logger with level
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	InitLogger(logLevel)
	logger := WithComponent("main")

	// Load dotenv if it exists
	if err := godotenv.Load(); err != nil {
		logger.Info("Notice: .env file not found, continuing without it")
	}

	// Check if config file path is provided
	if *configPath == "" {
		logger.Error("Config file path is required", "error", "Use -config flag to specify it")
		os.Exit(1)
	}

	// Load config file
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
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
		logger.Info("Starting MCP client initialization")
		tempMcpClients := make(map[string]*MCPClient)
		for name, serverCfg := range cfg.MCPServers {
			mcpCfg := ConvertToMCPClientConfig(serverCfg)
			client, err := NewMCPClient(mcpCfg)
			if err != nil {
				logger.Error("Failed to create MCP client",
					"server_name", name,
					"error", err)
				continue
			}

			err = withTimeout(ctx, time.Duration(*initTimeoutSec)*time.Second, func(ctx context.Context) error {
				_, err := client.Initialize(ctx)
				return err
			})
			if err != nil {
				logger.Error("Failed to initialize MCP client", "server_name", name, "error", err)
				continue
			}

			tempMcpClients[name] = client
			logger.Info("MCP Server initialized successfully", "server_name", name)

			// Log environment variables checksums if in debug mode
			if *debug {
				LogConfig(logger, name, serverCfg.Env)
			}
		}

		// Update server's mcpClients map atomically with initMu lock
		server.initMu.Lock()
		server.mcpClients = tempMcpClients
		server.initMu.Unlock()
		logger.Info("MCP client initialization completed; Now its ready")
	}()

	// Add cleanup for MCP clients on shutdown
	defer func() {
		server.initMu.RLock()
		defer server.initMu.RUnlock()
		mcpClients := server.mcpClients
		for _, client := range mcpClients {
			client.Close()
		}
	}()

	// Wait for interrupt signal or server error
	select {
	case <-ctx.Done():
		logger.Info("Shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("Server shutdown error", "error", err)
		}
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("Fatal server error", "error", err)
			os.Exit(1)
		}
	}
}

func withTimeout(parentCtx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	return fn(ctx)
}
