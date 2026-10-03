package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"tunnelforge/agent/forge/capture"
	tunnel "tunnelforge/agent/forge/config"
	"tunnelforge/internal/certs"
	"tunnelforge/internal/config"
	"tunnelforge/internal/proto"
	"tunnelforge/internal/transport"

	"github.com/gorilla/websocket"
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
	Tunnels     map[string]tunnel.TunnelEntry
	Capturer    capture.Capturer
	Broadcaster Broadcaster

	// TLS configuration
	CACertPath      string
	TLSConfig       *tls.Config
	InsecureSkipTLS bool

	// Transport configuration & state
	ForceWS            bool
	preferredTransport string
}

// Broadcaster is an interface for sending live event notifications to connected dashboards.
type Broadcaster interface {
	BroadcastJSON(v any) error
}

func New(serverAddr, token, agentID string, maxRetryCount int, tunnels map[string]tunnel.TunnelEntry, capturer capture.Capturer) *Client {
	return &Client{
		ServerAddr:    serverAddr,
		Token:         token,
		AgentID:       agentID,
		MaxRetryCount: maxRetryCount,
		Tunnels:       tunnels,
		Capturer:      capturer,
	}
}

// WithBroadcaster attaches a live event broadcaster (e.g. WebSocket Hub) to the client.
func (c *Client) WithBroadcaster(b Broadcaster) *Client {
	c.Broadcaster = b
	return c
}

// WithCACert sets a custom CA certificate file path to verify the server.
// If left empty, the embedded root CA certificate is used.
func (c *Client) WithCACert(caPath string) *Client {
	c.CACertPath = caPath
	return c
}

// WithTLSConfig sets an explicit *tls.Config for the agent client.
func (c *Client) WithTLSConfig(cfg *tls.Config) *Client {
	c.TLSConfig = cfg
	return c
}

// WithInsecureSkipTLS disables TLS encryption (using plain TCP). Used mainly for tests.
func (c *Client) WithInsecureSkipTLS(skip bool) *Client {
	c.InsecureSkipTLS = skip
	return c
}

// WithWebSocket forces using the WebSocket transport (/tunnel) bypassing port 7000.
func (c *Client) WithWebSocket(force bool) *Client {
	c.ForceWS = force
	return c
}

// ToTunnelEntries converts a simple subdomain -> local address map into a map of TunnelEntry structs.
func ToTunnelEntries(m map[string]string) map[string]tunnel.TunnelEntry {
	res := make(map[string]tunnel.TunnelEntry, len(m))
	for k, v := range m {
		res[k] = tunnel.TunnelEntry{Local: v}
	}
	return res
}

