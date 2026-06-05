#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${ROOT_DIR}/scripts/lib/common.sh"
# shellcheck source=scripts/lib/performance-common.sh
source "${ROOT_DIR}/scripts/lib/performance-common.sh"

source "${ROOT_DIR}/scripts/lib/kind-evidence.sh"
source "${ROOT_DIR}/scripts/lib/kind-image-sync.sh"
RUN_ID="${RUN_ID:-$(date +%Y-%m-%d-%H%M%S)-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)-kind-a4}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/reports/performance/runs/${RUN_ID}}"
SLO_OUTPUT="${OUTPUT_DIR}/slo-gate.json"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-aether-gateway}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
KIND_IMAGE_SYNC_LOCAL_REGISTRY="${KIND_IMAGE_SYNC_LOCAL_REGISTRY:-${LOCAL_REGISTRY_PUSH_HOST}}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
TCP_GATEWAY_HOST_PORT="${TCP_GATEWAY_HOST_PORT:-19000}"
UDP_GATEWAY_HOST_PORT="${UDP_GATEWAY_HOST_PORT:-5300}"
UDP_BACKEND_TIMEOUT_HOST_PORT="${UDP_BACKEND_TIMEOUT_HOST_PORT:-5302}"
UDP_BACKEND_TIMEOUT_LISTENER_PORT="${UDP_BACKEND_TIMEOUT_LISTENER_PORT:-5302}"
HTTP_HOST="${HTTP_HOST:-example.com}"
GRPC_AUTHORITY="${GRPC_AUTHORITY:-grpc.example.com}"
SMOKE_PATH="${SMOKE_PATH:-/}"
HTTP_REQUEST_TIMEOUT="${HTTP_REQUEST_TIMEOUT:-15}"
HTTP_CONNECTION_MODE="${HTTP_CONNECTION_MODE:-keepalive}"
STEADY_REQUESTS="${STEADY_REQUESTS:-2000}"
STEADY_CONCURRENCY="${STEADY_CONCURRENCY:-64}"
STEADY_P99_MS="${STEADY_P99_MS:-100}"
BURST_REQUESTS="${BURST_REQUESTS:-4000}"
BURST_CONCURRENCY="${BURST_CONCURRENCY:-128}"
BURST_P99_MS="${BURST_P99_MS:-150}"
CEILING_REQUESTS="${CEILING_REQUESTS:-8000}"
CEILING_CONCURRENCY="${CEILING_CONCURRENCY:-256}"
CEILING_P99_MS="${CEILING_P99_MS:-250}"
GRPC_REQUESTS="${GRPC_REQUESTS:-400}"
GRPC_CONCURRENCY="${GRPC_CONCURRENCY:-32}"
GRPC_P99_MS="${GRPC_P99_MS:-2500}"
WEBSOCKET_REQUESTS="${WEBSOCKET_REQUESTS:-20}"
WEBSOCKET_CONCURRENCY="${WEBSOCKET_CONCURRENCY:-20}"
WEBSOCKET_HOLD_MS="${WEBSOCKET_HOLD_MS:-1000}"
WEBSOCKET_P99_MS="${WEBSOCKET_P99_MS:-5000}"
STREAMING_REQUESTS="${STREAMING_REQUESTS:-6}"
STREAMING_CONCURRENCY="${STREAMING_CONCURRENCY:-3}"
SSE_P99_MS="${SSE_P99_MS:-5000}"
MCP_P99_MS="${MCP_P99_MS:-5000}"
RELOAD_HTTP_REQUESTS="${RELOAD_HTTP_REQUESTS:-1200}"
RELOAD_HTTP_CONCURRENCY="${RELOAD_HTTP_CONCURRENCY:-64}"
RELOAD_GRPC_REQUESTS="${RELOAD_GRPC_REQUESTS:-400}"
RELOAD_GRPC_CONCURRENCY="${RELOAD_GRPC_CONCURRENCY:-32}"
RELOAD_TCP_REQUESTS="${RELOAD_TCP_REQUESTS:-800}"
RELOAD_TCP_CONCURRENCY="${RELOAD_TCP_CONCURRENCY:-64}"
RELOAD_UDP_REQUESTS="${RELOAD_UDP_REQUESTS:-400}"
RELOAD_UDP_CONCURRENCY="${RELOAD_UDP_CONCURRENCY:-64}"
RELOAD_MUTATION_DELAY_SECONDS="${RELOAD_MUTATION_DELAY_SECONDS:-0.2}"
RELOAD_P99_MS="${RELOAD_P99_MS:-1000}"
BACKEND_ERROR_REQUESTS="${BACKEND_ERROR_REQUESTS:-400}"
BACKEND_ERROR_CONCURRENCY="${BACKEND_ERROR_CONCURRENCY:-32}"
BACKEND_ERROR_P99_MS="${BACKEND_ERROR_P99_MS:-500}"
BACKEND_SLOW_READ_REQUESTS="${BACKEND_SLOW_READ_REQUESTS:-400}"
BACKEND_SLOW_READ_CONCURRENCY="${BACKEND_SLOW_READ_CONCURRENCY:-32}"
BACKEND_SLOW_READ_BODY_BYTES="${BACKEND_SLOW_READ_BODY_BYTES:-65536}"
BACKEND_SLOW_READ_P99_MS="${BACKEND_SLOW_READ_P99_MS:-2500}"
BACKEND_SLOW_WRITE_REQUESTS="${BACKEND_SLOW_WRITE_REQUESTS:-400}"
BACKEND_SLOW_WRITE_CONCURRENCY="${BACKEND_SLOW_WRITE_CONCURRENCY:-32}"
BACKEND_SLOW_WRITE_P99_MS="${BACKEND_SLOW_WRITE_P99_MS:-2500}"
ENDPOINT_FLAPPING_REQUESTS="${ENDPOINT_FLAPPING_REQUESTS:-8000}"
ENDPOINT_FLAPPING_CONCURRENCY="${ENDPOINT_FLAPPING_CONCURRENCY:-64}"
ENDPOINT_FLAPPING_P99_MS="${ENDPOINT_FLAPPING_P99_MS:-1500}"
ENDPOINT_FLAPPING_MUTATION_DELAY_SECONDS="${ENDPOINT_FLAPPING_MUTATION_DELAY_SECONDS:-0.2}"
TCP_REQUESTS="${TCP_REQUESTS:-1000}"
TCP_CONCURRENCY="${TCP_CONCURRENCY:-64}"
TCP_P99_MS="${TCP_P99_MS:-500}"
UDP_REQUESTS="${UDP_REQUESTS:-500}"
UDP_CONCURRENCY="${UDP_CONCURRENCY:-64}"
UDP_P99_MS="${UDP_P99_MS:-500}"
UDP_HIGH_CHURN_REQUESTS="${UDP_HIGH_CHURN_REQUESTS:-500}"
UDP_HIGH_CHURN_CONCURRENCY="${UDP_HIGH_CHURN_CONCURRENCY:-64}"
UDP_HIGH_CHURN_P99_MS="${UDP_HIGH_CHURN_P99_MS:-750}"
UDP_MULTI_UPSTREAM_REQUESTS="${UDP_MULTI_UPSTREAM_REQUESTS:-500}"
UDP_MULTI_UPSTREAM_CONCURRENCY="${UDP_MULTI_UPSTREAM_CONCURRENCY:-64}"
UDP_MULTI_UPSTREAM_P99_MS="${UDP_MULTI_UPSTREAM_P99_MS:-500}"
UDP_MULTI_UPSTREAM_UPSTREAM_COUNT="${UDP_MULTI_UPSTREAM_UPSTREAM_COUNT:-2}"
UDP_BACKEND_TIMEOUT_REQUESTS="${UDP_BACKEND_TIMEOUT_REQUESTS:-64}"
UDP_BACKEND_TIMEOUT_CONCURRENCY="${UDP_BACKEND_TIMEOUT_CONCURRENCY:-16}"
UDP_BACKEND_TIMEOUT_SECONDS="${UDP_BACKEND_TIMEOUT_SECONDS:-0.2}"
UDP_BACKEND_TIMEOUT_P99_MS="${UDP_BACKEND_TIMEOUT_P99_MS:-1000}"
MIN_SUCCESS_RATE="${MIN_SUCCESS_RATE:-1.0}"
MAX_ERRORS="${MAX_ERRORS:-0}"
MAX_P99_MS="${MAX_P99_MS:-}"
MAX_LATENCY_MS="${MAX_LATENCY_MS:-30000}"
SLO_GATE_RISK_ACCEPTED="${SLO_GATE_RISK_ACCEPTED:-false}"
A4_UDP_BLACKHOLE_SOURCE_IMAGE="${A4_UDP_BLACKHOLE_SOURCE_IMAGE:-m.daocloud.io/docker.io/library/python:3.12-slim-bookworm}"
A4_UDP_BLACKHOLE_IMAGE="${A4_UDP_BLACKHOLE_IMAGE:-${LOCAL_REGISTRY_HOST}/aether-gateway-validation/udp-blackhole:3.12-slim-bookworm}"
A4_UDP_BLACKHOLE_PUSH_IMAGE="${A4_UDP_BLACKHOLE_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/aether-gateway-validation/udp-blackhole:3.12-slim-bookworm}"
A4_FAULT_SOURCE_IMAGE="${A4_FAULT_SOURCE_IMAGE:-m.daocloud.io/docker.io/library/python:3.12-slim-bookworm}"
A4_FAULT_IMAGE="${A4_FAULT_IMAGE:-${LOCAL_REGISTRY_HOST}/aether-gateway-validation/a4-http-fault:3.12-slim-bookworm}"
A4_FAULT_PUSH_IMAGE="${A4_FAULT_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/aether-gateway-validation/a4-http-fault:3.12-slim-bookworm}"
A4_BACKEND_ERROR_HOST="${A4_BACKEND_ERROR_HOST:-a4-backend-error.example.com}"
A4_BACKEND_SLOW_READ_HOST="${A4_BACKEND_SLOW_READ_HOST:-a4-backend-slow-read.example.com}"
A4_BACKEND_SLOW_WRITE_HOST="${A4_BACKEND_SLOW_WRITE_HOST:-a4-backend-slow-write.example.com}"
A4_ENDPOINT_FLAPPING_HOST="${A4_ENDPOINT_FLAPPING_HOST:-a4-endpoint-flapping.example.com}"
A4_BACKEND_SLOW_READ_CHUNK_DELAY_MS="${A4_BACKEND_SLOW_READ_CHUNK_DELAY_MS:-25}"
A4_BACKEND_SLOW_WRITE_CHUNK_DELAY_MS="${A4_BACKEND_SLOW_WRITE_CHUNK_DELAY_MS:-25}"
A4_BACKEND_SLOW_WRITE_CHUNKS="${A4_BACKEND_SLOW_WRITE_CHUNKS:-4}"
A4_FLAP_RESPONSE_DELAY_MS="${A4_FLAP_RESPONSE_DELAY_MS:-50}"
A4_RELOAD_TLS_HOSTNAME="${A4_RELOAD_TLS_HOSTNAME:-a4-reload.example.com}"
A4_RELOAD_TLS_SECRET_NAME="${A4_RELOAD_TLS_SECRET_NAME:-a4-reload-gateway-cert}"
A4_RELOAD_TLS_GATEWAY_NAME="${A4_RELOAD_TLS_GATEWAY_NAME:-edge-a4-reload-tls}"
A4_RELOAD_TLS_ROUTE_NAME="${A4_RELOAD_TLS_ROUTE_NAME:-a4-reload-tls}"
A4_RELOAD_TLS_PORT="${A4_RELOAD_TLS_PORT:-9443}"
A4_RELOAD_TEMP_LISTENER_NAME="${A4_RELOAD_TEMP_LISTENER_NAME:-a4-reload-temp}"
KEEP_A4_RESOURCES="${KEEP_A4_RESOURCES:-false}"
HTTP_CLIENT="${ROOT_DIR}/tests/e2e/http_concurrency_client.py"
TCP_CLIENT="${ROOT_DIR}/tests/e2e/tcp_concurrency_client.py"
UDP_CLIENT="${ROOT_DIR}/tests/e2e/udp_dns_concurrency_client.py"
TMP_DIR=""
GRPC_CLIENT_BIN=""
UDP_TIMEOUT_RELAY_PID=""
ORIGINAL_COREDNS_REPLICAS=""
ORIGINAL_ECHO_REPLICAS=""
UDP_MULTI_UPSTREAM_OBSERVED_UPSTREAMS=1
FAILURES=0

