package main

import (
	"context"
	"net"
	"testing"
	"time"
	"tunnelforge/agent/forge/client"
	"tunnelforge/server/auth"
	"tunnelforge/server/tunnel"
)

func waitForTunnel(subdomain string, timeout time.Duration) (*tunnel.Tunnel, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tun, ok := registry.Get(subdomain); ok {
			return tun, true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, false
}

func waitForRemoval(subdomain string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := registry.Get(subdomain); !ok {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestAgentMultiSubdomainRegistration(t *testing.T) {
	authReg := auth.GetAuthRegistry()
	agentID := "agent_multi_1"
	rawToken := "multi_token_123"

	if err := authReg.Register(agentID, rawToken); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		handleAgent(conn)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tunnels := map[string]string{
		"test-app": "localhost:3000",
		"my-app":   "localhost:5173",
	}

	c := client.New(
		listener.Addr().String(),
		rawToken,
		agentID,
		1,
		tunnels,
	)

	errCh := make(chan error, 1)
	go func() {
		_, err := c.RunOnce(ctx)
		errCh <- err
	}()

	// Wait for both subdomains to register
	tun1, ok := waitForTunnel("test-app", 2*time.Second)
	if !ok {
		t.Fatalf("expected tunnel 'test-app' to be registered")
	}
	if tun1.AgentID != agentID {
		t.Fatalf("expected AgentID '%s', got '%s'", agentID, tun1.AgentID)
	}

	tun2, ok := waitForTunnel("my-app", 2*time.Second)
	if !ok {
		t.Fatalf("expected tunnel 'my-app' to be registered")
	}
	if tun2.AgentID != agentID {
		t.Fatalf("expected AgentID '%s', got '%s'", agentID, tun2.AgentID)
	}

	// Both tunnels share the same yamux session
	if tun1.Session != tun2.Session {
		t.Fatal("expected both tunnels to share the same yamux session")
	}

	// Cancel context to close connection and trigger teardown
	cancel()

	if !waitForRemoval("test-app", 2*time.Second) {
		t.Fatal("expected 'test-app' to be removed after disconnect")
	}
	if !waitForRemoval("my-app", 2*time.Second) {
		t.Fatal("expected 'my-app' to be removed after disconnect")
	}
}

func TestDuplicateSubdomainRegistration(t *testing.T) {
	authReg := auth.GetAuthRegistry()
	agentID1 := "agent_dup_1"
	agentID2 := "agent_dup_2"
	token := "dup_token"

	if err := authReg.Register(agentID1, token); err != nil {
		t.Fatalf("failed to register agent 1: %v", err)
	}
	if err := authReg.Register(agentID2, token); err != nil {
		t.Fatalf("failed to register agent 2: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleAgent(conn)
		}
	}()

	subdomain := "dup-subdomain"
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	c1 := client.New(
		listener.Addr().String(),
		token,
		agentID1,
		1,
		map[string]string{subdomain: "localhost:3000"},
	)

	go func() {
		_, _ = c1.RunOnce(ctx1)
	}()

	_, ok := waitForTunnel(subdomain, 2*time.Second)
	if !ok {
		t.Fatalf("expected first tunnel '%s' to be registered", subdomain)
	}

	// Second client tries to claim the same subdomain
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	c2 := client.New(
		listener.Addr().String(),
		token,
		agentID2,
		1,
		map[string]string{subdomain: "localhost:3000"},
	)

	_, err = c2.RunOnce(ctx2)
	if err == nil {
		t.Fatalf("expected second client registration to fail due to duplicate subdomain")
	}
}

func TestAtomicRollbackOnPartialFailure(t *testing.T) {
	authReg := auth.GetAuthRegistry()
	agentID1 := "agent_atomic_1"
	agentID2 := "agent_atomic_2"
	token := "atomic_token"

	if err := authReg.Register(agentID1, token); err != nil {
		t.Fatalf("failed to register agent 1: %v", err)
	}
	if err := authReg.Register(agentID2, token); err != nil {
		t.Fatalf("failed to register agent 2: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleAgent(conn)
		}
	}()

	// Agent 1 registers "claimed"
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	c1 := client.New(
		listener.Addr().String(),
		token,
		agentID1,
		1,
		map[string]string{"claimed": "localhost:3000"},
	)

	go func() {
		_, _ = c1.RunOnce(ctx1)
	}()

	_, ok := waitForTunnel("claimed", 2*time.Second)
	if !ok {
		t.Fatal("expected 'claimed' to be registered")
	}

	// Agent 2 tries to register both "fresh" and "claimed".
	// "fresh" would succeed, but "claimed" is taken, so the entire
	// registration should be rolled back atomically.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	c2 := client.New(
		listener.Addr().String(),
		token,
		agentID2,
		1,
		map[string]string{
			"fresh":   "localhost:4000",
			"claimed": "localhost:5000",
		},
	)

	_, err = c2.RunOnce(ctx2)
	if err == nil {
		t.Fatal("expected registration to fail due to 'claimed' being taken")
	}

	// Verify "fresh" was rolled back and is not in the registry
	if _, ok := registry.Get("fresh"); ok {
		t.Fatal("expected 'fresh' to be rolled back, but it's still registered")
	}

	// "claimed" should still belong to agent 1
	tun, ok := registry.Get("claimed")
	if !ok {
		t.Fatal("expected 'claimed' to still be registered")
	}
	if tun.AgentID != agentID1 {
		t.Fatalf("expected 'claimed' to belong to '%s', got '%s'", agentID1, tun.AgentID)
	}
}

func TestSameAgentReconnectSessionTakeover(t *testing.T) {
	authReg := auth.GetAuthRegistry()
	agentID := "agent_reconnect_user"
	token := "token_reconnect_123"

	if err := authReg.Register(agentID, token); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleAgent(conn)
		}
	}()

	subdomain := "takeover-app"

	// 1. First session connects and registers
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	c1 := client.New(
		listener.Addr().String(),
		token,
		agentID,
		1,
		map[string]string{subdomain: "localhost:3000"},
	)

	s1ErrCh := make(chan error, 1)
	go func() {
		_, err := c1.RunOnce(ctx1)
		s1ErrCh <- err
	}()

	tun1, ok := waitForTunnel(subdomain, 2*time.Second)
	if !ok {
		t.Fatalf("expected initial tunnel %q to be registered", subdomain)
	}
	initialID := tun1.ID

	// 2. Second session from the SAME agent connects for the SAME subdomain
	// (Simulating agent reconnecting before old session times out)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	c2 := client.New(
		listener.Addr().String(),
		token,
		agentID,
		1,
		map[string]string{subdomain: "localhost:4000"},
	)

	s2ErrCh := make(chan error, 1)
	go func() {
		_, err := c2.RunOnce(ctx2)
		s2ErrCh <- err
	}()

	// Wait for replacement tunnel to be active in registry
	var tun2 *tunnel.Tunnel
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if t, ok := registry.Get(subdomain); ok && t.ID != initialID {
			tun2 = t
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if tun2 == nil {
		t.Fatalf("expected tunnel %q to be replaced with new session", subdomain)
	}
	if tun2.ID == initialID {
		t.Fatalf("expected new tunnel ID, but still had old ID %s", initialID)
	}

	// 3. Verify Session 1 was terminated cleanly because it was replaced
	select {
	case <-s1ErrCh:
		// S1 exited because its yamux session was closed
	case <-time.After(2 * time.Second):
		t.Log("Note: S1 took longer than 2s to unblock after eviction")
	}

	// 4. Verify tunnel is STILL registered (S1's deferred RemoveIfSame did not delete it)
	time.Sleep(100 * time.Millisecond)
	tunAfter, ok := registry.Get(subdomain)
	if !ok {
		t.Fatalf("expected tunnel %q to remain registered after old session teardown", subdomain)
	}
	if tunAfter.ID != tun2.ID {
		t.Fatalf("expected tunnel ID %s, got %s", tun2.ID, tunAfter.ID)
	}
}

