#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
AETHER_NAMESPACE="${AETHER_NAMESPACE:-aether-gateway}"
TEST_NAMESPACE="${TEST_NAMESPACE:-aether-http-security}"
TEST_HOST="${TEST_HOST:-security.example.com}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
ADMIN_FORWARD_PORT="${ADMIN_FORWARD_PORT:-29080}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"
PYTHON_SOURCE_IMAGE="${PYTHON_SOURCE_IMAGE:-m.daocloud.io/docker.io/library/python:3.12-slim-bookworm}"
PYTHON_IMAGE="${PYTHON_IMAGE:-${LOCAL_REGISTRY_HOST}/aether-gateway-validation/python-ws:3.12-slim-bookworm}"
RAW_CLIENT="${ROOT_DIR}/tests/e2e/http_raw_client.py"
OVERSIZED_HEADER_BYTES="${OVERSIZED_HEADER_BYTES:-262144}"
SLOW_CONNECTIONS="${SLOW_CONNECTIONS:-8}"
SLOW_SEND_INTERVAL_MS="${SLOW_SEND_INTERVAL_MS:-120}"
SLOW_LINGER_MS="${SLOW_LINGER_MS:-4000}"
SLOW_REQUEST_MAX_TIME="${SLOW_REQUEST_MAX_TIME:-5}"
SLOW_REQUEST_MAX_SECONDS="${SLOW_REQUEST_MAX_SECONDS:-3.0}"

TMP_DIR=""
PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""
SUCCESS="false"

