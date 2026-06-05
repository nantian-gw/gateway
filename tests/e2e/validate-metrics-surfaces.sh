#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
ENSURE_KIND="${ENSURE_KIND:-false}"
OUTPUT_DIR="${OUTPUT_DIR:-$(mktemp -d "${ROOT_DIR}/tmp/metrics-surfaces.XXXXXX")}"
KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-false}"
DATAPLANE_NAMESPACE="${DATAPLANE_NAMESPACE:-aether-gateway}"
DATAPLANE_DEPLOYMENT="${DATAPLANE_DEPLOYMENT:-aether-gateway-dataplane}"
DATAPLANE_SELECTOR="${DATAPLANE_SELECTOR:-app=aether-gateway-dataplane}"
DATAPLANE_ADMIN_PORT="${DATAPLANE_ADMIN_PORT:-19080}"
DATAPLANE_TOKEN="${DATAPLANE_TOKEN:-${PGW_ADMIN_TOKEN:-}}"
MULTI_REPLICA_SCRAPE_REPLICAS="${MULTI_REPLICA_SCRAPE_REPLICAS:-2}"
SUCCESS="false"
ORIGINAL_DATAPLANE_REPLICAS=""
POD_PORT_FORWARD_PIDS=()

log() {
  printf '[metrics-surfaces] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

is_tcp_port_listening() {
  local port="$1"

  ss -H -ltn "( sport = :${port} )" 2>/dev/null | grep -q .
}

find_free_tcp_port() {
  local start_port="$1"
  local port

  for port in $(seq "${start_port}" "$((start_port + 50))"); do
    if ! is_tcp_port_listening "${port}"; then
      printf '%s\n' "${port}"
      return
    fi
  done

  fail "failed to find a free TCP port starting at ${start_port}"
}

wait_for_http() {
  local url="$1"

  for _ in $(seq 1 30); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return
    fi
    sleep 0.5
  done

  return 1
}

cleanup() {
  cleanup_pod_port_forwards
  restore_dataplane_replicas

  if [[ "${SUCCESS}" == "true" || "${KEEP_ARTIFACTS}" != "true" ]]; then
    rm -rf "${OUTPUT_DIR}" >/dev/null 2>&1 || true
  else
    log "artifacts kept at ${OUTPUT_DIR}"
  fi
}
trap cleanup EXIT

cleanup_pod_port_forwards() {
  local pid

  for pid in "${POD_PORT_FORWARD_PIDS[@]:-}"; do
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  done
  POD_PORT_FORWARD_PIDS=()
}

debug_dump() {
  if [[ -d "${OUTPUT_DIR}/admin" ]]; then
    printf '\n[metrics-surfaces] debug: dataplane snapshot\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/snapshot.json" >&2 || true
    printf '\n[metrics-surfaces] debug: dataplane traffic\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/traffic.json" >&2 || true
    printf '\n[metrics-surfaces] debug: dataplane overload\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/overload.json" >&2 || true
    printf '\n[metrics-surfaces] debug: dataplane circuit-breakers\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/circuit-breakers.json" >&2 || true
    printf '\n[metrics-surfaces] debug: dataplane rate-limits\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/rate-limits.json" >&2 || true
    printf '\n[metrics-surfaces] debug: dataplane metrics excerpt\n' >&2
    sed -n '1,120p' "${OUTPUT_DIR}/admin/dataplane/metrics.prom" >&2 || true
    if [[ -d "${OUTPUT_DIR}/admin/dataplane-pod-metrics" ]]; then
      printf '\n[metrics-surfaces] debug: dataplane pod metrics files\n' >&2
      find "${OUTPUT_DIR}/admin/dataplane-pod-metrics" -maxdepth 1 -type f -print >&2 || true
    fi
    printf '\n[metrics-surfaces] debug: controlplane metrics excerpt\n' >&2
    sed -n '1,120p' "${OUTPUT_DIR}/admin/controlplane-metrics/metrics.prom" >&2 || true
  fi
}

fail() {
  log "$1"
  debug_dump
  exit 1
}

ensure_kind_cluster() {
  if kind_cluster_exists; then
    return
  fi
  if [[ "${ENSURE_KIND}" != "true" ]]; then
    fail "kind cluster ${CLUSTER_NAME} does not exist; run ./tests/e2e/run-kind.sh first or rerun with ENSURE_KIND=true"
  fi

  log "bootstrapping kind cluster via tests/e2e/run-kind.sh"
  (
    cd "${ROOT_DIR}"
    SKIP_BUILD="${SKIP_BUILD:-true}" ./tests/e2e/run-kind.sh
  )
}

ready_dataplane_pods() {
  kubectl -n "${DATAPLANE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o json \
    | jq -r '
      .items[]
      | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
      | .metadata.name
    ' \
    | sort
}

ready_dataplane_pod_count() {
  ready_dataplane_pods | wc -l | tr -d ' '
}

wait_for_dataplane_ready_pods() {
  local minimum="$1"
  local count

  for _ in $(seq 1 90); do
    count="$(ready_dataplane_pod_count)"
    if [[ "${count}" -ge "${minimum}" ]]; then
      return
    fi
    sleep 2
  done

  kubectl -n "${DATAPLANE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o wide >&2 || true
  fail "dataplane ready pod count did not reach ${minimum}"
}

ensure_dataplane_multi_replica() {
  if [[ "${MULTI_REPLICA_SCRAPE_REPLICAS}" -lt 2 ]]; then
    return
  fi

  ORIGINAL_DATAPLANE_REPLICAS="$(
    kubectl -n "${DATAPLANE_NAMESPACE}" get deployment "${DATAPLANE_DEPLOYMENT}" -o jsonpath='{.spec.replicas}'
  )"
  if [[ -z "${ORIGINAL_DATAPLANE_REPLICAS}" ]]; then
    fail "failed to read ${DATAPLANE_NAMESPACE}/${DATAPLANE_DEPLOYMENT} replica count"
  fi

  if [[ "${ORIGINAL_DATAPLANE_REPLICAS}" -lt "${MULTI_REPLICA_SCRAPE_REPLICAS}" ]]; then
    log "scaling ${DATAPLANE_NAMESPACE}/${DATAPLANE_DEPLOYMENT} to ${MULTI_REPLICA_SCRAPE_REPLICAS} replicas for multi-pod metrics scrape"
    kubectl -n "${DATAPLANE_NAMESPACE}" scale deployment "${DATAPLANE_DEPLOYMENT}" \
      --replicas="${MULTI_REPLICA_SCRAPE_REPLICAS}" >/dev/null
    kubectl -n "${DATAPLANE_NAMESPACE}" rollout status deployment/"${DATAPLANE_DEPLOYMENT}" --timeout=240s >/dev/null
  fi

  wait_for_dataplane_ready_pods "${MULTI_REPLICA_SCRAPE_REPLICAS}"
}

