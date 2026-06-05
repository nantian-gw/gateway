#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
PYTHON_SOURCE_IMAGE="${PYTHON_SOURCE_IMAGE:-m.daocloud.io/docker.io/library/python:3.12-slim-bookworm}"
PYTHON_IMAGE="${PYTHON_IMAGE:-${LOCAL_REGISTRY_HOST}/aether-gateway-validation/python-tls:3.12-slim-bookworm}"
TEST_NAMESPACE="${TEST_NAMESPACE:-aether-tls-assets}"
CONTROLPLANE_NAMESPACE="${CONTROLPLANE_NAMESPACE:-aether-gateway}"
DATAPLANE_NAMESPACE="${DATAPLANE_NAMESPACE:-aether-gateway}"
CONTROLPLANE_SELECTOR="${CONTROLPLANE_SELECTOR:-app=aether-gateway-controlplane}"
DATAPLANE_SELECTOR="${DATAPLANE_SELECTOR:-app=aether-gateway-dataplane}"
HTTPS_HOST_PORT="${HTTPS_HOST_PORT:-18443}"
TEST_HOST="${TEST_HOST:-tls-assets.example.com}"
BACKEND_TLS_HOSTNAME="${BACKEND_TLS_HOSTNAME:-tls-backend.${TEST_NAMESPACE}.svc}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"

TMP_DIR=""
SUCCESS="false"
SMOKE_GATEWAY_SUSPENDED="false"