log() {
  printf '[http-security] %s\n' "$*"
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

aether_stack_ready() {
  k -n "${AETHER_NAMESPACE}" get deployment aether-gateway-controlplane aether-gateway-dataplane >/dev/null 2>&1
}

smoke_http_ready() {
  curl -fsS -H 'Host: example.com' "http://127.0.0.1:${GATEWAY_HOST_PORT}/" 2>/dev/null | grep -q "aether-gateway-ok"
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
    bootstrap_kind_stack
    return
  fi

  if ! aether_stack_ready || ! smoke_http_ready; then
    bootstrap_kind_stack
  fi
}

ensure_local_registry() {
  if ! docker inspect "${LOCAL_REGISTRY_NAME}" >/dev/null 2>&1; then
    log "local registry ${LOCAL_REGISTRY_NAME} is not running; run ./tests/e2e/run-kind.sh first"
    exit 1
  fi
}

sync_image_to_local_registry() {
  local source_image="$1"
  local target_image="$2"
  local push_image="${target_image}"

  if [[ "${target_image}" == "${LOCAL_REGISTRY_HOST}/"* ]]; then
    push_image="${LOCAL_REGISTRY_PUSH_HOST}/${target_image#${LOCAL_REGISTRY_HOST}/}"
  fi

  log "syncing ${source_image} -> ${push_image}"
  if ! docker image inspect "${source_image}" >/dev/null 2>&1; then
    docker pull "${source_image}" >/dev/null
  fi
  docker tag "${source_image}" "${push_image}"
  docker push "${push_image}" >/dev/null
}

preload_kind_images() {
  local node

  log "preloading validation image into kind nodes via crictl"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    docker exec "${node}" crictl pull "${PYTHON_IMAGE}" >/dev/null
  done
}

sync_test_images() {
  ensure_local_registry
  sync_image_to_local_registry "${PYTHON_SOURCE_IMAGE}" "${PYTHON_IMAGE}"
  preload_kind_images
}

cleanup_namespace() {
  if ! k get namespace "${TEST_NAMESPACE}" >/dev/null 2>&1; then
    return
  fi

  log "cleaning namespace ${TEST_NAMESPACE}"
  k delete namespace "${TEST_NAMESPACE}" --wait=false >/dev/null 2>&1 || true
  if ! timeout 120 bash -c \
    "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${TEST_NAMESPACE}' >/dev/null 2>&1; do sleep 2; done"
  then
    log "forcing cleanup for namespace ${TEST_NAMESPACE}"
    k -n "${TEST_NAMESPACE}" delete pod --all --force --grace-period=0 >/dev/null 2>&1 || true
    k get namespace "${TEST_NAMESPACE}" -o json \
      | jq '{apiVersion, kind, metadata: {name: .metadata.name}, spec: {finalizers: []}}' \
      | kubectl --context "${KUBE_CONTEXT}" replace --raw "/api/v1/namespaces/${TEST_NAMESPACE}/finalize" -f - >/dev/null 2>&1 || true

    if ! timeout 30 bash -c \
      "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${TEST_NAMESPACE}' >/dev/null 2>&1; do sleep 2; done"
    then
      log "namespace ${TEST_NAMESPACE} is still terminating after force cleanup"
      exit 1
    fi
  fi
}

port_listening() {
  local port="$1"

  ss -H -ltn "( sport = :${port} )" 2>/dev/null | grep -q .
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
  k -n "${AETHER_NAMESPACE}" port-forward service/aether-gateway-dataplane-admin "${ADMIN_FORWARD_PORT}:19080" \
    >"${PORT_FORWARD_LOG}" 2>&1 &
  PORT_FORWARD_PID="$!"

  for _ in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/livez" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  log "timed out waiting for dataplane admin port-forward"
  cat "${PORT_FORWARD_LOG}" >&2 || true
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

dump_debug_state() {
  set +e
  printf '\n[http-security] debug: gateway\n' >&2
  k -n "${TEST_NAMESPACE}" get gateway security-edge -o yaml >&2
  printf '\n[http-security] debug: route\n' >&2
  k -n "${TEST_NAMESPACE}" get httproute inspect-route -o yaml >&2
  printf '\n[http-security] debug: deployment\n' >&2
  k -n "${TEST_NAMESPACE}" get deployment inspect-backend -o yaml >&2
  printf '\n[http-security] debug: service\n' >&2
  k -n "${TEST_NAMESPACE}" get service inspect-backend -o yaml >&2
  printf '\n[http-security] debug: endpointslices\n' >&2
  k -n "${TEST_NAMESPACE}" get endpointslices -o wide >&2
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    printf '\n[http-security] debug: dataplane summary\n' >&2
    summary_json | jq '.' >&2
    printf '\n[http-security] debug: dataplane routes\n' >&2
    curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/routes?namespace=${TEST_NAMESPACE}" | jq '.' >&2
    printf '\n[http-security] debug: dataplane backends\n' >&2
    curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/backends?namespace=${TEST_NAMESPACE}" | jq '.' >&2
  fi
  if [[ -f "${PORT_FORWARD_LOG}" ]]; then
    printf '\n[http-security] debug: port-forward log\n' >&2
    cat "${PORT_FORWARD_LOG}" >&2
  fi
  set -e
}

cleanup() {
  local exit_code="$?"

  if [[ "${SUCCESS}" != "true" ]]; then
    dump_debug_state
  fi
  stop_admin_port_forward

  if [[ "${KEEP_RESOURCES}" != "true" ]]; then
    cleanup_namespace
  else
    log "keeping namespace ${TEST_NAMESPACE}"
  fi

  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi

  exit "${exit_code}"
}

render_resources() {
  cat >"${TMP_DIR}/resources.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${TEST_NAMESPACE}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: inspect-backend-script
  namespace: ${TEST_NAMESPACE}
data:
  server.py: |
    import json
    import socketserver
    import threading
    from http.server import BaseHTTPRequestHandler

    class ThreadedServer(socketserver.ThreadingMixIn, socketserver.TCPServer):
        allow_reuse_address = True
        daemon_threads = True

        def __init__(self, addr, handler):
            super().__init__(addr, handler)
            self._lock = threading.Lock()
            self._requests_total = 0
            self._recent_paths = []

        def record_request(self, path):
            with self._lock:
                self._requests_total += 1
                self._recent_paths.append(path)
                self._recent_paths = self._recent_paths[-20:]
                return self._requests_total

        def stats_snapshot(self):
            with self._lock:
                return {
                    "requests_total": self._requests_total,
                    "recent_paths": list(self._recent_paths),
                }

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, format, *args):
            return

        def _discard_body(self):
            length = self.headers.get("Content-Length")
            if not length:
                return
            try:
                remaining = int(length)
            except ValueError:
                return
            while remaining > 0:
                chunk = self.rfile.read(min(65536, remaining))
                if not chunk:
                    return
                remaining -= len(chunk)

        def _send_json(self, payload):
            body = json.dumps(payload, sort_keys=True).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def _handle(self):
            if self.path == "/stats":
                self._send_json(self.server.stats_snapshot())
                return

            self._discard_body()
            request_number = self.server.record_request(self.path)
            headers = {}
            for key in self.headers.keys():
                headers[key.lower()] = self.headers.get_all(key) or []

            self._send_json(
                {
                    "headers": headers,
                    "host": self.headers.get("Host"),
                    "method": self.command,
                    "path": self.path,
                    "request_number": request_number,
                }
            )

        def do_GET(self):
            self._handle()

        def do_POST(self):
            self._handle()

    if __name__ == "__main__":
        server = ThreadedServer(("0.0.0.0", 8080), Handler)
        server.serve_forever()
---
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: aether
spec:
  controllerName: gateway.networking.k8s.io/aether-gateway
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: inspect-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: inspect-backend
  template:
    metadata:
      labels:
        app: inspect-backend
    spec:
      containers:
        - name: inspect
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: inspect-backend-script
---
apiVersion: v1
kind: Service
metadata:
  name: inspect-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: inspect-backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: security-edge
  namespace: ${TEST_NAMESPACE}
spec:
  gatewayClassName: aether
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      hostname: ${TEST_HOST}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: inspect-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: security-edge
      sectionName: http
  hostnames:
    - ${TEST_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: inspect-backend
          port: 8080
EOF
}

apply_resources() {
  render_resources
  k apply -f "${TMP_DIR}/resources.yaml" >/dev/null
}

wait_for_deployment() {
  local name="$1"
  k -n "${TEST_NAMESPACE}" rollout status deployment/"${name}" --timeout=180s >/dev/null
}

wait_for_service_endpoints() {
  local name="$1"
  local expected="$2"
  local ready=""

  for _ in $(seq 1 60); do
    ready="$(
      k -n "${TEST_NAMESPACE}" get endpointslices -l "kubernetes.io/service-name=${name}" -o json \
        | jq '[.items[]?.endpoints[]? | select(.conditions.ready == true)] | length'
    )"
    if [[ "${ready}" -ge "${expected}" ]]; then
      return
    fi
    sleep 2
  done

  log "service ${TEST_NAMESPACE}/${name} did not publish ${expected} ready endpoints"
  k -n "${TEST_NAMESPACE}" get endpointslices -o yaml >&2
  exit 1
}

wait_for_gateway_ready() {
  for _ in $(seq 1 60); do
    if k -n "${TEST_NAMESPACE}" get gateway security-edge -o json \
      | jq -e '
          ( [.status.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0 )
          and
          ( [.status.conditions[]? | select(.type=="Programmed" and .status=="True")] | length > 0 )
        ' >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "gateway ${TEST_NAMESPACE}/security-edge did not become ready"
  k -n "${TEST_NAMESPACE}" get gateway security-edge -o yaml >&2
  exit 1
}

wait_for_route_acceptance() {
  for _ in $(seq 1 60); do
    if k -n "${TEST_NAMESPACE}" get httproute inspect-route -o json \
      | jq -e '[.status.parents[]?.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0' \
      >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "route ${TEST_NAMESPACE}/inspect-route did not become accepted"
  k -n "${TEST_NAMESPACE}" get httproute inspect-route -o yaml >&2
  exit 1
}

inspect_json() {
  local path="$1"
  shift
  curl -fsS -H "Host: ${TEST_HOST}" "$@" "http://127.0.0.1:${GATEWAY_HOST_PORT}${path}"
}

backend_stats_json() {
  inspect_json "/stats"
}

backend_request_total() {
  backend_stats_json | jq -r '.requests_total'
}

assert_inspect_response() {
  local label="$1"
  local path="$2"
  local payload="$3"

  if ! jq -e --arg path "${path}" --arg host "${TEST_HOST}" '
    .path == $path
    and
    .host == $host
    and
    (.headers.host[0] == $host)
  ' <<<"${payload}" >/dev/null 2>&1
  then
    log "${label} returned unexpected payload"
    printf '%s\n' "${payload}" >&2
    exit 1
  fi
}

assert_clean_request() {
  local path="$1"
  local payload=""

  for _ in $(seq 1 30); do
    payload="$(inspect_json "${path}" 2>/dev/null || true)"
    if [[ -n "${payload}" ]] && jq -e --arg path "${path}" --arg host "${TEST_HOST}" '
      .path == $path
      and
      .host == $host
      and
      (.headers.host[0] == $host)
    ' <<<"${payload}" >/dev/null 2>&1
    then
      printf '%s\n' "${payload}"
      return
    fi
    sleep 1
  done

  log "clean request ${path} did not return the expected inspect payload"
  curl -i -H "Host: ${TEST_HOST}" "http://127.0.0.1:${GATEWAY_HOST_PORT}${path}" >&2 || true
  exit 1
}

assert_request_count() {
  local label="$1"
  local expected="$2"
  local actual="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    log "${label}: expected request count ${expected}, got ${actual}"
    backend_stats_json | jq '.' >&2
    exit 1
  fi
}

write_payload() {
  local name="$1"
  local path="${TMP_DIR}/${name}"

  python3 - "${path}" <<'PY'
import sys

path = sys.argv[1]
content = sys.stdin.read()
with open(path, "wb") as fh:
    fh.write(content.replace("\n", "\r\n").encode("latin1"))
PY
  printf '%s\n' "${path}"
}

run_raw_exchange() {
  local payload_file="$1"
  local result_file="$2"

  python3 "${RAW_CLIENT}" \
    --host 127.0.0.1 \
    --port "${GATEWAY_HOST_PORT}" \
    --payload-file "${payload_file}" \
    >"${result_file}"
}

validate_baseline() {
  local before_count
  local after_count
  local payload

  before_count="$(backend_request_total)"
  payload="$(assert_clean_request "/inspect/baseline")"
  assert_inspect_response "baseline" "/inspect/baseline" "${payload}"
  after_count="$(backend_request_total)"
  assert_request_count "baseline request" "$((before_count + 1))" "${after_count}"
  log "baseline inspection path is healthy"
}

validate_header_spoofing() {
  local before_count
  local denied_count
  local denied_code
  local payload
  local after_count

  before_count="$(backend_request_total)"
  denied_code="$(
    curl -sS -o /dev/null -w '%{http_code}' \
      -H "Host: attacker.example.com" \
      -H "X-Forwarded-Host: ${TEST_HOST}" \
      -H "X-Forwarded-Proto: https" \
      -H "X-Forwarded-For: 198.51.100.24" \
      "http://127.0.0.1:${GATEWAY_HOST_PORT}/inspect/spoof-denied" || true
  )"
  denied_count="$(backend_request_total)"
  assert_request_count "spoofed missing-host request" "${before_count}" "${denied_count}"
  if [[ "${denied_code}" == "200" ]]; then
    log "spoofed Host/X-Forwarded-* request unexpectedly returned 200"
    exit 1
  fi

  payload="$(
    inspect_json "/inspect/spoof-allowed" \
      -H "X-Forwarded-Host: attacker.example.com" \
      -H "X-Forwarded-Proto: https" \
      -H "X-Forwarded-For: 198.51.100.24"
  )"
  if ! jq -e --arg path "/inspect/spoof-allowed" --arg host "${TEST_HOST}" '
    .path == $path
    and
    .host == $host
    and
    (.headers.host[0] == $host)
    and
    (.headers["x-forwarded-host"][0] == "attacker.example.com")
    and
    (.headers["x-forwarded-proto"][0] == "https")
    and
    (.headers["x-forwarded-for"][0] == "198.51.100.24")
  ' <<<"${payload}" >/dev/null 2>&1
  then
    log "forwarded-header spoof sanity payload was unexpected"
    printf '%s\n' "${payload}" >&2
    exit 1
  fi
  after_count="$(backend_request_total)"
  assert_request_count "spoof sanity success path" "$((before_count + 1))" "${after_count}"
  log "Host and X-Forwarded-* spoofing checks passed"
}

