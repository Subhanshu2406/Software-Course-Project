#!/usr/bin/env python3
"""
Generate publication-quality plots for IEEE report from experiment data.

Usage:
    pip install matplotlib numpy pandas
    python plots/generate_plots.py

Reads:  results/comprehensive_metrics.csv
Writes: plots/*.pdf  (vector) and plots/*.png (raster 300 DPI)
"""

import os
import re
import sys

import matplotlib
matplotlib.use("Agg")  # non-interactive backend
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import numpy as np
import pandas as pd

# ── Paths ────────────────────────────────────────────────────────────
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.dirname(SCRIPT_DIR)
CSV_PATH = os.path.join(PROJECT_ROOT, "results", "comprehensive_metrics.csv")
OUT_DIR = os.path.join(PROJECT_ROOT, "plots")
os.makedirs(OUT_DIR, exist_ok=True)

# ── Style ────────────────────────────────────────────────────────────
plt.rcParams.update({
    "font.family": "serif",
    "font.size": 10,
    "axes.titlesize": 11,
    "axes.labelsize": 10,
    "xtick.labelsize": 9,
    "ytick.labelsize": 9,
    "legend.fontsize": 8.5,
    "figure.dpi": 150,
    "savefig.dpi": 300,
    "savefig.bbox": "tight",
    "axes.grid": True,
    "grid.alpha": 0.3,
    "lines.linewidth": 1.8,
    "lines.markersize": 6,
})

COLORS = {
    "single": "#2563eb",   # blue
    "cross":  "#dc2626",   # red
    "mixed":  "#16a34a",   # green
    "q1":     "#f59e0b",   # amber
    "q2":     "#2563eb",   # blue
    "q3":     "#7c3aed",   # purple
    "bar1":   "#3b82f6",
    "bar2":   "#ef4444",
    "bar3":   "#22c55e",
    "accent": "#6366f1",
}

def save(fig, name):
    """Save figure as both PDF (vector) and PNG (raster)."""
    fig.savefig(os.path.join(OUT_DIR, f"{name}.pdf"), format="pdf")
    fig.savefig(os.path.join(OUT_DIR, f"{name}.png"), format="png")
    plt.close(fig)
    print(f"  -> {name}.pdf / .png")


# ── Load data ────────────────────────────────────────────────────────
df = pd.read_csv(CSV_PATH)

# =====================================================================
#  PLOT 1: Throughput Scalability (TPS vs Virtual Users)
# =====================================================================
def plot_throughput_scaling():
    ts = df[df["experiment"] == "throughput_scaling"]
    fig, ax = plt.subplots(figsize=(6, 4))

    for variant, color, marker, label in [
        ("single_shard", COLORS["single"], "o", "Single-Shard"),
        ("cross_shard",  COLORS["cross"],  "s", "Cross-Shard (2PC)"),
        ("mixed",        COLORS["mixed"],  "^", "Mixed (70/30)"),
    ]:
        sub = ts[ts["variant"] == variant].sort_values("vus")
        ax.plot(sub["vus"], sub["tps"], color=color, marker=marker, label=label)

    # Annotate saturation region
    ax.axhspan(110, 220, alpha=0.06, color="gray")
    ax.annotate("Saturation\nregion", xy=(160, 130), fontsize=8,
                color="gray", ha="center", style="italic")

    ax.set_xlabel("Virtual Users (VUs)")
    ax.set_ylabel("Throughput (TPS)")
    ax.set_title("Throughput Scalability — Single vs Cross-Shard vs Mixed")
    ax.legend(loc="lower right")
    ax.set_xlim(0, 210)
    ax.set_ylim(0, 230)
    ax.xaxis.set_major_locator(ticker.MultipleLocator(25))
    save(fig, "fig1_throughput_scaling")


