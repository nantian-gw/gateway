#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-aether-gateway}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-false}"
CONTROLPLANE_DEPLOYMENT="${CONTROLPLANE_DEPLOYMENT:-aether-gateway-controlplane}"
DATAPLANE_DEPLOYMENT="${DATAPLANE_DEPLOYMENT:-aether-gateway-dataplane}"
CONTROLPLANE_SELECTOR="${CONTROLPLANE_SELECTOR:-app=aether-gateway-controlplane}"
DATAPLANE_SELECTOR="${DATAPLANE_SELECTOR:-app=aether-gateway-dataplane}"
CONTROLPLANE_CONFIGMAP="${CONTROLPLANE_CONFIGMAP:-aether-gateway-controlplane-config}"
DATAPLANE_CONFIGMAP="${DATAPLANE_CONFIGMAP:-aether-gateway-dataplane-config}"
CONTROLPLANE_TLS_SECRET="${CONTROLPLANE_TLS_SECRET:-aether-gateway-controlplane-grpc-tls}"
DATAPLANE_TLS_SECRET="${DATAPLANE_TLS_SECRET:-aether-gateway-dataplane-xds-tls}"
CONTROLPLANE_TLS_DIR="${CONTROLPLANE_TLS_DIR:-/etc/aether-gateway/grpc-tls}"
DATAPLANE_TLS_DIR="${DATAPLANE_TLS_DIR:-/etc/aether-gateway/xds-tls}"
CONTROLPLANE_GRPC_SERVICE_DNS="${CONTROLPLANE_GRPC_SERVICE_DNS:-aether-gateway-controlplane-grpc.aether-gateway.svc.cluster.local}"
CONTROLPLANE_GRPC_ADDR="${CONTROLPLANE_GRPC_ADDR:-https://${CONTROLPLANE_GRPC_SERVICE_DNS}:18080}"
DATAPLANE_ADMIN_PORT="${DATAPLANE_ADMIN_PORT:-19080}"
INITIAL_RECONNECT_BACKOFF_MS="${INITIAL_RECONNECT_BACKOFF_MS:-500}"
ROTATED_RECONNECT_BACKOFF_MS="${ROTATED_RECONNECT_BACKOFF_MS:-700}"
BAD_RECONNECT_BACKOFF_MS="${BAD_RECONNECT_BACKOFF_MS:-300}"
XDS_ROTATION_TIMEOUT_SEC="${XDS_ROTATION_TIMEOUT_SEC:-240}"
OUTPUT_DIR="${OUTPUT_DIR:-$(mktemp -d "${ROOT_DIR}/tmp/xds-mtls-rotation.XXXXXX")}"

SUCCESS="false"
RESTORE_READY="false"
PORT_FORWARD_PIDS=()

