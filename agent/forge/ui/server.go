package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
	"tunnelforge/agent/forge/capture"
	tunnel "tunnelforge/agent/forge/config"
)

//go:embed static/*
var staticFS embed.FS

// Server represents the local developer web server serving the dashboard and API.
type Server struct {
	Port       string
	Hub        *Hub
	Capturer   capture.Capturer
	Tunnels    map[string]tunnel.TunnelEntry
	mu         sync.RWMutex
	httpServer *http.Server
	listener   net.Listener
}

func NewServer(port string, capturer capture.Capturer, tunnels map[string]tunnel.TunnelEntry) *Server {
	if port == "" {
		port = "4040"
	}
	hub := NewHub()
	return &Server{
		Port:     port,
		Hub:      hub,
		Capturer: capturer,
		Tunnels:  tunnels,
	}
}

// Handler returns the configured HTTP serve mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Embedded static dashboard
	staticContent, err := fs.Sub(staticFS, "static")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(staticContent)))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "embedded UI assets not found", http.StatusNotFound)
		})
	}

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// WebSocket endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.Hub, w, r)
	})

	// API: List active tunnels
	mux.HandleFunc("/api/tunnels", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		defer s.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Tunnels)
	})

	// API: List captured requests
	mux.HandleFunc("/api/requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			subdomain := r.URL.Query().Get("subdomain")
			if subdomain != "" && s.Capturer != nil {
				_ = s.Capturer.Clear(subdomain)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		subdomain := r.URL.Query().Get("subdomain")
		limit := 50
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}

		w.Header().Set("Content-Type", "application/json")

		if s.Capturer == nil {
			_ = json.NewEncoder(w).Encode([]*capture.RequestEntry{})
			return
		}

		if subdomain != "" {
			entries, _ := s.Capturer.List(subdomain, limit)
			if entries == nil {
				entries = []*capture.RequestEntry{}
			}
			_ = json.NewEncoder(w).Encode(entries)
			return
		}

		// If no subdomain specified, aggregate from all tunnels
		s.mu.RLock()
		allEntries := make([]*capture.RequestEntry, 0)
		for sub := range s.Tunnels {
			list, _ := s.Capturer.List(sub, limit)
			allEntries = append(allEntries, list...)
		}
		s.mu.RUnlock()

		// Sort newest first
		sort.Slice(allEntries, func(i, j int) bool {
			return allEntries[i].Timestamp.After(allEntries[j].Timestamp)
		})

		if len(allEntries) > limit {
			allEntries = allEntries[:limit]
		}

		_ = json.NewEncoder(w).Encode(allEntries)
	})

	return mux
}

// Start launches the Hub and begins listening on the configured port.
func (s *Server) Start() error {
	go s.Hub.Run()

	ln, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		return fmt.Errorf("starting UI listener on port %s: %w", s.Port, err)
	}
	s.listener = ln

	s.httpServer = &http.Server{
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("[UI] Dashboard running at http://localhost:%s\n", s.Port)
	return s.httpServer.Serve(ln)
}

// Stop cleanly terminates the HTTP server and WebSocket hub.
func (s *Server) Stop(ctx context.Context) error {
	s.Hub.Stop()
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}
