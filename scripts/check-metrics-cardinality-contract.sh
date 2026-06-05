#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${repo_root}/scripts/lib/common.sh"

usage() {
  cat <<'EOF'
usage: check-metrics-cardinality-contract.sh [--repo-root <path>]

Verifies that the metrics cardinality contract, Grafana golden-signal layout,
and Prometheus documentation stay aligned.
EOF
}

log() {
  printf '[metrics-cardinality] %s\n' "$*"
}

require_pattern() {
  local file="$1"
  local pattern="$2"
  local label="$3"

  aeg_require_file "${file}"
  if ! grep -Eq -- "${pattern}" "${file}"; then
    aeg_fail "${file} is missing ${label}: ${pattern}"
  fi
}

validate_grafana_json() {
  local path="$1"

  python3 - "${path}" <<'PY'
import json
import re
from pathlib import Path
import sys

path = Path(sys.argv[1])
data = json.loads(path.read_text(encoding="utf-8"))
panels = data.get("panels", [])
row_titles = [panel.get("title") for panel in panels if panel.get("type") == "row"]
required_rows = [
    "Executive Overview",
    "Traffic SLO",
    "Dataplane Runtime",
    "Resource Pressure",
    "Controlplane",
    "Inventory / Debug",
]
missing_rows = [row for row in required_rows if row not in row_titles]
legacy_rows = [
    row
    for row in row_titles
    if row in {
        "Operator Overview",
        "Controlplane Deep Dive",
        "Dataplane Traffic / Performance Deep Dive",
    }
]
if missing_rows or legacy_rows:
    details = []
    if missing_rows:
        details.append(f"missing: {', '.join(missing_rows)}")
    if legacy_rows:
        details.append(f"legacy: {', '.join(legacy_rows)}")
    raise SystemExit(f"{path} missing production layout rows: {'; '.join(details)}")

row_positions = {title: row_titles.index(title) for title in required_rows}
if [row_positions[title] for title in required_rows] != sorted(
    row_positions[title] for title in required_rows
):
    raise SystemExit(
        f"{path} production layout rows must be ordered: {', '.join(required_rows)}"
    )

stat_titles = {panel.get("title") for panel in panels if panel.get("type") == "stat"}
required_overview_stats = {
    "Ready Pods",
    "QPS",
    "Success Rate",
    "P99 Latency",
    "5xx Rate",
    "xDS ACK p95",
    "CPU Throttle",
    "Memory Working Set",
}
missing_overview_stats = sorted(required_overview_stats - stat_titles)
if missing_overview_stats:
    raise SystemExit(
        f"{path} missing production overview stat panels: "
        + ", ".join(missing_overview_stats)
    )

timeseries_titles = {panel.get("title") for panel in panels if panel.get("type") == "timeseries"}
required_timeseries = {
    "Dataplane Admin API Health",
}
missing_timeseries = sorted(required_timeseries - timeseries_titles)
if missing_timeseries:
    raise SystemExit(
        f"{path} missing production timeseries panels: "
        + ", ".join(missing_timeseries)
    )

template_variables = {
    variable.get("name")
    for variable in data.get("templating", {}).get("list", [])
    if isinstance(variable, dict)
}
required_template_variables = {
    "namespace",
    "pod_dp",
    "job_controlplane",
    "instance_cp",
    "job_dataplane",
    "instance_dp",
}
missing_template_variables = sorted(
    required_template_variables - template_variables
)
if missing_template_variables:
    raise SystemExit(
        f"{path} missing production template variables: "
        + ", ".join(missing_template_variables)
    )

variables_by_name = {
    variable.get("name"): variable
    for variable in data.get("templating", {}).get("list", [])
    if isinstance(variable, dict)
}

def variable_query(name):
    variable = variables_by_name.get(name, {})
    definition = variable.get("definition")
    if isinstance(definition, str) and definition:
        return definition
    query = variable.get("query")
    if isinstance(query, dict) and isinstance(query.get("query"), str):
        return query["query"]
    if isinstance(query, str):
        return query
    return ""

bad_variable_sources = [
    name
    for name in ["namespace", "pod_dp"]
    if "aether_gateway_dataplane_ready" not in variable_query(name)
    or "aether_gateway_dataplane_container_cpu_cores" in variable_query(name)
]
if bad_variable_sources:
    raise SystemExit(
        f"{path} Grafana namespace and dataplane pod variables must use aether_gateway_dataplane_ready; "
        "do not source them from optional container resource recording rules"
    )

text = json.dumps(data, sort_keys=True)
exprs = []
panel_exprs = []

def collect_exprs(value):
    if isinstance(value, dict):
        expr = value.get("expr")
        if isinstance(expr, str):
            exprs.append(expr)
        for child in value.values():
            collect_exprs(child)
    elif isinstance(value, list):
        for child in value:
            collect_exprs(child)

collect_exprs(data)

def collect_panel_exprs(value):
    if isinstance(value, dict):
        targets = value.get("targets")
        if isinstance(targets, list):
            title = value.get("title")
            description = value.get("description")
            target_exprs = [
                target.get("expr")
                for target in targets
                if isinstance(target, dict) and isinstance(target.get("expr"), str)
            ]
            panel_exprs.append((title or "", description or "", target_exprs))
        for child in value.values():
            collect_panel_exprs(child)
    elif isinstance(value, list):
        for child in value:
            collect_panel_exprs(child)

collect_panel_exprs(data)

request_latency_protocol_selector = (
    'protocol=~"HTTP|HTTPS|GRPC|GRPCS|H2C|HTTP2|HTTP/2"'
)

hardcoded_namespace_exprs = [
    expr for expr in exprs if 'namespace="aether-gateway"' in expr
]
if hardcoded_namespace_exprs:
    raise SystemExit(
        f"{path} contains hardcoded Kubernetes namespace in Grafana expressions; "
        "use namespace=~\"$namespace\" and the namespace variable instead"
    )

precomputed_ratio_gauges = [
    "aether_gateway_dataplane_traffic_retry_rate",
    "aether_gateway_dataplane_traffic_failover_success_rate",
    "aether_gateway_dataplane_traffic_upstream_pool_hit_ratio",
    "aether_gateway_dataplane_traffic_upstream_connect_latency_ms_average",
    "aether_gateway_dataplane_container_cpu_throttle_ratio",
]
bad_ratio_exprs = [
    expr
    for expr in exprs
    if any(f"avg({metric}" in expr.replace(" ", "") for metric in precomputed_ratio_gauges)
]
if bad_ratio_exprs:
    raise SystemExit(
        f"{path} must not average precomputed dataplane ratio gauges; "
        "derive replica-level ratios from rate() counters instead"
    )

replicated_inventory_gauges = [
    "aether_gateway_dataplane_listener_count",
    "aether_gateway_dataplane_http_route_count",
    "aether_gateway_dataplane_grpc_route_count",
    "aether_gateway_dataplane_stream_route_count",
    "aether_gateway_dataplane_backend_count",
    "aether_gateway_dataplane_secret_count",
    "aether_gateway_dataplane_session_persistence_route_rule_count",
    "aether_gateway_dataplane_session_persistence_backend_policy_count",
]
bad_inventory_sum_exprs = [
    expr
    for expr in exprs
    if any(f"sum({metric}" in expr.replace(" ", "") for metric in replicated_inventory_gauges)
]
if bad_inventory_sum_exprs:
    raise SystemExit(
        f"{path} must not sum replicated dataplane inventory gauges; "
        "use max() for snapshot inventory so multi-replica dashboards do not multiply configured resource counts"
    )

bad_listener_state_sum_exprs = [
    expr
    for expr in exprs
    if re.search(
        r"\bsum(?:\s+(?:by|without)\s*\([^)]*\))?\s*\(\s*aether_gateway_dataplane_listener_.*_count\b",
        expr,
    )
]
if bad_listener_state_sum_exprs:
    raise SystemExit(
        f"{path} must not sum replicated dataplane listener state gauges; "
        "use max() for per-listener state, attention, recovery, convergence, and serving counts so multi-replica dashboards do not multiply the same listener set"
    )

runtime_current_failure_gauges = [
    "aether_gateway_dataplane_runtime_http_current_failure_count",
    "aether_gateway_dataplane_runtime_stream_current_failure_count",
    "aether_gateway_dataplane_runtime_tls_current_failure_count",
]
bad_runtime_current_failure_sum_exprs = [
    expr
    for expr in exprs
    if any(f"sum({metric}" in expr.replace(" ", "") for metric in runtime_current_failure_gauges)
]
if bad_runtime_current_failure_sum_exprs:
    raise SystemExit(
        f"{path} must not sum replicated dataplane runtime current failure gauges; "
        "use max() so listener failure counts are not multiplied by dataplane replica count"
    )

bad_http_ratio_denominator_exprs = [
    expr
    for expr in exprs
    if "/" in expr
    and "aether_gateway_dataplane_traffic_events_total" in expr
    and any(
        metric in expr
        for metric in [
            "aether_gateway_dataplane_traffic_status_5xx_total",
            "aether_gateway_dataplane_traffic_retried_events_total",
        ]
    )
]
if bad_http_ratio_denominator_exprs:
    raise SystemExit(
        f"{path} must not use total traffic events as an HTTP ratio denominator; "
        "derive HTTP success, error, and retry ratios from HTTP status counters so TCP/UDP traffic cannot dilute them"
    )

bad_http_request_panel_event_exprs = [
    title
    for title, description, target_exprs in panel_exprs
    if "http request" in f"{title} {description}".lower()
    and any("aether_gateway_dataplane_traffic_events_total" in expr for expr in target_exprs)
]
if bad_http_request_panel_event_exprs:
    raise SystemExit(
        f"{path} HTTP request panels must use request event counters; "
        "use aether_gateway_dataplane_traffic_request_events_total so TCP/UDP traffic cannot inflate HTTP request trends"
    )

protection_total_scope_metrics = [
    "aether_gateway_dataplane_http_overload_rejected_total",
    "aether_gateway_dataplane_tcp_overload_rejected_total",
    "aether_gateway_dataplane_udp_overload_rejected_total",
    "aether_gateway_dataplane_http_circuit_breaker_rejected_total",
    "aether_gateway_dataplane_http_rate_limit_rejected_total",
]
bad_protection_total_scope_exprs = [
    expr
    for expr in exprs
    if any(metric in expr for metric in protection_total_scope_metrics)
    and 'scope!="total"' in expr.replace(" ", "")
]
if bad_protection_total_scope_exprs:
    raise SystemExit(
        f"{path} protection total panels must use scope=\"total\" for scoped rejection counter families; "
        "scope!=\"total\" sums child scopes and can double-count when scope semantics expand or overlap"
    )

bad_request_latency_exprs = [
    expr
    for expr in exprs
    if "aether_gateway_dataplane_traffic_request_latency_ms_bucket" in expr
    and request_latency_protocol_selector not in expr
]
if bad_request_latency_exprs:
    raise SystemExit(
        f"{path} request latency queries must filter to the full request protocol selector "
        f"{request_latency_protocol_selector} so TCP/UDP session and datagram completion "
        "latency cannot pollute request SLOs and H2C/HTTP2/GRPCS request latency is not hidden"
    )

bad_xds_freshness_exprs = [
    expr
    for expr in exprs
    if "time()" in expr
    and "max(aether_gateway_dataplane_xds_last_apply_timestamp_seconds" in expr.replace(" ", "")
]
if bad_xds_freshness_exprs:
    raise SystemExit(
        f"{path} xDS freshness queries must subtract the minimum last apply timestamp "
        "so replica-wide panels show the stalest dataplane node, not the freshest one"
    )

bad_xds_freshness_zero_exprs = [
    expr
    for expr in exprs
    if "time()" in expr
    and "min(aether_gateway_dataplane_xds_last_apply_timestamp_seconds" in expr.replace(" ", "")
    and "aether_gateway_dataplane_xds_last_apply_timestamp_seconds" in expr
    and ">0" not in expr.replace(" ", "")
]
if bad_xds_freshness_zero_exprs:
    raise SystemExit(
        f"{path} xDS freshness queries must ignore zero last apply timestamps "
        "because 0 means no successful apply and would otherwise render epoch-age freshness"
    )

unsupported_metric_names = {
    "aether_requests_total": "aether_gateway_dataplane_traffic_request_events_total",
    "aether_latency_p50_ms": "histogram_quantile() over aether_gateway_dataplane_traffic_request_latency_ms_bucket",
    "aether_latency_p99_ms": "histogram_quantile() over aether_gateway_dataplane_traffic_request_latency_ms_bucket",
    "aether_errors_total": "aether_gateway_dataplane_traffic_status_5xx_total or aether_gateway_dataplane_traffic_response_flags_total",
    "aether_upstream_requests_total": "aether_gateway_dataplane_traffic_upstream_pool_hits_total plus aether_gateway_dataplane_traffic_upstream_pool_misses_total",
    "aether_upstream_connection_pool_size": "aether_gateway_dataplane_traffic_upstream_pool_hits_total and aether_gateway_dataplane_traffic_upstream_pool_misses_total",
    "aether_upstream_connections_active": "aether_gateway_dataplane_http_global_inflight_current or transport-specific inflight gauges",
    "aether_memory_rss_bytes": "process_resident_memory_bytes",
    "aether_task_queue_depth": "aether_gateway_dataplane_access_log_writer_queue_depth or transport-specific queue gauges",
    "aether_tasks_active": "aether_gateway_dataplane_http_global_inflight_current and runtime plane gauges",
    "aether_gateway_controlplane_gateway_convergence_stage_total": "aether_gateway_controlplane_gateway_convergence_stage_current",
}
stale_metric_names = sorted(name for name in unsupported_metric_names if name in text)
if stale_metric_names:
    mappings = ", ".join(
        f"{name} -> {unsupported_metric_names[name]}" for name in stale_metric_names
    )
    if "aether_gateway_controlplane_gateway_convergence_stage_total" in stale_metric_names:
        raise SystemExit(
            f"{path} must use gateway_convergence_stage_current for current Gateway convergence gauges: {mappings}"
        )
    raise SystemExit(f"{path} contains stale or unsupported metric names: {mappings}")

required_metrics = [
    "aether_gateway_dataplane_ready",
    "aether_gateway_dataplane_traffic_events_total",
    "aether_gateway_dataplane_traffic_request_events_total",
    "aether_gateway_dataplane_traffic_status_5xx_total",
    "aether_gateway_dataplane_traffic_response_flags_total",
    "aether_gateway_dataplane_traffic_request_latency_ms_bucket",
    "aether_gateway_dataplane_traffic_upstream_connect_latency_ms_bucket",
    "aether_gateway_dataplane_admin_requests_total",
    "aether_gateway_dataplane_admin_request_duration_seconds_bucket",
    "aether_gateway_controlplane_xds_publish_ack_lag_seconds",
    "aether_gateway_dataplane_xds_connect_failures_total",
    "aether_gateway_dataplane_runtime_tls_current_rejected",
    "aether_gateway_dataplane_runtime_tls_current_failure_count",
    "aether_gateway_dataplane_runtime_tls_listener_reload_failures_total",
    "aether_gateway_dataplane_listener_attention_tls_count",
    "aether_gateway_controlplane_xds_status_report_rejections_total",
    "aether_gateway_controlplane_gateway_convergence_stage_current",
    "aether_gateway_controlplane_gateway_convergence_generation_lag",
    "aether_gateway_controlplane_gateway_programmed_pending_total",
    "process_cpu_seconds_total",
    "process_resident_memory_bytes",
    "process_open_fds",
    "process_threads",
    "aether_gateway_dataplane_container_cpu_cores",
    "aether_gateway_dataplane_container_cpu_request_cores",
    "aether_gateway_dataplane_container_cpu_throttle_ratio",
    "aether_gateway_dataplane_container_memory_working_set_bytes",
    "aether_gateway_dataplane_container_memory_limit_bytes",
    "aether_gateway_dataplane_container_memory_request_bytes",
]
missing_metrics = [metric for metric in required_metrics if metric not in text]
if missing_metrics:
    raise SystemExit(f"{path} missing golden signal metrics: {', '.join(missing_metrics)}")
PY
}

