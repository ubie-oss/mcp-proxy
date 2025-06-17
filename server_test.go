package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// MockMCPClient is a mock implementation of MCPClientInterface for testing
type MockMCPClient struct {
	tools    []mcp.Tool
	callFunc func(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error)
	listFunc func(ctx context.Context) ([]mcp.Tool, error)
}

func (m *MockMCPClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return m.tools, nil
}

func (m *MockMCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	if m.callFunc != nil {
		return m.callFunc(ctx, name, args)
	}
	return &mcp.CallToolResult{}, nil
}

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

			// Test cache initialization
			if server.toolsCache == nil {
				t.Error("toolsCache should be initialized")
			}

			if server.cacheExpiry == nil {
				t.Error("cacheExpiry should be initialized")
			}

			// Cache should start empty
			if len(server.toolsCache) != 0 {
				t.Error("toolsCache should start empty")
			}

			if len(server.cacheExpiry) != 0 {
				t.Error("cacheExpiry should start empty")
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

// TestFlatModeAcceptsAnyPath tests that flat mode now accepts any path
func TestFlatModeAcceptsAnyPath(t *testing.T) {
	server := NewServer(make(map[string]*MCPClient), false)

	tests := []struct {
		path       string
		statusCode int
	}{
		{"/", http.StatusServiceUnavailable},                // Service not ready (no clients)
		{"/mcp", http.StatusServiceUnavailable},             // Service not ready (no clients)
		{"/api/mcp", http.StatusServiceUnavailable},         // Service not ready (no clients)
		{"/any/path", http.StatusServiceUnavailable},        // Service not ready (no clients)
		{"/custom/endpoint", http.StatusServiceUnavailable}, // Service not ready (no clients)
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.path, strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","params":{},"id":1}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleJSONRPC(w, req)

			if w.Code != tt.statusCode {
				t.Errorf("Path %s: expected status code %d, got %d", tt.path, tt.statusCode, w.Code)
			}
		})
	}
}

// TestSplitModePathValidation tests that split mode still validates server names
func TestSplitModePathValidation(t *testing.T) {
	server := NewServer(make(map[string]*MCPClient), true)

	tests := []struct {
		path       string
		statusCode int
	}{
		{"/", http.StatusBadRequest},            // Empty server name
		{"/nonexistent", http.StatusBadRequest}, // Server not found
		{"/server1", http.StatusBadRequest},     // Server not found (no clients configured)
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.path, strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","params":{},"id":1}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleJSONRPC(w, req)

			if w.Code != tt.statusCode {
				t.Errorf("Path %s: expected status code %d, got %d", tt.path, tt.statusCode, w.Code)
			}
		})
	}
}

// TestCacheInitialization tests that cache is properly initialized
func TestCacheInitialization(t *testing.T) {
	server := NewServer(make(map[string]*MCPClient), false)

	if server.toolsCache == nil {
		t.Fatal("toolsCache should be initialized")
	}

	if server.cacheExpiry == nil {
		t.Fatal("cacheExpiry should be initialized")
	}

	if len(server.toolsCache) != 0 {
		t.Error("toolsCache should start empty")
	}

	if len(server.cacheExpiry) != 0 {
		t.Error("cacheExpiry should start empty")
	}
}

// TestGetToolsWithCache tests the caching functionality
func TestGetToolsWithCache(t *testing.T) {
	server := NewServer(make(map[string]*MCPClient), false)
	ctx := context.Background()

	// Create mock tools
	mockTools := []mcp.Tool{
		{Name: "test_tool_1", Description: "Test tool 1"},
		{Name: "test_tool_2", Description: "Test tool 2"},
	}

	callCount := 0
	mockClient := &MockMCPClient{
		listFunc: func(ctx context.Context) ([]mcp.Tool, error) {
			callCount++
			return mockTools, nil
		},
	}

	// First call - should hit the API and cache the result
	tools1, err := server.getToolsWithCache(ctx, "server1", mockClient)
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	if len(tools1) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools1))
	}

	if callCount != 1 {
		t.Errorf("Expected 1 API call, got %d", callCount)
	}

	// Second call - should use cache
	tools2, err := server.getToolsWithCache(ctx, "server1", mockClient)
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}

	if len(tools2) != 2 {
		t.Errorf("Expected 2 tools from cache, got %d", len(tools2))
	}

	if callCount != 1 {
		t.Errorf("Expected still 1 API call due to cache hit, got %d", callCount)
	}

	// Verify cache contents
	server.cacheMu.RLock()
	cachedTools, exists := server.toolsCache["server1"]
	_, expiryExists := server.cacheExpiry["server1"]
	server.cacheMu.RUnlock()

	if !exists {
		t.Error("Tools should be cached")
	}

	if !expiryExists {
		t.Error("Cache expiry should be set")
	}

	if len(cachedTools) != 2 {
		t.Errorf("Expected 2 cached tools, got %d", len(cachedTools))
	}
}