log() {
  printf '[xds-mtls-rotation] %s\n' "$*"
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
  .metadata.annotations."kubectl.kubernetes.io/last-applied-configuration",
  .status
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
  set +e
  printf '\n[xds-mtls-rotation] debug: deployments\n' >&2
  k -n "${KUBE_NAMESPACE}" get deploy "${CONTROLPLANE_DEPLOYMENT}" "${DATAPLANE_DEPLOYMENT}" -o wide >&2 || true
  printf '\n[xds-mtls-rotation] debug: pods\n' >&2
  k -n "${KUBE_NAMESPACE}" get pod -l "${CONTROLPLANE_SELECTOR}" -o wide >&2 || true
  k -n "${KUBE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o wide >&2 || true
  printf '\n[xds-mtls-rotation] debug: dataplane summaries\n' >&2
  dump_dataplane_summaries >&2 || true
  if [[ -d "${OUTPUT_DIR}" ]]; then
    printf '\n[xds-mtls-rotation] debug: artifacts\n' >&2
    find "${OUTPUT_DIR}" -maxdepth 2 -type f -print >&2 || true
  fi
  set -e
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
  save_resource secret "${CONTROLPLANE_TLS_SECRET}" "${OUTPUT_DIR}/original/controlplane-tls-secret"
  save_resource secret "${DATAPLANE_TLS_SECRET}" "${OUTPUT_DIR}/original/dataplane-tls-secret"
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
  log "restoring xDS mTLS config and secrets"
  restore_resource configmap "${CONTROLPLANE_CONFIGMAP}" "${OUTPUT_DIR}/original/controlplane-configmap" || return 1
  restore_resource configmap "${DATAPLANE_CONFIGMAP}" "${OUTPUT_DIR}/original/dataplane-configmap" || return 1
  restore_resource secret "${CONTROLPLANE_TLS_SECRET}" "${OUTPUT_DIR}/original/controlplane-tls-secret" || return 1
  restore_resource secret "${DATAPLANE_TLS_SECRET}" "${OUTPUT_DIR}/original/dataplane-tls-secret" || return 1
  restart_and_wait_deployment "${CONTROLPLANE_DEPLOYMENT}" "${CONTROLPLANE_SELECTOR}" || return 1
  restart_and_wait_deployment "${DATAPLANE_DEPLOYMENT}" "${DATAPLANE_SELECTOR}" || return 1
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

  for _ in $(seq 1 90); do
    count="$(ready_pods "${selector}" | sed '/^$/d' | wc -l | tr -d ' ')"
    if [[ "${count}" -ge "${minimum}" ]]; then
      return
    fi
    sleep 2
  done

  fail "timed out waiting for ${minimum} ready pods matching ${selector}"
}

restart_and_wait_deployment() {
  local deployment="$1"
  local selector="$2"
  local replicas

  replicas="$(desired_replicas "${deployment}")"
  log "rolling out deployment ${deployment}"
  k -n "${KUBE_NAMESPACE}" rollout restart deployment/"${deployment}" >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/"${deployment}" --timeout=240s
  wait_for_ready_pods "${selector}" "${replicas}"
}

capture_pod_identity() {
  local selector="$1"
  local output="$2"

  k -n "${KUBE_NAMESPACE}" get pod -l "${selector}" -o json \
    | jq -S '
      [
        .items[]
        | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
        | {
            name: .metadata.name,
            uid: .metadata.uid,
            restartCounts: (
              [.status.containerStatuses[]? | {name: .name, restartCount: .restartCount}]
              | sort_by(.name)
            )
          }
      ]
      | sort_by(.name)
    ' >"${output}"
}

assert_pod_identity_unchanged() {
  local selector="$1"
  local before="$2"
  local current="${before}.current"

  capture_pod_identity "${selector}" "${current}"
  if ! diff -u "${before}" "${current}"; then
    fail "pods matching ${selector} restarted or were replaced during xDS mTLS rotation"
  fi
}

generate_ca() {
  local prefix="$1"

  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "${OUTPUT_DIR}/${prefix}-ca.key" \
    -out "${OUTPUT_DIR}/${prefix}-ca.crt" \
    -days 2 \
    -subj "/CN=aether-gateway-${prefix}-xds-ca" \
    -addext "basicConstraints=critical,CA:TRUE" \
    -addext "keyUsage=critical,keyCertSign,cRLSign" >/dev/null 2>&1
}

