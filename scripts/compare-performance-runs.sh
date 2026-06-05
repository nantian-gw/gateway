#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage:
  scripts/compare-performance-runs.sh <baseline-run-dir> <current-run-dir> [output-dir]

Compares archived performance evidence and writes:
  - summary.md: operator-facing comparison summary
  - index.json: machine-readable comparison index

Threshold environment variables:
  LATENCY_REGRESSION_PCT     default: 20
  LATENCY_REGRESSION_MIN_ABS_MS default: 0.1
  RPS_REGRESSION_PCT         default: 20
  SUCCESS_RATE_DROP_PCT      default: 1
  RESOURCE_REGRESSION_PCT    default: 20
  RESOURCE_RSS_REGRESSION_MIN_ABS_KIB default: 4096
  RESOURCE_FD_THREAD_REGRESSION_MIN_ABS default: 1
  RESOURCE_CPU_TICK_REGRESSION_MIN_ABS default: 5
  CPU_REGRESSION_PCT         default: 20
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "$#" -lt 2 || "$#" -gt 3 ]]; then
  usage >&2
  exit 2
fi

BASELINE_DIR="$1"
CURRENT_DIR="$2"
if [[ "$#" -eq 3 ]]; then
  OUTPUT_DIR="$3"
else
  baseline_name="$(basename "${BASELINE_DIR}")"
  current_name="$(basename "${CURRENT_DIR}")"
  OUTPUT_DIR="${ROOT_DIR}/reports/performance/comparisons/${baseline_name}--${current_name}"
fi

export BASELINE_DIR CURRENT_DIR OUTPUT_DIR
export LATENCY_REGRESSION_PCT="${LATENCY_REGRESSION_PCT:-20}"
export LATENCY_REGRESSION_MIN_ABS_MS="${LATENCY_REGRESSION_MIN_ABS_MS:-0.1}"
export RPS_REGRESSION_PCT="${RPS_REGRESSION_PCT:-20}"
export SUCCESS_RATE_DROP_PCT="${SUCCESS_RATE_DROP_PCT:-1}"
export RESOURCE_REGRESSION_PCT="${RESOURCE_REGRESSION_PCT:-20}"
export RESOURCE_RSS_REGRESSION_MIN_ABS_KIB="${RESOURCE_RSS_REGRESSION_MIN_ABS_KIB:-4096}"
export RESOURCE_FD_THREAD_REGRESSION_MIN_ABS="${RESOURCE_FD_THREAD_REGRESSION_MIN_ABS:-1}"
export RESOURCE_CPU_TICK_REGRESSION_MIN_ABS="${RESOURCE_CPU_TICK_REGRESSION_MIN_ABS:-5}"
export CPU_REGRESSION_PCT="${CPU_REGRESSION_PCT:-20}"

python3 <<'PY'
import json
import math
import os
import re
import sys
from datetime import datetime, timezone
from pathlib import Path


baseline_dir = Path(os.environ["BASELINE_DIR"])
current_dir = Path(os.environ["CURRENT_DIR"])
output_dir = Path(os.environ["OUTPUT_DIR"])

thresholds = {
    "latency_pct": float(os.environ["LATENCY_REGRESSION_PCT"]),
    "latency_min_abs_ms": float(os.environ["LATENCY_REGRESSION_MIN_ABS_MS"]),
    "rps_pct": float(os.environ["RPS_REGRESSION_PCT"]),
    "success_rate_drop_pct": float(os.environ["SUCCESS_RATE_DROP_PCT"]),
    "resource_pct": float(os.environ["RESOURCE_REGRESSION_PCT"]),
    "resource_rss_min_abs_kib": float(os.environ["RESOURCE_RSS_REGRESSION_MIN_ABS_KIB"]),
    "resource_fd_thread_min_abs": float(os.environ["RESOURCE_FD_THREAD_REGRESSION_MIN_ABS"]),
    "resource_cpu_tick_min_abs": float(os.environ["RESOURCE_CPU_TICK_REGRESSION_MIN_ABS"]),
    "cpu_pct": float(os.environ["CPU_REGRESSION_PCT"]),
}


def require_dir(path: Path, label: str) -> None:
    if not path.is_dir():
        print(f"{label} does not exist or is not a directory: {path}", file=sys.stderr)
        sys.exit(2)


def parse_number(raw: str):
    value = raw.strip().replace(",", "")
    if not value or value == "<not":
        return None
    value = value.rstrip("%")
    try:
        return float(value)
    except ValueError:
        return None


