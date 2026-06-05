#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${ROOT_DIR}/scripts/lib/common.sh"
# shellcheck source=scripts/lib/performance-common.sh
source "${ROOT_DIR}/scripts/lib/performance-common.sh"

RUN_ID="${RUN_ID:-$(date +%Y-%m-%d-%H%M%S)-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)-dataplane-throughput}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/reports/performance/runs/${RUN_ID}}"
INPUT_DIR="${INPUT_DIR:-}"
CHAOS_INPUT_DIR="${CHAOS_INPUT_DIR:-}"
SOAK_INPUT_DIR="${SOAK_INPUT_DIR:-}"
RUN_KIND_A4="${RUN_KIND_A4:-false}"
KIND_A4_SCRIPT="${KIND_A4_SCRIPT:-${ROOT_DIR}/scripts/run-kind-a4-baseline.sh}"
SOURCE_DIR="${SOURCE_DIR:-${OUTPUT_DIR}/source-kind-a4}"
REPORT_OUTPUT="${OUTPUT_DIR}/throughput-report.json"
SUMMARY_OUTPUT="${OUTPUT_DIR}/summary.md"
METADATA_OUTPUT="${OUTPUT_DIR}/metadata.txt"

log() {
  aeg_perf_log "dataplane-throughput-baseline" "$*"
}

usage() {
  cat <<'EOF'
usage: run-dataplane-throughput-baseline.sh

Generate a standardized dataplane throughput report from collected evidence.

Environment:
  INPUT_DIR=<path>       Existing evidence directory to read. Expected inputs
                         include http/*.json, grpc/*.json,
                         admin-after/dataplane/traffic.json, metrics.prom and
                         resources/after.tsv when available.
  CHAOS_INPUT_DIR=<path> Optional kind fault-injection evidence directory.
                         Expected inputs include metadata.txt,
                         traffic/summary.json and conclusions/summary.json.
  SOAK_INPUT_DIR=<path>  Optional kind soak evidence directory. Expected inputs
                         include metadata.txt, traffic/summary.json,
                         resources/summary.json and observability/summary.json.
  OUTPUT_DIR=<path>      Output directory. Defaults to
                         reports/performance/runs/<run-id>-dataplane-throughput/.
  RUN_ID=<id>            Report run identifier.
  RUN_KIND_A4=true       Collect a local kind A4 evidence source first, then
                         render this report from that source directory.
  KIND_A4_SCRIPT=<path>  Override the kind A4 evidence script.

This script does not recreate a kind cluster. It either renders an existing
evidence directory, or explicitly delegates traffic collection to
scripts/run-kind-a4-baseline.sh when RUN_KIND_A4=true.
EOF
}

require_command() {
  aeg_perf_require_command "dataplane-throughput-baseline" "$1"
}

write_metadata() {
  aeg_perf_metadata_common "${ROOT_DIR}" "${RUN_ID}" \
    | sed -e '/^kernel=.*/a\input_dir='"${INPUT_DIR}"'' \
    -e '/^input_dir=.*/a\chaos_input_dir='"${CHAOS_INPUT_DIR}"'' \
    -e '/^chaos_input_dir=.*/a\soak_input_dir='"${SOAK_INPUT_DIR}"'' \
    -e '/^soak_input_dir=.*/a\output_dir='"${OUTPUT_DIR}"'' \
    -e '/^output_dir=.*/a\run_kind_a4='"${RUN_KIND_A4}"'' \
    >"${METADATA_OUTPUT}"
}

collect_kind_source() {
  if [[ "${RUN_KIND_A4}" != "true" ]]; then
    return
  fi
  if [[ -n "${INPUT_DIR}" ]]; then
    log "INPUT_DIR is set; skipping kind A4 collection"
    return
  fi
  if [[ ! -x "${KIND_A4_SCRIPT}" ]]; then
    log "kind A4 script is not executable: ${KIND_A4_SCRIPT}"
    exit 1
  fi

  log "collecting source evidence with run-kind-a4-baseline.sh"
  RUN_ID="${RUN_ID}-source-kind-a4" \
    OUTPUT_DIR="${SOURCE_DIR}" \
    "${KIND_A4_SCRIPT}"
  INPUT_DIR="${SOURCE_DIR}"
}

