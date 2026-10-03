# TunnelForge

> **Self-hosted, secure reverse tunnel platform for developers.**  
> Expose local development services to the internet through your own VPS with persistent TLS multiplexing, corporate firewall bypass, live traffic inspection, and one-click request replay.

---

## 🚀 Key Features

* **Zero-Config Agent TLS**: Embedded Root CA (`//go:embed ca.crt`) allows agents to verify server certificates out-of-the-box with no certificate installation required.
* **Corporate Firewall Bypass**: Automatically probes the web port (`:443`) when raw TLS (`:7000`) is blocked, seamlessly falling back to WebSocket multiplexing (`wss://`).
* **Live Traffic Inspector & Dashboard**: Built-in debug web UI served at `http://localhost:4040` streaming live HTTP requests and responses via WebSockets.
* **One-Click Request Replay**: Re-fire captured HTTP requests directly to your local backend from the web UI.
* **Multi-Tunnel & Multi-Protocol**: Expose multiple HTTP subdomains or raw TCP ports simultaneously over a single persistent connection.
* **Declarative Configuration**: Define all your tunnels in a simple `tunnels.yaml` file.
* **Single Binary Agent**: No external dependencies; cross-compiles cleanly to macOS (M-series/Intel), Linux (ARM/AMD), and Windows.

---

## 📊 Feature Comparison

| Feature | ngrok Free | TunnelForge V1 |
| :--- | :--- | :--- |
| **Session Limit** | 2 Hours | **Unlimited** |
| **Concurrent Tunnels** | 1 | **Configurable (Multiple)** |
| **Self-Hostable** | No | **Yes (100% Owned)** |
| **Request Replay** | Paid | **Built-in Free** |
| **Corporate Firewall Bypass** | Partial | **WebSocket-over-443 Fallback** |
| **Declarative Config** | Limited | **Yes (`tunnels.yaml`)** |
| **Data Privacy** | Third-party cloud | **Your Own Infrastructure** |

---

## 🏗️ Architecture Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                       Your VPS / Server                      │
│                                                             │
│   ┌─────────────────────────────────────────────────────┐   │
│   │                 TunnelForge Server                  │   │
│   │                                                     │   │
│   │  ┌──────────────┐    ┌───────────────────────────┐  │   │
│   │  │ Public HTTPS │    │     Agent Session Mux     │  │   │
│   │  │ :443 / Nginx │───▶│  - Subdomain Registry     │  │   │
│   │  └──────────────┘    │  - Auth & Handshake       │  │   │
│   │                      │  - Yamux Multiplexer      │  │   │
│   │  ┌──────────────┐    └───────────────────────────┘  │   │
│   │  │ Agent Listener│              │                     │   │
│   │  │ :7000 (TLS)  │              │ stream.Open()       │   │
│   │  │ or           │              ▼                     │   │
│   │  │ :443/tunnel  │    ┌───────────────────────────┐  │   │
│   │  └──────────────┘    │     Reverse Proxy Engine  │  │   │
│   │                      │  - HTTP Header Injection  │  │   │
│   │  ┌──────────────┐    │  - TCP Stream Passthrough │  │   │
│   │  │ Admin API    │    └───────────────────────────┘  │   │
│   │  │ :8080        │                                   │   │
│   │  └──────────────┘                                   │   │
│   └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────┬───────────────────────────┘
                                  │
                  Persistent TLS / WSS Tunnel Pipe
                  (Outbound from developer laptop)
                                  │
┌─────────────────────────────────▼───────────────────────────┐
│                      Developer Laptop                        │
│                                                             │
│   ┌─────────────────────────────────────────────────────┐   │
│   │                 TunnelForge Agent                    │   │
│   │                                                     │   │
│   │  ┌──────────────┐    ┌───────────────────────────┐  │   │
│   │  │ CLI          │    │  Tunnel Manager            │  │   │
│   │  │ forge up     │───▶│  - Yamux Session          │  │   │
│   │  │ forge status │    │  - Reconnect Loop         │  │   │
│   │  │ forge logs   │    └───────────────────────────┘  │   │
│   │  └──────────────┘                  │                │   │
│   │                                    ▼                │   │
│   │  ┌──────────────┐    ┌───────────────────────────┐  │   │
│   │  │ Debug Web UI │    │  Local Connector          │  │   │
│   │  │ :4040        │    │  net.Dial("localhost:...")│  │   │
│   │  └──────────────┘    └───────────────────────────┘  │   │
│   └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## ⚡ Quickstart Guide

### 1. Install the Agent CLI

Build from source or download pre-compiled binaries:

```bash
# Build locally
make build-agent

# Install to system path
sudo cp bin/forge /usr/local/bin/
```

### 2. Register Agent with Your Server

Register your agent with your TunnelForge server domain:

```bash
forge register --server tunnels.yourdomain.com
```

*(Saved to `~/.config/tunnelforge/config.yaml`)*

### 3. Create a `tunnels.yaml` Configuration

Create a file named `tunnels.yaml` in your project directory:

```yaml
tunnels:
  web:
    local: localhost:3000
    capture: true
  api:
    local: localhost:8080
    capture: true
```

### 4. Start Tunnels

```bash
forge up -c tunnels.yaml
```

Output:

```text
[Main] Connecting to TunnelForge...
[Connect] Connected to server with TLS
[UI] Dashboard running at http://localhost:4040
Tunnel registered: web → localhost:3000
Tunnel registered: api → localhost:8080
[Tunnel] Tunnel is running
```

Your services are now publicly live at:

* `https://web.tunnels.yourdomain.com`
* `https://api.tunnels.yourdomain.com`

---

## 🛠️ CLI Reference

| Command | Description | Example |
| :--- | :--- | :--- |
| `forge register` | Authenticate agent and save enrollment credentials | `forge register --server tunnels.yourdomain.com -e my_secret_key` |
| `forge up` | Launch reverse tunnels defined in config | `forge up -c tunnels.yaml` |
| `forge status` | Display active tunnel routing and health status | `forge status` |
| `forge logs` | Tail captured HTTP request logs | `forge logs -f -n 20` |
| `forge cert` | Display or export embedded Root CA certificate | `forge cert -o ca.crt` |

### Registration & Enrollment Keys

When registering your agent, the enrollment key is resolved using the following priority order:

1. `-e, --enrollment-key` CLI flag
2. `FORGE_ENROLLMENT_KEY` environment variable
3. Default key (`tf_enroll_xyz123`)

On the server, you can set custom enrollment keys via:

* `tunnelforge-server --enrollment-key <your-key>`
* `FORGE_ENROLLMENT_KEY=<your-key>` environment variable

### Command Flags for `forge up`

* `-c, --config <path>`: Path to YAML config file (default `./tunnels.yaml`).
* `-s, --server <addr>`: Override server address.
* `--ws`: Force WebSocket transport fallback (bypasses port `:7000`).
* `--insecure`: Disable TLS encryption for local testing.

---

## 🌐 Self-Hosting & Deployment

To deploy your own TunnelForge server on an Oracle Cloud VPS, DigitalOcean, AWS, or bare metal, see the comprehensive **[Self-Hosting Guide (GUIDE.md)](GUIDE.md)**.

---

## 📜 License

MIT License. See [LICENSE](LICENSE) for details.
