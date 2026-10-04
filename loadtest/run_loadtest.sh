#!/usr/bin/env bash
# ==============================================================================
# TunnelForge Load Test Runner
# ==============================================================================
# PURPOSE:
#   Generates HTTP traffic through the TunnelForge tunnel to numerically
#   evaluate throughput (req/s), latency percentiles (p50/p95/p99), and
#   error rates at increasing concurrency levels.
#
# PREREQUISITES:
#   1. TunnelForge server running on VPS (tunnels.pratheepsri.me)
#   2. An agent registered with a known subdomain (TUNNEL_SUBDOMAIN below)
#   3. A local echo server running behind the agent:
#        python3 -m http.server 8080
#        (or use the built-in test server: go run ./loadtest/echo_server.go)
#   4. `hey` installed: go install github.com/rakyll/hey@latest
#   5. `python3` with matplotlib + pandas for the analysis script
#
# SAFETY LIMITS (Oracle VPS Friendly):
#   - Max concurrency: 100 workers (stay below 150 to be safe on 1 OCPU VPS)
#   - Rate-limited ramp-up (each stage runs 15 seconds)
#   - Sleeps between stages let the VPS breathe
#
# USAGE:
#   export TUNNEL_SUBDOMAIN="myagent"    # the subdomain of your registered tunnel
#   export TUNNEL_HOST="tunnels.pratheepsri.me"  # your VPS domain
#   bash loadtest/run_loadtest.sh
#
# OUTPUT:
#   loadtest/results/   — CSV data for each concurrency level
#   loadtest/summary.md — Human-readable results table
# ==============================================================================

set -euo pipefail

# --------------------------------------------------------------------------
# Configuration — override via env vars
# --------------------------------------------------------------------------
TUNNEL_SUBDOMAIN="${TUNNEL_SUBDOMAIN:-myagent}"
TUNNEL_HOST="${TUNNEL_HOST:-tunnels.pratheepsri.me}"
TARGET_URL="https://${TUNNEL_SUBDOMAIN}.${TUNNEL_HOST}/"

# Baseline (direct-to-agent host, bypassing the tunnel for comparison)
# Set this to empty string "" to skip the baseline test.
BASELINE_URL="${BASELINE_URL:-}"  # e.g. "http://localhost:8080/"

DURATION=15s        # Duration per stage (don't go above 30s per stage)
TIMEOUT=10          # Per-request timeout (seconds)
RESULTS_DIR="$(dirname "$0")/results"

# Concurrency ramp — conservative for a 1 OCPU / 1 GB RAM VPS
# Stages: 1, 5, 10, 25, 50, 75, 100
CONCURRENCY_LEVELS=(1 5 10 25 50 75 100 200 500)

# --------------------------------------------------------------------------
mkdir -p "$RESULTS_DIR"
SUMMARY="${RESULTS_DIR}/summary.txt"

echo "=================================================================" | tee "$SUMMARY"
echo "  TunnelForge Load Test — $(date)" | tee -a "$SUMMARY"
echo "  Target: $TARGET_URL" | tee -a "$SUMMARY"
echo "=================================================================" | tee -a "$SUMMARY"
echo "" | tee -a "$SUMMARY"

# ----------
# BASELINE — direct connection (no tunnel overhead)
# ----------
if [[ -n "$BASELINE_URL" ]]; then
    echo "[BASELINE] Direct connection to $BASELINE_URL" | tee -a "$SUMMARY"
    hey -z "$DURATION" -c 50 -t "$TIMEOUT" \
        -o csv \
        "$BASELINE_URL" > "${RESULTS_DIR}/baseline_c50.csv" 2>&1 || true
    
    BASELINE_SUMMARY=$(hey -z "$DURATION" -c 50 -t "$TIMEOUT" "$BASELINE_URL" 2>&1 || true)
    echo "$BASELINE_SUMMARY" | grep -E "Requests/sec|Average|Slowest|Fastest|p(50|75|90|95|99)" | tee -a "$SUMMARY"
    echo "" | tee -a "$SUMMARY"
    sleep 5
fi

# ----------
# TUNNEL RAMP TEST
# ----------
echo "" | tee -a "$SUMMARY"
echo "Stage | Concurrency | Req/s  | p50 (ms) | p95 (ms) | p99 (ms) | Errors" | tee -a "$SUMMARY"
echo "------|-------------|--------|----------|----------|----------|-------" | tee -a "$SUMMARY"

for c in "${CONCURRENCY_LEVELS[@]}"; do
    echo ""
    echo ">>> Stage: concurrency=$c, duration=$DURATION, target=$TARGET_URL"

    CSV_FILE="${RESULTS_DIR}/tunnel_c${c}.csv"
    FULL_OUTPUT="${RESULTS_DIR}/tunnel_c${c}_full.txt"

    # Run hey: capture full output AND csv
    hey -z "$DURATION" -c "$c" -t "$TIMEOUT" \
        -o csv \
        "$TARGET_URL" > "$CSV_FILE" 2>&1 || true

    hey -z "$DURATION" -c "$c" -t "$TIMEOUT" \
        "$TARGET_URL" > "$FULL_OUTPUT" 2>&1 || true

    # Parse the human-readable output
    RPS=$(grep "Requests/sec:" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $2}' || echo "N/A")
    P50=$(grep -E "50%* in" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $3 * 1000}' || echo "N/A")
    P95=$(grep -E "95%* in" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $3 * 1000}' || echo "N/A")
    P99=$(grep -E "99%* in" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $3 * 1000}' || echo "N/A")
    ERRORS=$(grep "Error distribution" -A 10 "$FULL_OUTPUT" 2>/dev/null | grep -v "Error distribution" | head -5 || echo "none")

    printf "  %-5s | %-11s | %-6s | %-8s | %-8s | %-8s | %s\n" \
        "$((${#CONCURRENCY_LEVELS[@]} - ${#CONCURRENCY_LEVELS[@]}))" \
        "$c" "$RPS" "$P50" "$P95" "$P99" "$ERRORS" | tee -a "$SUMMARY"

    echo "--- Full output for c=${c} ---" >> "$FULL_OUTPUT"
    cat "$FULL_OUTPUT" >> "$SUMMARY"
    echo "" | tee -a "$SUMMARY"

    # Breathing room between stages
    echo "Sleeping 10s before next stage..."
    sleep 10
done

echo "" | tee -a "$SUMMARY"
echo "=================================================================" | tee -a "$SUMMARY"
echo "  Done. Raw CSVs: $RESULTS_DIR" | tee -a "$SUMMARY"
echo "  Run: python3 loadtest/analyze.py  to generate plots" | tee -a "$SUMMARY"
echo "=================================================================" | tee -a "$SUMMARY"

echo ""
echo "✅ Load test complete. Check: $SUMMARY"