def parse_kind_summary(path: Path) -> dict:
    summary = path / "summary.md"
    if not summary.is_file():
        return {}

    metrics = {}
    section = None
    headers = None
    for line in summary.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if stripped == "## HTTP Profiles":
            section = "kind_http"
            headers = None
            continue
        if stripped == "## gRPC Profile":
            section = "kind_grpc"
            headers = None
            continue
        if stripped.startswith("## ") and stripped not in ("## HTTP Profiles", "## gRPC Profile"):
            section = None
            headers = None
            continue
        if section is None or not stripped.startswith("|"):
            continue

        cells = [cell.strip() for cell in stripped.strip("|").split("|")]
        if not cells:
            continue
        if cells[0] == "---" or set(cells[0]) <= {"-", ":"}:
            continue
        if cells[0] == "Profile":
            headers = cells
            continue
        if not headers or len(cells) != len(headers):
            continue

        row = dict(zip(headers, cells))
        profile = row.get("Profile", "").strip()
        if not profile:
            continue
        field_map = {
            "Success Rate": "success_rate_pct",
            "p95 ms": "p95_ms",
            "p99 ms": "p99_ms",
            "Max ms": "max_ms",
            "Achieved RPS": "rps",
        }
        for source, suffix in field_map.items():
            value = parse_number(row.get(source, ""))
            if value is not None:
                metrics[f"{section}.{profile}.{suffix}"] = value
    return metrics


def parse_bench_json(path: Path) -> dict:
    bench = path / "bench.json"
    if not bench.is_file():
        return {}
    data = json.loads(bench.read_text(encoding="utf-8"))
    metrics = {}
    for scenario in data.get("scenarios", []):
        name = scenario.get("name")
        if not name:
            continue
        for key, value in scenario.get("timing", {}).items():
            if isinstance(value, (int, float)):
                metrics[f"bench.{name}.timing.{key}"] = float(value)
        for key, value in scenario.get("resource_delta", {}).items():
            if isinstance(value, (int, float)):
                metrics[f"bench.{name}.resource_delta.{key}"] = float(value)
    return metrics


PERF_LINE = re.compile(r"^\s*([0-9][0-9,]*(?:\.[0-9]+)?)\s+(?:(msec|seconds)\s+)?([A-Za-z0-9_.:-]+)\b")


def parse_perf_stat(path: Path) -> dict:
    perf = path / "perf-stat.txt"
    if not perf.is_file():
        return {}
    metrics = {}
    for line in perf.read_text(encoding="utf-8", errors="replace").splitlines():
        if "<not supported>" in line:
            continue
        match = PERF_LINE.match(line)
        if not match:
            continue
        raw_value, unit, event = match.groups()
        value = parse_number(raw_value)
        if value is None:
            continue
        if unit == "seconds":
            value *= 1000
            event = f"{event}_msec"
        metrics[f"perf.{event}"] = float(value)
    return metrics


def collect_metrics(path: Path) -> dict:
    metrics = {}
    metrics.update(parse_kind_summary(path))
    metrics.update(parse_bench_json(path))
    metrics.update(parse_perf_stat(path))
    return metrics


def metric_policy(name: str):
    if name.endswith(".success_rate_pct"):
        return "higher", thresholds["success_rate_drop_pct"], "absolute_drop"
    if name.endswith(".rps"):
        return "higher", thresholds["rps_pct"], "percent"
    if ".resource_delta." in name:
        return "lower", thresholds["resource_pct"], "percent"
    if name.startswith("perf."):
        return "lower", thresholds["cpu_pct"], "percent"
    return "lower", thresholds["latency_pct"], "percent"


def min_abs_delta_for(name: str):
    if ".timing." in name:
        return thresholds["latency_min_abs_ms"]
    if name.endswith(".resource_delta.rss_kib"):
        return thresholds["resource_rss_min_abs_kib"]
    if name.endswith(".resource_delta.fd_count") or name.endswith(".resource_delta.threads"):
        return thresholds["resource_fd_thread_min_abs"]
    if name.endswith(".resource_delta.cpu_user_ticks") or name.endswith(".resource_delta.cpu_system_ticks"):
        return thresholds["resource_cpu_tick_min_abs"]
    return None


def percent_delta(baseline: float, current: float):
    if baseline == 0:
        if current == 0:
            return 0.0
        return None
    return ((current - baseline) / abs(baseline)) * 100