# =====================================================================
#  PLOT 2: Latency Degradation Under Load
# =====================================================================
def plot_latency_degradation():
    ts = df[(df["experiment"] == "throughput_scaling") &
            (df["variant"] == "single_shard")].sort_values("vus")

    fig, ax = plt.subplots(figsize=(6, 4))
    ax.plot(ts["vus"], ts["avg_latency_ms"], "o-", color=COLORS["single"],
            label="Average")
    ax.plot(ts["vus"], ts["p95_latency_ms"], "s--", color=COLORS["cross"],
            label="P95")
    ax.plot(ts["vus"], ts["p99_latency_ms"], "^:", color=COLORS["mixed"],
            label="P99")

    # SLA line
    ax.axhline(y=2000, color="red", linestyle="-.", linewidth=1, alpha=0.5)
    ax.annotate("SLA: p99 < 2 s", xy=(20, 2100), fontsize=8, color="red")

    ax.set_xlabel("Virtual Users (VUs)")
    ax.set_ylabel("Latency (ms)")
    ax.set_title("Single-Shard Latency Degradation Under Increasing Load")
    ax.legend(loc="upper left")
    ax.set_xlim(0, 210)
    ax.set_ylim(0, 5000)
    save(fig, "fig2_latency_degradation")


# =====================================================================
#  PLOT 3: 2PC Overhead — Single vs Cross-Shard Comparison
# =====================================================================
def plot_2pc_overhead():
    ov = df[df["experiment"] == "twopc_overhead"]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(8, 3.5))

    # 3a: Latency comparison at 50 and 100 VUs
    vu_levels = sorted(ov["vus"].unique())
    x = np.arange(len(vu_levels))
    width = 0.3

    single_lat = [ov[(ov["vus"] == v) & (ov["variant"] == "single_shard")]["avg_latency_ms"].values[0]
                  for v in vu_levels]
    cross_lat = [ov[(ov["vus"] == v) & (ov["variant"] == "cross_shard")]["avg_latency_ms"].values[0]
                 for v in vu_levels]

    bars1 = ax1.bar(x - width/2, single_lat, width, label="Single-Shard",
                    color=COLORS["bar1"], edgecolor="white")
    bars2 = ax1.bar(x + width/2, cross_lat, width, label="Cross-Shard (2PC)",
                    color=COLORS["bar2"], edgecolor="white")

    # Annotate overhead percentage
    for i, (s, c) in enumerate(zip(single_lat, cross_lat)):
        pct = ((c - s) / s) * 100
        ax1.annotate(f"+{pct:.0f}%", xy=(x[i] + width/2, c + 15),
                     ha="center", fontsize=8, fontweight="bold", color=COLORS["bar2"])

    ax1.set_xticks(x)
    ax1.set_xticklabels([f"{v} VUs" for v in vu_levels])
    ax1.set_ylabel("Avg Latency (ms)")
    ax1.set_title("(a) Latency Overhead")
    ax1.legend(fontsize=8)

    # 3b: TPS comparison
    single_tps = [ov[(ov["vus"] == v) & (ov["variant"] == "single_shard")]["tps"].values[0]
                  for v in vu_levels]
    cross_tps = [ov[(ov["vus"] == v) & (ov["variant"] == "cross_shard")]["tps"].values[0]
                 for v in vu_levels]

    bars3 = ax2.bar(x - width/2, single_tps, width, label="Single-Shard",
                    color=COLORS["bar1"], edgecolor="white")
    bars4 = ax2.bar(x + width/2, cross_tps, width, label="Cross-Shard (2PC)",
                    color=COLORS["bar2"], edgecolor="white")

    for i, (s, c) in enumerate(zip(single_tps, cross_tps)):
        pct = ((s - c) / s) * 100
        ax2.annotate(f"-{pct:.0f}%", xy=(x[i] + width/2, c + 2),
                     ha="center", fontsize=8, fontweight="bold", color=COLORS["bar2"])

    ax2.set_xticks(x)
    ax2.set_xticklabels([f"{v} VUs" for v in vu_levels])
    ax2.set_ylabel("Throughput (TPS)")
    ax2.set_title("(b) Throughput Impact")
    ax2.legend(fontsize=8)

    fig.suptitle("Two-Phase Commit (2PC) Overhead Analysis", fontsize=11, y=1.02)
    fig.tight_layout()
    save(fig, "fig3_2pc_overhead")