restore_dataplane_replicas() {
  if [[ -z "${ORIGINAL_DATAPLANE_REPLICAS}" || "${MULTI_REPLICA_SCRAPE_REPLICAS}" -lt 2 ]]; then
    return
  fi
  if [[ "${ORIGINAL_DATAPLANE_REPLICAS}" -ge "${MULTI_REPLICA_SCRAPE_REPLICAS}" ]]; then
    return
  fi

  log "restoring ${DATAPLANE_NAMESPACE}/${DATAPLANE_DEPLOYMENT} to ${ORIGINAL_DATAPLANE_REPLICAS} replicas"
  kubectl -n "${DATAPLANE_NAMESPACE}" scale deployment "${DATAPLANE_DEPLOYMENT}" \
    --replicas="${ORIGINAL_DATAPLANE_REPLICAS}" >/dev/null 2>&1 || true
  kubectl -n "${DATAPLANE_NAMESPACE}" rollout status deployment/"${DATAPLANE_DEPLOYMENT}" --timeout=240s >/dev/null 2>&1 || true
}

capture_admin_snapshots() {
  log "capturing admin surfaces and metrics"
  (
    cd "${ROOT_DIR}"
    ENABLE_KIND_PORT_FORWARD=true \
    OUTPUT_DIR="${OUTPUT_DIR}/admin" \
    STRICT=true \
    INCLUDE_CONTROLPLANE_METRICS=true \
    INCLUDE_DATAPLANE_METRICS=true \
    ./scripts/collect-admin-snapshots.sh
  )
}

