#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/common.sh"
source "${ROOT_DIR}/scripts/lib/kind-evidence.sh"
RUN_ID="${RUN_ID:-$(date +%Y-%m-%d-%H%M%S)-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)-kind-soak}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/reports/soak/runs/${RUN_ID}}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
HTTP_HOST="${HTTP_HOST:-example.com}"
HTTP_CLIENT="${ROOT_DIR}/tests/e2e/http_concurrency_client.py"
SOAK_DURATION_SECONDS="${SOAK_DURATION_SECONDS:-86400}"
SAMPLE_INTERVAL_SECONDS="${SAMPLE_INTERVAL_SECONDS:-60}"
TRAFFIC_BATCH_REQUESTS="${TRAFFIC_BATCH_REQUESTS:-200}"
TRAFFIC_BATCH_CONCURRENCY="${TRAFFIC_BATCH_CONCURRENCY:-16}"
SUMMARY_ONLY="${SUMMARY_ONLY:-false}"
MIN_SUCCESS_RATE="${MIN_SUCCESS_RATE:-1.0}"
MAX_ERRORS="${MAX_ERRORS:-0}"
MAX_P99_MS="${MAX_P99_MS:-5000}"
MAX_LATENCY_MS="${MAX_LATENCY_MS:-30000}"
SLO_GATE_RISK_ACCEPTED="${SLO_GATE_RISK_ACCEPTED:-false}"
REQUIRE_24H="${REQUIRE_24H:-false}"
MIN_REQUIRED_DURATION_SECONDS="${MIN_REQUIRED_DURATION_SECONDS:-86400}"
TRAFFIC_PID=""

log() {
  aeg_kind_log "kind-soak" "$*"
}

require_command() {
  aeg_require_command "kind-soak" "$1"
}

ensure_stack() {
  aeg_kind_ensure_stack "kind-soak" "${ROOT_DIR}" "${HTTP_HOST}" "${GATEWAY_HOST_PORT}"
}

collect_admin() {
  aeg_kind_collect_admin_snapshots "${ROOT_DIR}" "${OUTPUT_DIR}" "$1"
}

capture_resource_sample() {
  aeg_kind_capture_resource_snapshot "${OUTPUT_DIR}" "$1"
}

metadata_value() {
  local key="$1"
  local metadata="${OUTPUT_DIR}/metadata.txt"

  if [[ ! -f "${metadata}" ]]; then
    return
  fi

  awk -F= -v key="${key}" '
    $1 == key {
      sub(/^[^=]*=/, "")
      print
      exit
    }
  ' "${metadata}"
}

metadata_or_default() {
  local key="$1"
  local default_value="$2"
  local value
  value="$(metadata_value "${key}")"
  if [[ -n "${value}" ]]; then
    printf '%s\n' "${value}"
  else
    printf '%s\n' "${default_value}"
  fi
}

write_metadata() {
  local metadata="${OUTPUT_DIR}/metadata.txt"
  local tmp="${metadata}.tmp"
  local captured_at
  local git_commit
  local git_tree_state
  local code_tree_state
  local duration_seconds
  local sample_interval_seconds

  mkdir -p "${OUTPUT_DIR}"
  captured_at="$(metadata_or_default "captured_at" "$(date --iso-8601=seconds)")"
  git_commit="$(metadata_or_default "git_commit" "$(git -C "${ROOT_DIR}" rev-parse HEAD)")"
  git_tree_state="$(metadata_or_default "git_tree_state" "$(aeg_git_tree_state "${ROOT_DIR}")")"
  code_tree_state="$(metadata_or_default "code_tree_state" "$(aeg_code_tree_state "${ROOT_DIR}")")"
  duration_seconds="$(metadata_or_default "duration_seconds" "${SOAK_DURATION_SECONDS}")"
  sample_interval_seconds="$(metadata_or_default "sample_interval_seconds" "${SAMPLE_INTERVAL_SECONDS}")"

  {
    printf 'captured_at=%s\n' "${captured_at}"
    printf 'git_commit=%s\n' "${git_commit}"
    printf 'git_tree_state=%s\n' "${git_tree_state}"
    printf 'code_tree_state=%s\n' "${code_tree_state}"
    printf 'run_id=%s\n' "$(metadata_or_default "run_id" "${RUN_ID}")"
    printf 'output_dir=%s\n' "$(metadata_or_default "output_dir" "${OUTPUT_DIR}")"
    printf 'gateway_host_port=%s\n' "$(metadata_or_default "gateway_host_port" "${GATEWAY_HOST_PORT}")"
    printf 'http_host=%s\n' "$(metadata_or_default "http_host" "${HTTP_HOST}")"
    printf 'duration_seconds=%s\n' "${duration_seconds}"
    printf 'sample_interval_seconds=%s\n' "${sample_interval_seconds}"
    printf 'traffic_batch_requests=%s\n' "$(metadata_or_default "traffic_batch_requests" "${TRAFFIC_BATCH_REQUESTS}")"
    printf 'traffic_batch_concurrency=%s\n' "$(metadata_or_default "traffic_batch_concurrency" "${TRAFFIC_BATCH_CONCURRENCY}")"
    printf 'min_success_rate=%s\n' "${MIN_SUCCESS_RATE}"
    printf 'max_errors=%s\n' "${MAX_ERRORS}"
    printf 'max_p99_ms=%s\n' "${MAX_P99_MS}"
    printf 'max_latency_ms=%s\n' "${MAX_LATENCY_MS}"
    printf 'slo_gate_risk_accepted=%s\n' "${SLO_GATE_RISK_ACCEPTED}"
    printf 'require_24h=%s\n' "${REQUIRE_24H}"
    printf 'min_required_duration_seconds=%s\n' "${MIN_REQUIRED_DURATION_SECONDS}"
  } >"${tmp}"
  mv "${tmp}" "${metadata}"
}

