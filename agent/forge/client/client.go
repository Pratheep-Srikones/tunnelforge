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

type Client struct {
	ServerAddr string
	Token string
	Subdomain string
	LocalAddr string
	MaxRetryCount int
}

func New(serverAddr, token, subDomain, localAddr string, maxRetryCount int) *Client {
	return &Client{
		ServerAddr: serverAddr,
		Token: token,
		Subdomain: subDomain,
		LocalAddr: localAddr,
		MaxRetryCount: maxRetryCount,
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

		fmt.Println("Connecting to TunnelForge...")

		established, err := c.RunOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			fmt.Println("Tunnel disconnected:", err)
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

	fmt.Println("Tunnel is running")

	err = c.acceptStreams(ctx, session)

	return true, err
}

func (c *Client) connect(ctx context.Context) (net.Conn, error) {
	fmt.Println("Connecting to:", c.ServerAddr)

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

	fmt.Println("Connected to server")

	return conn, nil
}

func (c *Client) handshake(ctx context.Context, conn net.Conn) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	req := proto.HandshakeRequest{
		Type:      "handshake",
		Token:     c.Token,
		Subdomain: c.Subdomain,
	}
	if err := conn.SetWriteDeadline(
		time.Now().Add(HandshakeTimeout),
	); err != nil {
		return err
	}
	encoder := json.NewEncoder(conn)

	if err := encoder.Encode(req); err != nil {
		return fmt.Errorf("sending handshake: %w", err)
	}

	if err := conn.SetReadDeadline(
		time.Now().Add(HandshakeTimeout),
	); err != nil {
		return err
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

	fmt.Println("Server:", response.Message)

	return nil
}

func (c *Client) createSession(conn net.Conn) (*yamux.Session, error) {
	session, err := yamux.Client(conn, config.YamuxConfig())
	if err != nil {
		return nil, fmt.Errorf(
			"creating yamux session: %w",
			err,
		)
	}

	fmt.Println("Yamux session established")

	return session, nil
}

func (c *Client) acceptStreams(
	ctx context.Context,
	session *yamux.Session,
) error {
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

		fmt.Println("Incoming tunnel stream")

		go c.handleStream(stream)
	}
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}