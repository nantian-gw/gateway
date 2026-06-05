#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
TEST_NAMESPACE="${TEST_NAMESPACE:-aether-backend-protocols}"
PLAIN_HOST="${PLAIN_HOST:-plain.example.com}"
GRPC_HOST="${GRPC_HOST:-grpc.example.com}"
WS_HOST="${WS_HOST:-ws.example.com}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
ADMIN_FORWARD_PORT="${ADMIN_FORWARD_PORT:-29080}"
PROFILE_OUTPUT_DIR="${PROFILE_OUTPUT_DIR:-}"
WEBSOCKET_PROFILE_REQUESTS="${WEBSOCKET_PROFILE_REQUESTS:-20}"
WEBSOCKET_PROFILE_CONCURRENCY="${WEBSOCKET_PROFILE_CONCURRENCY:-20}"
WEBSOCKET_PROFILE_HOLD_MS="${WEBSOCKET_PROFILE_HOLD_MS:-1000}"
WEBSOCKET_PROFILE_REQUEST_TIMEOUT="${WEBSOCKET_PROFILE_REQUEST_TIMEOUT:-5}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"
DATAPLANE_NAMESPACE="${DATAPLANE_NAMESPACE:-aether-gateway}"
PLAIN_SOURCE_IMAGE="${PLAIN_SOURCE_IMAGE:-m.daocloud.io/docker.io/hashicorp/http-echo:1.0.0}"
PLAIN_IMAGE="${PLAIN_IMAGE:-${LOCAL_REGISTRY_HOST}/hashicorp/http-echo:1.0.0}"
GRPC_SOURCE_IMAGE="${GRPC_SOURCE_IMAGE:-m.daocloud.io/gcr.io/k8s-staging-gateway-api/echo-basic:v20240412-v1.0.0-394-g40c666fd}"
GRPC_IMAGE="${GRPC_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-basic:v20240412-v1.0.0-394-g40c666fd}"
WS_SOURCE_IMAGE="${WS_SOURCE_IMAGE:-m.daocloud.io/docker.io/library/python:3.12-slim-bookworm}"
WS_IMAGE="${WS_IMAGE:-${LOCAL_REGISTRY_HOST}/aether-gateway-validation/python-ws:3.12-slim-bookworm}"

TMP_DIR=""
PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""
SUCCESS="false"

log() {
  printf '[backend-protocols] %s\n' "$*"
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
    SKIP_BUILD="${SKIP_BUILD:-true}" SKIP_SMOKE=true ./tests/e2e/run-kind.sh
  )
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
  local image

  log "preloading validation images into kind nodes via crictl"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    for image in "${PLAIN_IMAGE}" "${GRPC_IMAGE}" "${WS_IMAGE}"; do
      docker exec "${node}" crictl pull "${image}" >/dev/null
    done
  done
}

