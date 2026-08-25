package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTunnelConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tunnels.yaml")

	content := `tunnels:
  test-app:
    local: localhost:3000
  my-app:
    local: localhost:5173
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadTunnelConfig(path)
	if err != nil {
		t.Fatalf("LoadTunnelConfig() error = %v", err)
	}

	if len(cfg.Tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(cfg.Tunnels))
	}

	if cfg.Tunnels["test-app"].Local != "localhost:3000" {
		t.Fatalf("expected test-app local to be localhost:3000, got %s", cfg.Tunnels["test-app"].Local)
	}

	if cfg.Tunnels["my-app"].Local != "localhost:5173" {
		t.Fatalf("expected my-app local to be localhost:5173, got %s", cfg.Tunnels["my-app"].Local)
	}

	// Test TunnelMap
	m := cfg.TunnelMap()
	if m["test-app"] != "localhost:3000" {
		t.Fatalf("TunnelMap test-app mismatch")
	}
	if m["my-app"] != "localhost:5173" {
		t.Fatalf("TunnelMap my-app mismatch")
	}

	// Test Subdomains
	subs := cfg.Subdomains()
	if len(subs) != 2 {
		t.Fatalf("expected 2 subdomains, got %d", len(subs))
	}
}

func TestLoadTunnelConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "empty tunnels",
			content: "tunnels:\n",
			wantErr: true,
		},
		{
			name: "missing local address",
			content: `tunnels:
  test-app:
    local: ""
`,
			wantErr: true,
		},
		{
			name: "invalid subdomain",
			content: `tunnels:
  INVALID_APP!:
    local: localhost:3000
`,
			wantErr: true,
		},
		{
			name: "valid single tunnel",
			content: `tunnels:
  api:
    local: localhost:8080
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "tunnels.yaml")

			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			_, err := LoadTunnelConfig(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadTunnelConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadTunnelConfigFileNotFound(t *testing.T) {
	_, err := LoadTunnelConfig("/nonexistent/path/tunnels.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
