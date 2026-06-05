#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/scripts/lib/conformance-report.sh"
source "${ROOT_DIR}/scripts/lib/kind-image-sync.sh"
TMP_DIR="${ROOT_DIR}/tmp"
HARNESS_DIR="${ROOT_DIR}/tests/conformance-harness"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.5.1}"
BACKENDLBPOLICY_CRD_VERSION="${BACKENDLBPOLICY_CRD_VERSION:-v1.2.1}"
WORK_DIR="${ROOT_DIR}/tmp/gateway-api-${GATEWAY_API_VERSION}"
GATEWAY_API_CLONE_URLS="${GATEWAY_API_CLONE_URLS:-https://gh-proxy.com/https://github.com/kubernetes-sigs/gateway-api.git,https://github.com/kubernetes-sigs/gateway-api.git}"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-gw}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
GATEWAY_CLASS="${GATEWAY_CLASS:-nantian}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_IMAGE="${LOCAL_REGISTRY_IMAGE:-registry:2}"
LOCAL_REGISTRY_MIRROR="${LOCAL_REGISTRY_MIRROR:-m.daocloud.io/docker.io/library/registry:2}"
REGISTRY_DATA_DIR="${REGISTRY_DATA_DIR:-${TMP_DIR}/kind/registry-data}"
REPORT_DIR="${REPORT_DIR:-${TMP_DIR}/conformance}"
REPORT_OUTPUT="${REPORT_OUTPUT:-${REPORT_DIR}/report-${GATEWAY_API_VERSION}.yaml}"
CONFORMANCE_LOG_PATH="${CONFORMANCE_LOG_PATH:-${REPORT_OUTPUT%.*}.log}"
REPORT_METADATA_PATH="${REPORT_METADATA_PATH:-${REPORT_OUTPUT%.*}.metadata.txt}"
RUN_TEST="${RUN_TEST:-}"
SKIP_TESTS="${SKIP_TESTS:-}"
SUPPORTED_FEATURES="${SUPPORTED_FEATURES:-}"
EXEMPT_FEATURES="${EXEMPT_FEATURES:-}"
CONFORMANCE_PROFILES="${CONFORMANCE_PROFILES:-}"
ALL_FEATURES="${ALL_FEATURES:-false}"
SKIP_PROVISIONAL_TESTS="${SKIP_PROVISIONAL_TESTS:-false}"
DEBUG="${DEBUG:-false}"
CLEANUP_BASE_RESOURCES="${CLEANUP_BASE_RESOURCES:-true}"
ALLOW_FOREIGN_GATEWAY_RESOURCES="${ALLOW_FOREIGN_GATEWAY_RESOURCES:-false}"
ALLOW_CRDS_MISMATCH="${ALLOW_CRDS_MISMATCH:-false}"
ORGANIZATION="${ORGANIZATION:-nantian-gw}"
PROJECT="${PROJECT:-nantian-gw}"
IMPLEMENTATION_URL="${IMPLEMENTATION_URL:-https://github.com/nantian-gw/gateway}"
IMPLEMENTATION_VERSION="${IMPLEMENTATION_VERSION:-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)}"
IMPLEMENTATION_CONTACT="${IMPLEMENTATION_CONTACT:-maintainers@nantian-gw.local}"
GATEWAY_CLASS_CONTROLLER="${GATEWAY_CLASS_CONTROLLER:-gateway.networking.k8s.io/nantian-gw}"
ECHO_BASIC_SOURCE_IMAGE="${ECHO_BASIC_SOURCE_IMAGE:-m.daocloud.io/gcr.io/k8s-staging-gateway-api/echo-basic:v20240412-v1.0.0-394-g40c666fd}"
ECHO_ADVANCED_SOURCE_IMAGE="${ECHO_ADVANCED_SOURCE_IMAGE:-m.daocloud.io/gcr.io/k8s-staging-gateway-api/echo-advanced:v20240412-v1.0.0-394-g40c666fd}"
COREDNS_SOURCE_IMAGE="${COREDNS_SOURCE_IMAGE:-m.daocloud.io/docker.io/coredns/coredns:latest}"
ECHO_BASIC_IMAGE="${ECHO_BASIC_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-basic:v20240412-v1.0.0-394-g40c666fd}"
ECHO_ADVANCED_IMAGE="${ECHO_ADVANCED_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-advanced:v20240412-v1.0.0-394-g40c666fd}"
COREDNS_IMAGE="${COREDNS_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/coredns:conformance}"
ECHO_BASIC_SOURCE_REPOSITORY="${ECHO_BASIC_SOURCE_REPOSITORY:-${ECHO_BASIC_SOURCE_IMAGE%:*}}"
ECHO_ADVANCED_SOURCE_REPOSITORY="${ECHO_ADVANCED_SOURCE_REPOSITORY:-${ECHO_ADVANCED_SOURCE_IMAGE%:*}}"
ECHO_BASIC_IMAGE_REPOSITORY="${ECHO_BASIC_IMAGE_REPOSITORY:-${ECHO_BASIC_IMAGE%:*}}"
ECHO_ADVANCED_IMAGE_REPOSITORY="${ECHO_ADVANCED_IMAGE_REPOSITORY:-${ECHO_ADVANCED_IMAGE%:*}}"
PRELOAD_KIND_IMAGES="${PRELOAD_KIND_IMAGES:-true}"
GO_TEST_TIMEOUT="${GO_TEST_TIMEOUT:-30m}"
RESET_CONFORMANCE_NAMESPACES="${RESET_CONFORMANCE_NAMESPACES:-true}"
ENABLE_HOST_PORT_RELAYS="${ENABLE_HOST_PORT_RELAYS:-true}"
HTTP_HOST_PORT="${HTTP_HOST_PORT:-80}"
HTTPS_HOST_PORT="${HTTPS_HOST_PORT:-443}"
HTTP_FALLBACK_PORT="${HTTP_FALLBACK_PORT:-18080}"
HTTPS_FALLBACK_PORT="${HTTPS_FALLBACK_PORT:-18443}"
ADDITIONAL_TCP_LISTENER_PORTS="${ADDITIONAL_TCP_LISTENER_PORTS:-8080,8090,8443,8883}"
ADDITIONAL_UDP_LISTENER_PORTS="${ADDITIONAL_UDP_LISTENER_PORTS:-5300}"
CONFORMANCE_USABLE_ADDRESSES="${CONFORMANCE_USABLE_ADDRESSES:-IPAddress=127.0.0.1}"
CONFORMANCE_UNUSABLE_ADDRESSES="${CONFORMANCE_UNUSABLE_ADDRESSES:-IPAddress=203.0.113.13}"

