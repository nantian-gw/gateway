#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/tmp/test-evidence/admin-$(date +%Y%m%d%H%M%S)}"
CONTROLPLANE_ADMIN_URL="${CONTROLPLANE_ADMIN_URL:-http://127.0.0.1:18081}"
DATAPLANE_ADMIN_URL="${DATAPLANE_ADMIN_URL:-http://127.0.0.1:19080}"
CONTROLPLANE_METRICS_URL="${CONTROLPLANE_METRICS_URL:-http://127.0.0.1:18082/metrics}"
DEFAULT_CONTROLPLANE_ADMIN_URL="http://127.0.0.1:18081"
DEFAULT_DATAPLANE_ADMIN_URL="http://127.0.0.1:19080"
DEFAULT_CONTROLPLANE_METRICS_URL="http://127.0.0.1:18082/metrics"
CONTROLPLANE_TOKEN="${CONTROLPLANE_TOKEN:-${PGW_ADMIN_TOKEN:-}}"
DATAPLANE_TOKEN="${DATAPLANE_TOKEN:-${PGW_ADMIN_TOKEN:-}}"
ENABLE_KIND_PORT_FORWARD="${ENABLE_KIND_PORT_FORWARD:-false}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-aether-gateway}"
CONTROLPLANE_ADMIN_SERVICE="${CONTROLPLANE_ADMIN_SERVICE:-aether-gateway-controlplane-admin}"
DATAPLANE_ADMIN_SERVICE="${DATAPLANE_ADMIN_SERVICE:-aether-gateway-dataplane-admin}"
CONTROLPLANE_METRICS_SERVICE="${CONTROLPLANE_METRICS_SERVICE:-aether-gateway-controlplane-metrics}"
CONTROLPLANE_ADMIN_SERVICE_PORT="${CONTROLPLANE_ADMIN_SERVICE_PORT:-18081}"
DATAPLANE_ADMIN_SERVICE_PORT="${DATAPLANE_ADMIN_SERVICE_PORT:-19080}"
CONTROLPLANE_METRICS_SERVICE_PORT="${CONTROLPLANE_METRICS_SERVICE_PORT:-18082}"
INCLUDE_DATAPLANE_METRICS="${INCLUDE_DATAPLANE_METRICS:-true}"
INCLUDE_CONTROLPLANE_METRICS="${INCLUDE_CONTROLPLANE_METRICS:-false}"
STRICT="${STRICT:-true}"
PORT_FORWARD_PIDS=()