render_report() {
  python3 - "${INPUT_DIR}" "${OUTPUT_DIR}" "${RUN_ID}" "${REPORT_OUTPUT}" "${SUMMARY_OUTPUT}" "${CHAOS_INPUT_DIR}" "${SOAK_INPUT_DIR}" <<'PY'
from __future__ import annotations

import csv
import json
import math
import re
import sys
from pathlib import Path
from typing import Any

input_dir = Path(sys.argv[1]).resolve()
output_dir = Path(sys.argv[2]).resolve()
run_id = sys.argv[3]
report_output = Path(sys.argv[4])
summary_output = Path(sys.argv[5])
chaos_input_dir_arg = sys.argv[6]
soak_input_dir_arg = sys.argv[7]

PROFILE_DIRS = {
    "http",
    "grpc",
    "websocket",
    "sse",
    "mcp",
    "tcp",
    "tls",
    "tls-passthrough",
    "udp",
    "stream",
    "streaming",
}
REQUIRED_PROTOCOLS = ["http", "grpc", "websocket", "sse", "mcp", "tcp", "udp"]
REQUIRED_SCENARIOS = [
    "steady",
    "burst",
    "ceiling",
    "long-lived-streaming",
    "backend-slow-read",
    "backend-slow-write",
    "backend-error",
    "endpoint-flapping",
    "reload-under-load",
]
UDP_REQUIRED_SCENARIOS = [
    "backend-timeout",
    "high-churn",
    "multi-client",
    "multi-upstream",
]
STREAM_RESOURCE_PROTOCOLS = {"tcp", "tls", "tls-passthrough", "stream"}
RELOAD_PHASES = ["before", "during", "after"]
LIVE_RELOAD_REQUIRED_PROTOCOLS = ["http", "grpc", "tcp", "udp"]
LIVE_RELOAD_REQUIRED_MUTATIONS = [
    "route-only",
    "backend-only",
    "endpoint-only",
    "secret-only",
    "tls-asset-rotation",
    "listener-add-remove",
]
UPSTREAM_TUNING_MIN_POOL_SAMPLES = 10
UPSTREAM_TUNING_LOW_POOL_HIT_RATIO = 0.20
UPSTREAM_TUNING_CONNECT_P99_SHARE = 0.50
UPSTREAM_TUNING_SHORT_PROTOCOLS = {"http", "grpc"}
UPSTREAM_TUNING_EXCLUDED_SCENARIOS = {
    "backend-error",
    "backend-slow-read",
    "backend-slow-write",
    "endpoint-flapping",
    "long-lived-streaming",
    "reload-under-load",
}


def load_json(path: Path) -> dict[str, Any] | None:
    if not path.is_file():
        return None
    with path.open(encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        return None
    return value


def first_json(candidates: list[Path]) -> tuple[Path | None, dict[str, Any]]:
    for path in candidates:
        value = load_json(path)
        if value is not None:
            return path, value
    return None, {}


def read_text(path: Path | None) -> str:
    if path is None or not path.is_file():
        return ""
    return path.read_text(encoding="utf-8")


def first_file(candidates: list[Path]) -> Path | None:
    for path in candidates:
        if path.is_file():
            return path
    return None


def is_profile_payload(payload: dict[str, Any]) -> bool:
    latency = payload.get("latency_ms")
    if not isinstance(latency, dict):
        return False
    return any(key in payload for key in ("requests", "completed", "success_rate", "achieved_rps"))


def as_number(value: Any) -> float | None:
    if value is None:
        return None
    if isinstance(value, bool):
        return None
    if isinstance(value, (int, float)):
        if math.isfinite(float(value)):
            return float(value)
        return None
    if isinstance(value, str):
        try:
            parsed = float(value)
        except ValueError:
            return None
        if math.isfinite(parsed):
            return parsed
    return None


def as_int(value: Any, default: int = 0) -> int:
    parsed = as_number(value)
    if parsed is None:
        return default
    return int(parsed)


def optional_path(value: str) -> Path | None:
    if not value.strip():
        return None
    return Path(value).resolve()


def read_metadata_txt(path: Path) -> dict[str, str]:
    metadata_path = path / "metadata.txt"
    if not metadata_path.is_file():
        return {}
    values: dict[str, str] = {}
    for raw in metadata_path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def metadata_int(metadata: dict[str, str], name: str) -> int | None:
    value = as_number(metadata.get(name))
    if value is None:
        return None
    return int(value)


def evidence_traffic_summary(payload: dict[str, Any]) -> dict[str, Any]:
    slo_gate = payload.get("slo_gate") if isinstance(payload.get("slo_gate"), dict) else {}
    success_rate = first_number(payload, "mean_success_rate", "success_rate")
    if success_rate is None:
        completed = as_number(payload.get("completed"))
        successes = as_number(payload.get("successes"))
        if completed is not None and completed > 0 and successes is not None:
            success_rate = successes / completed
    return {
        "batches": as_int(payload.get("batches")),
        "completed": as_int(payload.get("completed")),
        "successes": as_int(payload.get("successes")),
        "errors": as_int(payload.get("errors")),
        "success_rate": success_rate,
        "max_p95_ms": first_number(payload, "max_p95_ms", "p95_ms"),
        "max_p99_ms": first_number(payload, "max_p99_ms", "p99_ms"),
        "max_p999_ms": first_number(payload, "max_p999_ms", "p999_ms", "p99_9_ms"),
        "max_latency_ms": first_number(payload, "max_latency_ms", "latency_max_ms"),
        "slo_gate_status": str(slo_gate.get("status", "")) or None,
    }


def load_chaos_evidence(path: Path | None) -> dict[str, Any] | None:
    if path is None:
        return None
    metadata = read_metadata_txt(path)
    traffic = load_json(path / "traffic" / "summary.json") or {}
    conclusions = load_json(path / "conclusions" / "summary.json") or {}

    required_scenarios = conclusions.get("required_scenarios")
    if not isinstance(required_scenarios, list):
        required_scenarios = []
    observed_scenarios = conclusions.get("observed_scenarios")
    if not isinstance(observed_scenarios, list):
        observed_scenarios = []
    missing_scenarios = conclusions.get("missing_required_scenarios")
    if not isinstance(missing_scenarios, list):
        missing_scenarios = sorted(set(map(str, required_scenarios)) - set(map(str, observed_scenarios)))
    status_counts = conclusions.get("status_counts")
    if not isinstance(status_counts, dict):
        status_counts = {}

    scenarios: list[dict[str, Any]] = []
    raw_scenarios = conclusions.get("scenarios")
    if isinstance(raw_scenarios, dict):
        for name, item in sorted(raw_scenarios.items()):
            payload = item if isinstance(item, dict) else {}
            evidence = payload.get("evidence")
            if not isinstance(evidence, list):
                evidence = []
            scenarios.append(
                {
                    "scenario": str(name),
                    "status": str(payload.get("status", "")) or None,
                    "summary": str(payload.get("summary", "")) or None,
                    "evidence": [str(value) for value in evidence],
                }
            )

    return {
        "source_dir": str(path),
        "run_id": metadata.get("run_id") or path.name,
        "git_commit": metadata.get("git_commit"),
        "release_gate_status": conclusions.get("release_gate_status"),
        "traffic": evidence_traffic_summary(traffic),
        "coverage": {
            "required_scenarios": [str(value) for value in required_scenarios],
            "observed_scenarios": [str(value) for value in observed_scenarios],
            "missing_scenarios": [str(value) for value in missing_scenarios],
            "status_counts": {str(key): as_int(value) for key, value in status_counts.items()},
        },
        "scenarios": scenarios,
    }


def load_soak_evidence(path: Path | None) -> dict[str, Any] | None:
    if path is None:
        return None
    metadata = read_metadata_txt(path)
    traffic = load_json(path / "traffic" / "summary.json") or {}
    resources_summary = load_json(path / "resources" / "summary.json") or {}
    observability_summary = load_json(path / "observability" / "summary.json") or {}
    ack_wait_latency = (
        observability_summary.get("ack_wait_latency_ms")
        if isinstance(observability_summary.get("ack_wait_latency_ms"), dict)
        else {}
    )
    ready_replicas = (
        observability_summary.get("ready_replicas")
        if isinstance(observability_summary.get("ready_replicas"), dict)
        else {}
    )
    duration_seconds = metadata_int(metadata, "duration_seconds")
    sample_interval_seconds = metadata_int(metadata, "sample_interval_seconds")
    return {
        "source_dir": str(path),
        "run_id": metadata.get("run_id") or path.name,
        "git_commit": metadata.get("git_commit"),
        "duration_seconds": duration_seconds,
        "sample_interval_seconds": sample_interval_seconds,
        "is_24h": bool(duration_seconds is not None and duration_seconds >= 86400),
        "traffic": evidence_traffic_summary(traffic),
        "resources": resources_summary,
        "observability": {
            "metric_sample_count": as_int(observability_summary.get("metric_sample_count")),
            "xds_reconnect_delta": first_number(observability_summary, "xds_reconnect_delta"),
            "xds_stream_failure_delta": first_number(observability_summary, "xds_stream_failure_delta"),
            "xds_connect_failure_delta": first_number(observability_summary, "xds_connect_failure_delta"),
            "xds_nack_delta": first_number(observability_summary, "xds_nack_delta"),
            "ack_wait_p99_ms": first_number(ack_wait_latency, "p99"),
            "ready_replicas": ready_replicas,
        },
    }


def first_number(payload: dict[str, Any], *names: str) -> float | None:
    for name in names:
        value = as_number(payload.get(name))
        if value is not None:
            return value
    return None


def first_nested_number(payload: dict[str, Any], *names: str) -> float | None:
    containers = [payload]
    for key in ("resources", "resource_usage"):
        nested = payload.get(key)
        if isinstance(nested, dict):
            containers.append(nested)
    for container in containers:
        value = first_number(container, *names)
        if value is not None:
            return value
    return None


def profile_resource_sample(payload: dict[str, Any]) -> dict[str, float | None]:
    return {
        "rss_kib": first_nested_number(payload, "rss_kib", "rssKiB", "rss"),
        "fd_count": first_nested_number(payload, "fd_count", "fds", "fd"),
        "threads": first_nested_number(payload, "threads", "thread_count"),
        "cpu_millicores": first_nested_number(
            payload,
            "cpu_millicores",
            "cpu_millis",
            "cpu_m",
        ),
        "buffer_size_bytes": first_nested_number(
            payload,
            "buffer_size_bytes",
            "buffer_bytes",
            "tcp_buffer_size_bytes",
            "stream_buffer_size_bytes",
        ),
    }


def normalize_variant(value: Any) -> str | None:
    if not isinstance(value, str):
        return None
    normalized = value.strip().lower().replace("_", "-").replace(" ", "-")
    if normalized in {"baseline", "before", "old", "previous"}:
        return "baseline"
    if normalized in {"current", "after", "new", "optimized"}:
        return "current"
    return normalized or None


def normalize_reload_phase(value: Any) -> str | None:
    if not isinstance(value, str):
        return None
    normalized = value.strip().lower().replace("_", "-").replace(" ", "-")
    return normalized if normalized in RELOAD_PHASES else None


def normalize_live_reload_mutation(value: Any) -> str | None:
    if not isinstance(value, str):
        return None
    normalized = value.strip().lower().replace("_", "-").replace(" ", "-")
    aliases = {
        "route": "route-only",
        "routes": "route-only",
        "route-only": "route-only",
        "backend": "backend-only",
        "backends": "backend-only",
        "backend-only": "backend-only",
        "endpoint": "endpoint-only",
        "endpoints": "endpoint-only",
        "endpoint-only": "endpoint-only",
        "secret": "secret-only",
        "secrets": "secret-only",
        "secret-only": "secret-only",
        "tls": "tls-asset-rotation",
        "tls-asset": "tls-asset-rotation",
        "tls-assets": "tls-asset-rotation",
        "tls-asset-rotation": "tls-asset-rotation",
        "cert-rotation": "tls-asset-rotation",
        "certificate-rotation": "tls-asset-rotation",
        "listener": "listener-add-remove",
        "listeners": "listener-add-remove",
        "listener-add": "listener-add-remove",
        "listener-remove": "listener-add-remove",
        "listener-add-remove": "listener-add-remove",
    }
    return aliases.get(normalized)


def live_reload_mutation_values(payload: dict[str, Any]) -> list[str]:
    values: set[str] = set()
    for key in ("reload_mutation", "snapshot_mutation", "mutation"):
        mutation = normalize_live_reload_mutation(payload.get(key))
        if mutation:
            values.add(mutation)
    for key in ("reload_mutations", "snapshot_mutations", "mutations"):
        raw_values = payload.get(key)
        if isinstance(raw_values, list):
            for item in raw_values:
                mutation = normalize_live_reload_mutation(item)
                if mutation:
                    values.add(mutation)
    return sorted(values)


def profile_response_flags(payload: dict[str, Any]) -> dict[str, int]:
    raw = payload.get("response_flags")
    if isinstance(raw, dict):
        return {str(key): as_int(value) for key, value in raw.items() if as_int(value) > 0}
    if isinstance(raw, list):
        flags: dict[str, int] = {}
        for item in raw:
            flag = str(item)
            if flag:
                flags[flag] = flags.get(flag, 0) + 1
        return flags
    return {}


def success_rate(payload: dict[str, Any]) -> float | None:
    explicit = as_number(payload.get("success_rate"))
    if explicit is not None:
        return explicit
    successes = as_number(payload.get("successes"))
    completed = as_number(payload.get("completed"))
    requests = as_number(payload.get("requests"))
    denominator = completed if completed and completed > 0 else requests
    if successes is None or denominator is None or denominator <= 0:
        return None
    return successes / denominator


def normalize_scenario_name(value: Any, allowed: set[str] | list[str] = REQUIRED_SCENARIOS) -> str | None:
    if not isinstance(value, str):
        return None
    normalized = value.strip().lower().replace("_", "-").replace(" ", "-")
    return normalized if normalized in allowed else None


def scenario_values_for(payload: dict[str, Any], allowed: set[str] | list[str]) -> list[str]:
    values: set[str] = set()
    for key in ("scenario", "profile"):
        normalized = normalize_scenario_name(payload.get(key), allowed)
        if normalized:
            values.add(normalized)
    scenarios = payload.get("scenarios")
    if isinstance(scenarios, list):
        for item in scenarios:
            normalized = normalize_scenario_name(item, allowed)
            if normalized:
                values.add(normalized)
    return sorted(values)


def scenario_values(payload: dict[str, Any]) -> list[str]:
    return scenario_values_for(payload, REQUIRED_SCENARIOS)


def udp_scenario_values(payload: dict[str, Any]) -> list[str]:
    return scenario_values_for(payload, set(REQUIRED_SCENARIOS) | set(UDP_REQUIRED_SCENARIOS))


def latency_value(latency: dict[str, Any], *names: str) -> float | None:
    for name in names:
        value = as_number(latency.get(name))
        if value is not None:
            return value
    return None


def collect_profiles() -> list[dict[str, Any]]:
    profiles: list[dict[str, Any]] = []
    for protocol_dir in sorted(input_dir.iterdir() if input_dir.is_dir() else []):
        if not protocol_dir.is_dir() or protocol_dir.name not in PROFILE_DIRS:
            continue
        for path in sorted(protocol_dir.rglob("*.json")):
            payload = load_json(path)
            if payload is None or not is_profile_payload(payload):
                continue
            latency = payload.get("latency_ms") or {}
            rel = path.relative_to(protocol_dir).with_suffix("")
            profiles.append(
                {
                    "protocol": protocol_dir.name,
                    "profile": str(rel),
                    "source": str(path.relative_to(input_dir)),
                    "requests": as_int(payload.get("requests"), as_int(payload.get("completed"))),
                    "completed": as_int(payload.get("completed"), as_int(payload.get("requests"))),
                    "successes": as_int(payload.get("successes"), as_int(payload.get("completed"))),
                    "concurrency": as_int(payload.get("concurrency")),
                    "connection_count": first_number(
                        payload,
                        "connection_count",
                        "connections",
                        "active_connections",
                    ),
                    "comparison_group": str(payload.get("comparison_group", "")).strip()
                    or None,
                    "variant": normalize_variant(
                        payload.get("variant")
                        or payload.get("implementation")
                        or payload.get("profile_variant")
                    ),
                    "success_rate": success_rate(payload),
                    "latency_ms": {
                        "p50": latency_value(latency, "p50"),
                        "p90": latency_value(latency, "p90"),
                        "p95": latency_value(latency, "p95"),
                        "p99": latency_value(latency, "p99"),
                        "p999": latency_value(latency, "p999", "p99_9", "p99.9"),
                        "max": latency_value(latency, "max"),
                    },
                    "achieved_rps": as_number(payload.get("achieved_rps")),
                    "status_counts": payload.get("status_counts") if isinstance(payload.get("status_counts"), dict) else {},
                    "error_counts": payload.get("error_counts") if isinstance(payload.get("error_counts"), dict) else {},
                    "scenarios": scenario_values(payload),
                    "reload_phase": normalize_reload_phase(payload.get("reload_phase")) or normalize_reload_phase(payload.get("phase")),
                    "reload_mutations": live_reload_mutation_values(payload),
                    "profile_response_flags": profile_response_flags(payload),
                    "xds_ack_latency_ms": first_number(payload, "xds_ack_latency_ms", "ack_latency_ms", "snapshot_ack_latency_ms"),
                    "xds_nack_count": first_number(payload, "xds_nack_count", "nack_count", "snapshot_nack_count"),
                    "last_good_fallback_count": first_number(payload, "last_good_fallback_count", "last_good_fallbacks"),
                    "udp_scenarios": udp_scenario_values(payload) if protocol_dir.name == "udp" else [],
                    "client_count": first_number(payload, "client_count", "clients", "udp_client_count"),
                    "upstream_count": first_number(payload, "upstream_count", "upstreams", "udp_upstream_count"),
                    "session_opens": first_number(payload, "session_opens", "sessions_opened", "udp_session_opens"),
                    "session_evictions": first_number(payload, "session_evictions", "sessions_evicted", "udp_session_evictions"),
                    "datagrams_sent": first_number(
                        payload,
                        "datagrams_sent",
                        "packets_sent",
                        "udp_packets_sent",
                    ),
                    "datagrams_received": first_number(
                        payload,
                        "datagrams_received",
                        "packets_received",
                        "udp_packets_received",
                    ),
                    "datagrams_lost": first_number(
                        payload,
                        "datagrams_lost",
                        "packets_lost",
                        "udp_packets_lost",
                    ),
                    "resource_sample": profile_resource_sample(payload),
                }
            )
    return profiles


def observed_scenarios(profiles: list[dict[str, Any]]) -> list[str]:
    observed: set[str] = set()
    for profile in profiles:
        observed.update(profile_scenarios(profile))
    return sorted(observed)


def profile_scenarios(profile: dict[str, Any]) -> list[str]:
    observed: set[str] = set()
    scenarios = profile.get("scenarios")
    if isinstance(scenarios, list):
        observed.update(
            item for item in scenarios if isinstance(item, str) and item in REQUIRED_SCENARIOS
        )
    evidence_key = "-".join(
        str(profile.get(part, ""))
        for part in ("protocol", "profile", "source")
    ).lower()
    evidence_key = evidence_key.replace("_", "-").replace("/", "-")
    for scenario in REQUIRED_SCENARIOS:
        if scenario in evidence_key:
            observed.add(scenario)
    return sorted(observed)


def max_optional(values: list[float | None]) -> float | None:
    numeric = [value for value in values if value is not None]
    if not numeric:
        return None
    return max(numeric)


def scenario_summary(profiles: list[dict[str, Any]]) -> list[dict[str, Any]]:
    by_scenario: dict[str, list[dict[str, Any]]] = {}
    for profile in profiles:
        for scenario in profile_scenarios(profile):
            by_scenario.setdefault(scenario, []).append(profile)

    summary: list[dict[str, Any]] = []
    for scenario in REQUIRED_SCENARIOS:
        items = by_scenario.get(scenario)
        if not items:
            continue
        requests = sum(as_int(item.get("requests")) for item in items)
        completed = sum(as_int(item.get("completed")) for item in items)
        successes = sum(as_int(item.get("successes")) for item in items)
        denominator = completed if completed > 0 else requests
        latencies = [
            item.get("latency_ms") if isinstance(item.get("latency_ms"), dict) else {}
            for item in items
        ]
        summary.append(
            {
                "scenario": scenario,
                "profile_count": len(items),
                "protocols": sorted({str(item.get("protocol", "")) for item in items}),
                "requests": requests,
                "completed": completed,
                "successes": successes,
                "success_rate": successes / denominator if denominator > 0 else None,
                "max_p99_ms": max_optional([as_number(latency.get("p99")) for latency in latencies]),
                "max_p999_ms": max_optional([as_number(latency.get("p999")) for latency in latencies]),
                "max_rps": max_optional([as_number(item.get("achieved_rps")) for item in items]),
            }
        )
    return summary


def profile_reload_phase(profile: dict[str, Any]) -> str | None:
    phase = normalize_reload_phase(profile.get("reload_phase"))
    if phase:
        return phase
    evidence_key = "-".join(
        str(profile.get(part, ""))
        for part in ("protocol", "profile", "source")
    ).lower()
    evidence_key = evidence_key.replace("_", "-").replace("/", "-")
    for candidate in RELOAD_PHASES:
        if f"reload-{candidate}" in evidence_key or f"{candidate}-reload" in evidence_key:
            return candidate
    return None


def merge_profile_response_flags(profiles: list[dict[str, Any]]) -> dict[str, int]:
    merged: dict[str, int] = {}
    for profile in profiles:
        flags = profile.get("profile_response_flags")
        if not isinstance(flags, dict):
            continue
        for flag, count in flags.items():
            parsed = as_int(count)
            if parsed > 0:
                merged[str(flag)] = merged.get(str(flag), 0) + parsed
    return dict(sorted(merged.items()))


def latency_samples(values: list[float | None]) -> dict[str, Any]:
    numeric = [value for value in values if value is not None]
    total = sum(numeric)
    return {
        "observations": len(numeric),
        "sum": total,
        "average": total / len(numeric) if numeric else None,
        "max": max(numeric) if numeric else None,
    }


def reload_phase_summary(profiles: list[dict[str, Any]]) -> list[dict[str, Any]]:
    by_phase: dict[str, list[dict[str, Any]]] = {}
    for profile in profiles:
        phase = profile_reload_phase(profile)
        if phase:
            by_phase.setdefault(phase, []).append(profile)

    summary: list[dict[str, Any]] = []
    for phase in RELOAD_PHASES:
        items = by_phase.get(phase)
        if not items:
            continue
        requests = sum(as_int(item.get("requests")) for item in items)
        completed = sum(as_int(item.get("completed")) for item in items)
        successes = sum(as_int(item.get("successes")) for item in items)
        denominator = completed if completed > 0 else requests
        latencies = [
            item.get("latency_ms") if isinstance(item.get("latency_ms"), dict) else {}
            for item in items
        ]
        summary.append(
            {
                "phase": phase,
                "profile_count": len(items),
                "protocols": sorted({str(item.get("protocol", "")) for item in items}),
                "requests": requests,
                "completed": completed,
                "successes": successes,
                "success_rate": successes / denominator if denominator > 0 else None,
                "max_p95_ms": max_optional([as_number(latency.get("p95")) for latency in latencies]),
                "max_p99_ms": max_optional([as_number(latency.get("p99")) for latency in latencies]),
                "max_p999_ms": max_optional([as_number(latency.get("p999")) for latency in latencies]),
                "response_flags": merge_profile_response_flags(items),
                "ack_latency_ms": latency_samples(
                    [as_number(item.get("xds_ack_latency_ms")) for item in items]
                ),
                "nack_count": sum_profile_metric(items, "xds_nack_count") or 0,
                "last_good_fallback_count": sum_profile_metric(
                    items, "last_good_fallback_count"
                ) or 0,
            }
        )
    return summary


def ordered_live_reload_mutations(values: set[str]) -> list[str]:
    ordered = [
        mutation for mutation in LIVE_RELOAD_REQUIRED_MUTATIONS if mutation in values
    ]
    ordered.extend(sorted(values - set(LIVE_RELOAD_REQUIRED_MUTATIONS)))
    return ordered


def profile_live_reload_mutations(profile: dict[str, Any]) -> list[str]:
    values: set[str] = set()
    mutations = profile.get("reload_mutations")
    if isinstance(mutations, list):
        for item in mutations:
            mutation = normalize_live_reload_mutation(item)
            if mutation:
                values.add(mutation)
    evidence_key = "-".join(
        str(profile.get(part, ""))
        for part in ("protocol", "profile", "source")
    ).lower()
    evidence_key = evidence_key.replace("_", "-").replace("/", "-")
    for mutation in LIVE_RELOAD_REQUIRED_MUTATIONS:
        if mutation in evidence_key:
            values.add(mutation)
    return ordered_live_reload_mutations(values)


def profile_is_live_reload(profile: dict[str, Any]) -> bool:
    return (
        "reload-under-load" in profile_scenarios(profile)
        or profile_reload_phase(profile) is not None
        or bool(profile_live_reload_mutations(profile))
    )


def live_reload_mutation_summary(
    live_profiles: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    by_mutation: dict[str, list[dict[str, Any]]] = {}
    for profile in live_profiles:
        mutations = profile_live_reload_mutations(profile)
        if not mutations:
            mutations = ["unknown"]
        for mutation in mutations:
            by_mutation.setdefault(mutation, []).append(profile)

    summary: list[dict[str, Any]] = []
    for mutation in ordered_live_reload_mutations(set(by_mutation)):
        items = by_mutation[mutation]
        requests = sum(as_int(item.get("requests")) for item in items)
        completed = sum(as_int(item.get("completed")) for item in items)
        successes = sum(as_int(item.get("successes")) for item in items)
        denominator = completed if completed > 0 else requests
        latencies = [
            item.get("latency_ms") if isinstance(item.get("latency_ms"), dict) else {}
            for item in items
        ]
        phases = sorted(
            {
                phase
                for phase in (profile_reload_phase(item) for item in items)
                if phase is not None
            }
        )
        summary.append(
            {
                "mutation": mutation,
                "profile_count": len(items),
                "protocols": sorted({str(item.get("protocol", "")) for item in items}),
                "phases": phases,
                "requests": requests,
                "completed": completed,
                "successes": successes,
                "success_rate": successes / denominator if denominator > 0 else None,
                "max_p99_ms": max_optional([as_number(latency.get("p99")) for latency in latencies]),
                "max_p999_ms": max_optional([as_number(latency.get("p999")) for latency in latencies]),
                "response_flags": merge_profile_response_flags(items),
                "ack_latency_ms": latency_samples(
                    [as_number(item.get("xds_ack_latency_ms")) for item in items]
                ),
                "nack_count": sum_profile_metric(items, "xds_nack_count") or 0,
                "last_good_fallback_count": sum_profile_metric(
                    items, "last_good_fallback_count"
                ) or 0,
            }
        )
    return summary


def live_reload_traffic_summary(profiles: list[dict[str, Any]]) -> dict[str, Any]:
    live_profiles = [profile for profile in profiles if profile_is_live_reload(profile)]
    observed_protocols = sorted({str(profile.get("protocol", "")) for profile in live_profiles})
    observed_mutations = ordered_live_reload_mutations(
        {
            mutation
            for profile in live_profiles
            for mutation in profile_live_reload_mutations(profile)
        }
    )
    return {
        "profile_count": len(live_profiles),
        "required_protocols": LIVE_RELOAD_REQUIRED_PROTOCOLS,
        "observed_protocols": observed_protocols,
        "missing_protocols": sorted(
            set(LIVE_RELOAD_REQUIRED_PROTOCOLS) - set(observed_protocols)
        ),
        "required_mutations": LIVE_RELOAD_REQUIRED_MUTATIONS,
        "observed_mutations": observed_mutations,
        "missing_mutations": [
            mutation
            for mutation in LIVE_RELOAD_REQUIRED_MUTATIONS
            if mutation not in observed_mutations
        ],
        "mutation_summary": live_reload_mutation_summary(live_profiles),
    }


def metric_value(metrics: str, name: str) -> float | None:
    pattern = re.compile(rf"^{re.escape(name)}(?:\{{[^}}]*\}})?\s+([-+0-9.eE]+)\s*$")
    for raw in metrics.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        match = pattern.match(line)
        if match:
            return as_number(match.group(1))
    return None


def metric_label_values(metrics: str, name: str, label: str) -> dict[str, int]:
    pattern = re.compile(
        rf"^{re.escape(name)}\{{[^}}]*\b{re.escape(label)}=\"((?:\\.|[^\"])*)\"[^}}]*\}}\s+([-+0-9.eE]+)\s*$"
    )
    values: dict[str, int] = {}
    for raw in metrics.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        match = pattern.match(line)
        if not match:
            continue
        label_value = match.group(1).replace('\\"', '"').replace("\\\\", "\\")
        values[label_value] = int(float(match.group(2)))
    return values


def metric_histogram(metrics: str, family: str) -> tuple[list[dict[str, Any]], float | None, int | None]:
    buckets = [
        {"le": le, "cumulative_count": count}
        for le, count in metric_label_values(metrics, f"{family}_bucket", "le").items()
    ]
    buckets.sort(key=lambda item: bucket_sort_key(item["le"]))
    total_sum = metric_value(metrics, f"{family}_sum")
    total_count_raw = metric_value(metrics, f"{family}_count")
    total_count = int(total_count_raw) if total_count_raw is not None else None
    return buckets, total_sum, total_count


def bucket_sort_key(le: str) -> tuple[int, float]:
    if le == "+Inf":
        return (1, math.inf)
    try:
        return (0, float(le))
    except ValueError:
        return (0, math.inf)


def quantile_from_buckets(
    buckets: list[dict[str, Any]],
    quantile: float,
    total_count: int | None,
) -> float | None:
    if total_count is None:
        for bucket in reversed(buckets):
            count = as_int(bucket.get("cumulative_count"))
            if count > 0:
                total_count = count
                break
    if total_count is None or total_count <= 0:
        return None
    rank = max(1, int(math.ceil(total_count * quantile)))
    for bucket in sorted(buckets, key=lambda item: bucket_sort_key(str(item.get("le", "")))):
        if as_int(bucket.get("cumulative_count")) < rank:
            continue
        upper = str(bucket.get("le", ""))
        if upper == "+Inf":
            return None
        return as_number(upper)
    return None


def aggregate_profile_status_classes(profiles: list[dict[str, Any]]) -> dict[str, int]:
    classes = {"1xx": 0, "2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0, "other": 0}
    for profile in profiles:
        for status, count in profile.get("status_counts", {}).items():
            try:
                code = int(status)
            except (TypeError, ValueError):
                classes["other"] += as_int(count)
                continue
            if 100 <= code <= 199:
                classes["1xx"] += as_int(count)
            elif 200 <= code <= 299:
                classes["2xx"] += as_int(count)
            elif 300 <= code <= 399:
                classes["3xx"] += as_int(count)
            elif 400 <= code <= 499:
                classes["4xx"] += as_int(count)
            elif 500 <= code <= 599:
                classes["5xx"] += as_int(count)
            else:
                classes["other"] += as_int(count)
    return classes


def status_classes(traffic: dict[str, Any], metrics: str, profiles: list[dict[str, Any]]) -> dict[str, int]:
    values = {
        "1xx": as_int(traffic.get("status_1xx"), -1),
        "2xx": as_int(traffic.get("status_2xx"), -1),
        "3xx": as_int(traffic.get("status_3xx"), -1),
        "4xx": as_int(traffic.get("status_4xx"), -1),
        "5xx": as_int(traffic.get("status_5xx"), -1),
        "other": as_int(traffic.get("status_other"), -1),
    }
    if all(value >= 0 for value in values.values()):
        return values
    metric_values = {
        "1xx": metric_value(metrics, "nantian_gateway_dataplane_traffic_status_1xx_total"),
        "2xx": metric_value(metrics, "nantian_gateway_dataplane_traffic_status_2xx_total"),
        "3xx": metric_value(metrics, "nantian_gateway_dataplane_traffic_status_3xx_total"),
        "4xx": metric_value(metrics, "nantian_gateway_dataplane_traffic_status_4xx_total"),
        "5xx": metric_value(metrics, "nantian_gateway_dataplane_traffic_status_5xx_total"),
        "other": metric_value(metrics, "nantian_gateway_dataplane_traffic_status_other_total"),
    }
    if all(value is not None for value in metric_values.values()):
        return {key: int(value or 0) for key, value in metric_values.items()}
    return aggregate_profile_status_classes(profiles)


def collect_response_flags(traffic: dict[str, Any], metrics: str) -> dict[str, int]:
    flags = traffic.get("response_flags")
    if isinstance(flags, dict):
        return {str(key): as_int(value) for key, value in flags.items() if as_int(value) > 0}
    metric_flags = metric_label_values(metrics, "nantian_gateway_dataplane_traffic_response_flags_total", "flag")
    return {key: value for key, value in metric_flags.items() if value > 0}


def metric_labels(line: str) -> dict[str, str]:
    if "{" not in line or "}" not in line:
        return {}
    raw_labels = line.split("{", 1)[1].split("}", 1)[0]
    labels: dict[str, str] = {}
    for item in raw_labels.split(","):
        if "=" not in item:
            continue
        key, value = item.split("=", 1)
        labels[key.strip()] = value.strip().strip('"')
    return labels


def traffic_metric_high_cardinality_labels(metrics: str) -> list[str]:
    forbidden = {
        "route",
        "route_namespace",
        "route_name",
        "backend",
        "backend_namespace",
        "backend_name",
        "pod",
        "endpoint",
    }
    found: set[str] = set()
    for raw in metrics.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if not line.startswith("nantian_gateway_dataplane_traffic_"):
            continue
        found.update(label for label in metric_labels(line) if label in forbidden)
    return sorted(found)


def traffic_node_kind(node_id: str) -> str:
    if node_id.startswith("plane:"):
        return "plane"
    if node_id.startswith("listener:"):
        return "listener"
    if node_id.startswith("route:"):
        return "route"
    if node_id.startswith("backend:"):
        return "backend"
    if node_id.startswith("endpoint-set:"):
        return "endpoint_set"
    if node_id.startswith("endpoint:"):
        return "endpoint"
    return "unknown"


def collect_traffic_graph(traffic: dict[str, Any]) -> dict[str, Any]:
    nodes = traffic.get("nodes") if isinstance(traffic.get("nodes"), list) else []
    edges = traffic.get("edges") if isinstance(traffic.get("edges"), list) else []
    node_kinds: dict[str, int] = {}
    edge_kinds: dict[str, int] = {}

    for node in nodes:
        if not isinstance(node, dict):
            continue
        kind = traffic_node_kind(str(node.get("node_id", "")))
        node_kinds[kind] = node_kinds.get(kind, 0) + 1

    for edge in edges:
        if not isinstance(edge, dict):
            continue
        source_kind = traffic_node_kind(str(edge.get("source", "")))
        target_kind = traffic_node_kind(str(edge.get("target", "")))
        kind = f"{source_kind}_to_{target_kind}"
        edge_kinds[kind] = edge_kinds.get(kind, 0) + 1

    endpoint_count = node_kinds.get("endpoint", 0) + node_kinds.get("endpoint_set", 0)
    return {
        "node_count": len(nodes),
        "edge_count": len(edges),
        "node_kinds": dict(sorted(node_kinds.items())),
        "edge_kinds": dict(sorted(edge_kinds.items())),
        "has_route_topology": node_kinds.get("route", 0) > 0,
        "has_backend_topology": node_kinds.get("backend", 0) > 0,
        "has_endpoint_topology": endpoint_count > 0,
    }


def request_latency_histogram_series(traffic: dict[str, Any], metrics: str) -> int:
    histograms = traffic.get("request_latency_ms_histograms")
    if isinstance(histograms, list) and histograms:
        return len(histograms)

    series: set[tuple[tuple[str, str], ...]] = set()
    family = "nantian_gateway_dataplane_traffic_request_latency_ms_"
    for raw in metrics.splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or not line.startswith(family):
            continue
        labels = metric_labels(line)
        labels.pop("le", None)
        if labels:
            series.add(tuple(sorted(labels.items())))
    return len(series)


def metric_samples(metrics: str, name: str) -> list[tuple[dict[str, str], float]]:
    pattern = re.compile(rf"^{re.escape(name)}(?:\{{[^}}]*\}})?\s+([-+0-9.eE]+)\s*$")
    samples: list[tuple[dict[str, str], float]] = []
    for raw in metrics.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        match = pattern.match(line)
        if not match:
            continue
        value = as_number(match.group(1))
        if value is None:
            continue
        samples.append((metric_labels(line), value))
    return samples


def metric_total(metrics: str, name: str) -> int:
    samples = metric_samples(metrics, name)
    for labels, value in samples:
        if labels.get("scope") == "total":
            return int(value)
    for labels, value in samples:
        if not labels:
            return int(value)
    value = metric_value(metrics, name)
    return as_int(value)


def first_metric_total(metrics: str, names: list[str]) -> int:
    for name in names:
        samples = metric_samples(metrics, name)
        if samples:
            return metric_total(metrics, name)
    return 0


def first_metric_histogram(
    metrics: str,
    families: list[str],
) -> tuple[str | None, list[dict[str, Any]], float | None, int | None]:
    for family in families:
        buckets, total_sum, total_count = metric_histogram(metrics, family)
        if buckets or total_sum is not None or total_count is not None:
            return family, buckets, total_sum, total_count
    return None, [], None, None


def xds_apply_stage_histograms(metrics: str) -> dict[str, Any]:
    family = "nantian_gateway_dataplane_xds_apply_stage_duration_ms"
    buckets_by_stage: dict[str, list[dict[str, Any]]] = {}
    sums_by_stage: dict[str, float] = {}
    counts_by_stage: dict[str, int] = {}

    for labels, value in metric_samples(metrics, f"{family}_bucket"):
        stage = labels.get("stage")
        le = labels.get("le")
        if not stage or not le:
            continue
        buckets_by_stage.setdefault(stage, []).append(
            {"le": le, "cumulative_count": int(value)}
        )
    for labels, value in metric_samples(metrics, f"{family}_sum"):
        stage = labels.get("stage")
        if stage:
            sums_by_stage[stage] = value
    for labels, value in metric_samples(metrics, f"{family}_count"):
        stage = labels.get("stage")
        if stage:
            counts_by_stage[stage] = int(value)

    stages: list[dict[str, Any]] = []
    for stage in sorted(set(buckets_by_stage) | set(sums_by_stage) | set(counts_by_stage)):
        buckets = sorted(
            buckets_by_stage.get(stage, []),
            key=lambda item: bucket_sort_key(str(item.get("le", ""))),
        )
        observations = counts_by_stage.get(stage)
        if observations is None:
            for bucket in reversed(buckets):
                count = as_int(bucket.get("cumulative_count"))
                if count > 0:
                    observations = count
                    break
        if observations is None:
            observations = 0
        total_sum = sums_by_stage.get(stage)
        average = total_sum / observations if total_sum is not None and observations > 0 else None
        stages.append(
            {
                "stage": stage,
                "observations": observations,
                "sum": total_sum,
                "average": average,
                "p95": quantile_from_buckets(buckets, 0.95, observations or None),
                "p99": quantile_from_buckets(buckets, 0.99, observations or None),
                "buckets": buckets,
            }
        )

    return {"series": len(stages), "stages": stages}


def xds_reload_metrics(metrics: str) -> dict[str, int]:
    return {
        "connect_failures": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_xds_connect_failures_total")
        ),
        "stream_failures": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_xds_stream_failures_total")
        ),
        "snapshots_applied": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_xds_snapshots_applied_total")
        ),
        "snapshots_nacked": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_xds_snapshots_nacked_total")
        ),
        "snapshots_skipped": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_xds_snapshots_skipped_total")
        ),
        "last_apply_timestamp_seconds": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_xds_last_apply_timestamp_seconds")
        ),
    }