HOST_PORT_RELAY_PIDS=()
HOST_PORT_REDIRECT_RULES=()
CONFORMANCE_LOCAL_IMAGES=()
declare -A HOST_PORT_RELAYS=()

CONFORMANCE_NAMESPACES=(
  gateway-conformance-infra
  gateway-conformance-app-backend
  gateway-conformance-web-backend
  gateway-conformance-mesh
  gateway-conformance-mesh-consumer
)

log() {
  printf '[conformance] %s\n' "$*"
}

local_registry_push_image_for() {
  local target_image="$1"

  if [[ "${target_image}" == "${LOCAL_REGISTRY_HOST}/"* ]]; then
    printf '%s/%s\n' "${LOCAL_REGISTRY_PUSH_HOST}" "${target_image#${LOCAL_REGISTRY_HOST}/}"
    return
  fi

  printf '%s\n' "${target_image}"
}

derive_supported_features_from_controlplane() {
  local feature
  local supported_features=()

  if [[ -n "${SUPPORTED_FEATURES}" || "${ALL_FEATURES}" != "true" ]]; then
    return
  fi

  if ! command -v go >/dev/null 2>&1; then
    log "missing required command: go"
    exit 1
  fi

  while IFS= read -r feature; do
    [[ -n "${feature}" ]] || continue
    supported_features+=("${feature}")
  done < <(
    cd "${ROOT_DIR}/controlplane"
    GOWORK=off go run ./cmd/gateway-api-support -format names
  )

  if [[ ${#supported_features[@]} -eq 0 ]]; then
    log "failed to derive supported features from controlplane feature source"
    exit 1
  fi

  SUPPORTED_FEATURES="$(IFS=,; printf '%s' "${supported_features[*]}")"
  ALL_FEATURES="false"
  log "expanded ALL_FEATURES=true to explicit supported-features from controlplane feature source"
}

cleanup_smoke_resources() {
  if ! kubectl --context "${KUBE_CONTEXT}" get namespace nantian-gw >/dev/null 2>&1; then
    return
  fi

  log "cleaning nantian-gw smoke resources"
  kubectl --context "${KUBE_CONTEXT}" -n nantian-gw delete \
    service/grpc-echo \
    service/tls-backend \
    service/coredns \
    service/echo \
    deployment/grpc-echo \
    deployment/tls-backend \
    deployment/coredns \
    deployment/echo \
    configmap/coredns \
    secret/smoke-tls-passthrough-certificate \
    gateway.gateway.networking.k8s.io/edge \
    grpcroute.gateway.networking.k8s.io/grpc-echo \
    httproute.gateway.networking.k8s.io/echo \
    tcproute.gateway.networking.k8s.io/tcp-echo \
    tcproute.gateway.networking.k8s.io/tcp-missing \
    tlsroute.gateway.networking.k8s.io/tls-echo \
    udproute.gateway.networking.k8s.io/udp-coredns \
    udproute.gateway.networking.k8s.io/udp-missing \
    --ignore-not-found >/dev/null

  kubectl --context "${KUBE_CONTEXT}" delete gatewayclass.gateway.networking.k8s.io/"${GATEWAY_CLASS}" \
    --ignore-not-found >/dev/null
}

ensure_gateway_class() {
  kubectl --context "${KUBE_CONTEXT}" apply -f - >/dev/null <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: ${GATEWAY_CLASS}
spec:
  controllerName: ${GATEWAY_CLASS_CONTROLLER}
EOF
}

namespace_allowed_for_conformance() {
  local namespace="$1"
  local allowed

  for allowed in "${CONFORMANCE_NAMESPACES[@]}"; do
    if [[ "${allowed}" == "${namespace}" ]]; then
      return 0
    fi
  done

  return 1
}

ensure_clean_gateway_resources() {
  local foreign_resources=""
  local kind
  local namespace
  local name

  if [[ "${ALLOW_FOREIGN_GATEWAY_RESOURCES}" == "true" ]]; then
    return
  fi

  while IFS=$'\t' read -r kind namespace name; do
    [[ -z "${kind}" ]] && continue
    if namespace_allowed_for_conformance "${namespace}"; then
      continue
    fi
    foreign_resources+="${kind}\t${namespace}\t${name}"$'\n'
  done < <(
    kubectl --context "${KUBE_CONTEXT}" get \
      gateways.gateway.networking.k8s.io,\
httproutes.gateway.networking.k8s.io,\
grpcroutes.gateway.networking.k8s.io,\
backendlbpolicies.gateway.networking.k8s.io,\
backendtlspolicies.gateway.networking.k8s.io,\
referencegrants.gateway.networking.k8s.io,\
tcproutes.gateway.networking.k8s.io,\
udproutes.gateway.networking.k8s.io,\
tlsroutes.gateway.networking.k8s.io \
      -A -o json \
      | jq -r '.items[] | [.kind, .metadata.namespace, .metadata.name] | @tsv'
  )

  if [[ -z "${foreign_resources}" ]]; then
    return
  fi

  log "found gateway api resources outside conformance namespaces:"
  printf '%b' "${foreign_resources}"
  log "remove the resources above or rerun with ALLOW_FOREIGN_GATEWAY_RESOURCES=true"
  exit 1
}

is_port_listening() {
  local port="$1"
  local protocol="${2:-TCP}"
  local command_args

  case "${protocol^^}" in
    UDP)
      command_args=(-H -lun "( sport = :${port} )")
      ;;
    *)
      command_args=(-H -ltn "( sport = :${port} )")
      ;;
  esac

  ss "${command_args[@]}" 2>/dev/null \
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

cleanup_host_port_relays() {
  local rule
  local pid

  for rule in "${HOST_PORT_REDIRECT_RULES[@]:-}"; do
    iptables -w -t nat -D OUTPUT ${rule} >/dev/null 2>&1 || true
  done

  for pid in "${HOST_PORT_RELAY_PIDS[@]:-}"; do
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  done
}

port_listener_uses_process() {
  local port="$1"
  local protocol="$2"
  local process_name="$3"
  local command_args

  case "${protocol^^}" in
    UDP)
      command_args=(-H -lunp "( sport = :${port} )")
      ;;
    *)
      command_args=(-H -ltnp "( sport = :${port} )")
      ;;
  esac

  ss "${command_args[@]}" 2>/dev/null \
    | awk -v process="${process_name}" '
      {
        address = $4
        if ((address ~ /^127[.]/ || address ~ /^0[.]0[.]0[.]0:/ ||
             address ~ /^\[::1\]:/ || address ~ /^\[::\]:/ ||
             address ~ /^\*:/) && index($0, process) > 0) {
          found = 1
        }
      }
      END { if (found) { exit 0 } exit 1 }
    '
}