validate_prometheus_rule() {
  local path="$1"

  python3 - "${path}" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")

required_records = [
    "aether_gateway_dataplane_container_cpu_cores",
    "aether_gateway_dataplane_container_cpu_request_cores",
    "aether_gateway_dataplane_container_cpu_throttle_ratio",
    "aether_gateway_dataplane_container_memory_working_set_bytes",
    "aether_gateway_dataplane_container_memory_limit_bytes",
    "aether_gateway_dataplane_container_memory_request_bytes",
]
missing_records = [
    record for record in required_records if f"record: {record}" not in text
]
if missing_records:
    raise SystemExit(
        f"{path} missing container resource recording rules: {', '.join(missing_records)}"
    )

required_sources = [
    "container_cpu_usage_seconds_total",
    "container_cpu_cfs_throttled_periods_total",
    "container_cpu_cfs_periods_total",
    "container_memory_working_set_bytes",
    "kube_pod_container_resource_limits",
    "kube_pod_container_resource_requests",
]
missing_sources = [source for source in required_sources if source not in text]
if missing_sources:
    raise SystemExit(
        f"{path} missing container resource source metrics: {', '.join(missing_sources)}"
    )
PY
}

validate_prometheus_rule_alignment() {
  local operator_path="$1"
  local native_path="$2"

  python3 - "${operator_path}" "${native_path}" <<'PY'
from pathlib import Path
import re
import sys

operator_path = Path(sys.argv[1])
native_path = Path(sys.argv[2])

record_re = re.compile(r"^\s*-\s*record:\s*([A-Za-z_:][A-Za-z0-9_:]*)\s*$", re.MULTILINE)

def records(path):
    found = record_re.findall(path.read_text(encoding="utf-8"))
    if not found:
        raise SystemExit(f"{path} has no recording rules")
    return set(found)

operator_records = records(operator_path)
native_records = records(native_path)

missing_native = sorted(operator_records - native_records)
extra_native = sorted(native_records - operator_records)
if missing_native or extra_native:
    details = []
    if missing_native:
        details.append(f"missing from native: {', '.join(missing_native)}")
    if extra_native:
        details.append(f"extra in native: {', '.join(extra_native)}")
    raise SystemExit(
        f"recording rule drift between {operator_path} and {native_path}: "
        + "; ".join(details)
    )
PY
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --repo-root"
      }
      repo_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      aeg_usage_error "unknown argument: $1"
      ;;
  esac
