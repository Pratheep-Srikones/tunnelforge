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
	fmt.Println("Agent authenticated:", req.AgentID)

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

	// Accept initial stream as CONTROL stream
	controlStream, err := session.Accept()
	if err != nil {
		fmt.Println("Failed to accept control stream:", err)
		return
	}

	registerReq, err := receiveTunnelRegister(controlStream)
	if err != nil {
		fmt.Println("Tunnel registration failed:", err)
		_ = sendTunnelRegisterResponse(controlStream, false, nil, err.Error())
		return
	}

	// Register all requested subdomains atomically.
	tunnels, results, ok := registerSubdomains(req.AgentID, registerReq.Subdomains, session)

	if err := sendTunnelRegisterResponse(controlStream, ok, results, ""); err != nil {
		fmt.Println("Failed to send tunnel register response:", err)
		rollbackTunnels(tunnels)
		return
	}

	if !ok {
		fmt.Println("Tunnel registration partially failed, rolled back")
		rollbackTunnels(tunnels)
		return
	}

	for _, t := range tunnels {
		fmt.Printf("Registered tunnel '%s' for agent '%s'\n", t.Subdomain, t.AgentID)
	}

	defer func() {
		for _, t := range tunnels {
			registry.RemoveIfSame(t.Subdomain, t.ID)
			fmt.Println("Tunnel removed:", t.Subdomain)
		}
	}()

	acceptStreams(session, req.AgentID)
}

// registerSubdomains attempts to register all subdomains atomically.
// If any registration fails, all previously registered tunnels are
// rolled back and the function returns ok=false.
func registerSubdomains(
	agentID string,
	subdomains []string,
	session *yamux.Session,
) ([]*tunnel.Tunnel, map[string]proto.SubdomainResult, bool) {
	results := make(map[string]proto.SubdomainResult, len(subdomains))
	tunnels := make([]*tunnel.Tunnel, 0, len(subdomains))
	allOK := true

	for _, sub := range subdomains {
		t := &tunnel.Tunnel{
			ID: fmt.Sprintf(
				"%s-%d",
				sub,
				time.Now().UnixNano(),
			),
			AgentID:   agentID,
			Subdomain: sub,
			Session:   session,
			CreatedAt: time.Now(),
		}

		if err := registry.Register(t); err != nil {
			results[sub] = proto.SubdomainResult{
				OK:      false,
				Message: err.Error(),
			}
			allOK = false
			// Roll back everything registered so far
			rollbackTunnels(tunnels)
			tunnels = nil
			// Fill remaining results as failed
			for _, remaining := range subdomains {
				if _, exists := results[remaining]; !exists {
					results[remaining] = proto.SubdomainResult{
						OK:      false,
						Message: "rolled back due to sibling failure",
					}
				}
			}
			return tunnels, results, false
		}

		results[sub] = proto.SubdomainResult{
			OK:      true,
			Message: "registered",
		}
		tunnels = append(tunnels, t)
	}

	return tunnels, results, allOK
}

// rollbackTunnels removes all successfully registered tunnels.
func rollbackTunnels(tunnels []*tunnel.Tunnel) {
	for _, t := range tunnels {
		registry.RemoveIfSame(t.Subdomain, t.ID)
	}
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

func receiveTunnelRegister(stream net.Conn) (proto.TunnelRegisterRequest, error) {
	var req proto.TunnelRegisterRequest

	if err := stream.SetReadDeadline(time.Now().Add(HandshakeTimeout)); err != nil {
		return req, fmt.Errorf("failed to set control stream deadline: %w", err)
	}

	decoder := json.NewDecoder(stream)
	if err := decoder.Decode(&req); err != nil {
		return req, fmt.Errorf("error decoding tunnel register request: %w", err)
	}

	if err := req.Validate(); err != nil {
		return req, fmt.Errorf("tunnel register validation failed: %w", err)
	}

	if err := stream.SetReadDeadline(time.Time{}); err != nil {
		return req, fmt.Errorf("failed to clear control stream deadline: %w", err)
	}

	return req, nil
}

func sendTunnelRegisterResponse(
	stream net.Conn,
	ok bool,
	results map[string]proto.SubdomainResult,
	msg string,
) error {
	response := proto.TunnelRegisterResponse{
		Type:    "tunnel_register_ack",
		OK:      ok,
		Results: results,
		Message: msg,
	}

	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(response); err != nil {
		return fmt.Errorf("error encoding tunnel register response: %w", err)
	}

	return nil
}

func acceptStreams(session *yamux.Session, agentID string) {
	for {
		stream, err := session.Accept()
		if err != nil {
			fmt.Println(
				"Yamux session closed for agent:",
				agentID,
				err,
			)
			return
		}

		fmt.Println("New yamux stream from agent:", agentID)

		go handleStream(stream)
	}
}

func handleStream(stream net.Conn) {
	defer stream.Close()

	fmt.Println("Stream opened")

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