log() {
  aeg_kind_log "kind-a4-baseline" "$*"
}

require_command() {
  aeg_require_command "kind-a4-baseline" "$1"
}

ensure_stack() {
  aeg_kind_ensure_stack "kind-a4-baseline" "${ROOT_DIR}" "${HTTP_HOST}" "${GATEWAY_HOST_PORT}" "${SMOKE_PATH}"
}

write_metadata() {
  local metadata="${OUTPUT_DIR}/metadata.txt"
  local git_tree_state
  local code_tree_state
  git_tree_state="$(aeg_git_tree_state "${ROOT_DIR}")"
  code_tree_state="$(aeg_code_tree_state "${ROOT_DIR}")"
  mkdir -p "${OUTPUT_DIR}"
  {
    printf 'captured_at=%s\n' "$(date --iso-8601=seconds)"
    printf 'git_commit=%s\n' "$(git -C "${ROOT_DIR}" rev-parse HEAD)"
    printf 'git_tree_state=%s\n' "${git_tree_state}"
    printf 'code_tree_state=%s\n' "${code_tree_state}"
    printf 'run_id=%s\n' "${RUN_ID}"
    printf 'gateway_host_port=%s\n' "${GATEWAY_HOST_PORT}"
    printf 'tcp_gateway_host_port=%s\n' "${TCP_GATEWAY_HOST_PORT}"
    printf 'udp_gateway_host_port=%s\n' "${UDP_GATEWAY_HOST_PORT}"
    printf 'http_host=%s\n' "${HTTP_HOST}"
    printf 'http_connection_mode=%s\n' "${HTTP_CONNECTION_MODE}"
    printf 'grpc_authority=%s\n' "${GRPC_AUTHORITY}"
    printf 'kernel=%s\n' "$(uname -srmo)"
    printf 'cpu_count=%s\n' "$(nproc)"
    printf 'memory_kib=%s\n' "$(awk '/MemTotal:/ {print $2}' /proc/meminfo)"
  } >"${metadata}"
}

collect_admin() {
  aeg_kind_collect_admin_snapshots "${ROOT_DIR}" "${OUTPUT_DIR}" "$1"
}

cleanup() {
  local exit_code="$?"
  if [[ -n "${UDP_TIMEOUT_RELAY_PID}" ]]; then
    kill "${UDP_TIMEOUT_RELAY_PID}" >/dev/null 2>&1 || true
    wait "${UDP_TIMEOUT_RELAY_PID}" >/dev/null 2>&1 || true
  fi
  if [[ "${KEEP_A4_RESOURCES}" != "true" ]]; then
    remove_a4_reload_listener >/dev/null 2>&1 || true
    kubectl --context "${KUBE_CONTEXT}" -n "${KUBE_NAMESPACE}" annotate \
      httproute.gateway.networking.k8s.io/echo \
      a4.aether.dev/reload-route- >/dev/null 2>&1 || true
    kubectl --context "${KUBE_CONTEXT}" -n "${KUBE_NAMESPACE}" delete \
      backendlbpolicies.gateway.networking.k8s.io/a4-reload-backend-policy \
      gateway.gateway.networking.k8s.io/"${A4_RELOAD_TLS_GATEWAY_NAME}" \
      httproute.gateway.networking.k8s.io/"${A4_RELOAD_TLS_ROUTE_NAME}" \
      secret/"${A4_RELOAD_TLS_SECRET_NAME}" \
      --ignore-not-found >/dev/null 2>&1 || true
    if [[ -n "${ORIGINAL_ECHO_REPLICAS}" ]]; then
      kubectl --context "${KUBE_CONTEXT}" -n "${KUBE_NAMESPACE}" scale \
        deployment/echo \
        --replicas="${ORIGINAL_ECHO_REPLICAS}" >/dev/null 2>&1 || true
    fi
    kubectl --context "${KUBE_CONTEXT}" -n "${KUBE_NAMESPACE}" delete \
      udproute.gateway.networking.k8s.io/a4-udp-blackhole \
      gateway.gateway.networking.k8s.io/edge-a4-udp-timeout \
      service/a4-udp-blackhole \
      deployment/a4-udp-blackhole \
      configmap/a4-udp-blackhole \
      --ignore-not-found >/dev/null 2>&1 || true
    kubectl --context "${KUBE_CONTEXT}" -n "${KUBE_NAMESPACE}" delete \
      httproute.gateway.networking.k8s.io/a4-backend-error \
      httproute.gateway.networking.k8s.io/a4-backend-slow-read \
      httproute.gateway.networking.k8s.io/a4-backend-slow-write \
      httproute.gateway.networking.k8s.io/a4-endpoint-flapping \
      service/a4-fault-backend \
      service/a4-flap \
      deployment/a4-fault-backend \
      deployment/a4-flap-a \
      deployment/a4-flap-b \
      configmap/a4-fault-server \
      --ignore-not-found >/dev/null 2>&1 || true
    if [[ -n "${ORIGINAL_COREDNS_REPLICAS}" ]]; then
      kubectl --context "${KUBE_CONTEXT}" -n "${KUBE_NAMESPACE}" scale \
        deployment/coredns \
        --replicas="${ORIGINAL_COREDNS_REPLICAS}" >/dev/null 2>&1 || true
    fi
  fi
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}" >/dev/null 2>&1 || true
  fi
  exit "${exit_code}"
}

capture_resource_snapshot() {
  aeg_kind_capture_resource_snapshot "${OUTPUT_DIR}" "$1"
}

k() {
  kubectl --context "${KUBE_CONTEXT}" "$@"
}

kind_control_plane_ip() {
  docker inspect "${CLUSTER_NAME}-control-plane" \
    | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress'
}

shared_node_port_for() {
  local port="$1"
  local protocol="${2:-TCP}"

  case "${protocol^^}" in
    UDP)
      printf '%d\n' "$((31000 + (port % 1000)))"
      ;;
    *)
      if (( port < 1024 )); then
        printf '%d\n' "$((30000 + port))"
      else
        printf '%d\n' "$((32000 + (port % 1000)))"
      fi
      ;;
  esac
}

is_udp_port_listening() {
  local port="$1"

  ss -H -lun "( sport = :${port} )" 2>/dev/null \
    | awk '
      {
        address = $4
        if (address ~ /^127[.]/ || address ~ /^0[.]0[.]0[.]0:/ ||
            address ~ /^\[::1\]:/ || address ~ /^\[::\]:/ ||
            address ~ /^\*:/) {
          found = 1
        }
      }
      END { if (found) { exit 0 } exit 1 }
    '
}

wait_for_udp_port() {
  local port="$1"
  local deadline=$((SECONDS + 10))

  while (( SECONDS < deadline )); do
    if is_udp_port_listening "${port}"; then
      return 0
    fi
    sleep 0.2
  done

  return 1
}

start_udp_timeout_relay() {
  local node_ip
  local target_port

  if is_udp_port_listening "${UDP_BACKEND_TIMEOUT_HOST_PORT}"; then
    log "host port ${UDP_BACKEND_TIMEOUT_HOST_PORT}/UDP already available for UDP backend-timeout"
    return
  fi

  node_ip="$(kind_control_plane_ip)"
  if [[ -z "${node_ip}" || "${node_ip}" == "null" ]]; then
    log "failed to determine kind control-plane IP for UDP backend-timeout relay"
    exit 1
  fi

  target_port="$(shared_node_port_for "${UDP_BACKEND_TIMEOUT_LISTENER_PORT}" UDP)"
  log "bridging host port ${UDP_BACKEND_TIMEOUT_HOST_PORT}/UDP -> ${node_ip}:${target_port} for UDP backend-timeout"
  socat \
    "UDP-LISTEN:${UDP_BACKEND_TIMEOUT_HOST_PORT},bind=127.0.0.1,reuseaddr,fork" \
    "UDP:${node_ip}:${target_port}" >/dev/null 2>&1 &
  UDP_TIMEOUT_RELAY_PID="$!"

  if ! wait_for_udp_port "${UDP_BACKEND_TIMEOUT_HOST_PORT}"; then
    log "timed out waiting for UDP backend-timeout relay on ${UDP_BACKEND_TIMEOUT_HOST_PORT}/UDP"
    exit 1
  fi
}

wait_for_service_endpoint_count() {
  local namespace="$1"
  local service="$2"
  local minimum="$3"
  local deadline=$((SECONDS + 90))
  local count

  while (( SECONDS < deadline )); do
    count="$(
      k -n "${namespace}" get endpointslices \
        -l "kubernetes.io/service-name=${service}" \
        -o json \
        | jq '[.items[].endpoints[]? | select((.conditions.ready // true) != false)] | length'
    )"
    if (( count >= minimum )); then
      printf -v UDP_ENDPOINT_COUNT_RESULT '%s' "${count}"
      return 0
    fi
    sleep 1
  done

  log "timed out waiting for service ${namespace}/${service} to expose at least ${minimum} ready endpoints"
  return 1
}

