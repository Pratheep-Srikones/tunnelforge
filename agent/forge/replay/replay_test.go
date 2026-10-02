package replay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"tunnelforge/agent/forge/capture"
)

func TestReplayer_Replay_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/test-path" {
			t.Errorf("expected /test-path, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Custom-Header") != "hello" {
			t.Errorf("expected X-Custom-Header=hello, got %s", r.Header.Get("X-Custom-Header"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "ping" {
			t.Errorf("expected body ping, got %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created"}`))
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	replayer := NewReplayer(&http.Client{Timeout: 5 * time.Second})

	entry := &capture.RequestEntry{
		ID:        "orig-123",
		Subdomain: "test",
		Method:    http.MethodPost,
		URL:       "/test-path",
		RequestHeaders: http.Header{
			"X-Custom-Header": []string{"hello"},
			"Connection":      []string{"keep-alive"},
		},
		RequestBody: []byte("ping"),
	}

	// Test 1: ts.URL has "http://"
	resEntry, err := replayer.Replay(ts.URL, entry)
	if err != nil {
		t.Fatalf("expected successful replay, got: %v", err)
	}

	if resEntry.ResponseStatus != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resEntry.ResponseStatus)
	}
	if string(resEntry.ResponseBody) != `{"status":"created"}` {
		t.Errorf("expected response body {\"status\":\"created\"}, got %s", string(resEntry.ResponseBody))
	}
	if !resEntry.Replayed {
		t.Errorf("expected Replayed to be true")
	}

	// Verify original entry's RequestHeaders was not mutated by header deletions
	if entry.RequestHeaders.Get("Connection") != "keep-alive" {
		t.Errorf("original entry headers were mutated! Connection header missing")
	}

	// Test 2: localAddr without "http://" prefix (e.g., "127.0.0.1:PORT" or "localhost:PORT")
	rawAddr := strings.TrimPrefix(ts.URL, "http://")
	resEntry2, err2 := replayer.Replay(rawAddr, entry)
	if err2 != nil {
		t.Fatalf("replay failed when localAddr has no http:// scheme (%s): %v", rawAddr, err2)
	}
	if resEntry2.ResponseStatus != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resEntry2.ResponseStatus)
	}
}