def fault_isolation_metrics(metrics: str) -> dict[str, Any]:
    http_overload = metric_total(
        metrics, "nantian_gateway_dataplane_http_overload_rejected_total"
    )
    tcp_overload = metric_total(
        metrics, "nantian_gateway_dataplane_tcp_overload_rejected_total"
    )
    udp_overload = metric_total(
        metrics, "nantian_gateway_dataplane_udp_overload_rejected_total"
    )
    circuit_open = metric_total(
        metrics, "nantian_gateway_dataplane_http_circuit_breaker_rejected_total"
    )
    rate_limit_rejected = metric_total(
        metrics, "nantian_gateway_dataplane_http_rate_limit_rejected_total"
    )
    retry_budget_exhausted = first_metric_total(
        metrics,
        [
            "nantian_gateway_dataplane_http_retry_budget_rejected_total",
            "nantian_gateway_dataplane_http_retry_budget_retry_rejected_total",
            "nantian_gateway_dataplane_retry_budget_rejected_total",
        ],
    )
    passive_ejections = first_metric_total(
        metrics,
        [
            "nantian_gateway_dataplane_endpoint_passive_ejected_current",
            "nantian_gateway_dataplane_endpoint_passive_ejections_total",
            "nantian_gateway_dataplane_http_endpoint_passive_ejected_current",
            "nantian_gateway_dataplane_http_endpoint_passive_ejections_total",
        ],
    )
    active_unhealthy = first_metric_total(
        metrics,
        [
            "nantian_gateway_dataplane_endpoint_active_unhealthy_current",
            "nantian_gateway_dataplane_endpoint_active_unhealthy_total",
            "nantian_gateway_dataplane_http_endpoint_active_unhealthy_current",
        ],
    )
    recovery_family, recovery_buckets, recovery_sum, recovery_count = first_metric_histogram(
        metrics,
        [
            "nantian_gateway_dataplane_endpoint_recovery_latency_ms",
            "nantian_gateway_dataplane_http_endpoint_recovery_latency_ms",
        ],
    )
    recovery_observations = recovery_count
    if recovery_observations is None:
        for bucket in reversed(recovery_buckets):
            count = as_int(bucket.get("cumulative_count"))
            if count > 0:
                recovery_observations = count
                break
    if recovery_observations is None:
        recovery_observations = 0
    recovery_average = (
        recovery_sum / recovery_observations
        if recovery_sum is not None and recovery_observations > 0
        else None
    )

    fast_fail_total = (
        http_overload
        + tcp_overload
        + udp_overload
        + circuit_open
        + rate_limit_rejected
        + retry_budget_exhausted
    )

    return {
        "fast_fail_total": fast_fail_total,
        "http_overload_rejected_total": http_overload,
        "tcp_overload_rejected_total": tcp_overload,
        "udp_overload_rejected_total": udp_overload,
        "circuit_open_total": circuit_open,
        "rate_limit_rejected_total": rate_limit_rejected,
        "retry_budget_exhausted_total": retry_budget_exhausted,
        "passive_ejection_total": passive_ejections,
        "active_unhealthy_total": active_unhealthy,
        "last_good_snapshot_active": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_serving_last_good_snapshot")
        )
        > 0,
        "current_snapshot_rejected": as_int(
            metric_value(metrics, "nantian_gateway_dataplane_current_snapshot_rejected")
        )
        > 0,
        "recovery_latency_ms": {
            "source_metric": recovery_family,
            "observations": recovery_observations,
            "sum": recovery_sum,
            "average": recovery_average,
            "p95": quantile_from_buckets(
                recovery_buckets, 0.95, recovery_observations or None
            ),
            "p99": quantile_from_buckets(
                recovery_buckets, 0.99, recovery_observations or None
            ),
            "buckets": recovery_buckets,
        },
    }


