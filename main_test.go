package main

import (
	"testing"
)

// TestServerConfigDisabledHandling tests if disabled servers are properly skipped
func TestServerConfigDisabledHandling(t *testing.T) {
	tests := []struct {
		name          string
		serverConfigs map[string]ServerConfig
		expectedSkip  map[string]bool
	}{
		{
			name: "Single disabled server",
			serverConfigs: map[string]ServerConfig{
				"disabled-server": {
					Command: "echo",
					Args:    []string{"test"},
					Extensions: &Extensions{
						Disabled: true,
					},
				},
				"enabled-server": {
					Command: "echo",
					Args:    []string{"test"},
					Extensions: &Extensions{
						Disabled: false,
					},
				},
			},
			expectedSkip: map[string]bool{
				"disabled-server": true,
				"enabled-server":  false,
			},
		},
		{
			name: "Server without extensions (should be enabled)",
			serverConfigs: map[string]ServerConfig{
				"no-extensions": {
					Command: "echo",
					Args:    []string{"test"},
				},
			},
			expectedSkip: map[string]bool{
				"no-extensions": false,
			},
		},
		{
			name: "Server with nil extensions (should be enabled)",
			serverConfigs: map[string]ServerConfig{
				"nil-extensions": {
					Command:    "echo",
					Args:       []string{"test"},
					Extensions: nil,
				},
			},
			expectedSkip: map[string]bool{
				"nil-extensions": false,
			},
		},
		{
			name: "Multiple servers with mixed settings",
			serverConfigs: map[string]ServerConfig{
				"disabled-1": {
					Command:    "echo",
					Extensions: &Extensions{Disabled: true},
				},
				"disabled-2": {
					Command:    "echo",
					Extensions: &Extensions{Disabled: true, Sse: true},
				},
				"enabled-1": {
					Command:    "echo",
					Extensions: &Extensions{Disabled: false},
				},
				"enabled-2": {
					Command:    "echo",
					Extensions: &Extensions{Sse: true}, // Disabled defaults to false
				},
			},
			expectedSkip: map[string]bool{
				"disabled-1": true,
				"disabled-2": true,
				"enabled-1":  false,
				"enabled-2":  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for serverName, serverCfg := range tt.serverConfigs {
				shouldSkip := serverCfg.Extensions != nil && serverCfg.Extensions.Disabled
				expectedSkip := tt.expectedSkip[serverName]

				if shouldSkip != expectedSkip {
					t.Errorf("Server %s: expected skip=%v, got skip=%v",
						serverName, expectedSkip, shouldSkip)
				}
			}
		})
	}
}