// NewSimple creates a Client with a basic map of subdomains to local addresses and no capture.
func NewSimple(serverAddr, token, agentID string, maxRetryCount int, tunnels map[string]string) *Client {
	return New(serverAddr, token, agentID, maxRetryCount, ToTunnelEntries(tunnels), nil)
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

func NormalizeTCPAddr(rawAddr string) string {
	return ResolveServerEndpoints(rawAddr).TCPAddr
}

func (c *Client) connect(ctx context.Context) (net.Conn, error) {
	/*
		connect to server using TLS connection (or plain TCP if InsecureSkipTLS is set),
		with automatic WebSocket fallback when port 7000 is blocked by a firewall.
	*/
	endpoints := ResolveServerEndpoints(c.ServerAddr)

	var tlsCfg *tls.Config
	if !c.InsecureSkipTLS {
		if c.TLSConfig != nil {
			tlsCfg = c.TLSConfig.Clone()
			if tlsCfg.ServerName == "" {
				host, _, err := net.SplitHostPort(endpoints.TCPAddr)
				if err != nil {
					host = endpoints.TCPAddr
				}
				tlsCfg.ServerName = host
			}
		} else {
			var err error
			tlsCfg, err = certs.LoadClientTLSConfig(endpoints.TCPAddr, c.CACertPath)
			if err != nil {
				return nil, fmt.Errorf("loading TLS configuration: %w", err)
			}
		}
	}

	// 1. If WebSocket transport is forced or already selected for this session:
	if c.ForceWS || c.preferredTransport == "ws" {
		return c.connectWS(ctx, endpoints, tlsCfg)
	}

	// 2. Attempt primary connection on port :7000 (TLS or plain TCP)
	fmt.Println("[Connect] Connecting to:", endpoints.TCPAddr)
	var conn net.Conn
	var err error

	dialer := net.Dialer{
		Timeout: 3 * time.Second,
	}

	if c.InsecureSkipTLS {
		conn, err = dialer.DialContext(ctx, "tcp", endpoints.TCPAddr)
	} else {
		conn, err = tls.DialWithDialer(&dialer, "tcp", endpoints.TCPAddr, tlsCfg)
	}

	if err == nil {
		if c.InsecureSkipTLS {
			c.preferredTransport = "tcp"
			fmt.Println("[Connect] Connected to server (plain TCP)")
		} else {
			c.preferredTransport = "tls"
			fmt.Println("[Connect] Connected to server with TLS")
		}
		return conn, nil
	}

	// 3. Primary connection on :7000 failed
	fmt.Printf("[Connect] Primary connection to %s failed (%v)\n", endpoints.TCPAddr, err)
	fmt.Printf("[Connect] Probing server health at %s...\n", endpoints.RESTURL)

	// 4. Probe server health on web port (:443 or :8000)
	healthTLS := tlsCfg
	if c.InsecureSkipTLS {
		healthTLS = nil
	}

	if checkServerHealthEndpoint(ctx, endpoints.RESTURL, endpoints.IsTLS && !c.InsecureSkipTLS, healthTLS) {
		fmt.Println("[Connect] Server is ALIVE on web port! (Port 7000 is blocked by firewall)")
		fmt.Println("[Connect] Switching to WebSocket fallback transport...")
		c.preferredTransport = "ws" // Latch onto WebSocket for remainder of session
		return c.connectWS(ctx, endpoints, tlsCfg)
	}

	// Both failed -> server is truly down or client has no internet connection
	return nil, fmt.Errorf("server unreachable (primary on %s: %w)", endpoints.TCPAddr, err)
}

func (c *Client) connectWS(ctx context.Context, endpoints ServerEndpoints, tlsCfg *tls.Config) (net.Conn, error) {
	fmt.Printf("[Connect] Connecting via WebSocket: %s\n", endpoints.WSURL)

	wsDialer := websocket.Dialer{
		HandshakeTimeout: DialTimeout,
	}

	if endpoints.IsTLS && !c.InsecureSkipTLS {
		if tlsCfg != nil {
			wsDialer.TLSClientConfig = tlsCfg.Clone()
		} else {
			cfg, err := certs.LoadClientTLSConfig(endpoints.TCPAddr, c.CACertPath)
			if err != nil {
				return nil, fmt.Errorf("loading TLS configuration for WebSocket: %w", err)
			}
			wsDialer.TLSClientConfig = cfg
		}
	}

	ws, resp, err := wsDialer.DialContext(ctx, endpoints.WSURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("websocket dial to %s: %w", endpoints.WSURL, err)
	}

	fmt.Println("[Connect] Connected to server via WebSocket")
	return transport.NewWSConn(ws), nil
}

func checkServerHealthEndpoint(ctx context.Context, restURL string, isTLS bool, tlsCfg *tls.Config) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var tr *http.Transport
	if isTLS && tlsCfg != nil {
		tr = &http.Transport{
			TLSClientConfig: tlsCfg.Clone(),
		}
	} else {
		tr = &http.Transport{}
	}

	httpClient := &http.Client{
		Timeout:   3 * time.Second,
		Transport: tr,
	}

	healthURL := strings.TrimRight(restURL, "/") + "/forge/internal/health"
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
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
		fmt.Printf("Tunnel registered: %s → %s\n", sub, c.Tunnels[sub].Local)
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