// TestCacheExpiry tests that cache expires after TTL
func TestCacheExpiry(t *testing.T) {
	server := NewServer(make(map[string]*MCPClient), false)
	ctx := context.Background()

	mockTools := []mcp.Tool{
		{Name: "test_tool", Description: "Test tool"},
	}

	callCount := 0
	mockClient := &MockMCPClient{
		listFunc: func(ctx context.Context) ([]mcp.Tool, error) {
			callCount++
			return mockTools, nil
		},
	}

	// First call
	_, err := server.getToolsWithCache(ctx, "server1", mockClient)
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 API call, got %d", callCount)
	}

	// Manually expire the cache
	server.cacheMu.Lock()
	server.cacheExpiry["server1"] = time.Now().Add(-1 * time.Second) // Expired 1 second ago
	server.cacheMu.Unlock()

	// Second call - should hit API again due to expiry
	_, err = server.getToolsWithCache(ctx, "server1", mockClient)
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 API calls due to cache expiry, got %d", callCount)
	}
}

// TestCacheConcurrency tests cache safety under concurrent access
func TestCacheConcurrency(t *testing.T) {
	server := NewServer(make(map[string]*MCPClient), false)
	ctx := context.Background()

	mockTools := []mcp.Tool{
		{Name: "concurrent_tool", Description: "Concurrent test tool"},
	}

	mockClient := &MockMCPClient{
		listFunc: func(ctx context.Context) ([]mcp.Tool, error) {
			// Add small delay to simulate network call
			time.Sleep(10 * time.Millisecond)
			return mockTools, nil
		},
	}

	// Run multiple goroutines concurrently
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := server.getToolsWithCache(ctx, "server1", mockClient)
			results <- err
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent call %d failed: %v", i, err)
		}
	}

	// Verify cache is properly populated
	server.cacheMu.RLock()
	cachedTools, exists := server.toolsCache["server1"]
	server.cacheMu.RUnlock()

	if !exists {
		t.Error("Cache should contain tools after concurrent access")
	}

	if len(cachedTools) != 1 {
		t.Errorf("Expected 1 cached tool, got %d", len(cachedTools))
	}
}

// TestCacheOnlyInFlatMode tests that cache is only used for flat mode operations
func TestCacheOnlyInFlatMode(t *testing.T) {
	// This is more of a design verification - the cache methods are only called
	// from listAllTools and callToolAuto which are flat mode specific
	server := NewServer(make(map[string]*MCPClient), false) // flat mode

	if server.toolsCache == nil {
		t.Error("Flat mode server should have cache initialized")
	}

	// Split mode server also has cache initialized (for consistency)
	// but it won't be used since split mode doesn't call the cache methods
	splitServer := NewServer(make(map[string]*MCPClient), true)
	if splitServer.toolsCache == nil {
		t.Error("Split mode server should also have cache initialized (but unused)")
	}
}

// TestCacheErrorHandling tests cache behavior when API calls fail
func TestCacheErrorHandling(t *testing.T) {
	server := NewServer(make(map[string]*MCPClient), false)
	ctx := context.Background()

	callCount := 0
	mockClient := &MockMCPClient{
		listFunc: func(ctx context.Context) ([]mcp.Tool, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("API error")
			}
			// Second call succeeds
			return []mcp.Tool{{Name: "recovered_tool", Description: "Tool after error"}}, nil
		},
	}

	// First call fails
	_, err := server.getToolsWithCache(ctx, "server1", mockClient)
	if err == nil {
		t.Error("Expected error from first API call")
	}

	if callCount != 1 {
		t.Errorf("Expected 1 API call, got %d", callCount)
	}

	// Cache should not contain anything due to error
	server.cacheMu.RLock()
	_, exists := server.toolsCache["server1"]
	server.cacheMu.RUnlock()

	if exists {
		t.Error("Cache should not contain tools after API error")
	}

	// Second call succeeds and should cache
	tools, err := server.getToolsWithCache(ctx, "server1", mockClient)
	if err != nil {
		t.Fatalf("Second call should succeed: %v", err)
	}

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	if callCount != 2 {
		t.Errorf("Expected 2 API calls, got %d", callCount)
	}

	// Now cache should be populated
	server.cacheMu.RLock()
	cachedTools, exists := server.toolsCache["server1"]
	server.cacheMu.RUnlock()

	if !exists {
		t.Error("Cache should contain tools after successful API call")
	}

	if len(cachedTools) != 1 {
		t.Errorf("Expected 1 cached tool, got %d", len(cachedTools))
	}
}
