package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

const jsonrpcVersion = "2.0"

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

type Server struct {
	mcpClients map[string]*MCPClient
	mu         sync.RWMutex
}

func NewServer(mcpClients map[string]*MCPClient) *Server {
	return &Server{
		mcpClients: mcpClients,
	}
}

func (s *Server) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	// Extract server name from path
	serverName := strings.Trim(r.URL.Path, "/")
	if serverName == "" {
		http.Error(w, "Server name is required in path", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	mcpClient, exists := s.mcpClients[serverName]
	s.mu.RUnlock()
	if !exists {
		http.Error(w, fmt.Sprintf("Server %s not found", serverName), http.StatusNotFound)
		return
	}

	// 1. Method check
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 2. Parse JSONRPC request
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("Failed to parse JSON-RPC request: %v", err)
		writeJSONRPCError(w, -32700, "Parse error", nil, nil)
		return
	}

	// Validate required fields
	if err := validateJSONRPCRequest(&req); err != nil {
		log.Printf("Invalid JSON-RPC request: %v", err)
		writeJSONRPCError(w, -32600, err.Error(), nil, req.ID)
		return
	}

	// 3. Process methods
	log.Printf("[%s] Calling MCP method: %s", serverName, req.Method)
	var result interface{}
	s.mu.RLock()
	switch req.Method {
	case "initialize":
		result = &mcp.InitializeResult{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		}
	case "notifications/initialized":
		result = &mcp.InitializedNotification{}
	case "tools/list":
		tools, err := mcpClient.ListTools()
		if err == nil {
			result = &mcp.ListToolsResult{
				Tools: tools,
			}
		}
	case "tools/call":
		result, err = mcpClient.CallTool(req.Params["name"].(string), req.Params["arguments"].(map[string]interface{}))
	default:
		err = fmt.Errorf("method not found: %s", req.Method)
	}
	s.mu.RUnlock()

	if err != nil {
		log.Printf("MCP tool call error: %v", err)
		writeJSONRPCError(w, -32603, "Internal error", err.Error(), req.ID)
		return
	}

	// 4. Build response
	resp := JSONRPCResponse{
		JSONRPC: jsonrpcVersion,
		Result:  result,
		ID:      req.ID,
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to encode response: %v", err)
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
		log.Printf("Failed to encode error response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) Start(port string) error {
	http.HandleFunc("/_health", s.handleHealthCheck)
	http.HandleFunc("/", s.handleJSONRPC)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting MCP http proxy server on %s", addr)
	return http.ListenAndServe(addr, nil)
}