find_available_host_relay_port() {
  local protocol="${1:-TCP}"
  local port

  for ((port = 20000; port <= 29999; port++)); do
    if ! is_port_listening "${port}" "${protocol}"; then
      printf '%d\n' "${port}"
      return 0
    fi
  done

  return 1
}

wait_for_host_port_relay() {
  local port="$1"
  local protocol="${2:-TCP}"
  local deadline=$((SECONDS + 10))

  while (( SECONDS < deadline )); do
    if is_port_listening "${port}" "${protocol}"; then
      return 0
    fi
    sleep 0.2
  done

  return 1
}

install_output_dnat_rule() {
  local listen_port="$1"
  local protocol="$2"
  local target_host="$3"
  local target_port="$4"
  local label="$5"
  local protocol_lc
  local output_rule_present="false"
  local -a rule_args

  protocol_lc="$(printf '%s' "${protocol}" | tr '[:upper:]' '[:lower:]')"
  rule_args=(
    -d 127.0.0.1/32
    -p "${protocol_lc}"
    -m "${protocol_lc}"
    --dport "${listen_port}"
    -j DNAT
    --to-destination "${target_host}:${target_port}"
  )

  if iptables -w -t nat -C OUTPUT "${rule_args[@]}" >/dev/null 2>&1; then
    output_rule_present="true"
  elif iptables -w -t nat -A OUTPUT "${rule_args[@]}" 2>/dev/null; then
    log "redirecting host port ${listen_port}/${protocol} -> ${target_host}:${target_port} for ${label} (iptables DNAT)"
    HOST_PORT_REDIRECT_RULES+=("${rule_args[*]}")
    output_rule_present="true"
  fi

  if [[ "${output_rule_present}" != "true" ]]; then
    return 1
  fi

  if iptables -w -t nat -C OUTPUT "${rule_args[@]}" >/dev/null 2>&1; then
    log "host DNAT for ${listen_port}/${protocol} -> ${target_host}:${target_port} present for ${label}"
    return 0
  fi

  return 0
}

install_output_redirect_rule() {
  local listen_port="$1"
  local target_port="$2"
  local label="$3"
  local -a rule_args

  rule_args=(
    -d 127.0.0.1/32
    -p tcp
    -m tcp
    --dport "${listen_port}"
    -j REDIRECT
    --to-ports "${target_port}"
  )

  if iptables -w -t nat -C OUTPUT "${rule_args[@]}" >/dev/null 2>&1; then
    log "host redirect for ${listen_port} -> ${target_port} already present for ${label}"
    return 0
  fi

  if iptables -w -t nat -A OUTPUT "${rule_args[@]}" 2>/dev/null; then
    log "redirecting host port ${listen_port} -> ${target_port} for ${label} (iptables REDIRECT)"
    HOST_PORT_REDIRECT_RULES+=("${rule_args[*]}")
    return 0
  fi

  return 1
}

kind_control_plane_ip() {
  docker inspect "${CLUSTER_NAME}-control-plane" \
    | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress'
}

kind_registry_ip() {
  docker inspect "${LOCAL_REGISTRY_NAME}" \
    | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress'
}

wait_for_local_registry() {
  local deadline=$((SECONDS + 30))

  while (( SECONDS < deadline )); do
    if curl -fsSL "http://127.0.0.1:${LOCAL_REGISTRY_PORT}/v2/" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  return 1
}

