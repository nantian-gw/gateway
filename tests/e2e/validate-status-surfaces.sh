#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
ENSURE_KIND="${ENSURE_KIND:-false}"
GATEWAY_CLASS_NAME="${GATEWAY_CLASS_NAME:-aether}"
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-aether-gateway}"
GATEWAY_NAME="${GATEWAY_NAME:-edge}"
OUTPUT_DIR="${OUTPUT_DIR:-$(mktemp -d "${ROOT_DIR}/tmp/status-surfaces.XXXXXX")}"
KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-false}"
SUCCESS="false"

log() {
  printf '[status-surfaces] %s\n' "$*"
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

cleanup() {
  if [[ "${SUCCESS}" == "true" || "${KEEP_ARTIFACTS}" != "true" ]]; then
    rm -rf "${OUTPUT_DIR}" >/dev/null 2>&1 || true
  else
    log "artifacts kept at ${OUTPUT_DIR}"
  fi
}
trap cleanup EXIT

debug_dump() {
  printf '\n[status-surfaces] debug: GatewayClass %s\n' "${GATEWAY_CLASS_NAME}" >&2
  k get gatewayclass "${GATEWAY_CLASS_NAME}" -o json | jq '.' >&2 || true

  printf '\n[status-surfaces] debug: Gateway %s/%s\n' "${GATEWAY_NAMESPACE}" "${GATEWAY_NAME}" >&2
  k -n "${GATEWAY_NAMESPACE}" get gateway "${GATEWAY_NAME}" -o json | jq '.' >&2 || true

  printf '\n[status-surfaces] debug: HTTPRoute %s/%s\n' "${GATEWAY_NAMESPACE}" "echo" >&2
  k -n "${GATEWAY_NAMESPACE}" get httproute echo -o json | jq '.' >&2 || true

  if [[ -d "${OUTPUT_DIR}/admin" ]]; then
    printf '\n[status-surfaces] debug: controlplane summary\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/controlplane/summary.json" >&2 || true
    printf '\n[status-surfaces] debug: controlplane snapshot-sync\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/controlplane/snapshot-sync.json" >&2 || true
    printf '\n[status-surfaces] debug: dataplane summary\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/summary.json" >&2 || true
    printf '\n[status-surfaces] debug: dataplane listeners\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/listeners.json" >&2 || true
    printf '\n[status-surfaces] debug: dataplane routes\n' >&2
    jq '.' "${OUTPUT_DIR}/admin/dataplane/routes.json" >&2 || true
  fi
}

fail() {
  log "$1"
  debug_dump
  exit 1
}

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
    SKIP_BUILD="${SKIP_BUILD:-true}" ./tests/e2e/run-kind.sh
  )
}

capture_admin_snapshots() {
  log "capturing controlplane and dataplane admin snapshots"
  (
    cd "${ROOT_DIR}"
    ENABLE_KIND_PORT_FORWARD=true \
    OUTPUT_DIR="${OUTPUT_DIR}/admin" \
    STRICT=true \
    INCLUDE_CONTROLPLANE_METRICS=false \
    INCLUDE_DATAPLANE_METRICS=false \
    ./scripts/collect-admin-snapshots.sh
  )
}

capture_k8s_objects() {
  mkdir -p "${OUTPUT_DIR}/k8s"
  k get gatewayclass "${GATEWAY_CLASS_NAME}" -o json >"${OUTPUT_DIR}/k8s/gatewayclass.json"
  k -n "${GATEWAY_NAMESPACE}" get gateway "${GATEWAY_NAME}" -o json >"${OUTPUT_DIR}/k8s/gateway.json"
  k -n "${GATEWAY_NAMESPACE}" get httproute echo -o json >"${OUTPUT_DIR}/k8s/httproute-echo.json"
}

