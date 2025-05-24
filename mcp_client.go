package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient provides an interface to external MCP servers
type MCPClient struct {
	config       *MCPClientConfig
	client       *client.Client
	logger       *slog.Logger
	stderrCancel context.CancelFunc
	initOnce     sync.Once // Ensures monitoring starts only once during Initialize
	closeOnce    sync.Once // Ensures close operation is performed only once
}

// NewMCPClient creates a new MCP client
func NewMCPClient(config *MCPClientConfig) (*MCPClient, error) {
	// Convert map[string]string to []string for environment variables
	env := make([]string, 0, len(config.Env))
	for k, v := range config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	var c *client.Client
	var err error
	if config.Command != "" {
		c, err = client.NewStdioMCPClient(
			config.Command,
			env,
			config.Args...,
		)
	} else if config.Url != "" {
		if config.Extensions != nil && config.Extensions.Sse {
			c, err = client.NewSSEMCPClient(config.Url)
		} else {
			c, err = client.NewStreamableHttpClient(config.Url)
		}
	} else {
		return nil, fmt.Errorf("no MCP transport specified")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return &MCPClient{
		config: config,
		client: c,
		logger: WithComponent("mcp_client"),
	}, nil
}

func (c *MCPClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	// Start stderr monitoring once when Initialize is successful
	c.initOnce.Do(func() {
		stderrCtx, cancel := context.WithCancel(ctx)
		c.stderrCancel = cancel
		go c.captureStderr(stderrCtx)
		c.logger.Debug("stderr capture goroutine started")
	})

	if err := c.client.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start client: %w", err)
	}

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

func (c *MCPClient) captureStderr(ctx context.Context) {
	stderr, ok := client.GetStderr(c.client)
	if !ok {
		c.logger.Debug("stderr not available for this client type")
		return
	}
	scanner := bufio.NewScanner(stderr)
	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("stderr capture stopped due to context cancellation")
			return
		default:
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil && err != io.EOF && err != context.Canceled {
					c.logger.Error("error reading from stderr", "error", err)
				} else {
					c.logger.Debug("stderr stream closed or scanner finished")
				}
				return // Exit on error or EOF
			}
			line := scanner.Text()
			c.logger.Warn("subprocess stderr", "message", line)
		}
	}
}

func (c *MCPClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	req := mcp.ListToolsRequest{}
	resp, err := c.client.ListTools(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	// Skip filtering if no extensions are configured
	if c.config.Extensions == nil {
		return resp.Tools, nil
	}

	// Filter tools based on allow/deny lists
	var filteredTools []mcp.Tool
	for _, tool := range resp.Tools {
		if c.isToolAllowed(tool.Name) {
			filteredTools = append(filteredTools, tool)
		}
	}

	return filteredTools, nil
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
	var err error
	c.closeOnce.Do(func() {
		if c.stderrCancel != nil {
			c.logger.Debug("cancelling stderr capture")
			c.stderrCancel()
			c.stderrCancel = nil // Prevent being called again
		}

		c.logger.Debug("closing underlying stdio client")
		err = c.client.Close()
		if err != nil {
			c.logger.Error("error closing stdio client", "error", err)
		}
	})
	return err
}

// Call executes a request via the MCP client
func (c *MCPClient) Call(functionName string, params map[string]interface{}) (interface{}, error) {
	// In the actual implementation, this would handle the request to the external process
	return nil, fmt.Errorf("not implemented")
}
