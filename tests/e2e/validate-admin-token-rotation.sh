#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-aether-gateway}"
CONTROLPLANE_DEPLOYMENT="${CONTROLPLANE_DEPLOYMENT:-aether-gateway-controlplane}"
DATAPLANE_DEPLOYMENT="${DATAPLANE_DEPLOYMENT:-aether-gateway-dataplane}"
CONTROLPLANE_SELECTOR="${CONTROLPLANE_SELECTOR:-app=aether-gateway-controlplane}"
DATAPLANE_SELECTOR="${DATAPLANE_SELECTOR:-app=aether-gateway-dataplane}"
CONTROLPLANE_CONFIGMAP="${CONTROLPLANE_CONFIGMAP:-aether-gateway-controlplane-config}"
DATAPLANE_CONFIGMAP="${DATAPLANE_CONFIGMAP:-aether-gateway-dataplane-config}"
CONTROLPLANE_ADMIN_SECRET="${CONTROLPLANE_ADMIN_SECRET:-aether-gateway-controlplane-admin-auth}"
DATAPLANE_ADMIN_SECRET="${DATAPLANE_ADMIN_SECRET:-aether-gateway-dataplane-admin-auth}"
ADMIN_TOKEN_FILE_PATH="${ADMIN_TOKEN_FILE_PATH:-/etc/aether-gateway/admin-auth/token}"
CONTROLPLANE_ADMIN_PORT="${CONTROLPLANE_ADMIN_PORT:-18081}"
DATAPLANE_ADMIN_PORT="${DATAPLANE_ADMIN_PORT:-19080}"
INITIAL_TOKEN="${INITIAL_TOKEN:-aeg-admin-old-token}"
ROTATED_TOKEN="${ROTATED_TOKEN:-aeg-admin-new-token}"
TOKEN_UPDATE_TIMEOUT_SEC="${TOKEN_UPDATE_TIMEOUT_SEC:-180}"
OUTPUT_DIR="${OUTPUT_DIR:-$(mktemp -d "${ROOT_DIR}/tmp/admin-token-rotation.XXXXXX")}"
KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-false}"

SUCCESS="false"
RESTORE_READY="false"
PORT_FORWARD_PIDS=()

log() {
  printf '[admin-token-rotation] %s\n' "$*"
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

strip_k8s_metadata_filter() {
  cat <<'JQ'
del(
  .metadata.uid,
  .metadata.resourceVersion,
  .metadata.generation,
  .metadata.creationTimestamp,
  .metadata.managedFields,
  .metadata.annotations."kubectl.kubernetes.io/last-applied-configuration"
)
JQ
}

is_tcp_port_listening() {
  local port="$1"

  ss -H -ltn "( sport = :${port} )" 2>/dev/null | grep -q .
}

find_free_tcp_port() {
  local start_port="$1"
  local port

  for port in $(seq "${start_port}" "$((start_port + 80))"); do
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

cleanup_port_forwards() {
  local pid

  for pid in "${PORT_FORWARD_PIDS[@]:-}"; do
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  done
  PORT_FORWARD_PIDS=()
}

debug_dump() {
  printf '\n[admin-token-rotation] debug: deployments\n' >&2
  k -n "${KUBE_NAMESPACE}" get deploy "${CONTROLPLANE_DEPLOYMENT}" "${DATAPLANE_DEPLOYMENT}" -o wide >&2 || true
  printf '\n[admin-token-rotation] debug: pods\n' >&2
  k -n "${KUBE_NAMESPACE}" get pod -l "${CONTROLPLANE_SELECTOR}" -o wide >&2 || true
  k -n "${KUBE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o wide >&2 || true
  if [[ -d "${OUTPUT_DIR}" ]]; then
    printf '\n[admin-token-rotation] debug: artifacts\n' >&2
    find "${OUTPUT_DIR}" -maxdepth 2 -type f -print >&2 || true
  fi
}

fail() {
  log "$1"
  debug_dump
  exit 1
}

cleanup() {
  local exit_code="$?"
  local restore_code=0

  set +e
  cleanup_port_forwards
  if [[ "${RESTORE_READY}" == "true" ]]; then
    restore_resources
    restore_code="$?"
  fi

  if [[ "${SUCCESS}" == "true" || "${KEEP_ARTIFACTS}" != "true" ]]; then
    rm -rf "${OUTPUT_DIR}" >/dev/null 2>&1 || true
  else
    log "artifacts kept at ${OUTPUT_DIR}"
  fi

  if [[ "${exit_code}" -eq 0 && "${restore_code}" -ne 0 ]]; then
    exit "${restore_code}"
  fi
  exit "${exit_code}"
}
trap cleanup EXIT

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
    SKIP_BUILD="${SKIP_BUILD:-true}" SKIP_SMOKE=true ./tests/e2e/run-kind.sh
  )
}

