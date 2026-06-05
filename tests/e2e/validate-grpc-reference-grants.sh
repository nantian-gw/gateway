#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
ROUTE_NAMESPACE="${ROUTE_NAMESPACE:-aether-grpc-reference-grants-route}"
BACKEND_NAMESPACE="${BACKEND_NAMESPACE:-aether-grpc-reference-grants-backend}"
GRPC_HOST="${GRPC_HOST:-grpc-refgrant.example.com}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"
GRPC_SOURCE_IMAGE="${GRPC_SOURCE_IMAGE:-m.daocloud.io/gcr.io/k8s-staging-gateway-api/echo-basic:v20240412-v1.0.0-394-g40c666fd}"
GRPC_IMAGE="${GRPC_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-basic:v20240412-v1.0.0-394-g40c666fd}"

SUCCESS="false"

log() {
  printf '[grpc-reference-grants] %s\n' "$*"
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

  log "preloading validation image into kind nodes via crictl"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    docker exec "${node}" crictl pull "${GRPC_IMAGE}" >/dev/null
  done
}

sync_test_images() {
  ensure_local_registry
  sync_image_to_local_registry "${GRPC_SOURCE_IMAGE}" "${GRPC_IMAGE}"
  preload_kind_images
}

cleanup_namespace() {
  local namespace="$1"

  if ! k get namespace "${namespace}" >/dev/null 2>&1; then
    return
  fi

  log "cleaning namespace ${namespace}"
  k delete namespace "${namespace}" --wait=false >/dev/null 2>&1 || true
  if ! timeout 120 bash -c "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${namespace}' >/dev/null 2>&1; do sleep 2; done"; then
    log "namespace ${namespace} is still terminating"
    exit 1
  fi
}

dump_debug_state() {
  set +e
  printf '\n[grpc-reference-grants] debug: gateway\n' >&2
  k -n "${ROUTE_NAMESPACE}" get gateway grpc-refgrant-edge -o yaml >&2
  printf '\n[grpc-reference-grants] debug: grpcroute\n' >&2
  k -n "${ROUTE_NAMESPACE}" get grpcroute grpc-refgrant-route -o yaml >&2
  printf '\n[grpc-reference-grants] debug: referencegrant\n' >&2
  k -n "${BACKEND_NAMESPACE}" get referencegrant allow-grpc-backend -o yaml >&2
  printf '\n[grpc-reference-grants] debug: service\n' >&2
  k -n "${BACKEND_NAMESPACE}" get service grpc-backend -o yaml >&2
  printf '\n[grpc-reference-grants] debug: endpointslices\n' >&2
  k -n "${BACKEND_NAMESPACE}" get endpointslices -o wide >&2
  set -e
}

cleanup() {
  local exit_code="$?"

  if [[ "${SUCCESS}" != "true" ]]; then
    dump_debug_state
  fi

  if [[ "${KEEP_RESOURCES}" != "true" ]]; then
    cleanup_namespace "${ROUTE_NAMESPACE}"
    cleanup_namespace "${BACKEND_NAMESPACE}"
  else
    log "keeping namespaces ${ROUTE_NAMESPACE} and ${BACKEND_NAMESPACE}"
  fi

  exit "${exit_code}"
}

apply_base_resources() {
  cat <<EOF | k apply -f - >/dev/null
apiVersion: v1
kind: Namespace
metadata:
  name: ${ROUTE_NAMESPACE}
---
apiVersion: v1
kind: Namespace
metadata:
  name: ${BACKEND_NAMESPACE}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-backend
  namespace: ${BACKEND_NAMESPACE}
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
  name: grpc-backend
  namespace: ${BACKEND_NAMESPACE}
spec:
  selector:
    app: grpc-backend
  ports:
    - name: grpc
      port: 8080
      targetPort: 3000
      appProtocol: kubernetes.io/h2c
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: grpc-refgrant-edge
  namespace: ${ROUTE_NAMESPACE}
spec:
  gatewayClassName: aether
  listeners:
    - name: http
      protocol: HTTP
      port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: grpc-refgrant-route
  namespace: ${ROUTE_NAMESPACE}
spec:
  parentRefs:
    - name: grpc-refgrant-edge
      sectionName: http
  hostnames:
    - ${GRPC_HOST}
  rules:
    - backendRefs:
        - name: grpc-backend
          namespace: ${BACKEND_NAMESPACE}
          port: 8080
EOF
}

