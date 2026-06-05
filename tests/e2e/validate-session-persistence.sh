#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-gw}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
TEST_NAMESPACE="${TEST_NAMESPACE:-nantian-session-persistence}"
TEST_HOST="${TEST_HOST:-sticky.example.com}"
TEST_PATH="${TEST_PATH:-/sticky}"
SESSION_COOKIE_NAME="${SESSION_COOKIE_NAME:-sticky-backend}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
GATEWAY_ADDRESS="${GATEWAY_ADDRESS:-http://${TEST_HOST}:${GATEWAY_HOST_PORT}${TEST_PATH}}"
ADMIN_FORWARD_PORT="${ADMIN_FORWARD_PORT:-29080}"
BACKEND_IMAGE="${BACKEND_IMAGE:-localhost:5001/hashicorp/http-echo:1.0.0}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"
KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-false}"
DATAPLANE_NAMESPACE="${DATAPLANE_NAMESPACE:-nantian-gw}"
DATAPLANE_CONFIGMAP="${DATAPLANE_CONFIGMAP:-nantian-dataplane-config}"
DATAPLANE_DEPLOYMENT="${DATAPLANE_DEPLOYMENT:-nantian-dataplane}"
DATAPLANE_SELECTOR="${DATAPLANE_SELECTOR:-app=nantian-dataplane}"
DATAPLANE_HTTP_FORWARD_PORT="${DATAPLANE_HTTP_FORWARD_PORT:-80}"
SESSION_SECRET="${SESSION_SECRET:-0123456789abcdef0123456789abcdef}"
INITIAL_SESSION_SECRET="${INITIAL_SESSION_SECRET:-${SESSION_SECRET}}"
ROTATED_SESSION_SECRET="${ROTATED_SESSION_SECRET:-fedcba9876543210fedcba9876543210}"
SESSION_SECRET_NAME="${SESSION_SECRET_NAME:-nantian-dataplane-session-persistence-e2e}"
SESSION_SECRET_KEY="${SESSION_SECRET_KEY:-secret}"
SESSION_SECRET_VOLUME="${SESSION_SECRET_VOLUME:-session-persistence-rotation}"
SESSION_SECRET_MOUNT_DIR="${SESSION_SECRET_MOUNT_DIR:-/etc/nantian-gw/session-persistence-rotation}"
SESSION_SECRET_FILE_PATH="${SESSION_SECRET_FILE_PATH:-${SESSION_SECRET_MOUNT_DIR}/${SESSION_SECRET_KEY}}"
SESSION_SECRET_UPDATE_TIMEOUT_SEC="${SESSION_SECRET_UPDATE_TIMEOUT_SEC:-180}"

TMP_DIR=""
PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""
SUCCESS="false"
DATAPLANE_CONFIG_MODIFIED="false"
DATAPLANE_DEPLOYMENT_MODIFIED="false"
SESSION_SECRET_MODIFIED="false"

