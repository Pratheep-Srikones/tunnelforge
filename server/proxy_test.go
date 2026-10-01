package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tunnelforge/agent/forge/client"
	"tunnelforge/server/auth"

	"github.com/gin-gonic/gin"
)

func setupTestProxyRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.NoRoute(proxyHandler)
	return r
}

func TestProxy_EndToEnd_GET(t *testing.T) {
	// 1. Start mock local backend service (similar to test-client)
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hello" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Custom-Header", "tunnelforge-test")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":"hello"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer mockBackend.Close()

	backendAddr := mockBackend.Listener.Addr().String()

	// 2. Setup server agent listener & auth
	authReg := auth.GetAuthRegistry()
	agentID := "agent_proxy_get"
	token := "token_proxy_get_123"
	if err := authReg.Register(agentID, token); err != nil {
		t.Fatalf("failed to register agent auth: %v", err)
	}

	agentListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start agent listener: %v", err)
	}
	defer agentListener.Close()

	go func() {
		conn, err := agentListener.Accept()
		if err != nil {
			return
		}
		handleAgent(conn)
	}()

	// 3. Connect agent client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subdomain := "my-app"
	c := client.NewSimple(
		agentListener.Addr().String(),
		token,
		agentID,
		1,
		map[string]string{subdomain: backendAddr},
	)

	go func() {
		_, _ = c.RunOnce(ctx)
	}()

	_, ok := waitForTunnel(subdomain, 3*time.Second)
	if !ok {
		t.Fatalf("timed out waiting for tunnel %q to register", subdomain)
	}

	// 4. Start HTTP proxy server
	router := setupTestProxyRouter()
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	httpClient := proxyServer.Client()

	// 5. Test GET /
	t.Run("GET root path", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyServer.URL+"/", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Host = subdomain + ".example.com"

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("GET / request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		expectedBody := `{"message":"ok"}`
		if string(body) != expectedBody {
			t.Errorf("expected body %q, got %q", expectedBody, string(body))
		}
	})

	// 6. Test GET /hello with custom headers
	t.Run("GET /hello with headers", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyServer.URL+"/hello", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Host = subdomain + ".example.com"

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("GET /hello request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if customHeader := resp.Header.Get("X-Custom-Header"); customHeader != "tunnelforge-test" {
			t.Errorf("expected X-Custom-Header 'tunnelforge-test', got %q", customHeader)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		expectedBody := `{"message":"hello"}`
		if string(body) != expectedBody {
			t.Errorf("expected body %q, got %q", expectedBody, string(body))
		}
	})
}

func TestProxy_EndToEnd_POST(t *testing.T) {
	type echoPayload struct {
		Action string `json:"action"`
		Count  int    `json:"count"`
	}

	type echoResponse struct {
		Received echoPayload `json:"received"`
		Status   string      `json:"status"`
	}

	// 1. Mock local backend validating POST body and headers
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.Header.Get("X-Client-Trace") != "trace-12345" {
			http.Error(w, "missing or invalid X-Client-Trace header", http.StatusBadRequest)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var payload echoPayload
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		resp := echoResponse{
			Received: payload,
			Status:   "created",
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Server-Processed", "true")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockBackend.Close()

	backendAddr := mockBackend.Listener.Addr().String()

	// 2. Setup server agent listener & auth
	authReg := auth.GetAuthRegistry()
	agentID := "agent_proxy_post"
	token := "token_proxy_post_123"
	if err := authReg.Register(agentID, token); err != nil {
		t.Fatalf("failed to register agent auth: %v", err)
	}

	agentListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start agent listener: %v", err)
	}
	defer agentListener.Close()

	go func() {
		conn, err := agentListener.Accept()
		if err != nil {
			return
		}
		handleAgent(conn)
	}()

	// 3. Connect agent client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subdomain := "api-service"
	c := client.NewSimple(
		agentListener.Addr().String(),
		token,
		agentID,
		1,
		map[string]string{subdomain: backendAddr},
	)

	go func() {
		_, _ = c.RunOnce(ctx)
	}()

	_, ok := waitForTunnel(subdomain, 3*time.Second)
	if !ok {
		t.Fatalf("timed out waiting for tunnel %q to register", subdomain)
	}

	// 4. Start HTTP proxy server
	router := setupTestProxyRouter()
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	// 5. Send POST request with JSON body
	reqPayload := echoPayload{
		Action: "create_item",
		Count:  42,
	}
	reqData, _ := json.Marshal(reqPayload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyServer.URL+"/api/items", bytes.NewReader(reqData))
	if err != nil {
		t.Fatalf("failed to create POST request: %v", err)
	}
	req.Host = subdomain + ".example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Trace", "trace-12345")

	resp, err := proxyServer.Client().Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}

	if processed := resp.Header.Get("X-Server-Processed"); processed != "true" {
		t.Errorf("expected X-Server-Processed 'true', got %q", processed)
	}

	var resPayload echoResponse
	if err := json.NewDecoder(resp.Body).Decode(&resPayload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resPayload.Status != "created" {
		t.Errorf("expected status 'created', got %q", resPayload.Status)
	}
	if resPayload.Received.Action != "create_item" || resPayload.Received.Count != 42 {
		t.Errorf("echoed payload mismatch: %+v", resPayload.Received)
	}
}

