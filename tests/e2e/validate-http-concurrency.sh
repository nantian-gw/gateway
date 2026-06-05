#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-gw}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
AETHER_NAMESPACE="${AETHER_NAMESPACE:-nantian-gw}"
TEST_HOST="${TEST_HOST:-example.com}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
ENSURE_KIND="${ENSURE_KIND:-false}"
REQUESTS_STEADY="${REQUESTS_STEADY:-2000}"
CONCURRENCY_STEADY="${CONCURRENCY_STEADY:-64}"
REQUESTS_BURST="${REQUESTS_BURST:-4000}"
CONCURRENCY_BURST="${CONCURRENCY_BURST:-128}"
REQUEST_TIMEOUT_SECONDS="${REQUEST_TIMEOUT_SECONDS:-10}"
CONNECT_TIMEOUT_SECONDS="${CONNECT_TIMEOUT_SECONDS:-3}"
MAX_STEADY_P99_MS="${MAX_STEADY_P99_MS:-2500}"
MAX_BURST_P99_MS="${MAX_BURST_P99_MS:-3500}"
HTTP_CLIENT="${ROOT_DIR}/tests/e2e/http_concurrency_client.py"
TMP_DIR=""
SUCCESS="false"

log() {
  printf '[http-concurrency] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

k() {
  kubectl --context "${KUBE_CONTEXT}" "$@"
}

kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

nantian_stack_ready() {
  k -n "${AETHER_NAMESPACE}" get deployment nantian-controlplane nantian-dataplane >/dev/null 2>&1
}

smoke_http_ready() {
  curl -fsS -H "Host: ${TEST_HOST}" "http://127.0.0.1:${GATEWAY_HOST_PORT}/" 2>/dev/null | grep -q "nantian-gw-ok"
}

bootstrap_kind_stack() {
  local skip_build="${SKIP_BUILD:-false}"

  if kind_cluster_exists; then
    skip_build="${SKIP_BUILD:-true}"
  fi

  log "bootstrapping or refreshing kind gateway stack via tests/e2e/run-kind.sh"
  (
    cd "${ROOT_DIR}"
    SKIP_BUILD="${skip_build}" ./tests/e2e/run-kind.sh
  )
}

ensure_kind_stack() {
  if ! kind_cluster_exists; then
    if [[ "${ENSURE_KIND}" != "true" ]]; then
      log "kind cluster ${CLUSTER_NAME} does not exist; run ./tests/e2e/run-kind.sh first or rerun with ENSURE_KIND=true"
      exit 1
    fi
    bootstrap_kind_stack
    return
  fi

  if ! nantian_stack_ready || ! smoke_http_ready; then
    if [[ "${ENSURE_KIND}" != "true" ]]; then
      log "kind stack is not ready; rerun with ENSURE_KIND=true or refresh it manually"
      exit 1
    fi
    bootstrap_kind_stack
  fi
}

cleanup() {
  local exit_code="$?"

  if [[ "${SUCCESS}" != "true" && -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    for file in "${TMP_DIR}"/*.json; do
      if [[ -f "${file}" ]]; then
        printf '\n[http-concurrency] debug: %s\n' "$(basename "${file}")" >&2
        cat "${file}" >&2
      fi
    done
  fi

  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi

  exit "${exit_code}"
}

run_profile() {
  local label="$1"
  local requests="$2"
  local concurrency="$3"
  local output_file="$4"

  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/" \
    --host-header "${TEST_HOST}" \
    --requests "${requests}" \
    --concurrency "${concurrency}" \
    --connect-timeout "${CONNECT_TIMEOUT_SECONDS}" \
    --request-timeout "${REQUEST_TIMEOUT_SECONDS}" \
    --expect-status 200 \
    --expect-body-substring "nantian-gw-ok" \
    --output "${output_file}" >/dev/null

  log "${label} summary"
  jq '.' "${output_file}"
}

assert_profile() {
  local label="$1"
  local summary_file="$2"
  local expected_requests="$3"
  local max_p99_ms="$4"

  jq -e \
    --argjson expected_requests "${expected_requests}" \
    --argjson max_p99_ms "${max_p99_ms}" '
      .completed == $expected_requests
      and .successes == $expected_requests
      and .body_mismatches == 0
      and ((.error_counts | length) == 0)
      and ((.status_counts | keys) == ["200"])
      and (.latency_ms.p99 <= $max_p99_ms)
    ' "${summary_file}" >/dev/null || {
    log "${label} failed validation checks"
    cat "${summary_file}" >&2
    exit 1
  }
}

main() {
  trap cleanup EXIT

  require_command curl
  require_command jq
  require_command kind
  require_command kubectl
  require_command python3

  TMP_DIR="$(mktemp -d)"
  ensure_kind_stack

  local steady_file="${TMP_DIR}/steady.json"
  local burst_file="${TMP_DIR}/burst.json"

  run_profile "steady" "${REQUESTS_STEADY}" "${CONCURRENCY_STEADY}" "${steady_file}"
  assert_profile "steady" "${steady_file}" "${REQUESTS_STEADY}" "${MAX_STEADY_P99_MS}"

  run_profile "burst" "${REQUESTS_BURST}" "${CONCURRENCY_BURST}" "${burst_file}"
  assert_profile "burst" "${burst_file}" "${REQUESTS_BURST}" "${MAX_BURST_P99_MS}"

  SUCCESS="true"
  log "http concurrency validation passed"
}

main "$@"