log() {
  printf '[session-persistence] %s\n' "$*"
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

save_resource() {
  local kind="$1"
  local name="$2"
  local output_prefix="$3"

  if k -n "${DATAPLANE_NAMESPACE}" get "${kind}" "${name}" -o json >"${output_prefix}.raw.json" 2>/dev/null; then
    jq "$(strip_k8s_metadata_filter)" "${output_prefix}.raw.json" >"${output_prefix}.json"
    printf 'true\n' >"${output_prefix}.exists"
  else
    printf 'false\n' >"${output_prefix}.exists"
  fi
}

restore_resource() {
  local kind="$1"
  local name="$2"
  local output_prefix="$3"

  if [[ "$(cat "${output_prefix}.exists")" == "true" ]]; then
    k apply -f "${output_prefix}.json" >/dev/null
  else
    k -n "${DATAPLANE_NAMESPACE}" delete "${kind}" "${name}" --ignore-not-found >/dev/null
  fi
}

kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

ensure_kind_cluster() {
  if kind_cluster_exists; then
    return
  fi
  if [[ "${ENSURE_KIND}" != "true" ]]; then
    log "kind cluster ${CLUSTER_NAME} does not exist; run ./tests/e2e/run-kind.sh first or rerun with ENSURE_KIND=true"
    exit 1
  fi

  log "bootstrapping kind cluster via tests/e2e/run-kind.sh"
  (
    cd "${ROOT_DIR}"
    SKIP_BUILD="${SKIP_BUILD:-true}" ./tests/e2e/run-kind.sh
  )
}

cleanup_namespace() {
  if ! k get namespace "${TEST_NAMESPACE}" >/dev/null 2>&1; then
    return
  fi

  log "cleaning namespace ${TEST_NAMESPACE}"
  k delete namespace "${TEST_NAMESPACE}" --wait=false >/dev/null 2>&1 || true
  for _ in $(seq 1 60); do
    if ! k get namespace "${TEST_NAMESPACE}" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  log "namespace ${TEST_NAMESPACE} is still terminating"
  exit 1
}

port_listening() {
  local port="$1"

  ss -H -ltn "( sport = :${port} )" 2>/dev/null | grep -q .
}

find_free_tcp_port() {
  local start_port="$1"
  local port

  for port in $(seq "${start_port}" "$((start_port + 80))"); do
    if ! port_listening "${port}"; then
      printf '%s\n' "${port}"
      return
    fi
  done

  log "failed to find a free TCP port starting at ${start_port}"
  exit 1
}

pick_admin_forward_port() {
  local candidate="${ADMIN_FORWARD_PORT}"

  while port_listening "${candidate}"; do
    candidate=$((candidate + 1))
  done

  ADMIN_FORWARD_PORT="${candidate}"
}

start_admin_port_forward() {
  pick_admin_forward_port
  PORT_FORWARD_LOG="${TMP_DIR}/port-forward.log"
  k -n "${DATAPLANE_NAMESPACE}" port-forward service/nantian-dataplane-admin "${ADMIN_FORWARD_PORT}:19080" \
    >"${PORT_FORWARD_LOG}" 2>&1 &
  PORT_FORWARD_PID="$!"

  for _ in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/livez" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  log "timed out waiting for dataplane admin port-forward"
  if [[ -f "${PORT_FORWARD_LOG}" ]]; then
    cat "${PORT_FORWARD_LOG}" >&2
  fi
  exit 1
}

stop_admin_port_forward() {
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    kill "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  fi
}

summary_json() {
  curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/summary"
}

backup_dataplane_config() {
  k -n "${DATAPLANE_NAMESPACE}" get configmap "${DATAPLANE_CONFIGMAP}" -o jsonpath='{.data.config\.yaml}' \
    >"${TMP_DIR}/dataplane-config.original.yaml"
}

backup_session_secret() {
  save_resource secret "${SESSION_SECRET_NAME}" "${TMP_DIR}/session-secret.original"
}

apply_session_secret() {
  local secret="$1"

  k -n "${DATAPLANE_NAMESPACE}" create secret generic "${SESSION_SECRET_NAME}" \
    --from-literal="${SESSION_SECRET_KEY}=${secret}" \
    --dry-run=client \
    -o yaml | k apply -f - >/dev/null
  SESSION_SECRET_MODIFIED="true"
}

patch_dataplane_deployment_with_session_secret() {
  k -n "${DATAPLANE_NAMESPACE}" patch deployment "${DATAPLANE_DEPLOYMENT}" \
    --type=strategic \
    -p "$(cat <<EOF
spec:
  template:
    spec:
      containers:
        - name: dataplane
          volumeMounts:
            - name: ${SESSION_SECRET_VOLUME}
              mountPath: ${SESSION_SECRET_MOUNT_DIR}
              readOnly: true
      volumes:
        - name: ${SESSION_SECRET_VOLUME}
          secret:
            secretName: ${SESSION_SECRET_NAME}
            optional: false
EOF
)" >/dev/null
  DATAPLANE_DEPLOYMENT_MODIFIED="true"
}