start_pod_port_forward() {
  local pod="$1"
  local local_port="$2"
  local log_file="$3"

  kubectl -n "${DATAPLANE_NAMESPACE}" port-forward "pod/${pod}" \
    "${local_port}:${DATAPLANE_ADMIN_PORT}" >"${log_file}" 2>&1 &
  POD_PORT_FORWARD_PIDS+=("$!")

  if ! wait_for_http "http://127.0.0.1:${local_port}/livez"; then
    cat "${log_file}" >&2 || true
    fail "failed to establish dataplane pod port-forward for ${pod}"
  fi
}

capture_dataplane_pod_scrapes() {
  local output_dir="${OUTPUT_DIR}/admin/dataplane-pod-metrics"
  local pod
  local port
  local log_file
  local curl_args=()
  local pods=()

  if [[ "${MULTI_REPLICA_SCRAPE_REPLICAS}" -lt 2 ]]; then
    return
  fi

  mapfile -t pods < <(ready_dataplane_pods)
  if [[ "${#pods[@]}" -lt "${MULTI_REPLICA_SCRAPE_REPLICAS}" ]]; then
    fail "expected at least ${MULTI_REPLICA_SCRAPE_REPLICAS} ready dataplane pods for multi-replica scrape"
  fi

  if [[ -n "${DATAPLANE_TOKEN}" ]]; then
    curl_args+=(-H "Authorization: Bearer ${DATAPLANE_TOKEN}")
  fi

  mkdir -p "${output_dir}"
  log "capturing dataplane pod metrics from ${#pods[@]} ready pods"
  for pod in "${pods[@]}"; do
    port="$(find_free_tcp_port 39080)"
    log_file="${output_dir}/${pod}.port-forward.log"
    start_pod_port_forward "${pod}" "${port}" "${log_file}"
    curl -fsS "${curl_args[@]}" "http://127.0.0.1:${port}/metrics" >"${output_dir}/${pod}.metrics.prom"
    curl -fsS "${curl_args[@]}" "http://127.0.0.1:${port}/v1/node" \
      | jq . >"${output_dir}/${pod}.node.json"
    cleanup_pod_port_forwards
  done
}

