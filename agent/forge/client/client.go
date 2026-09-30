package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
	"tunnelforge/internal/config"
	"tunnelforge/internal/proto"

	"github.com/hashicorp/yamux"
)

const (
	HandshakeTimeout = 10 * time.Second
	DialTimeout      = 10 * time.Second

	InitialBackoff = 1 * time.Second
	MaxBackoff     = 60 * time.Second
)

// Client manages a single agent connection to the TunnelForge server.
// It supports multiple subdomains, each mapped to a different local address.
type Client struct {
	ServerAddr    string
	Token         string
	AgentID       string
	MaxRetryCount int

	// Tunnels maps subdomain → local address.
	// e.g. {"test-app": "localhost:3000", "my-app": "localhost:5173"}
	Tunnels map[string]string
}

func New(serverAddr, token, agentID string, maxRetryCount int, tunnels map[string]string) *Client {
	return &Client{
		ServerAddr:    serverAddr,
		Token:         token,
		AgentID:       agentID,
		MaxRetryCount: maxRetryCount,
		Tunnels:       tunnels,
	}
}

func (c *Client) Run(ctx context.Context) error {
	backoff := InitialBackoff
	retryCount := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if c.MaxRetryCount > 0 && retryCount >= c.MaxRetryCount {
			return fmt.Errorf(
				"max retry count reached: %d",
				c.MaxRetryCount,
			)
		}

		fmt.Println("[Main] Connecting to TunnelForge...")

		established, err := c.RunOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			fmt.Println("[Main] Tunnel disconnected:", err)
		}

		// The tunnel was successfully established.
		// Reset retry state because this is a new outage.
		if established {
			retryCount = 0
			backoff = InitialBackoff
		} else {
			retryCount++
		}

		fmt.Printf(
			"Reconnecting in %s...\n",
			backoff,
		)

		if err := sleepContext(ctx, backoff); err != nil {
			return err
		}
		backoff *= 2

		if backoff > MaxBackoff {
			backoff = MaxBackoff
		}
	}
}

func (c *Client) RunOnce(ctx context.Context) (bool, error) {
	conn, err := c.connect(ctx)
	if err != nil {
		return false, err
	}

	defer conn.Close()

	if err := c.handshake(ctx, conn); err != nil {
		return false, err
	}

	session, err := c.createSession(conn)
	if err != nil {
		return false, err
	}

	defer session.Close()

	controlStream, err := c.registerTunnels(ctx, session)
	if err != nil {
		return false, err
	}

	defer controlStream.Close()

	fmt.Println("[Tunnel] Tunnel is running")

	err = c.acceptStreams(ctx, session)

	return true, err
}

func (c *Client) connect(ctx context.Context) (net.Conn, error) {
	/*
		connect to server by using direct tcp connection
	*/
	fmt.Println("[Connect] Connecting to:", c.ServerAddr)

	dialer := net.Dialer{
		Timeout: DialTimeout,
	}

	conn, err := dialer.DialContext(ctx,
		"tcp",
		c.ServerAddr,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"connect to %s: %w",
			c.ServerAddr,
			err,
		)
	}

	fmt.Println("[Connect] Connected to server")

	return conn, nil
}

func (c *Client) handshake(ctx context.Context, conn net.Conn) error {
	/*
			handshake with server
		1. send handshake request
		2. receive handshake response
		3. check if handshake is successful
	*/
	if err := ctx.Err(); err != nil {
		return err
	}
	req := proto.HandshakeRequest{
		Type:    "handshake",
		Token:   c.Token,
		AgentID: c.AgentID,
	}

	if err := conn.SetWriteDeadline(
		time.Now().Add(HandshakeTimeout),
	); err != nil {
		return fmt.Errorf("setting deadline: %w", err)
	}
	encoder := json.NewEncoder(conn)

	if err := encoder.Encode(req); err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	if err := conn.SetReadDeadline(
		time.Now().Add(HandshakeTimeout),
	); err != nil {
		return fmt.Errorf("setting deadline: %w", err)
	}

	var response proto.HandshakeResponse

	decoder := json.NewDecoder(conn)

	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("reading handshake response: %w", err)
	}

	if !response.OK {
		return fmt.Errorf(
			"server rejected handshake: %s",
			response.Message,
		)
	}

	// Remove handshake deadlines.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return fmt.Errorf(
			"clearing connection deadline: %w",
			err,
		)
	}

	fmt.Println("[HandShake] Server:", response.Message)

	return nil
}

