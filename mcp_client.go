package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type MCPClient struct {
	client *client.StdioMCPClient
}

func NewMCPClient(cfg *Config) (*MCPClient, error) {
	// Convert map[string]string to []string for environment variables
	env := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	c, err := client.NewStdioMCPClient(
		cfg.Commands,
		env,
		cfg.Args...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	// Initialize the client
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "mcp-http-proxy",
		Version: "1.0.0",
	}

	if _, err := c.Initialize(context.Background(), initRequest); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return &MCPClient{
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

func (c *MCPClient) ListTools() ([]mcp.Tool, error) {
	req := mcp.ListToolsRequest{}
	resp, err := c.client.ListTools(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	return resp.Tools, nil
}

func (c *MCPClient) CallTool(name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	resp, err := c.client.CallTool(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}
	return resp, nil
}

func (c *MCPClient) Close() error {
	return c.client.Close()
}
