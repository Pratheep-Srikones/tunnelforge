package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
	"tunnelforge/internal/config"
	"tunnelforge/internal/proto"
	"tunnelforge/server/auth"
	"tunnelforge/server/tunnel"

	"github.com/hashicorp/yamux"
)

const HandshakeTimeout = 10 * time.Second

func handleAgent(conn net.Conn) {
	defer conn.Close()

	if err := conn.SetReadDeadline(
		time.Now().Add(HandshakeTimeout),
	); err != nil {
		fmt.Println("Failed to set handshake deadline:", err)
		return
	}

	req, err := receiveHandshake(conn)
	if err != nil {
		fmt.Println("Handshake failed:", err)
		return
	}
	fmt.Println("Agent connected")
	fmt.Println("  Subdomain:", req.Subdomain)

	if _, exists := registry.Get(req.Subdomain); exists {
		fmt.Println("Agent already registered:", req.Subdomain)
		_ = sendHandshakeErrorResponse(conn, fmt.Sprintf("subdomain '%s' is already registered", req.Subdomain))
		return
	}

	if err := sendHandshakeResponse(conn); err != nil {
		fmt.Println("Failed to send handshake response:", err)
		return
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		fmt.Println("Failed to clear connection deadline:", err)
		return
	}

	session, err := yamux.Server(conn, config.YamuxConfig())
	if err != nil {
		fmt.Println("Failed to create yamux server:", err)
		return
	}

	defer session.Close()

	t := &tunnel.Tunnel{
		ID: fmt.Sprintf(
			"%s-%d",
			req.Subdomain,
			time.Now().UnixNano(),
		),
		Subdomain: req.Subdomain,
		Session:   session,
		CreatedAt: time.Now(),
	}

	if !registry.Register(t) {
		fmt.Println("Agent registration race detected for:", req.Subdomain)
		return
	}

	fmt.Println("Registered tunnel:", req.Subdomain)

	defer func() {
		registry.Remove(t)
		fmt.Println("Tunnel removed:", req.Subdomain)
	}()

	acceptStreams(session, req.Subdomain)
}

func receiveHandshake(conn net.Conn) (proto.HandshakeRequest, error) {
	var req proto.HandshakeRequest

	decoder := json.NewDecoder(conn)

	if err := decoder.Decode(&req); err != nil {
		return req, fmt.Errorf("error decoding handshake: %w", err)
	}

	if err := req.Validate(); err != nil {
		return req, fmt.Errorf("handshake validation failed: %w", err)
	}

	valid, err := auth.ValidateToken(req.AgentID, req.Token)
	if err != nil {
		return req, fmt.Errorf("token validation failed: %w", err)
	}

	if !valid {
		return req, fmt.Errorf("invalid token")
	}

	return req, nil
}

func sendHandshakeResponse(conn net.Conn) error {
	response := proto.HandshakeResponse{
		Type:    "handshake_ack",
		OK:      true,
		Message: "server accepted your handshake",
	}

	encoder := json.NewEncoder(conn)

	if err := encoder.Encode(response); err != nil {
		return fmt.Errorf("error encoding handshake response: %w", err)
	}

	return nil
}

func sendHandshakeErrorResponse(conn net.Conn, msg string) error {
	response := proto.HandshakeResponse{
		Type:    "handshake_ack",
		OK:      false,
		Message: msg,
	}

	encoder := json.NewEncoder(conn)

	if err := encoder.Encode(response); err != nil {
		return fmt.Errorf("error encoding handshake response: %w", err)
	}

	return nil
}

func acceptStreams(session *yamux.Session, subdomain string) {
	for {
		stream, err := session.Accept()
		if err != nil {
			fmt.Println(
				"Yamux session closed:",
				subdomain,
				err,
			)
			return
		}

		fmt.Println("New yamux stream from:", subdomain)

		go handleStream(stream)
	}
}

func handleStream(stream net.Conn) {
	defer stream.Close()

	fmt.Println("Stream opened")

	// For now, just read whatever the agent sends.
	buf := make([]byte, 1024)

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Stream closed")
			} else {
				fmt.Println("Stream error:", err)
			}
			return
		}

		fmt.Println("Stream received:", string(buf[:n]))
	}
}
