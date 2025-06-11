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
	jsonrpcVersion = "2.0"
	// Default timeout for each request
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

// Server represents an HTTP server
type Server struct {
	mcpClients map[string]*MCPClient
	splitMode  bool
	initMu     sync.RWMutex
	server     *http.Server
	logger     *slog.Logger
}

// NewServer creates a new server
func NewServer(mcpClients map[string]*MCPClient, splitMode bool) *Server {
	return &Server{
		mcpClients: mcpClients,
		splitMode:  splitMode,
		logger:     WithComponent("server"),
	}
}

func (s *Server) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if s.splitMode {
		s.handleSplitMode(w, r)
	} else {
		s.handleFlatMode(w, r)
	}
}

func (s *Server) handleSplitMode(w http.ResponseWriter, r *http.Request) {
	// Check if the MCP servers are ready
	s.initMu.RLock()
	defer s.initMu.RUnlock()

	if len(s.mcpClients) == 0 {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	// Extract server name from the first path segment
	path := strings.Trim(r.URL.Path, "/")
	pathSegments := strings.SplitN(path, "/", 2)

	if len(pathSegments) == 0 || pathSegments[0] == "" {
		http.Error(w, "Server name is required in path", http.StatusBadRequest)
		return
	}

	serverName := pathSegments[0]
	mcpClient, exists := s.mcpClients[serverName]

	if !exists {
		http.Error(w, fmt.Sprintf("Server %s not found", serverName), http.StatusNotFound)
		return
	}

	// Create logger with server name
	logger := WithComponentAndServer("server", serverName)

	// Create a context with timeout for the request
	ctx, cancel := context.WithTimeout(r.Context(), defaultRequestTimeout)
	defer cancel()

	// Method check
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSONRPC request
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error("Failed to parse JSON-RPC request", "error", err)
		writeJSONRPCError(w, -32700, "Parse error", nil, nil)
		return
	}

	// Validate required fields
	if err := validateJSONRPCRequest(&req); err != nil {
		logger.Error("Invalid JSON-RPC request", "error", err)
		writeJSONRPCError(w, -32600, err.Error(), nil, req.ID)
		return
	}

	// Process methods
	logger.Info("Calling MCP method", "method", req.Method)
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
			tools, err := mcpClient.ListTools(ctx)
			if err == nil {
				result = &mcp.ListToolsResult{
					Tools: tools,
				}
			}
		}
	case "tools/call":
		select {
		case <-ctx.Done():
			err = fmt.Errorf("request timeout after %v", defaultRequestTimeout)
		default:
			toolName, _ := req.Params["name"].(string)
			logger.Info("Calling MCP tool", "tool", toolName)

			// Try type assertion for arguments, use empty map if it fails
			args, ok := req.Params["arguments"].(map[string]interface{})
			if !ok {
				logger.Warn("Arguments type assertion failed, using empty map")
				args = make(map[string]interface{})
			}
			result, err = mcpClient.CallTool(ctx, req.Params["name"].(string), args)
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

func (s *Server) handleFlatMode(w http.ResponseWriter, r *http.Request) {
	// Check if the MCP servers are ready
	s.initMu.RLock()
	defer s.initMu.RUnlock()

	if len(s.mcpClients) == 0 {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	// Check path: accept /mcp or /api/mcp
	path := strings.Trim(r.URL.Path, "/")
	if path != "mcp" && path != "api/mcp" {
		http.Error(w, "Path not found. Use /mcp or /api/mcp", http.StatusNotFound)
		return
	}

	logger := WithComponent("server")

	// Create a context with timeout for the request
	ctx, cancel := context.WithTimeout(r.Context(), defaultRequestTimeout)
	defer cancel()

	// Method check
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSONRPC request
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error("Failed to parse JSON-RPC request", "error", err)
		writeJSONRPCError(w, -32700, "Parse error", nil, nil)
		return
	}

	// Validate required fields
	if err := validateJSONRPCRequest(&req); err != nil {
		logger.Error("Invalid JSON-RPC request", "error", err)
		writeJSONRPCError(w, -32600, err.Error(), nil, req.ID)
		return
	}

	// Process methods
	logger.Info("Calling MCP method (flat mode)", "method", req.Method)
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
			result = s.listAllTools(ctx)
		}
	case "tools/call":
		select {
		case <-ctx.Done():
			err = fmt.Errorf("request timeout after %v", defaultRequestTimeout)
		default:
			result, err = s.callToolAuto(ctx, req.Params)
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
	toolMap := make(map[string]mcp.Tool)     // 重複排除用
	conflictLog := make(map[string][]string) // 重複記録用

	// サーバー名順（アルファベット順）で処理して deterministic order を保証
	serverNames := make([]string, 0, len(s.mcpClients))
	for name := range s.mcpClients {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, serverName := range serverNames {
		client := s.mcpClients[serverName]
		tools, err := client.ListTools(ctx)
		if err != nil {
			s.logger.Error("Failed to list tools from server", "server", serverName, "error", err)
			continue
		}

		for _, tool := range tools {
			if existing, exists := toolMap[tool.Name]; exists {
				// 重複検出
				if conflictLog[tool.Name] == nil {
					conflictLog[tool.Name] = []string{getServerNameFromToolMap(existing, s.mcpClients, ctx)} // 最初のサーバー名を記録
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

	// 配列に変換
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

	// サーバー名順（アルファベット順）で検索して deterministic order を保証
	serverNames := make([]string, 0, len(s.mcpClients))
	for name := range s.mcpClients {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, serverName := range serverNames {
		client := s.mcpClients[serverName]
		tools, err := client.ListTools(ctx)
		if err != nil {
			s.logger.Error("Failed to list tools for tool call routing", "server", serverName, "error", err)
			continue
		}

		for _, tool := range tools {
			if tool.Name == toolName {
				foundServers = append(foundServers, serverName)
				if len(foundServers) == 1 {
					// 最初に見つけたサーバーで実行
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

	// 重複があった場合は警告
	if len(foundServers) > 1 {
		s.logger.Warn("Tool name conflict detected during call",
			"tool", toolName,
			"servers", foundServers,
			"selected", foundServers[0])
	}

	return nil, fmt.Errorf("tool not found: %s", toolName)
}

// getServerNameFromToolMap はツールマップから最初にツールを提供したサーバー名を取得する補助関数
func getServerNameFromToolMap(tool mcp.Tool, clients map[string]*MCPClient, ctx context.Context) string {
	for serverName, client := range clients {
		tools, err := client.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			if t.Name == tool.Name {
				return serverName
			}
		}
	}
	return "unknown"
}