done

contract_path="${repo_root}/docs/contracts/metrics-cardinality.md"
versioning_path="${repo_root}/docs/contracts/versioning.md"
prometheus_readme_path="${repo_root}/deploy/observability/prometheus/README.md"
prometheus_rule_path="${repo_root}/deploy/observability/prometheus/operator/prometheusrule-dataplane.yaml"
native_prometheus_rule_path="${repo_root}/deploy/observability/prometheus/native/prometheus-dataplane-rules.yaml"
native_controlplane_scrape_path="${repo_root}/deploy/observability/prometheus/native/prometheus-controlplane-scrape.yaml"
operator_controlplane_podmonitor_path="${repo_root}/deploy/observability/prometheus/operator/podmonitor-controlplane.yaml"
operator_controlplane_servicemonitor_path="${repo_root}/deploy/observability/prometheus/operator/servicemonitor-controlplane.yaml"
operator_prometheus_scrape_networkpolicy_path="${repo_root}/deploy/observability/prometheus/operator/networkpolicy-prometheus-scrape.yaml"
grafana_json_path="${repo_root}/deploy/observability/grafana/aether-gateway-observability-dashboard.json"

aeg_require_file "${contract_path}"
aeg_require_file "${versioning_path}"
aeg_require_file "${prometheus_readme_path}"
aeg_require_file "${prometheus_rule_path}"
aeg_require_file "${native_prometheus_rule_path}"
aeg_require_file "${native_controlplane_scrape_path}"
aeg_require_file "${operator_controlplane_podmonitor_path}"
aeg_require_file "${operator_controlplane_servicemonitor_path}"
aeg_require_file "${operator_prometheus_scrape_networkpolicy_path}"
aeg_require_file "${grafana_json_path}"

