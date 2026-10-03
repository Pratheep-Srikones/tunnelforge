package transport_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"tunnelforge/internal/transport"

	"github.com/gorilla/websocket"
)

func TestWSConn_ReadWrite(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	serverReceived := make(chan []byte, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		defer ws.Close()

		conn := transport.NewWSConn(ws)

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			t.Errorf("server read error: %v", err)
			return
		}
		serverReceived <- buf[:n]

		_, _ = conn.Write([]byte("pong-response"))
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	clientWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("client dial error: %v", err)
	}
	defer clientWS.Close()

	clientConn := transport.NewWSConn(clientWS)
	_ = clientConn.SetDeadline(time.Now().Add(5 * time.Second))

	msg := []byte("ping-test-payload")
	n, err := clientConn.Write(msg)
	if err != nil || n != len(msg) {
		t.Fatalf("client write error: %v, n=%d", err, n)
	}

	select {
	case received := <-serverReceived:
		if !bytes.Equal(received, msg) {
			t.Fatalf("expected server to receive %q, got %q", msg, received)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server to receive payload")
	}

	replyBuf := make([]byte, 100)
	rn, err := clientConn.Read(replyBuf)
	if err != nil {
		t.Fatalf("client read error: %v", err)
	}
	if string(replyBuf[:rn]) != "pong-response" {
		t.Fatalf("expected 'pong-response', got %q", string(replyBuf[:rn]))
	}
}