# =====================================================================
#  PLOT 4: Fault Tolerance — TPS During Follower Failure
# =====================================================================
def plot_fault_tolerance():
    ft = df[(df["experiment"] == "fault_tolerance") &
            (df["variant"].str.startswith("follower_kill"))]

    fig, ax = plt.subplots(figsize=(6, 3.5))

    phases = ["Before\nFault", "During\nFault", "After\nRecovery"]
    tps_vals = ft["tps"].values
    colors = [COLORS["bar3"], COLORS["bar2"], COLORS["bar1"]]

    bars = ax.bar(phases, tps_vals, color=colors, edgecolor="white", width=0.5)

    # Annotate TPS values and drop percentage
    for i, (bar, val) in enumerate(zip(bars, tps_vals)):
        ax.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 1.5,
                f"{val:.1f}", ha="center", fontsize=9, fontweight="bold")

    # Drop annotation
    drop_pct = ((tps_vals[0] - tps_vals[1]) / tps_vals[0]) * 100
    recovery_pct = (tps_vals[2] / tps_vals[0]) * 100
    ax.annotate(f"Drop: {drop_pct:.1f}%",
                xy=(1, tps_vals[1]/2), fontsize=9, color="white",
                ha="center", fontweight="bold")
    ax.annotate(f"{recovery_pct:.1f}% of\nbaseline",
                xy=(2, tps_vals[2]/2), fontsize=9, color="white",
                ha="center", fontweight="bold")

    ax.set_ylabel("Throughput (TPS)")
    ax.set_title("Fault Tolerance: TPS During Follower Failure (Quorum = 2)")
    ax.set_ylim(0, 110)

    # Add recovery time annotations for leader/coordinator
    ax.text(0.98, 0.02,
            "Leader failover: 8.2 s\nCoordinator recovery: 6.5 s",
            transform=ax.transAxes, fontsize=8, ha="right", va="bottom",
            bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow",
                      edgecolor="gray", alpha=0.8))

    save(fig, "fig4_fault_tolerance")


# =====================================================================
#  PLOT 5: WAL Recovery Time vs Log Size
# =====================================================================
def plot_wal_recovery():
    wal = df[df["experiment"] == "wal_recovery"].copy()

    # Parse recovery_time_ms and throughput from notes
    wal["wal_entries"] = wal["total_requests"]
    wal["recovery_ms"] = wal["notes"].apply(
        lambda n: float(re.search(r"recovery_time_ms=(\d+\.?\d*)", n).group(1))
        if re.search(r"recovery_time_ms=", n) else 0
    )

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(8, 3.5))

    # 5a: Recovery time vs WAL entries
    ax1.plot(wal["wal_entries"], wal["recovery_ms"], "o-",
             color=COLORS["accent"], linewidth=2, markersize=7)
    ax1.fill_between(wal["wal_entries"], 0, wal["recovery_ms"],
                     alpha=0.1, color=COLORS["accent"])
    ax1.set_xlabel("WAL Entries")
    ax1.set_ylabel("Recovery Time (ms)")
    ax1.set_title("(a) Recovery Time vs Log Size")
    ax1.set_xscale("log")
    ax1.set_yscale("log")

    # Annotate sub-second for 1000 entries
    ax1.annotate("342 ms for\n1K entries",
                 xy=(1000, 342), xytext=(3000, 120),
                 arrowprops=dict(arrowstyle="->", color="gray"),
                 fontsize=8, color="gray")

    # 5b: Throughput (entries/sec) — shows near-linear scaling benefit
    throughput = wal["wal_entries"] / (wal["recovery_ms"] / 1000)
    ax2.bar(range(len(wal)), throughput, color=COLORS["accent"],
            edgecolor="white", alpha=0.8)
    ax2.set_xticks(range(len(wal)))
    ax2.set_xticklabels([f"{int(e):,}" for e in wal["wal_entries"]],
                        rotation=45, ha="right", fontsize=8)
    ax2.set_xlabel("WAL Entries")
    ax2.set_ylabel("Replay Throughput (entries/s)")
    ax2.set_title("(b) Replay Throughput")

    fig.suptitle("Write-Ahead Log Recovery Performance", fontsize=11, y=1.02)
    fig.tight_layout()
    save(fig, "fig5_wal_recovery")


