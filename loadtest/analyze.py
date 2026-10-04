#!/usr/bin/env python3
"""
TunnelForge Load Test Results Analyzer
=======================================
Reads CSV files produced by `hey -o csv` from the results/ directory
and produces:
  1. A markdown table (results/report.md)
  2. Latency distribution plots (results/latency_plot.png)
  3. Throughput vs. concurrency plot (results/throughput_plot.png)

Usage:
    python3 loadtest/analyze.py
    python3 loadtest/analyze.py --results-dir /path/to/results
"""

import os
import sys
import argparse
import glob
import re
from pathlib import Path
from datetime import datetime

# ─── Optional: matplotlib for plots ───────────────────────────────────────────
try:
    import matplotlib
    matplotlib.use("Agg")  # headless
    import matplotlib.pyplot as plt
    import matplotlib.ticker as mtick
    HAS_MATPLOTLIB = True
except ImportError:
    HAS_MATPLOTLIB = False
    print("[warn] matplotlib not installed — skipping plots. pip3 install matplotlib")

# ─── CSV columns from `hey -o csv` ────────────────────────────────────────────
# response-time,DNS+dialup,DNS,Request-write,Response-delay,Response-read,status-code,offset
COL_LATENCY = 0      # response-time in seconds
COL_STATUS  = 6      # status code


def parse_hey_csv(filepath: str) -> dict:
    """Parse a hey CSV file and return summary stats."""
    latencies = []
    status_counts = {}

    with open(filepath) as f:
        for i, line in enumerate(f):
            line = line.strip()
            if not line or i == 0:  # skip header
                continue
            parts = line.split(",")
            if len(parts) < 7:
                continue
            try:
                lat = float(parts[COL_LATENCY]) * 1000  # convert to ms
                status = int(parts[COL_STATUS])
                latencies.append(lat)
                status_counts[status] = status_counts.get(status, 0) + 1
            except (ValueError, IndexError):
                continue

    if not latencies:
        return {}

    latencies.sort()
    n = len(latencies)

    def pct(p):
        idx = int(p / 100 * n)
        return latencies[min(idx, n - 1)]

    total_req = sum(status_counts.values())
    errors = sum(v for k, v in status_counts.items() if k >= 400 or k == 0)

    return {
        "n": n,
        "mean_ms": sum(latencies) / n,
        "min_ms": latencies[0],
        "max_ms": latencies[-1],
        "p50_ms": pct(50),
        "p75_ms": pct(75),
        "p90_ms": pct(90),
        "p95_ms": pct(95),
        "p99_ms": pct(99),
        "errors": errors,
        "error_pct": errors / total_req * 100 if total_req else 0,
        "status_counts": status_counts,
        "latencies": latencies,
    }


def extract_concurrency(filename: str) -> int:
    """Extract concurrency level from filename like tunnel_c50.csv"""
    m = re.search(r"_c(\d+)", filename)
    return int(m.group(1)) if m else 0


def extract_rps_from_full(full_txt: str) -> float:
    """Parse Requests/sec from the hey full text output file."""
    try:
        with open(full_txt) as f:
            for line in f:
                if "Requests/sec:" in line:
                    return float(line.split(":")[1].strip())
    except Exception:
        pass
    return 0.0