def sum_profile_metric(
    profiles: list[dict[str, Any]],
    primary: str,
    fallback: str | None = None,
) -> int | None:
    values = [as_number(profile.get(primary)) for profile in profiles]
    values = [value for value in values if value is not None]
    if values:
        return int(sum(values))
    if fallback is None:
        return None
    return sum(as_int(profile.get(fallback)) for profile in profiles)


def max_profile_metric(profiles: list[dict[str, Any]], name: str) -> int:
    values = [as_number(profile.get(name)) for profile in profiles]
    values = [value for value in values if value is not None]
    return int(max(values)) if values else 0


def udp_profile_scenarios(profile: dict[str, Any]) -> list[str]:
    values: set[str] = set()
    scenarios = profile.get("udp_scenarios")
    if isinstance(scenarios, list):
        values.update(str(item) for item in scenarios)
    evidence_key = "-".join(
        str(profile.get(part, ""))
        for part in ("protocol", "profile", "source")
    ).lower()
    evidence_key = evidence_key.replace("_", "-").replace("/", "-")
    for scenario in set(REQUIRED_SCENARIOS) | set(UDP_REQUIRED_SCENARIOS):
        if scenario in evidence_key:
            values.add(scenario)
    return sorted(values)


def udp_scenario_summary(udp_profiles: list[dict[str, Any]]) -> list[dict[str, Any]]:
    by_scenario: dict[str, list[dict[str, Any]]] = {}
    for profile in udp_profiles:
        for scenario in udp_profile_scenarios(profile):
            by_scenario.setdefault(scenario, []).append(profile)

    summary: list[dict[str, Any]] = []
    for scenario in sorted(by_scenario):
        items = by_scenario[scenario]
        requests = sum(as_int(item.get("requests")) for item in items)
        completed = sum(as_int(item.get("completed")) for item in items)
        successes = sum(as_int(item.get("successes")) for item in items)
        denominator = completed if completed > 0 else requests
        datagrams_sent = sum_profile_metric(items, "datagrams_sent", "requests") or 0
        datagrams_received = sum_profile_metric(items, "datagrams_received", "successes") or 0
        explicit_lost = sum_profile_metric(items, "datagrams_lost")
        datagrams_lost = (
            explicit_lost
            if explicit_lost is not None
            else max(datagrams_sent - datagrams_received, 0)
        )
        latencies = [
            item.get("latency_ms") if isinstance(item.get("latency_ms"), dict) else {}
            for item in items
        ]
        summary.append(
            {
                "scenario": scenario,
                "profile_count": len(items),
                "protocols": sorted({str(item.get("protocol", "")) for item in items}),
                "requests": requests,
                "completed": completed,
                "successes": successes,
                "success_rate": successes / denominator if denominator > 0 else None,
                "datagrams_sent": datagrams_sent,
                "datagrams_received": datagrams_received,
                "datagrams_lost": datagrams_lost,
                "packet_loss_rate": datagrams_lost / datagrams_sent if datagrams_sent > 0 else None,
                "max_p99_ms": max_optional([as_number(latency.get("p99")) for latency in latencies]),
                "max_p999_ms": max_optional([as_number(latency.get("p999")) for latency in latencies]),
                "max_rps": max_optional([as_number(item.get("achieved_rps")) for item in items]),
                "client_count": max_profile_metric(items, "client_count"),
                "upstream_count": max_profile_metric(items, "upstream_count"),
                "session_opens": sum_profile_metric(items, "session_opens") or 0,
                "session_evictions": sum_profile_metric(items, "session_evictions") or 0,
            }
        )
    return summary


def profile_resource_number(profile: dict[str, Any], name: str) -> float | None:
    sample = profile.get("resource_sample")
    if not isinstance(sample, dict):
        return None
    return as_number(sample.get(name))


def metric_delta(current: Any, baseline: Any) -> float | None:
    current_value = as_number(current)
    baseline_value = as_number(baseline)
    if current_value is None or baseline_value is None:
        return None
    return current_value - baseline_value


def reduction_ratio(delta: Any, baseline: Any) -> float | None:
    delta_value = as_number(delta)
    baseline_value = as_number(baseline)
    if delta_value is None or baseline_value is None or baseline_value <= 0:
        return None
    return -delta_value / baseline_value


def udp_resource_point(profile: dict[str, Any]) -> dict[str, Any]:
    latency = profile.get("latency_ms") if isinstance(profile.get("latency_ms"), dict) else {}
    return {
        "profile": profile.get("profile"),
        "source": profile.get("source"),
        "p99_ms": as_number(latency.get("p99")),
        "p999_ms": as_number(latency.get("p999")),
        "rss_kib": profile_resource_number(profile, "rss_kib"),
        "fd_count": profile_resource_number(profile, "fd_count"),
        "threads": profile_resource_number(profile, "threads"),
        "cpu_millicores": profile_resource_number(profile, "cpu_millicores"),
        "throughput_rps": as_number(profile.get("achieved_rps")),
        "packet_loss_rate": None,
    }


def udp_resource_comparison(
    group: str,
    baseline: dict[str, Any],
    current: dict[str, Any],
) -> dict[str, Any]:
    baseline_point = udp_resource_point(baseline)
    current_point = udp_resource_point(current)
    p99_delta = metric_delta(current_point.get("p99_ms"), baseline_point.get("p99_ms"))
    rss_delta = metric_delta(current_point.get("rss_kib"), baseline_point.get("rss_kib"))
    rps_delta = metric_delta(
        current_point.get("throughput_rps"),
        baseline_point.get("throughput_rps"),
    )
    return {
        "group": group,
        "baseline_profile": baseline.get("profile"),
        "current_profile": current.get("profile"),
        "baseline": baseline_point,
        "current": current_point,
        "delta": {
            "p99_ms": p99_delta,
            "rss_kib": rss_delta,
            "throughput_rps": rps_delta,
        },
        "improvement": {
            "p99_reduction_ratio": reduction_ratio(p99_delta, baseline_point.get("p99_ms")),
            "rss_reduction_ratio": reduction_ratio(rss_delta, baseline_point.get("rss_kib")),
        },
        "improved": (p99_delta is not None and p99_delta <= 0)
        and (rss_delta is not None and rss_delta <= 0),
    }


def udp_resource_improvements(udp_profiles: list[dict[str, Any]]) -> dict[str, Any]:
    grouped: dict[str, list[dict[str, Any]]] = {}
    for profile in udp_profiles:
        group = profile.get("comparison_group")
        if not isinstance(group, str) or not group:
            continue
        variant = profile.get("variant")
        if variant not in {"baseline", "current"}:
            continue
        if profile_resource_number(profile, "rss_kib") is None:
            continue
        latency = profile.get("latency_ms") if isinstance(profile.get("latency_ms"), dict) else {}
        if as_number(latency.get("p99")) is None:
            continue
        grouped.setdefault(group, []).append(profile)

    comparisons: list[dict[str, Any]] = []
    for group in sorted(grouped):
        profiles_for_group = grouped[group]
        baselines = [item for item in profiles_for_group if item.get("variant") == "baseline"]
        currents = [item for item in profiles_for_group if item.get("variant") == "current"]
        if not baselines or not currents:
            continue
        baseline = sorted(baselines, key=lambda item: str(item.get("profile", "")))[0]
        current = sorted(currents, key=lambda item: str(item.get("profile", "")))[0]
        comparisons.append(udp_resource_comparison(group, baseline, current))

    return {
        "comparison_count": len(comparisons),
        "all_improved": bool(comparisons)
        and all(bool(item.get("improved")) for item in comparisons),
        "comparisons": comparisons,
    }


def udp_route_summary(profiles: list[dict[str, Any]], metrics: str) -> dict[str, Any]:
    udp_profiles = [profile for profile in profiles if profile.get("protocol") == "udp"]
    datagrams_sent = sum_profile_metric(udp_profiles, "datagrams_sent", "requests") or 0
    datagrams_received = (
        sum_profile_metric(udp_profiles, "datagrams_received", "successes") or 0
    )
    explicit_lost = sum_profile_metric(udp_profiles, "datagrams_lost")
    datagrams_lost = (
        explicit_lost
        if explicit_lost is not None
        else max(datagrams_sent - datagrams_received, 0)
    )
    packet_loss_rate = datagrams_lost / datagrams_sent if datagrams_sent > 0 else None
    max_p99 = max_optional(
        [
            as_number(
                profile.get("latency_ms", {}).get("p99")
                if isinstance(profile.get("latency_ms"), dict)
                else None
            )
            for profile in udp_profiles
        ]
    )
    observed_udp_scenarios = sorted(
        {scenario for profile in udp_profiles for scenario in udp_profile_scenarios(profile)}
    )
    missing_udp_scenarios = [
        scenario for scenario in UDP_REQUIRED_SCENARIOS if scenario not in observed_udp_scenarios
    ]

    return {
        "profile_count": len(udp_profiles),
        "datagrams_sent": datagrams_sent,
        "datagrams_received": datagrams_received,
        "datagrams_lost": datagrams_lost,
        "packet_loss_rate": packet_loss_rate,
        "max_p99_ms": max_p99,
        "coverage": {
            "required_scenarios": UDP_REQUIRED_SCENARIOS,
            "observed_scenarios": observed_udp_scenarios,
            "missing_scenarios": missing_udp_scenarios,
            "scenario_summary": udp_scenario_summary(udp_profiles),
        },
        "session_churn": {
            "session_opens": sum_profile_metric(udp_profiles, "session_opens") or 0,
            "session_evictions": sum_profile_metric(udp_profiles, "session_evictions") or 0,
            "max_client_count": max_profile_metric(udp_profiles, "client_count"),
            "max_upstream_count": max_profile_metric(udp_profiles, "upstream_count"),
        },
        "resource_improvements": udp_resource_improvements(udp_profiles),
        "sessions": {
            "active_current": as_int(
                metric_value(
                    metrics,
                    "nantian_gateway_dataplane_udp_sessions_active_current",
                )
            ),
            "queue_depth_current": as_int(
                metric_value(
                    metrics,
                    "nantian_gateway_dataplane_udp_session_queue_depth_current",
                )
            ),
            "queue_overflow_dropped_total": as_int(
                metric_value(
                    metrics,
                    "nantian_gateway_dataplane_udp_session_queue_overflow_dropped_total",
                )
            ),
            "idle_evictions_total": as_int(
                metric_value(
                    metrics,
                    "nantian_gateway_dataplane_udp_session_idle_evictions_total",
                )
            ),
            "active_by_listener": metric_label_values(
                metrics,
                "nantian_gateway_dataplane_udp_sessions_active_listener_current",
                "listener",
            ),
            "queue_depth_by_listener": metric_label_values(
                metrics,
                "nantian_gateway_dataplane_udp_session_queue_depth_listener_current",
                "listener",
            ),
            "queue_overflow_dropped_by_listener": metric_label_values(
                metrics,
                "nantian_gateway_dataplane_udp_session_queue_overflow_dropped_listener_total",
                "listener",
            ),
            "idle_evictions_by_listener": metric_label_values(
                metrics,
                "nantian_gateway_dataplane_udp_session_idle_evictions_listener_total",
                "listener",
            ),
        },
    }


def has_stream_resource_sample(profile: dict[str, Any]) -> bool:
    sample = profile.get("resource_sample")
    if not isinstance(sample, dict):
        return False
    return any(
        as_number(sample.get(name)) is not None
        for name in ("rss_kib", "fd_count", "threads", "cpu_millicores")
    )


def per_connection(value: Any, connections: float | None) -> float | None:
    parsed = as_number(value)
    if parsed is None or connections is None or connections <= 0:
        return None
    return parsed / connections