save_resource() {
  local kind="$1"
  local name="$2"
  local output_prefix="$3"

  if k -n "${KUBE_NAMESPACE}" get "${kind}" "${name}" -o json >"${output_prefix}.raw.json" 2>/dev/null; then
    jq "$(strip_k8s_metadata_filter)" "${output_prefix}.raw.json" >"${output_prefix}.json"
    printf 'true\n' >"${output_prefix}.exists"
  else
    printf 'false\n' >"${output_prefix}.exists"
  fi
}

save_original_resources() {
  mkdir -p "${OUTPUT_DIR}/original"
  save_resource configmap "${CONTROLPLANE_CONFIGMAP}" "${OUTPUT_DIR}/original/controlplane-configmap"
  save_resource configmap "${DATAPLANE_CONFIGMAP}" "${OUTPUT_DIR}/original/dataplane-configmap"
  save_resource secret "${CONTROLPLANE_ADMIN_SECRET}" "${OUTPUT_DIR}/original/controlplane-admin-secret"
  save_resource secret "${DATAPLANE_ADMIN_SECRET}" "${OUTPUT_DIR}/original/dataplane-admin-secret"
  RESTORE_READY="true"
}

restore_resource() {
  local kind="$1"
  local name="$2"
  local output_prefix="$3"

  if [[ "$(cat "${output_prefix}.exists")" == "true" ]]; then
    k apply -f "${output_prefix}.json" >/dev/null
  else
    k -n "${KUBE_NAMESPACE}" delete "${kind}" "${name}" --ignore-not-found >/dev/null
  fi
}

restore_resources() {
  log "restoring admin auth config and secrets"
  restore_resource configmap "${CONTROLPLANE_CONFIGMAP}" "${OUTPUT_DIR}/original/controlplane-configmap" || return 1
  restore_resource configmap "${DATAPLANE_CONFIGMAP}" "${OUTPUT_DIR}/original/dataplane-configmap" || return 1
  restore_resource secret "${CONTROLPLANE_ADMIN_SECRET}" "${OUTPUT_DIR}/original/controlplane-admin-secret" || return 1
  restore_resource secret "${DATAPLANE_ADMIN_SECRET}" "${OUTPUT_DIR}/original/dataplane-admin-secret" || return 1
  restart_and_wait_deployment "${CONTROLPLANE_DEPLOYMENT}" "${CONTROLPLANE_SELECTOR}" || return 1
  restart_and_wait_deployment "${DATAPLANE_DEPLOYMENT}" "${DATAPLANE_SELECTOR}" || return 1
}

apply_token_secret() {
  local secret_name="$1"
  local token="$2"

  k -n "${KUBE_NAMESPACE}" create secret generic "${secret_name}" \
    --from-literal="token=${token}" \
    --dry-run=client \
    -o yaml \
    | k apply -f - >/dev/null
}

enable_configmap_token_file() {
  local configmap="$1"

  k -n "${KUBE_NAMESPACE}" get configmap "${configmap}" -o json \
    | jq --arg token_path "${ADMIN_TOKEN_FILE_PATH}" "$(strip_k8s_metadata_filter)"'
      | .data["config.yaml"] = (
          .data["config.yaml"]
          | gsub("bearerToken: \"[^\"]*\""; "bearerToken: \"\"")
          | gsub("bearerTokenFile: \"[^\"]*\""; "bearerTokenFile: \"" + $token_path + "\"")
        )
    ' \
    | k apply -f - >/dev/null
}

