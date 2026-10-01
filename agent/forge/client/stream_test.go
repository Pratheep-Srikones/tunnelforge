package client

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
	"tunnelforge/agent/forge/capture"
	tunnel "tunnelforge/agent/forge/config"
	"tunnelforge/internal/proto"
)

func TestForwardWithCapture_GET(t *testing.T) {
	rb := capture.NewRingBuffer(10)
	c := &Client{
		Capturer: rb,
		Tunnels: map[string]tunnel.TunnelEntry{
			"api": {Local: "mock:80", Capture: true},
		},
	}

	// streamPipe: streamClient <---> streamServer (passed to forwardWithCapture)
	streamClient, streamServer := net.Pipe()
	defer streamClient.Close()
	defer streamServer.Close()

	// localPipe: localConn (passed to forwardWithCapture) <---> mockBackend
	localConn, mockBackend := net.Pipe()
	defer localConn.Close()
	defer mockBackend.Close()

	// 1. Mock local backend handler
	go func() {
		req, err := http.ReadRequest(bufio.NewReader(mockBackend))
		if err != nil {
			return
		}
		if req.URL.Path != "/users" {
			t.Errorf("expected /users, got %s", req.URL.Path)
		}

		bodyStr := `{"status":"ok"}`
		resp := &http.Response{
			StatusCode:    http.StatusOK,
			ProtoMajor:    1,
			ProtoMinor:    1,
			Header:        make(http.Header),
			Body:          io.NopCloser(bytes.NewBufferString(bodyStr)),
			ContentLength: int64(len(bodyStr)),
		}
		resp.Header.Set("Content-Type", "application/json")
		_ = resp.Write(mockBackend)
	}()

	// 2. Client sends HTTP request into streamClient
	go func() {
		req, _ := http.NewRequest(http.MethodGet, "http://api.example.com/users", nil)
		_ = req.Write(streamClient)
	}()

	// 3. forwardWithCapture executes
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.forwardWithCapture(streamServer, localConn, "api")
	}()

	// 4. Client reads response from streamClient
	resp, err := http.ReadResponse(bufio.NewReader(streamClient), nil)
	if err != nil {
		t.Fatalf("failed reading response on stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("expected {\"status\":\"ok\"}, got %s", string(body))
	}

	if err := <-errCh; err != nil {
		t.Fatalf("forwardWithCapture returned error: %v", err)
	}

	// 5. Verify request captured in RingBuffer (allow async goroutine to complete)
	var entries []*capture.RequestEntry
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ = rb.List("api", 0)
		if len(entries) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 captured entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", entry.Method)
	}
	if entry.URL != "/users" {
		t.Errorf("expected /users, got %s", entry.URL)
	}
	if entry.ResponseStatus != http.StatusOK {
		t.Errorf("expected 200, got %d", entry.ResponseStatus)
	}
	if string(entry.ResponseBody) != `{"status":"ok"}` {
		t.Errorf("expected body in capture, got %s", string(entry.ResponseBody))
	}
	if entry.Truncated {
		t.Errorf("expected truncated to be false")
	}
}

func TestForwardWithCapture_POST_WithBody(t *testing.T) {
	rb := capture.NewRingBuffer(10)
	c := &Client{
		Capturer: rb,
		Tunnels: map[string]tunnel.TunnelEntry{
			"web": {Local: "mock:80", Capture: true},
		},
	}

	streamClient, streamServer := net.Pipe()
	defer streamClient.Close()
	defer streamServer.Close()

	localConn, mockBackend := net.Pipe()
	defer localConn.Close()
	defer mockBackend.Close()

	// 1. Mock local backend handler
	go func() {
		req, err := http.ReadRequest(bufio.NewReader(mockBackend))
		if err != nil {
			return
		}
		reqBody, _ := io.ReadAll(req.Body)
		if string(reqBody) != `{"name":"alice"}` {
			t.Errorf("expected req body {\"name\":\"alice\"}, got %s", string(reqBody))
		}

		resp := &http.Response{
			StatusCode:    http.StatusCreated,
			ProtoMajor:    1,
			ProtoMinor:    1,
			Header:        make(http.Header),
			Body:          io.NopCloser(bytes.NewBufferString(`{"id":1}`)),
			ContentLength: 8,
		}
		resp.Header.Set("Content-Type", "application/json")
		_ = resp.Write(mockBackend)
	}()

	// 2. Client sends HTTP POST request with body
	go func() {
		reqBody := bytes.NewBufferString(`{"name":"alice"}`)
		req, _ := http.NewRequest(http.MethodPost, "http://web.example.com/items", reqBody)
		req.ContentLength = int64(reqBody.Len())
		req.Header.Set("Content-Type", "application/json")
		_ = req.Write(streamClient)
	}()

	// 3. forwardWithCapture executes
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.forwardWithCapture(streamServer, localConn, "web")
	}()

	// 4. Client reads response
	resp, err := http.ReadResponse(bufio.NewReader(streamClient), nil)
	if err != nil {
		t.Fatalf("failed reading response on stream: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"id":1}` {
		t.Fatalf("expected {\"id\":1}, got %s", string(body))
	}

	if err := <-errCh; err != nil {
		t.Fatalf("forwardWithCapture returned error: %v", err)
	}

	// 5. Verify captured entry
	var entries []*capture.RequestEntry
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ = rb.List("web", 0)
		if len(entries) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 captured entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", entry.Method)
	}
	if string(entry.RequestBody) != `{"name":"alice"}` {
		t.Errorf("expected captured request body, got %s", string(entry.RequestBody))
	}
	if string(entry.ResponseBody) != `{"id":1}` {
		t.Errorf("expected captured response body, got %s", string(entry.ResponseBody))
	}
	if entry.ResponseStatus != http.StatusCreated {
		t.Errorf("expected 201, got %d", entry.ResponseStatus)
	}
}