wait_for_service_endpoint_exact_count() {
  local namespace="$1"
  local service="$2"
  local expected="$3"
  local deadline=$((SECONDS + 90))
  local count

  while (( SECONDS < deadline )); do
    count="$(
      k -n "${namespace}" get endpointslices \
        -l "kubernetes.io/service-name=${service}" \
        -o json \
        | jq '[.items[].endpoints[]? | select((.conditions.ready // true) != false)] | length'
    )"
    if (( count == expected )); then
      printf -v SERVICE_ENDPOINT_COUNT_RESULT '%s' "${count}"
      return 0
    fi
    sleep 1
  done

  log "timed out waiting for service ${namespace}/${service} to expose exactly ${expected} ready endpoints"
  return 1
}

wait_for_shared_udp_listener_nodeport() {
  local listener_port="$1"
  local expected_node_port
  local deadline=$((SECONDS + 90))

  expected_node_port="$(shared_node_port_for "${listener_port}" UDP)"
  while (( SECONDS < deadline )); do
    if k -n "${KUBE_NAMESPACE}" get service aether-gateway-dataplane -o json \
      | jq -e \
        --argjson listener_port "${listener_port}" \
        --argjson expected_node_port "${expected_node_port}" '
          any(.spec.ports[]?;
            .protocol == "UDP"
            and .port == $listener_port
            and .nodePort == $expected_node_port
          )
        ' >/dev/null; then
      return 0
    fi
    sleep 1
  done

  log "timed out waiting for shared dataplane service UDP listener ${listener_port} nodePort ${expected_node_port}"
  return 1
}

sync_udp_blackhole_image() {
  kind_image_sync_ensure_registry_copy \
    "${A4_UDP_BLACKHOLE_SOURCE_IMAGE}" \
    "${A4_UDP_BLACKHOLE_PUSH_IMAGE}" \
    "docker.1ms.run/library/python:3.12-slim-bookworm" \
    "python:3.12-slim-bookworm"

  log "preloading UDP blackhole image into kind nodes"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    docker exec "${node}" crictl pull "${A4_UDP_BLACKHOLE_IMAGE}" >/dev/null
  done
}

