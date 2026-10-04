// ==============================================================================
// TunnelForge Load Test — Echo Server
// ==============================================================================
// A minimal HTTP echo server to run LOCALLY behind the TunnelForge agent.
// It responds with the request path + method + body so you can verify
// end-to-end correctness during load testing.
//
// Usage:
//   go run loadtest/echo_server.go              (default port 8080)
//   PORT=9090 go run loadtest/echo_server.go
//
// Then register a TunnelForge agent targeting this port:
//   tunnelforge-agent --local-port 8080 --subdomain myagent
// ==============================================================================

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type EchoResponse struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	Timestamp string            `json:"timestamp"`
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024)) // max 64KB
	defer r.Body.Close()

	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resp := EchoResponse{
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   headers,
		Body:      string(body),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Echo-Server", "tunnelforge-loadtest")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/", echoHandler)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("[echo-server] Listening on %s", addr)
	log.Printf("[echo-server] Point your TunnelForge agent at --local-port %s", port)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("echo server error: %v", err)
	}
}
