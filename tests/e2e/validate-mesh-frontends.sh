#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
PRODUCER_NAMESPACE="${PRODUCER_NAMESPACE:-aether-mesh-validation}"
CONSUMER_NAMESPACE="${CONSUMER_NAMESPACE:-aether-mesh-consumer-validation}"
ADMIN_FORWARD_PORT="${ADMIN_FORWARD_PORT:-29080}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"
ECHO_SOURCE_IMAGE="${ECHO_SOURCE_IMAGE:-m.daocloud.io/gcr.io/k8s-staging-gateway-api/echo-advanced:v20240412-v1.0.0-394-g40c666fd}"
ECHO_IMAGE="${ECHO_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-advanced:v20240412-v1.0.0-394-g40c666fd}"
CLIENT_SOURCE_IMAGE="${CLIENT_SOURCE_IMAGE:-m.daocloud.io/docker.io/library/busybox:1.36.1}"
CLIENT_IMAGE="${CLIENT_IMAGE:-${LOCAL_REGISTRY_HOST}/aether-gateway-validation/busybox:1.36.1}"

TMP_DIR=""
PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""
SUCCESS="false"

log() {
  printf '[mesh-validation] %s\n' "$*"
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
    for image in "${ECHO_IMAGE}" "${CLIENT_IMAGE}"; do
      docker exec "${node}" crictl pull "${image}" >/dev/null
    done
  done
}

sync_test_images() {
  ensure_local_registry
  sync_image_to_local_registry "${ECHO_SOURCE_IMAGE}" "${ECHO_IMAGE}"
  sync_image_to_local_registry "${CLIENT_SOURCE_IMAGE}" "${CLIENT_IMAGE}"
  preload_kind_images
}

cleanup_namespace() {
  local namespace="$1"

  if ! k get namespace "${namespace}" >/dev/null 2>&1; then
    return
  fi

  log "cleaning namespace ${namespace}"
  k delete namespace "${namespace}" --wait=false >/dev/null 2>&1 || true
  if ! timeout 120 bash -c \
    "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${namespace}' >/dev/null 2>&1; do sleep 2; done"
  then
    log "forcing cleanup for namespace ${namespace}"
    k -n "${namespace}" delete pod --all --force --grace-period=0 >/dev/null 2>&1 || true
    k get namespace "${namespace}" -o json \
      | jq '{apiVersion, kind, metadata: {name: .metadata.name}, spec: {finalizers: []}}' \
      | kubectl --context "${KUBE_CONTEXT}" replace --raw "/api/v1/namespaces/${namespace}/finalize" -f - >/dev/null 2>&1 || true

    if ! timeout 30 bash -c \
      "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${namespace}' >/dev/null 2>&1; do sleep 2; done"
    then
      log "namespace ${namespace} is still terminating after force cleanup"
      exit 1
    fi
  fi
}