func TestForward_RoutingToCaptureOrRaw(t *testing.T) {
	// Tests forward() reads the frame header and routes to capture if enabled
	rb := capture.NewRingBuffer(5)

	// Create a local backend listener
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, err := http.ReadRequest(bufio.NewReader(c))
				if err != nil {
					return
				}
				bodyStr := "captured-ok"
				resp := &http.Response{
					StatusCode:    http.StatusOK,
					ProtoMajor:    1,
					ProtoMinor:    1,
					Header:        make(http.Header),
					Body:          io.NopCloser(bytes.NewBufferString(bodyStr)),
					ContentLength: int64(len(bodyStr)),
				}
				_ = resp.Write(c)
			}(conn)
		}
	}()

	c := &Client{
		Capturer: rb,
		Tunnels: map[string]tunnel.TunnelEntry{
			"routed-app": {Local: backendListener.Addr().String(), Capture: true},
		},
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	// Write frame header first
	go func() {
		var headerBuf bytes.Buffer
		_ = proto.WriteStreamHeader(&headerBuf, "routed-app")
		_, _ = clientConn.Write(headerBuf.Bytes())

		req, _ := http.NewRequest(http.MethodGet, "http://routed-app/test", nil)
		_ = req.Write(clientConn)
	}()

	// Execute forward
	go func() {
		_ = c.forward(serverConn)
	}()

	// Read response on clientConn (after stream header has been consumed by forward)
	resp, err := http.ReadResponse(bufio.NewReader(clientConn), nil)
	if err != nil {
		t.Fatalf("failed reading response: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "captured-ok" {
		t.Fatalf("expected 'captured-ok', got %q", string(body))
	}

	// Verify RingBuffer received entry
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := rb.List("routed-app", 0)
		if len(entries) > 0 {
			if entries[0].URL != "/test" {
				t.Fatalf("expected /test, got %s", entries[0].URL)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected request to be captured in ring buffer")
}

func TestForwardWithCapture_NonHTTPFallback(t *testing.T) {
	rb := capture.NewRingBuffer(5)
	c := &Client{
		Capturer: rb,
		Tunnels:  map[string]tunnel.TunnelEntry{},
	}

	streamClient, streamServer := net.Pipe()
	defer streamClient.Close()
	defer streamServer.Close()

	localConn, mockBackend := net.Pipe()
	defer localConn.Close()
	defer mockBackend.Close()

	// Backend echoes whatever it receives and closes
	go func() {
		buf := make([]byte, 128)
		n, _ := mockBackend.Read(buf)
		_, _ = mockBackend.Write(append([]byte("echo: "), buf[:n]...))
		_ = mockBackend.Close()
	}()

	// Client sends arbitrary non-HTTP binary data
	go func() {
		_, _ = streamClient.Write([]byte("NON-HTTP-DATA\r\n\r\n"))
	}()

	go func() {
		_ = c.forwardWithCapture(streamServer, localConn, "raw")
	}()

	// Read response on client
	reply := make([]byte, 128)
	n, err := streamClient.Read(reply)
	if err != nil && err != io.EOF {
		t.Fatalf("read error: %v", err)
	}

	got := string(reply[:n])
	if got != "echo: NON-HTTP-DATA\r\n\r\n" {
		t.Fatalf("expected 'echo: NON-HTTP-DATA\\r\\n\\r\\n', got %q", got)
	}
}
