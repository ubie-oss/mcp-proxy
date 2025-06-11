package main

import (
	"testing"
)

// TestNewServer tests server creation with different modes
func TestNewServer(t *testing.T) {
	tests := []struct {
		name      string
		splitMode bool
	}{
		{"flat mode", false},
		{"split mode", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpClients := make(map[string]*MCPClient)
			server := NewServer(mcpClients, tt.splitMode)

			if server.splitMode != tt.splitMode {
				t.Errorf("Expected splitMode=%v, got %v", tt.splitMode, server.splitMode)
			}

			if server.mcpClients == nil {
				t.Error("mcpClients should not be nil")
			}

			if server.logger == nil {
				t.Error("logger should not be nil")
			}
		})
	}
}

// TestSplitModeFlag tests that the split mode flag is properly set
func TestSplitModeFlag(t *testing.T) {
	// Test flat mode (default)
	flatServer := NewServer(make(map[string]*MCPClient), false)
	if flatServer.splitMode != false {
		t.Error("Flat mode server should have splitMode=false")
	}

	// Test split mode
	splitServer := NewServer(make(map[string]*MCPClient), true)
	if splitServer.splitMode != true {
		t.Error("Split mode server should have splitMode=true")
	}
}

// TestServerInitialization tests basic server initialization
func TestServerInitialization(t *testing.T) {
	mcpClients := make(map[string]*MCPClient)
	server := NewServer(mcpClients, false)

	// Check that server is properly initialized
	if server == nil {
		t.Fatal("Server should not be nil")
	}

	if server.mcpClients == nil {
		t.Error("mcpClients should be initialized")
	}

	if server.logger == nil {
		t.Error("logger should be initialized")
	}

	// Check that we can change split mode
	splitServer := NewServer(mcpClients, true)
	if splitServer.splitMode == server.splitMode {
		t.Error("Different split mode settings should create different server configurations")
	}
}