validate_metrics_consistency() {
  log "validating controlplane and dataplane metrics consistency"
  python3 - "${OUTPUT_DIR}" <<'PY' || exit 1
import json
import re
import sys
from pathlib import Path

output_dir = Path(sys.argv[1])


def load_json(*parts):
    return json.loads((output_dir.joinpath(*parts)).read_text())


def load_text(*parts):
    return output_dir.joinpath(*parts).read_text()


sample_re = re.compile(r'^([A-Za-z_:][A-Za-z0-9_:]*)(?:\{([^}]*)\})?\s+(.+)$')


def parse_labels(raw):
    labels = {}
    if not raw:
        return labels
    for item in raw.split(','):
        name, value = item.split('=', 1)
        labels[name] = value.strip().strip('"')
    return labels


def parse_metrics(text):
    samples = []
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        match = sample_re.match(line)
        if not match:
            continue
        name, labels_raw, value = match.groups()
        samples.append((name, parse_labels(labels_raw), value))
    return samples


def empty_family_count(text):
    current = None
    has_sample = False
    empty = []
    for line in text.splitlines():
        if line.startswith('# HELP '):
            if current is not None and not has_sample:
                empty.append(current)
            current = line.split()[2]
            has_sample = False
            continue
        if line.startswith('#'):
            continue
        if line.strip():
            has_sample = True
    if current is not None and not has_sample:
        empty.append(current)
    return len(empty)


def metric_value(samples, name, labels=None):
    for sample_name, sample_labels, value in samples:
        if sample_name != name:
            continue
        if labels is not None and sample_labels != labels:
            continue
        return value
    raise AssertionError(f'missing metric sample {name} labels={labels}')


def metric_int(samples, name, labels=None):
    return int(float(metric_value(samples, name, labels)))


def ensure_present(samples, name):
    metric_value(samples, name)


dataplane_metrics_text = load_text('admin', 'dataplane', 'metrics.prom')
controlplane_metrics_text = load_text('admin', 'controlplane-metrics', 'metrics.prom')
dataplane_samples = parse_metrics(dataplane_metrics_text)
controlplane_samples = parse_metrics(controlplane_metrics_text)
dataplane_snapshot = load_json('admin', 'dataplane', 'snapshot.json')
dataplane_traffic = load_json('admin', 'dataplane', 'traffic.json')
dataplane_overload = load_json('admin', 'dataplane', 'overload.json')
dataplane_circuit_breakers = load_json('admin', 'dataplane', 'circuit-breakers.json')
dataplane_rate_limits = load_json('admin', 'dataplane', 'rate-limits.json')

route_rule_count = 0
for route in dataplane_snapshot.get('http_routes', []):
    route_rule_count += sum(1 for rule in route.get('rules', []) if rule.get('session_persistence'))
for route in dataplane_snapshot.get('grpc_routes', []):
    route_rule_count += sum(1 for rule in route.get('rules', []) if rule.get('session_persistence'))

backend_policy_count = sum(
    1
    for policy in dataplane_snapshot.get('backend_policies', {}).values()
    if policy.get('session_persistence')
)

checks = [
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_ready'),
        1 if dataplane_snapshot.get('id') else 0,
        'dataplane ready',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_listener_count'),
        len(dataplane_snapshot.get('listeners', [])),
        'dataplane listener_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_http_route_count'),
        len(dataplane_snapshot.get('http_routes', [])),
        'dataplane http_route_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_grpc_route_count'),
        len(dataplane_snapshot.get('grpc_routes', [])),
        'dataplane grpc_route_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_stream_route_count'),
        len(dataplane_snapshot.get('stream_routes', [])),
        'dataplane stream_route_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_backend_count'),
        len(dataplane_snapshot.get('backends', [])),
        'dataplane backend_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_secret_count'),
        len(dataplane_snapshot.get('secrets', [])),
        'dataplane secret_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_session_persistence_active'),
        1 if (route_rule_count + backend_policy_count) > 0 else 0,
        'dataplane session_persistence_active',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_session_persistence_route_rule_count'),
        route_rule_count,
        'dataplane session_persistence_route_rule_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_session_persistence_backend_policy_count'),
        backend_policy_count,
        'dataplane session_persistence_backend_policy_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_events_total'),
        dataplane_traffic.get('total_events', 0),
        'dataplane traffic_events_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_request_events_total'),
        dataplane_traffic.get('total_request_events', 0),
        'dataplane traffic_request_events_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_bytes_received_total'),
        dataplane_traffic.get('total_bytes_received', 0),
        'dataplane traffic_bytes_received_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_bytes_sent_total'),
        dataplane_traffic.get('total_bytes_sent', 0),
        'dataplane traffic_bytes_sent_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_latency_ms_total'),
        dataplane_traffic.get('total_latency_ms', 0),
        'dataplane traffic_latency_ms_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_latency_ms_max'),
        dataplane_traffic.get('max_latency_ms', 0),
        'dataplane traffic_latency_ms_max',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_retried_events_total'),
        dataplane_traffic.get('total_retried_events', 0),
        'dataplane traffic_retried_events_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_retry_attempts_total'),
        dataplane_traffic.get('total_retry_attempts', 0),
        'dataplane traffic_retry_attempts_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_retried_success_events_total'),
        dataplane_traffic.get('total_retried_success_events', 0),
        'dataplane traffic_retried_success_events_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_pool_hits_total'),
        dataplane_traffic.get('total_upstream_pool_hits', 0),
        'dataplane traffic_upstream_pool_hits_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_pool_misses_total'),
        dataplane_traffic.get('total_upstream_pool_misses', 0),
        'dataplane traffic_upstream_pool_misses_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_peer_build_failures_total'),
        dataplane_traffic.get('total_upstream_peer_build_failures', 0),
        'dataplane traffic_upstream_peer_build_failures_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_connect_latency_ms_total'),
        dataplane_traffic.get('total_upstream_connect_latency_ms', 0),
        'dataplane traffic_upstream_connect_latency_ms_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_connect_latency_ms_max'),
        dataplane_traffic.get('max_upstream_connect_latency_ms', 0),
        'dataplane traffic_upstream_connect_latency_ms_max',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_connect_latency_ms_sum'),
        dataplane_traffic.get('total_upstream_connect_latency_ms', 0),
        'dataplane traffic_upstream_connect_latency_ms_sum',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_connect_latency_ms_count'),
        dataplane_traffic.get('total_upstream_connect_latency_observations', 0),
        'dataplane traffic_upstream_connect_latency_ms_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_tls_handshake_failures_total'),
        dataplane_traffic.get('total_upstream_tls_handshake_failures', 0),
        'dataplane traffic_upstream_tls_handshake_failures_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_tls_handshake_failure_latency_ms_sum'),
        dataplane_traffic.get('total_upstream_tls_handshake_failure_latency_ms', 0),
        'dataplane traffic_upstream_tls_handshake_failure_latency_ms_sum',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_upstream_tls_handshake_failure_latency_ms_count'),
        dataplane_traffic.get('total_upstream_tls_handshake_failure_latency_observations', 0),
        'dataplane traffic_upstream_tls_handshake_failure_latency_ms_count',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_status_1xx_total'),
        dataplane_traffic.get('status_1xx', 0),
        'dataplane traffic_status_1xx_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_status_2xx_total'),
        dataplane_traffic.get('status_2xx', 0),
        'dataplane traffic_status_2xx_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_status_3xx_total'),
        dataplane_traffic.get('status_3xx', 0),
        'dataplane traffic_status_3xx_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_status_4xx_total'),
        dataplane_traffic.get('status_4xx', 0),
        'dataplane traffic_status_4xx_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_status_5xx_total'),
        dataplane_traffic.get('status_5xx', 0),
        'dataplane traffic_status_5xx_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_traffic_status_other_total'),
        dataplane_traffic.get('status_other', 0),
        'dataplane traffic_status_other_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_http_global_inflight_current'),
        dataplane_overload.get('httpGlobalInflightCurrent', 0),
        'dataplane http_global_inflight_current',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_http_overload_rejected_total',
            {'scope': 'total'},
        ),
        dataplane_overload.get('httpRejectedTotal', 0),
        'dataplane http_overload_rejected_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_tcp_global_connections_current'),
        dataplane_overload.get('tcpGlobalConnectionsCurrent', 0),
        'dataplane tcp_global_connections_current',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_tcp_overload_rejected_total',
            {'scope': 'total'},
        ),
        dataplane_overload.get('tcpRejectedTotal', 0),
        'dataplane tcp_overload_rejected_total',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_udp_global_datagrams_current'),
        dataplane_overload.get('udpGlobalDatagramsCurrent', 0),
        'dataplane udp_global_datagrams_current',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_udp_overload_rejected_total',
            {'scope': 'total'},
        ),
        dataplane_overload.get('udpRejectedTotal', 0),
        'dataplane udp_overload_rejected_total',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_http_circuit_breaker_backend_max_inflight_requests',
        ),
        dataplane_circuit_breakers.get('backendMaxInflightRequests', 0),
        'dataplane circuit_breaker_backend_max_inflight_requests',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_http_circuit_breaker_rejected_total',
            {'scope': 'total'},
        ),
        dataplane_circuit_breakers.get('rejectedTotal', 0),
        'dataplane circuit_breaker_rejected_total',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_http_rate_limit_global_requests_per_second',
        ),
        dataplane_rate_limits.get('global', {}).get('requestsPerSecond', 0),
        'dataplane rate_limit_global_requests_per_second',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_http_rate_limit_global_burst'),
        dataplane_rate_limits.get('global', {}).get('burst', 0),
        'dataplane rate_limit_global_burst',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_http_rate_limit_global_available_tokens',
        ),
        dataplane_rate_limits.get('global', {}).get('availableTokens', 0),
        'dataplane rate_limit_global_available_tokens',
    ),
    (
        metric_int(dataplane_samples, 'aether_gateway_dataplane_http_rate_limit_allowed_total'),
        dataplane_rate_limits.get('allowedTotal', 0),
        'dataplane rate_limit_allowed_total',
    ),
    (
        metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_http_rate_limit_rejected_total',
            {'scope': 'total'},
        ),
        dataplane_rate_limits.get('rejectedTotal', 0),
        'dataplane rate_limit_rejected_total',
    ),
]