remove_dataplane_session_secret_volume() {
  local patch

  patch="$(
    k -n "${DATAPLANE_NAMESPACE}" get deployment "${DATAPLANE_DEPLOYMENT}" -o json \
      | jq -c --arg volume "${SESSION_SECRET_VOLUME}" '
        [
          (
            .spec.template.spec.containers
            | to_entries[]
            | select(.value.name == "dataplane")
            | .key as $container_index
            | (.value.volumeMounts // [])
            | to_entries[]
            | select(.value.name == $volume)
            | {
                op: "remove",
                path: ("/spec/template/spec/containers/\($container_index)/volumeMounts/\(.key)")
              }
          ),
          (
            (.spec.template.spec.volumes // [])
            | to_entries[]
            | select(.value.name == $volume)
            | {
                op: "remove",
                path: ("/spec/template/spec/volumes/\(.key)")
              }
          )
        ]
      '
  )"

  if [[ "${patch}" != "[]" ]]; then
    k -n "${DATAPLANE_NAMESPACE}" patch deployment "${DATAPLANE_DEPLOYMENT}" \
      --type=json \
      -p "${patch}" >/dev/null
  fi
}

render_dataplane_config_with_session_secret_file() {
  local input_path="$1"
  local output_path="$2"

  if ! grep -q '^sessionPersistence:' "${input_path}"; then
    cat "${input_path}" >"${output_path}"
    cat >>"${output_path}" <<EOF
sessionPersistence:
  secretKey: ""
  secretKeyFile: "${SESSION_SECRET_FILE_PATH}"
EOF
    return
  fi

  awk -v secret_file="${SESSION_SECRET_FILE_PATH}" '
    function emit_block() {
      print "sessionPersistence:"
      print "  secretKey: \"\""
      print "  secretKeyFile: \"" secret_file "\""
      wrote = 1
    }
    BEGIN {
      in_block = 0
      wrote = 0
    }
    /^sessionPersistence:[[:space:]]*$/ {
      emit_block()
      in_block = 1
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
        emit_block()
      }
    }
  ' "${input_path}" >"${output_path}"
}

apply_dataplane_config() {
  local config_path="$1"

  k -n "${DATAPLANE_NAMESPACE}" create configmap "${DATAPLANE_CONFIGMAP}" \
    --from-file=config.yaml="${config_path}" \
    --dry-run=client \
    -o yaml | k apply -f - >/dev/null
}

rollout_dataplane() {
  log "rolling out dataplane deployment ${DATAPLANE_DEPLOYMENT}"
  k -n "${DATAPLANE_NAMESPACE}" rollout restart deployment/"${DATAPLANE_DEPLOYMENT}" >/dev/null
  k -n "${DATAPLANE_NAMESPACE}" rollout status deployment/"${DATAPLANE_DEPLOYMENT}" --timeout=180s
}

configure_stable_session_secret() {
  local rendered_config="${TMP_DIR}/dataplane-config.session.yaml"

  backup_dataplane_config
  backup_session_secret
  apply_session_secret "${INITIAL_SESSION_SECRET}"
  patch_dataplane_deployment_with_session_secret
  render_dataplane_config_with_session_secret_file \
    "${TMP_DIR}/dataplane-config.original.yaml" \
    "${rendered_config}"
  apply_dataplane_config "${rendered_config}"
  DATAPLANE_CONFIG_MODIFIED="true"
  rollout_dataplane
}

restore_dataplane_config() {
  local needs_rollout="false"

  if [[ "${DATAPLANE_CONFIG_MODIFIED}" == "true" && -f "${TMP_DIR}/dataplane-config.original.yaml" ]]; then
    log "restoring dataplane session persistence configuration"
    apply_dataplane_config "${TMP_DIR}/dataplane-config.original.yaml"
    needs_rollout="true"
  fi

  if [[ "${DATAPLANE_DEPLOYMENT_MODIFIED}" == "true" ]]; then
    remove_dataplane_session_secret_volume
    needs_rollout="true"
  fi

  if [[ "${needs_rollout}" == "true" ]]; then
    rollout_dataplane
  fi

  if [[ "${SESSION_SECRET_MODIFIED}" == "true" && -f "${TMP_DIR}/session-secret.original.exists" ]]; then
    restore_resource secret "${SESSION_SECRET_NAME}" "${TMP_DIR}/session-secret.original"
  fi
}