def status_for(name: str, baseline: float, current: float):
    direction, threshold, mode = metric_policy(name)
    delta = percent_delta(baseline, current)
    min_abs_delta = min_abs_delta_for(name)
    if min_abs_delta is not None and abs(current - baseline) <= min_abs_delta:
        return delta, threshold, False
    if mode == "absolute_drop":
        drop = baseline - current
        regressed = drop > threshold
        return delta, threshold, regressed
    if direction == "lower":
        regressed = current > baseline if delta is None else delta > threshold
    else:
        regressed = current < baseline if delta is None else delta < -threshold
    return delta, threshold, regressed


def format_value(value):
    if value is None:
        return "n/a"
    if isinstance(value, float) and math.isinf(value):
        return "inf"
    if abs(value) >= 100:
        return f"{value:.2f}"
    return f"{value:.4f}".rstrip("0").rstrip(".")


def write_summary(records, regressions):
    lines = [
        "# Performance Run Comparison",
        "",
        f"- Baseline: `{baseline_dir}`",
        f"- Current: `{current_dir}`",
        f"- Result: `{'regressed' if regressions else 'passed'}`",
        "",
        "## Thresholds",
        "",
        f"- Latency regression: `{thresholds['latency_pct']}%`",
        f"- Latency noise floor: `{thresholds['latency_min_abs_ms']} ms` absolute delta",
        f"- RPS regression: `{thresholds['rps_pct']}%` drop",
        f"- Success rate regression: `{thresholds['success_rate_drop_pct']}` percentage point drop",
        f"- Resource regression: `{thresholds['resource_pct']}%`",
        f"- RSS noise floor: `{thresholds['resource_rss_min_abs_kib']} KiB` absolute delta",
        f"- FD/thread noise floor: `{thresholds['resource_fd_thread_min_abs']}` absolute delta",
        f"- CPU tick noise floor: `{thresholds['resource_cpu_tick_min_abs']}` absolute delta",
        f"- CPU/perf-stat regression: `{thresholds['cpu_pct']}%`",
        "",
        "## Metric Comparison",
        "",
        "| Metric | Baseline | Current | Delta % | Threshold | Status |",
        "| --- | ---: | ---: | ---: | ---: | --- |",
    ]
    for record in records:
        lines.append(
            "| {name} | {baseline} | {current} | {delta} | {threshold} | {status} |".format(
                name=record["name"],
                baseline=format_value(record["baseline"]),
                current=format_value(record["current"]),
                delta=format_value(record["delta_pct"]),
                threshold=format_value(record["threshold"]),
                status=record["status"],
            )
        )
    if regressions:
        lines.extend(
            [
                "",
                "## Regression Summary",
                "",
            ]
        )
        for record in regressions:
            lines.append(
                "- `{name}` regressed: baseline `{baseline}`, current `{current}`, delta `{delta}%`, threshold `{threshold}`.".format(
                    name=record["name"],
                    baseline=format_value(record["baseline"]),
                    current=format_value(record["current"]),
                    delta=format_value(record["delta_pct"]),
                    threshold=format_value(record["threshold"]),
                )
            )
    else:
        lines.extend(["", "## Regression Summary", "", "- No metrics exceeded configured thresholds."])
    (output_dir / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def main():
    require_dir(baseline_dir, "baseline run directory")
    require_dir(current_dir, "current run directory")
    output_dir.mkdir(parents=True, exist_ok=True)

    baseline_metrics = collect_metrics(baseline_dir)
    current_metrics = collect_metrics(current_dir)
    names = sorted(set(baseline_metrics) & set(current_metrics))
    if not names:
        print("no comparable performance metrics found", file=sys.stderr)
        sys.exit(2)

    records = []
    regressions = []
    for name in names:
        baseline = baseline_metrics[name]
        current = current_metrics[name]
        delta, threshold, regressed = status_for(name, baseline, current)
        record = {
            "name": name,
            "baseline": baseline,
            "current": current,
            "delta_pct": delta,
            "threshold": threshold,
            "status": "regressed" if regressed else "ok",
        }
        records.append(record)
        if regressed:
            regressions.append(record)

    index = {
        "schemaVersion": 1,
        "generatedAt": datetime.now(timezone.utc).isoformat(),
        "baselineRun": str(baseline_dir),
        "currentRun": str(current_dir),
        "thresholds": thresholds,
        "result": "regressed" if regressions else "passed",
        "metrics": records,
        "regressions": regressions,
    }
    (output_dir / "index.json").write_text(
        json.dumps(index, allow_nan=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    write_summary(records, regressions)

    if regressions:
        print(f"performance regressions detected: {len(regressions)}", file=sys.stderr)
        sys.exit(1)
    print(f"performance comparison passed: {len(records)} metrics compared")


main()
PY
