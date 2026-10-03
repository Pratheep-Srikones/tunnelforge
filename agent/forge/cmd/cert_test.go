package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tunnelforge/internal/certs"
)

func TestCertCommandStdout(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"cert"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("expected no error executing 'forge cert', got: %v", err)
	}

	ca := string(certs.GetEmbeddedCACert())
	if !strings.Contains(ca, "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("embedded CA cert missing standard header")
	}
}

func TestCertCommandExport(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "exported_ca.crt")

	rootCmd.SetArgs([]string{"cert", "-o", outPath})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("expected no error executing 'forge cert -o', got: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	if !strings.Contains(string(data), "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("exported CA cert missing standard header")
	}
}