def stream_resource_curve(profiles: list[dict[str, Any]]) -> dict[str, Any]:
    points: list[dict[str, Any]] = []
    for profile in profiles:
        protocol = str(profile.get("protocol", ""))
        if protocol not in STREAM_RESOURCE_PROTOCOLS or not has_stream_resource_sample(profile):
            continue
        connections = as_number(profile.get("connection_count"))
        if connections is None:
            connections = as_number(profile.get("concurrency"))
        if connections is None or connections <= 0:
            continue
        sample = profile.get("resource_sample")
        latency = profile.get("latency_ms") if isinstance(profile.get("latency_ms"), dict) else {}
        if not isinstance(sample, dict):
            continue
        rss_kib = as_number(sample.get("rss_kib"))
        fd_count = as_number(sample.get("fd_count"))
        threads = as_number(sample.get("threads"))
        points.append(
            {
                "protocol": protocol,
                "profile": profile.get("profile"),
                "source": profile.get("source"),
                "connections": connections,
                "concurrency": as_number(profile.get("concurrency")),
                "success_rate": as_number(profile.get("success_rate")),
                "p99_ms": as_number(latency.get("p99")),
                "p999_ms": as_number(latency.get("p999")),
                "throughput_rps": as_number(profile.get("achieved_rps")),
                "rss_kib": rss_kib,
                "fd_count": fd_count,
                "threads": threads,
                "cpu_millicores": as_number(sample.get("cpu_millicores")),
                "rss_per_connection_kib": per_connection(rss_kib, connections),
                "fd_per_connection": per_connection(fd_count, connections),
                "threads_per_connection": per_connection(threads, connections),
                "buffer_size_bytes": as_number(sample.get("buffer_size_bytes")),
            }
        )
    points.sort(
        key=lambda item: (
            str(item.get("protocol", "")),
            as_number(item.get("connections")) or 0,
            str(item.get("profile", "")),
        )
    )
    return {
        "point_count": len(points),
        "protocols": sorted({str(point.get("protocol", "")) for point in points}),
        "max_connections": max_optional([as_number(point.get("connections")) for point in points]),
        "max_rss_kib": max_optional([as_number(point.get("rss_kib")) for point in points]),
        "max_fd_count": max_optional([as_number(point.get("fd_count")) for point in points]),
        "max_threads": max_optional([as_number(point.get("threads")) for point in points]),
        "max_p99_ms": max_optional([as_number(point.get("p99_ms")) for point in points]),
        "max_rps": max_optional([as_number(point.get("throughput_rps")) for point in points]),
        "points": points,
    }


def stream_buffer_point(profile: dict[str, Any]) -> dict[str, Any]:
    latency = profile.get("latency_ms") if isinstance(profile.get("latency_ms"), dict) else {}
    connections = as_number(profile.get("connection_count"))
    if connections is None:
        connections = as_number(profile.get("concurrency"))
    return {
        "protocol": profile.get("protocol"),
        "profile": profile.get("profile"),
        "source": profile.get("source"),
        "buffer_size_bytes": profile_resource_number(profile, "buffer_size_bytes"),
        "connections": connections,
        "p99_ms": as_number(latency.get("p99")),
        "p999_ms": as_number(latency.get("p999")),
        "rss_kib": profile_resource_number(profile, "rss_kib"),
        "fd_count": profile_resource_number(profile, "fd_count"),
        "threads": profile_resource_number(profile, "threads"),
        "cpu_millicores": profile_resource_number(profile, "cpu_millicores"),
        "throughput_rps": as_number(profile.get("achieved_rps")),
    }


def stream_buffer_comparison(
    group: str,
    baseline: dict[str, Any],
    current: dict[str, Any],
) -> dict[str, Any]:
    baseline_point = stream_buffer_point(baseline)
    current_point = stream_buffer_point(current)
    p99_delta = metric_delta(current_point.get("p99_ms"), baseline_point.get("p99_ms"))
    rss_delta = metric_delta(current_point.get("rss_kib"), baseline_point.get("rss_kib"))
    rps_delta = metric_delta(
        current_point.get("throughput_rps"),
        baseline_point.get("throughput_rps"),
    )
    return {
        "group": group,
        "baseline_profile": baseline.get("profile"),
        "current_profile": current.get("profile"),
        "baseline": baseline_point,
        "current": current_point,
        "delta": {
            "p99_ms": p99_delta,
            "rss_kib": rss_delta,
            "throughput_rps": rps_delta,
        },
        "improvement": {
            "p99_reduction_ratio": reduction_ratio(p99_delta, baseline_point.get("p99_ms")),
            "rss_reduction_ratio": reduction_ratio(rss_delta, baseline_point.get("rss_kib")),
        },
        "improved": (p99_delta is not None and p99_delta <= 0)
        and (rss_delta is not None and rss_delta <= 0),
    }


def stream_buffer_evaluation(profiles: list[dict[str, Any]]) -> dict[str, Any]:
    grouped: dict[str, list[dict[str, Any]]] = {}
    for profile in profiles:
        protocol = str(profile.get("protocol", ""))
        if protocol not in STREAM_RESOURCE_PROTOCOLS:
            continue
        group = profile.get("comparison_group")
        if not isinstance(group, str) or not group:
            continue
        variant = profile.get("variant")
        if variant not in {"baseline", "current"}:
            continue
        if profile_resource_number(profile, "buffer_size_bytes") is None:
            continue
        if profile_resource_number(profile, "rss_kib") is None:
            continue
        latency = profile.get("latency_ms") if isinstance(profile.get("latency_ms"), dict) else {}
        if as_number(latency.get("p99")) is None:
            continue
        grouped.setdefault(group, []).append(profile)

    comparisons: list[dict[str, Any]] = []
    for group in sorted(grouped):
        profiles_for_group = grouped[group]
        baselines = [item for item in profiles_for_group if item.get("variant") == "baseline"]
        currents = [item for item in profiles_for_group if item.get("variant") == "current"]
        if not baselines or not currents:
            continue
        baseline = sorted(baselines, key=lambda item: str(item.get("profile", "")))[0]
        current = sorted(currents, key=lambda item: str(item.get("profile", "")))[0]
        comparisons.append(stream_buffer_comparison(group, baseline, current))

    return {
        "comparison_count": len(comparisons),
        "all_improved": bool(comparisons)
        and all(bool(item.get("improved")) for item in comparisons),
        "comparisons": comparisons,
    }


def resource_summary() -> dict[str, dict[str, int | None]]:
    candidates = [
        input_dir / "resources" / "after.tsv",
        input_dir / "resources-after.tsv",
        input_dir / "after.tsv",
    ]
    path = first_file(candidates)
    if path is None:
        return {}
    summary: dict[str, dict[str, int | None]] = {}
    with path.open(encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle, delimiter="\t")
        for row in reader:
            component = row.get("component") or "unknown"
            current = summary.setdefault(
                component,
                {"fd_count": 0, "rss_kib": 0, "threads": 0, "cpu_millicores": None},
            )
            current["fd_count"] = int(current["fd_count"] or 0) + as_int(row.get("fd_count"))
            current["rss_kib"] = int(current["rss_kib"] or 0) + as_int(row.get("rss_kib"))
            current["threads"] = int(current["threads"] or 0) + as_int(row.get("threads"))
            cpu = as_number(row.get("cpu_millicores"))
            if cpu is not None:
                current["cpu_millicores"] = int(current["cpu_millicores"] or 0) + int(cpu)
    return summary


def fmt_number(value: Any) -> str:
    parsed = as_number(value)
    if parsed is None:
        return "n/a"
    if parsed == 0:
        return "0"
    text = f"{parsed:.2f}"
    return text.rstrip("0").rstrip(".")


def fmt_percent(value: Any) -> str:
    parsed = as_number(value)
    if parsed is None:
        return "n/a"
    return f"{parsed * 100:.2f}%"


def fmt_ms(value: Any) -> str:
    parsed = as_number(value)
    if parsed is None:
        return "n/a"
    return f"{fmt_number(parsed)}ms"


def fmt_flags(flags: Any) -> str:
    if not isinstance(flags, dict) or not flags:
        return "none"
    return ", ".join(f"{flag}={count}" for flag, count in sorted(flags.items()))


def profile_row(profile: dict[str, Any]) -> str:
    latency = profile["latency_ms"]
    return (
        f"| {profile['protocol']} | {profile['profile']} | {profile['requests']} | "
        f"{profile['concurrency']} | {fmt_percent(profile['success_rate'])} | "
        f"{fmt_number(latency.get('p50'))} | {fmt_number(latency.get('p90'))} | "
        f"{fmt_number(latency.get('p95'))} | {fmt_number(latency.get('p99'))} | "
        f"{fmt_number(latency.get('p999'))} | {fmt_number(latency.get('max'))} | "
        f"{fmt_number(profile.get('achieved_rps'))} |"
    )


def classify_bottlenecks(
    scenario_rows: list[dict[str, Any]],
    pool_hit_ratio: float | None,
    connect_p99: float | None,
    max_profile_p99: float | None,
    observability: dict[str, Any],
    reload: dict[str, Any],
    resources: dict[str, dict[str, int | None]],
) -> dict[str, dict[str, Any]]:
    scenarios = {item["scenario"]: item for item in scenario_rows}

    upstream_evidence: list[str] = []
    for scenario in ("backend-slow-read", "backend-slow-write"):
        item = scenarios.get(scenario)
        if item and as_number(item.get("max_p99_ms")) is not None:
            upstream_evidence.append(f"{scenario} max p99 {fmt_ms(item.get('max_p99_ms'))}")

    fault_evidence: list[str] = []
    for scenario in ("backend-error", "endpoint-flapping"):
        item = scenarios.get(scenario)
        if item and as_number(item.get("success_rate")) is not None:
            fault_evidence.append(f"{scenario} success rate {fmt_percent(item.get('success_rate'))}")

    reload_evidence: list[str] = []
    reload_item = scenarios.get("reload-under-load")
    if reload_item and as_number(reload_item.get("success_rate")) is not None:
        reload_evidence.append(
            f"reload-under-load success rate {fmt_percent(reload_item.get('success_rate'))}"
        )
    xds = reload.get("xds") if isinstance(reload.get("xds"), dict) else {}
    if as_int(xds.get("snapshots_nacked")) > 0:
        reload_evidence.append(f"xDS NACKs {as_int(xds.get('snapshots_nacked'))}")
    if as_int(xds.get("stream_failures")) > 0 or as_int(xds.get("connect_failures")) > 0:
        reload_evidence.append(
            f"xDS stream/connect failures {as_int(xds.get('stream_failures'))}/{as_int(xds.get('connect_failures'))}"
        )

    pool_evidence: list[str] = []
    pool_indicated = False
    if pool_hit_ratio is not None:
        pool_evidence.append(f"pool hit ratio {fmt_percent(pool_hit_ratio)}")
        pool_indicated = pool_hit_ratio < 0.20
    if connect_p99 is not None and max_profile_p99 is not None and connect_p99 >= max_profile_p99 * 0.50:
        pool_evidence.append(f"connect p99 {fmt_ms(connect_p99)} vs profile p99 {fmt_ms(max_profile_p99)}")
        pool_indicated = True

    high_cardinality = observability.get("traffic_metric_high_cardinality_labels")
    if not isinstance(high_cardinality, list):
        high_cardinality = []
    request_series = as_int(observability.get("request_latency_histogram_series"))
    observability_indicated = bool(high_cardinality) or request_series > 1000
    observability_evidence = [
        "traffic high-cardinality labels "
        + (", ".join(str(item) for item in high_cardinality) if high_cardinality else "none"),
        f"request latency histogram series {request_series}",
    ]

    resource_evidence: list[str] = []
    dataplane_resources = resources.get("dataplane", {})
    if dataplane_resources:
        resource_evidence.append(
            "dataplane resources cpu/rss/fd/threads "
            f"{fmt_number(dataplane_resources.get('cpu_millicores'))}/"
            f"{fmt_number(dataplane_resources.get('rss_kib'))}/"
            f"{fmt_number(dataplane_resources.get('fd_count'))}/"
            f"{fmt_number(dataplane_resources.get('threads'))}"
        )
    else:
        resource_evidence.append("dataplane resource snapshot unavailable")

    gateway_indicated = (
        bool(scenario_rows)
        and not upstream_evidence
        and not fault_evidence
        and not reload_evidence
        and not pool_indicated
        and not observability_indicated
    )

    return {
        "gateway_forwarding_bottleneck": {
            "indicated": gateway_indicated,
            "evidence": resource_evidence,
        },
        "runtime_snapshot_read_or_string_clone": {
            "indicated": False,
            "evidence": [
                "no direct RwLock or string clone p999 evidence captured",
            ],
        },
        "upstream_slow_response": {
            "indicated": bool(upstream_evidence),
            "evidence": upstream_evidence or ["no backend slow-read/write scenario evidence"],
        },
        "connection_pool_reuse": {
            "indicated": pool_indicated,
            "evidence": pool_evidence or ["pool/connect evidence unavailable"],
        },
        "reload_jitter": {
            "indicated": bool(reload_evidence),
            "evidence": reload_evidence or ["no reload-under-load or xDS failure evidence"],
        },
        "observability_overhead": {
            "indicated": observability_indicated,
            "evidence": observability_evidence,
        },
        "fault_injection": {
            "indicated": bool(fault_evidence),
            "evidence": fault_evidence or ["no backend-error or endpoint-flapping scenario evidence"],
        },
    }


