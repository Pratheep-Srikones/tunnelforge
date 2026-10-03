package certs

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"fmt"
	"net"
	"os"
)

// DefaultCACertPEM contains the embedded TunnelForge Root CA certificate.
// This is embedded directly into the agent binary, allowing zero-config TLS verification.
//
//go:embed ca.crt
var DefaultCACertPEM []byte

// GetEmbeddedCACert returns a copy of the embedded TunnelForge Root CA certificate PEM.
func GetEmbeddedCACert() []byte {
	out := make([]byte, len(DefaultCACertPEM))
	copy(out, DefaultCACertPEM)
	return out
}

// Candidate search locations for server certificate and key
var defaultCertSearchPaths = [][2]string{
	{"./server.crt", "./server.key"},
	{"./certs/server.crt", "./certs/server.key"},
	{"./internal/certs/server.crt", "./internal/certs/server.key"},
	{"../internal/certs/server.crt", "../internal/certs/server.key"},
	{"/etc/tunnelforge/certs/server.crt", "/etc/tunnelforge/certs/server.key"},
	{"/etc/tunnelforge/server.crt", "/etc/tunnelforge/server.key"},
}

// ResolveServerCertPaths finds the server certificate and key using priority order:
// 1. Explicit CLI flag parameters
// 2. Environment variables: FORGE_TLS_CERT and FORGE_TLS_KEY
// 3. Default search paths
func ResolveServerCertPaths(flagCert, flagKey string) (string, string, error) {
	// Priority 1: Explicit CLI parameters
	if flagCert != "" && flagKey != "" {
		if fileExists(flagCert) && fileExists(flagKey) {
			return flagCert, flagKey, nil
		}
		return "", "", fmt.Errorf("specified TLS cert (%s) or key (%s) does not exist", flagCert, flagKey)
	}

	// Priority 2: Environment variables
	envCert := os.Getenv("FORGE_TLS_CERT")
	envKey := os.Getenv("FORGE_TLS_KEY")
	if envCert != "" && envKey != "" {
		if fileExists(envCert) && fileExists(envKey) {
			return envCert, envKey, nil
		}
		return "", "", fmt.Errorf("FORGE_TLS_CERT (%s) or FORGE_TLS_KEY (%s) file does not exist", envCert, envKey)
	}

	// Priority 3: Default search paths
	for _, pair := range defaultCertSearchPaths {
		if fileExists(pair[0]) && fileExists(pair[1]) {
			return pair[0], pair[1], nil
		}
	}

	return "", "", fmt.Errorf("TLS certificate and key not found.\n" +
		"Please specify --tls-cert / --tls-key, set FORGE_TLS_CERT / FORGE_TLS_KEY, " +
		"or run 'go run internal/certs/generate.go'")
}

// LoadServerTLSConfig builds a *tls.Config for the server listener.
func LoadServerTLSConfig(flagCert, flagKey string) (*tls.Config, error) {
	certPath, keyPath, err := ResolveServerCertPaths(flagCert, flagKey)
	if err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading TLS key pair (%s, %s): %w", certPath, keyPath, err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// LoadClientTLSConfig creates a *tls.Config for the agent client.
// It uses customCAPath if provided, otherwise falls back to the embedded DefaultCACertPEM.
func LoadClientTLSConfig(targetAddr string, customCAPath string) (*tls.Config, error) {
	pool := x509.NewCertPool()

	var caData []byte
	if customCAPath != "" {
		var err error
		caData, err = os.ReadFile(customCAPath)
		if err != nil {
			return nil, fmt.Errorf("reading custom CA cert file %q: %w", customCAPath, err)
		}
	} else {
		caData = DefaultCACertPEM
	}

	if len(caData) > 0 {
		if !pool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("failed to parse CA certificate PEM")
		}
	} else {
		var err error
		pool, err = x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
	}

	host := targetAddr
	if h, _, err := net.SplitHostPort(targetAddr); err == nil {
		host = h
	}

	return &tls.Config{
		RootCAs:    pool,
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

//TODO:followup