render_udp_blackhole_resources() {
  cat <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: a4-udp-blackhole
  namespace: ${KUBE_NAMESPACE}
data:
  server.py: |
    import socket

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(("0.0.0.0", 53))
    while True:
        data, addr = sock.recvfrom(4096)
        print(f"received {len(data)} bytes from {addr[0]}:{addr[1]}", flush=True)
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: a4-udp-blackhole
  namespace: ${KUBE_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: a4-udp-blackhole
  template:
    metadata:
      labels:
        app: a4-udp-blackhole
    spec:
      containers:
        - name: udp-blackhole
          image: ${A4_UDP_BLACKHOLE_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          ports:
            - name: udp
              containerPort: 53
              protocol: UDP
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: a4-udp-blackhole
---
apiVersion: v1
kind: Service
metadata:
  name: a4-udp-blackhole
  namespace: ${KUBE_NAMESPACE}
spec:
  selector:
    app: a4-udp-blackhole
  ports:
    - name: udp
      protocol: UDP
      port: 53
      targetPort: 53
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: edge-a4-udp-timeout
  namespace: ${KUBE_NAMESPACE}
spec:
  gatewayClassName: aether
  listeners:
    - name: udp-timeout
      protocol: UDP
      port: ${UDP_BACKEND_TIMEOUT_LISTENER_PORT}
      allowedRoutes:
        kinds:
          - kind: UDPRoute
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: UDPRoute
metadata:
  name: a4-udp-blackhole
  namespace: ${KUBE_NAMESPACE}
spec:
  parentRefs:
    - name: edge-a4-udp-timeout
      sectionName: udp-timeout
  rules:
    - backendRefs:
        - name: a4-udp-blackhole
          port: 53
EOF
}

udp_blackhole_log_count() {
  k -n "${KUBE_NAMESPACE}" logs deploy/a4-udp-blackhole 2>/dev/null \
    | grep -c '^received ' || true
}

ensure_udp_timeout_resources() {
  sync_udp_blackhole_image
  render_udp_blackhole_resources | k apply -f - >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/a4-udp-blackhole --timeout=120s
  wait_for_service_endpoint_count "${KUBE_NAMESPACE}" a4-udp-blackhole 1
  wait_for_shared_udp_listener_nodeport "${UDP_BACKEND_TIMEOUT_LISTENER_PORT}"
  start_udp_timeout_relay
}

ensure_udp_timeout_forwarding() {
  local attempt
  local before
  local after
  local warmup_output="${TMP_DIR}/udp-backend-timeout-warmup.json"

  for attempt in $(seq 1 20); do
    before="$(udp_blackhole_log_count)"
    python3 "${UDP_CLIENT}" \
      --addr "127.0.0.1:${UDP_BACKEND_TIMEOUT_HOST_PORT}" \
      --requests 1 \
      --concurrency 1 \
      --name foo.bar.com \
      --timeout "${UDP_BACKEND_TIMEOUT_SECONDS}" \
      --socket-mode per-request \
      --scenario backend-timeout-warmup \
      --expect-timeout \
      --output "${warmup_output}" >/dev/null || true
    after="$(udp_blackhole_log_count)"
    if (( after > before )); then
      return 0
    fi
    sleep 1
  done

  log "UDP backend-timeout warmup did not reach the blackhole backend"
  return 1
}

sync_a4_fault_image() {
  kind_image_sync_ensure_registry_copy \
    "${A4_FAULT_SOURCE_IMAGE}" \
    "${A4_FAULT_PUSH_IMAGE}" \
    "docker.1ms.run/library/python:3.12-slim-bookworm" \
    "python:3.12-slim-bookworm"

  log "preloading A4 HTTP fault image into kind nodes"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    docker exec "${node}" crictl pull "${A4_FAULT_IMAGE}" >/dev/null
  done
}

render_a4_fault_scenario_resources() {
  cat <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: a4-fault-server
  namespace: ${KUBE_NAMESPACE}
data:
  server.py: |
    import os
    import time
    from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

    RESPONSE_BODY = os.environ.get("RESPONSE_BODY", "a4-fault-ok")
    FLAP_RESPONSE_DELAY_MS = int(os.environ.get("FLAP_RESPONSE_DELAY_MS", "0"))
    SLOW_READ_CHUNK_DELAY_MS = int(os.environ.get("SLOW_READ_CHUNK_DELAY_MS", "25"))
    SLOW_WRITE_CHUNK_DELAY_MS = int(os.environ.get("SLOW_WRITE_CHUNK_DELAY_MS", "25"))
    SLOW_WRITE_CHUNKS = max(int(os.environ.get("SLOW_WRITE_CHUNKS", "4")), 1)
    SLOW_READ_CHUNK_SIZE = 4096

    class A4HTTPServer(ThreadingHTTPServer):
        daemon_threads = True
        request_queue_size = 1024

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"
        server_version = "aeg-a4-fault/1.0"

        def log_message(self, format, *args):
            return

        def send_plain(self, status, body):
            payload = body.encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            self.wfile.flush()

        def read_body_slowly(self):
            remaining = int(self.headers.get("Content-Length", "0") or "0")
            while remaining > 0:
                chunk = self.rfile.read(min(SLOW_READ_CHUNK_SIZE, remaining))
                if not chunk:
                    break
                remaining -= len(chunk)
                if remaining > 0 and SLOW_READ_CHUNK_DELAY_MS > 0:
                    time.sleep(SLOW_READ_CHUNK_DELAY_MS / 1000.0)

        def send_slow_write(self):
            payload = ("a4-backend-slow-write:" + ("x" * 32768)).encode("utf-8")
            chunk_size = max(len(payload) // SLOW_WRITE_CHUNKS, 1)
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            for offset in range(0, len(payload), chunk_size):
                self.wfile.write(payload[offset:offset + chunk_size])
                self.wfile.flush()
                if offset + chunk_size < len(payload) and SLOW_WRITE_CHUNK_DELAY_MS > 0:
                    time.sleep(SLOW_WRITE_CHUNK_DELAY_MS / 1000.0)

        def do_GET(self):
            if self.path.startswith("/healthz"):
                self.send_plain(200, "ok")
                return
            if self.path.startswith("/error"):
                self.send_plain(503, "a4-backend-error")
                return
            if self.path.startswith("/slow-write"):
                self.send_slow_write()
                return
            if self.path.startswith("/flap"):
                if FLAP_RESPONSE_DELAY_MS > 0:
                    time.sleep(FLAP_RESPONSE_DELAY_MS / 1000.0)
                self.send_plain(200, RESPONSE_BODY)
                return
            self.send_plain(200, RESPONSE_BODY)

        def do_POST(self):
            if self.path.startswith("/slow-read"):
                self.read_body_slowly()
                self.send_plain(200, "a4-backend-slow-read")
                return
            self.read_body_slowly()
            self.send_plain(200, RESPONSE_BODY)

    if __name__ == "__main__":
        A4HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: a4-fault-backend
  namespace: ${KUBE_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: a4-fault-backend
  template:
    metadata:
      labels:
        app: a4-fault-backend
    spec:
      containers:
        - name: server
          image: ${A4_FAULT_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: SLOW_READ_CHUNK_DELAY_MS
              value: "${A4_BACKEND_SLOW_READ_CHUNK_DELAY_MS}"
            - name: SLOW_WRITE_CHUNK_DELAY_MS
              value: "${A4_BACKEND_SLOW_WRITE_CHUNK_DELAY_MS}"
            - name: SLOW_WRITE_CHUNKS
              value: "${A4_BACKEND_SLOW_WRITE_CHUNKS}"
          ports:
            - containerPort: 8080
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: a4-fault-server
---
apiVersion: v1
kind: Service
metadata:
  name: a4-fault-backend
  namespace: ${KUBE_NAMESPACE}
spec:
  selector:
    app: a4-fault-backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: a4-flap-a
  namespace: ${KUBE_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: a4-flap
      member: a
  template:
    metadata:
      labels:
        app: a4-flap
        member: a
    spec:
      terminationGracePeriodSeconds: 15
      containers:
        - name: server
          image: ${A4_FAULT_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: a4-endpoint-flapping
            - name: FLAP_RESPONSE_DELAY_MS
              value: "${A4_FLAP_RESPONSE_DELAY_MS}"
          lifecycle:
            preStop:
              exec:
                command: ["/bin/sh", "-c", "sleep 10"]
          ports:
            - containerPort: 8080
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: a4-fault-server
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: a4-flap-b
  namespace: ${KUBE_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: a4-flap
      member: b
  template:
    metadata:
      labels:
        app: a4-flap
        member: b
    spec:
      terminationGracePeriodSeconds: 15
      containers:
        - name: server
          image: ${A4_FAULT_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: a4-endpoint-flapping
            - name: FLAP_RESPONSE_DELAY_MS
              value: "${A4_FLAP_RESPONSE_DELAY_MS}"
          lifecycle:
            preStop:
              exec:
                command: ["/bin/sh", "-c", "sleep 10"]
          ports:
            - containerPort: 8080
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: a4-fault-server
---
apiVersion: v1
kind: Service
metadata:
  name: a4-flap
  namespace: ${KUBE_NAMESPACE}
spec:
  selector:
    app: a4-flap
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: a4-backend-error
  namespace: ${KUBE_NAMESPACE}
spec:
  parentRefs:
    - name: edge
      sectionName: http
  hostnames:
    - ${A4_BACKEND_ERROR_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /error
      backendRefs:
        - name: a4-fault-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: a4-backend-slow-read
  namespace: ${KUBE_NAMESPACE}
spec:
  parentRefs:
    - name: edge
      sectionName: http
  hostnames:
    - ${A4_BACKEND_SLOW_READ_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /slow-read
      backendRefs:
        - name: a4-fault-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: a4-backend-slow-write
  namespace: ${KUBE_NAMESPACE}
spec:
  parentRefs:
    - name: edge
      sectionName: http
  hostnames:
    - ${A4_BACKEND_SLOW_WRITE_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /slow-write
      backendRefs:
        - name: a4-fault-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: a4-endpoint-flapping
  namespace: ${KUBE_NAMESPACE}
spec:
  parentRefs:
    - name: edge
      sectionName: http
  hostnames:
    - ${A4_ENDPOINT_FLAPPING_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /flap
      backendRefs:
        - name: a4-flap
          port: 8080
EOF
}

assert_a4_fault_warmup() {
  local label="$1"
  local file="$2"
  local extra_filter="${3:-true}"

  if ! jq -e "
    .completed == .requests
    and .successes == .requests
    and .body_mismatches == 0
    and ((.error_counts // {}) | length == 0)
    and (${extra_filter})
  " "${file}" >/dev/null; then
    log "A4 fault scenario warmup ${label} failed"
    cat "${file}" >&2 || true
    return 1
  fi
}

ensure_a4_fault_forwarding() {
  local warmup_dir="${TMP_DIR}/a4-fault-warmup"
  mkdir -p "${warmup_dir}"

  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/error" \
    --host-header "${A4_BACKEND_ERROR_HOST}" \
    --requests 4 \
    --concurrency 2 \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --expect-status 503 \
    --expect-body-substring "a4-backend-error" \
    --output "${warmup_dir}/backend-error.json" >/dev/null
  assert_a4_fault_warmup backend-error "${warmup_dir}/backend-error.json" \
    '(.status_counts["503"] // 0) == .requests'

  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/slow-read" \
    --host-header "${A4_BACKEND_SLOW_READ_HOST}" \
    --method POST \
    --body-bytes "${BACKEND_SLOW_READ_BODY_BYTES}" \
    --requests 4 \
    --concurrency 2 \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --expect-status 200 \
    --expect-body-substring "a4-backend-slow-read" \
    --output "${warmup_dir}/backend-slow-read.json" >/dev/null
  assert_a4_fault_warmup backend-slow-read "${warmup_dir}/backend-slow-read.json"

  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/slow-write" \
    --host-header "${A4_BACKEND_SLOW_WRITE_HOST}" \
    --requests 4 \
    --concurrency 2 \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --expect-status 200 \
    --expect-body-substring "a4-backend-slow-write" \
    --output "${warmup_dir}/backend-slow-write.json" >/dev/null
  assert_a4_fault_warmup backend-slow-write "${warmup_dir}/backend-slow-write.json"

  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/flap" \
    --host-header "${A4_ENDPOINT_FLAPPING_HOST}" \
    --requests 4 \
    --concurrency 2 \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --expect-status 200 \
    --expect-body-substring "a4-endpoint-flapping" \
    --output "${warmup_dir}/endpoint-flapping.json" >/dev/null
  assert_a4_fault_warmup endpoint-flapping "${warmup_dir}/endpoint-flapping.json"
}

ensure_a4_fault_scenario_resources() {
  sync_a4_fault_image
  render_a4_fault_scenario_resources | k apply -f - >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/a4-fault-backend --timeout=120s
  k -n "${KUBE_NAMESPACE}" rollout status deployment/a4-flap-a --timeout=120s
  k -n "${KUBE_NAMESPACE}" rollout status deployment/a4-flap-b --timeout=120s
  wait_for_service_endpoint_count "${KUBE_NAMESPACE}" a4-fault-backend 1
  wait_for_service_endpoint_exact_count "${KUBE_NAMESPACE}" a4-flap 2
  wait_for_httproute_accepted a4-backend-error
  wait_for_httproute_accepted a4-backend-slow-read
  wait_for_httproute_accepted a4-backend-slow-write
  wait_for_httproute_accepted a4-endpoint-flapping
  ensure_a4_fault_forwarding
}

ensure_udp_multi_upstream_resources() {
  local replicas

  ORIGINAL_COREDNS_REPLICAS="$(
    k -n "${KUBE_NAMESPACE}" get deployment/coredns -o jsonpath='{.spec.replicas}' 2>/dev/null || true
  )"
  if [[ -z "${ORIGINAL_COREDNS_REPLICAS}" ]]; then
    log "unable to read coredns deployment replica count"
    exit 1
  fi

  replicas="${UDP_MULTI_UPSTREAM_UPSTREAM_COUNT}"
  k -n "${KUBE_NAMESPACE}" scale deployment/coredns --replicas="${replicas}" >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/coredns --timeout=120s
  wait_for_service_endpoint_count "${KUBE_NAMESPACE}" coredns "${replicas}"
  UDP_MULTI_UPSTREAM_OBSERVED_UPSTREAMS="${UDP_ENDPOINT_COUNT_RESULT}"
}

generate_a4_reload_tls_cert() {
  local prefix="$1"
  local cn="$2"
  local openssl_config="${TMP_DIR}/${prefix}.cnf"

  cat >"${openssl_config}" <<EOF
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = ${cn}

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${A4_RELOAD_TLS_HOSTNAME}
EOF

  openssl req \
    -x509 \
    -nodes \
    -newkey rsa:2048 \
    -days 2 \
    -keyout "${TMP_DIR}/${prefix}.key" \
    -out "${TMP_DIR}/${prefix}.crt" \
    -config "${openssl_config}" \
    -extensions v3_req >/dev/null 2>&1
}

apply_a4_reload_tls_secret() {
  local prefix="$1"

  k -n "${KUBE_NAMESPACE}" create secret tls "${A4_RELOAD_TLS_SECRET_NAME}" \
    --cert="${TMP_DIR}/${prefix}.crt" \
    --key="${TMP_DIR}/${prefix}.key" \
    --dry-run=client \
    -o yaml | k apply -f - >/dev/null
}

render_a4_reload_tls_resources() {
  cat <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${A4_RELOAD_TLS_GATEWAY_NAME}
  namespace: ${KUBE_NAMESPACE}
spec:
  gatewayClassName: aether
  listeners:
    - name: https
      protocol: HTTPS
      port: ${A4_RELOAD_TLS_PORT}
      hostname: ${A4_RELOAD_TLS_HOSTNAME}
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: ${A4_RELOAD_TLS_SECRET_NAME}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ${A4_RELOAD_TLS_ROUTE_NAME}
  namespace: ${KUBE_NAMESPACE}
spec:
  parentRefs:
    - name: ${A4_RELOAD_TLS_GATEWAY_NAME}
      sectionName: https
  hostnames:
    - ${A4_RELOAD_TLS_HOSTNAME}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: echo
          port: 8080
EOF
}

wait_for_gateway_programmed() {
  local gateway="$1"
  local deadline=$((SECONDS + 90))

  while (( SECONDS < deadline )); do
    if k -n "${KUBE_NAMESPACE}" get gateway "${gateway}" -o json \
      | jq -e '
          ([.status.listeners[]?.conditions[]? | select(.type=="Programmed" and .status=="True")] | length > 0)
          and
          ([.status.listeners[]?.conditions[]? | select(.type=="ResolvedRefs" and .status=="True")] | length > 0)
        ' >/dev/null 2>&1
    then
      return 0
    fi
    sleep 1
  done

  log "gateway ${gateway} did not become programmed"
  k -n "${KUBE_NAMESPACE}" get gateway "${gateway}" -o yaml >&2 || true
  return 1
}

wait_for_httproute_accepted() {
  local route="$1"
  local deadline=$((SECONDS + 90))

  while (( SECONDS < deadline )); do
    if k -n "${KUBE_NAMESPACE}" get httproute "${route}" -o json \
      | jq -e '[.status.parents[]?.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0' >/dev/null 2>&1
    then
      return 0
    fi
    sleep 1
  done

  log "httproute ${route} did not become accepted"
  k -n "${KUBE_NAMESPACE}" get httproute "${route}" -o yaml >&2 || true
  return 1
}

ensure_a4_reload_tls_resources() {
  generate_a4_reload_tls_cert a4-reload-initial a4-reload-initial
  generate_a4_reload_tls_cert a4-reload-secret-only a4-reload-secret-only
  generate_a4_reload_tls_cert a4-reload-rotated a4-reload-rotated
  apply_a4_reload_tls_secret a4-reload-initial
  render_a4_reload_tls_resources | k apply -f - >/dev/null
  wait_for_gateway_programmed "${A4_RELOAD_TLS_GATEWAY_NAME}"
  wait_for_httproute_accepted "${A4_RELOAD_TLS_ROUTE_NAME}"
}

remove_a4_reload_listener() {
  local listener_index

  listener_index="$(
    k -n "${KUBE_NAMESPACE}" get gateway edge -o json 2>/dev/null \
      | jq -r --arg name "${A4_RELOAD_TEMP_LISTENER_NAME}" '
          .spec.listeners
          | to_entries[]
          | select(.value.name == $name)
          | .key
        ' \
      | head -n 1
  )"
  if [[ -n "${listener_index}" ]]; then
    k -n "${KUBE_NAMESPACE}" patch gateway edge --type=json \
      -p "[{\"op\":\"remove\",\"path\":\"/spec/listeners/${listener_index}\"}]" >/dev/null
  fi
}

apply_reload_mutation() {
  local mutation="$1"

  case "${mutation}" in
    route-only)
      k -n "${KUBE_NAMESPACE}" annotate \
        httproute.gateway.networking.k8s.io/echo \
        "a4.aether.dev/reload-route=$(date +%s%N)" \
        --overwrite >/dev/null
      ;;
    backend-only)
      cat <<EOF | k apply -f - >/dev/null
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: BackendLBPolicy
metadata:
  name: a4-reload-backend-policy
  namespace: ${KUBE_NAMESPACE}
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: echo
  sessionPersistence:
    sessionName: x-a4-reload-session
    type: Header
EOF
      ;;
    endpoint-only)
      if [[ -z "${ORIGINAL_ECHO_REPLICAS}" ]]; then
        ORIGINAL_ECHO_REPLICAS="$(
          k -n "${KUBE_NAMESPACE}" get deployment/echo -o jsonpath='{.spec.replicas}' 2>/dev/null || true
        )"
      fi
      k -n "${KUBE_NAMESPACE}" scale deployment/echo --replicas=2 >/dev/null
      k -n "${KUBE_NAMESPACE}" rollout status deployment/echo --timeout=120s >/dev/null
      wait_for_service_endpoint_count "${KUBE_NAMESPACE}" echo 2 >/dev/null
      ;;
    secret-only)
      apply_a4_reload_tls_secret a4-reload-secret-only
      ;;
    tls-asset-rotation)
      apply_a4_reload_tls_secret a4-reload-rotated
      ;;
    listener-add-remove)
      remove_a4_reload_listener >/dev/null 2>&1 || true
      k -n "${KUBE_NAMESPACE}" patch gateway edge --type=json \
        -p "[{\"op\":\"add\",\"path\":\"/spec/listeners/-\",\"value\":{\"name\":\"${A4_RELOAD_TEMP_LISTENER_NAME}\",\"protocol\":\"HTTP\",\"port\":8081}}]" >/dev/null
      sleep 1
      remove_a4_reload_listener
      ;;
    *)
      log "unknown reload mutation ${mutation}"
      return 1
      ;;
  esac
}

augment_json_with_runtime() {
  local file="$1"
  local elapsed_ms="$2"
  local threshold="$3"
  local tmp
  tmp="$(mktemp)"
  jq \
    --argjson elapsed_ms "${elapsed_ms}" \
    --argjson threshold_p99_ms "${threshold}" \
    '. + {
      elapsed_ms: $elapsed_ms,
      achieved_rps: (if $elapsed_ms > 0 then (.completed / ($elapsed_ms / 1000.0)) else 0 end),
      threshold_p99_ms: $threshold_p99_ms
    }' "${file}" >"${tmp}"
  mv "${tmp}" "${file}"
}

run_http_profile() {
  local label="$1"
  local requests="$2"
  local concurrency="$3"
  local threshold_p99_ms="$4"
  local output="${OUTPUT_DIR}/http/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms

  mkdir -p "${OUTPUT_DIR}/http"
  started_ms="$(date +%s%3N)"
  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}${SMOKE_PATH}" \
    --host-header "${HTTP_HOST}" \
    --requests "${requests}" \
    --concurrency "${concurrency}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --connection-mode "${HTTP_CONNECTION_MODE}" \
    --expect-status 200 \
    --expect-body-substring "aether-gateway-ok" \
    --output "${output}" >/dev/null
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${threshold_p99_ms}"

  if ! jq -e '
    .completed == .requests
    and .successes == .requests
    and ((.error_counts | length) == 0)
    and .body_mismatches == 0
    and (.latency_ms.p99 <= .threshold_p99_ms)
  ' "${output}" >/dev/null; then
    log "http profile ${label} exceeded threshold"
  fi
}

run_grpc_profile() {
  local label="$1"
  local requests="$2"
  local concurrency="$3"
  local threshold_p99_ms="$4"
  local output="${OUTPUT_DIR}/grpc/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms

  mkdir -p "${OUTPUT_DIR}/grpc"
  started_ms="$(date +%s%3N)"
  "${GRPC_CLIENT_BIN}" \
    -json \
    -addr "127.0.0.1:${GATEWAY_HOST_PORT}" \
    -authority "${GRPC_AUTHORITY}" \
    -requests "${requests}" \
    -concurrency "${concurrency}" >"${output}"
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${threshold_p99_ms}"

  if ! jq -e '
    .completed == .requests
    and .successes == .requests
    and ((.error_counts // {}) | length) == 0
    and (.latency_ms.p99 <= .threshold_p99_ms)
  ' "${output}" >/dev/null; then
    log "grpc profile ${label} exceeded threshold"
  fi
}

run_tcp_profile() {
  local label="$1"
  local requests="$2"
  local concurrency="$3"
  local threshold_p99_ms="$4"
  local output="${OUTPUT_DIR}/tcp/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms

  mkdir -p "${OUTPUT_DIR}/tcp"
  started_ms="$(date +%s%3N)"
  python3 "${TCP_CLIENT}" \
    --addr "127.0.0.1:${TCP_GATEWAY_HOST_PORT}" \
    --requests "${requests}" \
    --concurrency "${concurrency}" \
    --host-header "${HTTP_HOST}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --expect-substring "aether-gateway-ok" \
    --scenario "${label}" \
    --output "${output}" >/dev/null
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${threshold_p99_ms}"

  if ! jq -e '
    .completed == .requests
    and .successes == .requests
    and ((.error_counts // {}) | length) == 0
    and (.latency_ms.p99 <= .threshold_p99_ms)
  ' "${output}" >/dev/null; then
    log "tcp profile ${label} exceeded threshold"
  fi
}

run_udp_profile() {
  local label="$1"
  local requests="$2"
  local concurrency="$3"
  local threshold_p99_ms="$4"
  local socket_mode="$5"
  local output="${OUTPUT_DIR}/udp/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms
  shift 5
  local extra_args=("$@")

  mkdir -p "${OUTPUT_DIR}/udp"
  started_ms="$(date +%s%3N)"
  python3 "${UDP_CLIENT}" \
    --addr "127.0.0.1:${UDP_GATEWAY_HOST_PORT}" \
    --requests "${requests}" \
    --concurrency "${concurrency}" \
    --name foo.bar.com \
    --timeout 3 \
    --socket-mode "${socket_mode}" \
    --scenario "${label}" \
    --output "${output}" \
    "${extra_args[@]}" >/dev/null
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${threshold_p99_ms}"

  if ! jq -e '
    .completed == .requests
    and .successes == .requests
    and .packets_lost == 0
    and ((.error_counts // {}) | length) == 0
    and (.latency_ms.p99 <= .threshold_p99_ms)
  ' "${output}" >/dev/null; then
    log "udp profile ${label} exceeded threshold"
  fi
}

run_udp_backend_timeout_profile() {
  local label="backend-timeout"
  local output="${OUTPUT_DIR}/udp/${label}.json"
  local backend_before
  local backend_after
  local backend_delta
  local started_ms
  local ended_ms
  local elapsed_ms
  local tmp

  mkdir -p "${OUTPUT_DIR}/udp"
  backend_before="$(udp_blackhole_log_count)"
  started_ms="$(date +%s%3N)"
  python3 "${UDP_CLIENT}" \
    --addr "127.0.0.1:${UDP_BACKEND_TIMEOUT_HOST_PORT}" \
    --requests "${UDP_BACKEND_TIMEOUT_REQUESTS}" \
    --concurrency "${UDP_BACKEND_TIMEOUT_CONCURRENCY}" \
    --name foo.bar.com \
    --timeout "${UDP_BACKEND_TIMEOUT_SECONDS}" \
    --socket-mode per-worker \
    --scenario "${label}" \
    --expect-timeout \
    --upstream-count 1 \
    --output "${output}" >/dev/null
  ended_ms="$(date +%s%3N)"
  backend_after="$(udp_blackhole_log_count)"
  backend_delta="$((backend_after - backend_before))"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${UDP_BACKEND_TIMEOUT_P99_MS}"
  tmp="$(mktemp)"
  jq \
    --argjson backend_datagrams_received "${backend_delta}" \
    --argjson backend_log_count_before "${backend_before}" \
    --argjson backend_log_count_after "${backend_after}" \
    '. + {
      backend_datagrams_received: $backend_datagrams_received,
      backend_timeout_evidence: {
        backend_log_count_before: $backend_log_count_before,
        backend_log_count_after: $backend_log_count_after,
        backend_received_at_least_requests: ($backend_datagrams_received >= .requests)
      }
    }' "${output}" >"${tmp}"
  mv "${tmp}" "${output}"

  if ! jq -e '
    .completed == .requests
    and .successes == .requests
    and .expected_timeout == true
    and .packets_received == 0
    and .packets_lost == .requests
    and ((.error_counts // {}).timeout == .requests)
    and .backend_datagrams_received >= .requests
    and (.latency_ms.p99 <= .threshold_p99_ms)
  ' "${output}" >/dev/null; then
    log "udp profile ${label} exceeded threshold or did not reach backend"
    FAILURES=$((FAILURES + 1))
  fi
}

annotate_fault_profile() {
  local file="$1"
  local scenario="$2"
  local tmp

  tmp="$(mktemp)"
  jq \
    --arg scenario "${scenario}" \
    '. + {
      protocol: "http",
      scenario: $scenario,
      scenarios: (((.scenarios // []) + [$scenario]) | unique)
    }' "${file}" >"${tmp}"
  mv "${tmp}" "${file}"
}

assert_fault_profile() {
  local label="$1"
  local file="$2"
  local extra_filter="${3:-true}"

  if ! jq -e "
    .scenario == \"${label}\"
    and .completed == .requests
    and .successes == .requests
    and .body_mismatches == 0
    and ((.error_counts // {}) | length == 0)
    and (.latency_ms.p99 <= .threshold_p99_ms)
    and (${extra_filter})
  " "${file}" >/dev/null; then
    log "fault scenario profile ${label} exceeded threshold"
    FAILURES=$((FAILURES + 1))
  fi
}

run_backend_error_profile() {
  local label="backend-error"
  local output="${OUTPUT_DIR}/http/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms

  mkdir -p "${OUTPUT_DIR}/http"
  started_ms="$(date +%s%3N)"
  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/error" \
    --host-header "${A4_BACKEND_ERROR_HOST}" \
    --requests "${BACKEND_ERROR_REQUESTS}" \
    --concurrency "${BACKEND_ERROR_CONCURRENCY}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --connection-mode "${HTTP_CONNECTION_MODE}" \
    --expect-status 503 \
    --expect-body-substring "a4-backend-error" \
    --output "${output}" >/dev/null
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${BACKEND_ERROR_P99_MS}"
  annotate_fault_profile "${output}" "${label}"
  assert_fault_profile "${label}" "${output}" '(.status_counts["503"] // 0) == .requests'
}

run_backend_slow_read_profile() {
  local label="backend-slow-read"
  local output="${OUTPUT_DIR}/http/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms

  mkdir -p "${OUTPUT_DIR}/http"
  started_ms="$(date +%s%3N)"
  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/slow-read" \
    --host-header "${A4_BACKEND_SLOW_READ_HOST}" \
    --method POST \
    --body-bytes "${BACKEND_SLOW_READ_BODY_BYTES}" \
    --requests "${BACKEND_SLOW_READ_REQUESTS}" \
    --concurrency "${BACKEND_SLOW_READ_CONCURRENCY}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --connection-mode "${HTTP_CONNECTION_MODE}" \
    --expect-status 200 \
    --expect-body-substring "a4-backend-slow-read" \
    --output "${output}" >/dev/null
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${BACKEND_SLOW_READ_P99_MS}"
  annotate_fault_profile "${output}" "${label}"
  assert_fault_profile "${label}" "${output}" '(.status_counts["200"] // 0) == .requests'
}

run_backend_slow_write_profile() {
  local label="backend-slow-write"
  local output="${OUTPUT_DIR}/http/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms

  mkdir -p "${OUTPUT_DIR}/http"
  started_ms="$(date +%s%3N)"
  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/slow-write" \
    --host-header "${A4_BACKEND_SLOW_WRITE_HOST}" \
    --requests "${BACKEND_SLOW_WRITE_REQUESTS}" \
    --concurrency "${BACKEND_SLOW_WRITE_CONCURRENCY}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --connection-mode "${HTTP_CONNECTION_MODE}" \
    --expect-status 200 \
    --expect-body-substring "a4-backend-slow-write" \
    --output "${output}" >/dev/null
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${BACKEND_SLOW_WRITE_P99_MS}"
  annotate_fault_profile "${output}" "${label}"
  assert_fault_profile "${label}" "${output}" '(.status_counts["200"] // 0) == .requests'
}

run_endpoint_flapping_profile() {
  local label="endpoint-flapping"
  local output="${OUTPUT_DIR}/http/${label}.json"
  local started_ms
  local ended_ms
  local elapsed_ms
  local mutation_started_ms
  local mutation_ended_ms
  local mutation_elapsed_ms
  local client_pid
  local tmp

  mkdir -p "${OUTPUT_DIR}/http"
  k -n "${KUBE_NAMESPACE}" scale deployment/a4-flap-a --replicas=1 >/dev/null
  k -n "${KUBE_NAMESPACE}" scale deployment/a4-flap-b --replicas=1 >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/a4-flap-a --timeout=120s >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/a4-flap-b --timeout=120s >/dev/null
  wait_for_service_endpoint_exact_count "${KUBE_NAMESPACE}" a4-flap 2 >/dev/null

  started_ms="$(date +%s%3N)"
  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/flap" \
    --host-header "${A4_ENDPOINT_FLAPPING_HOST}" \
    --requests "${ENDPOINT_FLAPPING_REQUESTS}" \
    --concurrency "${ENDPOINT_FLAPPING_CONCURRENCY}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --connection-mode "${HTTP_CONNECTION_MODE}" \
    --expect-status 200 \
    --expect-body-substring "a4-endpoint-flapping" \
    --output "${output}" >/dev/null &
  client_pid="$!"
  sleep "${ENDPOINT_FLAPPING_MUTATION_DELAY_SECONDS}"
  mutation_started_ms="$(date +%s%3N)"
  k -n "${KUBE_NAMESPACE}" scale deployment/a4-flap-a --replicas=0 >/dev/null
  wait_for_service_endpoint_exact_count "${KUBE_NAMESPACE}" a4-flap 1 >/dev/null
  k -n "${KUBE_NAMESPACE}" scale deployment/a4-flap-a --replicas=1 >/dev/null
  k -n "${KUBE_NAMESPACE}" rollout status deployment/a4-flap-a --timeout=120s >/dev/null
  wait_for_service_endpoint_exact_count "${KUBE_NAMESPACE}" a4-flap 2 >/dev/null
  mutation_ended_ms="$(date +%s%3N)"
  wait "${client_pid}"
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  mutation_elapsed_ms="$((mutation_ended_ms - mutation_started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${ENDPOINT_FLAPPING_P99_MS}"
  annotate_fault_profile "${output}" "${label}"
  tmp="$(mktemp)"
  jq \
    --arg flap_backend "a4-flap-a" \
    --argjson mutation_apply_elapsed_ms "${mutation_elapsed_ms}" \
    '. + {
      endpoint_flapping_phase: "during",
      flap_backend: $flap_backend,
      endpoint_mutation: "scale-down-up",
      endpoint_mutation_elapsed_ms: $mutation_apply_elapsed_ms,
      mutation_apply_elapsed_ms: $mutation_apply_elapsed_ms
    }' "${output}" >"${tmp}"
  mv "${tmp}" "${output}"
  assert_fault_profile "${label}" "${output}" '(.status_counts["200"] // 0) == .requests'
}

run_fault_scenario_profiles() {
  run_backend_error_profile
  run_backend_slow_read_profile
  run_backend_slow_write_profile
  run_endpoint_flapping_profile
}

annotate_reload_profile() {
  local file="$1"
  local protocol="$2"
  local mutation="$3"
  local apply_elapsed_ms="$4"
  local tmp

  tmp="$(mktemp)"
  jq \
    --arg protocol "${protocol}" \
    --arg mutation "${mutation}" \
    --argjson mutation_apply_elapsed_ms "${apply_elapsed_ms}" \
    '. + {
      protocol: $protocol,
      scenario: "reload-under-load",
      scenarios: (((.scenarios // []) + ["reload-under-load"]) | unique),
      reload_phase: "during",
      reload_mutation: $mutation,
      snapshot_mutation: $mutation,
      reload_mutations: [$mutation],
      mutation_apply_elapsed_ms: $mutation_apply_elapsed_ms
    }' "${file}" >"${tmp}"
  mv "${tmp}" "${file}"
}

assert_reload_profile() {
  local label="$1"
  local file="$2"
  local extra_filter="${3:-true}"

  if ! jq -e "
    .scenario == \"reload-under-load\"
    and .reload_phase == \"during\"
    and (.reload_mutation | length > 0)
    and .completed == .requests
    and .successes == .requests
    and ((.error_counts // {}) | length == 0)
    and (.latency_ms.p99 <= .threshold_p99_ms)
    and (${extra_filter})
  " "${file}" >/dev/null; then
    log "live reload profile ${label} exceeded threshold"
    FAILURES=$((FAILURES + 1))
  fi
}

run_http_reload_profile() {
  local mutation="$1"
  local output="${OUTPUT_DIR}/http/live-reload-${mutation}.json"
  local started_ms
  local ended_ms
  local mutation_started_ms
  local mutation_ended_ms
  local elapsed_ms
  local mutation_elapsed_ms
  local client_pid

  mkdir -p "${OUTPUT_DIR}/http"
  started_ms="$(date +%s%3N)"
  python3 "${HTTP_CLIENT}" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}${SMOKE_PATH}" \
    --host-header "${HTTP_HOST}" \
    --requests "${RELOAD_HTTP_REQUESTS}" \
    --concurrency "${RELOAD_HTTP_CONCURRENCY}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --connection-mode "${HTTP_CONNECTION_MODE}" \
    --expect-status 200 \
    --expect-body-substring "aether-gateway-ok" \
    --output "${output}" >/dev/null &
  client_pid="$!"
  sleep "${RELOAD_MUTATION_DELAY_SECONDS}"
  mutation_started_ms="$(date +%s%3N)"
  apply_reload_mutation "${mutation}"
  mutation_ended_ms="$(date +%s%3N)"
  wait "${client_pid}"
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  mutation_elapsed_ms="$((mutation_ended_ms - mutation_started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${RELOAD_P99_MS}"
  annotate_reload_profile "${output}" http "${mutation}" "${mutation_elapsed_ms}"
  assert_reload_profile "http/${mutation}" "${output}" '.body_mismatches == 0'
}

run_grpc_reload_profile() {
  local mutation="$1"
  local output="${OUTPUT_DIR}/grpc/live-reload-${mutation}.json"
  local started_ms
  local ended_ms
  local mutation_started_ms
  local mutation_ended_ms
  local elapsed_ms
  local mutation_elapsed_ms
  local client_pid

  mkdir -p "${OUTPUT_DIR}/grpc"
  started_ms="$(date +%s%3N)"
  "${GRPC_CLIENT_BIN}" \
    -json \
    -addr "127.0.0.1:${GATEWAY_HOST_PORT}" \
    -authority "${GRPC_AUTHORITY}" \
    -requests "${RELOAD_GRPC_REQUESTS}" \
    -concurrency "${RELOAD_GRPC_CONCURRENCY}" >"${output}" &
  client_pid="$!"
  sleep "${RELOAD_MUTATION_DELAY_SECONDS}"
  mutation_started_ms="$(date +%s%3N)"
  apply_reload_mutation "${mutation}"
  mutation_ended_ms="$(date +%s%3N)"
  wait "${client_pid}"
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  mutation_elapsed_ms="$((mutation_ended_ms - mutation_started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${RELOAD_P99_MS}"
  annotate_reload_profile "${output}" grpc "${mutation}" "${mutation_elapsed_ms}"
  assert_reload_profile "grpc/${mutation}" "${output}"
}

run_tcp_reload_profile() {
  local mutation="$1"
  local output="${OUTPUT_DIR}/tcp/live-reload-${mutation}.json"
  local started_ms
  local ended_ms
  local mutation_started_ms
  local mutation_ended_ms
  local elapsed_ms
  local mutation_elapsed_ms
  local client_pid

  mkdir -p "${OUTPUT_DIR}/tcp"
  started_ms="$(date +%s%3N)"
  python3 "${TCP_CLIENT}" \
    --addr "127.0.0.1:${TCP_GATEWAY_HOST_PORT}" \
    --requests "${RELOAD_TCP_REQUESTS}" \
    --concurrency "${RELOAD_TCP_CONCURRENCY}" \
    --host-header "${HTTP_HOST}" \
    --connect-timeout 3 \
    --request-timeout "${HTTP_REQUEST_TIMEOUT}" \
    --expect-substring "aether-gateway-ok" \
    --scenario "reload-under-load" \
    --output "${output}" >/dev/null &
  client_pid="$!"
  sleep "${RELOAD_MUTATION_DELAY_SECONDS}"
  mutation_started_ms="$(date +%s%3N)"
  apply_reload_mutation "${mutation}"
  mutation_ended_ms="$(date +%s%3N)"
  wait "${client_pid}"
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  mutation_elapsed_ms="$((mutation_ended_ms - mutation_started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${RELOAD_P99_MS}"
  annotate_reload_profile "${output}" tcp "${mutation}" "${mutation_elapsed_ms}"
  assert_reload_profile "tcp/${mutation}" "${output}"
}

run_udp_reload_profile() {
  local mutation="$1"
  local output="${OUTPUT_DIR}/udp/live-reload-${mutation}.json"
  local started_ms
  local ended_ms
  local mutation_started_ms
  local mutation_ended_ms
  local elapsed_ms
  local mutation_elapsed_ms
  local client_pid

  mkdir -p "${OUTPUT_DIR}/udp"
  started_ms="$(date +%s%3N)"
  python3 "${UDP_CLIENT}" \
    --addr "127.0.0.1:${UDP_GATEWAY_HOST_PORT}" \
    --requests "${RELOAD_UDP_REQUESTS}" \
    --concurrency "${RELOAD_UDP_CONCURRENCY}" \
    --name foo.bar.com \
    --timeout 3 \
    --socket-mode per-worker \
    --scenario "reload-under-load" \
    --upstream-count "${UDP_MULTI_UPSTREAM_OBSERVED_UPSTREAMS}" \
    --output "${output}" >/dev/null &
  client_pid="$!"
  sleep "${RELOAD_MUTATION_DELAY_SECONDS}"
  mutation_started_ms="$(date +%s%3N)"
  apply_reload_mutation "${mutation}"
  mutation_ended_ms="$(date +%s%3N)"
  wait "${client_pid}"
  ended_ms="$(date +%s%3N)"
  elapsed_ms="$((ended_ms - started_ms))"
  mutation_elapsed_ms="$((mutation_ended_ms - mutation_started_ms))"
  augment_json_with_runtime "${output}" "${elapsed_ms}" "${RELOAD_P99_MS}"
  annotate_reload_profile "${output}" udp "${mutation}" "${mutation_elapsed_ms}"
  assert_reload_profile "udp/${mutation}" "${output}" '.packets_lost == 0'
}

run_live_reload_profiles() {
  run_http_reload_profile route-only
  run_http_reload_profile backend-only
  run_tcp_reload_profile endpoint-only
  run_udp_reload_profile secret-only
  run_grpc_reload_profile tls-asset-rotation
  run_tcp_reload_profile listener-add-remove
}

run_specialized_e2e() {
  mkdir -p "${OUTPUT_DIR}/logs"
  (
    cd "${ROOT_DIR}"
    PROFILE_OUTPUT_DIR="${OUTPUT_DIR}" \
      STREAM_PROFILE_REQUESTS="${STREAMING_REQUESTS}" \
      STREAM_PROFILE_CONCURRENCY="${STREAMING_CONCURRENCY}" \
      ./tests/e2e/validate-upstream-behavior.sh
  ) >"${OUTPUT_DIR}/logs/upstream-behavior.log" 2>&1
  (
    cd "${ROOT_DIR}"
    PROFILE_OUTPUT_DIR="${OUTPUT_DIR}" \
      WEBSOCKET_PROFILE_REQUESTS="${WEBSOCKET_REQUESTS}" \
      WEBSOCKET_PROFILE_CONCURRENCY="${WEBSOCKET_CONCURRENCY}" \
      WEBSOCKET_PROFILE_HOLD_MS="${WEBSOCKET_HOLD_MS}" \
      ./tests/e2e/validate-backend-protocols.sh
  ) >"${OUTPUT_DIR}/logs/backend-protocols.log" 2>&1
}

capture_component_logs() {
  mkdir -p "${OUTPUT_DIR}/logs"
  kubectl --context kind-aether-gateway -n aether-gateway logs deploy/aether-gateway-controlplane --tail=200 \
    >"${OUTPUT_DIR}/logs/controlplane.log"
  kubectl --context kind-aether-gateway -n aether-gateway logs deploy/aether-gateway-dataplane --tail=200 \
    >"${OUTPUT_DIR}/logs/dataplane.log"
}

profile_row() {
  local label="$1"
  local file="$2"
  jq -r --arg label "${label}" '
    "| \($label) | \(.connection_mode // "close") | \(.requests) | \(.concurrency) | \(((.success_rate * 10000)|floor/100)|tostring + "%") | \(((.latency_ms.p95 * 100)|floor/100)|tostring) | \(((.latency_ms.p99 * 100)|floor/100)|tostring) | \(((.latency_ms.max * 100)|floor/100)|tostring) | \(((.achieved_rps * 100)|floor/100)|tostring) |"
  ' "${file}"
}

stream_profile_row() {
  local label="$1"
  local file="$2"
  jq -r --arg label "${label}" '
    "| \($label) | \(.requests) | \(.concurrency) | \(.connection_count // .concurrency) | \(((.success_rate * 10000)|floor/100)|tostring + "%") | \(((.latency_ms.p95 * 100)|floor/100)|tostring) | \(((.latency_ms.p99 * 100)|floor/100)|tostring) | \(((.latency_ms.p999 * 100)|floor/100)|tostring) | \(((.latency_ms.max * 100)|floor/100)|tostring) | \(((.achieved_rps * 100)|floor/100)|tostring) |"
  ' "${file}"
}

udp_profile_row() {
  local label="$1"
  local file="$2"
  jq -r --arg label "${label}" '
    "| \($label) | \(.requests) | \(.concurrency) | \(.client_count) | \(.upstream_count // 1) | \((.expected_timeout // false)|tostring) | \(.session_opens) | \(.packets_sent) | \(.packets_received) | \(.packets_lost) | \(((.success_rate * 10000)|floor/100)|tostring + "%") | \(((.latency_ms.p95 * 100)|floor/100)|tostring) | \(((.latency_ms.p99 * 100)|floor/100)|tostring) | \(((.latency_ms.p999 * 100)|floor/100)|tostring) | \(((.achieved_rps * 100)|floor/100)|tostring) |"
  ' "${file}"
}

long_lived_profile_row() {
  local label="$1"
  local file="$2"
  jq -r --arg label "${label}" '
    def rounded($value): if $value == null then "n/a" else ((($value * 100)|floor/100)|tostring) end;
    "| \($label) | \(.requests) | \(.concurrency) | \(((.success_rate * 10000)|floor/100)|tostring + "%") | \(rounded(.latency_ms.p95)) | \(rounded(.latency_ms.p99)) | \(rounded(.latency_ms.p999)) | \(rounded(.latency_ms.max)) | \(rounded(.achieved_rps)) |"
  ' "${file}"
}

live_reload_profile_row() {
  local label="$1"
  local file="$2"
  jq -r --arg label "${label}" '
    def rounded($value): if $value == null then "n/a" else ((($value * 100)|floor/100)|tostring) end;
    "| \($label) | \(.protocol) | \(.reload_mutation) | \(.requests) | \(.concurrency) | \(((.success_rate * 10000)|floor/100)|tostring + "%") | \(rounded(.latency_ms.p95)) | \(rounded(.latency_ms.p99)) | \(rounded(.latency_ms.p999)) | \(rounded(.latency_ms.max)) | \(rounded(.achieved_rps)) | \(.mutation_apply_elapsed_ms) |"
  ' "${file}"
}

fault_profile_row() {
  local label="$1"
  local file="$2"
  jq -r --arg label "${label}" '
    def rounded($value): if $value == null then "n/a" else ((($value * 100)|floor/100)|tostring) end;
    def statuses: (.status_counts // {}) | to_entries | map("\(.key)=\(.value)") | join(",");
    "| \($label) | \(.method // "GET") | \(statuses) | \(.requests) | \(.concurrency) | \(((.success_rate * 10000)|floor/100)|tostring + "%") | \(rounded(.latency_ms.p95)) | \(rounded(.latency_ms.p99)) | \(rounded(.latency_ms.max)) | \(rounded(.achieved_rps)) | \(.flap_backend // "n/a") | \(.endpoint_mutation_elapsed_ms // "n/a") |"
  ' "${file}"
}

profile_p99_threshold() {
  local default_threshold="$1"

  if [[ -n "${MAX_P99_MS}" ]]; then
    printf '%s\n' "${MAX_P99_MS}"
    return
  fi

  printf '%s\n' "${default_threshold}"
}

write_slo_gate() {
  aeg_kind_write_profile_slo_gate \
    "${SLO_OUTPUT}" \
    "${MIN_SUCCESS_RATE}" \
    "${MAX_ERRORS}" \
    "${MAX_LATENCY_MS}" \
    "${SLO_GATE_RISK_ACCEPTED}" \
    "steady:$(profile_p99_threshold "${STEADY_P99_MS}"):${OUTPUT_DIR}/http/steady.json" \
    "burst:$(profile_p99_threshold "${BURST_P99_MS}"):${OUTPUT_DIR}/http/burst.json" \
    "ceiling:$(profile_p99_threshold "${CEILING_P99_MS}"):${OUTPUT_DIR}/http/ceiling.json" \
    "grpc-unary:$(profile_p99_threshold "${GRPC_P99_MS}"):${OUTPUT_DIR}/grpc/unary.json" \
    "tcp-steady:$(profile_p99_threshold "${TCP_P99_MS}"):${OUTPUT_DIR}/tcp/steady.json" \
    "udp-multi-client:$(profile_p99_threshold "${UDP_P99_MS}"):${OUTPUT_DIR}/udp/multi-client.json" \
    "udp-high-churn:$(profile_p99_threshold "${UDP_HIGH_CHURN_P99_MS}"):${OUTPUT_DIR}/udp/high-churn.json" \
    "udp-multi-upstream:$(profile_p99_threshold "${UDP_MULTI_UPSTREAM_P99_MS}"):${OUTPUT_DIR}/udp/multi-upstream.json" \
    "udp-backend-timeout:$(profile_p99_threshold "${UDP_BACKEND_TIMEOUT_P99_MS}"):${OUTPUT_DIR}/udp/backend-timeout.json" \
    "websocket-long-lived:$(profile_p99_threshold "${WEBSOCKET_P99_MS}"):${OUTPUT_DIR}/websocket/long-lived-streaming.json" \
    "sse-long-lived:$(profile_p99_threshold "${SSE_P99_MS}"):${OUTPUT_DIR}/sse/long-lived-streaming.json" \
    "mcp-streamable-http:$(profile_p99_threshold "${MCP_P99_MS}"):${OUTPUT_DIR}/mcp/streamable-http.json" \
    "backend-error:$(profile_p99_threshold "${BACKEND_ERROR_P99_MS}"):${OUTPUT_DIR}/http/backend-error.json" \
    "backend-slow-read:$(profile_p99_threshold "${BACKEND_SLOW_READ_P99_MS}"):${OUTPUT_DIR}/http/backend-slow-read.json" \
    "backend-slow-write:$(profile_p99_threshold "${BACKEND_SLOW_WRITE_P99_MS}"):${OUTPUT_DIR}/http/backend-slow-write.json" \
    "endpoint-flapping:$(profile_p99_threshold "${ENDPOINT_FLAPPING_P99_MS}"):${OUTPUT_DIR}/http/endpoint-flapping.json" \
    "live-reload-route-only:$(profile_p99_threshold "${RELOAD_P99_MS}"):${OUTPUT_DIR}/http/live-reload-route-only.json" \
    "live-reload-backend-only:$(profile_p99_threshold "${RELOAD_P99_MS}"):${OUTPUT_DIR}/http/live-reload-backend-only.json" \
    "live-reload-endpoint-only:$(profile_p99_threshold "${RELOAD_P99_MS}"):${OUTPUT_DIR}/tcp/live-reload-endpoint-only.json" \
    "live-reload-secret-only:$(profile_p99_threshold "${RELOAD_P99_MS}"):${OUTPUT_DIR}/udp/live-reload-secret-only.json" \
    "live-reload-tls-asset-rotation:$(profile_p99_threshold "${RELOAD_P99_MS}"):${OUTPUT_DIR}/grpc/live-reload-tls-asset-rotation.json" \
    "live-reload-listener-add-remove:$(profile_p99_threshold "${RELOAD_P99_MS}"):${OUTPUT_DIR}/tcp/live-reload-listener-add-remove.json"
}

slo_profile_rows() {
  jq -r '
    .profiles
    | to_entries[]
    | .key as $name
    | .value
    | "| \($name) | \(.status) | \(((.observed.success_rate * 10000)|floor/100)|tostring + "%") | \(.thresholds.min_success_rate * 100 | tostring + "%") | \(.observed.errors) | \(.thresholds.max_errors) | \(.observed.p99_ms) | \(.thresholds.max_p99_ms) | \(.observed.max_latency_ms) | \(.thresholds.max_latency_ms) |"
  ' "${SLO_OUTPUT}"
}

assert_slo_gate() {
  local status
  status="$(aeg_kind_slo_status "${SLO_OUTPUT}")"

  if [[ "${status}" == "fail" ]]; then
    log "SLO gate failed"
    exit 1
  fi
  if [[ "${status}" == "risk-accepted" ]]; then
    log "SLO gate is risk-accepted"
  fi
}

write_summary() {
  local summary="${OUTPUT_DIR}/summary.md"
  local slo_status
  slo_status="$(aeg_kind_slo_status "${SLO_OUTPUT}")"
  cat >"${summary}" <<EOF
# Kind A4 Baseline

- Run ID: \`${RUN_ID}\`
- Git commit: \`$(git -C "${ROOT_DIR}" rev-parse --short HEAD)\`
- Environment: local kind, host port \`${GATEWAY_HOST_PORT}\`, controlplane replicas \`$(kubectl --context kind-aether-gateway -n aether-gateway get deploy aether-gateway-controlplane -o jsonpath='{.status.readyReplicas}')\`, dataplane replicas \`$(kubectl --context kind-aether-gateway -n aether-gateway get deploy aether-gateway-dataplane -o jsonpath='{.status.readyReplicas}')\`
- SLO gate: \`${slo_status}\`

## HTTP Profiles

| Profile | Connection Mode | Requests | Concurrency | Success Rate | p95 ms | p99 ms | Max ms | Achieved RPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
$(profile_row steady "${OUTPUT_DIR}/http/steady.json")
$(profile_row burst "${OUTPUT_DIR}/http/burst.json")
$(profile_row ceiling "${OUTPUT_DIR}/http/ceiling.json")

## gRPC Profile

| Profile | Requests | Concurrency | Success Rate | p95 ms | p99 ms | Max ms | Achieved RPS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| unary | $(jq -r '.requests' "${OUTPUT_DIR}/grpc/unary.json") | $(jq -r '.concurrency' "${OUTPUT_DIR}/grpc/unary.json") | $(jq -r '((.success_rate * 10000)|floor/100) | tostring + "%"' "${OUTPUT_DIR}/grpc/unary.json") | $(jq -r '((.latency_ms.p95 * 100)|floor/100)' "${OUTPUT_DIR}/grpc/unary.json") | $(jq -r '((.latency_ms.p99 * 100)|floor/100)' "${OUTPUT_DIR}/grpc/unary.json") | $(jq -r '((.latency_ms.max * 100)|floor/100)' "${OUTPUT_DIR}/grpc/unary.json") | $(jq -r '((.achieved_rps * 100)|floor/100)' "${OUTPUT_DIR}/grpc/unary.json") |

## TCPRoute Profile

| Profile | Requests | Concurrency | Connection Count | Success Rate | p95 ms | p99 ms | p999 ms | Max ms | Achieved RPS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
$(stream_profile_row steady "${OUTPUT_DIR}/tcp/steady.json")

## UDPRoute Profiles

| Profile | Requests | Concurrency | Client Count | Upstreams | Expected Timeout | Session Opens | Packets Sent | Packets Received | Packets Lost | Success Rate | p95 ms | p99 ms | p999 ms | Achieved RPS |
| --- | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
$(udp_profile_row multi-client "${OUTPUT_DIR}/udp/multi-client.json")
$(udp_profile_row high-churn "${OUTPUT_DIR}/udp/high-churn.json")
$(udp_profile_row multi-upstream "${OUTPUT_DIR}/udp/multi-upstream.json")
$(udp_profile_row backend-timeout "${OUTPUT_DIR}/udp/backend-timeout.json")

## Long-Lived HTTP Profiles

| Profile | Requests | Concurrency | Success Rate | p95 ms | p99 ms | p999 ms | Max ms | Achieved RPS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
$(long_lived_profile_row websocket "${OUTPUT_DIR}/websocket/long-lived-streaming.json")
$(long_lived_profile_row sse "${OUTPUT_DIR}/sse/long-lived-streaming.json")
$(long_lived_profile_row mcp "${OUTPUT_DIR}/mcp/streamable-http.json")

## Fault Scenario Profiles

| Profile | Method | Status Counts | Requests | Concurrency | Success Rate | p95 ms | p99 ms | Max ms | Achieved RPS | Flapped Backend | Endpoint Mutation ms |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: |
$(fault_profile_row backend-error "${OUTPUT_DIR}/http/backend-error.json")
$(fault_profile_row backend-slow-read "${OUTPUT_DIR}/http/backend-slow-read.json")
$(fault_profile_row backend-slow-write "${OUTPUT_DIR}/http/backend-slow-write.json")
$(fault_profile_row endpoint-flapping "${OUTPUT_DIR}/http/endpoint-flapping.json")

## Live Reload Profiles

| Profile | Protocol | Mutation | Requests | Concurrency | Success Rate | p95 ms | p99 ms | p999 ms | Max ms | Achieved RPS | Mutation Apply ms |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
$(live_reload_profile_row route-only "${OUTPUT_DIR}/http/live-reload-route-only.json")
$(live_reload_profile_row backend-only "${OUTPUT_DIR}/http/live-reload-backend-only.json")
$(live_reload_profile_row endpoint-only "${OUTPUT_DIR}/tcp/live-reload-endpoint-only.json")
$(live_reload_profile_row secret-only "${OUTPUT_DIR}/udp/live-reload-secret-only.json")
$(live_reload_profile_row tls-asset-rotation "${OUTPUT_DIR}/grpc/live-reload-tls-asset-rotation.json")
$(live_reload_profile_row listener-add-remove "${OUTPUT_DIR}/tcp/live-reload-listener-add-remove.json")

## SLO Gate

| Profile | Status | Observed Success Rate | Min Success Rate | Observed Errors | Max Errors | Observed p99 ms | Max p99 ms | Observed max ms | Max latency ms |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
$(slo_profile_rows)

\`\`\`json
$(jq -S . "${SLO_OUTPUT}")
\`\`\`

## Resource Snapshot Delta

\`\`\`text
before:
$(cat "${OUTPUT_DIR}/resources/before.tsv")

after:
$(cat "${OUTPUT_DIR}/resources/after.tsv")
\`\`\`

## Specialized Checks

- \`tests/e2e/validate-upstream-behavior.sh\`: passed, see \`logs/upstream-behavior.log\`
- \`tests/e2e/validate-backend-protocols.sh\`: passed, see \`logs/backend-protocols.log\`

## Scoped Conclusion

- This run establishes a repeatable local-kind baseline, not a production global ceiling.
- Under this environment, the current head sustained the recorded HTTP and gRPC concurrency profiles without request loss, while upstream keepalive, retry, failover, h2c and WebSocket checks also passed.
- The attached \`admin-before\`, \`admin-after\`, \`metrics.prom\`, component logs and per-pod FD/RSS snapshots are the immutable evidence set for this run.
EOF
}

main() {
  require_command curl
  require_command docker
  require_command git
  require_command go
  require_command jq
  require_command kind
  require_command kubectl
  require_command openssl
  require_command python3
  require_command socat
  require_command ss

  trap cleanup EXIT
  ensure_stack
  TMP_DIR="$(mktemp -d "${ROOT_DIR}/tmp/kind-a4-baseline.XXXXXX")"
  GRPC_CLIENT_BIN="${TMP_DIR}/grpc-smoke-client"
  mkdir -p "${OUTPUT_DIR}"
  write_metadata
  collect_admin admin-before
  capture_resource_snapshot before

  log "building grpc smoke client"
  (
    cd "${ROOT_DIR}/controlplane"
    go build -o "${GRPC_CLIENT_BIN}" ./cmd/grpc-smoke-client
  )

  log "running http baseline profiles"
  run_http_profile steady "${STEADY_REQUESTS}" "${STEADY_CONCURRENCY}" "${STEADY_P99_MS}"
  run_http_profile burst "${BURST_REQUESTS}" "${BURST_CONCURRENCY}" "${BURST_P99_MS}"
  run_http_profile ceiling "${CEILING_REQUESTS}" "${CEILING_CONCURRENCY}" "${CEILING_P99_MS}"

  log "running grpc baseline profile"
  run_grpc_profile unary "${GRPC_REQUESTS}" "${GRPC_CONCURRENCY}" "${GRPC_P99_MS}"

  log "running TCPRoute baseline profile"
  run_tcp_profile steady "${TCP_REQUESTS}" "${TCP_CONCURRENCY}" "${TCP_P99_MS}"

  log "preparing UDPRoute multi-upstream and backend-timeout resources"
  ensure_udp_multi_upstream_resources
  ensure_udp_timeout_resources
  ensure_udp_timeout_forwarding

  log "running UDPRoute baseline profiles"
  run_udp_profile multi-client "${UDP_REQUESTS}" "${UDP_CONCURRENCY}" "${UDP_P99_MS}" per-worker
  run_udp_profile high-churn "${UDP_HIGH_CHURN_REQUESTS}" "${UDP_HIGH_CHURN_CONCURRENCY}" "${UDP_HIGH_CHURN_P99_MS}" per-request
  run_udp_profile multi-upstream \
    "${UDP_MULTI_UPSTREAM_REQUESTS}" \
    "${UDP_MULTI_UPSTREAM_CONCURRENCY}" \
    "${UDP_MULTI_UPSTREAM_P99_MS}" \
    per-worker \
    --upstream-count "${UDP_MULTI_UPSTREAM_OBSERVED_UPSTREAMS}"
  run_udp_backend_timeout_profile

  log "preparing A4 fault scenario resources"
  ensure_a4_fault_scenario_resources

  log "running A4 fault scenario profiles"
  run_fault_scenario_profiles

  log "preparing live reload resources"
  ensure_a4_reload_tls_resources

  log "running reload-under-load profiles"
  run_live_reload_profiles

  log "running specialized upstream/backend protocol checks"
  run_specialized_e2e
  write_slo_gate

  collect_admin admin-after
  capture_resource_snapshot after
  capture_component_logs
  write_summary

  if [[ "${FAILURES}" -ne 0 ]]; then
    log "completed with ${FAILURES} threshold failures"
    exit 1
  fi
  assert_slo_gate
  log "baseline evidence written to ${OUTPUT_DIR}"
}

main "$@"