log() {
  printf '[tls-asset-rotation] %s\n' "$*"
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

preload_kind_image() {
  local node

  log "preloading validation image into kind nodes via crictl"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    docker exec "${node}" crictl pull "${PYTHON_IMAGE}" >/dev/null
  done
}

sync_test_image() {
  ensure_local_registry
  sync_image_to_local_registry "${PYTHON_SOURCE_IMAGE}" "${PYTHON_IMAGE}"
  preload_kind_image
}

cleanup_namespace() {
  if ! k get namespace "${TEST_NAMESPACE}" >/dev/null 2>&1; then
    return
  fi

  log "cleaning namespace ${TEST_NAMESPACE}"
  k delete namespace "${TEST_NAMESPACE}" --wait=false >/dev/null 2>&1 || true
  if ! timeout 120 bash -c "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${TEST_NAMESPACE}' >/dev/null 2>&1; do sleep 2; done"; then
    log "namespace ${TEST_NAMESPACE} is still terminating"
    exit 1
  fi
}

suspend_conflicting_smoke_gateway() {
  if ! k -n aether-gateway get gateway edge >/dev/null 2>&1; then
    return
  fi

  log "temporarily deleting smoke gateway to avoid shared HTTPS bind conflicts"
  k -n aether-gateway delete gateway edge --ignore-not-found >/dev/null
  SMOKE_GATEWAY_SUSPENDED="true"
}

debug_dump() {
  set +e
  printf '\n[tls-asset-rotation] debug: gateway\n' >&2
  k -n "${TEST_NAMESPACE}" get gateway tls-assets-edge -o yaml >&2
  printf '\n[tls-asset-rotation] debug: route\n' >&2
  k -n "${TEST_NAMESPACE}" get httproute tls-assets-route -o yaml >&2
  printf '\n[tls-asset-rotation] debug: BackendTLSPolicy\n' >&2
  k -n "${TEST_NAMESPACE}" get backendtlspolicy tls-backend-validation -o yaml >&2
  printf '\n[tls-asset-rotation] debug: services/endpoints\n' >&2
  k -n "${TEST_NAMESPACE}" get service,endpointslice -o wide >&2
  printf '\n[tls-asset-rotation] debug: dataplane pods\n' >&2
  k -n "${DATAPLANE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o wide >&2
  set -e
}

cleanup() {
  local exit_code="$?"

  if [[ "${SUCCESS}" != "true" ]]; then
    debug_dump
  fi
  if [[ "${KEEP_RESOURCES}" != "true" ]]; then
    cleanup_namespace
  else
    log "keeping namespace ${TEST_NAMESPACE}"
  fi
  if [[ "${SMOKE_GATEWAY_SUSPENDED}" == "true" ]]; then
    log "restoring smoke gateway resources"
    k apply -f "${ROOT_DIR}/tests/e2e/smoke.yaml" >/dev/null 2>&1 || true
  fi
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi

  exit "${exit_code}"
}

generate_ca() {
  local prefix="$1"

  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "${TMP_DIR}/${prefix}-ca.key" \
    -out "${TMP_DIR}/${prefix}-ca.crt" \
    -days 2 \
    -subj "/CN=${prefix}-ca" \
    -addext "basicConstraints=critical,CA:TRUE" \
    -addext "keyUsage=critical,keyCertSign,cRLSign" >/dev/null 2>&1
}

generate_signed_cert() {
  local prefix="$1"
  local ca_prefix="$2"
  local cn="$3"
  local usage="$4"
  local san="$5"
  local ext_file="${TMP_DIR}/${prefix}.ext"

  openssl req -nodes -newkey rsa:2048 \
    -keyout "${TMP_DIR}/${prefix}.key" \
    -out "${TMP_DIR}/${prefix}.csr" \
    -subj "/CN=${cn}" >/dev/null 2>&1

  {
    printf 'basicConstraints=critical,CA:FALSE\n'
    printf 'keyUsage=critical,digitalSignature,keyEncipherment\n'
    printf 'extendedKeyUsage=%s\n' "${usage}"
    if [[ -n "${san}" ]]; then
      printf 'subjectAltName=%s\n' "${san}"
    fi
  } >"${ext_file}"

  openssl x509 -req \
    -in "${TMP_DIR}/${prefix}.csr" \
    -CA "${TMP_DIR}/${ca_prefix}-ca.crt" \
    -CAkey "${TMP_DIR}/${ca_prefix}-ca.key" \
    -CAcreateserial \
    -out "${TMP_DIR}/${prefix}.crt" \
    -days 2 \
    -sha256 \
    -extfile "${ext_file}" >/dev/null 2>&1
}

generate_gateway_cert() {
  local prefix="$1"
  local cn="$2"

  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "${TMP_DIR}/${prefix}.key" \
    -out "${TMP_DIR}/${prefix}.crt" \
    -days 2 \
    -subj "/CN=${cn}" \
    -addext "subjectAltName=DNS:${TEST_HOST}" >/dev/null 2>&1
}

generate_cert_material() {
  generate_gateway_cert gateway-initial gateway-initial
  generate_gateway_cert gateway-rotated gateway-rotated
  generate_ca backend-server-initial
  generate_ca backend-server-rotated
  generate_ca backend-client-initial
  generate_ca backend-client-rotated
  generate_signed_cert backend-server-initial backend-server-initial "${BACKEND_TLS_HOSTNAME}" serverAuth "DNS:${BACKEND_TLS_HOSTNAME}"
  generate_signed_cert backend-server-rotated backend-server-rotated "${BACKEND_TLS_HOSTNAME}" serverAuth "DNS:${BACKEND_TLS_HOSTNAME}"
  generate_signed_cert backend-client-initial backend-client-initial backend-client-initial clientAuth ""
  generate_signed_cert backend-client-rotated backend-client-rotated backend-client-rotated clientAuth ""
}

apply_gateway_cert_secret() {
  local prefix="$1"

  k -n "${TEST_NAMESPACE}" create secret tls gateway-cert \
    --cert="${TMP_DIR}/${prefix}.crt" \
    --key="${TMP_DIR}/${prefix}.key" \
    --dry-run=client \
    -o yaml | k apply -f - >/dev/null
}

apply_backend_client_secret() {
  local prefix="$1"

  k -n "${TEST_NAMESPACE}" create secret tls backend-client-cert \
    --cert="${TMP_DIR}/${prefix}.crt" \
    --key="${TMP_DIR}/${prefix}.key" \
    --dry-run=client \
    -o yaml | k apply -f - >/dev/null
}

apply_backend_ca_configmap() {
  local prefix="$1"

  k -n "${TEST_NAMESPACE}" create configmap backend-ca \
    --from-file=ca.crt="${TMP_DIR}/${prefix}-ca.crt" \
    --dry-run=client \
    -o yaml | k apply -f - >/dev/null
}

apply_backend_server_secret() {
  local variant="$1"

  k -n "${TEST_NAMESPACE}" create secret generic "backend-server-${variant}" \
    --from-file=tls.crt="${TMP_DIR}/backend-server-${variant}.crt" \
    --from-file=tls.key="${TMP_DIR}/backend-server-${variant}.key" \
    --from-file=client-ca.crt="${TMP_DIR}/backend-client-${variant}-ca.crt" \
    --dry-run=client \
    -o yaml | k apply -f - >/dev/null
}

render_resources() {
  cat >"${TMP_DIR}/resources.yaml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: tls-backend-script
  namespace: ${TEST_NAMESPACE}
data:
  server.py: |
    import os
    import ssl
    from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

    RESPONSE_BODY = os.environ.get("RESPONSE_BODY", "backend")

    def client_cn(cert):
        for item in cert.get("subject", ()):
            for key, value in item:
                if key == "commonName":
                    return value
        return ""

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, format, *args):
            return

        def do_GET(self):
            if self.path != "/tls":
                self.send_error(404)
                return
            peer = self.connection.getpeercert()
            body = f"{RESPONSE_BODY} client-cn={client_cn(peer)}".encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    if __name__ == "__main__":
        context = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
        context.load_cert_chain("/tls/tls.crt", "/tls/tls.key")
        context.load_verify_locations(cafile="/tls/client-ca.crt")
        context.verify_mode = ssl.CERT_REQUIRED
        server = ThreadingHTTPServer(("0.0.0.0", 8443), Handler)
        server.socket = context.wrap_socket(server.socket, server_side=True)
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
  name: tls-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: tls-backend
    version: initial
  ports:
    - name: https
      port: 8443
      targetPort: 8443
      appProtocol: https
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tls-backend-initial
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tls-backend
      version: initial
  template:
    metadata:
      labels:
        app: tls-backend
        version: initial
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: backend-initial
          ports:
            - containerPort: 8443
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
            - name: tls
              mountPath: /tls
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: tls-backend-script
        - name: tls
          secret:
            secretName: backend-server-initial
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tls-backend-rotated
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tls-backend
      version: rotated
  template:
    metadata:
      labels:
        app: tls-backend
        version: rotated
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: backend-rotated
          ports:
            - containerPort: 8443
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
            - name: tls
              mountPath: /tls
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: tls-backend-script
        - name: tls
          secret:
            secretName: backend-server-rotated
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: tls-assets-edge
  namespace: ${TEST_NAMESPACE}
spec:
  gatewayClassName: aether
  tls:
    backend:
      clientCertificateRef:
        group: ""
        kind: Secret
        name: backend-client-cert
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
      hostname: ${TEST_HOST}
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: gateway-cert
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: tls-assets-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: tls-assets-edge
      sectionName: https
  hostnames:
    - ${TEST_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /tls
      backendRefs:
        - name: tls-backend
          port: 8443
---
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: tls-backend-validation
  namespace: ${TEST_NAMESPACE}
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: tls-backend
      sectionName: https
  validation:
    hostname: ${BACKEND_TLS_HOSTNAME}
    caCertificateRefs:
      - group: ""
        kind: ConfigMap
        name: backend-ca
    subjectAltNames:
      - type: Hostname
        hostname: ${BACKEND_TLS_HOSTNAME}
EOF
}

apply_resources() {
  k create namespace "${TEST_NAMESPACE}" >/dev/null
  apply_gateway_cert_secret gateway-initial
  apply_backend_client_secret backend-client-initial
  apply_backend_ca_configmap backend-server-initial
  apply_backend_server_secret initial
  apply_backend_server_secret rotated
  render_resources
  k apply -f "${TMP_DIR}/resources.yaml" >/dev/null
}

ready_pods() {
  local namespace="$1"
  local selector="$2"

  k -n "${namespace}" get pod -l "${selector}" -o json \
    | jq -r '.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | .metadata.name' \
    | sort
}

capture_pod_identity() {
  local namespace="$1"
  local selector="$2"
  local output="$3"

  k -n "${namespace}" get pod -l "${selector}" -o json \
    | jq -S '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | {name: .metadata.name, uid: .metadata.uid, restartCounts: ([.status.containerStatuses[]? | {name: .name, restartCount: .restartCount}] | sort_by(.name))}] | sort_by(.name)' \
    >"${output}"
}

assert_pod_identity_unchanged() {
  local namespace="$1"
  local selector="$2"
  local before="$3"
  local current="${before}.current"

  capture_pod_identity "${namespace}" "${selector}" "${current}"
  if ! diff -u "${before}" "${current}"; then
    log "pods matching ${namespace}/${selector} changed during TLS asset rotation"
    exit 1
  fi
}

wait_for_deployment() {
  local name="$1"
  k -n "${TEST_NAMESPACE}" rollout status deployment/"${name}" --timeout=180s >/dev/null
}

wait_for_gateway_ready() {
  for _ in $(seq 1 90); do
    if k -n "${TEST_NAMESPACE}" get gateway tls-assets-edge -o json \
      | jq -e '
          ([.status.listeners[]?.conditions[]? | select(.type=="Programmed" and .status=="True")] | length > 0)
          and
          ([.status.listeners[]?.conditions[]? | select(.type=="ResolvedRefs" and .status=="True")] | length > 0)
        ' >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "gateway did not become ready"
  k -n "${TEST_NAMESPACE}" get gateway tls-assets-edge -o yaml >&2
  exit 1
}

wait_for_route_accepted() {
  for _ in $(seq 1 90); do
    if k -n "${TEST_NAMESPACE}" get httproute tls-assets-route -o json \
      | jq -e '[.status.parents[]?.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0' \
      >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "HTTPRoute did not become accepted"
  k -n "${TEST_NAMESPACE}" get httproute tls-assets-route -o yaml >&2
  exit 1
}

wait_for_service_endpoints() {
  local version="$1"

  for _ in $(seq 1 60); do
    if [[ "$(ready_pods "${TEST_NAMESPACE}" "app=tls-backend,version=${version}" | wc -l | tr -d ' ')" -ge 1 ]] \
      && [[ "$(
        k -n "${TEST_NAMESPACE}" get endpointslice -l kubernetes.io/service-name=tls-backend -o json \
          | jq '[.items[].endpoints[]? | select(.conditions.ready != false)] | length'
      )" -ge 1 ]]; then
      return
    fi
    sleep 2
  done

  log "tls-backend endpoints did not converge to ${version}"
  k -n "${TEST_NAMESPACE}" get pod,endpointslice -o wide >&2
  exit 1
}

gateway_cert_subject() {
  openssl s_client \
    -connect "127.0.0.1:${HTTPS_HOST_PORT}" \
    -servername "${TEST_HOST}" \
    </dev/null 2>/dev/null \
    | openssl x509 -noout -subject 2>/dev/null
}

wait_for_gateway_cert_cn() {
  local expected="$1"
  local subject

  for _ in $(seq 1 90); do
    subject="$(gateway_cert_subject || true)"
    if [[ "${subject}" == *"CN = ${expected}"* || "${subject}" == *"CN=${expected}"* ]]; then
      return
    fi
    sleep 2
  done

  log "gateway did not present certificate CN ${expected}"
  gateway_cert_subject >&2 || true
  exit 1
}

request_gateway() {
  curl -ksS --fail \
    --http1.1 \
    --noproxy '*' \
    --resolve "${TEST_HOST}:${HTTPS_HOST_PORT}:127.0.0.1" \
    -H 'Connection: close' \
    "https://${TEST_HOST}:${HTTPS_HOST_PORT}/tls"
}

wait_for_body() {
  local expected="$1"
  local body=""

  for _ in $(seq 1 90); do
    body="$(request_gateway 2>/dev/null || true)"
    if [[ "${body}" == "${expected}" ]]; then
      return
    fi
    sleep 2
  done

  log "gateway did not return expected body"
  printf 'expected=%s\nlast_body=%s\n' "${expected}" "${body}" >&2
  exit 1
}

rotate_tls_assets() {
  log "rotating Gateway certificate, BackendTLSPolicy CA bundle and backend client certificate"
  apply_gateway_cert_secret gateway-rotated
  apply_backend_client_secret backend-client-rotated
  apply_backend_ca_configmap backend-server-rotated
  k -n "${TEST_NAMESPACE}" patch service tls-backend \
    -p '{"spec":{"selector":{"app":"tls-backend","version":"rotated"}}}' >/dev/null
}

main() {
  require_command curl
  require_command diff
  require_command docker
  require_command jq
  require_command kind
  require_command kubectl
  require_command openssl

  ensure_kind_cluster
  sync_test_image
  TMP_DIR="$(mktemp -d "${ROOT_DIR}/tmp/tls-asset-rotation.XXXXXX")"
  trap cleanup EXIT

  cleanup_namespace
  suspend_conflicting_smoke_gateway
  generate_cert_material
  apply_resources
  wait_for_deployment tls-backend-initial
  wait_for_deployment tls-backend-rotated
  wait_for_service_endpoints initial
  wait_for_gateway_ready
  wait_for_route_accepted
  wait_for_gateway_cert_cn gateway-initial
  wait_for_body "backend-initial client-cn=backend-client-initial"

  capture_pod_identity "${CONTROLPLANE_NAMESPACE}" "${CONTROLPLANE_SELECTOR}" "${TMP_DIR}/controlplane-pods-before.json"
  capture_pod_identity "${DATAPLANE_NAMESPACE}" "${DATAPLANE_SELECTOR}" "${TMP_DIR}/dataplane-pods-before.json"

  local rotation_started
  local gateway_cert_rotated_after
  rotation_started="${SECONDS}"
  rotate_tls_assets
  wait_for_service_endpoints rotated
  wait_for_gateway_cert_cn gateway-rotated
  gateway_cert_rotated_after="$((SECONDS - rotation_started))"
  wait_for_body "backend-rotated client-cn=backend-client-rotated"
  log "rotated assets became effective: gateway certificate ${gateway_cert_rotated_after}s, backend TLS/client certificate $((SECONDS - rotation_started))s"
  assert_pod_identity_unchanged "${CONTROLPLANE_NAMESPACE}" "${CONTROLPLANE_SELECTOR}" "${TMP_DIR}/controlplane-pods-before.json"
  assert_pod_identity_unchanged "${DATAPLANE_NAMESPACE}" "${DATAPLANE_SELECTOR}" "${TMP_DIR}/dataplane-pods-before.json"

  SUCCESS="true"
  log "TLS asset rotation validation passed"
}

main "$@"
