package client_test

import (
	"testing"
	"tunnelforge/agent/forge/client"
)

func TestNormalizeTCPAddr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://141.148.208.164:8000", "141.148.208.164:7000"},
		{"http://141.148.208.164:7000", "141.148.208.164:7000"},
		{"https://example.com:8000/path", "example.com:7000"},
		{"141.148.208.164:7000", "141.148.208.164:7000"},
		{"141.148.208.164:8000", "141.148.208.164:7000"},
		{"141.148.208.164", "141.148.208.164:7000"},
		{"http://localhost:8000", "localhost:7000"},
		{"localhost:7000", "localhost:7000"},
	}

	for _, tt := range tests {
		got := client.NormalizeTCPAddr(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeTCPAddr(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}