// registerTunnels opens the control stream and registers all subdomains
// with the server in a single request.
func (c *Client) registerTunnels(
	ctx context.Context,
	session *yamux.Session,
) (net.Conn, error) {
	/*
		register tunnels with the server
		1. open a control stream
		2. send tunnel registration request
		3. receive tunnel registration response
		4. check if tunnel registration is successful
	*/
	controlStream, err := session.Open()
	if err != nil {
		return nil, fmt.Errorf("opening control stream: %w", err)
	}

	subdomains := make([]string, 0, len(c.Tunnels))
	for sub := range c.Tunnels {
		subdomains = append(subdomains, sub)
	}

	req := proto.TunnelRegisterRequest{
		Type:       "tunnel_register",
		Subdomains: subdomains,
	}

	if err := controlStream.SetWriteDeadline(time.Now().Add(HandshakeTimeout)); err != nil {
		controlStream.Close()
		return nil, fmt.Errorf("setting control stream write deadline: %w", err)
	}

	if err := json.NewEncoder(controlStream).Encode(req); err != nil {
		controlStream.Close()
		return nil, fmt.Errorf("encoding tunnel registration request: %w", err)
	}

	if err := controlStream.SetReadDeadline(time.Now().Add(HandshakeTimeout)); err != nil {
		controlStream.Close()
		return nil, fmt.Errorf("setting control stream read deadline: %w", err)
	}

	var resp proto.TunnelRegisterResponse
	if err := json.NewDecoder(controlStream).Decode(&resp); err != nil {
		controlStream.Close()
		return nil, fmt.Errorf("reading tunnel registration response: %w", err)
	}

	if !resp.OK {
		// Print per-subdomain results for diagnostics
		for sub, result := range resp.Results {
			if !result.OK {
				fmt.Printf("[Tunnel Register] %s: %s\n", sub, result.Message)
			}
		}
		controlStream.Close()
		return nil, fmt.Errorf("server rejected tunnel registration: %s", resp.Message)
	}

	if err := controlStream.SetDeadline(time.Time{}); err != nil {
		controlStream.Close()
		return nil, fmt.Errorf("clearing control stream deadline: %w", err)
	}

	for sub := range c.Tunnels {
		fmt.Printf("Tunnel registered: %s → %s\n", sub, c.Tunnels[sub])
	}

	return controlStream, nil
}

func (c *Client) createSession(conn net.Conn) (*yamux.Session, error) {
	/*
		create a yamux session based on the tcp connection
	*/
	session, err := yamux.Client(conn, config.YamuxConfig())
	if err != nil {
		return nil, fmt.Errorf(
			"creating yamux session: %w",
			err,
		)
	}

	fmt.Println("[Yamux] session established")

	return session, nil
}

func (c *Client) acceptStreams(
	ctx context.Context,
	session *yamux.Session,
) error {
	/*
		accept streams from the server
		1. open a channel to receive streams
		2. accept streams from the server
		3. handle streams
	*/

	done := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			session.Close()

		case <-done:
		}
	}()

	defer close(done)

	for {
		stream, err := session.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf(
				"yamux session closed: %w",
				err,
			)
		}

		fmt.Println("[Stream] Incoming tunnel stream")

		// spawn a new goroutine to handle the stream
		go c.handleStream(stream)
	}
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	/*
		sleep for a given duration
		if the context is cancelled, return the context error
	*/
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}
