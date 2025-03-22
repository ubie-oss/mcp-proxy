package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// default initialize timeout
	DEFAULT_TIMEOUT = 30 * time.Second
)

// MCPClient provides an interface to external MCP servers
type MCPClient struct {
	config *MCPClientConfig
	client *client.StdioMCPClient
}

// NewMCPClient creates a new MCP client
func NewMCPClient(config *MCPClientConfig) (*MCPClient, error) {
	// Convert map[string]string to []string for environment variables
	env := make([]string, 0, len(config.Env))
	for k, v := range config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	c, err := client.NewStdioMCPClient(
		config.Command,
		env,
		config.Args...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	// Initialize the client with timeout
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "mcp-http-proxy",
		Version: "1.0.0",
	}

	ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_TIMEOUT)
	defer cancel()

	if _, err := c.Initialize(ctx, initRequest); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return &MCPClient{
		config: config,
		client: c,
	}, nil
}

func (c *MCPClient) Initialize(ctx context.Context, req mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	resp, err := c.client.Initialize(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}
	return resp, nil
}

func (c *MCPClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	req := mcp.ListToolsRequest{}
	resp, err := c.client.ListTools(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	return resp.Tools, nil
}

func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	resp, err := c.client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}
	return resp, nil
}

// Close closes the connection to the MCP client
func (c *MCPClient) Close() error {
	return c.client.Close()
}

// Call executes a request via the MCP client
func (c *MCPClient) Call(functionName string, params map[string]interface{}) (interface{}, error) {
	// In the actual implementation, this would handle the request to the external process
	return nil, fmt.Errorf("not implemented")
}