require_pattern "${contract_path}" '^# Metrics Cardinality And Golden Signals$' "contract title"
require_pattern "${contract_path}" '^## Cardinality Classes$' "cardinality class section"
require_pattern "${contract_path}" '^## Gateway Golden Signals$' "golden signal section"
require_pattern "${contract_path}" '^## Current Gaps$' "current gaps section"

for label in \
  'listener' \
  'method' \
  'protocol' \
  'route_kind' \
  'status_class' \
  'response_flag' \
  'route_namespace' \
  'route_name' \
  'backend' \
  'node_id' \
  'snapshot_version' \
  'request_id' \
  'raw host' \
  'path' \
  'raw error'; do
  require_pattern "${contract_path}" "${label}" "label policy for ${label}"
done

for signal in \
  'request success rate' \
  'p99 / p999 latency' \
  'upstream connect latency' \
  'dataplane admin API health' \
  'snapshot apply latency' \
  'snapshot ACK latency' \
  'dataplane ready replicas' \
  'error flag / 5xx rate' \
  'xDS reconnect rate' \
  'RSS / FD / thread slope'; do
  require_pattern "${contract_path}" "${signal}" "golden signal ${signal}"
done

for metric in \
  'aether_gateway_dataplane_traffic_events_total' \
  'aether_gateway_dataplane_traffic_request_events_total' \
  'aether_gateway_dataplane_traffic_status_5xx_total' \
  'aether_gateway_dataplane_traffic_response_flags_total' \
  'aether_gateway_dataplane_traffic_request_latency_ms_bucket' \
  'aether_gateway_dataplane_traffic_upstream_connect_latency_ms_bucket' \
  'aether_gateway_dataplane_traffic_upstream_connect_latency_ms_average' \
  'aether_gateway_dataplane_admin_requests_total' \
  'aether_gateway_dataplane_admin_request_duration_seconds_bucket' \
  'aether_gateway_controlplane_xds_publish_ack_lag_seconds' \
  'aether_gateway_controlplane_xds_status_report_rejections_total' \
  'aether_gateway_controlplane_gateway_convergence_stage_current' \
  'aether_gateway_controlplane_gateway_convergence_generation_lag' \
  'aether_gateway_controlplane_gateway_programmed_pending_total' \
  'process_cpu_seconds_total' \
  'process_resident_memory_bytes' \
  'process_open_fds' \
  'process_threads' \
  'aether_gateway_dataplane_container_cpu_cores' \
  'aether_gateway_dataplane_container_cpu_request_cores' \
  'aether_gateway_dataplane_container_cpu_throttle_ratio' \
  'aether_gateway_dataplane_container_memory_working_set_bytes' \
  'aether_gateway_dataplane_container_memory_limit_bytes' \
  'aether_gateway_dataplane_container_memory_request_bytes' \
  'aether_gateway_dataplane_ready_replicas'; do
  require_pattern "${contract_path}" "${metric}" "metric mapping for ${metric}"
