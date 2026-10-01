package cmd

import (
	"net"
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

func TestCheckHealth(t *testing.T) {
	// Test offline/empty
	if checkLocalHealth("") {
		t.Error("expected false for empty address")
	}
	if checkServerHealth("") {
		t.Error("expected false for empty server address")
	}
	if checkLocalHealth("127.0.0.1:59999") {
		t.Error("expected false for non-listening address")
	}

	// Start temporary listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	addr := l.Addr().String()

	if !checkLocalHealth(addr) {
		t.Errorf("expected checkLocalHealth(%s) to be true", addr)
	}
	if !checkServerHealth(addr) {
		t.Errorf("expected checkServerHealth(%s) to be true", addr)
	}
}