desired_replicas() {
  local deployment="$1"
  local replicas

  replicas="$(k -n "${KUBE_NAMESPACE}" get deployment "${deployment}" -o jsonpath='{.spec.replicas}')"
  if [[ -z "${replicas}" || "${replicas}" -lt 1 ]]; then
    printf '1\n'
  else
    printf '%s\n' "${replicas}"
  fi
}

ready_pods() {
  local selector="$1"

  k -n "${KUBE_NAMESPACE}" get pod -l "${selector}" -o json \
    | jq -r '
      .items[]
      | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
      | .metadata.name
    ' \
    | sort
}

wait_for_ready_pods() {
  local selector="$1"
  local minimum="$2"
  local count

  for _ in $(seq 1 120); do
    count="$(ready_pods "${selector}" | wc -l | tr -d ' ')"
    if [[ "${count}" -ge "${minimum}" ]]; then
      return
    fi
    sleep 2
  done

  k -n "${KUBE_NAMESPACE}" get pod -l "${selector}" -o wide >&2 || true
  fail "ready pod count for ${selector} did not reach ${minimum}"
}

restart_and_wait_deployment() {
  local deployment="$1"
  local selector="$2"
  local replicas

  replicas="$(desired_replicas "${deployment}")"
  log "restarting ${deployment}"
  k -n "${KUBE_NAMESPACE}" rollout restart deployment/"${deployment}" >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/"${deployment}" --timeout=300s >/dev/null
  wait_for_ready_pods "${selector}" "${replicas}"
}

