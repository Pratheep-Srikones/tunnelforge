package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"tunnelforge/agent/forge/config"

	"github.com/spf13/cobra"
)

func newTestUpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "up",
		Args: cobra.MaximumNArgs(1),
	}
	cmd.Flags().StringP("server", "s", "localhost:7000", "Server address")
	cmd.Flags().StringP("config", "c", "", "Path to tunnel config YAML file")
	cmd.Flags().IntP("max-retries", "r", 10, "Maximum number of reconnect retries")
	cmd.Flags().String("token", "", "Agent auth token")
	cmd.Flags().String("agent-id", "", "Agent ID")
	return cmd
}

func TestNormalizeLocalAddr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"3000", "localhost:3000"},
		{":3000", "localhost:3000"},
		{"8080", "localhost:8080"},
		{":8080", "localhost:8080"},
		{"localhost:3000", "localhost:3000"},
		{"127.0.0.1:5173", "127.0.0.1:5173"},
		{"0.0.0.0:8000", "0.0.0.0:8000"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		got := normalizeLocalAddr(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeLocalAddr(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveTunnels_ConfigFileAll(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tunnels.yaml")
	content := `tunnels:
  api:
    local: localhost:8080
  web:
    local: 3000
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cmd := newTestUpCommand()
	_ = cmd.Flags().Set("config", configPath)

	tunnels, err := resolveTunnels(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(tunnels))
	}
	if tunnels["api"] != "localhost:8080" {
		t.Errorf("expected localhost:8080 for api, got %s", tunnels["api"])
	}
	if tunnels["web"] != "localhost:3000" {
		t.Errorf("expected localhost:3000 for web, got %s", tunnels["web"])
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}
	if got := config.GetString("active_config"); got != absPath {
		t.Errorf("expected active_config %q, got %q", absPath, got)
	}
}

func TestResolveTunnels_Errors(t *testing.T) {
	cmd := newTestUpCommand()
	_ = cmd.Flags().Set("config", "/nonexistent/path/tunnels.yaml")
	_, err := resolveTunnels(cmd)
	if err == nil {
		t.Fatal("expected error for nonexistent config file")
	}
}
