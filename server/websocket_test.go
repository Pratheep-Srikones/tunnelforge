package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tunnelforge/agent/forge/client"
	"tunnelforge/internal/transport"
	"tunnelforge/server/auth"
	"tunnelforge/server/routes"

	"github.com/gin-gonic/gin"
)

func setupTestWSServer() (*gin.Engine, *httptest.Server) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// WebSocket tunnel endpoint
	r.GET("/tunnel", func(ctx *gin.Context) {
		wss, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			return
		}
		conn := transport.NewWSConn(wss)
		go handleAgent(conn)
	})

	internal := r.Group("/forge/internal")
	routes.UseAuthRoutes(internal)
	routes.UseInternalRoutes(internal, registry)

	r.NoRoute(proxyHandler)

	ts := httptest.NewServer(r)
	return r, ts
}

func TestAgent_WebSocketTransport_Forced(t *testing.T) {
	_, ts := setupTestWSServer()
	defer ts.Close()

	authReg := auth.GetAuthRegistry()
	agentID := "agent_ws_forced"
	token := "token_ws_forced_123"
	_ = authReg.Register(agentID, token)

	// Mock local backend service
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from ws tunnel backend"))
	}))
	defer mockBackend.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subdomain := "ws-forced-app"
	c := client.NewSimple(
		ts.URL,
		token,
		agentID,
		1,
		map[string]string{subdomain: mockBackend.Listener.Addr().String()},
	).WithWebSocket(true).WithInsecureSkipTLS(true)

	go func() {
		_, _ = c.RunOnce(ctx)
	}()

	tun, ok := waitForTunnel(subdomain, 3*time.Second)
	if !ok {
		t.Fatalf("timed out waiting for tunnel %q via WebSocket", subdomain)
	}
	if tun.AgentID != agentID {
		t.Fatalf("expected AgentID %q, got %q", agentID, tun.AgentID)
	}

	// Make an HTTP request through the proxy to verify the WebSocket tunnel works end-to-end
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/test", nil)
	req.Host = subdomain + ".example.com"

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestAgent_WebSocketTransport_AutoFallback(t *testing.T) {
	_, ts := setupTestWSServer()
	defer ts.Close()

	authReg := auth.GetAuthRegistry()
	agentID := "agent_ws_autofallback"
	token := "token_ws_autofallback_123"
	_ = authReg.Register(agentID, token)

	// Mock local backend service
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from autofallback ws backend"))
	}))
	defer mockBackend.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subdomain := "ws-fallback-app"
	// Auto mode (WithWebSocket(false)): port 7000 will fail, then it will probe ts.URL,
	// see health check 200 OK, and automatically switch to WebSocket!
	c := client.NewSimple(
		ts.URL,
		token,
		agentID,
		1,
		map[string]string{subdomain: mockBackend.Listener.Addr().String()},
	).WithInsecureSkipTLS(true)

	go func() {
		_, _ = c.RunOnce(ctx)
	}()

	tun, ok := waitForTunnel(subdomain, 5*time.Second)
	if !ok {
		t.Fatalf("timed out waiting for tunnel %q via auto-fallback WebSocket", subdomain)
	}
	if tun.AgentID != agentID {
		t.Fatalf("expected AgentID %q, got %q", agentID, tun.AgentID)
	}

	fmt.Printf("[Test] Successfully verified auto-fallback tunnel: %s\n", tun.Subdomain)
}