sync_test_images() {
  ensure_local_registry
  sync_image_to_local_registry "${PLAIN_SOURCE_IMAGE}" "${PLAIN_IMAGE}"
  sync_image_to_local_registry "${GRPC_SOURCE_IMAGE}" "${GRPC_IMAGE}"
  sync_image_to_local_registry "${WS_SOURCE_IMAGE}" "${WS_IMAGE}"
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
  k -n "${DATAPLANE_NAMESPACE}" port-forward service/aether-gateway-dataplane-admin "${ADMIN_FORWARD_PORT}:19080" \
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

dump_debug_state() {
  set +e
  printf '\n[backend-protocols] debug: gateway\n' >&2
  k -n "${TEST_NAMESPACE}" get gateway protocol-edge -o yaml >&2
  printf '\n[backend-protocols] debug: httproutes\n' >&2
  k -n "${TEST_NAMESPACE}" get httproute plain-route ws-route -o yaml >&2
  printf '\n[backend-protocols] debug: grpcroute\n' >&2
  k -n "${TEST_NAMESPACE}" get grpcroute grpc-route -o yaml >&2
  printf '\n[backend-protocols] debug: services\n' >&2
  k -n "${TEST_NAMESPACE}" get service plain-backend grpc-backend ws-backend -o yaml >&2
  printf '\n[backend-protocols] debug: endpointslices\n' >&2
  k -n "${TEST_NAMESPACE}" get endpointslices -o wide >&2
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    printf '\n[backend-protocols] debug: dataplane backends\n' >&2
    curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/backends?namespace=${TEST_NAMESPACE}" | jq '.' >&2
  fi
  if [[ -f "${PORT_FORWARD_LOG}" ]]; then
    printf '\n[backend-protocols] debug: port-forward log\n' >&2
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
  name: ws-backend-script
  namespace: ${TEST_NAMESPACE}
data:
  server.py: |
    import base64
    import hashlib
    import socketserver
    from http.server import BaseHTTPRequestHandler

    GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

    def recv_exact(stream, size):
        data = b""
        while len(data) < size:
            chunk = stream.read(size - len(data))
            if not chunk:
                raise ConnectionError("unexpected websocket EOF")
            data += chunk
        return data

    def recv_frame(stream):
        first, second = recv_exact(stream, 2)
        opcode = first & 0x0F
        masked = (second & 0x80) != 0
        length = second & 0x7F
        if length == 126:
            length = int.from_bytes(recv_exact(stream, 2), "big")
        elif length == 127:
            length = int.from_bytes(recv_exact(stream, 8), "big")
        mask = recv_exact(stream, 4) if masked else b""
        payload = recv_exact(stream, length)
        if masked:
            payload = bytes(payload[i] ^ mask[i % 4] for i in range(length))
        return opcode, payload

    def send_text_frame(stream, payload):
        if isinstance(payload, str):
            payload = payload.encode("utf-8")
        header = bytearray([0x81])
        length = len(payload)
        if length < 126:
            header.append(length)
        elif length < 65536:
            header.append(126)
            header.extend(length.to_bytes(2, "big"))
        else:
            header.append(127)
            header.extend(length.to_bytes(8, "big"))
        stream.write(header + payload)
        stream.flush()

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, format, *args):
            return

        def do_GET(self):
            if self.path == "/reject":
                body = b"websocket rejected"
                self.send_response(400)
                self.send_header("Content-Type", "text/plain")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return

            if self.path != "/ws":
                self.send_error(404)
                return

            key = self.headers.get("Sec-WebSocket-Key", "")
            upgrade = self.headers.get("Upgrade", "")
            connection = self.headers.get("Connection", "")
            if upgrade.lower() != "websocket" or "upgrade" not in connection.lower() or not key:
                self.send_error(400)
                return

            accept = base64.b64encode(
                hashlib.sha1((key + GUID).encode("utf-8")).digest()
            ).decode("ascii")
            self.send_response_only(101, "Switching Protocols")
            self.send_header("Upgrade", "websocket")
            self.send_header("Connection", "Upgrade")
            self.send_header("Sec-WebSocket-Accept", accept)
            self.end_headers()

            opcode, payload = recv_frame(self.rfile)
            if opcode == 0x8:
                return
            if opcode != 0x1:
                raise RuntimeError(f"unexpected opcode {opcode}")
            send_text_frame(self.wfile, payload)

    class ThreadedServer(socketserver.ThreadingMixIn, socketserver.TCPServer):
        allow_reuse_address = True

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
apiVersion: v1
kind: Service
metadata:
  name: plain-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: plain-backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: plain-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: plain-backend
  template:
    metadata:
      labels:
        app: plain-backend
    spec:
      containers:
        - name: echo
          image: ${PLAIN_IMAGE}
          imagePullPolicy: IfNotPresent
          args:
            - "-listen=:8080"
            - "-text=plain-backend"
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: grpc-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: grpc-backend
  ports:
    - name: grpc
      port: 8080
      targetPort: 3000
      appProtocol: kubernetes.io/h2c
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grpc-backend
  template:
    metadata:
      labels:
        app: grpc-backend
    spec:
      containers:
        - name: grpc-echo
          image: ${GRPC_IMAGE}
          imagePullPolicy: IfNotPresent
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: GRPC_ECHO_SERVER
              value: "1"
---
apiVersion: v1
kind: Service
metadata:
  name: ws-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: ws-backend
  ports:
    - name: ws
      port: 8080
      targetPort: 8080
      appProtocol: kubernetes.io/ws
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ws-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ws-backend
  template:
    metadata:
      labels:
        app: ws-backend
    spec:
      containers:
        - name: ws
          image: ${WS_IMAGE}
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
            name: ws-backend-script
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: protocol-edge
  namespace: ${TEST_NAMESPACE}
spec:
  gatewayClassName: aether
  listeners:
    - name: http
      protocol: HTTP
      port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: plain-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: protocol-edge
      sectionName: http
  hostnames:
    - ${PLAIN_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /plain
      backendRefs:
        - name: plain-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ws-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: protocol-edge
      sectionName: http
  hostnames:
    - ${WS_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /ws
      backendRefs:
        - name: ws-backend
          port: 8080
    - matches:
        - path:
            type: PathPrefix
            value: /reject
      backendRefs:
        - name: ws-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: grpc-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: protocol-edge
      sectionName: http
  hostnames:
    - ${GRPC_HOST}
  rules:
    - backendRefs:
        - name: grpc-backend
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

wait_for_gateway_ready() {
  for _ in $(seq 1 60); do
    if k -n "${TEST_NAMESPACE}" get gateway protocol-edge -o json \
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

  log "gateway ${TEST_NAMESPACE}/protocol-edge did not become ready"
  k -n "${TEST_NAMESPACE}" get gateway protocol-edge -o yaml >&2
  exit 1
}

wait_for_route_acceptance() {
  local kind="$1"
  local name="$2"

  for _ in $(seq 1 60); do
    if k -n "${TEST_NAMESPACE}" get "${kind}" "${name}" -o json \
      | jq -e '[.status.parents[]?.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0' \
      >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "route ${TEST_NAMESPACE}/${name} did not become accepted"
  k -n "${TEST_NAMESPACE}" get "${kind}" "${name}" -o yaml >&2
  exit 1
}

wait_for_admin_backends() {
  local payload

  for _ in $(seq 1 60); do
    payload="$(curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/backends?namespace=${TEST_NAMESPACE}" 2>/dev/null || true)"
    if [[ -z "${payload}" ]]; then
      sleep 1
      continue
    fi

    if jq -e '
      any(.[]; .name=="plain-backend:8080" and .protocol=="TCP" and ((.endpoints // []) | length > 0)) and
      any(.[]; .name=="grpc-backend:8080" and .protocol=="H2C" and ((.endpoints // []) | length > 0)) and
      any(.[]; .name=="ws-backend:8080" and .protocol=="HTTP" and ((.endpoints // []) | length > 0))
    ' <<<"${payload}" >/dev/null 2>&1
    then
      log "dataplane admin reports expected backend protocol mapping with resolved endpoints"
      jq '.[] | select(.namespace=="'"${TEST_NAMESPACE}"'") | {name, protocol, endpoint_count: ((.endpoints // []) | length)}' <<<"${payload}"
      return
    fi
    sleep 2
  done

  log "dataplane admin did not report expected backend protocols"
  curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/backends?namespace=${TEST_NAMESPACE}" | jq '.' >&2
  exit 1
}

validate_plain_http_backend() {
  local body

  for _ in $(seq 1 45); do
    body="$(
      curl -fsS \
        --http1.1 \
        --noproxy '*' \
        --resolve "${PLAIN_HOST}:${GATEWAY_HOST_PORT}:127.0.0.1" \
        "http://${PLAIN_HOST}:${GATEWAY_HOST_PORT}/plain"
    )"
    if [[ "${body}" == "plain-backend" ]]; then
      log "plain backend without appProtocol responded over HTTP/1.1"
      return
    fi
    sleep 2
  done

  log "plain backend validation failed"
  exit 1
}

validate_grpc_h2c_backend() {
  log "verifying grpc backend over cleartext h2"
  (
    cd "${ROOT_DIR}/controlplane"
    go run ./cmd/grpc-smoke-client \
      -addr "127.0.0.1:${GATEWAY_HOST_PORT}" \
      -authority "${GRPC_HOST}"
  )
}

validate_websocket_success() {
  log "verifying websocket upgrade success path"
  GATEWAY_PORT="${GATEWAY_HOST_PORT}" WS_HOSTNAME="${WS_HOST}" python3 - <<'PY'
import base64
import hashlib
import os
import socket

GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
host = os.environ["WS_HOSTNAME"]
port = int(os.environ["GATEWAY_PORT"])
payload = "aether-websocket"
key = base64.b64encode(b"aether-backend-protocols").decode("ascii")

def recv_exact(sock, size):
    data = b""
    while len(data) < size:
      chunk = sock.recv(size - len(data))
      if not chunk:
        raise RuntimeError("unexpected EOF")
      data += chunk
    return data

with socket.create_connection(("127.0.0.1", port), timeout=5) as sock:
    request = (
        f"GET /ws HTTP/1.1\r\n"
        f"Host: {host}\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: {key}\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        "\r\n"
    ).encode("ascii")
    sock.sendall(request)

    response = b""
    while b"\r\n\r\n" not in response:
        response += sock.recv(4096)
    header_block, remaining = response.split(b"\r\n\r\n", 1)
    header_text = header_block.decode("latin1")
    if not header_text.startswith("HTTP/1.1 101"):
        raise SystemExit(f"expected 101 upgrade, got:\n{header_text}")

    headers = {}
    for line in header_text.split("\r\n")[1:]:
        name, value = line.split(":", 1)
        headers[name.strip().lower()] = value.strip()

    expected = base64.b64encode(hashlib.sha1((key + GUID).encode("ascii")).digest()).decode("ascii")
    if headers.get("sec-websocket-accept") != expected:
        raise SystemExit("unexpected Sec-WebSocket-Accept")

    encoded = payload.encode("utf-8")
    mask = b"\x01\x02\x03\x04"
    masked = bytes(encoded[i] ^ mask[i % 4] for i in range(len(encoded)))
    frame = bytes([0x81, 0x80 | len(encoded)]) + mask + masked
    sock.sendall(frame)

    data = remaining
    while len(data) < 2:
        data += sock.recv(4096)
    first, second = data[0], data[1]
    opcode = first & 0x0F
    length = second & 0x7F
    offset = 2
    if length == 126:
        while len(data) < offset + 2:
            data += sock.recv(4096)
        length = int.from_bytes(data[offset:offset + 2], "big")
        offset += 2
    elif length == 127:
        while len(data) < offset + 8:
            data += sock.recv(4096)
        length = int.from_bytes(data[offset:offset + 8], "big")
        offset += 8
    while len(data) < offset + length:
        data += sock.recv(4096)
    payload_out = data[offset:offset + length].decode("utf-8")
    if opcode != 0x1 or payload_out != payload:
        raise SystemExit(f"unexpected websocket echo opcode={opcode} payload={payload_out!r}")
PY
}

validate_websocket_failure() {
  log "verifying websocket upgrade rejection path"
  GATEWAY_PORT="${GATEWAY_HOST_PORT}" WS_HOSTNAME="${WS_HOST}" python3 - <<'PY'
import os
import socket

host = os.environ["WS_HOSTNAME"]
port = int(os.environ["GATEWAY_PORT"])

with socket.create_connection(("127.0.0.1", port), timeout=5) as sock:
    request = (
        f"GET /reject HTTP/1.1\r\n"
        f"Host: {host}\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        "Sec-WebSocket-Key: cGluZ29yYS1yZWplY3Q=\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        "\r\n"
    ).encode("ascii")
    sock.sendall(request)
    response = b""
    while b"\r\n\r\n" not in response:
        chunk = sock.recv(4096)
        if not chunk:
            break
        response += chunk
    header_block, body = response.split(b"\r\n\r\n", 1)
    headers = {}
    for line in header_block.decode("latin1").split("\r\n")[1:]:
        if ":" in line:
            name, value = line.split(":", 1)
            headers[name.strip().lower()] = value.strip()
    content_length = int(headers.get("content-length", "0"))
    while len(body) < content_length:
        chunk = sock.recv(4096)
        if not chunk:
            break
        body += chunk
    text = header_block.decode("latin1") + "\r\n\r\n" + body.decode("latin1")
    if text.startswith("HTTP/1.1 101"):
        raise SystemExit(f"expected websocket rejection, got upgrade:\n{text}")
    if "websocket rejected" not in text:
        raise SystemExit(f"expected rejection body, got:\n{text}")
PY
}

write_websocket_profile() {
  local output

  if [[ -z "${PROFILE_OUTPUT_DIR}" ]]; then
    return
  fi

  output="${PROFILE_OUTPUT_DIR}/websocket/long-lived-streaming.json"
  mkdir -p "$(dirname "${output}")"
  log "writing websocket profile evidence to ${output}"
  python3 "${ROOT_DIR}/tests/e2e/websocket_concurrency_client.py" \
    --url "ws://127.0.0.1:${GATEWAY_HOST_PORT}/ws" \
    --host-header "${WS_HOST}" \
    --requests "${WEBSOCKET_PROFILE_REQUESTS}" \
    --concurrency "${WEBSOCKET_PROFILE_CONCURRENCY}" \
    --request-timeout "${WEBSOCKET_PROFILE_REQUEST_TIMEOUT}" \
    --hold-ms "${WEBSOCKET_PROFILE_HOLD_MS}" \
    --scenario long-lived-streaming \
    --output "${output}" >/dev/null

  jq -e '
    .completed == .requests
    and .successes == .requests
    and .upgrade_successes == .requests
    and .messages_received == .requests
  ' "${output}" >/dev/null
}

main() {
  require_command curl
  require_command docker
  require_command go
  require_command jq
  require_command kind
  require_command kubectl
  require_command python3

  ensure_kind_cluster
  sync_test_images
  TMP_DIR="$(mktemp -d "${ROOT_DIR}/tmp/backend-protocols.XXXXXX")"
  trap cleanup EXIT

  cleanup_namespace
  apply_resources
  wait_for_deployment plain-backend
  wait_for_deployment grpc-backend
  wait_for_deployment ws-backend
  wait_for_gateway_ready
  wait_for_route_acceptance httproute plain-route
  wait_for_route_acceptance httproute ws-route
  wait_for_route_acceptance grpcroute grpc-route
  start_admin_port_forward
  wait_for_admin_backends
  validate_plain_http_backend
  validate_grpc_h2c_backend
  validate_websocket_success
  validate_websocket_failure
  write_websocket_profile

  SUCCESS="true"
  log "backend protocol validation passed"
}

main "$@"