capture_pod_identity() {
  local selector="$1"
  local output="$2"

  k -n "${KUBE_NAMESPACE}" get pod -l "${selector}" -o json \
    | jq '
      [
        .items[]
        | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
        | {
            name: .metadata.name,
            uid: .metadata.uid,
            restarts: ([.status.containerStatuses[]?.restartCount] | add // 0)
          }
      ]
      | sort_by(.name)
    ' >"${output}"
}

assert_pod_identity_unchanged() {
  local selector="$1"
  local before="$2"
  local after="${before}.after"

  capture_pod_identity "${selector}" "${after}"
  if ! diff -u "${before}" "${after}" >"${after}.diff"; then
    cat "${after}.diff" >&2
    fail "pod identity or restart count changed for ${selector} during token rotation"
  fi
}

start_pod_port_forward() {
  local pod="$1"
  local admin_port="$2"
  local local_port="$3"
  local log_file="$4"

  k -n "${KUBE_NAMESPACE}" port-forward "pod/${pod}" "${local_port}:${admin_port}" >"${log_file}" 2>&1 &
  PORT_FORWARD_PIDS+=("$!")
  if ! wait_for_http "http://127.0.0.1:${local_port}/livez"; then
    cat "${log_file}" >&2 || true
    return 1
  fi
}

admin_status() {
  local local_port="$1"
  local token="$2"
  local curl_args=(-sS -o /dev/null -w '%{http_code}')

  if [[ -n "${token}" ]]; then
    curl_args+=(-H "Authorization: Bearer ${token}")
  fi

  curl "${curl_args[@]}" "http://127.0.0.1:${local_port}/v1/summary" 2>/dev/null || printf '000'
}

check_component_auth_state_once() {
  local component="$1"
  local selector="$2"
  local admin_port="$3"
  local expected_initial_status="$4"
  local expected_rotated_status="$5"
  local port_start="$6"
  local pod
  local local_port
  local missing_status
  local initial_status
  local rotated_status

  while IFS= read -r pod; do
    [[ -n "${pod}" ]] || continue
    local_port="$(find_free_tcp_port "${port_start}")"
    if ! start_pod_port_forward "${pod}" "${admin_port}" "${local_port}" "${OUTPUT_DIR}/${component}-${pod}.port-forward.log"; then
      cleanup_port_forwards
      return 1
    fi

    missing_status="$(admin_status "${local_port}" "")"
    initial_status="$(admin_status "${local_port}" "${INITIAL_TOKEN}")"
    rotated_status="$(admin_status "${local_port}" "${ROTATED_TOKEN}")"
    cleanup_port_forwards

    if [[ "${missing_status}" != "401" \
      || "${initial_status}" != "${expected_initial_status}" \
      || "${rotated_status}" != "${expected_rotated_status}" ]]; then
      {
        printf 'component=%s pod=%s\n' "${component}" "${pod}"
        printf 'missing_status=%s expected=401\n' "${missing_status}"
        printf 'initial_status=%s expected=%s\n' "${initial_status}" "${expected_initial_status}"
        printf 'rotated_status=%s expected=%s\n' "${rotated_status}" "${expected_rotated_status}"
      } >"${OUTPUT_DIR}/${component}-${pod}.auth-state.txt"
      return 1
    fi
  done < <(ready_pods "${selector}")
}

wait_for_component_auth_state() {
  local component="$1"
  local selector="$2"
  local admin_port="$3"
  local expected_initial_status="$4"
  local expected_rotated_status="$5"
  local port_start="$6"
  local deadline

  deadline="$((SECONDS + TOKEN_UPDATE_TIMEOUT_SEC))"
  while (( SECONDS < deadline )); do
    if check_component_auth_state_once \
      "${component}" \
      "${selector}" \
      "${admin_port}" \
      "${expected_initial_status}" \
      "${expected_rotated_status}" \
      "${port_start}"; then
      return
    fi
    sleep 2
  done

  fail "${component} admin token state did not converge before timeout"
}

main() {
  require_command curl
  require_command diff
  require_command jq
  require_command kind
  require_command kubectl
  require_command ss

  ensure_kind_cluster
  save_original_resources

  log "enabling admin bearer token files"
  apply_token_secret "${CONTROLPLANE_ADMIN_SECRET}" "${INITIAL_TOKEN}"
  apply_token_secret "${DATAPLANE_ADMIN_SECRET}" "${INITIAL_TOKEN}"
  enable_configmap_token_file "${CONTROLPLANE_CONFIGMAP}"
  enable_configmap_token_file "${DATAPLANE_CONFIGMAP}"
  restart_and_wait_deployment "${CONTROLPLANE_DEPLOYMENT}" "${CONTROLPLANE_SELECTOR}"
  restart_and_wait_deployment "${DATAPLANE_DEPLOYMENT}" "${DATAPLANE_SELECTOR}"

  log "verifying initial admin token state"
  wait_for_component_auth_state controlplane "${CONTROLPLANE_SELECTOR}" "${CONTROLPLANE_ADMIN_PORT}" 200 401 28081
  wait_for_component_auth_state dataplane "${DATAPLANE_SELECTOR}" "${DATAPLANE_ADMIN_PORT}" 200 401 29080

  capture_pod_identity "${CONTROLPLANE_SELECTOR}" "${OUTPUT_DIR}/controlplane-pods-before.json"
  capture_pod_identity "${DATAPLANE_SELECTOR}" "${OUTPUT_DIR}/dataplane-pods-before.json"

  log "rotating admin token Secrets without restarting pods"
  apply_token_secret "${CONTROLPLANE_ADMIN_SECRET}" "${ROTATED_TOKEN}"
  apply_token_secret "${DATAPLANE_ADMIN_SECRET}" "${ROTATED_TOKEN}"

  wait_for_component_auth_state controlplane "${CONTROLPLANE_SELECTOR}" "${CONTROLPLANE_ADMIN_PORT}" 401 200 28081
  wait_for_component_auth_state dataplane "${DATAPLANE_SELECTOR}" "${DATAPLANE_ADMIN_PORT}" 401 200 29080
  assert_pod_identity_unchanged "${CONTROLPLANE_SELECTOR}" "${OUTPUT_DIR}/controlplane-pods-before.json"
  assert_pod_identity_unchanged "${DATAPLANE_SELECTOR}" "${OUTPUT_DIR}/dataplane-pods-before.json"

  SUCCESS="true"
  log "admin token rotation validation passed"
}

main "$@"