validate_raw_rejection_case() {
  local label="$1"
  local payload_file="$2"
  local before_count
  local after_count
  local final_count
  local result_file="${TMP_DIR}/${label}.json"

  before_count="$(backend_request_total)"
  run_raw_exchange "${payload_file}" "${result_file}"
  after_count="$(backend_request_total)"
  assert_request_count "${label} malformed request" "${before_count}" "${after_count}"
  assert_clean_request "/inspect/after-${label}" >/dev/null
  final_count="$(backend_request_total)"
  assert_request_count "${label} follow-up clean request" "$((after_count + 1))" "${final_count}"
  log "${label} regression passed"
}

validate_duplicate_host() {
  local payload_file

  payload_file="$(
    write_payload "duplicate-host.raw" <<EOF
GET /inspect/duplicate-host HTTP/1.1
Host: ${TEST_HOST}
Host: attacker.example.com
Connection: close

EOF
  )"
  validate_raw_rejection_case "duplicate-host" "${payload_file}"
}

validate_conflicting_content_length() {
  local payload_file

  payload_file="$(
    write_payload "conflicting-content-length.raw" <<EOF
POST /inspect/conflicting-content-length HTTP/1.1
Host: ${TEST_HOST}
Connection: close
Content-Length: 0
Content-Length: 4

PING
EOF
  )"
  validate_raw_rejection_case "conflicting-content-length" "${payload_file}"
}