wait_for_session_persistence_activation() {
  local summary
  local active
  local backend_policies
  local configured

  for _ in $(seq 1 60); do
    summary="$(summary_json 2>/dev/null || true)"
    if [[ -z "${summary}" ]]; then
      sleep 1
      continue
    fi

    active="$(jq -r '.sessionPersistenceActive' <<<"${summary}")"
    backend_policies="$(jq -r '.sessionPersistenceBackendPolicyCount' <<<"${summary}")"
    configured="$(jq -r '.sessionPersistenceConfigured' <<<"${summary}")"
    if [[ "${active}" == "true" && "${configured}" == "true" && "${backend_policies}" =~ ^[0-9]+$ && "${backend_policies}" -ge 1 ]]; then
      log "dataplane summary reports active backend session persistence"
      jq '{
        nodeId,
        snapshotVersion,
        sessionPersistenceConfigured,
        sessionPersistenceUsesEphemeralSecret,
        sessionPersistenceActive,
        sessionPersistenceBackendPolicyCount
      }' <<<"${summary}"
      return
    fi
    sleep 1
  done

  log "session persistence was not reported as active by the dataplane"
  summary_json | jq '.'
  exit 1
}

dump_debug_state() {
  set +e
  printf '\n[session-persistence] debug: gateway\n' >&2
  k -n "${TEST_NAMESPACE}" get gateway sticky-edge -o yaml >&2
  printf '\n[session-persistence] debug: httproute\n' >&2
  k -n "${TEST_NAMESPACE}" get httproute sticky-route -o yaml >&2
  printf '\n[session-persistence] debug: backendlbpolicy\n' >&2
  k -n "${TEST_NAMESPACE}" get backendlbpolicy sticky-policy -o yaml >&2
  printf '\n[session-persistence] debug: endpoints\n' >&2
  k -n "${TEST_NAMESPACE}" get endpointslices -o wide >&2
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    printf '\n[session-persistence] debug: dataplane summary\n' >&2
    summary_json \
      | jq '{
          nodeId,
          snapshotVersion,
          sessionPersistenceConfigured,
          sessionPersistenceUsesEphemeralSecret,
          sessionPersistenceActive,
          sessionPersistenceBackendPolicyCount,
          warningCategories
        }' >&2
  fi
  if [[ -f "${PORT_FORWARD_LOG}" ]]; then
    printf '\n[session-persistence] debug: port-forward log\n' >&2
    cat "${PORT_FORWARD_LOG}" >&2
  fi
  set -e
}