errors = []
for actual, expected, label in checks:
    if actual != expected:
        errors.append(f'{label}: metrics={actual} expected={expected}')

for flag, expected in dataplane_traffic.get('response_flags', {}).items():
    actual = metric_int(
        dataplane_samples,
        'aether_gateway_dataplane_traffic_response_flags_total',
        {'flag': flag},
    )
    if actual != expected:
        errors.append(f'dataplane traffic_response_flags_total flag={flag}: metrics={actual} expected={expected}')

if 'none' not in dataplane_traffic.get('response_flags', {}):
    actual = metric_int(
        dataplane_samples,
        'aether_gateway_dataplane_traffic_response_flags_total',
        {'flag': 'none'},
    )
    if actual != 0:
        errors.append(f'dataplane traffic_response_flags_total flag=none: metrics={actual} expected=0')

for histogram in dataplane_traffic.get('request_latency_ms_histograms', []):
    labels = {
        'listener': histogram.get('listener', ''),
        'protocol': histogram.get('protocol', ''),
        'route_kind': histogram.get('route_kind', ''),
        'status_class': histogram.get('status_class', ''),
        'response_flag': histogram.get('response_flag', ''),
    }
    actual_sum = metric_int(
        dataplane_samples,
        'aether_gateway_dataplane_traffic_request_latency_ms_sum',
        labels,
    )
    actual_count = metric_int(
        dataplane_samples,
        'aether_gateway_dataplane_traffic_request_latency_ms_count',
        labels,
    )
    if actual_sum != histogram.get('sum', 0):
        errors.append(f'dataplane request latency sum {labels}: metrics={actual_sum} expected={histogram.get("sum", 0)}')
    if actual_count != histogram.get('count', 0):
        errors.append(f'dataplane request latency count {labels}: metrics={actual_count} expected={histogram.get("count", 0)}')
    for bucket in histogram.get('buckets', []):
        bucket_labels = dict(labels)
        bucket_labels['le'] = bucket.get('le', '')
        actual = metric_int(
            dataplane_samples,
            'aether_gateway_dataplane_traffic_request_latency_ms_bucket',
            bucket_labels,
        )
        expected = bucket.get('cumulative_count', 0)
        if actual != expected:
            errors.append(f'dataplane request latency bucket {bucket_labels}: metrics={actual} expected={expected}')