# =====================================================================
#  PLOT 6: Replication Overhead — Quorum Size Comparison
# =====================================================================
def plot_replication_overhead():
    rp = df[df["experiment"] == "replication_overhead"]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(8, 3.5))

    labels = ["No Replication\n(Q=1)", "Quorum = 2\n(1 ACK)", "Quorum = 3\n(2 ACKs)"]
    tps_vals = rp["tps"].values
    lat_vals = rp["avg_latency_ms"].values
    colors = [COLORS["q1"], COLORS["q2"], COLORS["q3"]]

    # 6a: TPS comparison
    bars1 = ax1.bar(labels, tps_vals, color=colors, edgecolor="white", width=0.5)
    for bar, val in zip(bars1, tps_vals):
        ax1.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 2,
                 f"{val:.1f}", ha="center", fontsize=9, fontweight="bold")
    ax1.set_ylabel("Throughput (TPS)")
    ax1.set_title("(a) TPS by Quorum Size")
    ax1.set_ylim(0, 170)

    # 6b: Latency comparison
    bars2 = ax2.bar(labels, lat_vals, color=colors, edgecolor="white", width=0.5)
    for bar, val in zip(bars2, lat_vals):
        ax2.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 10,
                 f"{val:.0f} ms", ha="center", fontsize=9, fontweight="bold")
    ax2.set_ylabel("Average Latency (ms)")
    ax2.set_title("(b) Latency by Quorum Size")
    ax2.set_ylim(0, 850)

    # Annotate overhead
    overhead_q2 = ((lat_vals[1] - lat_vals[0]) / lat_vals[0]) * 100
    overhead_q3 = ((lat_vals[2] - lat_vals[0]) / lat_vals[0]) * 100
    ax2.annotate(f"+{overhead_q2:.0f}%", xy=(1, lat_vals[1] + 55),
                 ha="center", fontsize=8, color=COLORS["q2"], fontweight="bold")
    ax2.annotate(f"+{overhead_q3:.0f}%", xy=(2, lat_vals[2] + 55),
                 ha="center", fontsize=8, color=COLORS["q3"], fontweight="bold")

    fig.suptitle("Replication Overhead: Cost of Durability Guarantees", fontsize=11, y=1.02)
    fig.tight_layout()
    save(fig, "fig6_replication_overhead")


# =====================================================================
#  PLOT 7: Partition Distribution (SHA-256 Uniformity)
# =====================================================================
def plot_partition_distribution():
    pd_data = df[df["experiment"] == "partition_distribution"].copy()
    pd_data["partition"] = pd_data["notes"].apply(
        lambda n: int(re.search(r"partition=(\d+)", n).group(1))
    )
    pd_data["account_count"] = pd_data["notes"].apply(
        lambda n: int(re.search(r"account_count=(\d+)", n).group(1))
    )
    pd_data = pd_data.sort_values("partition")

    fig, ax = plt.subplots(figsize=(8, 3.5))

    partitions = pd_data["partition"].values
    counts = pd_data["account_count"].values
    ideal = 1000 / 30  # 33.33

    colors_list = []
    for p in partitions:
        if p < 10:
            colors_list.append(COLORS["single"])
        elif p < 20:
            colors_list.append(COLORS["cross"])
        else:
            colors_list.append(COLORS["mixed"])

    ax.bar(partitions, counts, color=colors_list, edgecolor="white", alpha=0.85)
    ax.axhline(y=ideal, color="red", linestyle="--", linewidth=1.2, label=f"Ideal: {ideal:.1f}")

    # Std dev annotation
    std = np.std(counts)
    cv = std / np.mean(counts) * 100
    ax.text(0.98, 0.95,
            f"$\\sigma$ = {std:.1f}  |  CV = {cv:.1f}%",
            transform=ax.transAxes, fontsize=9, ha="right", va="top",
            bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow",
                      edgecolor="gray", alpha=0.8))

    # Shard labels
    for start, label in [(0, "Shard 1\n(P0–P9)"), (10, "Shard 2\n(P10–P19)"), (20, "Shard 3\n(P20–P29)")]:
        ax.text(start + 4.5, max(counts) + 2, label, ha="center", fontsize=8, style="italic")

    ax.set_xlabel("Partition ID")
    ax.set_ylabel("Account Count")
    ax.set_title("SHA-256 Partition Distribution — 1,000 Accounts Across 30 Partitions")
    ax.legend(loc="lower right")
    ax.set_xlim(-0.5, 29.5)
    ax.set_ylim(0, max(counts) + 6)
    ax.xaxis.set_major_locator(ticker.MultipleLocator(1))
    save(fig, "fig7_partition_distribution")


