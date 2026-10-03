package certs

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClientTLSConfig_Embedded(t *testing.T) {
	cfg, err := LoadClientTLSConfig("localhost:7000", "")
	if err != nil {
		t.Fatalf("failed to load client TLS with embedded CA: %v", err)
	}

	if cfg.RootCAs == nil {
		t.Fatal("expected non-nil RootCAs pool")
	}
	if cfg.ServerName != "localhost" {
		t.Errorf("expected ServerName localhost, got %s", cfg.ServerName)
	}
}

func TestResolveServerCertPaths_PriorityOrder(t *testing.T) {
	tmpDir := t.TempDir()
	cert1 := filepath.Join(tmpDir, "c1.crt")
	key1 := filepath.Join(tmpDir, "k1.key")
	cert2 := filepath.Join(tmpDir, "c2.crt")
	key2 := filepath.Join(tmpDir, "k2.key")

	_ = os.WriteFile(cert1, []byte("cert1"), 0600)
	_ = os.WriteFile(key1, []byte("key1"), 0600)
	_ = os.WriteFile(cert2, []byte("cert2"), 0600)
	_ = os.WriteFile(key2, []byte("key2"), 0600)

	// Priority 1: Flag overrides everything
	os.Setenv("FORGE_TLS_CERT", cert2)
	os.Setenv("FORGE_TLS_KEY", key2)
	defer func() {
		os.Unsetenv("FORGE_TLS_CERT")
		os.Unsetenv("FORGE_TLS_KEY")
	}()

	c, k, err := ResolveServerCertPaths(cert1, key1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != cert1 || k != key1 {
		t.Errorf("expected flag paths (%s, %s), got (%s, %s)", cert1, key1, c, k)
	}

	// Priority 2: Env var used when flags empty
	c2, k2, err := ResolveServerCertPaths("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c2 != cert2 || k2 != key2 {
		t.Errorf("expected env paths (%s, %s), got (%s, %s)", cert2, key2, c2, k2)
	}
}

func TestTLS_EndToEndHandshake(t *testing.T) {
	serverTLS, err := LoadServerTLSConfig("", "")
	if err != nil {
		t.Skipf("skipping e2e handshake: server cert/key not in default path: %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("failed to start TLS listener: %v", err)
	}
	defer ln.Close()

	// Server accept
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("tls-hello"))
	}()

	clientTLS, err := LoadClientTLSConfig(ln.Addr().String(), "")
	if err != nil {
		t.Fatalf("failed to load client TLS: %v", err)
	}

	conn, err := tls.Dial("tcp", ln.Addr().String(), clientTLS)
	if err != nil {
		t.Fatalf("failed to connect over TLS: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 9)
	n, err := conn.Read(buf)
	if err != nil || string(buf[:n]) != "tls-hello" {
		t.Fatalf("unexpected read: %s (err: %v)", string(buf[:n]), err)
	}
}