validate_duration_requirement() {
  local duration_seconds
  duration_seconds="$(metadata_or_default "duration_seconds" "${SOAK_DURATION_SECONDS}")"

  if [[ "${REQUIRE_24H}" != "true" ]]; then
    return
  fi

  if awk -v duration="${duration_seconds}" -v required="${MIN_REQUIRED_DURATION_SECONDS}" 'BEGIN { exit !(duration + 0 >= required + 0) }'; then
    return
  fi

  printf '[kind-soak] duration_seconds %s is less than required %s for REQUIRE_24H=true\n' \
    "${duration_seconds}" \
    "${MIN_REQUIRED_DURATION_SECONDS}" >&2
  exit 1
}

start_background_traffic() {
  aeg_kind_start_background_http_traffic \
    TRAFFIC_PID \
    "${HTTP_CLIENT}" \
    "${OUTPUT_DIR}" \
    "${GATEWAY_HOST_PORT}" \
    "${HTTP_HOST}" \
    "${TRAFFIC_BATCH_REQUESTS}" \
    "${TRAFFIC_BATCH_CONCURRENCY}"
}

stop_background_traffic() {
  aeg_kind_stop_background_pid "${TRAFFIC_PID}"
}

cleanup() {
  local exit_code="$?"
  stop_background_traffic
  exit "${exit_code}"
}

