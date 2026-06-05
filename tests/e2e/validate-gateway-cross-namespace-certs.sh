#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-gw}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
ROUTE_NAMESPACE="${ROUTE_NAMESPACE:-nantian-gw-cert-route}"
SECRET_NAMESPACE="${SECRET_NAMESPACE:-nantian-gw-cert-secret}"
HTTPS_HOST_PORT="${HTTPS_HOST_PORT:-18443}"
TEST_HOST="${TEST_HOST:-cross-cert.example.com}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"

TMP_DIR=""
SUCCESS="false"
SMOKE_GATEWAY_SUSPENDED="false"

log() {
  printf '[gateway-cross-namespace-certs] %s\n' "$*"
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
  printf '\n[gateway-cross-namespace-certs] debug: gateway\n' >&2
  k -n "${ROUTE_NAMESPACE}" get gateway cert-edge -o yaml >&2
  printf '\n[gateway-cross-namespace-certs] debug: route\n' >&2
  k -n "${ROUTE_NAMESPACE}" get httproute cert-route -o yaml >&2
  printf '\n[gateway-cross-namespace-certs] debug: referencegrant\n' >&2
  k -n "${SECRET_NAMESPACE}" get referencegrant allow-gateway-cert -o yaml >&2
  printf '\n[gateway-cross-namespace-certs] debug: secret\n' >&2
  k -n "${SECRET_NAMESPACE}" get secret shared-cert -o yaml >&2
  set -e
}