func TestProxy_SubdomainNotFound(t *testing.T) {
	router := setupTestProxyRouter()
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	req, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Host = "unregistered-subdomain.example.com"

	resp, err := proxyServer.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected status 502 Bad Gateway for unregistered subdomain, got %d", resp.StatusCode)
	}

	var res map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res["message"] != "agent not found" {
		t.Errorf("expected message 'agent not found', got %q", res["message"])
	}
}

func TestProxy_InvalidHostname(t *testing.T) {
	router := setupTestProxyRouter()
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	req, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Host = "localhost" // Missing subdomain (no dot)

	resp, err := proxyServer.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected status 502 Bad Gateway for invalid hostname, got %d", resp.StatusCode)
	}
}

func TestProxy_EndToEnd_MultiTunnel(t *testing.T) {
	// 1. Start two distinct mock backend services
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Service-ID", "service-1")
		_, _ = w.Write([]byte(`{"service":"backend-one"}`))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Service-ID", "service-2")
		_, _ = w.Write([]byte(`{"service":"backend-two"}`))
	}))
	defer backend2.Close()

	// 2. Setup agent listener and auth
	authReg := auth.GetAuthRegistry()
	agentID := "agent_proxy_multi"
	token := "token_proxy_multi_123"
	if err := authReg.Register(agentID, token); err != nil {
		t.Fatalf("failed to register agent auth: %v", err)
	}

	agentListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start agent listener: %v", err)
	}
	defer agentListener.Close()

	go func() {
		conn, err := agentListener.Accept()
		if err != nil {
			return
		}
		handleAgent(conn)
	}()

	// 3. Connect agent with both tunnels mapped
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tunnels := map[string]string{
		"app-one": backend1.Listener.Addr().String(),
		"app-two": backend2.Listener.Addr().String(),
	}

	c := client.NewSimple(
		agentListener.Addr().String(),
		token,
		agentID,
		1,
		tunnels,
	)

	go func() {
		_, _ = c.RunOnce(ctx)
	}()

	for sub := range tunnels {
		if _, ok := waitForTunnel(sub, 3*time.Second); !ok {
			t.Fatalf("timed out waiting for tunnel %q to register", sub)
		}
	}

	// 4. Start HTTP proxy server
	router := setupTestProxyRouter()
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	httpClient := proxyServer.Client()

	// 5. Test request to app-one
	t.Run("Route to app-one", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxyServer.URL+"/api", nil)
		req.Host = "app-one.example.com"

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("request to app-one failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		if sID := resp.Header.Get("X-Service-ID"); sID != "service-1" {
			t.Errorf("expected service-1, got %q", sID)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != `{"service":"backend-one"}` {
			t.Errorf("unexpected body: %s", string(body))
		}
	})

	// 6. Test request to app-two
	t.Run("Route to app-two", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxyServer.URL+"/api", nil)
		req.Host = "app-two.example.com"

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("request to app-two failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		if sID := resp.Header.Get("X-Service-ID"); sID != "service-2" {
			t.Errorf("expected service-2, got %q", sID)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != `{"service":"backend-two"}` {
			t.Errorf("unexpected body: %s", string(body))
		}
	})
}