validate_cl_te_smuggling() {
  local payload_file

  payload_file="$(
    write_payload "clte.raw" <<EOF
POST /inspect/clte HTTP/1.1
Host: ${TEST_HOST}
Connection: keep-alive
Content-Length: 4
Transfer-Encoding: chunked

0

GET /inspect/poison-clte HTTP/1.1
Host: attacker.example.com
Connection: close

EOF
  )"
  validate_raw_rejection_case "clte" "${payload_file}"
}

validate_te_cl_smuggling() {
  local payload_file

  payload_file="$(
    write_payload "tecl.raw" <<EOF
POST /inspect/tecl HTTP/1.1
Host: ${TEST_HOST}
Connection: keep-alive
Transfer-Encoding: chunked
Content-Length: 4

0

GET /inspect/poison-tecl HTTP/1.1
Host: attacker.example.com
Connection: close

EOF
  )"
  validate_raw_rejection_case "tecl" "${payload_file}"
}

validate_malformed_chunked() {
  local payload_file

  payload_file="$(
    write_payload "malformed-chunked.raw" <<EOF
POST /inspect/malformed-chunked HTTP/1.1
Host: ${TEST_HOST}
Connection: close
Transfer-Encoding: chunked

Z
oops
0

EOF
  )"
  validate_raw_rejection_case "malformed-chunked" "${payload_file}"
}

