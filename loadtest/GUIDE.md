# TunnelForge — Numerical Load Evaluation Guide

> **Goal**: Obtain real latency percentiles (p50/p95/p99), throughput (req/s), and error rates
> through the TunnelForge tunnel stack, and compare against a direct baseline connection.

---

## Why Numerical Evaluation Matters

Rather than saying *"the tunnel adds some latency"*, this guide produces concrete numbers:

| Claim (vague) | Measurement (concrete) |
|---|---|
| "Low latency tunneling" | p50 = 12ms, p99 = 38ms at c=50 |
| "Handles concurrent traffic" | 487 req/s sustained at c=100, 0% errors |
| "Minimal overhead vs. direct" | +9ms p50 overhead vs. direct connection |

---

## Architecture Under Test

```
[hey load generator]  ──HTTPS──►  [Nginx :443]
                                       │
                                  proxy_pass
                                       │
                               [TunnelForge Server :8000]
                                       │
                              Yamux multiplexed stream
                                       │
                               [TunnelForge Agent]
                                       │
                              local TCP dial :8080
                                       │
                              [Echo Server :8080]
```

The test exercises the **complete tunnel stack** end-to-end:
TLS termination → Nginx → TunnelForge server → Yamux stream → Agent → local service.

---

## Prerequisites

### 1. Install `hey`
```bash
go install github.com/rakyll/hey@latest
# Verify:
hey --help
```

### 2. Install Python deps (for plots)
```bash
pip3 install matplotlib
```

### 3. Start the echo server locally
```bash
# Terminal 1: run the echo server
go run loadtest/echo_server.go
# Should print: [echo-server] Listening on :8080
```

### 4. Register a TunnelForge agent pointing at the echo server
```bash
# Terminal 2: register agent
tunnelforge-agent --local-port 8080 --subdomain loadtest
# Or using your config file
```

### 5. Verify the tunnel works before load testing
```bash
curl https://loadtest.tunnels.pratheepsri.me/health
# Expected: {"status":"ok"}
```

---

## Running the Load Test

```bash
# Set your tunnel subdomain
export TUNNEL_SUBDOMAIN="loadtest"
export TUNNEL_HOST="tunnels.pratheepsri.me"

# Optional: enable baseline comparison (direct local connection)
# export BASELINE_URL="http://localhost:8080/"

chmod +x loadtest/run_loadtest.sh
bash loadtest/run_loadtest.sh
```

The script runs **7 stages** with increasing concurrency, sleeping 10 seconds between each:

| Stage | Concurrency | Duration | Purpose |
|-------|-------------|----------|---------|
| 1     | 1           | 15s      | Warm-up, establish baseline latency |
| 2     | 5           | 15s      | Light load |
| 3     | 10          | 15s      | Moderate load |
| 4     | 25          | 15s      | Medium load |
| 5     | 50          | 15s      | Standard production simulation |
| 6     | 75          | 15s      | Near-peak for Oracle Free Tier |
| 7     | 100         | 15s      | Stress ceiling |

> **Safety**: 100 concurrent `hey` workers generate ~400–600 req/s on a well-optimized path.
> The Oracle Free Tier (1 OCPU, 1 GB RAM) can handle this comfortably for 15-second bursts.
> The 10s sleep between stages lets the VPS CPU cool down.

---

## Analyzing Results

```bash
python3 loadtest/analyze.py
# Or specify a custom results dir:
python3 loadtest/analyze.py --results-dir loadtest/results
```

**Outputs:**
- `loadtest/results/report.md` — Full markdown table
- `loadtest/results/throughput_plot.png` — Req/s vs. concurrency
- `loadtest/results/latency_plot.png` — p50/p95/p99 vs. concurrency
- `loadtest/results/error_plot.png` — Error % vs. concurrency (if non-zero errors)

---

## What to Measure on the VPS Simultaneously

While the load test runs, open an SSH session to your VPS and monitor:

```bash
# Watch CPU + memory every 2 seconds
watch -n 2 'top -bn1 | head -20'

# Or more readable:
htop

# Watch TunnelForge goroutine/memory stats (if pprof endpoint is enabled):
# curl http://127.0.0.1:8000/debug/pprof/goroutine?debug=1 | head -30

# Watch active connections:
ss -s

# Watch Nginx access log for errors:
sudo tail -f /var/log/nginx/error.log
```

**Key metrics to record:**
| Metric | How to capture |
|--------|---------------|
| CPU % | `top` → `%Cpu(s)` line |
| Memory used | `top` → `KiB Mem` or `free -h` |
| Active TCP connections | `ss -s` → `estab` count |
| Goroutine count | `/debug/pprof/goroutine` (if enabled) |

---

## Expected Results (Reference Ranges)

These are **expected** ranges for an Oracle Free Tier (1 OCPU, 1 GB RAM) VPS with
a low-latency agent (agent on same LAN or localhost).

| Concurrency | Expected Req/s | Expected p50 | Expected p99 |
|-------------|---------------|-------------|-------------|
| 1           | 60–120        | 8–20 ms     | 25–50 ms    |
| 10          | 200–400       | 15–30 ms    | 50–100 ms   |
| 50          | 300–600       | 30–70 ms    | 100–250 ms  |
| 100         | 300–700       | 50–120 ms   | 200–500 ms  |

> Numbers vary heavily based on: network RTT between load gen machine and VPS,
> local service response time, VPS CPU/network throttling.

---

## Interpreting the Results

### Throughput Saturation
If req/s **plateaus or drops** between concurrency levels (e.g., flat from c=50→c=100),
the system has reached its throughput ceiling. The bottleneck could be:
- CPU on VPS (check `top`)
- Yamux stream scheduling (internal)
- Network bandwidth

### Latency Growth
- **Linear growth** with concurrency = normal queuing behavior
- **Exponential growth** = saturation, consider the VPS limit

### Error Rate
- 0% errors up to c=100 → excellent
- Errors starting at c=X → that's your practical concurrency limit

### Overhead vs. Baseline
```
Tunnel overhead = tunnel_p50 - baseline_p50
```
For a same-machine agent, overhead should be **< 5ms at p50** (Yamux + HTTP proxy layer).
For an agent on a different network, add RTT.

---

## Documenting Results for the Architecture Doc

Once you have real numbers, add them to `ARCHITCTURE.md` under a **Performance Evaluation** section:

```markdown
## Performance Evaluation

Tested on Oracle Free Tier (1 OCPU, 1 GB RAM), using `hey` with a local echo server
behind the agent. Load generated from a local machine (RTT ≈ Xms to VPS).

| Concurrency | Req/s | p50 (ms) | p95 (ms) | p99 (ms) | Error % |
|-------------|-------|----------|----------|----------|---------|
| 1           | X     | X        | X        | X        | 0%      |
| 10          | X     | X        | X        | X        | 0%      |
| 50          | X     | X        | X        | X        | 0%      |
| 100         | X     | X        | X        | X        | 0%      |

**Tunnel overhead vs. direct connection**: +Xms at p50 (measured at c=50).
**Peak sustainable throughput**: ~X req/s before error rate exceeds 1%.
```

---

## Files

| File | Purpose |
|------|---------|
| `loadtest/run_loadtest.sh` | Main load test runner (uses `hey`) |
| `loadtest/echo_server.go` | Minimal local HTTP echo server |
| `loadtest/analyze.py` | Parse CSVs → markdown table + plots |
| `loadtest/results/` | Auto-created output directory |