connect_registry_to_kind_network() {
  if ! docker network inspect kind >/dev/null 2>&1; then
    return
  fi

  if docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${LOCAL_REGISTRY_NAME}" | grep -q 'null'; then
    docker network connect kind "${LOCAL_REGISTRY_NAME}" >/dev/null 2>&1 || true
  fi
}

registry_storage_writable() {
  docker exec "${LOCAL_REGISTRY_NAME}" sh -c \
    'mkdir -p /var/lib/registry/docker/registry/v2/repositories &&
     touch /var/lib/registry/.nantian-gw-write-test &&
     rm -f /var/lib/registry/.nantian-gw-write-test' >/dev/null 2>&1
}

recreate_local_registry() {
  mkdir -p "${REGISTRY_DATA_DIR}"
  kind_image_sync_ensure_image_available \
    "${LOCAL_REGISTRY_IMAGE}" \
    "${LOCAL_REGISTRY_IMAGE}" \
    "${LOCAL_REGISTRY_MIRROR}" \
    "m.daocloud.io/docker.io/registry:2" \
    "docker.1ms.run/library/registry:2" \
    "docker.1ms.run/registry:2" || exit 1

  docker rm -f "${LOCAL_REGISTRY_NAME}" >/dev/null 2>&1 || true
  docker run -d \
    --restart=always \
    -p "127.0.0.1:${LOCAL_REGISTRY_PORT}:5000" \
    -v "${REGISTRY_DATA_DIR}:/var/lib/registry" \
    --name "${LOCAL_REGISTRY_NAME}" \
    "${LOCAL_REGISTRY_IMAGE}" >/dev/null

  if ! wait_for_local_registry; then
    log "local registry ${LOCAL_REGISTRY_NAME} is not reachable on 127.0.0.1:${LOCAL_REGISTRY_PORT}"
    exit 1
  fi

  connect_registry_to_kind_network
}

ensure_local_registry_storage() {
  if registry_storage_writable; then
    return
  fi

  log "local registry ${LOCAL_REGISTRY_NAME} storage is not writable; recreating registry"
  recreate_local_registry
}

ensure_kind_registry_hosts() {
  local registry_ip
  local node
  local node_registry_ip

  registry_ip="$(kind_registry_ip)"
  if [[ -z "${registry_ip}" || "${registry_ip}" == "null" ]]; then
    log "failed to determine kind registry IP for ${LOCAL_REGISTRY_NAME}"
    exit 1
  fi

  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    node_registry_ip="$(docker exec "${node}" getent hosts "${LOCAL_REGISTRY_NAME}" 2>/dev/null | awk '{print $1}' | tail -n 1 || true)"
    if [[ "${node_registry_ip}" == "${registry_ip}" ]]; then
      continue
    fi

    log "updating ${LOCAL_REGISTRY_NAME} host entry on ${node}"
    docker exec "${node}" sh -c \
      "awk '\$2 != \"${LOCAL_REGISTRY_NAME}\" { print }' /etc/hosts > /tmp/hosts.pgw && \
       cat /tmp/hosts.pgw > /etc/hosts && \
       printf '%s\t%s\n' '${registry_ip}' '${LOCAL_REGISTRY_NAME}' >> /etc/hosts"
  done
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
        local node_port
        node_port="$((32000 + (port % 1000)))"
        if (( node_port > 32767 )); then
          node_port="$((node_port - 1000))"
        fi
        printf '%d\n' "${node_port}"
      fi
      ;;
  esac
}

start_host_port_relay() {
  local listen_port="$1"
  local protocol="$2"
  local target_host="$3"
  local target_port="$4"
  local label="$5"
  local relay_key="${protocol^^}:${listen_port}"
  local listen_spec
  local relay_port
  local target_spec

  if [[ "${ENABLE_HOST_PORT_RELAYS}" != "true" ]]; then
    return
  fi

  if [[ -n "${HOST_PORT_RELAYS[${relay_key}]:-}" ]]; then
    return
  fi

  if is_port_listening "${listen_port}" "${protocol}"; then
    if port_listener_uses_process "${listen_port}" "${protocol}" "docker-proxy"; then
      log "host port ${listen_port}/${protocol} already available for ${label} via docker-proxy"
      HOST_PORT_RELAYS["${relay_key}"]="docker-proxy"
      return
    fi

    relay_port="$(find_available_host_relay_port "${protocol}")" || {
      log "failed to find a free local relay port for occupied host port ${listen_port}/${protocol}"
      exit 1
    }
    start_host_port_relay "${relay_port}" "${protocol}" "${target_host}" "${target_port}" "${label} relay"
    if install_output_dnat_rule "${listen_port}" "${protocol}" "127.0.0.1" "${relay_port}" "${label}"; then
      HOST_PORT_RELAYS["${relay_key}"]="iptables-dnat:${relay_port}"
      return
    fi

    log "host port ${listen_port}/${protocol} is already in use for ${label}, and localhost DNAT to relay port ${relay_port} could not be installed"
    exit 1
  fi

  case "${protocol^^}" in
    UDP)
      listen_spec="UDP-LISTEN:${listen_port},bind=127.0.0.1,reuseaddr,fork"
      target_spec="UDP:${target_host}:${target_port}"
      ;;
    *)
      listen_spec="TCP-LISTEN:${listen_port},bind=127.0.0.1,reuseaddr,fork"
      target_spec="TCP:${target_host}:${target_port}"
      ;;
  esac

  log "bridging host port ${listen_port}/${protocol} -> ${target_host}:${target_port} for ${label}"
  socat "${listen_spec}" "${target_spec}" >/dev/null 2>&1 &
  HOST_PORT_RELAY_PIDS+=("$!")
  HOST_PORT_RELAYS["${relay_key}"]="$!"

  if ! wait_for_host_port_relay "${listen_port}" "${protocol}"; then
    log "timed out waiting for host relay on port ${listen_port}/${protocol}"
    exit 1
  fi
}