generate_leaf_cert() {
  local prefix="$1"
  local role="$2"
  local common_name="$3"
  local extended_key_usage="$4"
  local san="$5"
  local ext_file="${OUTPUT_DIR}/${prefix}-${role}.ext"

  openssl req -nodes -newkey rsa:2048 \
    -keyout "${OUTPUT_DIR}/${prefix}-${role}.key" \
    -out "${OUTPUT_DIR}/${prefix}-${role}.csr" \
    -subj "/CN=${common_name}" >/dev/null 2>&1

  {
    printf 'basicConstraints=critical,CA:FALSE\n'
    printf 'keyUsage=critical,digitalSignature,keyEncipherment\n'
    printf 'extendedKeyUsage=%s\n' "${extended_key_usage}"
    if [[ -n "${san}" ]]; then
      printf 'subjectAltName=%s\n' "${san}"
    fi
  } >"${ext_file}"

  openssl x509 -req \
    -in "${OUTPUT_DIR}/${prefix}-${role}.csr" \
    -CA "${OUTPUT_DIR}/${prefix}-ca.crt" \
    -CAkey "${OUTPUT_DIR}/${prefix}-ca.key" \
    -CAcreateserial \
    -out "${OUTPUT_DIR}/${prefix}-${role}.crt" \
    -days 2 \
    -sha256 \
    -extfile "${ext_file}" >/dev/null 2>&1
}

generate_cert_set() {
  local prefix="$1"

  generate_ca "${prefix}"
  generate_leaf_cert "${prefix}" "server" "aether-gateway-controlplane-${prefix}" "serverAuth" "DNS:${CONTROLPLANE_GRPC_SERVICE_DNS}"
  generate_leaf_cert "${prefix}" "client" "aether-gateway-dataplane-${prefix}" "clientAuth" ""
}

create_cert_sets() {
  generate_cert_set initial
  generate_cert_set rotated
}

apply_tls_secrets() {
  local prefix="$1"

  k -n "${KUBE_NAMESPACE}" create secret generic "${CONTROLPLANE_TLS_SECRET}" \
    --from-file=tls.crt="${OUTPUT_DIR}/${prefix}-server.crt" \
    --from-file=tls.key="${OUTPUT_DIR}/${prefix}-server.key" \
    --from-file=ca.crt="${OUTPUT_DIR}/${prefix}-ca.crt" \
    --dry-run=client \
    -o yaml \
    | k apply -f - >/dev/null

  k -n "${KUBE_NAMESPACE}" create secret generic "${DATAPLANE_TLS_SECRET}" \
    --from-file=ca.crt="${OUTPUT_DIR}/${prefix}-ca.crt" \
    --from-file=tls.crt="${OUTPUT_DIR}/${prefix}-client.crt" \
    --from-file=tls.key="${OUTPUT_DIR}/${prefix}-client.key" \
    --dry-run=client \
    -o yaml \
    | k apply -f - >/dev/null
}

write_block_replacement() {
  local output="$1"
  shift

  printf '%s\n' "$@" >"${output}"
}

replace_top_level_block() {
  local input="$1"
  local output="$2"
  local block_name="$3"
  local replacement="$4"

  awk -v block="${block_name}" -v replacement_file="${replacement}" '
    BEGIN {
      while ((getline line < replacement_file) > 0) {
        repl = repl line "\n"
      }
      in_block = 0
      wrote = 0
    }
    $0 ~ "^" block ":[[:space:]]*$" {
      printf "%s", repl
      in_block = 1
      wrote = 1
      next
    }
    in_block {
      if ($0 ~ /^[^[:space:]]/) {
        in_block = 0
      } else {
        next
      }
    }
    { print }
    END {
      if (!wrote) {
        printf "%s", repl
      }
    }
  ' "${input}" >"${output}"
}

set_top_level_scalar() {
  local input="$1"
  local output="$2"
  local key="$3"
  local value="$4"

  awk -v key="${key}" -v value="${value}" '
    BEGIN { wrote = 0 }
    $0 ~ "^" key ":" {
      print key ": \"" value "\""
      wrote = 1
      next
    }
    { print }
    END {
      if (!wrote) {
        print key ": \"" value "\""
      }
    }
  ' "${input}" >"${output}"
}

