package main

import (
	"testing"
)

func TestIsToolAllowed(t *testing.T) {
	tests := []struct {
		name      string
		client    *MCPClient
		toolName  string
		allowList []string
		denyList  []string
		expected  bool
	}{
		{
			name:     "No extensions - should allow",
			client:   &MCPClient{config: &MCPClientConfig{}},
			toolName: "tool1",
			expected: true,
		},
		{
			name: "Empty allow and deny lists - should allow",
			client: &MCPClient{
				config: &MCPClientConfig{
					Extensions: &Extensions{
						Tools: ToolsExtensions{
							Allow: []string{},
							Deny:  []string{},
						},
					},
				},
			},
			toolName: "tool1",
			expected: true,
		},
		{
			name: "Tool in allow list - should allow",
			client: &MCPClient{
				config: &MCPClientConfig{
					Extensions: &Extensions{
						Tools: ToolsExtensions{
							Allow: []string{"tool1", "tool2"},
							Deny:  []string{"tool3", "tool4"},
						},
					},
				},
			},
			toolName: "tool1",
			expected: true,
		},
		{
			name: "Tool not in allow list - should deny",
			client: &MCPClient{
				config: &MCPClientConfig{
					Extensions: &Extensions{
						Tools: ToolsExtensions{
							Allow: []string{"tool2", "tool3"},
							Deny:  []string{},
						},
					},
				},
			},
			toolName: "tool1",
			expected: false,
		},
		{
			name: "Tool in deny list, no allow list - should deny",
			client: &MCPClient{
				config: &MCPClientConfig{
					Extensions: &Extensions{
						Tools: ToolsExtensions{
							Allow: []string{},
							Deny:  []string{"tool1", "tool2"},
						},
					},
				},
			},
			toolName: "tool1",
			expected: false,
		},
		{
			name: "Tool not in deny list, no allow list - should allow",
			client: &MCPClient{
				config: &MCPClientConfig{
					Extensions: &Extensions{
						Tools: ToolsExtensions{
							Allow: []string{},
							Deny:  []string{"tool2", "tool3"},
						},
					},
				},
			},
			toolName: "tool1",
			expected: true,
		},
		{
			name: "Both allow and deny lists exist - allow list takes precedence",
			client: &MCPClient{
				config: &MCPClientConfig{
					Extensions: &Extensions{
						Tools: ToolsExtensions{
							Allow: []string{"tool1", "tool2"},
							Deny:  []string{"tool1", "tool3"},
						},
					},
				},
			},
			toolName: "tool1",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.client.isToolAllowed(tt.toolName)
			if result != tt.expected {
				t.Errorf("isToolAllowed(%s) = %v, want %v", tt.toolName, result, tt.expected)
			}
		})
	}
}