ensure_host_port_relay() {
  local listen_port="$1"
  local target_port="$2"
  local label="$3"

  if is_port_listening "${listen_port}"; then
    log "host port ${listen_port} already available for ${label}"
    return
  fi

  if ! is_port_listening "${target_port}"; then
    log "host port ${listen_port} is unavailable and fallback target ${target_port} is not listening"
    exit 1
  fi

  start_host_port_relay "${listen_port}" "TCP" "127.0.0.1" "${target_port}" "${label}"
}

ensure_local_redirect() {
  local listen_port="$1"
  local target_port="$2"
  local label="$3"
  local listen_available="false"

  if [[ "${ENABLE_HOST_PORT_RELAYS}" != "true" ]]; then
    return
  fi

  if is_port_listening "${listen_port}"; then
    listen_available="true"
  fi

  if ! is_port_listening "${target_port}"; then
    if [[ "${listen_available}" == "true" ]]; then
      log "host port ${listen_port} already available for ${label}"
      return
    fi
    log "host port ${listen_port} is unavailable and redirect target ${target_port} is not listening"
    exit 1
  fi

  if install_output_dnat_rule "${listen_port}" "TCP" "127.0.0.1" "${target_port}" "${label}"; then
    return
  fi

  if install_output_redirect_rule "${listen_port}" "${target_port}" "${label}"; then
    return
  fi

  if [[ "${listen_available}" == "true" ]]; then
    log "host port ${listen_port} is already in use for ${label}, and iptables redirect to ${target_port} could not be installed"
    exit 1
  fi

  # iptables redirect is unavailable; fall back to a local socat relay.
  log "iptables REDIRECT unavailable; using socat relay ${listen_port} -> ${target_port} for ${label}"
  socat "TCP-LISTEN:${listen_port},fork,reuseaddr,bind=127.0.0.1" "TCP:127.0.0.1:${target_port}" &
  HOST_PORT_RELAY_PIDS+=("$!")
}

ensure_predicted_listener_relays() {
  local node_ip
  local port
  local target_port

  if [[ "${ENABLE_HOST_PORT_RELAYS}" != "true" ]]; then
    return
  fi

  node_ip="$(kind_control_plane_ip)"
  if [[ -z "${node_ip}" || "${node_ip}" == "null" ]]; then
    log "failed to determine kind control-plane IP for additional listener relays"
    exit 1
  fi

  IFS=',' read -r -a tcp_ports <<<"${ADDITIONAL_TCP_LISTENER_PORTS}"
  for port in "${tcp_ports[@]}"; do
    port="${port//[[:space:]]/}"
    [[ -z "${port}" ]] && continue
    if [[ "${port}" == "${HTTP_HOST_PORT}" || "${port}" == "${HTTPS_HOST_PORT}" ]]; then
      continue
    fi
    target_port="$(shared_node_port_for "${port}" "TCP")"
    start_host_port_relay "${port}" "TCP" "${node_ip}" "${target_port}" "listener tcp"
  done

  IFS=',' read -r -a udp_ports <<<"${ADDITIONAL_UDP_LISTENER_PORTS}"
  for port in "${udp_ports[@]}"; do
    port="${port//[[:space:]]/}"
    [[ -z "${port}" ]] && continue
    target_port="$(shared_node_port_for "${port}" "UDP")"
    start_host_port_relay "${port}" "UDP" "${node_ip}" "${target_port}" "listener udp"
  done
}

ensure_host_port_relays() {
  ensure_local_redirect "${HTTP_HOST_PORT}" "${HTTP_FALLBACK_PORT}" "http"
  ensure_local_redirect "${HTTPS_HOST_PORT}" "${HTTPS_FALLBACK_PORT}" "https"
  ensure_predicted_listener_relays
}

escape_sed_replacement() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

ensure_gateway_api_repo() {
  if [[ -d "${WORK_DIR}/.git" ]]; then
    log "reusing cached gateway-api repo ${WORK_DIR}"
    git -C "${WORK_DIR}" reset --hard HEAD >/dev/null
    git -C "${WORK_DIR}" clean -fd >/dev/null
    return
  fi

  clone_gateway_api_repo
}

clone_gateway_api_repo() {
  local clone_status=1
  local clone_url
  local clone_urls=()

  IFS=',' read -r -a clone_urls <<<"${GATEWAY_API_CLONE_URLS}"
  for clone_url in "${clone_urls[@]}"; do
    clone_url="${clone_url//[[:space:]]/}"
    [[ -n "${clone_url}" ]] || continue

    rm -rf "${WORK_DIR}"
    log "cloning gateway-api ${GATEWAY_API_VERSION} from ${clone_url}"
    if git clone --depth=1 --branch "${GATEWAY_API_VERSION}" "${clone_url}" "${WORK_DIR}"; then
      return
    else
      clone_status=$?
      log "clone failed from ${clone_url}; trying next configured URL"
    fi
  done

  log "failed to clone gateway-api ${GATEWAY_API_VERSION}; checked GATEWAY_API_CLONE_URLS=${GATEWAY_API_CLONE_URLS}"
  return "${clone_status}"
}

bool_flag() {
  local value="$1"
  if [[ "${value}" == "true" ]]; then
    printf 'true'
  else
    printf 'false'
  fi
}