apply_reference_grant() {
  cat <<EOF | k apply -f - >/dev/null
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-grpc-backend
  namespace: ${BACKEND_NAMESPACE}
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: GRPCRoute
      namespace: ${ROUTE_NAMESPACE}
  to:
    - group: ""
      kind: Service
      name: grpc-backend
EOF
}

delete_reference_grant() {
  k -n "${BACKEND_NAMESPACE}" delete referencegrant allow-grpc-backend --ignore-not-found >/dev/null
}

wait_for_deployment() {
  k -n "${BACKEND_NAMESPACE}" rollout status deployment/grpc-backend --timeout=180s >/dev/null
}

wait_for_route_condition() {
  local condition_type="$1"
  local want_status="$2"
  local want_reason="$3"
  local item status reason

  for _ in $(seq 1 90); do
    item="$(k -n "${ROUTE_NAMESPACE}" get grpcroute grpc-refgrant-route -o json | jq -c --arg type "${condition_type}" '.status.parents[0].conditions[]? | select(.type == $type)' 2>/dev/null || true)"
    if [[ -n "${item}" ]]; then
      status="$(jq -r '.status' <<<"${item}")"
      reason="$(jq -r '.reason' <<<"${item}")"
      if [[ "${status}" == "${want_status}" && "${reason}" == "${want_reason}" ]]; then
        return
      fi
    fi
    sleep 2
  done

  log "grpc route condition ${condition_type} did not converge to ${want_status}/${want_reason}"
  k -n "${ROUTE_NAMESPACE}" get grpcroute grpc-refgrant-route -o yaml >&2
  exit 1
}

wait_for_gateway_ready() {
  local accepted programmed

  for _ in $(seq 1 90); do
    accepted="$(k -n "${ROUTE_NAMESPACE}" get gateway grpc-refgrant-edge -o json | jq -r '.status.conditions[]? | select(.type == "Accepted") | .status' 2>/dev/null || true)"
    programmed="$(k -n "${ROUTE_NAMESPACE}" get gateway grpc-refgrant-edge -o json | jq -r '.status.conditions[]? | select(.type == "Programmed") | .status' 2>/dev/null || true)"
    if [[ "${accepted}" == "True" && "${programmed}" == "True" ]]; then
      return
    fi
    sleep 2
  done

  log "gateway did not become ready"
  k -n "${ROUTE_NAMESPACE}" get gateway grpc-refgrant-edge -o yaml >&2
  exit 1
}

expect_grpc_success() {
  for _ in $(seq 1 30); do
    if (
      cd "${ROOT_DIR}/controlplane" &&
        go run ./cmd/grpc-smoke-client \
          -addr "127.0.0.1:${GATEWAY_HOST_PORT}" \
          -authority "${GRPC_HOST}"
    ) >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done

  log "grpc request did not succeed after ReferenceGrant creation"
  exit 1
}

main() {
  require_command docker
  require_command go
  require_command jq
  require_command kind
  require_command kubectl

  ensure_kind_cluster
  sync_test_images
  trap cleanup EXIT

  cleanup_namespace "${ROUTE_NAMESPACE}"
  cleanup_namespace "${BACKEND_NAMESPACE}"
  apply_base_resources
  wait_for_deployment
  wait_for_gateway_ready

  wait_for_route_condition ResolvedRefs False RefNotPermitted
  wait_for_route_condition Accepted True Accepted
  log "grpc route correctly reports RefNotPermitted before ReferenceGrant"

  apply_reference_grant
  wait_for_route_condition ResolvedRefs True ResolvedRefs
  wait_for_route_condition Accepted True Accepted
  expect_grpc_success
  log "grpc route becomes routable after ReferenceGrant is created"

  delete_reference_grant
  wait_for_route_condition ResolvedRefs False RefNotPermitted
  wait_for_route_condition Accepted True Accepted
  log "grpc route becomes unresolved again after ReferenceGrant deletion"

  SUCCESS="true"
  log "grpc reference grant validation passed"
}

main "$@"