log() {
  printf '[collect-admin-snapshots] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

maybe_write_json() {
  local target="$1"
  local payload_file="$2"

  if command -v jq >/dev/null 2>&1; then
    jq . <"${payload_file}" >"${target}"
  else
    cat "${payload_file}" >"${target}"
  fi
}

cleanup_port_forwards() {
  local pid

  for pid in "${PORT_FORWARD_PIDS[@]:-}"; do
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  done
}

url_port() {
  local url="$1"

  printf '%s\n' "${url}" | sed -E 's#^https?://[^:]+:([0-9]+).*$#\1#'
}

url_host() {
  local url="$1"

  printf '%s\n' "${url}" | sed -E 's#^https?://([^:/]+).*$#\1#'
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
      return 0
    fi
  done

  return 1
}

wait_for_http() {
  local url="$1"
  local attempt

  for attempt in $(seq 1 20); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done

  return 1
}

start_port_forward() {
  local service_name="$1"
  local local_port="$2"
  local service_port="$3"
  local probe_url="$4"

  log "port-forwarding svc/${service_name} ${local_port}:${service_port}"
  kubectl -n "${KUBE_NAMESPACE}" port-forward "svc/${service_name}" \
    "${local_port}:${service_port}" >/dev/null 2>&1 &
  PORT_FORWARD_PIDS+=("$!")

  if ! wait_for_http "${probe_url}"; then
    log "failed to establish port-forward for svc/${service_name}"
    exit 1
  fi
}

adjust_port_forward_urls() {
  local host
  local port
  local free_port

  host="$(url_host "${CONTROLPLANE_ADMIN_URL}")"
  port="$(url_port "${CONTROLPLANE_ADMIN_URL}")"
  if [[ "${CONTROLPLANE_ADMIN_URL}" == "${DEFAULT_CONTROLPLANE_ADMIN_URL}" ]] \
    && [[ "${host}" =~ ^(127\.0\.0\.1|localhost)$ ]] \
    && is_tcp_port_listening "${port}"; then
    free_port="$(find_free_tcp_port 28081)"
    CONTROLPLANE_ADMIN_URL="http://${host}:${free_port}"
  fi

  host="$(url_host "${DATAPLANE_ADMIN_URL}")"
  port="$(url_port "${DATAPLANE_ADMIN_URL}")"
  if [[ "${DATAPLANE_ADMIN_URL}" == "${DEFAULT_DATAPLANE_ADMIN_URL}" ]] \
    && [[ "${host}" =~ ^(127\.0\.0\.1|localhost)$ ]] \
    && is_tcp_port_listening "${port}"; then
    free_port="$(find_free_tcp_port 29080)"
    DATAPLANE_ADMIN_URL="http://${host}:${free_port}"
  fi

  host="$(url_host "${CONTROLPLANE_METRICS_URL}")"
  port="$(url_port "${CONTROLPLANE_METRICS_URL}")"
  if [[ "${CONTROLPLANE_METRICS_URL}" == "${DEFAULT_CONTROLPLANE_METRICS_URL}" ]] \
    && [[ "${host}" =~ ^(127\.0\.0\.1|localhost)$ ]] \
    && is_tcp_port_listening "${port}"; then
    free_port="$(find_free_tcp_port 28082)"
    CONTROLPLANE_METRICS_URL="http://${host}:${free_port}/metrics"
  fi
}

fetch_endpoint() {
  local component="$1"
  local path="$2"
  local output_name="$3"
  local token="${4:-}"
  local kind="${5:-json}"
  local url
  local tmp_file
  local target_dir
  local target_path
  local curl_args=()

  case "${component}" in
    controlplane)
      url="${CONTROLPLANE_ADMIN_URL}${path}"
      ;;
    dataplane)
      url="${DATAPLANE_ADMIN_URL}${path}"
      ;;
    controlplane-metrics)
      url="${CONTROLPLANE_METRICS_URL}"
      ;;
    *)
      log "unsupported component: ${component}"
      exit 1
      ;;
  esac

  if [[ -n "${token}" ]]; then
    curl_args+=(-H "Authorization: Bearer ${token}")
  fi

  target_dir="${OUTPUT_DIR}/${component}"
  target_path="${target_dir}/${output_name}"
  mkdir -p "${target_dir}"
  tmp_file="$(mktemp)"

  if curl -fsS "${curl_args[@]}" "${url}" >"${tmp_file}"; then
    case "${kind}" in
      json)
        maybe_write_json "${target_path}" "${tmp_file}"
        ;;
      *)
        cat "${tmp_file}" >"${target_path}"
        ;;
    esac
    log "captured ${component}${path} -> ${target_path}"
    rm -f "${tmp_file}"
    return 0
  fi

  log "failed to capture ${component}${path}"
  if [[ "${STRICT}" == "true" ]]; then
    rm -f "${tmp_file}"
    exit 1
  fi

  {
    printf 'capture failed\n'
    printf 'component=%s\n' "${component}"
    printf 'path=%s\n' "${path}"
    printf 'url=%s\n' "${url}"
  } >"${target_path}.error.txt"
  rm -f "${tmp_file}"
}

write_metadata() {
  local metadata="${OUTPUT_DIR}/metadata.txt"

  mkdir -p "${OUTPUT_DIR}"
  {
    printf 'captured_at=%s\n' "$(date --iso-8601=seconds)"
    printf 'controlplane_admin_url=%s\n' "${CONTROLPLANE_ADMIN_URL}"
    printf 'dataplane_admin_url=%s\n' "${DATAPLANE_ADMIN_URL}"
    printf 'controlplane_metrics_url=%s\n' "${CONTROLPLANE_METRICS_URL}"
    printf 'enable_kind_port_forward=%s\n' "${ENABLE_KIND_PORT_FORWARD}"
    printf 'kube_namespace=%s\n' "${KUBE_NAMESPACE}"
    printf 'include_dataplane_metrics=%s\n' "${INCLUDE_DATAPLANE_METRICS}"
    printf 'include_controlplane_metrics=%s\n' "${INCLUDE_CONTROLPLANE_METRICS}"
    printf 'strict=%s\n' "${STRICT}"
    if git -C "${ROOT_DIR}" rev-parse --short HEAD >/dev/null 2>&1; then
      printf 'git_commit=%s\n' "$(git -C "${ROOT_DIR}" rev-parse --short HEAD)"
    fi
  } >"${metadata}"
}

