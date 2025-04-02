package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
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

	return &MCPClient{
		config: config,
		client: c,
	}, nil
}

func (c *MCPClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "mcp-http-proxy",
		Version: "1.0.0",
	}

	resp, err := c.client.Initialize(ctx, initRequest)
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
	// Check tool restrictions
	if c.config.Extensions != nil && !c.isToolAllowed(name) {
		logger := WithComponent("mcp_client")
		logger.Warn("Tool access denied", "tool", name)
		return nil, fmt.Errorf("tool %s is not allowed", name)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	resp, err := c.client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}
	return resp, nil
}

// isToolAllowed checks if the tool is allowed to be called
func (c *MCPClient) isToolAllowed(toolName string) bool {
	ext := c.config.Extensions
	if ext == nil {
		return true
	}

	// If allow list is specified, only allow tools in the list
	if len(ext.Tools.Allow) > 0 {
		for _, allowed := range ext.Tools.Allow {
			if allowed == toolName {
				return true
			}
		}
		return false
	}

	// If no allow list but deny list exists, allow tools not in deny list
	if len(ext.Tools.Deny) > 0 {
		for _, denied := range ext.Tools.Deny {
			if denied == toolName {
				return false
			}
		}
	}

	// If neither list is specified, allow all tools
	return true
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