def main():
    parser = argparse.ArgumentParser(description="TunnelForge load test analyzer")
    parser.add_argument("--results-dir", default="loadtest/results",
                        help="Directory containing hey CSV output files")
    args = parser.parse_args()

    results_dir = Path(args.results_dir)
    if not results_dir.exists():
        print(f"[error] Results directory not found: {results_dir}")
        sys.exit(1)

    csv_files = sorted(glob.glob(str(results_dir / "tunnel_c*.csv")),
                       key=lambda p: extract_concurrency(p))

    if not csv_files:
        print(f"[error] No tunnel_c*.csv files found in {results_dir}")
        sys.exit(1)

    rows = []
    for csv_file in csv_files:
        concurrency = extract_concurrency(csv_file)
        stats = parse_hey_csv(csv_file)
        if not stats:
            print(f"[warn] No data in {csv_file}, skipping.")
            continue

        # Try to get Requests/sec from the companion _full.txt
        full_txt = csv_file.replace(".csv", "_full.txt")
        rps = extract_rps_from_full(full_txt)

        rows.append({
            "concurrency": concurrency,
            "rps": rps,
            **stats,
        })
        print(f"  c={concurrency:>4}  rps={rps:>8.1f}  "
              f"p50={stats['p50_ms']:>7.1f}ms  "
              f"p95={stats['p95_ms']:>7.1f}ms  "
              f"p99={stats['p99_ms']:>7.1f}ms  "
              f"errors={stats['error_pct']:.1f}%")

    if not rows:
        print("[error] No valid data rows.")
        sys.exit(1)

    # ── Baseline comparison (if present) ──────────────────────────────────────
    baseline_csv = results_dir / "baseline_c50.csv"
    baseline = None
    if baseline_csv.exists():
        baseline = parse_hey_csv(str(baseline_csv))
        baseline_rps = extract_rps_from_full(
            str(results_dir / "baseline_c50_full.txt"))

    # ── Markdown report ────────────────────────────────────────────────────────
    report_path = results_dir / "report.md"
    with open(report_path, "w") as f:
        f.write(f"# TunnelForge Load Test Report\n\n")
        f.write(f"**Generated**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n\n")

        if baseline:
            f.write("## Baseline (Direct Connection, c=50)\n\n")
            f.write(f"| Metric | Value |\n|--------|-------|\n")
            f.write(f"| Req/sec | {baseline_rps:.1f} |\n")
            f.write(f"| p50 | {baseline['p50_ms']:.1f} ms |\n")
            f.write(f"| p95 | {baseline['p95_ms']:.1f} ms |\n")
            f.write(f"| p99 | {baseline['p99_ms']:.1f} ms |\n")
            f.write(f"| Errors | {baseline['error_pct']:.1f}% |\n\n")

        f.write("## Tunnel Throughput & Latency vs. Concurrency\n\n")
        f.write("| Concurrency | Req/s | p50 (ms) | p75 (ms) | p90 (ms) | "
                "p95 (ms) | p99 (ms) | Max (ms) | Error % |\n")
        f.write("|-------------|-------|----------|----------|----------|"
                "----------|----------|----------|---------|\n")
        for r in rows:
            f.write(f"| {r['concurrency']} | {r['rps']:.1f} | "
                    f"{r['p50_ms']:.1f} | {r['p75_ms']:.1f} | "
                    f"{r['p90_ms']:.1f} | {r['p95_ms']:.1f} | "
                    f"{r['p99_ms']:.1f} | {r['max_ms']:.1f} | "
                    f"{r['error_pct']:.1f}% |\n")

        # Overhead calculation vs baseline
        if baseline and baseline_rps:
            f.write("\n## Tunnel Overhead\n\n")
            for r in rows:
                if r['rps'] > 0:
                    overhead_ms = r['p50_ms'] - baseline['p50_ms']
                    f.write(f"- **c={r['concurrency']}**: "
                            f"+{overhead_ms:.1f} ms p50 overhead vs. direct\n")

        f.write("\n## Status Code Distribution\n\n")
        for r in rows:
            f.write(f"**c={r['concurrency']}**: ")
            f.write(", ".join(f"HTTP {k}: {v}" for k, v in
                              sorted(r['status_counts'].items())))
            f.write("\n\n")

        f.write("\n---\n*Generated by `loadtest/analyze.py`*\n")

    print(f"\n📄 Markdown report: {report_path}")

    # ── Plots ──────────────────────────────────────────────────────────────────
    if not HAS_MATPLOTLIB:
        return

    concurrencies = [r['concurrency'] for r in rows]
    rpss = [r['rps'] for r in rows]
    p50s = [r['p50_ms'] for r in rows]
    p95s = [r['p95_ms'] for r in rows]
    p99s = [r['p99_ms'] for r in rows]
    error_pcts = [r['error_pct'] for r in rows]

    # ── Plot 1: Throughput vs. Concurrency ────────────────────────────────────
    fig, ax = plt.subplots(figsize=(9, 5))
    ax.plot(concurrencies, rpss, "o-", color="#4A90D9", linewidth=2,
            markersize=7, label="Tunnel Req/s")
    if baseline and baseline_rps:
        ax.axhline(baseline_rps, color="#E74C3C", linestyle="--",
                   linewidth=1.5, label=f"Baseline (direct) {baseline_rps:.0f} req/s")
    ax.set_xlabel("Concurrency (workers)", fontsize=12)
    ax.set_ylabel("Requests / second", fontsize=12)
    ax.set_title("TunnelForge: Throughput vs. Concurrency", fontsize=14, fontweight="bold")
    ax.legend()
    ax.grid(True, alpha=0.3)
    plt.tight_layout()
    throughput_plot = results_dir / "throughput_plot.png"
    plt.savefig(throughput_plot, dpi=150)
    plt.close()
    print(f"📊 Throughput plot: {throughput_plot}")

    # ── Plot 2: Latency Percentiles vs. Concurrency ───────────────────────────
    fig, ax = plt.subplots(figsize=(9, 5))
    ax.plot(concurrencies, p50s, "o-", label="p50", color="#2ECC71", linewidth=2)
    ax.plot(concurrencies, p95s, "s-", label="p95", color="#F39C12", linewidth=2)
    ax.plot(concurrencies, p99s, "^-", label="p99", color="#E74C3C", linewidth=2)
    if baseline:
        ax.axhline(baseline['p50_ms'], color="#2ECC71", linestyle=":",
                   alpha=0.5, label=f"Baseline p50 ({baseline['p50_ms']:.0f}ms)")
    ax.set_xlabel("Concurrency (workers)", fontsize=12)
    ax.set_ylabel("Latency (ms)", fontsize=12)
    ax.set_title("TunnelForge: Latency Percentiles vs. Concurrency", fontsize=14, fontweight="bold")
    ax.legend()
    ax.grid(True, alpha=0.3)
    plt.tight_layout()
    latency_plot = results_dir / "latency_plot.png"
    plt.savefig(latency_plot, dpi=150)
    plt.close()
    print(f"📊 Latency plot: {latency_plot}")

    # ── Plot 3: Error Rate vs. Concurrency ───────────────────────────────────
    if any(e > 0 for e in error_pcts):
        fig, ax = plt.subplots(figsize=(9, 4))
        bars = ax.bar(concurrencies, error_pcts, color="#E74C3C", alpha=0.8, width=3)
        ax.set_xlabel("Concurrency", fontsize=12)
        ax.set_ylabel("Error %", fontsize=12)
        ax.set_title("TunnelForge: Error Rate vs. Concurrency", fontsize=14, fontweight="bold")
        ax.yaxis.set_major_formatter(mtick.PercentFormatter())
        ax.grid(True, axis="y", alpha=0.3)
        plt.tight_layout()
        error_plot = results_dir / "error_plot.png"
        plt.savefig(error_plot, dpi=150)
        plt.close()
        print(f"📊 Error plot: {error_plot}")

    print("\n✅ Analysis complete.")


if __name__ == "__main__":
    main()