validate_oversized_header() {
  local payload_file="${TMP_DIR}/oversized-header.raw"

  python3 - "${payload_file}" "${TEST_HOST}" "${OVERSIZED_HEADER_BYTES}" <<'PY'
import sys

path = sys.argv[1]
host = sys.argv[2]
header_bytes = int(sys.argv[3])
oversized = "a" * header_bytes
payload = (
    f"GET /inspect/oversized-header HTTP/1.1\r\n"
    f"Host: {host}\r\n"
    f"X-Oversized: {oversized}\r\n"
    "Connection: close\r\n"
    "\r\n"
).encode("ascii")
with open(path, "wb") as fh:
    fh.write(payload)
PY
  validate_raw_rejection_case "oversized-header" "${payload_file}"
}

validate_slow_header_probe() {
  local partial_file
  local before_count
  local mid_count
  local after_count
  local body_file="${TMP_DIR}/slowloris-clean.json"
  local timing_file="${TMP_DIR}/slowloris-clean.time"
  local curl_code
  local curl_time
  local output=""
  local slow_pids=()
  local pid

  partial_file="$(
    write_payload "slow-header.partial" <<EOF
GET /inspect/slow-header HTTP/1.1
Host: ${TEST_HOST}
X-Slow: this-header-never-finishes
EOF
  )"

  before_count="$(backend_request_total)"
  for _ in $(seq 1 "${SLOW_CONNECTIONS}"); do
    python3 "${RAW_CLIENT}" \
      --host 127.0.0.1 \
      --port "${GATEWAY_HOST_PORT}" \
      --payload-file "${partial_file}" \
      --send-chunk-size 1 \
      --send-interval-ms "${SLOW_SEND_INTERVAL_MS}" \
      --linger-ms "${SLOW_LINGER_MS}" \
      --skip-read \
      --no-shutdown \
      >/dev/null 2>&1 &
    slow_pids+=("$!")
  done

  sleep 1
  mid_count="$(backend_request_total)"
  assert_request_count "slow-header partial requests" "${before_count}" "${mid_count}"

  output="$(
    curl -sS \
      --max-time "${SLOW_REQUEST_MAX_TIME}" \
      -o "${body_file}" \
      -w '%{http_code} %{time_total}' \
      -H "Host: ${TEST_HOST}" \
      "http://127.0.0.1:${GATEWAY_HOST_PORT}/inspect/slowloris-clean"
  )"
  curl_code="${output%% *}"
  curl_time="${output##* }"
  if [[ "${curl_code}" != "200" ]]; then
    log "clean request under slow-header pressure returned ${curl_code}"
    cat "${body_file}" >&2 || true
    exit 1
  fi
  if ! awk -v total="${curl_time}" -v max="${SLOW_REQUEST_MAX_SECONDS}" 'BEGIN { exit !(total <= max) }'; then
    log "clean request under slow-header pressure took ${curl_time}s, expected <= ${SLOW_REQUEST_MAX_SECONDS}s"
    exit 1
  fi
  if ! jq -e --arg path "/inspect/slowloris-clean" --arg host "${TEST_HOST}" '
    .path == $path
    and
    .host == $host
    and
    (.headers.host[0] == $host)
  ' "${body_file}" >/dev/null 2>&1
  then
    log "clean request under slow-header pressure returned unexpected payload"
    cat "${body_file}" >&2
    exit 1
  fi

  for pid in "${slow_pids[@]}"; do
    wait "${pid}" || true
  done

  after_count="$(backend_request_total)"
  assert_request_count "slow-header clean request count" "$((before_count + 1))" "${after_count}"
  printf '%s\n' "${curl_time}" >"${timing_file}"
  log "slow-header probe passed in ${curl_time}s with ${SLOW_CONNECTIONS} concurrent partial connections"
}

main() {
  require_command awk
  require_command curl
  require_command docker
  require_command jq
  require_command kind
  require_command kubectl
  require_command python3
  require_command ss

  ensure_kind_stack
  sync_test_images
  TMP_DIR="$(mktemp -d "${ROOT_DIR}/tmp/http-security.XXXXXX")"
  trap cleanup EXIT

  cleanup_namespace
  apply_resources
  wait_for_deployment inspect-backend
  wait_for_service_endpoints inspect-backend 1
  wait_for_gateway_ready
  wait_for_route_acceptance
  start_admin_port_forward

  validate_baseline
  validate_header_spoofing
  validate_duplicate_host
  validate_conflicting_content_length
  validate_cl_te_smuggling
  validate_te_cl_smuggling
  validate_malformed_chunked
  validate_oversized_header
  validate_slow_header_probe

  SUCCESS="true"
  log "http security validation passed"
}

main "$@"
