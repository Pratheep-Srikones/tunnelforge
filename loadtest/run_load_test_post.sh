#!/usr/bin/env bash
# ==============================================================================
# TunnelForge POST / Webhook Load Test Runner
# ==============================================================================
# PURPOSE:
#   Evaluates TunnelForge's performance and memory safety under realistic
#   HTTP POST webhook traffic carrying JSON payloads.
#
# WHAT THIS TESTS SPECIFICALLY:
#   - Yamux data frame chunking and flow-control window updates
#   - Agent-side request capture buffer (BoundedBuffer / io.TeeReader)
#   - Concurrent body transmission across the persistent tunnel
#
# OUTPUT:
#   Results are stored in: loadtest/post_results/
#   Plots and Markdown report are generated automatically via analyze.py.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --------------------------------------------------------------------------
# Configuration — override via environment variables if needed
# --------------------------------------------------------------------------
TUNNEL_SUBDOMAIN="${TUNNEL_SUBDOMAIN:-loadtest}"
TUNNEL_HOST="${TUNNEL_HOST:-tunnels.pratheepsri.me}"
TARGET_PATH="${TARGET_PATH:-/webhook}"
TARGET_URL="https://${TUNNEL_SUBDOMAIN}.${TUNNEL_HOST}${TARGET_PATH}"

PAYLOAD_FILE="${PAYLOAD_FILE:-${SCRIPT_DIR}/webhook_payload.json}"
RESULTS_DIR="${SCRIPT_DIR}/post_results"

DURATION="${DURATION:-15s}"     # Duration per stage
TIMEOUT="${TIMEOUT:-10}"        # Per-request timeout in seconds

# Concurrency ramp (1 to 200 workers)
CONCURRENCY_LEVELS=(1 5 10 25 50 75 100 200)

# --------------------------------------------------------------------------
# Validation
# --------------------------------------------------------------------------
if [[ ! -f "$PAYLOAD_FILE" ]]; then
    echo "❌ Error: Payload file not found at $PAYLOAD_FILE"
    exit 1
fi

PAYLOAD_SIZE=$(wc -c < "$PAYLOAD_FILE" | tr -d ' ')

mkdir -p "$RESULTS_DIR"
SUMMARY="${RESULTS_DIR}/summary.txt"

echo "=================================================================" | tee "$SUMMARY"
echo "  TunnelForge POST / Webhook Load Test — $(date)" | tee -a "$SUMMARY"
echo "  Target URL : $TARGET_URL" | tee -a "$SUMMARY"
echo "  Method     : POST" | tee -a "$SUMMARY"
echo "  Payload    : $PAYLOAD_FILE (${PAYLOAD_SIZE} bytes JSON)" | tee -a "$SUMMARY"
echo "  Duration   : ${DURATION} per stage" | tee -a "$SUMMARY"
echo "  Results Dir: $RESULTS_DIR" | tee -a "$SUMMARY"
echo "=================================================================" | tee -a "$SUMMARY"
echo "" | tee -a "$SUMMARY"

# Print Table Header
printf "%-13s | %-10s | %-10s | %-10s | %-10s | %s\n" \
    "Concurrency" "Req/s" "p50 (ms)" "p95 (ms)" "p99 (ms)" "Errors / Status" | tee -a "$SUMMARY"
echo "--------------|------------|------------|------------|------------|-----------------------" | tee -a "$SUMMARY"

# --------------------------------------------------------------------------
# Run Concurrency Stages
# --------------------------------------------------------------------------
for c in "${CONCURRENCY_LEVELS[@]}"; do
    echo ""
    echo ">>> Running POST Stage: concurrency=$c, duration=$DURATION"

    CSV_FILE="${RESULTS_DIR}/tunnel_c${c}.csv"
    FULL_OUTPUT="${RESULTS_DIR}/tunnel_c${c}_full.txt"

    # 1. Capture raw CSV data for analyzer
    hey -z "$DURATION" -c "$c" -t "$TIMEOUT" \
        -m POST \
        -D "$PAYLOAD_FILE" \
        -H "Content-Type: application/json" \
        -o csv \
        "$TARGET_URL" > "$CSV_FILE" 2>&1 || true

    # 2. Capture human-readable output
    hey -z "$DURATION" -c "$c" -t "$TIMEOUT" \
        -m POST \
        -D "$PAYLOAD_FILE" \
        -H "Content-Type: application/json" \
        "$TARGET_URL" > "$FULL_OUTPUT" 2>&1 || true

    # 3. Parse stats from output
    RPS=$(grep "Requests/sec:" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $2}' || echo "N/A")
    P50=$(grep -E "50%* in" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $3 * 1000}' || echo "N/A")
    P95=$(grep -E "95%* in" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $3 * 1000}' || echo "N/A")
    P99=$(grep -E "99%* in" "$FULL_OUTPUT" 2>/dev/null | awk '{printf "%.1f", $3 * 1000}' || echo "N/A")

    ERRORS=$(grep "Error distribution" -A 10 "$FULL_OUTPUT" 2>/dev/null | grep -v "Error distribution" | tr '\n' '; ' || true)
    if [[ -z "$ERRORS" ]]; then
        ERRORS="0 errors"
    fi

    printf "%-13s | %-10s | %-10s | %-10s | %-10s | %s\n" \
        "$c" "$RPS" "$P50" "$P95" "$P99" "$ERRORS" | tee -a "$SUMMARY"

    echo "--- Raw Output c=${c} ---" >> "$FULL_OUTPUT"
    cat "$FULL_OUTPUT" >> "$SUMMARY"
    echo "" | tee -a "$SUMMARY"

    echo "Cooldown: sleeping 10s before next stage..."
    sleep 10
done

echo "" | tee -a "$SUMMARY"
echo "=================================================================" | tee -a "$SUMMARY"
echo "  POST Load Test Complete! Generating Report & Plots..." | tee -a "$SUMMARY"
echo "=================================================================" | tee -a "$SUMMARY"

# Run automated analysis on post_results
if command -v python3 &>/dev/null; then
    python3 "${SCRIPT_DIR}/analyze.py" --results-dir "$RESULTS_DIR" || true
elif command -v python &>/dev/null; then
    python "${SCRIPT_DIR}/analyze.py" --results-dir "$RESULTS_DIR" || true
fi

echo ""
echo "✅ Done! Full report available at:"
echo "   $RESULTS_DIR/report.md"
echo "   $RESULTS_DIR/throughput_plot.png"
echo "   $RESULTS_DIR/latency_plot.png"