func TestProxy_ForwardingHeaders(t *testing.T) {
	// 1. Mock backend verifying X-Forwarded-* headers (FR-04-2)
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]string{
			"x_forwarded_for":   r.Header.Get("X-Forwarded-For"),
			"x_forwarded_proto": r.Header.Get("X-Forwarded-Proto"),
			"x_tunnel_host":     r.Header.Get("X-Tunnel-Host"),
			"x_forwarded_host":  r.Header.Get("X-Forwarded-Host"),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer mockBackend.Close()

	authReg := auth.GetAuthRegistry()
	agentID := "agent_proxy_headers"
	token := "token_proxy_headers_123"
	_ = authReg.Register(agentID, token)

	agentListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer agentListener.Close()

	go func() {
		conn, err := agentListener.Accept()
		if err != nil {
			return
		}
		handleAgent(conn)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subdomain := "headers-app"
	c := client.NewSimple(
		agentListener.Addr().String(),
		token,
		agentID,
		1,
		map[string]string{subdomain: mockBackend.Listener.Addr().String()},
	)

	go func() {
		_, _ = c.RunOnce(ctx)
	}()

	if _, ok := waitForTunnel(subdomain, 3*time.Second); !ok {
		t.Fatalf("timed out waiting for tunnel %q to register", subdomain)
	}

	router := setupTestProxyRouter()
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxyServer.URL+"/check-headers", nil)
	req.Host = subdomain + ".example.com"

	resp, err := proxyServer.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var headers map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&headers); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if headers["x_forwarded_for"] == "" {
		t.Error("expected X-Forwarded-For to be injected")
	}
	if headers["x_forwarded_proto"] != "http" {
		t.Errorf("expected X-Forwarded-Proto 'http', got %q", headers["x_forwarded_proto"])
	}
	if headers["x_tunnel_host"] != subdomain+".example.com" {
		t.Errorf("expected X-Tunnel-Host '%s.example.com', got %q", subdomain, headers["x_tunnel_host"])
	}
	if headers["x_forwarded_host"] != subdomain+".example.com" {
		t.Errorf("expected X-Forwarded-Host '%s.example.com', got %q", subdomain, headers["x_forwarded_host"])
	}
}

func TestProxy_LocalBackendDown(t *testing.T) {
	// 1. Point tunnel to an unreachable local port where no backend is listening
	unreachableAddr := "127.0.0.1:59999"

	authReg := auth.GetAuthRegistry()
	agentID := "agent_proxy_down"
	token := "token_proxy_down_123"
	_ = authReg.Register(agentID, token)

	agentListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer agentListener.Close()

	go func() {
		conn, err := agentListener.Accept()
		if err != nil {
			return
		}
		handleAgent(conn)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subdomain := "down-app"
	c := client.NewSimple(
		agentListener.Addr().String(),
		token,
		agentID,
		1,
		map[string]string{subdomain: unreachableAddr},
	)

	go func() {
		_, _ = c.RunOnce(ctx)
	}()

	if _, ok := waitForTunnel(subdomain, 3*time.Second); !ok {
		t.Fatalf("timed out waiting for tunnel %q to register", subdomain)
	}

	router := setupTestProxyRouter()
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxyServer.URL+"/ping", nil)
	req.Host = subdomain + ".example.com"

	resp, err := proxyServer.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected status 502 Bad Gateway when local backend is down, got %d", resp.StatusCode)
	}

	var res map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if res["message"] != "local service unreachable" {
		t.Errorf("expected message 'local service unreachable', got %q", res["message"])
	}
}