def upstream_tuning_gate(
    profiles: list[dict[str, Any]],
    pool_hit_ratio: float | None,
    pool_sample_count: int,
    connect_p99: float | None,
) -> dict[str, Any]:
    short_profiles: list[dict[str, Any]] = []
    excluded_long_or_noisy = 0
    for profile in profiles:
        protocol = str(profile.get("protocol", ""))
        p99 = as_number((profile.get("latency_ms") or {}).get("p99"))
        if p99 is None:
            continue
        scenarios = set(profile_scenarios(profile))
        if protocol not in UPSTREAM_TUNING_SHORT_PROTOCOLS:
            excluded_long_or_noisy += 1
            continue
        if scenarios & UPSTREAM_TUNING_EXCLUDED_SCENARIOS:
            excluded_long_or_noisy += 1
            continue
        short_profiles.append(profile)

    short_request_max_p99 = max_optional(
        [
            as_number((profile.get("latency_ms") or {}).get("p99"))
            for profile in short_profiles
        ]
    )
    connect_share = (
        connect_p99 / short_request_max_p99
        if connect_p99 is not None and short_request_max_p99 is not None and short_request_max_p99 > 0
        else None
    )
    pool_hit_ratio_low = (
        pool_hit_ratio is not None
        and pool_sample_count >= UPSTREAM_TUNING_MIN_POOL_SAMPLES
        and pool_hit_ratio < UPSTREAM_TUNING_LOW_POOL_HIT_RATIO
    )
    connect_p99_material = (
        connect_share is not None
        and connect_share >= UPSTREAM_TUNING_CONNECT_P99_SHARE
    )

    insufficient_reasons: list[str] = []
    if pool_sample_count < UPSTREAM_TUNING_MIN_POOL_SAMPLES:
        insufficient_reasons.append(
            f"pool samples {pool_sample_count} < {UPSTREAM_TUNING_MIN_POOL_SAMPLES}"
        )
    if pool_hit_ratio is None:
        insufficient_reasons.append("pool hit ratio unavailable")
    if connect_p99 is None:
        insufficient_reasons.append("connect p99 unavailable")
    if short_request_max_p99 is None:
        insufficient_reasons.append("short-request HTTP/gRPC p99 unavailable")

    if pool_hit_ratio_low or connect_p99_material:
        decision = "supported_by_evidence"
    elif insufficient_reasons:
        decision = "insufficient_evidence"
    else:
        decision = "not_indicated"

    candidate_defaults: list[str] = []
    if pool_hit_ratio_low or connect_p99_material:
        candidate_defaults.append("keepalive")
    if connect_p99_material and any(
        str(profile.get("protocol", "")) == "grpc" for profile in short_profiles
    ):
        candidate_defaults.append("http2_upstream_max_streams")

    evidence = [
        f"pool hit ratio {fmt_percent(pool_hit_ratio)} across {pool_sample_count} samples "
        f"(low threshold < {fmt_percent(UPSTREAM_TUNING_LOW_POOL_HIT_RATIO)})",
        f"connect p99 {fmt_ms(connect_p99)} vs short-request max p99 {fmt_ms(short_request_max_p99)} "
        f"(share {fmt_percent(connect_share)}, material threshold >= {fmt_percent(UPSTREAM_TUNING_CONNECT_P99_SHARE)})",
    ]
    if insufficient_reasons:
        evidence.append("insufficient evidence: " + "; ".join(insufficient_reasons))

    return {
        "decision": decision,
        "supported_by_evidence": decision == "supported_by_evidence",
        "pool_hit_ratio": pool_hit_ratio,
        "pool_sample_count": pool_sample_count,
        "pool_hit_ratio_low": pool_hit_ratio_low,
        "pool_hit_ratio_low_threshold": UPSTREAM_TUNING_LOW_POOL_HIT_RATIO,
        "connect_p99_ms": connect_p99,
        "connect_p99_material": connect_p99_material,
        "connect_p99_share_threshold": UPSTREAM_TUNING_CONNECT_P99_SHARE,
        "connect_p99_share_of_short_request_p99": connect_share,
        "short_request_profile_count": len(short_profiles),
        "short_request_profiles": [
            f"{profile.get('protocol')}/{profile.get('profile')}"
            for profile in short_profiles
        ],
        "short_request_max_p99_ms": short_request_max_p99,
        "excluded_profile_count": excluded_long_or_noisy,
        "candidate_defaults": candidate_defaults,
        "evidence": evidence,
    }


traffic_path, traffic = first_json(
    [
        input_dir / "admin-after" / "dataplane" / "traffic.json",
        input_dir / "admin" / "dataplane" / "traffic.json",
        input_dir / "dataplane" / "traffic.json",
        input_dir / "traffic.json",
    ]
)
metrics_path = first_file(
    [
        input_dir / "admin-after" / "dataplane" / "metrics.prom",
        input_dir / "admin" / "dataplane" / "metrics.prom",
        input_dir / "dataplane" / "metrics.prom",
        input_dir / "metrics.prom",
    ]
)
metrics = read_text(metrics_path)
profiles = collect_profiles()
observed_protocols = sorted({profile["protocol"] for profile in profiles})
missing_protocols = sorted(set(REQUIRED_PROTOCOLS) - set(observed_protocols))
observed_scenario_names = observed_scenarios(profiles)
missing_scenarios = sorted(set(REQUIRED_SCENARIOS) - set(observed_scenario_names))
scenario_rows = scenario_summary(profiles)

connect_buckets = traffic.get("upstream_connect_latency_ms_buckets")
if not isinstance(connect_buckets, list) or not connect_buckets:
    connect_buckets, metric_sum, metric_count = metric_histogram(
        metrics, "nantian_gateway_dataplane_traffic_upstream_connect_latency_ms"
    )
else:
    metric_sum, metric_count = None, None

connect_count = as_int(
    traffic.get("total_upstream_connect_latency_observations"),
    metric_count if metric_count is not None else 0,
)
connect_sum = as_number(traffic.get("total_upstream_connect_latency_ms"))
if connect_sum is None:
    connect_sum = metric_sum
connect_max = as_number(traffic.get("max_upstream_connect_latency_ms"))
if connect_max is None:
    connect_max = metric_value(metrics, "nantian_gateway_dataplane_traffic_upstream_connect_latency_ms_max")
connect_avg = connect_sum / connect_count if connect_sum is not None and connect_count > 0 else None

pool_hits = as_int(
    traffic.get("total_upstream_pool_hits"),
    as_int(metric_value(metrics, "nantian_gateway_dataplane_traffic_upstream_pool_hits_total")),
)
pool_misses = as_int(
    traffic.get("total_upstream_pool_misses"),
    as_int(metric_value(metrics, "nantian_gateway_dataplane_traffic_upstream_pool_misses_total")),
)
pool_total = pool_hits + pool_misses
pool_hit_ratio = pool_hits / pool_total if pool_total > 0 else None

retried_events = as_int(
    traffic.get("total_retried_events"),
    as_int(metric_value(metrics, "nantian_gateway_dataplane_traffic_retried_events_total")),
)
retry_attempts = as_int(
    traffic.get("total_retry_attempts"),
    as_int(metric_value(metrics, "nantian_gateway_dataplane_traffic_retry_attempts_total")),
)
retried_success_events = as_int(
    traffic.get("total_retried_success_events"),
    as_int(metric_value(metrics, "nantian_gateway_dataplane_traffic_retried_success_events_total")),
)
retry_after_success_rate = retried_success_events / retried_events if retried_events > 0 else None

status = status_classes(traffic, metrics, profiles)
response_flags = collect_response_flags(traffic, metrics)
resources = resource_summary()
observability = {
    "traffic_graph": collect_traffic_graph(traffic),
    "traffic_metric_high_cardinality_labels": traffic_metric_high_cardinality_labels(metrics),
    "request_latency_histogram_series": request_latency_histogram_series(traffic, metrics),
}
reload = {
    "xds": xds_reload_metrics(metrics),
    "xds_apply_stage_duration_ms": xds_apply_stage_histograms(metrics),
    "phase_summary": reload_phase_summary(profiles),
    "live_traffic": live_reload_traffic_summary(profiles),
}
fault_isolation = fault_isolation_metrics(metrics)
udp = udp_route_summary(profiles, metrics)
stream_curve = stream_resource_curve(profiles)
stream_buffer_eval = stream_buffer_evaluation(profiles)
chaos_evidence = load_chaos_evidence(optional_path(chaos_input_dir_arg))
soak_evidence = load_soak_evidence(optional_path(soak_input_dir_arg))

connect_p95 = quantile_from_buckets(connect_buckets, 0.95, connect_count or None)
connect_p99 = quantile_from_buckets(connect_buckets, 0.99, connect_count or None)
upstream_gate = upstream_tuning_gate(profiles, pool_hit_ratio, pool_total, connect_p99)
max_profile_p99 = max(
    (profile["latency_ms"].get("p99") for profile in profiles if profile["latency_ms"].get("p99") is not None),
    default=None,
)
bottleneck_classification = classify_bottlenecks(
    scenario_rows,
    pool_hit_ratio,
    connect_p99,
    max_profile_p99,
    observability,
    reload,
    resources,
)

notes: list[str] = []
if not profiles:
    notes.append("No traffic profile JSON was found under protocol directories.")
if missing_protocols:
    notes.append(
        "Required protocol coverage is incomplete; missing "
        + ", ".join(missing_protocols)
        + "."
    )
if missing_scenarios:
    notes.append(
        "Required scenario coverage is incomplete; missing "
        + ", ".join(missing_scenarios)
        + "."
    )
if connect_p99 is None:
    notes.append("Upstream connect latency p95/p99 is unavailable; capture a current dataplane /v1/traffic snapshot or metrics.prom with histogram buckets.")
if upstream_gate["decision"] == "supported_by_evidence":
    notes.append(
        "Upstream tuning gate has evidence; run an A/B profile for "
        + ", ".join(upstream_gate["candidate_defaults"])
        + " before changing runtime defaults."
    )
elif upstream_gate["decision"] == "insufficient_evidence":
    notes.append(
        "Upstream tuning gate has insufficient evidence; capture pool hit ratio, connect p99 and short-request HTTP/gRPC profiles before changing keepalive or HTTP/2 stream defaults."
    )
if pool_hit_ratio is not None and pool_total >= 10 and pool_hit_ratio < 0.20:
    notes.append("Connection pool reuse is low; compare keepalive and upstream protocol settings before changing defaults.")
if connect_p99 is not None and max_profile_p99 is not None and connect_p99 >= max_profile_p99 * 0.50:
    notes.append("Upstream connect p99 is a material share of end-to-end p99; connection establishment may be a bottleneck.")
if status.get("5xx", 0) > 0 or status.get("other", 0) > 0:
    notes.append("5xx or non-HTTP-status traffic was observed; inspect response flags and upstream failure logs before attributing latency to gateway CPU.")
if not resources:
    notes.append("Resource snapshot is unavailable; capture resources/after.tsv to compare CPU, RSS, FD and thread usage.")
if chaos_input_dir_arg and chaos_evidence is None:
    notes.append("Chaos evidence input was provided but could not be loaded.")
if soak_input_dir_arg and soak_evidence is None:
    notes.append("Soak evidence input was provided but could not be loaded.")
if soak_evidence and not soak_evidence["is_24h"]:
    notes.append("Soak evidence is shorter than 24h; keep 24h soak validation open before making 24h production stability claims.")
if observability["traffic_metric_high_cardinality_labels"]:
    notes.append(
        "Prometheus traffic metrics include high-cardinality topology labels: "
        + ", ".join(observability["traffic_metric_high_cardinality_labels"])
        + ". Keep route/backend/pod breakdowns in admin traffic summaries or explicit debug metrics."
    )
if as_int(traffic.get("total_events")) > 0 and not (
    observability["traffic_graph"]["has_route_topology"]
    and observability["traffic_graph"]["has_backend_topology"]
):
    notes.append("Traffic graph topology is incomplete for observed traffic; verify /v1/traffic before downsampling or async aggregation changes.")
if reload["xds_apply_stage_duration_ms"]["series"] == 0:
    notes.append("xDS apply stage duration histogram is unavailable; capture dataplane /metrics after reload scenarios to attribute reload jitter.")
if reload["live_traffic"]["profile_count"] == 0:
    notes.append("Live-traffic reload evidence is unavailable; add HTTP/gRPC/TCP/UDP reload-under-load profile JSON with snapshot mutation metadata before claiming reload p99 stability.")
elif reload["live_traffic"]["missing_protocols"]:
    notes.append(
        "Live-traffic reload protocol coverage is incomplete; missing "
        + ", ".join(reload["live_traffic"]["missing_protocols"])
        + "."
    )
elif reload["live_traffic"]["missing_mutations"]:
    notes.append(
        "Live-traffic reload mutation coverage is incomplete; missing "
        + ", ".join(reload["live_traffic"]["missing_mutations"])
        + "."
    )
if reload["xds"]["snapshots_nacked"] > 0:
    notes.append("xDS NACKs were observed during the evidence window; inspect listener apply failures and last-good fallback state before attributing errors to backend behavior.")
if reload["xds"]["stream_failures"] > 0 or reload["xds"]["connect_failures"] > 0:
    notes.append("xDS stream or connect failures were observed; correlate reload timing with control plane availability before judging dataplane apply cost.")
if fault_isolation["fast_fail_total"] > 0:
    notes.append("Gateway-side fast-fail counters were observed; compare overload, circuit breaker, rate limit and retry budget exhaustion before attributing errors to upstream health alone.")
if fault_isolation["passive_ejection_total"] > 0 or fault_isolation["active_unhealthy_total"] > 0:
    notes.append("Endpoint health isolation counters were observed; correlate passive ejections, active unhealthy endpoints and recovery latency with fault-injection scenarios.")
if fault_isolation["last_good_snapshot_active"] or fault_isolation["current_snapshot_rejected"]:
    notes.append("Last-good snapshot or current snapshot rejection state was observed; verify readyz, ACK/NACK and listener status before evaluating failure isolation results.")
if udp["profile_count"] == 0:
    notes.append("UDPRoute throughput evidence is unavailable; add UDP profile JSON with packet loss and session churn samples before claiming UDPRoute p99 improvements.")
elif udp["coverage"]["missing_scenarios"]:
    notes.append(
        "UDPRoute scenario coverage is incomplete; missing "
        + ", ".join(udp["coverage"]["missing_scenarios"])
        + "."
    )
elif as_int(udp["datagrams_lost"]) > 0:
    notes.append("UDP packet loss was observed; correlate lost datagrams with queue overflow drops, idle evictions and backend timeout scenarios.")
if stream_curve["point_count"] == 0:
    notes.append("TCP/TLS passthrough resource curve is unavailable; add TCP or TLS profile JSON with connection count, RSS, FD, threads, p99 and RPS samples.")
if not notes:
    notes.append("No aggregate gateway forwarding, upstream latency, pool reuse, reload jitter, or observability bottleneck is indicated by this evidence set.")

report = {
    "run_id": run_id,
    "input_dir": str(input_dir),
    "traffic_source": str(traffic_path.relative_to(input_dir)) if traffic_path else None,
    "metrics_source": str(metrics_path.relative_to(input_dir)) if metrics_path else None,
    "profiles": profiles,
    "coverage": {
        "required_protocols": REQUIRED_PROTOCOLS,
        "observed_protocols": observed_protocols,
        "missing_protocols": missing_protocols,
        "required_scenarios": REQUIRED_SCENARIOS,
        "observed_scenarios": observed_scenario_names,
        "missing_scenarios": missing_scenarios,
        "scenario_summary": scenario_rows,
    },
    "upstream": {
        "pool_hits": pool_hits,
        "pool_misses": pool_misses,
        "pool_hit_ratio": pool_hit_ratio,
        "peer_build_failures": as_int(
            traffic.get("total_upstream_peer_build_failures"),
            as_int(metric_value(metrics, "nantian_gateway_dataplane_traffic_upstream_peer_build_failures_total")),
        ),
        "tls_handshake_failures": as_int(
            traffic.get("total_upstream_tls_handshake_failures"),
            as_int(metric_value(metrics, "nantian_gateway_dataplane_traffic_upstream_tls_handshake_failures_total")),
        ),
        "connect_latency_ms": {
            "observations": connect_count,
            "sum": connect_sum,
            "average": connect_avg,
            "max": connect_max,
            "p95": connect_p95,
            "p99": connect_p99,
            "buckets": connect_buckets,
        },
    },
    "upstream_tuning_gate": upstream_gate,
    "retries": {
        "retried_events": retried_events,
        "retry_attempts": retry_attempts,
        "retried_success_events": retried_success_events,
        "retry_after_success_rate": retry_after_success_rate,
    },
    "status_classes": status,
    "response_flags": response_flags,
    "observability": observability,
    "reload": reload,
    "fault_isolation": fault_isolation,
    "udp": udp,
    "stream_resource_curve": stream_curve,
    "stream_buffer_evaluation": stream_buffer_eval,
    "chaos_evidence": chaos_evidence,
    "soak_evidence": soak_evidence,
    "resources": resources,
    "bottleneck_classification": bottleneck_classification,
    "bottleneck_notes": notes,
}