cleanup() {
  local exit_code="$?"

  if [[ "${SUCCESS}" != "true" ]]; then
    dump_debug_state
  fi

  if [[ "${KEEP_RESOURCES}" != "true" ]]; then
    cleanup_namespace "${ROUTE_NAMESPACE}"
    cleanup_namespace "${SECRET_NAMESPACE}"
  else
    log "keeping namespaces ${ROUTE_NAMESPACE} and ${SECRET_NAMESPACE}"
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

generate_tls_cert() {
  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "${TMP_DIR}/tls.key" \
    -out "${TMP_DIR}/tls.crt" \
    -days 1 \
    -subj "/CN=${TEST_HOST}" \
    -addext "subjectAltName=DNS:${TEST_HOST}" >/dev/null 2>&1
}

suspend_conflicting_smoke_gateway() {
  if ! k -n nantian-gw get gateway edge >/dev/null 2>&1; then
    return
  fi

  log "temporarily deleting smoke gateway to avoid TLS passthrough bind conflicts"
  k -n nantian-gw delete gateway edge --ignore-not-found >/dev/null
  SMOKE_GATEWAY_SUSPENDED="true"
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
  name: ${SECRET_NAMESPACE}
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: cert-edge
  namespace: ${ROUTE_NAMESPACE}
spec:
  gatewayClassName: nantian
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
      hostname: ${TEST_HOST}
      tls:
        mode: Terminate
        certificateRefs:
          - name: shared-cert
            namespace: ${SECRET_NAMESPACE}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: cert-route
  namespace: ${ROUTE_NAMESPACE}
spec:
  parentRefs:
    - name: cert-edge
      sectionName: https
  hostnames:
    - ${TEST_HOST}
  rules:
    - filters:
        - type: RequestRedirect
          requestRedirect:
            statusCode: 302
            path:
              type: ReplaceFullPath
              replaceFullPath: /granted
EOF

  k -n "${SECRET_NAMESPACE}" create secret tls shared-cert \
    --cert="${TMP_DIR}/tls.crt" \
    --key="${TMP_DIR}/tls.key" >/dev/null
}

apply_reference_grant() {
  cat <<EOF | k apply -f - >/dev/null
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-gateway-cert
  namespace: ${SECRET_NAMESPACE}
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: Gateway
      namespace: ${ROUTE_NAMESPACE}
  to:
    - group: ""
      kind: Secret
      name: shared-cert
EOF
}

delete_reference_grant() {
  k -n "${SECRET_NAMESPACE}" delete referencegrant allow-gateway-cert --ignore-not-found >/dev/null
}

gateway_listener_condition_json() {
  local condition_type="$1"

  k -n "${ROUTE_NAMESPACE}" get gateway cert-edge -o json \
    | jq -c --arg type "${condition_type}" '.status.listeners[0].conditions[]? | select(.type == $type)'
}

route_parent_condition_json() {
  local condition_type="$1"

  k -n "${ROUTE_NAMESPACE}" get httproute cert-route -o json \
    | jq -c --arg type "${condition_type}" '.status.parents[0].conditions[]? | select(.type == $type)'
}

wait_for_condition() {
  local scope="$1"
  local condition_type="$2"
  local want_status="$3"
  local want_reason="$4"
  local item status reason

  for _ in $(seq 1 90); do
    if [[ "${scope}" == "gateway" ]]; then
      item="$(gateway_listener_condition_json "${condition_type}" 2>/dev/null || true)"
    else
      item="$(route_parent_condition_json "${condition_type}" 2>/dev/null || true)"
    fi

    if [[ -n "${item}" ]]; then
      status="$(jq -r '.status' <<<"${item}")"
      reason="$(jq -r '.reason' <<<"${item}")"
      if [[ "${status}" == "${want_status}" && "${reason}" == "${want_reason}" ]]; then
        return
      fi
    fi
    sleep 2
  done

  log "${scope} condition ${condition_type} did not converge to ${want_status}/${want_reason}"
  if [[ "${scope}" == "gateway" ]]; then
    k -n "${ROUTE_NAMESPACE}" get gateway cert-edge -o yaml >&2
  else
    k -n "${ROUTE_NAMESPACE}" get httproute cert-route -o yaml >&2
  fi
  exit 1
}

expect_https_redirect() {
  local expected="$1"
  local status_line
  local location_header

  for _ in $(seq 1 60); do
    status_line="$(curl -ksSI --resolve "${TEST_HOST}:${HTTPS_HOST_PORT}:127.0.0.1" "https://${TEST_HOST}:${HTTPS_HOST_PORT}/" | tr -d '\r' | sed -n '1p' || true)"
    location_header="$(curl -ksSI --resolve "${TEST_HOST}:${HTTPS_HOST_PORT}:127.0.0.1" "https://${TEST_HOST}:${HTTPS_HOST_PORT}/" | tr -d '\r' | grep -i '^location:' || true)"
    if [[ "${status_line}" == *"302"* && "${location_header}" == *"${expected}"* ]]; then
      return
    fi
    sleep 2
  done

  log "gateway did not return expected https redirect ${expected}"
  curl -kSI --resolve "${TEST_HOST}:${HTTPS_HOST_PORT}:127.0.0.1" "https://${TEST_HOST}:${HTTPS_HOST_PORT}/" >&2 || true
  exit 1
}

expect_https_unavailable() {
  for _ in $(seq 1 20); do
    if ! curl -ksS --resolve "${TEST_HOST}:${HTTPS_HOST_PORT}:127.0.0.1" "https://${TEST_HOST}:${HTTPS_HOST_PORT}/" >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done

  log "gateway unexpectedly still served https after ReferenceGrant deletion"
  curl -kSI --resolve "${TEST_HOST}:${HTTPS_HOST_PORT}:127.0.0.1" "https://${TEST_HOST}:${HTTPS_HOST_PORT}/" >&2 || true
  exit 1
}

main() {
  require_command curl
  require_command jq
  require_command kind
  require_command kubectl
  require_command openssl

  ensure_kind_cluster
  TMP_DIR="$(mktemp -d "${ROOT_DIR}/tmp/gateway-cross-namespace-certs.XXXXXX")"
  trap cleanup EXIT

  cleanup_namespace "${ROUTE_NAMESPACE}"
  cleanup_namespace "${SECRET_NAMESPACE}"
  suspend_conflicting_smoke_gateway
  generate_tls_cert
  apply_base_resources

  wait_for_condition gateway ResolvedRefs False RefNotPermitted
  wait_for_condition gateway Programmed False Invalid
  wait_for_condition route Accepted True Accepted
  log "gateway correctly rejects cross-namespace certificateRef before ReferenceGrant"

  apply_reference_grant
  wait_for_condition gateway ResolvedRefs True ResolvedRefs
  wait_for_condition gateway Programmed True Programmed
  wait_for_condition route Accepted True Accepted
  expect_https_redirect /granted
  log "gateway becomes routable after ReferenceGrant is created"

  delete_reference_grant
  wait_for_condition gateway ResolvedRefs False RefNotPermitted
  wait_for_condition gateway Programmed False Invalid
  expect_https_unavailable
  log "gateway becomes invalid again after ReferenceGrant deletion"

  SUCCESS="true"
  log "cross-namespace certificate validation passed"
}

main "$@"