render_controlplane_config() {
  local input="$1"
  local output="$2"
  local block_file="${OUTPUT_DIR}/grpcTLS.block.yaml"

  write_block_replacement "${block_file}" \
    "grpcTLS:" \
    "  enabled: true" \
    "  certPath: \"${CONTROLPLANE_TLS_DIR}/tls.crt\"" \
    "  keyPath: \"${CONTROLPLANE_TLS_DIR}/tls.key\"" \
    "  clientCAPath: \"${CONTROLPLANE_TLS_DIR}/ca.crt\"" \
    "  requireClientCert: true"

  replace_top_level_block "${input}" "${output}" "grpcTLS" "${block_file}"
}

render_dataplane_config() {
  local input="$1"
  local output="$2"
  local ca_path="$3"
  local reconnect_backoff_ms="$4"
  local scalar_output="${OUTPUT_DIR}/dataplane.scalar.yaml"
  local tls_output="${OUTPUT_DIR}/dataplane.tls.yaml"
  local tls_block="${OUTPUT_DIR}/xdsTls.block.yaml"
  local transport_block="${OUTPUT_DIR}/xdsTransport.block.yaml"

  set_top_level_scalar "${input}" "${scalar_output}" "controlPlaneAddr" "${CONTROLPLANE_GRPC_ADDR}"
  write_block_replacement "${tls_block}" \
    "xdsTls:" \
    "  enabled: true" \
    "  caPath: \"${ca_path}\"" \
    "  certPath: \"${DATAPLANE_TLS_DIR}/tls.crt\"" \
    "  keyPath: \"${DATAPLANE_TLS_DIR}/tls.key\"" \
    "  domainName: \"${CONTROLPLANE_GRPC_SERVICE_DNS}\""
  replace_top_level_block "${scalar_output}" "${tls_output}" "xdsTls" "${tls_block}"

  write_block_replacement "${transport_block}" \
    "xdsTransport:" \
    "  initialReconnectBackoffMs: ${reconnect_backoff_ms}" \
    "  maxReconnectBackoffMs: 1000" \
    "  connectTimeoutMs: 2000" \
    "  keepaliveIntervalMs: 10000" \
    "  keepaliveTimeoutMs: 5000" \
    "  applyTimeoutMs: 3000" \
    "  applyPollIntervalMs: 25" \
    "  staleStreamTimeoutMs: 30000" \
    "  snapshotFreshnessTimeoutMs: 75000"
  replace_top_level_block "${tls_output}" "${output}" "xdsTransport" "${transport_block}"
}

apply_configmap_from_file() {
  local configmap="$1"
  local config_path="$2"

  k -n "${KUBE_NAMESPACE}" create configmap "${configmap}" \
    --from-file=config.yaml="${config_path}" \
    --dry-run=client \
    -o yaml \
    | k apply -f - >/dev/null
}

backup_configmaps() {
  k -n "${KUBE_NAMESPACE}" get configmap "${CONTROLPLANE_CONFIGMAP}" -o jsonpath='{.data.config\.yaml}' \
    >"${OUTPUT_DIR}/controlplane-config.original.yaml"
  k -n "${KUBE_NAMESPACE}" get configmap "${DATAPLANE_CONFIGMAP}" -o jsonpath='{.data.config\.yaml}' \
    >"${OUTPUT_DIR}/dataplane-config.original.yaml"
}

enable_initial_mtls_config() {
  local controlplane_rendered="${OUTPUT_DIR}/controlplane-config.mtls.yaml"
  local dataplane_rendered="${OUTPUT_DIR}/dataplane-config.mtls.yaml"

  render_controlplane_config "${OUTPUT_DIR}/controlplane-config.original.yaml" "${controlplane_rendered}"
  render_dataplane_config \
    "${OUTPUT_DIR}/dataplane-config.original.yaml" \
    "${dataplane_rendered}" \
    "${DATAPLANE_TLS_DIR}/ca.crt" \
    "${INITIAL_RECONNECT_BACKOFF_MS}"

  apply_configmap_from_file "${CONTROLPLANE_CONFIGMAP}" "${controlplane_rendered}"
  apply_configmap_from_file "${DATAPLANE_CONFIGMAP}" "${dataplane_rendered}"
}

