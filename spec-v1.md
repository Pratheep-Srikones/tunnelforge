# TunnelForge — V1 Specification
>
> Self-hosted secure tunnel platform · Initial Architecture & Requirements Document  
> Status: `DRAFT` | Version: `0.1.0` | Started: May 2026

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Goals & Non-Goals for V1](#2-goals--non-goals-for-v1)
3. [System Architecture](#3-system-architecture)
4. [Component Breakdown](#4-component-breakdown)
5. [Data Flow](#5-data-flow)
6. [Repository Structure](#6-repository-structure)
7. [Functional Requirements](#7-functional-requirements)
8. [Non-Functional Requirements](#8-non-functional-requirements)
9. [API & Protocol Design](#9-api--protocol-design)
10. [Configuration Spec](#10-configuration-spec)
11. [Security Model](#11-security-model)
12. [Infrastructure & Deployment](#12-infrastructure--deployment)
13. [Extension Points (Future Versions)](#13-extension-points-future-versions)
14. [Milestones & Phased Build Plan](#14-milestones--phased-build-plan)
15. [Decision Log](#15-decision-log)
16. [Open Questions](#16-open-questions)

---

## 1. Project Overview

TunnelForge is a self-hosted reverse tunnel platform that exposes local development
services to the public internet through a persistent outbound connection to a
user-controlled VPS. It is designed as a developer tool with deep protocol visibility
and a pluggable middleware architecture — not just a clone of existing tools, but a
platform that can grow.

### Problem Statement

- Existing solutions (ngrok, VS Code port forwarding) are either blocked by corporate
  firewalls, require trust in a third party, or restrict useful features behind paywalls.
- Developers need a tunnel they own: no rate limits, no session timeouts, no black-box
  inspection, and no data leaving their own infrastructure.

### Key Differentiators from V1

| Feature | ngrok Free | TunnelForge V1 |
| --- | --- | --- |
| Session time limit | 2 hours | None |
| Concurrent tunnels | 1 | Configurable |
| Traffic inspection | Basic | Full request/response log |
| Request replay | Paid | Built-in |
| Self-hostable | No | Core design requirement |
| Corporate proxy bypass | Partial | WebSocket-over-443 fallback |
| Declarative config | No | Yes (`tunnels.yaml`) |
| Open source | No | Yes |

---

## 2. Goals & Non-Goals for V1

### Goals

- [x] A working server binary deployable on Oracle Free Tier (ARM64 Ubuntu)
- [x] A working agent binary that runs on developer laptops (macOS, Linux, Windows)
- [x] Persistent multiplexed tunnels via yamux over TLS
- [x] WebSocket transport fallback for corporate proxy environments
- [x] HTTP/HTTPS tunnel type (the most common use case)
- [x] TCP tunnel type (raw passthrough for non-HTTP services)
- [x] Subdomain-based routing (`myapp.tunnel.yourdomain.com`)
- [x] Token-based agent authentication
- [x] Wildcard TLS via Let's Encrypt (auto-renewal)
- [x] Declarative `tunnels.yaml` config on the agent side
- [x] Request/response logging with in-memory ring buffer (last N requests)
- [x] Minimal local web UI: live request log + one-click replay
- [x] CLI for the agent: `forge up`, `forge up <name>`, `forge status`, `forge logs`
- [x] Graceful reconnect with exponential backoff
- [x] Server admin: list active sessions, disconnect a tunnel

### Non-Goals for V1 (explicitly deferred)

- ❌ mTLS agent authentication (V2)
- ❌ gRPC/WebSocket protocol-aware routing (V2)
- ❌ Request mutation middleware / rule engine (V2)
- ❌ Response diffing / regression detection (V2)
- ❌ QUIC transport (V3)
- ❌ Multi-user / team support (V3)
- ❌ Metrics export (Prometheus/Grafana) (V2)
- ❌ GUI desktop app (not planned)
- ❌ Kubernetes operator (not planned for personal project)

---

## 3. System Architecture

### High-Level Topology

```mermaid
┌─────────────────────────────────────────────────────────────┐
│                      Oracle Free Tier VPS                    │
│                                                             │
│   ┌─────────────────────────────────────────────────────┐   │
│   │                  TunnelForge Server                  │   │
│   │                                                     │   │
│   │  ┌──────────────┐    ┌───────────────────────────┐  │   │
│   │  │ Public HTTPS │    │     Session Manager        │  │   │
│   │  │ Listener     │───▶│  - Registry (subdomain→   │  │   │
│   │  │ :443         │    │    yamux session)          │  │   │
│   │  └──────────────┘    │  - Auth validator          │  │   │
│   │                      │  - Heartbeat monitor       │  │   │
│   │  ┌──────────────┐    └───────────────────────────┘  │   │
│   │  │ Agent Tunnel │              │                     │   │
│   │  │ Listener     │              │ stream.Open()       │   │
│   │  │ :7000 (TLS)  │              ▼                     │   │
│   │  │ or           │    ┌───────────────────────────┐  │   │
│   │  │ :443/ws path │    │     Reverse Proxy Core     │  │   │
│   │  └──────────────┘    │  - HTTP proxy handler      │  │   │
│   │                      │  - TCP proxy handler        │  │   │
│   │  ┌──────────────┐    │  - Request logger           │  │   │
│   │  │ Admin API    │    └───────────────────────────┘  │   │
│   │  │ :8080        │                                   │   │
│   │  └──────────────┘                                   │   │
│   └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────┬───────────────────────────┘
                                  │
                       Persistent TLS/WS conn
                       (outbound from laptop,
                        not blocked by firewall)
                                  │
┌─────────────────────────────────▼───────────────────────────┐
│                     Developer Laptop                         │
│                                                             │
│   ┌─────────────────────────────────────────────────────┐   │
│   │                  TunnelForge Agent                   │   │
│   │                                                     │   │
│   │  ┌──────────────┐    ┌───────────────────────────┐  │   │
│   │  │ CLI          │    │  Tunnel Manager            │  │   │
│   │  │ forge up     │───▶│  - yamux session           │  │   │
│   │  │ forge status │    │  - Stream acceptor         │  │   │
│   │  │ forge logs   │    │  - Reconnect logic         │  │   │
│   │  └──────────────┘    └───────────────────────────┘  │   │
│   │                                  │                   │   │
│   │  ┌──────────────┐                │ io.Copy           │   │
│   │  │ Local Web UI │                ▼                   │   │
│   │  │ :4040        │    ┌───────────────────────────┐  │   │
│   │  │ (request log │    │  Local Service Connector   │  │   │
│   │  │  + replay)   │    │  net.Dial("localhost:3000")│  │   │
│   │  └──────────────┘    └───────────────────────────┘  │   │
│   └─────────────────────────────────────────────────────┘   │
│                                                             │
│         localhost:3000 (your actual app)                    │
└─────────────────────────────────────────────────────────────┘
```

### Transport Layer Decision Tree

```memaid
Agent startup
     │
     ▼
Try raw TLS on :7000
     │
     ├─ Success ──────────────────────► Use TLS/yamux transport
     │
     └─ Fail (blocked)
          │
          ▼
     Try WebSocket upgrade on :443/tunnel
          │
          ├─ Success ──────────────────► Use WS/yamux transport
          │
          └─ Fail
               │
               ▼
          Report error, retry with backoff
```

---

## 4. Component Breakdown

### 4.1 Server (`cmd/server`)

Responsibilities:

- Listen for public HTTP/HTTPS traffic on :443
- Listen for agent connections on :7000 (TLS) and :443/ws (WebSocket upgrade)
- Maintain a registry of `subdomain → yamux.Session`
- Route incoming public requests to the correct session, open a stream, proxy it
- Expose admin API on :8080 (localhost only)
- Manage TLS certificates (autocert or configured cert path)

Key internal packages:

- `internal/registry` — thread-safe map of active tunnel sessions
- `internal/proxy` — HTTP and TCP reverse proxy implementations
- `internal/transport` — accepts raw TLS and WebSocket agent connections, abstracts both as `net.Conn`
- `internal/auth` — validates agent token from handshake
- `internal/admin` — HTTP admin API handlers

### 4.2 Agent (`cmd/agent`)

Responsibilities:

- Read `tunnels.yaml` and establish one session per configured tunnel
- Connect outbound to the server (TLS or WS fallback)
- Send auth handshake with token and requested subdomain
- Accept yamux streams and dial the local service for each one
- Capture HTTP request/response data into a ring buffer
- Serve the local web UI at `:4040`
- Handle reconnection with exponential backoff

Key internal packages:

- `internal/session` — manages a single tunnel's yamux session and reconnect loop
- `internal/capture` — HTTP-aware middleware that buffers req/res for the UI
- `internal/replay` — takes a stored request entry and re-fires it
- `internal/ui` — embeds and serves the web UI static files + JSON API

### 4.3 Shared (`internal/`)

- `internal/proto` — the handshake message format (shared between server and agent)
- `internal/config` — config file parsing and validation
- `internal/version` — single source of truth for version string

---

## 5. Data Flow

### 5.1 Tunnel Setup Flow

```mermaid
Agent                           Server
  │                               │
  │── TLS Dial :7000 ────────────▶│
  │                               │
  │── Handshake{                  │
  │     token: "xxx",             │
  │     subdomain: "myapp",       │
  │     version: "0.1.0",         │
  │     transport: "tcp"          │
  │   } ─────────────────────────▶│
  │                               │── Validate token
  │                               │── Check subdomain availability
  │                               │── Register session
  │◀── HandshakeAck{              │
  │      ok: true,                │
  │      assigned: "myapp",       │
  │      public_url: "https://myapp.tunnel.yourdomain.com"
  │    } ─────────────────────────│
  │                               │
  │══════ yamux session open ═════│
  │     (persistent connection)   │
```

### 5.2 HTTP Request Flow

```mermaid
Browser                  Server                  Agent              Local App
  │                        │                       │                    │
  │── GET /api/users ──────▶│                       │                    │
  │   Host: myapp.tunnel.. │                       │                    │
  │                        │── Lookup "myapp" ─    │                    │
  │                        │   in registry         │                    │
  │                        │── session.Open() ────▶│                    │
  │                        │   (new yamux stream)  │                    │
  │                        │                       │── net.Dial ───────▶│
  │                        │                       │  localhost:3000    │
  │                        │── Write HTTP req ─────▶│                    │
  │                        │   onto stream         │── Write req ──────▶│
  │                        │                       │◀── Response ───────│
  │                        │◀── HTTP response ─────│                    │
  │◀── Response ───────────│                       │                    │
  │                        │── stream.Close()      │                    │
```

### 5.3 Request Capture Flow (HTTP tunnels only)

```mermaid
Stream accepted by agent
        │
        ▼
 capture.Interceptor wraps the stream
        │
        ├── Reads and buffers the full HTTP request
        ├── Dials local service
        ├── Reads and buffers the HTTP response
        │
        ▼
 RingBuffer.Push(RequestEntry{
   id, timestamp, method, path,
   headers, body, duration,
   response_status, response_headers, response_body
 })
        │
        ▼
 WebSocket broadcast to local UI (if connected)
```

---

## 6. Repository Structure

```mermaid
tunnelforge/
├── cmd/
│   ├── server/
│   │   └── main.go              # Server entrypoint
│   └── agent/
│       └── main.go              # Agent entrypoint + CLI
│
├── internal/
│   ├── proto/
│   │   └── handshake.go         # Shared wire protocol types
│   ├── config/
│   │   ├── config.go            # Config structs
│   │   └── loader.go            # YAML parsing + validation
│   ├── registry/
│   │   └── registry.go          # Thread-safe subdomain→session map
│   ├── transport/
│   │   ├── tls.go               # Raw TLS listener/dialer
│   │   └── websocket.go         # WebSocket adapter (net.Conn wrapper)
│   ├── proxy/
│   │   ├── http.go              # HTTP/HTTPS reverse proxy
│   │   └── tcp.go               # Raw TCP stream copier
│   ├── auth/
│   │   └── token.go             # Token validation
│   ├── session/
│   │   └── session.go           # Agent-side yamux session + reconnect
│   ├── capture/
│   │   ├── interceptor.go       # HTTP req/res buffering middleware
│   │   └── ringbuffer.go        # Fixed-size ring buffer for entries
│   ├── replay/
│   │   └── replay.go            # Re-fires a stored request entry
│   ├── ui/
│   │   ├── server.go            # Local UI HTTP server + WS broadcast
│   │   └── embed.go             # go:embed for static files
│   ├── admin/
│   │   └── api.go               # Server admin API handlers
│   └── version/
│       └── version.go           # Version constant
│
├── web/                         # Local UI (plain HTML/JS, no build step)
│   ├── index.html
│   ├── app.js
│   └── style.css
│
├── deploy/
│   ├── server.service           # systemd unit file for server
│   ├── setup.sh                 # VPS bootstrap script
│   └── Caddyfile.example        # If using Caddy as TLS frontend
│
├── docs/
│   ├── SPEC_V1.md               # This document
│   └── CONTRIBUTING.md
│
├── tunnels.yaml.example         # Reference config for agents
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 7. Functional Requirements

### FR-01: Agent Transport

| ID | Requirement |
| --- | --- |
| FR-01-1 | Agent MUST initiate all connections outbound (no inbound listener required on agent) |
| FR-01-2 | Agent MUST attempt raw TLS on port 7000 first |
| FR-01-3 | Agent MUST fall back to WebSocket over port 443 at path `/tunnel` if TLS fails |
| FR-01-4 | Agent MUST send a heartbeat ping every 25 seconds to prevent idle connection drops |
| FR-01-5 | Agent MUST reconnect automatically on disconnect, using exponential backoff starting at 1s, capped at 60s |
| FR-01-6 | Agent MUST log the public URL to stdout on successful tunnel establishment |

### FR-02: Authentication

| ID | Requirement |
| --- | --- |
| FR-02-1 | Server MUST reject agent connections that do not present a valid token |
| FR-02-2 | Token is a shared secret configured on both server and agent via env var or config file |
| FR-02-3 | Server MUST respond with a structured error on invalid auth before closing the conn |
| FR-02-4 | Token MUST be at minimum 32 bytes of random data (enforced on server startup) |

### FR-03: Tunnel Management

| ID | Requirement |
| --- | --- |
| FR-03-1 | Server MUST support multiple concurrent sessions (multiple agents/tunnels at once) |
| FR-03-2 | Subdomains must be unique; server MUST reject a registration if the subdomain is already active |
| FR-03-3 | When a session disconnects, its subdomain MUST be freed within 5 seconds |
| FR-03-4 | Server MUST return 502 to the browser if the target tunnel is not registered |
| FR-03-5 | Agent MUST read all tunnels from `tunnels.yaml` and establish each one concurrently |

### FR-04: HTTP Proxying

| ID | Requirement |
| --- | --- |
| FR-04-1 | Server MUST forward all HTTP headers from the public request to the local service |
| FR-04-2 | Server MUST inject `X-Forwarded-For`, `X-Forwarded-Proto`, and `X-Tunnel-Host` headers |
| FR-04-3 | Server MUST support streaming responses (chunked transfer encoding) |
| FR-04-4 | Server MUST support WebSocket upgrade on HTTP tunnels (pass-through) |
| FR-04-5 | Agent's local connector MUST support HTTP/1.1 keep-alive to the local service |

### FR-05: TCP Proxying

| ID | Requirement |
| --- | --- |
| FR-05-1 | TCP tunnels MUST do bidirectional raw byte copy with no protocol awareness |
| FR-05-2 | TCP tunnels MUST close both sides when either side closes |
| FR-05-3 | TCP tunnel entries MUST appear in admin listing but NOT in the request log (no HTTP parsing) |

### FR-06: Request Logging & Replay (HTTP tunnels only)

| ID | Requirement |
| --- | --- |
| FR-06-1 | Agent MUST buffer the last 200 HTTP requests per tunnel in memory |
| FR-06-2 | Each entry MUST store: id, timestamp, method, URL, request headers, request body (up to 1MB), response status, response headers, response body (up to 1MB), duration_ms |
| FR-06-3 | Bodies larger than 1MB MUST be stored truncated with a `truncated: true` flag |
| FR-06-4 | Agent UI MUST allow re-sending any stored request to the local service |
| FR-06-5 | Replay MUST show the new response inline in the UI |
| FR-06-6 | Ring buffer MUST be per-tunnel, not shared |

### FR-07: Local Web UI

| ID | Requirement |
| --- | --- |
| FR-07-1 | UI MUST be served at `http://localhost:4040` when the agent is running |
| FR-07-2 | UI MUST show a live feed of requests (WebSocket push from agent) |
| FR-07-3 | UI MUST show: method, path, status, duration for each request in list view |
| FR-07-4 | UI MUST show full headers and body when a request is selected |
| FR-07-5 | UI MUST have a Replay button per request |
| FR-07-6 | UI MUST show connection status (connected/disconnected/reconnecting) per tunnel |
| FR-07-7 | UI MUST be a single HTML file with embedded JS — no build step, no node_modules |

### FR-08: CLI

| ID | Requirement |
| --- | --- |
| FR-08-1 | `forge up` — start all tunnels from `tunnels.yaml` |
| FR-08-2 | `forge up <name>` — start a single named tunnel |
| FR-08-3 | `forge status` — show active tunnels, public URLs, request counts |
| FR-08-4 | `forge logs [name]` — tail request log to stdout (JSON or human-readable) |
| FR-08-5 | `forge token generate` — generate a cryptographically secure token |
| FR-08-6 | All commands support `--config <path>` flag |

### FR-09: Server Admin API

| ID | Requirement |
| --- | --- |
| FR-09-1 | `GET /admin/sessions` — list all active tunnel sessions with metadata |
| FR-09-2 | `DELETE /admin/sessions/{subdomain}` — force-disconnect a tunnel |
| FR-09-3 | Admin API MUST bind to localhost only (127.0.0.1:8080), never publicly |
| FR-09-4 | Admin API MUST require a separate admin token (env var) |

---

## 8. Non-Functional Requirements

### NFR-01: Performance

- Single tunnel proxy overhead MUST be < 5ms P99 added latency on a typical HTTP request
- Server MUST handle at least 50 concurrent open streams without goroutine leak
- Ring buffer MUST not block the proxy path — writes to it MUST be non-blocking (drop if full)

### NFR-02: Reliability

- Agent reconnect logic MUST resume automatically with no manual intervention
- Server MUST recover from a panicking handler without taking down other tunnels (recover middleware)
- Yamux sessions MUST be cleaned up fully on disconnect (no goroutine or memory leak)

### NFR-03: Security

- All agent→server connections MUST be TLS 1.2 minimum
- Token MUST be transmitted only inside the TLS handshake, never in cleartext
- Admin API MUST be localhost-bound, not configurable to 0.0.0.0 (hardcoded)
- Server MUST not allow subdomain squatting: `admin`, `api`, `health`, `tunnel` are reserved

### NFR-04: Portability

- Agent binary MUST be a single static binary (CGO_ENABLED=0)
- Agent MUST run on: macOS (amd64, arm64), Linux (amd64, arm64), Windows (amd64)
- Server MUST run on: Linux arm64 (Oracle Free Tier Ampere)
- Cross-compilation targets MUST be defined in Makefile

### NFR-05: Observability

- Server MUST write structured JSON logs (zerolog or slog)
- Agent MUST write human-readable logs by default, `--json` flag for structured
- Both binaries MUST expose a `/health` endpoint returning `{"status":"ok","version":"x.x.x"}`
- Both MUST log goroutine count on SIGUSR1 (useful for leak detection)

### NFR-06: Maintainability

- All extension points (transport, proxy, middleware) MUST be defined as interfaces
- No circular imports between packages
- All exported functions and types MUST have godoc comments
- V1 passes `go vet`, `staticcheck`, and has > 60% test coverage on `internal/` packages

---

## 9. API & Protocol Design

### 9.1 Handshake Protocol

The handshake is a length-prefixed JSON message sent immediately after the TLS (or WS)
connection is established, before yamux session negotiation.

```mermaid
Wire format:
  [4 bytes big-endian uint32: message length]
  [N bytes: JSON payload]
```

#### Agent → Server (HandshakeRequest)

```json
{
  "version": "0.1.0",
  "token": "base64-encoded-secret",
  "tunnels": [
    {
      "subdomain": "myapp",
      "protocol": "http",
      "local_addr": "localhost:3000"
    },
    {
      "subdomain": "mydb",
      "protocol": "tcp",
      "local_addr": "localhost:5432"
    }
  ]
}
```

#### Server → Agent (HandshakeResponse)

Success:

```json
{
  "ok": true,
  "tunnels": [
    {
      "subdomain": "myapp",
      "protocol": "http",
      "public_url": "https://myapp.tunnel.yourdomain.com"
    },
    {
      "subdomain": "mydb",
      "protocol": "tcp",
      "public_url": "tcp://mydb.tunnel.yourdomain.com:7100"
    }
  ]
}
```

Failure:

```json
{
  "ok": false,
  "error": "subdomain 'myapp' is already in use",
  "code": "SUBDOMAIN_CONFLICT"
}
```

**Error codes (defined as constants in `internal/proto`):**

| Code | Meaning |
| --- | --- |
| `AUTH_FAILED` | Token rejected |
| `SUBDOMAIN_CONFLICT` | Subdomain already registered |
| `SUBDOMAIN_RESERVED` | Subdomain is on the reserved list |
| `VERSION_MISMATCH` | Server requires a newer agent |
| `INTERNAL_ERROR` | Catch-all server error |

### 9.2 Stream Header Protocol

Each yamux stream opened by the server begins with a 1-byte protocol tag so the agent
knows how to handle it:

```bash
0x01 = HTTP stream  → parse and capture for UI
0x02 = TCP stream   → raw copy, no capture
0x03 = Control      → reserved for future use
```

### 9.3 Admin API

All admin endpoints return JSON. All require header `X-Admin-Token: <token>`.

```bash
GET  /admin/health
GET  /admin/sessions
GET  /admin/sessions/{subdomain}
DELETE /admin/sessions/{subdomain}
```

`GET /admin/sessions` response:

```json
{
  "sessions": [
    {
      "subdomain": "myapp",
      "protocol": "http",
      "public_url": "https://myapp.tunnel.yourdomain.com",
      "connected_at": "2026-05-25T10:00:00Z",
      "request_count": 142,
      "bytes_in": 88320,
      "bytes_out": 1204480,
      "remote_addr": "203.0.113.4:54321"
    }
  ]
}
```

### 9.4 Local UI WebSocket Events

The agent pushes events to the local UI over a WebSocket at `ws://localhost:4040/ws`.

```json
// New request captured
{
  "type": "request",
  "tunnel": "myapp",
  "entry": {
    "id": "01HX...",
    "timestamp": "2026-05-25T10:01:00Z",
    "method": "POST",
    "url": "/api/webhooks",
    "request_headers": { "Content-Type": "application/json" },
    "request_body": "{\"event\":\"push\"}",
    "response_status": 200,
    "response_headers": { "Content-Type": "application/json" },
    "response_body": "{\"ok\":true}",
    "duration_ms": 12,
    "truncated": false
  }
}

// Tunnel status change
{
  "type": "status",
  "tunnel": "myapp",
  "status": "connected" // connected | disconnected | reconnecting
}
```

---

## 10. Configuration Spec

### Agent Config (`tunnels.yaml`)

```yaml
# tunnels.yaml
server:
  host: tunnel.yourdomain.com
  agent_port: 7000            # Raw TLS port (fallback to WS if blocked)
  ws_path: /tunnel            # WebSocket path for fallback transport
  token: "${TUNNEL_TOKEN}"    # Supports env var expansion

agent:
  ui_port: 4040               # Local web UI port
  log_level: info             # debug | info | warn | error
  log_format: human           # human | json

tunnels:
  api:
    local: localhost:3000
    subdomain: myapp-api
    protocol: http            # http | tcp
    capture: true             # Enable request logging (HTTP only)
    capture_limit: 200        # Ring buffer size

  db:
    local: localhost:5432
    subdomain: myapp-db
    protocol: tcp
    capture: false
```

### Server Config (env vars + optional `server.yaml`)

```yaml
# server.yaml (or set as env vars with FORGE_ prefix)
server:
  agent_port: 7000
  public_port: 443
  admin_port: 8080            # Always binds to 127.0.0.1 only
  domain: tunnel.yourdomain.com
  reserved_subdomains:
    - admin
    - api
    - health
    - tunnel
    - www

tls:
  mode: autocert              # autocert | manual
  email: you@yourdomain.com   # For Let's Encrypt
  cert_dir: /etc/tunnelforge/certs
  # For manual mode:
  # cert_file: /path/to/cert.pem
  # key_file: /path/to/key.pem

auth:
  token: "${FORGE_TOKEN}"     # Min 32 bytes random, base64 encoded
  admin_token: "${FORGE_ADMIN_TOKEN}"

logging:
  level: info
  format: json
```

---

## 11. Security Model

### Threat Model

| Threat | Mitigation |
| --- | --- |
| Unauthenticated agent connects | Token checked before yamux session opens |
| Token brute force | Token is 32+ bytes random; no auth retry loop (conn closed on failure) |
| Subdomain hijacking after disconnect | Subdomain freed only after session confirmed dead, 5s grace |
| Public traffic to non-existent tunnel | 502 returned immediately, no info leakage |
| Admin API exposed publicly | Hardcoded to 127.0.0.1, no override |
| Cleartext token over network | Token only sent inside TLS layer |
| Agent binary exfiltrating data | Self-hosted; user runs their own server |

### What V1 Does NOT Protect Against

- A compromised VPS — if someone controls your server, they see all tunnel traffic. This is
  a self-hosted tool; securing your VPS is outside scope.
- Request body content — the ring buffer stores decrypted request/response bodies in memory.
  These are not encrypted at rest.
- Multiple users on the same VPS — V1 is single-tenant by design.

### Token Requirements (enforced at startup)

```go
// Server refuses to start if token is shorter than this
const MinTokenBytes = 32

// Generate a valid token:
// forge token generate
// → prints a base64url-encoded 32-byte random token
```

---

## 12. Infrastructure & Deployment

### VPS Requirements

- Oracle Free Tier — Ampere A1 (ARM64) Ubuntu 22.04
- 1 OCPU, 6GB RAM (free tier limit) — more than enough
- Static public IP (assigned free on OCI)

### Ports to Open

| Port | Where | Purpose |
| --- | --- | --- |
| 443 | OCI Security List + iptables | Public HTTPS + WS fallback for agents |
| 7000 | OCI Security List + iptables | Agent TLS connection (if not blocked) |
| 8080 | NOT opened | Admin API (localhost only) |

### OCI Security List Rules

```bash
Ingress: 0.0.0.0/0  TCP  443   ALLOW
Ingress: 0.0.0.0/0  TCP  7000  ALLOW
Egress:  0.0.0.0/0  ALL        ALLOW (default)
```

### iptables (Ubuntu)

```bash
sudo iptables -I INPUT 6 -p tcp --dport 443 -j ACCEPT
sudo iptables -I INPUT 6 -p tcp --dport 7000 -j ACCEPT
sudo netfilter-persistent save
```

### DNS Setup

```bash
# In your DNS provider:
*.tunnel.yourdomain.com  300  IN  A  <YOUR_VPS_IP>
tunnel.yourdomain.com    300  IN  A  <YOUR_VPS_IP>
```

### Systemd Service (`deploy/server.service`)

```ini
[Unit]
Description=TunnelForge Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=tunnelforge
ExecStart=/usr/local/bin/forge-server --config /etc/tunnelforge/server.yaml
Restart=always
RestartSec=5
Environment=FORGE_TOKEN_FILE=/etc/tunnelforge/token
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### Makefile Targets

```makefile
build-server:
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
    go build -ldflags="-s -w -X tunnelforge/internal/version.Version=$(VERSION)" \
    -o dist/forge-server ./cmd/server

build-agent-mac-arm:
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
    go build -o dist/forge-darwin-arm64 ./cmd/agent

build-agent-mac-intel:
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
    go build -o dist/forge-darwin-amd64 ./cmd/agent

build-agent-linux:
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -o dist/forge-linux-amd64 ./cmd/agent

build-agent-windows:
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -o dist/forge-windows-amd64.exe ./cmd/agent

build-all: build-server build-agent-mac-arm build-agent-mac-intel \
           build-agent-linux build-agent-windows

test:
    go test ./internal/... -race -cover

lint:
    go vet ./...
    staticcheck ./...
```

---

## 13. Extension Points (Future Versions)

The architecture is designed so these can be added without restructuring core code.

### Transport Interface

```go
// internal/transport/transport.go
// Any new transport (QUIC, etc.) implements this
type Transport interface {
    Dial(ctx context.Context, addr string) (net.Conn, error)
    Listen(addr string) (net.Listener, error)
    Name() string
}
```

New transports in V3+ (QUIC) just implement this interface and register themselves.

### Proxy Middleware Chain

```go
// internal/proxy/middleware.go
// Handlers are chained; each can inspect/mutate req and res
// V2 adds: auth injection, IP allowlisting, request mutation rules
type Middleware func(next http.Handler) http.Handler

type Pipeline struct {
    middlewares []Middleware
}

func (p *Pipeline) Use(m Middleware) { ... }
func (p *Pipeline) Build() http.Handler { ... }
```

### Protocol Detector

```go
// internal/proxy/protocol.go
// V2 adds gRPC, WebSocket frame visibility by implementing this
type ProtocolHandler interface {
    Detect(firstBytes []byte) bool
    Handle(stream net.Conn, localAddr string, capture Capturer) error
    Name() string
}
```

### Capture Interface

```go
// internal/capture/capturer.go
// V2 can swap in a persistent (SQLite) capturer for the in-memory ring buffer
type Capturer interface {
    Push(entry RequestEntry)
    List(tunnel string, limit int) []RequestEntry
    Get(id string) (RequestEntry, bool)
    Clear(tunnel string)
}
```

---

## 14. Milestones & Phased Build Plan

This is a slow side project. Each milestone should be independently shippable.

### Milestone 0 — Skeleton (1–2 weekends)

- [x] Repository setup, go.mod, Makefile
- [x] `internal/proto` — handshake types only
- [x] `internal/config` — parse tunnels.yaml
- [x] `cmd/server` and `cmd/agent` main.go stubs that compile and print version
- [ ] Basic README with project vision

### Milestone 1 — Raw Tunnel Works (2–3 weekends)

- [x] (TLS) listener on server (:7000)
- [x] (TLS) dialer on agent
- [x] Handshake implementation (send/receive, auth check)
- [x] yamux session on both sides
- [x] Single HTTP tunnel working (no UI, no config file yet)
- [x] Manual testing: `curl https://myapp.tunnel.yourdomain.com` hits `localhost:3000`

### Milestone 2 — Production-Ready Tunneling (2–3 weekends)

- [x] Multi-tunnel support (multiple subdomains per agent session)
- [x] TCP tunnel type
- [ ] WebSocket transport fallback
- [x] Reconnect with exponential backoff
- [x] `tunnels.yaml` config file loading
- [ ] Wildcard TLS via autocert
- [ ] systemd service + deploy scripts
- [x] Server deploys and runs stably on Oracle VPS

### Milestone 3 — CLI & Observability (1–2 weekends)

- [x] `forge up`, `forge up <name>`, `forge status` commands
- [ ] ~~`forge token generate`~~
- [ ] Structured JSON logging on server
- [x] Human-readable logging on agent
- [x] `/health` endpoint on both binaries
- [x] Admin API (list sessions, disconnect)

### Milestone 4 — Request Capture & UI (2–3 weekends)

- [x] Ring buffer implementation
- [x] HTTP interceptor (capture req/res without blocking proxy path)
- [x] Local web UI: live request list, request detail view
- [x] WebSocket push from agent to UI
- [x] Request replay from UI
- [x] `forge logs` CLI command

### Milestone 5 — Polish & V1 Release (1 weekend)

- [ ] Cross-platform build targets in Makefile
- [ ] Test coverage > 60% for internal packages
- [ ] `go vet` and `staticcheck` clean
- [ ] Write CONTRIBUTING.md
- [ ] Tag `v1.0.0`, create release with pre-built binaries
- [ ] Update README with actual install instructions and demo GIF

---

## 15. Decision Log

| Date | Decision | Rationale | Alternatives Considered |
| --- | --- | --- | --- |
| 2026-05 | Use yamux for multiplexing | Battle-tested, simple Go API, used by HashiCorp tools in production | smux (similar, less community), HTTP/2 (overkill for this, adds complexity) |
| 2026-05 | WebSocket fallback on :443 | Corporate proxies that block raw TCP still allow WS over 443; critical for the core use case | HTTP CONNECT tunnel (complex to implement, many proxies block it too) |
| 2026-05 | Token auth for V1, not mTLS | Gets V1 done faster; mTLS is architecturally isolated in `internal/auth` so it's a drop-in replacement in V2 | mTLS (better security, more setup friction for a personal tool) |
| 2026-05 | In-memory ring buffer for capture | Zero dependencies, zero latency impact; 200 entries is plenty for dev workflow | SQLite (persistent but adds setup; deferred to V2 via Capturer interface) |
| 2026-05 | Plain HTML/JS for local UI | No build step means the UI works immediately; no npm, no webpack, no maintenance | React/Vue (unnecessary complexity for a debug dashboard) |
| 2026-05 | Go over C | Network I/O bound, not CPU bound; Go stdlib is excellent for this; author familiarity | C (no real perf gain for this use case; massively more code) |
| 2026-05 | Single agent binary (not daemon + CLI) | Simpler for a personal tool; daemon model deferred to when multi-user is needed | daemon + socket CLI (better for teams, overkill for V1) |

---

## 16. Open Questions

These are unresolved items that need a decision before the relevant milestone starts.

| # | Question | Needs Decision By | Notes |
| --- | --- | --- | --- |
| 1 | Project name: "TunnelForge" — placeholder, find a better name | Before M1 | Should be short, domain-available, memorable |
| 2 | Should TCP tunnels get their own port range on the server (e.g. :7100-:7200) or be muxed over 443? | Before M2 | Own port range is simpler to implement; muxed is friendlier through firewalls |
| 3 | How should subdomain conflicts be resolved when the agent reconnects — should the old session be evicted or should the reconnect fail? | Before M2 | Probably: evict old session with a grace period |
| 4 | Should `forge logs` stream from the ring buffer over a local socket, or just call the UI API? | Before M3 | UI API is simpler; local socket is more composable |
| 5 | Wildcard cert via autocert requires DNS-01 challenge. Which DNS provider to support first? | Before M2 | Cloudflare is most common; lego library supports many providers |

---

### End of TunnelForge V1 Specification

---
> **Living document.** Update the Decision Log when architectural choices are made.  
> Update Open Questions when resolved. Bump the version header when spec changes substantially.