ensure_local_registry() {
  mkdir -p "${REGISTRY_DATA_DIR}"

  if ! docker inspect "${LOCAL_REGISTRY_NAME}" >/dev/null 2>&1; then
    log "creating local registry ${LOCAL_REGISTRY_NAME}"
    recreate_local_registry
  elif [[ "$(docker inspect -f '{{.State.Running}}' "${LOCAL_REGISTRY_NAME}")" != "true" ]]; then
    log "starting local registry ${LOCAL_REGISTRY_NAME}"
    docker start "${LOCAL_REGISTRY_NAME}" >/dev/null
  fi

  if ! wait_for_local_registry; then
    log "local registry ${LOCAL_REGISTRY_NAME} is not reachable; recreating registry"
    recreate_local_registry
  fi

  ensure_local_registry_storage
  connect_registry_to_kind_network
}

cleanup_conformance_namespaces() {
  local namespace
  local deleted_any=false

  if [[ "${RESET_CONFORMANCE_NAMESPACES}" != "true" ]]; then
    return
  fi

  for namespace in "${CONFORMANCE_NAMESPACES[@]}"; do
    if ! kubectl --context "${KUBE_CONTEXT}" get namespace "${namespace}" >/dev/null 2>&1; then
      continue
    fi

    log "deleting stale conformance namespace ${namespace}"
    kubectl --context "${KUBE_CONTEXT}" delete namespace "${namespace}" --wait=false >/dev/null || true
    deleted_any=true
  done

  if [[ "${deleted_any}" != "true" ]]; then
    return
  fi

  for namespace in "${CONFORMANCE_NAMESPACES[@]}"; do
    if ! kubectl --context "${KUBE_CONTEXT}" get namespace "${namespace}" >/dev/null 2>&1; then
      continue
    fi

    if ! timeout 120 bash -c \
      "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${namespace}' >/dev/null 2>&1; do sleep 2; done"
    then
      log "forcing cleanup for namespace ${namespace}"
      kubectl --context "${KUBE_CONTEXT}" -n "${namespace}" \
        delete pod --all --force --grace-period=0 >/dev/null 2>&1 || true
      kubectl --context "${KUBE_CONTEXT}" get namespace "${namespace}" -o json \
        | jq '{apiVersion, kind, metadata: {name: .metadata.name}, spec: {finalizers: []}}' \
        | kubectl --context "${KUBE_CONTEXT}" replace --raw "/api/v1/namespaces/${namespace}/finalize" -f - >/dev/null 2>&1 || true

      if ! timeout 30 bash -c \
        "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${namespace}' >/dev/null 2>&1; do sleep 2; done"
      then
        log "namespace ${namespace} is still terminating after force cleanup"
      fi
    fi
  done
}

sync_image_to_local_registry() {
  local source_image="$1"
  local target_image="$2"
  shift 2

  local push_image

  ensure_local_registry_storage
  push_image="$(local_registry_push_image_for "${target_image}")"

  KIND_IMAGE_SYNC_LOCAL_REGISTRY="${LOCAL_REGISTRY_PUSH_HOST}" \
    kind_image_sync_ensure_registry_copy "${source_image}" "${push_image}" "$@"
}

echo_basic_image_available_locally() {
  local source_image="$1"
  local upstream_image="$2"

  docker image inspect "${source_image}" >/dev/null 2>&1 \
    || docker image inspect "${upstream_image}" >/dev/null 2>&1
}

build_echo_basic_image_from_source() {
  local target_image="$1"
  local tag="$2"
  local push_image
  local output_dir
  local source_dir

  push_image="$(local_registry_push_image_for "${target_image}")"
  output_dir="${TMP_DIR}/kind/conformance-image-build/echo-basic-${tag}"
  source_dir="${output_dir}/src"

  log "building conformance echo-basic ${tag} from ${WORK_DIR}/conformance/echo-basic"
  rm -rf "${output_dir}"
  mkdir -p "${source_dir}"
  cp -a "${WORK_DIR}/conformance/echo-basic/." "${source_dir}/"

  (
    cd "${source_dir}"
    mv -f .go.mod go.mod
    mv -f .go.sum go.sum
    CGO_ENABLED=0 go build -trimpath -ldflags="-buildid= -s -w" -o "${output_dir}/echo-basic" .
  )

  cat >"${output_dir}/Dockerfile" <<'EOF'
FROM scratch
COPY echo-basic /echo-basic
USER 65532:65532
ENTRYPOINT ["/echo-basic"]
EOF

  docker build \
    -f "${output_dir}/Dockerfile" \
    -t "${push_image}" \
    "${output_dir}" >/dev/null
  docker push "${push_image}" >/dev/null
}

discover_gateway_api_image_tags() {
  local image_name="$1"

  grep -RhoE "gcr\\.io/k8s-staging-gateway-api/${image_name}:[^[:space:]\"']+" \
    "${WORK_DIR}/conformance" 2>/dev/null \
    | awk -F: '{print $NF}' \
    | sort -u
}

