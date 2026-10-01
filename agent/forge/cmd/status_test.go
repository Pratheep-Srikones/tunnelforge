package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"tunnelforge/agent/forge/config"

	"github.com/spf13/cobra"
)

func newTestStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "status",
		RunE: statusCmd.RunE,
	}
	cmd.Flags().StringP("config", "c", "", "Path to tunnel config YAML file")
	return cmd
}

func TestGetRoutingDetails_ConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tunnels.yaml")
	content := `tunnels:
  web:
    local: 3000
  api:
    local: localhost:8080
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cmd := newTestStatusCmd()
	_ = cmd.Flags().Set("config", configPath)

	tunnels, err := getRoutingDetails(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(tunnels))
	}
	if tunnels["web"] != "localhost:3000" {
		t.Errorf("expected localhost:3000 for web, got %s", tunnels["web"])
	}
	if tunnels["api"] != "localhost:8080" {
		t.Errorf("expected localhost:8080 for api, got %s", tunnels["api"])
	}
}

func TestGetRoutingDetails_NonExistentConfigFile(t *testing.T) {
	cmd := newTestStatusCmd()
	_ = cmd.Flags().Set("config", "/nonexistent/tunnels.yaml")

	_, err := getRoutingDetails(cmd)
	if err == nil {
		t.Fatal("expected error for nonexistent config file")
	}
}

func TestGetRoutingDetails_DefaultViperConfig(t *testing.T) {
	cmd := newTestStatusCmd()
	_ = config.Set("tunnels", map[string]any{
		"my-app": map[string]any{
			"local": "localhost:5173",
		},
	})
	defer func() {
		_ = config.Delete("tunnels")
	}()

	tunnels, err := getRoutingDetails(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(tunnels))
	}
	if tunnels["my-app"] != "localhost:5173" {
		t.Errorf("expected localhost:5173 for my-app, got %s", tunnels["my-app"])
	}
}
