package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func newTestUpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "up [subdomain]",
		Args: cobra.MaximumNArgs(1),
	}
	cmd.Flags().StringP("server", "s", "localhost:7000", "Server address")
	cmd.Flags().StringP("to", "t", "", "Local destination port or address")
	cmd.Flags().StringP("local-addr", "l", "", "Local address (alias for --to)")
	cmd.Flags().StringP("subdomain", "d", "", "Subdomain for single tunnel mode")
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

func TestResolveTunnels_PositionalWithTo(t *testing.T) {
	cmd := newTestUpCommand()
	_ = cmd.Flags().Set("to", "3000")

	tunnels, err := resolveTunnels(cmd, []string{"test-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(tunnels))
	}
	if tunnels["test-app"] != "localhost:3000" {
		t.Fatalf("expected localhost:3000, got %s", tunnels["test-app"])
	}
}

func TestResolveTunnels_PositionalWithFullAddr(t *testing.T) {
	cmd := newTestUpCommand()
	_ = cmd.Flags().Set("to", "127.0.0.1:8080")

	tunnels, err := resolveTunnels(cmd, []string{"api-service"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tunnels["api-service"] != "127.0.0.1:8080" {
		t.Fatalf("expected 127.0.0.1:8080, got %s", tunnels["api-service"])
	}
}

func TestResolveTunnels_FlagsCompatibility(t *testing.T) {
	cmd := newTestUpCommand()
	_ = cmd.Flags().Set("subdomain", "my-app")
	_ = cmd.Flags().Set("local-addr", "localhost:5173")

	tunnels, err := resolveTunnels(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tunnels["my-app"] != "localhost:5173" {
		t.Fatalf("expected localhost:5173, got %s", tunnels["my-app"])
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

	tunnels, err := resolveTunnels(cmd, nil)
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
}

func TestResolveTunnels_ConfigFileSingleTunnel(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tunnels.yaml")
	content := `tunnels:
  api:
    local: localhost:8080
  web:
    local: localhost:3000
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cmd := newTestUpCommand()
	_ = cmd.Flags().Set("config", configPath)

	tunnels, err := resolveTunnels(cmd, []string{"web"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(tunnels))
	}
	if tunnels["web"] != "localhost:3000" {
		t.Errorf("expected localhost:3000 for web, got %s", tunnels["web"])
	}

	// Missing named tunnel
	_, err = resolveTunnels(cmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent tunnel in config")
	}
}

func TestResolveTunnels_Errors(t *testing.T) {
	// Subdomain with invalid characters
	cmd := newTestUpCommand()
	_ = cmd.Flags().Set("to", "3000")
	_, err := resolveTunnels(cmd, []string{"INVALID_SUB!"})
	if err == nil {
		t.Fatal("expected error for invalid subdomain")
	}

	// Subdomain without destination and no config
	cmd2 := newTestUpCommand()
	_, err = resolveTunnels(cmd2, []string{"test-app"})
	if err == nil {
		t.Fatal("expected error for subdomain without destination and no config")
	}

	// No arguments, no flags, no config
	cmd3 := newTestUpCommand()
	_, err = resolveTunnels(cmd3, nil)
	if err == nil {
		t.Fatal("expected error when no tunnels or config specified")
	}
}
