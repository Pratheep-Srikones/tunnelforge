package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

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

	if err := forwardRequest(c.Request, stream); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "failed to forward request to agent",
		})
		return
	}

	fmt.Println("Request forwarded to agent")

	if err := forwardResponse(c, stream); err != nil {
		fmt.Println("Error forwarding response:", err)
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

func forwardRequest(req *http.Request, stream net.Conn) error {
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