output_dir.mkdir(parents=True, exist_ok=True)
report_output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")

profile_rows = "\n".join(profile_row(profile) for profile in profiles)
if not profile_rows:
    profile_rows = "| n/a | n/a | 0 | 0 | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |"

observed_protocols_text = ", ".join(observed_protocols) if observed_protocols else "n/a"
missing_protocols_text = ", ".join(missing_protocols) if missing_protocols else "none"
observed_scenarios_text = ", ".join(observed_scenario_names) if observed_scenario_names else "n/a"
missing_scenarios_text = ", ".join(missing_scenarios) if missing_scenarios else "none"

status_rows = "\n".join(f"| {name} | {count} |" for name, count in status.items())
flag_rows = "\n".join(f"| {flag} | {count} |" for flag, count in sorted(response_flags.items()))
if not flag_rows:
    flag_rows = "| n/a | 0 |"

topology_rows = "\n".join(
    f"| {kind} | {count} |"
    for kind, count in sorted(observability["traffic_graph"]["node_kinds"].items())
)
if not topology_rows:
    topology_rows = "| n/a | 0 |"
high_cardinality_labels_text = ", ".join(observability["traffic_metric_high_cardinality_labels"])
if not high_cardinality_labels_text:
    high_cardinality_labels_text = "none"

xds_apply = reload["xds_apply_stage_duration_ms"]
xds_apply_rows = "\n".join(
    f"| {stage['stage']} | {stage['observations']} | {fmt_number(stage.get('average'))} | "
    f"{fmt_number(stage.get('p95'))} | {fmt_number(stage.get('p99'))} | {fmt_number(stage.get('sum'))} |"
    for stage in xds_apply["stages"]
)
if not xds_apply_rows:
    xds_apply_rows = "| n/a | 0 | n/a | n/a | n/a | n/a |"

reload_phase_rows = "\n".join(
    f"| {item['phase']} | {item['profile_count']} | {', '.join(item['protocols']) or 'n/a'} | "
    f"{item['requests']} | {fmt_percent(item.get('success_rate'))} | "
    f"{fmt_number(item.get('max_p95_ms'))} | {fmt_number(item.get('max_p99_ms'))} | "
    f"{fmt_number(item.get('max_p999_ms'))} | {fmt_flags(item.get('response_flags'))} | "
    f"{fmt_number(item.get('ack_latency_ms', {}).get('average'))} / {fmt_number(item.get('ack_latency_ms', {}).get('max'))} | "
    f"{fmt_number(item.get('nack_count'))} | {fmt_number(item.get('last_good_fallback_count'))} |"
    for item in reload["phase_summary"]
)
if not reload_phase_rows:
    reload_phase_rows = "| n/a | 0 | n/a | 0 | n/a | n/a | n/a | n/a | none | n/a / n/a | 0 | 0 |"

live_reload = reload["live_traffic"]
observed_live_reload_protocols_text = ", ".join(live_reload["observed_protocols"])
if not observed_live_reload_protocols_text:
    observed_live_reload_protocols_text = "n/a"
missing_live_reload_protocols_text = ", ".join(live_reload["missing_protocols"])
if not missing_live_reload_protocols_text:
    missing_live_reload_protocols_text = "none"
observed_live_reload_mutations_text = ", ".join(live_reload["observed_mutations"])
if not observed_live_reload_mutations_text:
    observed_live_reload_mutations_text = "n/a"
missing_live_reload_mutations_text = ", ".join(live_reload["missing_mutations"])
if not missing_live_reload_mutations_text:
    missing_live_reload_mutations_text = "none"
live_reload_mutation_rows = "\n".join(
    f"| {item['mutation']} | {item['profile_count']} | {', '.join(item['protocols']) or 'n/a'} | "
    f"{', '.join(item['phases']) or 'n/a'} | {item['requests']} | "
    f"{fmt_percent(item.get('success_rate'))} | {fmt_number(item.get('max_p99_ms'))} | "
    f"{fmt_number(item.get('max_p999_ms'))} | {fmt_flags(item.get('response_flags'))} | "
    f"{fmt_number(item.get('ack_latency_ms', {}).get('average'))} / {fmt_number(item.get('ack_latency_ms', {}).get('max'))} | "
    f"{fmt_number(item.get('nack_count'))} | {fmt_number(item.get('last_good_fallback_count'))} |"
    for item in live_reload["mutation_summary"]
)
if not live_reload_mutation_rows:
    live_reload_mutation_rows = "| n/a | 0 | n/a | n/a | 0 | n/a | n/a | n/a | none | n/a / n/a | 0 | 0 |"

fault_recovery_latency = fault_isolation["recovery_latency_ms"]
udp_sessions = udp["sessions"]
udp_coverage = udp["coverage"]
udp_churn = udp["session_churn"]
udp_resource_improvements = udp["resource_improvements"]
stream_buffer_eval = report["stream_buffer_evaluation"]
upstream_gate = report["upstream_tuning_gate"]
stream_curve_points = stream_curve["points"]
observed_udp_scenarios_text = ", ".join(udp_coverage["observed_scenarios"])
if not observed_udp_scenarios_text:
    observed_udp_scenarios_text = "n/a"
missing_udp_scenarios_text = ", ".join(udp_coverage["missing_scenarios"])
if not missing_udp_scenarios_text:
    missing_udp_scenarios_text = "none"
udp_scenario_summary_rows = "\n".join(
    f"| {item['scenario']} | {item['profile_count']} | {', '.join(item['protocols']) or 'n/a'} | "
    f"{item['requests']} | {fmt_percent(item.get('success_rate'))} | {fmt_number(item.get('max_p99_ms'))} | "
    f"{fmt_number(item.get('max_p999_ms'))} | {fmt_number(item.get('max_rps'))} | "
    f"{fmt_number(item.get('client_count'))} | {fmt_number(item.get('upstream_count'))} | "
    f"{fmt_number(item.get('session_opens'))} | {fmt_number(item.get('session_evictions'))} |"
    for item in udp_coverage["scenario_summary"]
)
if not udp_scenario_summary_rows:
    udp_scenario_summary_rows = "| n/a | 0 | n/a | 0 | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |"

udp_resource_improvement_rows = "\n".join(
    f"| {item['group']} | {item['baseline_profile']} | {item['current_profile']} | "
    f"{fmt_number(item.get('baseline', {}).get('p99_ms'))} | {fmt_number(item.get('current', {}).get('p99_ms'))} | "
    f"{fmt_number(item.get('delta', {}).get('p99_ms'))} | {fmt_percent(item.get('improvement', {}).get('p99_reduction_ratio'))} | "
    f"{fmt_number(item.get('baseline', {}).get('rss_kib'))} | {fmt_number(item.get('current', {}).get('rss_kib'))} | "
    f"{fmt_number(item.get('delta', {}).get('rss_kib'))} | {fmt_percent(item.get('improvement', {}).get('rss_reduction_ratio'))} | "
    f"{fmt_number(item.get('baseline', {}).get('throughput_rps'))} | {fmt_number(item.get('current', {}).get('throughput_rps'))} | "
    f"{fmt_number(item.get('delta', {}).get('throughput_rps'))} |"
    for item in udp_resource_improvements["comparisons"]
)
if not udp_resource_improvement_rows:
    udp_resource_improvement_rows = "| n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |"

stream_buffer_evaluation_rows = "\n".join(
    f"| {item['group']} | {item['baseline_profile']} | {item['current_profile']} | "
    f"{fmt_number(item.get('baseline', {}).get('buffer_size_bytes'))} | {fmt_number(item.get('current', {}).get('buffer_size_bytes'))} | "
    f"{fmt_number(item.get('baseline', {}).get('p99_ms'))} | {fmt_number(item.get('current', {}).get('p99_ms'))} | "
    f"{fmt_number(item.get('delta', {}).get('p99_ms'))} | {fmt_percent(item.get('improvement', {}).get('p99_reduction_ratio'))} | "
    f"{fmt_number(item.get('baseline', {}).get('rss_kib'))} | {fmt_number(item.get('current', {}).get('rss_kib'))} | "
    f"{fmt_number(item.get('delta', {}).get('rss_kib'))} | {fmt_percent(item.get('improvement', {}).get('rss_reduction_ratio'))} | "
    f"{fmt_number(item.get('baseline', {}).get('throughput_rps'))} | {fmt_number(item.get('current', {}).get('throughput_rps'))} | "
    f"{fmt_number(item.get('delta', {}).get('throughput_rps'))} |"
    for item in stream_buffer_eval["comparisons"]
)
if not stream_buffer_evaluation_rows:
    stream_buffer_evaluation_rows = "| n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |"

stream_resource_curve_rows = "\n".join(
    f"| {item['protocol']} | {item['profile']} | {fmt_number(item.get('connections'))} | "
    f"{fmt_number(item.get('concurrency'))} | {fmt_percent(item.get('success_rate'))} | "
    f"{fmt_number(item.get('p99_ms'))} | {fmt_number(item.get('p999_ms'))} | "
    f"{fmt_number(item.get('throughput_rps'))} | {fmt_number(item.get('rss_kib'))} | "
    f"{fmt_number(item.get('fd_count'))} | {fmt_number(item.get('threads'))} | "
    f"{fmt_number(item.get('cpu_millicores'))} | {fmt_number(item.get('rss_per_connection_kib'))} | "
    f"{fmt_number(item.get('fd_per_connection'))} | {fmt_number(item.get('buffer_size_bytes'))} |"
    for item in stream_curve_points
)
if not stream_resource_curve_rows:
    stream_resource_curve_rows = "| n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |"

scenario_summary_rows = "\n".join(
    f"| {item['scenario']} | {item['profile_count']} | {', '.join(item['protocols']) or 'n/a'} | "
    f"{item['requests']} | {fmt_percent(item.get('success_rate'))} | {fmt_number(item.get('max_p99_ms'))} | "
    f"{fmt_number(item.get('max_p999_ms'))} | {fmt_number(item.get('max_rps'))} |"
    for item in scenario_rows
)
if not scenario_summary_rows:
    scenario_summary_rows = "| n/a | 0 | n/a | 0 | n/a | n/a | n/a | n/a |"

bottleneck_rows = "\n".join(
    f"| {name} | {'yes' if item['indicated'] else 'no'} | {'; '.join(item['evidence'])} |"
    for name, item in bottleneck_classification.items()
)
upstream_gate_candidates_text = ", ".join(upstream_gate["candidate_defaults"])
if not upstream_gate_candidates_text:
    upstream_gate_candidates_text = "none"
upstream_gate_evidence_text = "; ".join(upstream_gate["evidence"])
if not upstream_gate_evidence_text:
    upstream_gate_evidence_text = "n/a"

resource_rows = []
for component, item in sorted(resources.items()):
    resource_rows.append(
        f"| {component} | {fmt_number(item.get('cpu_millicores'))} | "
        f"{fmt_number(item.get('rss_kib'))} | {fmt_number(item.get('fd_count'))} | "
        f"{fmt_number(item.get('threads'))} |"
    )
if not resource_rows:
    resource_rows = ["| n/a | n/a | n/a | n/a | n/a |"]

if chaos_evidence:
    chaos_traffic = chaos_evidence["traffic"]
    chaos_coverage = chaos_evidence["coverage"]
    chaos_missing_scenarios_text = ", ".join(chaos_coverage["missing_scenarios"])
    if not chaos_missing_scenarios_text:
        chaos_missing_scenarios_text = "none"
    chaos_observed_scenarios_text = ", ".join(chaos_coverage["observed_scenarios"])
    if not chaos_observed_scenarios_text:
        chaos_observed_scenarios_text = "n/a"
    chaos_scenario_rows = "\n".join(
        f"| {item['scenario']} | {item.get('status') or 'n/a'} | "
        f"{item.get('summary') or 'n/a'} | "
        f"{', '.join(item.get('evidence') or []) or 'n/a'} |"
        for item in chaos_evidence["scenarios"]
    )
    if not chaos_scenario_rows:
        chaos_scenario_rows = "| n/a | n/a | n/a | n/a |"
    chaos_summary_section = f"""
## Chaos Evidence

- Chaos source evidence: `{chaos_evidence['source_dir']}`
- Chaos run ID: `{chaos_evidence.get('run_id') or 'n/a'}`
- Chaos git commit: `{chaos_evidence.get('git_commit') or 'n/a'}`
- Release gate status: `{chaos_evidence.get('release_gate_status') or 'n/a'}`
- Required chaos scenarios: `{", ".join(chaos_coverage['required_scenarios']) or 'n/a'}`
- Observed chaos scenarios: `{chaos_observed_scenarios_text}`
- Missing chaos scenarios: `{chaos_missing_scenarios_text}`
- Chaos traffic completed/successes/errors: `{chaos_traffic['completed']}` / `{chaos_traffic['successes']}` / `{chaos_traffic['errors']}`
- Chaos traffic success rate: `{fmt_percent(chaos_traffic.get('success_rate'))}`
- Chaos traffic max p99/max latency: `{fmt_ms(chaos_traffic.get('max_p99_ms'))}` / `{fmt_ms(chaos_traffic.get('max_latency_ms'))}`
- Chaos traffic SLO gate: `{chaos_traffic.get('slo_gate_status') or 'n/a'}`

| Chaos Scenario | Status | Summary | Evidence |
| --- | --- | --- | --- |
{chaos_scenario_rows}
"""
else:
    chaos_summary_section = """
## Chaos Evidence

- Chaos evidence: `unavailable`
"""