aggregate_traffic() {
  local input="${OUTPUT_DIR}/traffic/http-batches.jsonl"
  local output="${OUTPUT_DIR}/traffic/summary.json"
  mkdir -p "$(dirname "${output}")"
  python3 - "${input}" "${output}" <<'PY'
import json
import sys
from pathlib import Path

src = Path(sys.argv[1])
dst = Path(sys.argv[2])
items = [json.loads(line) for line in src.read_text(encoding="utf-8").splitlines() if line.strip()]
if not items:
    raise SystemExit("no traffic batches captured")

summary = {
    "batches": len(items),
    "completed": sum(item["completed"] for item in items),
    "successes": sum(item["successes"] for item in items),
    "mean_success_rate": sum(item["success_rate"] for item in items) / len(items),
    "max_p95_ms": max(item["latency_ms"]["p95"] for item in items),
    "max_p99_ms": max(item["latency_ms"]["p99"] for item in items),
    "max_p999_ms": max(item["latency_ms"].get("p999", item["latency_ms"]["p99"]) for item in items),
    "max_latency_ms": max(item["latency_ms"]["max"] for item in items),
}
dst.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

apply_traffic_slo_gate() {
  local summary="${OUTPUT_DIR}/traffic/summary.json"

  aeg_kind_write_traffic_slo_gate \
    "${summary}" \
    "${summary}" \
    "${MIN_SUCCESS_RATE}" \
    "${MAX_ERRORS}" \
    "${MAX_P99_MS}" \
    "${MAX_LATENCY_MS}" \
    "${SLO_GATE_RISK_ACCEPTED}"
}

aggregate_resources() {
  local input_dir="${OUTPUT_DIR}/resources"
  local output="${OUTPUT_DIR}/resources/summary.json"
  mkdir -p "$(dirname "${output}")"
  python3 - "${input_dir}" "${output}" <<'PY'
import csv
import json
import sys
from collections import defaultdict
from pathlib import Path

src = Path(sys.argv[1])
dst = Path(sys.argv[2])


def snapshot_order(path: Path):
    name = path.stem
    if name == "before":
        return (0, 0, name)
    if name.startswith("sample-"):
        try:
            return (1, int(name.split("-", 1)[1]), name)
        except ValueError:
            return (1, 0, name)
    if name == "after":
        return (2, 0, name)
    return (3, 0, name)


def slope(values):
    if len(values) < 2:
        return 0
    return (values[-1] - values[0]) / (len(values) - 1)


snapshots = []
if src.exists():
    for path in sorted(src.glob("*.tsv"), key=snapshot_order):
        totals = defaultdict(lambda: {"fd_count": 0, "rss_kib": 0, "threads": 0})
        with path.open(encoding="utf-8", newline="") as handle:
            reader = csv.DictReader(handle, delimiter="\t")
            for row in reader:
                component = row.get("component", "").strip()
                if not component:
                    continue
                totals[component]["fd_count"] += int(row.get("fd_count") or 0)
                totals[component]["rss_kib"] += int(row.get("rss_kib") or 0)
                totals[component]["threads"] += int(row.get("threads") or 0)
        snapshots.append({"name": path.stem, "components": dict(totals)})

components = sorted({component for item in snapshots for component in item["components"]})
summary = {"sample_count": len(snapshots), "components": {}}
for component in components:
    fd_values = [item["components"].get(component, {}).get("fd_count", 0) for item in snapshots]
    rss_values = [item["components"].get(component, {}).get("rss_kib", 0) for item in snapshots]
    thread_values = [item["components"].get(component, {}).get("threads", 0) for item in snapshots]
    summary["components"][component] = {
        "fd_slope_per_sample": slope(fd_values),
        "rss_kib_slope_per_sample": slope(rss_values),
        "threads_slope_per_sample": slope(thread_values),
        "first": {
            "fd_count": fd_values[0] if fd_values else 0,
            "rss_kib": rss_values[0] if rss_values else 0,
            "threads": thread_values[0] if thread_values else 0,
        },
        "last": {
            "fd_count": fd_values[-1] if fd_values else 0,
            "rss_kib": rss_values[-1] if rss_values else 0,
            "threads": thread_values[-1] if thread_values else 0,
        },
    }

dst.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

aggregate_observability() {
  local output="${OUTPUT_DIR}/observability/summary.json"
  mkdir -p "$(dirname "${output}")"
  python3 - "${OUTPUT_DIR}" "${output}" <<'PY'
import json
import math
import re
import sys
from collections import defaultdict
from pathlib import Path

root = Path(sys.argv[1])
dst = Path(sys.argv[2])


def sample_index(path: Path):
    for part in path.parts:
        if part == "admin-before":
            return (0, 0, str(path))
        if part == "admin-after":
            return (2, 0, str(path))
        if part.startswith("admin-"):
            try:
                return (1, int(part.split("-", 1)[1]), str(path))
            except ValueError:
                return (1, 0, str(path))
    return (3, 0, str(path))


def metric_files():
    candidates = []
    candidates.extend(root.glob("admin-before/dataplane/metrics.prom"))
    candidates.extend(root.glob("samples/admin-*/dataplane/metrics.prom"))
    candidates.extend(root.glob("admin-after/dataplane/metrics.prom"))
    return sorted(candidates, key=sample_index)


def controlplane_summaries():
    candidates = []
    candidates.extend(root.glob("admin-before/controlplane/summary.json"))
    candidates.extend(root.glob("samples/admin-*/controlplane/summary.json"))
    candidates.extend(root.glob("admin-after/controlplane/summary.json"))
    return sorted(candidates, key=sample_index)


def parse_metric_text(text: str):
    scalars = defaultdict(float)
    buckets = defaultdict(float)
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) < 2:
            continue
        series = parts[0]
        try:
            value = float(parts[1])
        except ValueError:
            continue
        metric = series.split("{", 1)[0]
        scalars[metric] += value
        if metric == "aether_gateway_dataplane_xds_apply_stage_duration_ms_bucket" and 'stage="ack_wait"' in series:
            match = re.search(r'le="([^"]+)"', series)
            if match:
                buckets[match.group(1)] += value
    return scalars, buckets


def bucket_bound(value: str):
    if value == "+Inf":
        return math.inf
    return float(value)


def histogram_quantile(buckets, quantile):
    if not buckets:
        return None
    sorted_buckets = sorted(buckets.items(), key=lambda item: bucket_bound(item[0]))
    total = None
    for label, value in sorted_buckets:
        if label == "+Inf":
            total = value
            break
    if total is None:
        total = sorted_buckets[-1][1]
    if total <= 0:
        return None
    target = total * quantile
    for label, value in sorted_buckets:
        if value >= target:
            bound = bucket_bound(label)
            if math.isinf(bound):
                return None
            return bound
    return None


metrics = []
for path in metric_files():
    scalars, buckets = parse_metric_text(path.read_text(encoding="utf-8"))
    metrics.append({"path": str(path.relative_to(root)), "scalars": scalars, "buckets": buckets})

first = metrics[0]["scalars"] if metrics else {}
last = metrics[-1]["scalars"] if metrics else {}
stream_delta = last.get("aether_gateway_dataplane_xds_stream_failures_total", 0) - first.get("aether_gateway_dataplane_xds_stream_failures_total", 0)
connect_delta = last.get("aether_gateway_dataplane_xds_connect_failures_total", 0) - first.get("aether_gateway_dataplane_xds_connect_failures_total", 0)
nack_delta = last.get("aether_gateway_dataplane_xds_snapshots_nacked_total", 0) - first.get("aether_gateway_dataplane_xds_snapshots_nacked_total", 0)
ack_p99 = histogram_quantile(metrics[-1]["buckets"], 0.99) if metrics else None

ready_values = []
for path in controlplane_summaries():
    payload = json.loads(path.read_text(encoding="utf-8"))
    ready = payload.get("readyNodeCount", payload.get("currentVersionReadyCount"))
    if ready is not None:
        ready_values.append(int(ready))

summary = {
    "metric_sample_count": len(metrics),
    "xds_reconnect_delta": stream_delta + connect_delta,
    "xds_stream_failure_delta": stream_delta,
    "xds_connect_failure_delta": connect_delta,
    "xds_nack_delta": nack_delta,
    "ack_wait_latency_ms": {"p99": ack_p99},
    "ready_replicas": {
        "sample_count": len(ready_values),
        "first_ready": ready_values[0] if ready_values else None,
        "min_ready": min(ready_values) if ready_values else None,
        "max_ready": max(ready_values) if ready_values else None,
        "last_ready": ready_values[-1] if ready_values else None,
    },
}

dst.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

write_summary() {
  local git_commit
  git_commit="$(git -C "${ROOT_DIR}" rev-parse --short HEAD)"
  python3 - "${OUTPUT_DIR}" "${RUN_ID}" "${git_commit}" "${SOAK_DURATION_SECONDS}" "${SAMPLE_INTERVAL_SECONDS}" <<'PY'
import json
import math
import sys
from pathlib import Path

root = Path(sys.argv[1])
run_id = sys.argv[2]
git_commit = sys.argv[3]
duration_seconds = sys.argv[4]
sample_interval_seconds = sys.argv[5]


def load_json(path, default):
    if path.exists():
        return json.loads(path.read_text(encoding="utf-8"))
    return default


def fmt_number(value):
    if value is None:
        return "n/a"
    if isinstance(value, float) and math.isnan(value):
        return "n/a"
    numeric = float(value)
    if numeric.is_integer():
        return str(int(numeric))
    return f"{numeric:.3f}".rstrip("0").rstrip(".")


def fmt_ms(value):
    return f"{fmt_number(value)}ms" if value is not None else "n/a"


traffic = load_json(root / "traffic" / "summary.json", {})
gate = traffic.get("slo_gate", {})
observed = gate.get("observed", {})
thresholds = gate.get("thresholds", {})
resources = load_json(root / "resources" / "summary.json", {"components": {}})
observability = load_json(root / "observability" / "summary.json", {})
ack_wait = observability.get("ack_wait_latency_ms", {})
ready = observability.get("ready_replicas", {})

lines = [
    "# Kind Soak",
    "",
    f"- Run ID: `{run_id}`",
    f"- Git commit: `{git_commit}`",
    f"- Duration Seconds: `{duration_seconds}`",
    f"- Sample Interval Seconds: `{sample_interval_seconds}`",
    f"- traffic SLO gate: `{gate.get('status', 'unknown')}`",
    "",
    "## Traffic Summary",
    "",
    f"- max p99 / p999: `{fmt_ms(traffic.get('max_p99_ms'))}` / `{fmt_ms(traffic.get('max_p999_ms'))}`",
    "- SLO: "
    f"success rate `{fmt_number(float(observed.get('success_rate')) * 100) + '%' if observed.get('success_rate') is not None else 'n/a'}` >= "
    f"`{fmt_number(float(thresholds.get('min_success_rate')) * 100) + '%' if thresholds.get('min_success_rate') is not None else 'n/a'}`; "
    f"errors `{fmt_number(observed.get('errors'))}` <= `{fmt_number(thresholds.get('max_errors'))}`; "
    f"p99 `{fmt_ms(observed.get('p99_ms'))}` <= `{fmt_ms(thresholds.get('max_p99_ms'))}`; "
    f"max latency `{fmt_ms(observed.get('max_latency_ms'))}` <= `{fmt_ms(thresholds.get('max_latency_ms'))}`",
    "",
    "```json",
    json.dumps(traffic, indent=2, sort_keys=True),
    "```",
    "",
    "## Resource Slopes",
    "",
    "| Component | FD slope/sample | RSS KiB slope/sample | Threads slope/sample |",
    "| --- | ---: | ---: | ---: |",
]

components = resources.get("components", {})
if components:
    for name in sorted(components):
        item = components[name]
        lines.append(
            f"| {name} | {fmt_number(item.get('fd_slope_per_sample'))} | "
            f"{fmt_number(item.get('rss_kib_slope_per_sample'))} | "
            f"{fmt_number(item.get('threads_slope_per_sample'))} |"
        )
else:
    lines.append("| n/a | n/a | n/a | n/a |")

lines.extend([
    "",
    "## Observability Summary",
    "",
    f"- xDS reconnect delta: `{fmt_number(observability.get('xds_reconnect_delta'))}`",
    f"- xDS NACK delta: `{fmt_number(observability.get('xds_nack_delta'))}`",
    f"- snapshot ACK wait p99: `{fmt_ms(ack_wait.get('p99'))}`",
    "- ready replicas first/min/max/last: "
    f"`{fmt_number(ready.get('first_ready'))}` / "
    f"`{fmt_number(ready.get('min_ready'))}` / "
    f"`{fmt_number(ready.get('max_ready'))}` / "
    f"`{fmt_number(ready.get('last_ready'))}`",
    "",
    "```json",
    json.dumps(observability, indent=2, sort_keys=True),
    "```",
    "",
    "## Samples",
    "",
    "- Resource snapshots: `resources/`",
    "- Admin snapshots: `admin-before/`, `samples/`, `admin-after/`",
    "",
    "## Note",
    "",
    "- This script is the soak automation entrypoint.",
    "- Only a full `24h` run can close the corresponding P0 item in `TODO.md`.",
])

(root / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
}

summarize_evidence() {
  aggregate_traffic
  apply_traffic_slo_gate
  aggregate_resources
  aggregate_observability
  write_summary
  assert_traffic_slo_gate
}

assert_traffic_slo_gate() {
  local status
  status="$(aeg_kind_slo_status "${OUTPUT_DIR}/traffic/summary.json")"

  if [[ "${status}" == "fail" ]]; then
    log "traffic SLO gate failed"
    exit 1
  fi
  if [[ "${status}" == "risk-accepted" ]]; then
    log "traffic SLO gate is risk-accepted"
  fi
}


main() {
  trap cleanup EXIT

  require_command git
  require_command python3

  if [[ "${SUMMARY_ONLY}" == "true" ]]; then
    write_metadata
    validate_duration_requirement
    summarize_evidence
    log "soak summary written to ${OUTPUT_DIR}"
    return
  fi

  write_metadata
  validate_duration_requirement
  require_command curl
  require_command kubectl

  ensure_stack
  mkdir -p "${OUTPUT_DIR}/samples"

  collect_admin admin-before
  capture_resource_sample before
  start_background_traffic

  local started_at
  local sample_index=0
  started_at="$(date +%s)"
  while (( $(date +%s) - started_at < SOAK_DURATION_SECONDS )); do
    sleep "${SAMPLE_INTERVAL_SECONDS}"
    sample_index=$((sample_index + 1))
    collect_admin "samples/admin-${sample_index}"
    capture_resource_sample "sample-${sample_index}"
  done

  stop_background_traffic
  TRAFFIC_PID=""
  collect_admin admin-after
  capture_resource_sample after
  summarize_evidence

  log "soak evidence written to ${OUTPUT_DIR}"
}

main "$@"