render_resources() {
  cat >"${TMP_DIR}/resources.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${PRODUCER_NAMESPACE}
---
apiVersion: v1
kind: Namespace
metadata:
  name: ${CONSUMER_NAMESPACE}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo-v1
  namespace: ${PRODUCER_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echo
      version: v1
  template:
    metadata:
      labels:
        app: echo
        version: v1
    spec:
      containers:
      - name: echo
        image: ${ECHO_IMAGE}
        imagePullPolicy: IfNotPresent
        args:
        - --tcp=9090
        - --port=8080
        - --grpc=7070
        - --port=8443
        - --tls=8443
        - --crt=/cert.crt
        - --key=/cert.key
---
apiVersion: v1
kind: Service
metadata:
  name: echo-v1
  namespace: ${PRODUCER_NAMESPACE}
spec:
  selector:
    app: echo
    version: v1
  ports:
  - name: http
    port: 80
    appProtocol: http
    targetPort: 8080
  - name: http-alt
    port: 8080
    appProtocol: http
    targetPort: 8080
  - name: grpc
    port: 7070
    appProtocol: grpc
    targetPort: 7070
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo-v2
  namespace: ${PRODUCER_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echo
      version: v2
  template:
    metadata:
      labels:
        app: echo
        version: v2
    spec:
      containers:
      - name: echo
        image: ${ECHO_IMAGE}
        imagePullPolicy: IfNotPresent
        args:
        - --tcp=9090
        - --port=8080
        - --grpc=7070
        - --port=8443
        - --tls=8443
        - --crt=/cert.crt
        - --key=/cert.key
---
apiVersion: v1
kind: Service
metadata:
  name: echo-v2
  namespace: ${PRODUCER_NAMESPACE}
spec:
  selector:
    app: echo
    version: v2
  ports:
  - name: http
    port: 80
    appProtocol: http
    targetPort: 8080
  - name: http-alt
    port: 8080
    appProtocol: http
    targetPort: 8080
  - name: grpc
    port: 7070
    appProtocol: grpc
    targetPort: 7070
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: client
  namespace: ${PRODUCER_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: client
  template:
    metadata:
      labels:
        app: client
    spec:
      containers:
      - name: client
        image: ${CLIENT_IMAGE}
        imagePullPolicy: IfNotPresent
        command: ["sh", "-c", "sleep 3600"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: client
  namespace: ${CONSUMER_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: client
  template:
    metadata:
      labels:
        app: client
    spec:
      containers:
      - name: client
        image: ${CLIENT_IMAGE}
        imagePullPolicy: IfNotPresent
        command: ["sh", "-c", "sleep 3600"]
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mesh-consumer-route
  namespace: ${CONSUMER_NAMESPACE}
spec:
  parentRefs:
  - group: ""
    kind: Service
    name: echo-v1
    namespace: ${PRODUCER_NAMESPACE}
  rules:
  - filters:
    - type: ResponseHeaderModifier
      responseHeaderModifier:
        set:
        - name: X-Mesh-Consumer
          value: consumer
    backendRefs:
    - name: echo-v1
      namespace: ${PRODUCER_NAMESPACE}
      port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mesh-port-route
  namespace: ${PRODUCER_NAMESPACE}
spec:
  parentRefs:
  - group: ""
    kind: Service
    name: echo-v2
    port: 80
  rules:
  - filters:
    - type: ResponseHeaderModifier
      responseHeaderModifier:
        set:
        - name: X-Mesh-Port
          value: port-80
    backendRefs:
    - name: echo-v2
      port: 80
EOF
}

apply_resources() {
  render_resources
  k apply -f "${TMP_DIR}/resources.yaml" >/dev/null
}

wait_for_deployment() {
  local namespace="$1"
  local name="$2"
  k -n "${namespace}" rollout status deployment/"${name}" --timeout=180s >/dev/null
}

wait_for_route_acceptance() {
  local namespace="$1"
  local name="$2"

  for _ in $(seq 1 60); do
    if k -n "${namespace}" get httproute "${name}" -o json \
      | jq -e '[.status.parents[]?.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0' \
      >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "route ${namespace}/${name} did not become accepted"
  k -n "${namespace}" get httproute "${name}" -o yaml >&2
  exit 1
}

wait_for_mesh_resources() {
  local service shadow_name

  for service in echo-v1 echo-v2; do
    shadow_name="aeg-shadow-${service}"
    for _ in $(seq 1 60); do
      if [[ "$(k -n "${PRODUCER_NAMESPACE}" get service "${service}" -o jsonpath='{.metadata.annotations.pgw\.io/mesh-frontend}' 2>/dev/null || true)" == "true" ]] \
        && k -n "${PRODUCER_NAMESPACE}" get service "${shadow_name}" >/dev/null 2>&1 \
        && k -n "${PRODUCER_NAMESPACE}" get endpointslice \
             -l "kubernetes.io/service-name=${service},nantian.dev/service-role=mesh-frontend-endpoints" \
             -o name | grep -q .
      then
        break
      fi
      sleep 2
    done

    if [[ "$(k -n "${PRODUCER_NAMESPACE}" get service "${service}" -o jsonpath='{.metadata.annotations.pgw\.io/mesh-frontend}' 2>/dev/null || true)" != "true" ]]; then
      log "service ${PRODUCER_NAMESPACE}/${service} was not converted to a mesh frontend"
      exit 1
    fi
    if ! k -n "${PRODUCER_NAMESPACE}" get service "${shadow_name}" >/dev/null 2>&1; then
      log "shadow service ${PRODUCER_NAMESPACE}/${shadow_name} was not created"
      exit 1
    fi
  done

  wait_for_route_acceptance "${CONSUMER_NAMESPACE}" mesh-consumer-route
  wait_for_route_acceptance "${PRODUCER_NAMESPACE}" mesh-port-route
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
  k -n aether-gateway port-forward service/aether-gateway-dataplane-admin "${ADMIN_FORWARD_PORT}:19080" \
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

client_pod() {
  local namespace="$1"

  k -n "${namespace}" get pod -l app=client -o jsonpath='{.items[0].metadata.name}'
}

request_from_client() {
  local namespace="$1"
  local url="$2"
  local pod

  pod="$(client_pod "${namespace}")"
  k -n "${namespace}" exec "${pod}" -- sh -ceu "wget -T 5 -S -q -O - '${url}' 2>&1"
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local description="$3"

  if ! grep -Fq "${needle}" <<<"${haystack}"; then
    log "expected ${description} to contain ${needle}"
    printf '%s\n' "${haystack}" >&2
    exit 1
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local description="$3"

  if grep -Fq "${needle}" <<<"${haystack}"; then
    log "expected ${description} not to contain ${needle}"
    printf '%s\n' "${haystack}" >&2
    exit 1
  fi
}

wait_for_admin_mesh_state() {
  local listeners routes

  for _ in $(seq 1 60); do
    listeners="$(curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/listeners" 2>/dev/null || true)"
    routes="$(curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/routes" 2>/dev/null || true)"
    if [[ -z "${listeners}" || -z "${routes}" ]]; then
      sleep 1
      continue
    fi

    if jq -e \
      --arg ns "${PRODUCER_NAMESPACE}" \
      '[.[] | select(.metadata["nantian.dev/frontend-kind"]=="Service" and .metadata["nantian.dev/frontend-namespace"]==$ns)] | length >= 2' \
      <<<"${listeners}" >/dev/null 2>&1 \
      && jq -e \
        --arg producer_ns "${PRODUCER_NAMESPACE}" \
        --arg consumer_ns "${CONSUMER_NAMESPACE}" \
        '[.http[]? | select((.namespace==$producer_ns and .name=="mesh-port-route") or (.namespace==$consumer_ns and .name=="mesh-consumer-route"))] | length == 2' \
        <<<"${routes}" >/dev/null 2>&1
    then
      log "dataplane admin reports mesh listeners and routes"
      jq '{listenerCount, routeCount, httpRouteCount, warnings}' \
        <<<"$(curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/summary")"
      return
    fi
    sleep 1
  done

  log "dataplane admin did not report expected mesh listeners/routes"
  curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/listeners" | jq '.' >&2
  curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/routes" | jq '.' >&2
  exit 1
}

validate_consumer_route() {
  local consumer_response producer_response

  consumer_response="$(request_from_client "${CONSUMER_NAMESPACE}" "http://echo-v1.${PRODUCER_NAMESPACE}/")"
  producer_response="$(request_from_client "${PRODUCER_NAMESPACE}" "http://echo-v1.${PRODUCER_NAMESPACE}/")"

  assert_contains "${consumer_response}" "X-Mesh-Consumer: consumer" "consumer mesh response"
  assert_contains "${consumer_response}" "Hostname=echo-v1-" "consumer mesh response"
  assert_not_contains "${producer_response}" "X-Mesh-Consumer: consumer" "producer mesh response"
  assert_contains "${producer_response}" "Hostname=echo-v1-" "producer mesh response"

  log "mesh consumer-route namespace isolation passed"
}

echo_v2_pod_ip() {
  k -n "${PRODUCER_NAMESPACE}" get pod -l app=echo,version=v2 -o jsonpath='{.items[0].status.podIP}'
}

validate_mesh_frontend_behavior() {
  local service_response alt_port_response pod_response pod_ip

  service_response="$(request_from_client "${PRODUCER_NAMESPACE}" "http://echo-v2.${PRODUCER_NAMESPACE}/")"
  alt_port_response="$(request_from_client "${PRODUCER_NAMESPACE}" "http://echo-v2.${PRODUCER_NAMESPACE}:8080/")"
  pod_ip="$(echo_v2_pod_ip)"
  pod_response="$(request_from_client "${PRODUCER_NAMESPACE}" "http://${pod_ip}:8080/")"

  assert_contains "${service_response}" "X-Mesh-Port: port-80" "mesh service response"
  assert_contains "${service_response}" "Hostname=echo-v2-" "mesh service response"
  assert_not_contains "${alt_port_response}" "X-Mesh-Port: port-80" "mesh excluded port response"
  assert_contains "${alt_port_response}" "Hostname=echo-v2-" "mesh excluded port response"
  assert_not_contains "${pod_response}" "X-Mesh-Port: port-80" "mesh direct pod response"
  assert_contains "${pod_response}" "Hostname=echo-v2-" "mesh direct pod response"

  log "mesh frontend routing and parentRef.port scoping passed"
}

dump_debug_state() {
  set +e
  printf '\n[mesh-validation] debug: services\n' >&2
  k -n "${PRODUCER_NAMESPACE}" get service echo-v1 echo-v2 aeg-shadow-echo-v1 aeg-shadow-echo-v2 -o yaml >&2
  printf '\n[mesh-validation] debug: routes\n' >&2
  k -n "${PRODUCER_NAMESPACE}" get httproute mesh-port-route -o yaml >&2
  k -n "${CONSUMER_NAMESPACE}" get httproute mesh-consumer-route -o yaml >&2
  printf '\n[mesh-validation] debug: endpoint slices\n' >&2
  k -n "${PRODUCER_NAMESPACE}" get endpointslices -o wide >&2
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    printf '\n[mesh-validation] debug: admin summary\n' >&2
    curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/summary" | jq '.' >&2 || true
    printf '\n[mesh-validation] debug: admin listeners\n' >&2
    curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/listeners" | jq '.' >&2 || true
    printf '\n[mesh-validation] debug: admin routes\n' >&2
    curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/routes" | jq '.' >&2 || true
  fi
  set -e
}

cleanup() {
  local status=$?

  stop_admin_port_forward
  if [[ ${status} -ne 0 || "${SUCCESS}" != "true" ]]; then
    dump_debug_state
  fi

  if [[ "${KEEP_RESOURCES}" != "true" ]]; then
    cleanup_namespace "${CONSUMER_NAMESPACE}" || true
    cleanup_namespace "${PRODUCER_NAMESPACE}" || true
  fi

  rm -rf "${TMP_DIR}"
}

main() {
  require_command kubectl
  require_command kind
  require_command docker
  require_command curl
  require_command jq
  require_command ss
  require_command timeout

  TMP_DIR="$(mktemp -d)"
  trap cleanup EXIT

  ensure_kind_cluster
  sync_test_images
  cleanup_namespace "${CONSUMER_NAMESPACE}" || true
  cleanup_namespace "${PRODUCER_NAMESPACE}" || true
  apply_resources

  log "waiting for mesh validation deployments"
  wait_for_deployment "${PRODUCER_NAMESPACE}" echo-v1
  wait_for_deployment "${PRODUCER_NAMESPACE}" echo-v2
  wait_for_deployment "${PRODUCER_NAMESPACE}" client
  wait_for_deployment "${CONSUMER_NAMESPACE}" client

  wait_for_mesh_resources
  start_admin_port_forward
  wait_for_admin_mesh_state
  validate_consumer_route
  validate_mesh_frontend_behavior

  SUCCESS="true"
  log "mesh frontend validation passed"
}

main "$@"
