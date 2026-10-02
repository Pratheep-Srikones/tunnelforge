package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"tunnelforge/agent/forge/capture"
	tunnel "tunnelforge/agent/forge/config"
	"tunnelforge/agent/forge/replay"

	"github.com/gorilla/websocket"
)

func TestServer_Health(t *testing.T) {
	srv := NewServer("0", nil, nil, nil)
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

	srv := NewServer("0", nil, tunnels, nil)
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
	}, nil)
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
	srv := NewServer("0", nil, nil, nil)
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

func TestServer_ReplayAPI(t *testing.T) {
	localBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"pong"}`))
	}))
	defer localBackend.Close()

	localAddr := strings.TrimPrefix(localBackend.URL, "http://")

	rb := capture.NewRingBuffer(10)
	origEntry := &capture.RequestEntry{
		ID:              "req-to-replay",
		Subdomain:       "api",
		Method:          http.MethodGet,
		URL:             "/ping",
		ResponseStatus:  http.StatusOK,
		ResponseHeaders: http.Header{},
		ResponseBody:    []byte(`{"message":"orig"}`),
		Timestamp:       time.Now(),
	}
	_ = rb.Push("api", origEntry)

	replayer := replay.NewReplayer(&http.Client{Timeout: 2 * time.Second})
	srv := NewServer("0", rb, map[string]tunnel.TunnelEntry{
		"api": {Local: localAddr, Capture: true},
	}, replayer)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Success replay with capture=true
	bodyBytes, _ := json.Marshal(replay.ReplayRequest{
		Subdomain: "api",
		RequestID: "req-to-replay",
		Capture:   true,
	})
	resp, err := http.Post(ts.URL+"/api/requests/replay", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("replay request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var replayed capture.RequestEntry
	if err := json.NewDecoder(resp.Body).Decode(&replayed); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !replayed.Replayed {
		t.Errorf("expected Replayed to be true")
	}
	if string(replayed.ResponseBody) != `{"message":"pong"}` {
		t.Errorf("expected pong response, got %s", string(replayed.ResponseBody))
	}

	// Verify it was pushed to the ring buffer (now len should be 2)
	list, _ := rb.List("api", 10)
	if len(list) != 2 {
		t.Errorf("expected 2 entries in ring buffer after replay capture, got %d", len(list))
	}

	// 2. Request not found (404)
	badBody, _ := json.Marshal(replay.ReplayRequest{
		Subdomain: "api",
		RequestID: "non-existent-id",
		Capture:   false,
	})
	badResp, err := http.Post(ts.URL+"/api/requests/replay", "application/json", bytes.NewReader(badBody))
	if err != nil {
		t.Fatalf("replay request failed: %v", err)
	}
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", badResp.StatusCode)
	}

	// 3. Method not allowed (405)
	getResp, err := http.Get(ts.URL + "/api/requests/replay")
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", getResp.StatusCode)
	}
}

