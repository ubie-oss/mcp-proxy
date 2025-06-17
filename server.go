package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	jsonrpcVersion        = "2.0"
	defaultRequestTimeout = 60 * time.Second
)

type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
	ID      interface{}            `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	} `json:"error,omitempty"`
	ID interface{} `json:"id"`
}

// MCPClientInterface defines the interface for MCP clients
type MCPClientInterface interface {
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error)
}

// Server represents an HTTP server
type Server struct {
	mcpClients map[string]*MCPClient
	splitMode  bool
	initMu     sync.RWMutex
	server     *http.Server
	logger     *slog.Logger

	// Cache for tools (flat mode only)
	toolsCache  map[string][]mcp.Tool
	cacheExpiry map[string]time.Time
	cacheMu     sync.RWMutex
}

// NewServer creates a new server with the specified MCP clients and mode
func NewServer(mcpClients map[string]*MCPClient, splitMode bool) *Server {
	return &Server{
		mcpClients:  mcpClients,
		splitMode:   splitMode,
		logger:      WithComponent("server"),
		toolsCache:  make(map[string][]mcp.Tool),
		cacheExpiry: make(map[string]time.Time),
	}
}

// ModeHandler defines the interface for mode-specific handling
type ModeHandler interface {
	validateRequest(r *http.Request) (*slog.Logger, error)
	handleToolsList(ctx context.Context) (interface{}, error)
	handleToolsCall(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// SplitModeHandler handles requests in split mode
type SplitModeHandler struct {
	server    *Server
	mcpClient *MCPClient
	logger    *slog.Logger
}

// FlatModeHandler handles requests in flat mode
type FlatModeHandler struct {
	server *Server
	logger *slog.Logger
}

// handleJSONRPC routes requests to the appropriate handler based on server mode
func (s *Server) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	var handler ModeHandler
	var err error

	if s.splitMode {
		handler, err = s.createSplitModeHandler(r)
	} else {
		handler, err = s.createFlatModeHandler(r)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.processRequest(w, r, handler)
}

// createSplitModeHandler creates a handler for split mode requests
func (s *Server) createSplitModeHandler(r *http.Request) (ModeHandler, error) {
	path := strings.Trim(r.URL.Path, "/")
	pathSegments := strings.SplitN(path, "/", 2)

	if len(pathSegments) == 0 || pathSegments[0] == "" {
		return nil, fmt.Errorf("Server name is required in path")
	}

	serverName := pathSegments[0]
	mcpClient, exists := s.mcpClients[serverName]

	if !exists {
		return nil, fmt.Errorf("Server %s not found", serverName)
	}

	return &SplitModeHandler{
		server:    s,
		mcpClient: mcpClient,
		logger:    WithComponentAndServer("server", serverName),
	}, nil
}

// createFlatModeHandler creates a handler for flat mode requests
func (s *Server) createFlatModeHandler(r *http.Request) (ModeHandler, error) {
	return &FlatModeHandler{
		server: s,
		logger: WithComponent("server"),
	}, nil
}

// processRequest handles the common request processing logic
func (s *Server) processRequest(w http.ResponseWriter, r *http.Request, handler ModeHandler) {
	s.initMu.RLock()
	defer s.initMu.RUnlock()

	if len(s.mcpClients) == 0 {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	logger, err := handler.validateRequest(r)
	if err != nil {
		logger.Error("Request validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), defaultRequestTimeout)
	defer cancel()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error("Failed to parse JSON-RPC request", "error", err)
		writeJSONRPCError(w, -32700, "Parse error", nil, nil)
		return
	}

	if err := validateJSONRPCRequest(&req); err != nil {
		logger.Error("Invalid JSON-RPC request", "error", err)
		writeJSONRPCError(w, -32600, err.Error(), nil, req.ID)
		return
	}

	logger.Info("Processing MCP method", "method", req.Method)
	var result interface{}

	switch req.Method {
	case "initialize":
		result = &mcp.InitializeResult{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		}
	case "notifications/initialized":
		result = &mcp.InitializedNotification{}
	case "tools/list":
		select {
		case <-ctx.Done():
			err = fmt.Errorf("request timeout after %v", defaultRequestTimeout)
		default:
			result, err = handler.handleToolsList(ctx)
		}
	case "tools/call":
		select {
		case <-ctx.Done():
			err = fmt.Errorf("request timeout after %v", defaultRequestTimeout)
		default:
			result, err = handler.handleToolsCall(ctx, req.Params)
		}
	default:
		err = fmt.Errorf("method not found: %s", req.Method)
	}

	if err != nil {
		logger.Error("MCP error", "error", err)
		writeJSONRPCError(w, -32603, "Internal error", err.Error(), req.ID)
		return
	}

	// Build response
	resp := JSONRPCResponse{
		JSONRPC: jsonrpcVersion,
		Result:  result,
		ID:      req.ID,
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// SplitModeHandler implementations
func (h *SplitModeHandler) validateRequest(r *http.Request) (*slog.Logger, error) {
	return h.logger, nil
}

func (h *SplitModeHandler) handleToolsList(ctx context.Context) (interface{}, error) {
	tools, err := h.mcpClient.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	return &mcp.ListToolsResult{Tools: tools}, nil
}

func (h *SplitModeHandler) handleToolsCall(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	toolName, _ := params["name"].(string)
	h.logger.Info("Calling MCP tool", "tool", toolName)

	// Try type assertion for arguments, use empty map if it fails
	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		h.logger.Warn("Arguments type assertion failed, using empty map")
		args = make(map[string]interface{})
	}
	return h.mcpClient.CallTool(ctx, params["name"].(string), args)
}

// FlatModeHandler implementations
func (h *FlatModeHandler) validateRequest(r *http.Request) (*slog.Logger, error) {
	return h.logger, nil
}

func (h *FlatModeHandler) handleToolsList(ctx context.Context) (interface{}, error) {
	return h.server.listAllTools(ctx), nil
}

func (h *FlatModeHandler) handleToolsCall(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	return h.server.callToolAuto(ctx, params)
}

func validateJSONRPCRequest(req *JSONRPCRequest) error {
	if req.JSONRPC != jsonrpcVersion {
		return fmt.Errorf("invalid jsonrpc version: %s", req.JSONRPC)
	}
	if req.Method == "" {
		return fmt.Errorf("method is required")
	}
	if req.Params == nil {
		req.Params = make(map[string]interface{})
	}
	return nil
}

func writeJSONRPCError(w http.ResponseWriter, code int, message string, data interface{}, id interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: jsonrpcVersion,
		Error: &struct {
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    interface{} `json:"data,omitempty"`
		}{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger := WithComponent("server")
		logger.Error("Failed to encode error response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("Liveness OK"))
	if err != nil {
		s.logger.Error("Failed to write liveness response", "error", err)
	}
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	s.initMu.RLock()
	defer s.initMu.RUnlock()

	ready := len(s.mcpClients) > 0

	if ready {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("Readiness OK"))
		if err != nil {
			s.logger.Error("Failed to write readiness response", "error", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte("Readiness NOT OK"))
		if err != nil {
			s.logger.Error("Failed to write readiness response", "error", err)
		}
	}
}

// Start starts the server
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/liveness", s.handleLiveness)
	mux.HandleFunc("/health/readiness", s.handleReadiness)
	mux.HandleFunc("/", s.handleJSONRPC)

	addr := ":" + port
	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	s.logger.Info("Starting MCP http proxy server", "address", addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// listAllTools aggregates tools from all connected MCP servers
func (s *Server) listAllTools(ctx context.Context) *mcp.ListToolsResult {
	toolMap := make(map[string]mcp.Tool)
	conflictLog := make(map[string][]string)

	serverNames := make([]string, 0, len(s.mcpClients))
	for name := range s.mcpClients {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, serverName := range serverNames {
		client := s.mcpClients[serverName]
		tools, err := s.getToolsWithCache(ctx, serverName, client)
		if err != nil {
			s.logger.Error("Failed to list tools from server", "server", serverName, "error", err)
			continue
		}

		for _, tool := range tools {
			if _, exists := toolMap[tool.Name]; exists {
				if conflictLog[tool.Name] == nil {
					firstServer := "unknown"
					for prevServerName := range s.mcpClients {
						if prevServerName == serverName {
							break
						}
						if prevTools, err := s.getToolsWithCache(ctx, prevServerName, s.mcpClients[prevServerName]); err == nil {
							for _, prevTool := range prevTools {
								if prevTool.Name == tool.Name {
									firstServer = prevServerName
									break
								}
							}
							if firstServer != "unknown" {
								break
							}
						}
					}
					conflictLog[tool.Name] = []string{firstServer}
				}
				conflictLog[tool.Name] = append(conflictLog[tool.Name], serverName)
				s.logger.Warn("Tool name conflict in tools/list",
					"tool", tool.Name,
					"keeping_from", conflictLog[tool.Name][0],
					"conflicting_server", serverName)
			} else {
				toolMap[tool.Name] = tool
			}
		}
	}

	var allTools []mcp.Tool
	for _, tool := range toolMap {
		allTools = append(allTools, tool)
	}

	return &mcp.ListToolsResult{Tools: allTools}
}

// callToolAuto automatically routes tool calls to the appropriate MCP server
func (s *Server) callToolAuto(ctx context.Context, params map[string]interface{}) (*mcp.CallToolResult, error) {
	toolName, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("tool name is required")
	}

	var foundServers []string

	serverNames := make([]string, 0, len(s.mcpClients))
	for name := range s.mcpClients {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, serverName := range serverNames {
		client := s.mcpClients[serverName]
		tools, err := s.getToolsWithCache(ctx, serverName, client)
		if err != nil {
			s.logger.Error("Failed to list tools for tool call routing", "server", serverName, "error", err)
			continue
		}

		for _, tool := range tools {
			if tool.Name == toolName {
				foundServers = append(foundServers, serverName)
				if len(foundServers) == 1 {
					args, _ := params["arguments"].(map[string]interface{})
					if args == nil {
						args = make(map[string]interface{})
					}
					s.logger.Info("Calling tool", "tool", toolName, "server", serverName)
					return client.CallTool(ctx, toolName, args)
				}
			}
		}
	}

	if len(foundServers) > 1 {
		s.logger.Warn("Tool name conflict detected during call",
			"tool", toolName,
			"servers", foundServers,
			"selected", foundServers[0])
	}

	return nil, fmt.Errorf("tool not found: %s", toolName)
}

// getToolsWithCache returns tools with 60s TTL caching for flat mode
func (s *Server) getToolsWithCache(ctx context.Context, serverName string, client MCPClientInterface) ([]mcp.Tool, error) {
	s.cacheMu.RLock()
	if tools, exists := s.toolsCache[serverName]; exists {
		if expiry, hasExpiry := s.cacheExpiry[serverName]; hasExpiry && time.Now().Before(expiry) {
			s.cacheMu.RUnlock()
			return tools, nil
		}
	}
	s.cacheMu.RUnlock()

	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	s.toolsCache[serverName] = tools
	s.cacheExpiry[serverName] = time.Now().Add(60 * time.Second)
	s.cacheMu.Unlock()

	return tools, nil
}