apply_bad_dataplane_xds_config() {
  local rendered="${OUTPUT_DIR}/dataplane-config.bad-xds-ca.yaml"

  render_dataplane_config \
    "${OUTPUT_DIR}/dataplane-config.original.yaml" \
    "${rendered}" \
    "${DATAPLANE_TLS_DIR}/missing-ca.crt" \
    "${BAD_RECONNECT_BACKOFF_MS}"
  apply_configmap_from_file "${DATAPLANE_CONFIGMAP}" "${rendered}"
}

apply_rotated_dataplane_xds_config() {
  local rendered="${OUTPUT_DIR}/dataplane-config.rotated-xds.yaml"

  render_dataplane_config \
    "${OUTPUT_DIR}/dataplane-config.original.yaml" \
    "${rendered}" \
    "${DATAPLANE_TLS_DIR}/ca.crt" \
    "${ROTATED_RECONNECT_BACKOFF_MS}"
  apply_configmap_from_file "${DATAPLANE_CONFIGMAP}" "${rendered}"
}

start_pod_admin_port_forward() {
  local pod="$1"
  local local_port="$2"
  local log_file="$3"

  k -n "${KUBE_NAMESPACE}" port-forward "pod/${pod}" "${local_port}:${DATAPLANE_ADMIN_PORT}" >"${log_file}" 2>&1 &
  PORT_FORWARD_PIDS+=("$!")
  if ! wait_for_http "http://127.0.0.1:${local_port}/livez"; then
    cat "${log_file}" >&2 || true
    return 1
  fi
}

dataplane_summary_for_pod() {
  local pod="$1"
  local local_port
  local log_file
  local status

  local_port="$(find_free_tcp_port 29080)"
  log_file="${OUTPUT_DIR}/${pod}.admin-port-forward.log"
  start_pod_admin_port_forward "${pod}" "${local_port}" "${log_file}"
  set +e
  curl -fsS "http://127.0.0.1:${local_port}/v1/summary"
  status="$?"
  set -e
  cleanup_port_forwards
  return "${status}"
}

dump_dataplane_summaries() {
  local pod

  while IFS= read -r pod; do
    [[ -n "${pod}" ]] || continue
    printf '\n--- %s ---\n' "${pod}"
    dataplane_summary_for_pod "${pod}" \
      | jq '{nodeId,ready,readinessReason,snapshotVersion,xdsStreamConnected,xdsConnectFailures,xdsLastConnectError,xdsLastSnapshotVersion,currentSnapshotStatus,currentSnapshotFallbackState}' \
      || true
  done < <(ready_pods "${DATAPLANE_SELECTOR}")
}

