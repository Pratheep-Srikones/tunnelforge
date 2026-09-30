package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"tunnelforge/internal/proto"

	"github.com/gin-gonic/gin"
)

func proxyHandler(c *gin.Context) {
	host := c.Request.Host

	subdomain, err := extractSubdomain(host)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": err.Error(),
		})
		return
	}

	fmt.Println("Incoming request for:", subdomain)
	fmt.Println("Path:", c.Request.URL.Path)

	t, ok := registry.Get(subdomain)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "agent not found",
		})
		return
	}

	stream, err := t.Session.Open()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "failed to open tunnel stream",
		})
		return
	}

	defer stream.Close()

	// Write stream header so the agent knows which subdomain this request is for.
	if err := proto.WriteStreamHeader(stream, subdomain); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "failed to write stream header",
		})
		return
	}

	if err := forwardRequest(c, stream); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "failed to forward request to agent",
		})
		return
	}

	fmt.Println("[Proxy] Request forwarded to agent")

	if err := forwardResponse(c, stream); err != nil {
		fmt.Println("[Proxy] Error forwarding response:", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "local service unreachable",
		})
	}
}

func extractSubdomain(host string) (string, error) {
	if host == "" {
		return "", fmt.Errorf("missing host")
	}

	// Host may be:
	//
	// test_app.example.com:8000
	// test_app.example.com
	// test_app.localhost:8000

	if hostWithPort, _, err := net.SplitHostPort(host); err == nil {
		host = hostWithPort
	}

	parts := strings.Split(host, ".")

	if len(parts) < 2 {
		return "", fmt.Errorf("invalid tunnel hostname: %s", host)
	}

	if parts[0] == "" {
		return "", fmt.Errorf("empty subdomain")
	}

	return parts[0], nil
}

func forwardRequest(c *gin.Context, stream net.Conn) error {
	req := c.Request

	// Inject forwarding headers (FR-04-2)
	clientIP := c.ClientIP()
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		req.Header.Set("X-Forwarded-For", xff+", "+clientIP)
	} else if clientIP != "" {
		req.Header.Set("X-Forwarded-For", clientIP)
	}

	scheme := "http"
	if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	req.Header.Set("X-Forwarded-Proto", scheme)

	if req.Host != "" {
		req.Header.Set("X-Tunnel-Host", req.Host)
		if req.Header.Get("X-Forwarded-Host") == "" {
			req.Header.Set("X-Forwarded-Host", req.Host)
		}
	}

	if err := req.Write(stream); err != nil {
		return fmt.Errorf("writing request: %w", err)
	}

	return nil
}

func forwardResponse(c *gin.Context, stream net.Conn) error {
	response, err := http.ReadResponse(
		bufio.NewReader(stream),
		c.Request,
	)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	defer response.Body.Close()

	copyResponseHeaders(c, response)

	c.Status(response.StatusCode)

	if _, err := io.Copy(c.Writer, response.Body); err != nil {
		return fmt.Errorf("copying response body: %w", err)
	}

	return nil
}

func copyResponseHeaders(c *gin.Context, response *http.Response) {
	for key, values := range response.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
}
