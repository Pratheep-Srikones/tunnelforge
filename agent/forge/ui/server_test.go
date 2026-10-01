package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"tunnelforge/agent/forge/capture"
	tunnel "tunnelforge/agent/forge/config"

	"github.com/gorilla/websocket"
)

func TestServer_Health(t *testing.T) {
	srv := NewServer("0", nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
}

func TestServer_TunnelsAPI(t *testing.T) {
	tunnels := map[string]tunnel.TunnelEntry{
		"test-app": {Local: "localhost:3000", Capture: true, CaptureLimit: 100},
		"my-app":   {Local: "localhost:5173", Capture: false},
	}

	srv := NewServer("0", nil, tunnels)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/tunnels")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var got map[string]tunnel.TunnelEntry
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(got))
	}
	if got["test-app"].Local != "localhost:3000" || !got["test-app"].Capture {
		t.Errorf("unexpected test-app config: %+v", got["test-app"])
	}
}

func TestServer_RequestsAPI(t *testing.T) {
	rb := capture.NewRingBuffer(10)
	_ = rb.Push("api", &capture.RequestEntry{
		ID:             "req-1",
		Subdomain:      "api",
		Method:         http.MethodGet,
		URL:            "/users",
		ResponseStatus: http.StatusOK,
		Timestamp:      time.Now(),
	})
	_ = rb.Push("api", &capture.RequestEntry{
		ID:             "req-2",
		Subdomain:      "api",
		Method:         http.MethodPost,
		URL:            "/users",
		ResponseStatus: http.StatusCreated,
		Timestamp:      time.Now().Add(time.Second),
	})

	srv := NewServer("0", rb, map[string]tunnel.TunnelEntry{
		"api": {Local: "localhost:3000", Capture: true},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. GET /api/requests?subdomain=api
	resp, err := http.Get(ts.URL + "/api/requests?subdomain=api&limit=10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var entries []*capture.RequestEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decoding error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != "req-1" || entries[1].ID != "req-2" {
		t.Errorf("unexpected entry IDs: %s, %s", entries[0].ID, entries[1].ID)
	}

	// 2. DELETE /api/requests?subdomain=api
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/requests?subdomain=api", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", delResp.StatusCode)
	}

	// 3. Verify empty after delete
	resp2, _ := http.Get(ts.URL + "/api/requests?subdomain=api")
	var entries2 []*capture.RequestEntry
	_ = json.NewDecoder(resp2.Body).Decode(&entries2)
	resp2.Body.Close()
	if len(entries2) != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", len(entries2))
	}
}

func TestServer_WebSocketBroadcast(t *testing.T) {
	srv := NewServer("0", nil, nil)
	go srv.Hub.Run()
	defer srv.Hub.Stop()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Wait briefly for registration
	time.Sleep(50 * time.Millisecond)
	if srv.Hub.ClientCount() != 1 {
		t.Fatalf("expected 1 connected client, got %d", srv.Hub.ClientCount())
	}

	// Broadcast an event from server
	event := UIEvent{
		Type:      "request",
		Subdomain: "api",
		Entry: &capture.RequestEntry{
			ID:     "ws-req-1",
			Method: http.MethodGet,
			URL:    "/live",
		},
	}
	_ = srv.Hub.BroadcastJSON(event)

	// Read on client
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read WS message: %v", err)
	}

	var received UIEvent
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("failed to unmarshal WS message: %v", err)
	}
	if received.Type != "request" || received.Subdomain != "api" || received.Entry.ID != "ws-req-1" {
		t.Fatalf("unexpected event received: %+v", received)
	}
}
