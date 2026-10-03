package client

import (
	"net"
	"strings"
)

// ServerEndpoints contains the resolved URLs and network addresses for a server.
type ServerEndpoints struct {
	// Host is the raw hostname or IP address without port or scheme.
	Host string

	// RESTURL is the base HTTP/HTTPS URL for REST API calls (register, auth, health).
	// e.g. "http://localhost:8000" or "https://tunnel.mydomain.com"
	RESTURL string

	// TCPAddr is the host:port for raw TLS connection (default port 7000).
	// e.g. "localhost:7000" or "tunnel.mydomain.com:7000"
	TCPAddr string

	// WSURL is the full ws:// or wss:// URL for the fallback tunnel.
	// e.g. "ws://localhost:8000/tunnel" or "wss://tunnel.mydomain.com/tunnel"
	WSURL string

	// IsTLS indicates whether the HTTP/WS transport uses TLS (https / wss).
	IsTLS bool
}

// ResolveServerEndpoints parses a raw server address or URL and derives all needed endpoints.
func ResolveServerEndpoints(rawAddr string) ServerEndpoints {
	addr := strings.TrimSpace(rawAddr)
	isTLS := false
	schemeSpecified := false

	if strings.HasPrefix(addr, "https://") {
		isTLS = true
		schemeSpecified = true
		addr = strings.TrimPrefix(addr, "https://")
	} else if strings.HasPrefix(addr, "http://") {
		isTLS = false
		schemeSpecified = true
		addr = strings.TrimPrefix(addr, "http://")
	}

	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		port = ""
	}

	// Determine defaults if scheme or port was omitted
	if !schemeSpecified {
		if port == "443" {
			isTLS = true
		} else if port == "80" || port == "8000" || host == "localhost" || host == "127.0.0.1" {
			isTLS = false
		} else if port == "" {
			// Domain names without port default to HTTPS (:443)
			isTLS = true
		}
	}

	var httpPort string
	var tcpPort string

	switch port {
	case "":
		if isTLS {
			httpPort = "443"
		} else {
			httpPort = "80"
		}
		tcpPort = "7000"
	case "8000":
		httpPort = "8000"
		tcpPort = "7000"
	case "7000":
		tcpPort = "7000"
		if isTLS {
			httpPort = "443"
		} else {
			httpPort = "8000"
		}
	case "443":
		httpPort = "443"
		tcpPort = "7000"
		isTLS = true
	case "80":
		httpPort = "80"
		tcpPort = "7000"
		isTLS = false
	default:
		httpPort = port
		if schemeSpecified {
			tcpPort = "7000"
		} else {
			tcpPort = port
		}
	}

	// Build RESTURL
	var restURL string
	scheme := "http"
	if isTLS {
		scheme = "https"
	}

	if (isTLS && httpPort == "443") || (!isTLS && httpPort == "80") {
		restURL = scheme + "://" + host
	} else {
		restURL = scheme + "://" + net.JoinHostPort(host, httpPort)
	}

	// Build WSURL
	wsScheme := "ws"
	if isTLS {
		wsScheme = "wss"
	}

	var wsHostPort string
	if (isTLS && httpPort == "443") || (!isTLS && httpPort == "80") {
		wsHostPort = host
	} else {
		wsHostPort = net.JoinHostPort(host, httpPort)
	}
	wsURL := wsScheme + "://" + wsHostPort + "/tunnel"

	return ServerEndpoints{
		Host:    host,
		RESTURL: restURL,
		TCPAddr: net.JoinHostPort(host, tcpPort),
		WSURL:   wsURL,
		IsTLS:   isTLS,
	}
}
