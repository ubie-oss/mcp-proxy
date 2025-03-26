package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	initMu     sync.RWMutex
	server     *http.Server
	logger     *slog.Logger
}

// NewServer creates a new server
func NewServer(mcpClients map[string]*MCPClient) *Server {
	return &Server{
		mcpClients: mcpClients,
		logger:     WithComponent("server"),
	}
}

func (s *Server) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
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