record_dataplane_xds_state() {
  local output="$1"
  local pod
  local summary

  : >"${output}"
  while IFS= read -r pod; do
    [[ -n "${pod}" ]] || continue
    summary="$(dataplane_summary_for_pod "${pod}")"
    jq -r --arg pod "${pod}" '
      [
        $pod,
        (.xdsConnectFailures | tostring),
        (.xdsStreamFailures | tostring),
        (.xdsLastSnapshotVersion // ""),
        (.snapshotVersion // "")
      ] | @tsv
    ' <<<"${summary}" >>"${output}"
  done < <(ready_pods "${DATAPLANE_SELECTOR}")
  sort -o "${output}" "${output}"
}

wait_for_dataplane_connected() {
  local deadline="$((SECONDS + XDS_ROTATION_TIMEOUT_SEC))"
  local expected_count
  local pod
  local summary
  local connected
  local ready
  local last_snapshot

  expected_count="$(desired_replicas "${DATAPLANE_DEPLOYMENT}")"
  while (( SECONDS < deadline )); do
    local all_connected="true"
    local seen_count=0
    while IFS= read -r pod; do
      [[ -n "${pod}" ]] || continue
      seen_count=$((seen_count + 1))
      summary="$(dataplane_summary_for_pod "${pod}" 2>/dev/null || true)"
      if [[ -z "${summary}" ]]; then
        all_connected="false"
        break
      fi
      connected="$(jq -r '.xdsStreamConnected' <<<"${summary}")"
      ready="$(jq -r '.ready' <<<"${summary}")"
      last_snapshot="$(jq -r '.xdsLastSnapshotVersion // ""' <<<"${summary}")"
      if [[ "${connected}" != "true" || "${ready}" != "true" || -z "${last_snapshot}" ]]; then
        all_connected="false"
        break
      fi
    done < <(ready_pods "${DATAPLANE_SELECTOR}")

    if [[ "${all_connected}" == "true" && "${seen_count}" -ge "${expected_count}" ]]; then
      return
    fi
    sleep 2
  done

  fail "dataplane xDS streams did not become connected before timeout"
}

wait_for_bad_xds_config_observed() {
  local before_file="$1"
  local deadline="$((SECONDS + XDS_ROTATION_TIMEOUT_SEC))"
  local pod before_connect_failures before_stream_failures before_last_snapshot before_snapshot
  local summary connected connect_failures last_snapshot current_status
  local observed_count
  local expected_count

  expected_count="$(wc -l <"${before_file}" | tr -d ' ')"
  while (( SECONDS < deadline )); do
    observed_count=0
    while IFS=$'\t' read -r pod before_connect_failures before_stream_failures before_last_snapshot before_snapshot; do
      [[ -n "${pod}" ]] || continue
      summary="$(dataplane_summary_for_pod "${pod}" 2>/dev/null || true)"
      if [[ -z "${summary}" ]]; then
        continue
      fi
      connected="$(jq -r '.xdsStreamConnected' <<<"${summary}")"
      connect_failures="$(jq -r '.xdsConnectFailures' <<<"${summary}")"
      last_snapshot="$(jq -r '.xdsLastSnapshotVersion // ""' <<<"${summary}")"
      current_status="$(jq -r '.currentSnapshotStatus // ""' <<<"${summary}")"
      if [[ "${connected}" == "false" \
        && "${connect_failures}" =~ ^[0-9]+$ \
        && "${before_connect_failures}" =~ ^[0-9]+$ \
        && "${connect_failures}" -gt "${before_connect_failures}" \
        && -n "${last_snapshot}" \
        && "${last_snapshot}" == "${before_last_snapshot}" \
        && "${current_status}" == "accepted" ]]; then
        observed_count=$((observed_count + 1))
      fi
    done <"${before_file}"

    if [[ "${observed_count}" -eq "${expected_count}" ]]; then
      return
    fi
    sleep 2
  done

  fail "dataplane pods did not report failed xDS reconnect with last-good snapshot preserved"
}

wait_for_pod_file_content() {
  local selector="$1"
  local container="$2"
  local remote_path="$3"
  local expected_file="$4"
  local expected_hash
  local deadline
  local pod
  local actual_hash

  expected_hash="$(sha256sum "${expected_file}" | awk '{print $1}')"
  deadline="$((SECONDS + XDS_ROTATION_TIMEOUT_SEC))"

  while (( SECONDS < deadline )); do
    local ready="true"
    local seen_count=0
    while IFS= read -r pod; do
      [[ -n "${pod}" ]] || continue
      seen_count=$((seen_count + 1))
      actual_hash="$(
        k -n "${KUBE_NAMESPACE}" exec "${pod}" -c "${container}" -- cat "${remote_path}" 2>/dev/null \
          | sha256sum \
          | awk '{print $1}'
      )"
      if [[ "${actual_hash}" != "${expected_hash}" ]]; then
        ready="false"
        break
      fi
    done < <(ready_pods "${selector}")

    if [[ "${ready}" == "true" && "${seen_count}" -gt 0 ]]; then
      return
    fi
    sleep 2
  done

  fail "mounted file ${remote_path} did not converge to ${expected_file} for pods matching ${selector}"
}

wait_for_rotated_secret_projection() {
  local spec
  local selector
  local container
  local remote_path
  local expected_file

  log "waiting for rotated TLS Secret projection in running pods"
  for spec in \
    "${CONTROLPLANE_SELECTOR}|controlplane|${CONTROLPLANE_TLS_DIR}/tls.crt|${OUTPUT_DIR}/rotated-server.crt" \
    "${CONTROLPLANE_SELECTOR}|controlplane|${CONTROLPLANE_TLS_DIR}/ca.crt|${OUTPUT_DIR}/rotated-ca.crt" \
    "${DATAPLANE_SELECTOR}|dataplane|${DATAPLANE_TLS_DIR}/ca.crt|${OUTPUT_DIR}/rotated-ca.crt" \
    "${DATAPLANE_SELECTOR}|dataplane|${DATAPLANE_TLS_DIR}/tls.crt|${OUTPUT_DIR}/rotated-client.crt"; do
    IFS='|' read -r selector container remote_path expected_file <<<"${spec}"
    wait_for_pod_file_content "${selector}" "${container}" "${remote_path}" "${expected_file}"
  done
}

main() {
  require_command awk
  require_command curl
  require_command diff
  require_command jq
  require_command kind
  require_command kubectl
  require_command openssl
  require_command sha256sum
  require_command ss

  mkdir -p "${OUTPUT_DIR}"
  ensure_kind_cluster
  save_original_resources
  backup_configmaps
  create_cert_sets

  log "enabling controlplane gRPC mTLS and dataplane xDS mTLS"
  apply_tls_secrets initial
  enable_initial_mtls_config
  restart_and_wait_deployment "${CONTROLPLANE_DEPLOYMENT}" "${CONTROLPLANE_SELECTOR}"
  restart_and_wait_deployment "${DATAPLANE_DEPLOYMENT}" "${DATAPLANE_SELECTOR}"
  wait_for_dataplane_connected

  capture_pod_identity "${CONTROLPLANE_SELECTOR}" "${OUTPUT_DIR}/controlplane-pods-before.json"
  capture_pod_identity "${DATAPLANE_SELECTOR}" "${OUTPUT_DIR}/dataplane-pods-before.json"
  record_dataplane_xds_state "${OUTPUT_DIR}/dataplane-xds-before.tsv"

  log "rotating xDS mTLS Secrets without restarting pods"
  apply_tls_secrets rotated
  wait_for_rotated_secret_projection
  assert_pod_identity_unchanged "${CONTROLPLANE_SELECTOR}" "${OUTPUT_DIR}/controlplane-pods-before.json"
  assert_pod_identity_unchanged "${DATAPLANE_SELECTOR}" "${OUTPUT_DIR}/dataplane-pods-before.json"

  log "forcing a failed xDS reconnect and checking last-good snapshot state"
  apply_bad_dataplane_xds_config
  wait_for_bad_xds_config_observed "${OUTPUT_DIR}/dataplane-xds-before.tsv"
  assert_pod_identity_unchanged "${CONTROLPLANE_SELECTOR}" "${OUTPUT_DIR}/controlplane-pods-before.json"
  assert_pod_identity_unchanged "${DATAPLANE_SELECTOR}" "${OUTPUT_DIR}/dataplane-pods-before.json"

  log "restoring valid xDS config and verifying reconnect with rotated mTLS material"
  apply_rotated_dataplane_xds_config
  wait_for_dataplane_connected
  assert_pod_identity_unchanged "${CONTROLPLANE_SELECTOR}" "${OUTPUT_DIR}/controlplane-pods-before.json"
  assert_pod_identity_unchanged "${DATAPLANE_SELECTOR}" "${OUTPUT_DIR}/dataplane-pods-before.json"

  SUCCESS="true"
  log "xDS mTLS rotation validation passed"
}

main "$@"