done

require_pattern "${versioning_path}" 'metrics-cardinality\.md' "metrics contract link"
require_pattern "${prometheus_readme_path}" 'metrics-cardinality\.md' "Prometheus README metrics contract link"
require_pattern "${prometheus_readme_path}" 'prometheus-controlplane-scrape\.yaml' "Prometheus README native controlplane scrape file"
require_pattern "${prometheus_readme_path}" 'prometheus-dataplane-rules\.yaml' "Prometheus README native rules file"
require_pattern "${prometheus_readme_path}" 'servicemonitor-controlplane\.yaml' "Prometheus README Operator controlplane ServiceMonitor"
require_pattern "${prometheus_readme_path}" 'networkpolicy-prometheus-scrape\.yaml' "Prometheus README scrape NetworkPolicy"
require_pattern "${prometheus_readme_path}" 'rule_files' "Prometheus README native rule_files wiring"
require_pattern "${prometheus_readme_path}" 'cAdvisor' "Prometheus README cAdvisor prerequisite"
require_pattern "${prometheus_readme_path}" 'kube-state-metrics' "Prometheus README kube-state-metrics prerequisite"
require_pattern "${prometheus_readme_path}" 'aether_gateway_dataplane_admin_requests_total' "Prometheus README dataplane admin request query"
require_pattern "${prometheus_readme_path}" 'aether_gateway_dataplane_admin_request_duration_seconds_bucket' "Prometheus README dataplane admin duration query"
validate_prometheus_rule "${prometheus_rule_path}"
validate_prometheus_rule "${native_prometheus_rule_path}"
validate_prometheus_rule_alignment "${prometheus_rule_path}" "${native_prometheus_rule_path}"
validate_grafana_json "${grafana_json_path}"

log "metrics cardinality contract aligned"
