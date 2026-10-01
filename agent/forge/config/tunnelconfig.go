package config

import (
	"fmt"
	"os"
	"tunnelforge/internal/proto"

	"go.yaml.in/yaml/v3"
)

// TunnelEntry defines a single tunnel mapping.
type TunnelEntry struct {
	Local        string `yaml:"local"`
	Capture      bool   `yaml:"capture"`
	CaptureLimit int    `yaml:"capture_limit"`
}

// TunnelConfig represents a user-authored YAML tunnel definition file.
//
// Example:
//
//	tunnels:
//	  test-app:
//	    local: localhost:3000
//	    capture: true
//	    capture_limit: 100
//	  my-app:
//	    local: localhost:5173
//	    capture: true
//	    capture_limit: 200
type TunnelConfig struct {
	Tunnels map[string]TunnelEntry `yaml:"tunnels"`
}

// LoadTunnelConfig reads and validates a tunnel config YAML file.
func LoadTunnelConfig(path string) (*TunnelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading tunnel config %q: %w", path, err)
	}

	var cfg TunnelConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing tunnel config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid tunnel config: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the tunnel config is well-formed.
func (c *TunnelConfig) Validate() error {
	if len(c.Tunnels) == 0 {
		return fmt.Errorf("no tunnels defined")
	}

	for subdomain, entry := range c.Tunnels {
		if subdomain == "" {
			return fmt.Errorf("tunnel subdomain must not be empty")
		}

		if err := proto.ValidateSubdomain(subdomain); err != nil {
			return fmt.Errorf("tunnel %q: %w", subdomain, err)
		}

		if entry.Local == "" {
			return fmt.Errorf("tunnel %q: local address is required", subdomain)
		}
	}

	return nil
}

// Subdomains returns the list of subdomain names defined in the config.
func (c *TunnelConfig) Subdomains() []string {
	subs := make([]string, 0, len(c.Tunnels))
	for sub := range c.Tunnels {
		subs = append(subs, sub)
	}
	return subs
}

// TunnelMap returns a mapping of subdomain → local address.
func (c *TunnelConfig) TunnelMap() map[string]string {
	m := make(map[string]string, len(c.Tunnels))
	for sub, entry := range c.Tunnels {
		m[sub] = entry.Local
	}
	return m
}
