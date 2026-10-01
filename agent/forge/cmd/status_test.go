package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"tunnelforge/agent/forge/config"
)

func TestGetRoutingDetails_ActiveConfig(t *testing.T) {
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

	_ = config.Set("active_config", configPath)
	defer func() {
		_ = config.Delete("active_config")
	}()

	tunnels, err := getRoutingDetails()
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

func TestGetRoutingDetails_NonExistentActiveConfig(t *testing.T) {
	_ = config.Set("active_config", "/nonexistent/tunnels.yaml")
	defer func() {
		_ = config.Delete("active_config")
	}()

	_, err := getRoutingDetails()
	if err == nil {
		t.Fatal("expected error for nonexistent active_config file")
	}
}

func TestGetRoutingDetails_DefaultViperConfig(t *testing.T) {
	_ = config.Delete("active_config")
	_ = config.Set("tunnels", map[string]any{
		"my-app": map[string]any{
			"local": "localhost:5173",
		},
	})
	defer func() {
		_ = config.Delete("tunnels")
	}()

	tunnels, err := getRoutingDetails()
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