for metric_prefix, traffic_key in (
    (
        'aether_gateway_dataplane_traffic_upstream_connect_latency_ms',
        'upstream_connect_latency_ms_buckets',
    ),
    (
        'aether_gateway_dataplane_traffic_upstream_tls_handshake_failure_latency_ms',
        'upstream_tls_handshake_failure_latency_ms_buckets',
    ),
):
    for bucket in dataplane_traffic.get(traffic_key, []):
        labels = {'le': bucket.get('le', '')}
        actual = metric_int(dataplane_samples, f'{metric_prefix}_bucket', labels)
        expected = bucket.get('cumulative_count', 0)
        if actual != expected:
            errors.append(f'dataplane {metric_prefix}_bucket {labels}: metrics={actual} expected={expected}')

for metric_name in (
    'process_cpu_seconds_total',
    'process_resident_memory_bytes',
    'process_open_fds',
    'process_threads',
):
    try:
        ensure_present(dataplane_samples, metric_name)
    except AssertionError as exc:
        errors.append(f'dataplane {exc}')

if empty_family_count(dataplane_metrics_text) != 0:
    errors.append('dataplane metrics contain header-only families')

pod_metrics_dir = output_dir / 'admin' / 'dataplane-pod-metrics'
pod_metric_files = sorted(pod_metrics_dir.glob('*.metrics.prom'))
if len(pod_metric_files) < 2:
    errors.append(f'expected metrics scrapes from at least 2 dataplane pods, got {len(pod_metric_files)}')