# =====================================================================
#  PLOT 8: Latency Breakdown (Stacked Components)
# =====================================================================
def plot_latency_breakdown():
    lb = df[df["experiment"] == "latency_breakdown"]
    lb = lb[lb["variant"] != "total_measured"]  # exclude total

    fig, ax = plt.subplots(figsize=(6, 4))

    components = lb["variant"].values
    values = lb["avg_latency_ms"].values
    total = values.sum()

    # Nice labels
    label_map = {
        "coordinator_routing": "Coord. Routing\n(SHA-256 + map)",
        "wal_fsync":           "WAL fsync\n(2× per txn)",
        "replication":         "Replication\n(quorum ACK)",
        "ledger_apply":        "Ledger Apply\n(in-memory)",
        "network_overhead":    "Network\n(HTTP RTT)",
    }
    labels = [label_map.get(c, c) for c in components]

    pie_colors = ["#3b82f6", "#ef4444", "#f59e0b", "#22c55e", "#8b5cf6"]

    wedges, texts, autotexts = ax.pie(
        values, labels=labels, colors=pie_colors, autopct="%1.1f%%",
        startangle=90, pctdistance=0.75,
        wedgeprops=dict(width=0.5, edgecolor="white", linewidth=2),
    )
    for t in autotexts:
        t.set_fontsize(8)
        t.set_fontweight("bold")

    # Center text with total
    # Queueing contributes: total_measured(541) - sum_of_components
    queueing = 541 - total
    ax.text(0, 0, f"Total: 541 ms\nQueueing: {queueing:.0f} ms",
            ha="center", va="center", fontsize=9, fontweight="bold")

    ax.set_title("Transaction Latency Breakdown (50 VUs, Single-Shard)")
    save(fig, "fig8_latency_breakdown")


# =====================================================================
#  PLOT 9: Error Rate vs Load (Reliability Curve)
# =====================================================================
def plot_error_rate():
    ts = df[(df["experiment"] == "throughput_scaling")].copy()

    fig, ax1 = plt.subplots(figsize=(6, 4))
    ax2 = ax1.twinx()

    for variant, color, marker, label in [
        ("single_shard", COLORS["single"], "o", "Single-Shard"),
        ("cross_shard",  COLORS["cross"],  "s", "Cross-Shard"),
        ("mixed",        COLORS["mixed"],  "^", "Mixed"),
    ]:
        sub = ts[ts["variant"] == variant].sort_values("vus")
        ax1.plot(sub["vus"], sub["error_rate_pct"], color=color,
                 marker=marker, label=label, linewidth=2)

    # SLA line
    ax1.axhline(y=5.0, color="red", linestyle="-.", linewidth=1, alpha=0.6)
    ax1.annotate("SLA: < 5% errors", xy=(30, 5.2), fontsize=8, color="red")

    # TPS on secondary axis (single-shard only)
    ss = ts[ts["variant"] == "single_shard"].sort_values("vus")
    ax2.plot(ss["vus"], ss["tps"], color="gray", linestyle=":", alpha=0.5,
             label="TPS (single)")
    ax2.set_ylabel("TPS (dashed)", color="gray", alpha=0.7)
    ax2.tick_params(axis="y", labelcolor="gray")

    ax1.set_xlabel("Virtual Users (VUs)")
    ax1.set_ylabel("Error Rate (%)")
    ax1.set_title("System Reliability Under Increasing Load")
    ax1.legend(loc="upper left")
    ax1.set_xlim(0, 210)
    ax1.set_ylim(-0.5, 7)
    save(fig, "fig9_error_rate")


# ═══════════════════════════════════════════════════════════════════
#  Main
# ═══════════════════════════════════════════════════════════════════
if __name__ == "__main__":
    print(f"Reading data from: {CSV_PATH}")
    print(f"Output directory:  {OUT_DIR}\n")

    print("Generating plots:")
    plot_throughput_scaling()
    plot_latency_degradation()
    plot_2pc_overhead()
    plot_fault_tolerance()
    plot_wal_recovery()
    plot_replication_overhead()
    plot_partition_distribution()
    plot_latency_breakdown()
    plot_error_rate()

    print(f"\nDone! {9} plots saved to {OUT_DIR}/")
    print("Formats: PDF (vector, for LaTeX) + PNG (raster, 300 DPI)")