if soak_evidence:
    soak_traffic = soak_evidence["traffic"]
    soak_observability = soak_evidence["observability"]
    soak_ready = soak_observability.get("ready_replicas")
    if not isinstance(soak_ready, dict):
        soak_ready = {}
    soak_resource_components = soak_evidence.get("resources", {}).get("components", {})
    if not isinstance(soak_resource_components, dict):
        soak_resource_components = {}
    soak_resource_rows = "\n".join(
        f"| {component} | {fmt_number(item.get('fd_slope_per_sample'))} | "
        f"{fmt_number(item.get('rss_kib_slope_per_sample'))} | "
        f"{fmt_number(item.get('threads_slope_per_sample'))} | "
        f"{fmt_number((item.get('first') or {}).get('fd_count'))} / {fmt_number((item.get('last') or {}).get('fd_count'))} | "
        f"{fmt_number((item.get('first') or {}).get('rss_kib'))} / {fmt_number((item.get('last') or {}).get('rss_kib'))} | "
        f"{fmt_number((item.get('first') or {}).get('threads'))} / {fmt_number((item.get('last') or {}).get('threads'))} |"
        for component, item in sorted(soak_resource_components.items())
        if isinstance(item, dict)
    )
    if not soak_resource_rows:
        soak_resource_rows = "| n/a | n/a | n/a | n/a | n/a | n/a | n/a |"
    soak_summary_section = f"""
## Soak Evidence

- Soak source evidence: `{soak_evidence['source_dir']}`
- Soak run ID: `{soak_evidence.get('run_id') or 'n/a'}`
- Soak git commit: `{soak_evidence.get('git_commit') or 'n/a'}`
- Duration seconds: `{fmt_number(soak_evidence.get('duration_seconds'))}`
- Sample interval seconds: `{fmt_number(soak_evidence.get('sample_interval_seconds'))}`
- 24h soak: `{'yes' if soak_evidence['is_24h'] else 'no'}`
- Soak traffic completed/successes/errors: `{soak_traffic['completed']}` / `{soak_traffic['successes']}` / `{soak_traffic['errors']}`
- Soak traffic success rate: `{fmt_percent(soak_traffic.get('success_rate'))}`
- Soak traffic max p99/max latency: `{fmt_ms(soak_traffic.get('max_p99_ms'))}` / `{fmt_ms(soak_traffic.get('max_latency_ms'))}`
- Soak traffic SLO gate: `{soak_traffic.get('slo_gate_status') or 'n/a'}`
- Soak xDS reconnect/stream/connect/NACK delta: `{fmt_number(soak_observability.get('xds_reconnect_delta'))}` / `{fmt_number(soak_observability.get('xds_stream_failure_delta'))}` / `{fmt_number(soak_observability.get('xds_connect_failure_delta'))}` / `{fmt_number(soak_observability.get('xds_nack_delta'))}`
- Soak ACK wait p99: `{fmt_ms(soak_observability.get('ack_wait_p99_ms'))}`
- Soak ready replicas min/max/last: `{fmt_number(soak_ready.get('min_ready'))}` / `{fmt_number(soak_ready.get('max_ready'))}` / `{fmt_number(soak_ready.get('last_ready'))}`

| Component | FD slope/sample | RSS KiB slope/sample | Threads slope/sample | FD first/last | RSS KiB first/last | Threads first/last |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
{soak_resource_rows}
"""
else:
    soak_summary_section = """
## Soak Evidence

- Soak evidence: `unavailable`
"""

notes_text = "\n".join(f"- {note}" for note in notes)

summary = f"""# Dataplane Throughput Baseline

- Run ID: `{run_id}`
- Source evidence: `{input_dir}`
- Traffic source: `{report['traffic_source'] or 'n/a'}`
- Metrics source: `{report['metrics_source'] or 'n/a'}`

## Traffic Profiles

| Protocol | Profile | Requests | Concurrency | Success Rate | p50 ms | p90 ms | p95 ms | p99 ms | p999 ms | Max ms | RPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
{profile_rows}

## Protocol Coverage

- Required protocols: `{", ".join(REQUIRED_PROTOCOLS)}`
- Observed protocols: `{observed_protocols_text}`
- Missing required protocols: `{missing_protocols_text}`

## Scenario Coverage

- Required scenarios: `{", ".join(REQUIRED_SCENARIOS)}`
- Observed scenarios: `{observed_scenarios_text}`
- Missing required scenarios: `{missing_scenarios_text}`

| Scenario | Profiles | Protocols | Requests | Success Rate | max p99 ms | max p999 ms | max RPS |
| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: |
{scenario_summary_rows}

## Upstream Connection

- Pool hit ratio: `{fmt_percent(pool_hit_ratio)}` (hits `{pool_hits}`, misses `{pool_misses}`)
- Connect latency p95/p99: `{fmt_ms(connect_p95)}` / `{fmt_ms(connect_p99)}` (bucket upper bounds, observations `{connect_count}`)
- Connect latency average/max: `{fmt_ms(connect_avg)}` / `{fmt_ms(connect_max)}`
- Peer build failures: `{report['upstream']['peer_build_failures']}`
- TLS handshake failures: `{report['upstream']['tls_handshake_failures']}`
- Upstream tuning gate: `{upstream_gate['decision']}`
- Short-request max p99: `{fmt_ms(upstream_gate.get('short_request_max_p99_ms'))}` across `{upstream_gate['short_request_profile_count']}` profiles
- Connect p99 share of short-request p99: `{fmt_percent(upstream_gate.get('connect_p99_share_of_short_request_p99'))}`
- Candidate defaults: `{upstream_gate_candidates_text}`
- Gate evidence: `{upstream_gate_evidence_text}`

## Bottleneck Classification

| Category | Indicated | Evidence |
| --- | --- | --- |
{bottleneck_rows}

## Retry And Failover

- Retried events: `{retried_events}`
- Retry attempts: `{retry_attempts}`
- Retried success events: `{retried_success_events}`
- Retry after success rate: `{fmt_percent(retry_after_success_rate)}`

## Fault Isolation

- Fast-fail total: `{fault_isolation['fast_fail_total']}`
- HTTP/TCP/UDP overload rejected: `{fault_isolation['http_overload_rejected_total']}` / `{fault_isolation['tcp_overload_rejected_total']}` / `{fault_isolation['udp_overload_rejected_total']}`
- Circuit open / retry budget exhausted: `{fault_isolation['circuit_open_total']}` / `{fault_isolation['retry_budget_exhausted_total']}`
- Rate limit rejected: `{fault_isolation['rate_limit_rejected_total']}`
- Passive ejections / active unhealthy: `{fault_isolation['passive_ejection_total']}` / `{fault_isolation['active_unhealthy_total']}`
- Recovery latency p95/p99: `{fmt_ms(fault_recovery_latency.get('p95'))}` / `{fmt_ms(fault_recovery_latency.get('p99'))}` (observations `{fault_recovery_latency['observations']}`)
- Last-good snapshot active / current rejected: `{'yes' if fault_isolation['last_good_snapshot_active'] else 'no'}` / `{'yes' if fault_isolation['current_snapshot_rejected'] else 'no'}`

{chaos_summary_section}

{soak_summary_section}

## UDPRoute Evidence

- UDP packet loss: `{fmt_percent(udp.get('packet_loss_rate'))}` (lost `{udp['datagrams_lost']}` / sent `{udp['datagrams_sent']}`)
- UDP datagrams received: `{udp['datagrams_received']}`
- UDP max p99: `{fmt_ms(udp.get('max_p99_ms'))}` (profiles `{udp['profile_count']}`)
- UDP sessions active / queue depth: `{udp_sessions['active_current']}` / `{udp_sessions['queue_depth_current']}`
- UDP queue overflow drops / idle evictions: `{udp_sessions['queue_overflow_dropped_total']}` / `{udp_sessions['idle_evictions_total']}`
- UDP session opens / evictions: `{udp_churn['session_opens']}` / `{udp_churn['session_evictions']}`
- UDP max client/upstream count: `{udp_churn['max_client_count']}` / `{udp_churn['max_upstream_count']}`
- Required UDP scenarios: `{", ".join(UDP_REQUIRED_SCENARIOS)}`
- Observed UDP scenarios: `{observed_udp_scenarios_text}`
- Missing UDP scenarios: `{missing_udp_scenarios_text}`

| UDP Scenario | Profiles | Protocols | Requests | Success Rate | max p99 ms | max p999 ms | max RPS | max clients | max upstreams | session opens | session evictions |
| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
{udp_scenario_summary_rows}

## UDPRoute Resource Improvement

- UDP resource improvement comparisons: `{udp_resource_improvements['comparison_count']}`
- UDP all resource/p99 comparisons improved: `{'yes' if udp_resource_improvements['all_improved'] else 'no'}`

| Group | Baseline | Current | baseline p99 ms | current p99 ms | p99 delta ms | p99 reduction | baseline RSS KiB | current RSS KiB | RSS delta KiB | RSS reduction | baseline RPS | current RPS | RPS delta |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
{udp_resource_improvement_rows}

## TCP/TLS Passthrough Resource Curve

- TCP/TLS resource curve points: `{stream_curve['point_count']}`
- TCP/TLS protocols: `{", ".join(stream_curve['protocols']) if stream_curve['protocols'] else 'n/a'}`
- TCP/TLS max connections: `{fmt_number(stream_curve.get('max_connections'))}`
- TCP/TLS max RSS / FD / threads: `{fmt_number(stream_curve.get('max_rss_kib'))} KiB` / `{fmt_number(stream_curve.get('max_fd_count'))}` / `{fmt_number(stream_curve.get('max_threads'))}`
- TCP/TLS max p99 / RPS: `{fmt_ms(stream_curve.get('max_p99_ms'))}` / `{fmt_number(stream_curve.get('max_rps'))}`

## TCP/TLS Buffer Profile Evaluation

- TCP/TLS buffer evaluation comparisons: `{stream_buffer_eval['comparison_count']}`
- TCP/TLS all buffer comparisons improved: `{'yes' if stream_buffer_eval['all_improved'] else 'no'}`

| Group | Baseline | Current | baseline buffer bytes | current buffer bytes | baseline p99 ms | current p99 ms | p99 delta ms | p99 reduction | baseline RSS KiB | current RSS KiB | RSS delta KiB | RSS reduction | baseline RPS | current RPS | RPS delta |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
{stream_buffer_evaluation_rows}

## TCP/TLS Resource Curve Points

| Protocol | Profile | Connections | Concurrency | Success Rate | p99 ms | p999 ms | RPS | RSS KiB | FD | Threads | CPU millicores | RSS/conn KiB | FD/conn | Buffer bytes |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
{stream_resource_curve_rows}

## Status Classes

| Class | Events |
| --- | ---: |
{status_rows}

## Response Flags

| Flag | Events |
| --- | ---: |
{flag_rows}

## Observability

- Traffic graph nodes/edges: `{observability['traffic_graph']['node_count']}` / `{observability['traffic_graph']['edge_count']}`
- Prometheus traffic high-cardinality labels: `{high_cardinality_labels_text}`
- Request latency histogram series: `{observability['request_latency_histogram_series']}`

| Traffic graph node kind | Count |
| --- | ---: |
{topology_rows}

## Reload / xDS Apply

- xDS snapshots applied/NACKed/skipped: `{reload['xds']['snapshots_applied']}` / `{reload['xds']['snapshots_nacked']}` / `{reload['xds']['snapshots_skipped']}`
- xDS stream/connect failures: `{reload['xds']['stream_failures']}` / `{reload['xds']['connect_failures']}`
- xDS last apply timestamp seconds: `{reload['xds']['last_apply_timestamp_seconds']}`
- xDS apply stage histogram series: `{xds_apply['series']}`

| Stage | Observations | Average ms | p95 ms | p99 ms | Sum ms |
| --- | ---: | ---: | ---: | ---: | ---: |
{xds_apply_rows}

## Reload Traffic Phases

| Phase | Profiles | Protocols | Requests | Success Rate | max p95 ms | max p99 ms | max p999 ms | Error Flags | ACK latency avg/max ms | NACKs | Last-good fallback |
| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: |
{reload_phase_rows}

## Live Traffic Reload Coverage

- Live reload profile count: `{live_reload['profile_count']}`
- Required live reload protocols: `{", ".join(LIVE_RELOAD_REQUIRED_PROTOCOLS)}`
- Observed live reload protocols: `{observed_live_reload_protocols_text}`
- Missing live reload protocols: `{missing_live_reload_protocols_text}`
- Required live reload mutations: `{", ".join(LIVE_RELOAD_REQUIRED_MUTATIONS)}`
- Observed live reload mutations: `{observed_live_reload_mutations_text}`
- Missing live reload mutations: `{missing_live_reload_mutations_text}`

| Mutation | Profiles | Protocols | Phases | Requests | Success Rate | max p99 ms | max p999 ms | Error Flags | ACK latency avg/max ms | NACKs | Last-good fallback |
| --- | ---: | --- | --- | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: |
{live_reload_mutation_rows}

## Resources

| Component | CPU millicores | RSS KiB | FD | Threads |
| --- | ---: | ---: | ---: | ---: |
{chr(10).join(resource_rows)}

## Bottleneck Notes

{notes_text}

## Artifact Contract

- `metadata.txt`: run metadata, git commit, tree state and source evidence path
- `throughput-report.json`: machine-readable profile, upstream connection, retry, fault isolation, chaos, soak, UDP, UDP resource comparison, TCP/TLS resource curve, TCP/TLS buffer profile evaluation, status, response flag, observability, reload/live-traffic reload and resource summary
- `summary.md`: operator-facing throughput baseline report

## Scope

- This report standardizes collected dataplane throughput evidence. It does not by itself prove HTTP/1.1, H2C gRPC, WebSocket, SSE/MCP, TCPRoute and UDPRoute coverage; the profile table reflects only JSON evidence present under the source directory.
- UDP resource improvement rows require explicit profile pairs with the same `comparison_group` and `variant=baseline/current`; ordinary UDP profile samples are not treated as before/after evidence.
- TCP/TLS buffer evaluation rows require explicit profile pairs with the same `comparison_group`, `variant=baseline/current`, and profile-level `buffer_size_bytes`, RSS and p99 samples; they are evidence for listener profile tuning, not an automatic runtime default change.
- TCP/TLS resource curve points require profile-level connection count plus RSS/FD/thread samples; the aggregate `resources/after.tsv` snapshot remains useful for run-level totals but cannot by itself establish a concurrency curve.
- Chaos and soak sections aggregate external evidence directories when `CHAOS_INPUT_DIR` or `SOAK_INPUT_DIR` are set; mismatched embedded git commits are cross-run evidence and must not be treated as same-commit release gates.
- Use this report before changing keepalive, HTTP/2 upstream stream limits, allocator defaults or hot-path caching so pool reuse, connect p95/p99, retry success and resource pressure are visible in the same artifact.
"""
summary_output.write_text(summary, encoding="utf-8")
PY
}

main() {
  case "${1:-}" in
    -h|--help)
      usage
      exit 0
      ;;
  esac

  require_command git
  require_command python3

  collect_kind_source
  if [[ -z "${INPUT_DIR}" ]]; then
    usage >&2
    log "INPUT_DIR is required unless RUN_KIND_A4=true"
    exit 2
  fi
  if [[ ! -d "${INPUT_DIR}" ]]; then
    log "INPUT_DIR does not exist: ${INPUT_DIR}"
    exit 1
  fi

  mkdir -p "${OUTPUT_DIR}"
  write_metadata
  render_report

  log "throughput report written to ${OUTPUT_DIR}"
}

main "$@"
