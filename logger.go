package main

import (
	"log/slog"
	"os"
)

// InitLogger initializes the slog logger
func InitLogger(level slog.Level) {
	// Create JSON handler
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	// Set default logger
	slog.SetDefault(slog.New(handler))
}

// WithComponent creates a logger with component context
func WithComponent(component string) *slog.Logger {
	return slog.With("component", component)
}

// WithServer creates a logger with server name context
func WithServer(serverName string) *slog.Logger {
	return slog.With("server_name", serverName)
}

// WithComponentAndServer creates a logger with both component and server name context
func WithComponentAndServer(component, serverName string) *slog.Logger {
	return slog.With(
		"component", component,
		"server_name", serverName,
	)
}