cleanup() {
  local exit_code="$?"

  if [[ "${SUCCESS}" != "true" ]]; then
    dump_debug_state
  fi
  if declare -F stop_all_pod_http_port_forwards >/dev/null 2>&1; then
    stop_all_pod_http_port_forwards
  fi
  stop_admin_port_forward
  restore_dataplane_config

  if [[ "${KEEP_RESOURCES}" != "true" ]]; then
    cleanup_namespace
  else
    log "keeping namespace ${TEST_NAMESPACE}"
  fi

  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" && ( "${SUCCESS}" == "true" || "${KEEP_ARTIFACTS}" != "true" ) ]]; then
    rm -rf "${TMP_DIR}"
  elif [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    log "artifacts kept at ${TMP_DIR}"
  fi

  exit "${exit_code}"
}

apply_test_resources() {
  k create namespace "${TEST_NAMESPACE}" >/dev/null
  k apply -f - >/dev/null <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: v1
kind: Service
metadata:
  name: sticky-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: sticky-echo
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sticky-echo-a
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sticky-echo
      variant: a
  template:
    metadata:
      labels:
        app: sticky-echo
        variant: a
    spec:
      containers:
        - name: echo
          image: ${BACKEND_IMAGE}
          args:
            - "-listen=:8080"
            - "-text=backend-a"
          ports:
            - containerPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sticky-echo-b
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sticky-echo
      variant: b
  template:
    metadata:
      labels:
        app: sticky-echo
        variant: b
    spec:
      containers:
        - name: echo
          image: ${BACKEND_IMAGE}
          args:
            - "-listen=:8080"
            - "-text=backend-b"
          ports:
            - containerPort: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: sticky-edge
  namespace: ${TEST_NAMESPACE}
spec:
  gatewayClassName: nantian
  listeners:
    - name: http
      hostname: ${TEST_HOST}
      protocol: HTTP
      port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: sticky-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: sticky-edge
      sectionName: http
  hostnames:
    - ${TEST_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: ${TEST_PATH}
      backendRefs:
        - name: sticky-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: BackendLBPolicy
metadata:
  name: sticky-policy
  namespace: ${TEST_NAMESPACE}
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: sticky-backend
  sessionPersistence:
    sessionName: sticky-backend
    type: Cookie
    absoluteTimeout: 10m
    idleTimeout: 1m
    cookieConfig:
      lifetimeType: Permanent
EOF
}

wait_for_backends() {
  log "waiting for sticky backend deployments"
  k -n "${TEST_NAMESPACE}" rollout status deployment/sticky-echo-a --timeout=180s
  k -n "${TEST_NAMESPACE}" rollout status deployment/sticky-echo-b --timeout=180s
}

source "${ROOT_DIR}/tests/e2e/validate-session-persistence-rotation-lib.sh"

request_gateway() {
  local headers_file="$1"
  local body_file="$2"
  local cookie_jar="$3"

  curl -sS \
    --http1.1 \
    --noproxy '*' \
    --resolve "${TEST_HOST}:${GATEWAY_HOST_PORT}:127.0.0.1" \
    --connect-timeout 2 \
    --max-time 10 \
    -D "${headers_file}" \
    -o "${body_file}" \
    -c "${cookie_jar}" \
    -b "${cookie_jar}" \
    -H 'Connection: close' \
    -w '%{http_code}' \
    "${GATEWAY_ADDRESS}"
}

wait_for_initial_response() {
  local headers_file="$1"
  local body_file="$2"
  local cookie_jar="$3"
  local status
  local body

  for _ in $(seq 1 45); do
    : >"${headers_file}"
    : >"${body_file}"
    status="$(request_gateway "${headers_file}" "${body_file}" "${cookie_jar}" || true)"
    body="$(tr -d '\r\n' <"${body_file}")"
    if [[ "${status}" == "200" && ( "${body}" == "backend-a" || "${body}" == "backend-b" ) ]]; then
      printf '%s\n' "${body}"
      return
    fi
    sleep 2
  done

  log "did not receive a successful response from ${GATEWAY_ADDRESS}"
  exit 1
}

verify_sticky_session() {
  local cookie_jar="${TMP_DIR}/cookies.txt"
  local headers_file="${TMP_DIR}/headers.txt"
  local body_file="${TMP_DIR}/body.txt"
  local expected
  local observed
  local status

  expected="$(wait_for_initial_response "${headers_file}" "${body_file}" "${cookie_jar}")"
  if ! grep -qi "^set-cookie: ${SESSION_COOKIE_NAME}=" "${headers_file}"; then
    log "first response did not set the expected sticky cookie"
    cat "${headers_file}" >&2
    exit 1
  fi

  log "first sticky response selected ${expected}"
  for attempt in $(seq 1 6); do
    : >"${headers_file}"
    : >"${body_file}"
    status="$(request_gateway "${headers_file}" "${body_file}" "${cookie_jar}")"
    observed="$(tr -d '\r\n' <"${body_file}")"
    if [[ "${status}" != "200" ]]; then
      log "sticky request ${attempt} returned status ${status}"
      cat "${headers_file}" >&2
      exit 1
    fi
    if [[ "${observed}" != "${expected}" ]]; then
      log "sticky request ${attempt} selected ${observed}, expected ${expected}"
      exit 1
    fi
  done

  log "sticky session remained pinned to ${expected}"
}

main() {
  require_command curl
  require_command diff
  require_command kind
  require_command jq
  require_command kubectl
  require_command openssl
  require_command ss

  ensure_kind_cluster
  TMP_DIR="$(mktemp -d "${ROOT_DIR}/tmp/session-persistence.XXXXXX")"
  trap cleanup EXIT

  cleanup_namespace
  apply_test_resources
  wait_for_backends
  configure_stable_session_secret
  start_admin_port_forward
  wait_for_session_persistence_activation
  verify_sticky_session
  verify_session_secret_rotation

  SUCCESS="true"
  log "session persistence validation passed"
}

main "$@"