sync_gateway_api_echo_images() {
  local image_name="$1"
  local source_repository="$2"
  local target_repository="$3"
  local source_image
  local tag
  local target_image
  local tags=()
  local upstream_image

  while IFS= read -r tag; do
    [[ -n "${tag}" ]] || continue
    tags+=("${tag}")
  done < <(discover_gateway_api_image_tags "${image_name}")

  if [[ ${#tags[@]} -eq 0 ]]; then
    log "no conformance ${image_name} images found under ${WORK_DIR}/conformance"
    return
  fi

  for tag in "${tags[@]}"; do
    source_image="${source_repository}:${tag}"
    target_image="${target_repository}:${tag}"
    upstream_image="gcr.io/k8s-staging-gateway-api/${image_name}:${tag}"

    if [[ "${image_name}" == "echo-basic" ]] && ! echo_basic_image_available_locally "${source_image}" "${upstream_image}"; then
      build_echo_basic_image_from_source "${target_image}" "${tag}"
    else
      sync_image_to_local_registry \
        "${source_image}" \
        "${target_image}" \
        "${upstream_image}"
    fi
    CONFORMANCE_LOCAL_IMAGES+=("${target_image}")
  done
}

sync_conformance_images_to_local_registry() {
  CONFORMANCE_LOCAL_IMAGES=()

  sync_gateway_api_echo_images \
    "echo-basic" \
    "${ECHO_BASIC_SOURCE_REPOSITORY}" \
    "${ECHO_BASIC_IMAGE_REPOSITORY}"
  sync_gateway_api_echo_images \
    "echo-advanced" \
    "${ECHO_ADVANCED_SOURCE_REPOSITORY}" \
    "${ECHO_ADVANCED_IMAGE_REPOSITORY}"
  sync_image_to_local_registry "${COREDNS_SOURCE_IMAGE}" "${COREDNS_IMAGE}"
  CONFORMANCE_LOCAL_IMAGES+=("${COREDNS_IMAGE}")
}

preload_kind_images() {
  local node
  local image

  if [[ "${PRELOAD_KIND_IMAGES}" != "true" ]]; then
    return
  fi

  log "preloading conformance images into kind nodes via crictl"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    for image in "${CONFORMANCE_LOCAL_IMAGES[@]}"; do
      docker exec "${node}" crictl pull "${image}" >/dev/null
    done
  done
}

rewrite_conformance_images() {
  local conformance_manifest
  local echo_basic_repository_replacement
  local echo_advanced_repository_replacement
  local coredns_replacement

  echo_basic_repository_replacement="$(escape_sed_replacement "${ECHO_BASIC_IMAGE_REPOSITORY}")"
  echo_advanced_repository_replacement="$(escape_sed_replacement "${ECHO_ADVANCED_IMAGE_REPOSITORY}")"
  coredns_replacement="$(escape_sed_replacement "${COREDNS_IMAGE}")"

  while IFS= read -r -d '' conformance_manifest; do
    sed -E -i \
      -e "s|gcr\.io/k8s-staging-gateway-api/echo-basic:([^[:space:]\"']+)|${echo_basic_repository_replacement}:\\1|g" \
      -e "s|[[:alnum:].:-]+/gateway-api-conformance/echo-basic:([^[:space:]\"']+)|${echo_basic_repository_replacement}:\\1|g" \
      -e "s|gcr\.io/k8s-staging-gateway-api/echo-advanced:([^[:space:]\"']+)|${echo_advanced_repository_replacement}:\\1|g" \
      -e "s|[[:alnum:].:-]+/gateway-api-conformance/echo-advanced:([^[:space:]\"']+)|${echo_advanced_repository_replacement}:\\1|g" \
      -e "s|registry\.k8s\.io/coredns/coredns(:[^[:space:]\"']+)?|${coredns_replacement}|g" \
      -e "s|docker\.io/coredns/coredns(:[^[:space:]\"']+)?|${coredns_replacement}|g" \
      -e "s|coredns/coredns(:[^[:space:]\"']+)?|${coredns_replacement}|g" \
      -e "s|[[:alnum:].:-]+/gateway-api-conformance/coredns:[^[:space:]\"']+|${coredns_replacement}|g" \
      "${conformance_manifest}"
  done < <(find "${WORK_DIR}/conformance" -type f \( -name '*.yaml' -o -name '*.yml' \) -print0)
}

strip_gateway_api_bundle_annotations() {
  local manifest="$1"

  if [[ ! -f "${manifest}" ]]; then
    return
  fi

  sed -E -i \
    -e "s|(gateway\.networking\.k8s\.io/bundle-version:[[:space:]]*).*$|\\1${GATEWAY_API_VERSION}|" \
    "${manifest}"
}

ensure_gateway_api_crds() {
  local manifest
  local modcache
  local backendlb_module_dir
  local backendlb_manifest

  backendlb_manifest="${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml"
  modcache="$(cd "${ROOT_DIR}/controlplane" && go env GOMODCACHE 2>/dev/null || true)"
  backendlb_module_dir=""
  if [[ -n "${modcache}" ]]; then
    backendlb_module_dir="${modcache}/sigs.k8s.io/gateway-api@${BACKENDLBPOLICY_CRD_VERSION}"
  fi
  if [[ -n "${backendlb_module_dir}" && -f "${backendlb_module_dir}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml" ]]; then
    cp \
      "${backendlb_module_dir}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml" \
      "${backendlb_manifest}"
  else
    curl -fsSL \
      "https://gh-proxy.com/https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${BACKENDLBPOLICY_CRD_VERSION}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml" \
      -o "${backendlb_manifest}"
  fi
  strip_gateway_api_bundle_annotations "${backendlb_manifest}"

  for manifest in \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_gatewayclasses.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_gateways.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_httproutes.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_grpcroutes.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_backendtlspolicies.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_referencegrants.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_listenersets.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_tcproutes.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_tlsroutes.yaml" \
    "${WORK_DIR}/config/crd/experimental/gateway.networking.k8s.io_udproutes.yaml"; do
    kubectl --context "${KUBE_CONTEXT}" apply --server-side --force-conflicts -f "${manifest}" >/dev/null
  done
}

wait_conformance_readiness() {
  local kube_context="$1"
  local ns

  for ns in "${CONFORMANCE_NAMESPACES[@]}"; do
    log "waiting for gateways to become Programmed in ${ns}"
    kubectl --context "${kube_context}" wait gateway --all \
      -n "${ns}" \
      --for=condition=Programmed \
      --timeout=120s 2>/dev/null || log "no gateways (yet) in ${ns}"
  done

  log "waiting for infra backend pods to become Ready"
  kubectl --context "${kube_context}" wait pod --all \
    -n gateway-conformance-infra \
    --for=condition=Ready \
    --timeout=120s 2>/dev/null || log "no infra pods to wait for"
}

main() {
  local status

  ensure_gateway_api_repo
  ensure_local_registry
  ensure_kind_registry_hosts
  trap cleanup_host_port_relays EXIT
  kubectl config use-context "${KUBE_CONTEXT}" >/dev/null
  ensure_gateway_api_crds
  cleanup_conformance_namespaces
  cleanup_smoke_resources
  ensure_clean_gateway_resources
  ensure_gateway_class
  sync_conformance_images_to_local_registry
  preload_kind_images
  rewrite_conformance_images
  ensure_host_port_relays
  conformance_prepare_report_artifacts \
    "${ROOT_DIR}" \
    "${REPORT_DIR}" \
    "${REPORT_OUTPUT}" \
    "${CONFORMANCE_LOG_PATH}" \
    "${REPORT_METADATA_PATH}"
  conformance_write_metadata "running" "preparing conformance harness"
  derive_supported_features_from_controlplane

  FLAGS=(
    "-gateway-class=${GATEWAY_CLASS}"
    "-cleanup-base-resources=$(bool_flag "${CLEANUP_BASE_RESOURCES}")"
    "-debug=$(bool_flag "${DEBUG}")"
    "-allow-crds-mismatch=$(bool_flag "${ALLOW_CRDS_MISMATCH}")"
    "-skip-provisional-tests=$(bool_flag "${SKIP_PROVISIONAL_TESTS}")"
    "-organization=${ORGANIZATION}"
    "-project=${PROJECT}"
    "-url=${IMPLEMENTATION_URL}"
    "-version=${IMPLEMENTATION_VERSION}"
    "-contact=${IMPLEMENTATION_CONTACT}"
    "-report-output=${REPORT_OUTPUT}"
  )

  if [[ -n "${RUN_TEST}" ]]; then
    FLAGS+=("-run-test=${RUN_TEST}")
  fi

  if [[ -n "${SKIP_TESTS}" ]]; then
    FLAGS+=("-skip-tests=${SKIP_TESTS}")
  fi

  if [[ -n "${SUPPORTED_FEATURES}" ]]; then
    FLAGS+=("-supported-features=${SUPPORTED_FEATURES}")
  fi

  if [[ -n "${EXEMPT_FEATURES}" ]]; then
    FLAGS+=("-exempt-features=${EXEMPT_FEATURES}")
  fi

  if [[ -n "${CONFORMANCE_PROFILES}" ]]; then
    FLAGS+=("-conformance-profiles=${CONFORMANCE_PROFILES}")
  fi

  if [[ "${ALL_FEATURES}" == "true" ]]; then
    FLAGS+=("-all-features=true")
  fi

  if [[ -z "${RUN_TEST}" && -z "${SUPPORTED_FEATURES}" && -z "${CONFORMANCE_PROFILES}" && "${ALL_FEATURES}" != "true" ]]; then
    FLAGS+=("-conformance-profiles=GATEWAY-HTTP")
  fi

  # Ensure conformance infrastructure is ready before tests start.
  # This eliminates cold-start transient retries (not_ready_yet / response_expectation_failed)
  # caused by Gateway Programmed latency and backend pod warm-up.
  log "waiting for conformance infrastructure readiness"
  wait_conformance_readiness "${KUBE_CONTEXT}"

  log "running gateway-api conformance from ${WORK_DIR}"
  set +e
  (
    cd "${HARNESS_DIR}"
    GATEWAY_API_WORK_DIR="${WORK_DIR}" \
    CONFORMANCE_USABLE_ADDRESSES="${CONFORMANCE_USABLE_ADDRESSES}" \
    CONFORMANCE_UNUSABLE_ADDRESSES="${CONFORMANCE_UNUSABLE_ADDRESSES}" \
    ALLOW_FOREIGN_GATEWAY_RESOURCES="${ALLOW_FOREIGN_GATEWAY_RESOURCES}" \
    GOWORK=off \
    go test -mod=mod \
      -count=1 \
      -timeout "${GO_TEST_TIMEOUT}" \
      -v \
      -args "${FLAGS[@]}"
   ) 2>&1 | tee "${CONFORMANCE_LOG_PATH}"
  status=${PIPESTATUS[0]}
  set -e

  if [[ ${status} -ne 0 ]]; then
    conformance_write_metadata "failed" "go test failed; see ${CONFORMANCE_LOG_PATH}"
    exit "${status}"
  fi

  if [[ ! -f "${REPORT_OUTPUT}" ]]; then
    conformance_write_metadata "failed" "report output missing after successful go test"
    log "go test completed without writing report output: ${REPORT_OUTPUT}"
    log "conformance log preserved at ${CONFORMANCE_LOG_PATH}"
    exit 1
  fi

  conformance_write_metadata "passed" "report and log artifacts captured"

  log "report written to ${REPORT_OUTPUT}"
  log "conformance log written to ${CONFORMANCE_LOG_PATH}"
}

if [[ "${CONFORMANCE_RUN_SH_SOURCE_ONLY:-false}" != "true" ]]; then
  main "$@"
fi