main() {
  require_command curl
  if [[ "${ENABLE_KIND_PORT_FORWARD}" == "true" ]]; then
    require_command kubectl
    trap cleanup_port_forwards EXIT
    adjust_port_forward_urls
    start_port_forward \
      "${CONTROLPLANE_ADMIN_SERVICE}" \
      "$(url_port "${CONTROLPLANE_ADMIN_URL}")" \
      "${CONTROLPLANE_ADMIN_SERVICE_PORT}" \
      "${CONTROLPLANE_ADMIN_URL}/livez"
    start_port_forward \
      "${DATAPLANE_ADMIN_SERVICE}" \
      "$(url_port "${DATAPLANE_ADMIN_URL}")" \
      "${DATAPLANE_ADMIN_SERVICE_PORT}" \
      "${DATAPLANE_ADMIN_URL}/livez"
    if [[ "${INCLUDE_CONTROLPLANE_METRICS}" == "true" ]]; then
      start_port_forward \
        "${CONTROLPLANE_METRICS_SERVICE}" \
        "$(url_port "${CONTROLPLANE_METRICS_URL}")" \
        "${CONTROLPLANE_METRICS_SERVICE_PORT}" \
        "${CONTROLPLANE_METRICS_URL}"
    fi
  fi

  write_metadata

  fetch_endpoint controlplane "/livez" "livez.txt" "${CONTROLPLANE_TOKEN}" text
  fetch_endpoint controlplane "/readyz" "readyz.txt" "${CONTROLPLANE_TOKEN}" text
  fetch_endpoint controlplane "/v1/summary" "summary.json" "${CONTROLPLANE_TOKEN}" json
  fetch_endpoint controlplane "/v1/snapshot-sync" "snapshot-sync.json" "${CONTROLPLANE_TOKEN}" json
  fetch_endpoint controlplane "/v1/snapshot" "snapshot.json" "${CONTROLPLANE_TOKEN}" json
  fetch_endpoint controlplane "/v1/listeners" "listeners.json" "${CONTROLPLANE_TOKEN}" json
  fetch_endpoint controlplane "/v1/routes" "routes.json" "${CONTROLPLANE_TOKEN}" json
  fetch_endpoint controlplane "/v1/backends?all=true" "backends-all.json" "${CONTROLPLANE_TOKEN}" json
  fetch_endpoint controlplane "/v1/nodes" "nodes.json" "${CONTROLPLANE_TOKEN}" json

  fetch_endpoint dataplane "/livez" "livez.txt" "${DATAPLANE_TOKEN}" text
  fetch_endpoint dataplane "/readyz" "readyz.txt" "${DATAPLANE_TOKEN}" text
  fetch_endpoint dataplane "/v1/summary" "summary.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/node" "node.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/snapshot" "snapshot.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/listeners" "listeners.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/routes" "routes.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/backends" "backends.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/traffic" "traffic.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/overload" "overload.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/circuit-breakers" "circuit-breakers.json" "${DATAPLANE_TOKEN}" json
  fetch_endpoint dataplane "/v1/rate-limits" "rate-limits.json" "${DATAPLANE_TOKEN}" json

  if [[ "${INCLUDE_DATAPLANE_METRICS}" == "true" ]]; then
    fetch_endpoint dataplane "/metrics" "metrics.prom" "${DATAPLANE_TOKEN}" text
  fi

  if [[ "${INCLUDE_CONTROLPLANE_METRICS}" == "true" ]]; then
    fetch_endpoint controlplane-metrics "" "metrics.prom" "" text
  fi

  log "snapshots written to ${OUTPUT_DIR}"
}

main "$@"