pod_node_ids = set()
for metrics_file in pod_metric_files:
    pod_metrics_text = metrics_file.read_text()
    pod_samples = parse_metrics(pod_metrics_text)
    if empty_family_count(pod_metrics_text) != 0:
        errors.append(f'{metrics_file.name} contains header-only metric families')
    try:
        ready = metric_int(pod_samples, 'aether_gateway_dataplane_ready')
    except AssertionError as exc:
        errors.append(f'{metrics_file.name}: {exc}')
        ready = None
    if ready != 1:
        errors.append(f'{metrics_file.name}: dataplane ready metric is {ready}, expected 1')
    try:
        ensure_present(pod_samples, 'aether_gateway_dataplane_node_info')
    except AssertionError as exc:
        errors.append(f'{metrics_file.name}: {exc}')
    for metric_name in (
        'process_cpu_seconds_total',
        'process_resident_memory_bytes',
        'process_open_fds',
        'process_threads',
    ):
        try:
            ensure_present(pod_samples, metric_name)
        except AssertionError as exc:
            errors.append(f'{metrics_file.name}: {exc}')

    node_file = pod_metrics_dir / metrics_file.name.replace('.metrics.prom', '.node.json')
    try:
        node_payload = json.loads(node_file.read_text())
    except FileNotFoundError:
        errors.append(f'{node_file.name}: missing node admin payload')
        continue
    node_id = node_payload.get('nodeId')
    if not node_id:
        errors.append(f'{node_file.name}: missing nodeId')
    else:
        pod_node_ids.add(node_id)

if len(pod_node_ids) < 2:
    errors.append(f'expected metrics scrapes from at least 2 distinct dataplane nodeIds, got {sorted(pod_node_ids)}')

for metric_name in (
    'aether_gateway_snapshot_builds_total',
    'aether_gateway_snapshot_published_total',
    'aether_gateway_controlplane_admin_requests_total',
    'go_threads',
    'process_cpu_seconds_total',
):
    try:
        ensure_present(controlplane_samples, metric_name)
    except AssertionError as exc:
        errors.append(str(exc))

for route in ('livez', 'readyz', 'summary'):
    try:
        value = metric_int(
            controlplane_samples,
            'aether_gateway_controlplane_admin_requests_total',
            {'method': 'GET', 'route': route, 'status_class': '2xx'},
        )
    except AssertionError as exc:
        errors.append(str(exc))
        continue
    if value < 1:
        errors.append(f'controlplane admin metric for route={route} did not record requests')

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    sys.exit(1)
PY
}

main() {
  require_command curl
  require_command jq
  require_command kind
  require_command kubectl
  require_command python3
  require_command ss

  ensure_kind_cluster
  ensure_dataplane_multi_replica
  capture_admin_snapshots
  capture_dataplane_pod_scrapes
  validate_metrics_consistency

  SUCCESS="true"
  log "metrics surface validation passed"
}

main "$@"
