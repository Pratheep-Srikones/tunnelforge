package client_test

import (
	"testing"
	"tunnelforge/agent/forge/client"
)

func TestResolveServerEndpoints(t *testing.T) {
	tests := []struct {
		input       string
		expected    client.ServerEndpoints
	}{
		{
			input: "localhost:8000",
			expected: client.ServerEndpoints{
				Host:    "localhost",
				RESTURL: "http://localhost:8000",
				TCPAddr: "localhost:7000",
				WSURL:   "ws://localhost:8000/tunnel",
				IsTLS:   false,
			},
		},
		{
			input: "http://localhost:8000",
			expected: client.ServerEndpoints{
				Host:    "localhost",
				RESTURL: "http://localhost:8000",
				TCPAddr: "localhost:7000",
				WSURL:   "ws://localhost:8000/tunnel",
				IsTLS:   false,
			},
		},
		{
			input: "141.148.208.164:8000",
			expected: client.ServerEndpoints{
				Host:    "141.148.208.164",
				RESTURL: "http://141.148.208.164:8000",
				TCPAddr: "141.148.208.164:7000",
				WSURL:   "ws://141.148.208.164:8000/tunnel",
				IsTLS:   false,
			},
		},
		{
			input: "tunnel.mydomain.com",
			expected: client.ServerEndpoints{
				Host:    "tunnel.mydomain.com",
				RESTURL: "https://tunnel.mydomain.com",
				TCPAddr: "tunnel.mydomain.com:7000",
				WSURL:   "wss://tunnel.mydomain.com/tunnel",
				IsTLS:   true,
			},
		},
		{
			input: "https://tunnel.mydomain.com",
			expected: client.ServerEndpoints{
				Host:    "tunnel.mydomain.com",
				RESTURL: "https://tunnel.mydomain.com",
				TCPAddr: "tunnel.mydomain.com:7000",
				WSURL:   "wss://tunnel.mydomain.com/tunnel",
				IsTLS:   true,
			},
		},
		{
			input: "https://tunnel.mydomain.com:8443",
			expected: client.ServerEndpoints{
				Host:    "tunnel.mydomain.com",
				RESTURL: "https://tunnel.mydomain.com:8443",
				TCPAddr: "tunnel.mydomain.com:7000",
				WSURL:   "wss://tunnel.mydomain.com:8443/tunnel",
				IsTLS:   true,
			},
		},
	}

	for _, tt := range tests {
		got := client.ResolveServerEndpoints(tt.input)
		if got.RESTURL != tt.expected.RESTURL {
			t.Errorf("Resolve(%q).RESTURL = %q, want %q", tt.input, got.RESTURL, tt.expected.RESTURL)
		}
		if got.TCPAddr != tt.expected.TCPAddr {
			t.Errorf("Resolve(%q).TCPAddr = %q, want %q", tt.input, got.TCPAddr, tt.expected.TCPAddr)
		}
		if got.WSURL != tt.expected.WSURL {
			t.Errorf("Resolve(%q).WSURL = %q, want %q", tt.input, got.WSURL, tt.expected.WSURL)
		}
		if got.IsTLS != tt.expected.IsTLS {
			t.Errorf("Resolve(%q).IsTLS = %v, want %v", tt.input, got.IsTLS, tt.expected.IsTLS)
		}
	}
}