validate_gatewayclass_status() {
  log "validating GatewayClass conditions and supportedFeatures"
  jq -e '
    (.status.conditions // [] | any(.type == "Accepted" and .status == "True" and .reason == "Accepted")) and
    (.status.conditions // [] | any(.type == "SupportedVersion" and .status == "True" and .reason == "SupportedVersion")) and
    ((.status.supportedFeatures // []) | length > 0) and
    (((.status.supportedFeatures // []) | map(.name)) == (((.status.supportedFeatures // []) | map(.name)) | sort))
  ' "${OUTPUT_DIR}/k8s/gatewayclass.json" >/dev/null || fail "GatewayClass status is missing Accepted/SupportedVersion or supportedFeatures is unsorted"
}

validate_gateway_status() {
  log "validating Gateway and listener conditions"
  jq -e '
    (.status.conditions // [] | any(.type == "Accepted" and .status == "True")) and
    (.status.conditions // [] | any(.type == "Programmed" and .status == "True")) and
    (.status.listeners // [] | any(
      .name == "http" and
      (.conditions // [] | any(.type == "Accepted" and .status == "True")) and
      (.conditions // [] | any(.type == "ResolvedRefs" and .status == "True")) and
      (.conditions // [] | any(.type == "Programmed" and .status == "True"))
    ))
  ' "${OUTPUT_DIR}/k8s/gateway.json" >/dev/null || fail "Gateway edge is missing accepted/programmed listener status"
}

validate_route_status() {
  log "validating HTTPRoute parent conditions"
  jq -e --arg gateway_name "${GATEWAY_NAME}" '
    (.status.parents // [] | any(
      .parentRef.name == $gateway_name and
      (.conditions // [] | any(.type == "Accepted" and .status == "True")) and
      (.conditions // [] | any(.type == "ResolvedRefs" and .status == "True"))
    ))
  ' "${OUTPUT_DIR}/k8s/httproute-echo.json" >/dev/null || fail "HTTPRoute echo is missing Accepted/ResolvedRefs status for its Gateway parent"
}

validate_admin_consistency() {
  local snapshot_version

  log "validating admin summaries and snapshot propagation"
  jq -e '
    (.snapshotVersion // "") != "" and
    (.listenerCount // 0) > 0 and
    (.routeCount // 0) > 0 and
    (.backendCount // 0) > 0
  ' "${OUTPUT_DIR}/admin/controlplane/summary.json" >/dev/null || fail "controlplane summary is missing listener/route/backend counts"

  snapshot_version="$(jq -r '.snapshotVersion' "${OUTPUT_DIR}/admin/controlplane/summary.json")"
  jq -e --arg version "${snapshot_version}" '
    .snapshotVersion == $version
  ' "${OUTPUT_DIR}/admin/controlplane/snapshot-sync.json" >/dev/null || fail "controlplane snapshot-sync is not aligned with summary snapshotVersion"

  jq -e --arg version "${snapshot_version}" '
    .snapshotVersion == $version and
    (.listenerCount // 0) > 0 and
    (.routeCount // 0) > 0 and
    (.backendCount // 0) > 0
  ' "${OUTPUT_DIR}/admin/dataplane/summary.json" >/dev/null || fail "dataplane summary is not aligned with controlplane snapshotVersion"
}

validate_dataplane_views() {
  log "validating dataplane listener and route views"
  jq -e '
    any(.[]; .name == "aether-gateway/edge/http" and (.attached_routes | index("aether-gateway/echo")) and (.attached_routes | index("aether-gateway/grpc-echo")))
  ' "${OUTPUT_DIR}/admin/dataplane/listeners.json" >/dev/null || fail "dataplane listeners view is missing edge/http attachments"

  jq -e '
    (.http | any(.[]; .namespace == "aether-gateway" and .name == "echo")) and
    (.grpc | any(.[]; .namespace == "aether-gateway" and .name == "grpc-echo")) and
    (.stream | any(.[]; .namespace == "aether-gateway" and .name == "tcp-echo"))
  ' "${OUTPUT_DIR}/admin/dataplane/routes.json" >/dev/null || fail "dataplane routes view is missing expected HTTP/GRPC/stream routes"
}

main() {
  require_command curl
  require_command jq
  require_command kind
  require_command kubectl
  require_command ss

  ensure_kind_cluster
  capture_admin_snapshots
  capture_k8s_objects
  validate_gatewayclass_status
  validate_gateway_status
  validate_route_status
  validate_admin_consistency
  validate_dataplane_views

  SUCCESS="true"
  log "status and admin surface validation passed"
}

main "$@"
