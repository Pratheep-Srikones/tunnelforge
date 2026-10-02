package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"tunnelforge/agent/forge/capture"
	tunnel "tunnelforge/agent/forge/config"
	"tunnelforge/agent/forge/ui"
)

func TestLogs_FetchAndFormat(t *testing.T) {
	rb := capture.NewRingBuffer(10)
	now := time.Date(2026, 10, 2, 14, 30, 0, 0, time.UTC)
	_ = rb.Push("web", &capture.RequestEntry{
		ID:             "1",
		Subdomain:      "web",
		Method:         http.MethodGet,
		URL:            "/index.html",
		ResponseStatus: http.StatusOK,
		DurationMS:     12,
		Timestamp:      now,
	})
	_ = rb.Push("api", &capture.RequestEntry{
		ID:             "2",
		Subdomain:      "api",
		Method:         http.MethodPost,
		URL:            "/v1/users",
		ResponseStatus: http.StatusCreated,
		DurationMS:     45,
		Timestamp:      now.Add(time.Second),
		Replayed:       true,
	})

	srv := ui.NewServer("0", rb, map[string]tunnel.TunnelEntry{
		"web": {Local: "localhost:3000", Capture: true},
		"api": {Local: "localhost:8080", Capture: true},
	}, nil)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	port := strings.TrimPrefix(ts.URL, "http://127.0.0.1:")

	// 1. Human-readable output across all tunnels
	var out bytes.Buffer
	err := RunLogs(context.Background(), LogsOptions{
		Port:       port,
		Limit:      10,
		Follow:     false,
		JSONOutput: false,
		Out:        &out,
	})
	if err != nil {
		t.Fatalf("RunLogs error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "[web] GET /index.html 200 OK (12ms)") {
		t.Errorf("expected web entry in output, got: %s", output)
	}
	if !strings.Contains(output, "[api] [REPLAY] POST /v1/users 201 Created (45ms)") {
		t.Errorf("expected api replay entry in output, got: %s", output)
	}

	// 2. Filter by subdomain
	var outWeb bytes.Buffer
	err = RunLogs(context.Background(), LogsOptions{
		Port:       port,
		Subdomain:  "web",
		Limit:      10,
		Follow:     false,
		JSONOutput: false,
		Out:        &outWeb,
	})
	if err != nil {
		t.Fatalf("RunLogs filtered error: %v", err)
	}
	if !strings.Contains(outWeb.String(), "[web] GET /index.html") {
		t.Errorf("expected web log, got: %s", outWeb.String())
	}
	if strings.Contains(outWeb.String(), "[api]") {
		t.Errorf("did not expect api log in web filtered output, got: %s", outWeb.String())
	}

	// 3. JSON output
	var outJSON bytes.Buffer
	err = RunLogs(context.Background(), LogsOptions{
		Port:       port,
		Subdomain:  "api",
		Limit:      10,
		Follow:     false,
		JSONOutput: true,
		Out:        &outJSON,
	})
	if err != nil {
		t.Fatalf("RunLogs JSON error: %v", err)
	}

	var parsed capture.RequestEntry
	if err := json.Unmarshal(outJSON.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON line: %v (raw: %s)", err, outJSON.String())
	}
	if parsed.Subdomain != "api" || parsed.URL != "/v1/users" || !parsed.Replayed {
		t.Errorf("unexpected parsed JSON entry: %+v", parsed)
	}
}

func TestLogs_FollowStream(t *testing.T) {
	srv := ui.NewServer("0", nil, nil, nil)
	go srv.Hub.Run()
	defer srv.Hub.Stop()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	port := strings.TrimPrefix(ts.URL, "http://127.0.0.1:")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunLogs(ctx, LogsOptions{
			Port:       port,
			Limit:      0, // skip history
			Follow:     true,
			JSONOutput: false,
			Out:        &out,
		})
	}()

	// Wait for WebSocket connection to establish
	time.Sleep(100 * time.Millisecond)

	// Broadcast an event
	_ = srv.Hub.BroadcastJSON(ui.UIEvent{
		Type:      "request",
		Subdomain: "stream-app",
		Entry: &capture.RequestEntry{
			ID:             "ws-1",
			Subdomain:      "stream-app",
			Method:         http.MethodGet,
			URL:            "/live-check",
			ResponseStatus: http.StatusOK,
			DurationMS:     8,
			Timestamp:      time.Now(),
		},
	})

	time.Sleep(100 * time.Millisecond)
	cancel() // Stop following

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunLogs follow error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RunLogs to exit")
	}

	if !strings.Contains(out.String(), "[stream-app] GET /live-check 200 OK (8ms)") {
		t.Errorf("expected live streamed entry in output, got: %s", out.String())
	}
}

func TestLogs_ServerDown(t *testing.T) {
	var out bytes.Buffer
	err := RunLogs(context.Background(), LogsOptions{
		Port:       "59998",
		Limit:      10,
		Follow:     false,
		JSONOutput: false,
		Out:        &out,
	})
	if err == nil {
		t.Fatal("expected error when server is offline, got nil")
	}
	if !strings.Contains(err.Error(), "Is 'forge up' running?") {
		t.Errorf("expected helpful error message, got: %v", err)
	}
}
