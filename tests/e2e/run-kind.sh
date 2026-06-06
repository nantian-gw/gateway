#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/scripts/lib/kind-image-sync.sh"
KIND_CACHE_DIR="${ROOT_DIR}/tmp/kind"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.5.1}"
BACKENDLBPOLICY_CRD_VERSION="${BACKENDLBPOLICY_CRD_VERSION:-v1.2.1}"
CRD_CACHE_DIR="${KIND_CACHE_DIR}/gateway-api-crds/${GATEWAY_API_VERSION}"
LAST_TAG_FILE="${KIND_CACHE_DIR}/last-image-tag"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-gw}"
KUBE_CONTEXT="kind-${CLUSTER_NAME}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_IMAGE="${LOCAL_REGISTRY_IMAGE:-registry:2}"
LOCAL_REGISTRY_MIRROR="${LOCAL_REGISTRY_MIRROR:-m.daocloud.io/docker.io/library/registry:2}"
REGISTRY_DATA_DIR="${REGISTRY_DATA_DIR:-${KIND_CACHE_DIR}/registry-data}"
KIND_NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.34.0}"
KIND_NODE_MIRROR="${KIND_NODE_MIRROR:-m.daocloud.io/docker.io/kindest/node:v1.34.0}"
KIND_WORKER_NODES="${KIND_WORKER_NODES:-0}"
KIND_IP_FAMILY="${KIND_IP_FAMILY:-dual}"
CONTROLPLANE_GO_IMAGE="${CONTROLPLANE_GO_IMAGE:-m.daocloud.io/docker.io/library/golang:1.26-bookworm}"
DATAPLANE_RUST_IMAGE="${DATAPLANE_RUST_IMAGE:-m.daocloud.io/docker.io/library/rust:1.88-bookworm}"
DATAPLANE_CARGO_FEATURES="${DATAPLANE_CARGO_FEATURES:-allocator-jemalloc}"
RUNTIME_IMAGE="${RUNTIME_IMAGE:-m.daocloud.io/docker.io/library/debian:bookworm-slim}"
DOCKER_BUILD_NETWORK="${DOCKER_BUILD_NETWORK:-}"
SMOKE_SOURCE_IMAGE="${SMOKE_SOURCE_IMAGE:-m.daocloud.io/docker.io/hashicorp/http-echo:1.0.0}"
SMOKE_IMAGE_REPO="${SMOKE_IMAGE_REPO:-${LOCAL_REGISTRY_HOST}/hashicorp/http-echo}"
SMOKE_PUSH_IMAGE_REPO="${SMOKE_PUSH_IMAGE_REPO:-${LOCAL_REGISTRY_PUSH_HOST}/hashicorp/http-echo}"
SMOKE_ECHO_BASIC_SOURCE_IMAGE="${SMOKE_ECHO_BASIC_SOURCE_IMAGE:-m.daocloud.io/gcr.io/k8s-staging-gateway-api/echo-basic:v20240412-v1.0.0-394-g40c666fd}"
SMOKE_COREDNS_SOURCE_IMAGE="${SMOKE_COREDNS_SOURCE_IMAGE:-m.daocloud.io/docker.io/coredns/coredns:latest}"
RECREATE_CLUSTER="${RECREATE_CLUSTER:-false}"
SKIP_BUILD="${SKIP_BUILD:-false}"
SKIP_SMOKE="${SKIP_SMOKE:-false}"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-180s}"
KIND_IMAGE_SYNC_LOCAL_REGISTRY="${KIND_IMAGE_SYNC_LOCAL_REGISTRY:-${LOCAL_REGISTRY_PUSH_HOST}}"
KIND_IMAGE_SYNC_PULL_ATTEMPTS="${KIND_IMAGE_SYNC_PULL_ATTEMPTS:-3}"
KIND_IMAGE_SYNC_PUSH_ATTEMPTS="${KIND_IMAGE_SYNC_PUSH_ATTEMPTS:-3}"
KIND_IMAGE_SYNC_RETRY_DELAY_SECONDS="${KIND_IMAGE_SYNC_RETRY_DELAY_SECONDS:-2}"
SMOKE_HTTP_PORT="${SMOKE_HTTP_PORT:-18080}"
SMOKE_HTTPS_PORT="${SMOKE_HTTPS_PORT:-18443}"
SMOKE_UDP_PORT="${SMOKE_UDP_PORT:-5300}"
SMOKE_UDP_FAILURE_PORT="${SMOKE_UDP_FAILURE_PORT:-5301}"
SMOKE_UDP_FAILURE_LISTENER_PORT="${SMOKE_UDP_FAILURE_LISTENER_PORT:-5301}"
SMOKE_TCP_PORT="${SMOKE_TCP_PORT:-19000}"
SMOKE_ADMIN_PORT="${SMOKE_ADMIN_PORT:-29080}"
SMOKE_TCP_FAILURE_PORT="${SMOKE_TCP_FAILURE_PORT:-19001}"
SMOKE_TCP_LISTENER_PORT="${SMOKE_TCP_LISTENER_PORT:-9000}"
SMOKE_TCP_FAILURE_LISTENER_PORT="${SMOKE_TCP_FAILURE_LISTENER_PORT:-9001}"
SMOKE_GRPC_AUTHORITY="${SMOKE_GRPC_AUTHORITY:-grpc.example.com}"
SMOKE_GRPC_FAILURE_AUTHORITY="${SMOKE_GRPC_FAILURE_AUTHORITY:-missing.grpc.example.com}"
SMOKE_TLS_HOSTNAME="${SMOKE_TLS_HOSTNAME:-abc.example.com}"
SMOKE_TLS_FAILURE_HOSTNAME="${SMOKE_TLS_FAILURE_HOSTNAME:-missing.example.com}"
SMOKE_TLS_SECRET_NAME="${SMOKE_TLS_SECRET_NAME:-smoke-tls-passthrough-certificate}"

HOST_PORT_RELAY_PIDS=()
SMOKE_RUNTIME_DIR=""
SMOKE_ADMIN_PORT_FORWARD_PID=""
SMOKE_ADMIN_PORT_FORWARD_LOG=""

if [[ -z "${DOCKER_BUILD_NETWORK}" ]]; then
  if grep -Eq '^[[:space:]]*nameserver[[:space:]]+127\.' /etc/resolv.conf 2>/dev/null; then
    DOCKER_BUILD_NETWORK="host"
  else
    DOCKER_BUILD_NETWORK="default"
  fi
fi

log() {
  printf '[kind-e2e] %s\n' "$*"
}

wait_for_local_registry() {
  local attempt

  for attempt in $(seq 1 10); do
    if curl -fsSL "http://127.0.0.1:${LOCAL_REGISTRY_PORT}/v2/" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  return 1
}

cleanup_smoke_resources() {
  if ! kubectl --context "${KUBE_CONTEXT}" get namespace nantian-gw >/dev/null 2>&1; then
    return
  fi

  log "cleaning previous smoke resources"
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
    secret/"${SMOKE_TLS_SECRET_NAME}" \
    gateway.gateway.networking.k8s.io/edge \
    grpcroute.gateway.networking.k8s.io/grpc-echo \
    httproute.gateway.networking.k8s.io/echo \
    tcproute.gateway.networking.k8s.io/tcp-echo \
    tcproute.gateway.networking.k8s.io/tcp-missing \
    tlsroute.gateway.networking.k8s.io/tls-echo \
    udproute.gateway.networking.k8s.io/udp-coredns \
    udproute.gateway.networking.k8s.io/udp-missing \
    --ignore-not-found >/dev/null
}

cleanup_host_port_relays() {
  local pid

  for pid in "${HOST_PORT_RELAY_PIDS[@]:-}"; do
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  done
}

cleanup_transient_state() {
  if [[ -n "${SMOKE_ADMIN_PORT_FORWARD_PID}" ]]; then
    kill "${SMOKE_ADMIN_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${SMOKE_ADMIN_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  fi
  cleanup_host_port_relays
  if [[ -n "${SMOKE_RUNTIME_DIR}" && -d "${SMOKE_RUNTIME_DIR}" ]]; then
    rm -rf "${SMOKE_RUNTIME_DIR}"
  fi
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

start_host_port_relay() {
  local listen_port="$1"
  local protocol="$2"
  local target_host="$3"
  local target_port="$4"
  local label="$5"
  local listen_spec
  local target_spec

  if is_port_listening "${listen_port}" "${protocol}"; then
    log "host port ${listen_port}/${protocol} already available for ${label}"
    return
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

  if ! wait_for_host_port_relay "${listen_port}" "${protocol}"; then
    log "timed out waiting for host relay on ${listen_port}/${protocol}"
    exit 1
  fi
}

ensure_tcp_smoke_relay() {
  local node_ip
  local target_port

  node_ip="$(kind_control_plane_ip)"
  if [[ -z "${node_ip}" || "${node_ip}" == "null" ]]; then
    log "failed to determine kind control-plane IP for TCP smoke relay"
    exit 1
  fi

  target_port="$(shared_node_port_for "${SMOKE_TCP_LISTENER_PORT}" "TCP")"
  start_host_port_relay "${SMOKE_TCP_PORT}" "TCP" "${node_ip}" "${target_port}" "tcp smoke"
}

ensure_failure_smoke_relays() {
  local node_ip
  local tcp_target_port
  local udp_target_port

  node_ip="$(kind_control_plane_ip)"
  if [[ -z "${node_ip}" || "${node_ip}" == "null" ]]; then
    log "failed to determine kind control-plane IP for failure smoke relays"
    exit 1
  fi

  tcp_target_port="$(shared_node_port_for "${SMOKE_TCP_FAILURE_LISTENER_PORT}" "TCP")"
  udp_target_port="$(shared_node_port_for "${SMOKE_UDP_FAILURE_LISTENER_PORT}" "UDP")"
  start_host_port_relay "${SMOKE_TCP_FAILURE_PORT}" "TCP" "${node_ip}" "${tcp_target_port}" "tcp failure smoke"
  start_host_port_relay "${SMOKE_UDP_FAILURE_PORT}" "UDP" "${node_ip}" "${udp_target_port}" "udp failure smoke"
}

prepare_smoke_tls_secret() {
  local cert_path
  local key_path
  local openssl_config

  if [[ -z "${SMOKE_RUNTIME_DIR}" ]]; then
    SMOKE_RUNTIME_DIR="$(mktemp -d "${KIND_CACHE_DIR}/smoke.XXXXXX")"
  fi

  cert_path="${SMOKE_RUNTIME_DIR}/tls.crt"
  key_path="${SMOKE_RUNTIME_DIR}/tls.key"
  openssl_config="${SMOKE_RUNTIME_DIR}/openssl.cnf"

  cat >"${openssl_config}" <<EOF
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = ${SMOKE_TLS_HOSTNAME}

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${SMOKE_TLS_HOSTNAME}
EOF

  openssl req \
    -x509 \
    -nodes \
    -newkey rsa:2048 \
    -days 7 \
    -keyout "${key_path}" \
    -out "${cert_path}" \
    -config "${openssl_config}" \
    -extensions v3_req >/dev/null 2>&1

  kubectl --context "${KUBE_CONTEXT}" -n nantian-gw create secret tls "${SMOKE_TLS_SECRET_NAME}" \
    --cert="${cert_path}" \
    --key="${key_path}" \
    --dry-run=client \
    -o yaml | kubectl --context "${KUBE_CONTEXT}" apply -f - >/dev/null
}

retry_probe() {
  local description="$1"
  local attempts="$2"
  local delay_seconds="$3"
  shift 3

  local try
  for try in $(seq 1 "${attempts}"); do
    if "$@"; then
      log "${description} smoke check passed"
      return 0
    fi
    sleep "${delay_seconds}"
  done

  log "${description} smoke check failed"
  return 1
}

probe_http() {
  curl -fsS -H 'Host: example.com' "http://127.0.0.1:${SMOKE_HTTP_PORT}/" | grep -q "nantian-gw-ok"
}

probe_grpc() {
  (
    cd "${ROOT_DIR}/controlplane"
    go run ./cmd/grpc-smoke-client \
      -addr "127.0.0.1:${SMOKE_HTTP_PORT}" \
      -authority "${SMOKE_GRPC_AUTHORITY}"
  ) >/dev/null
}

probe_tcp() {
  printf 'GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n' \
    | nc -w 5 127.0.0.1 "${SMOKE_TCP_PORT}" | grep -q "nantian-gw-ok"
}

probe_tls() {
  curl -sk --resolve "${SMOKE_TLS_HOSTNAME}:${SMOKE_HTTPS_PORT}:127.0.0.1" \
    "https://${SMOKE_TLS_HOSTNAME}:${SMOKE_HTTPS_PORT}/" \
    | jq -e --arg hostname "${SMOKE_TLS_HOSTNAME}" '.tls.serverName == $hostname' >/dev/null
}

probe_udp() {
  python3 "${ROOT_DIR}/tests/e2e/udp_dns_smoke.py" \
    --addr "127.0.0.1:${SMOKE_UDP_PORT}" \
    --name foo.bar.com >/dev/null
}

pick_smoke_admin_forward_port() {
  local candidate="${SMOKE_ADMIN_PORT}"

  while is_port_listening "${candidate}" "TCP"; do
    candidate=$((candidate + 1))
  done

  SMOKE_ADMIN_PORT="${candidate}"
}

start_smoke_admin_port_forward() {
  pick_smoke_admin_forward_port

  if [[ -z "${SMOKE_RUNTIME_DIR}" ]]; then
    SMOKE_RUNTIME_DIR="$(mktemp -d "${KIND_CACHE_DIR}/smoke.XXXXXX")"
  fi

  SMOKE_ADMIN_PORT_FORWARD_LOG="${SMOKE_RUNTIME_DIR}/admin-port-forward.log"
  kubectl --context "${KUBE_CONTEXT}" -n nantian-gw \
    port-forward service/nantian-dataplane-admin "${SMOKE_ADMIN_PORT}:19080" \
    >"${SMOKE_ADMIN_PORT_FORWARD_LOG}" 2>&1 &
  SMOKE_ADMIN_PORT_FORWARD_PID="$!"

  for _ in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:${SMOKE_ADMIN_PORT}/livez" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  log "timed out waiting for dataplane admin port-forward"
  cat "${SMOKE_ADMIN_PORT_FORWARD_LOG}" >&2 || true
  exit 1
}

stop_smoke_admin_port_forward() {
  if [[ -z "${SMOKE_ADMIN_PORT_FORWARD_PID}" ]]; then
    return
  fi

  kill "${SMOKE_ADMIN_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  wait "${SMOKE_ADMIN_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  SMOKE_ADMIN_PORT_FORWARD_PID=""
}

probe_metrics() {
  local metrics

  metrics="$(curl -fsS "http://127.0.0.1:${SMOKE_ADMIN_PORT}/metrics")"
  grep -q '^# HELP nantian_gateway_dataplane_ready ' <<<"${metrics}"
  grep -q '^nantian_gateway_dataplane_ready ' <<<"${metrics}"
  awk '
    /^# HELP / {
      if (current != "" && has_sample == 0) {
        exit 1
      }
      current = $3
      has_sample = 0
      next
    }
    /^#/ || NF == 0 {
      next
    }
    {
      has_sample = 1
    }
    END {
      if (current != "" && has_sample == 0) {
        exit 1
      }
    }
  ' <<<"${metrics}"
}

run_smoke_checks() {
  retry_probe "http" 30 2 probe_http
  retry_probe "grpc" 30 2 probe_grpc
  retry_probe "tcp" 30 2 probe_tcp
  retry_probe "tls" 30 2 probe_tls
  retry_probe "udp" 30 2 probe_udp
  start_smoke_admin_port_forward
  retry_probe "metrics" 15 1 probe_metrics
  stop_smoke_admin_port_forward
}

probe_http_unmatched_host() {
  ! curl -fsS -H 'Host: missing.example.com' "http://127.0.0.1:${SMOKE_HTTP_PORT}/" >/dev/null 2>&1
}

probe_grpc_unmatched_host() {
  ! (
    cd "${ROOT_DIR}/controlplane"
    go run ./cmd/grpc-smoke-client \
      -addr "127.0.0.1:${SMOKE_HTTP_PORT}" \
      -authority "${SMOKE_GRPC_FAILURE_AUTHORITY}"
  ) >/dev/null 2>&1
}

probe_tcp_missing_backend() {
  local response
  response="$(
    timeout 5 bash -lc \
      "printf 'GET / HTTP/1.1\r\nHost: missing.example.com\r\nConnection: close\r\n\r\n' | nc -w 2 127.0.0.1 ${SMOKE_TCP_FAILURE_PORT}" \
      2>/dev/null || true
  )"
  [[ -z "${response}" ]]
}

probe_tls_unmatched_sni() {
  ! curl -sk \
    --connect-timeout 2 \
    --max-time 5 \
    --resolve "${SMOKE_TLS_FAILURE_HOSTNAME}:${SMOKE_HTTPS_PORT}:127.0.0.1" \
    "https://${SMOKE_TLS_FAILURE_HOSTNAME}:${SMOKE_HTTPS_PORT}/" >/dev/null 2>&1
}

probe_udp_missing_backend() {
  python3 "${ROOT_DIR}/tests/e2e/udp_dns_smoke.py" \
    --addr "127.0.0.1:${SMOKE_UDP_FAILURE_PORT}" \
    --name foo.bar.com \
    --timeout 2 \
    --expect-timeout >/dev/null
}

run_failure_checks() {
  retry_probe "http unmatched host" 10 1 probe_http_unmatched_host
  retry_probe "grpc unmatched host" 10 1 probe_grpc_unmatched_host
  retry_probe "tcp missing backend" 10 1 probe_tcp_missing_backend
  retry_probe "tls unmatched sni" 10 1 probe_tls_unmatched_sni
  retry_probe "udp missing backend" 10 1 probe_udp_missing_backend
}

if [[ "${SKIP_BUILD}" == "true" && -z "${IMAGE_TAG:-}" && ! -f "${LAST_TAG_FILE}" ]]; then
  log "SKIP_BUILD=true requires IMAGE_TAG or ${LAST_TAG_FILE}"
  exit 1
fi

if [[ -n "${IMAGE_TAG:-}" ]]; then
  RESOLVED_IMAGE_TAG="${IMAGE_TAG}"
elif [[ "${SKIP_BUILD}" == "true" && -f "${LAST_TAG_FILE}" ]]; then
  RESOLVED_IMAGE_TAG="$(cat "${LAST_TAG_FILE}")"
else
  RESOLVED_IMAGE_TAG="$(date +%Y%m%d%H%M%S)"
fi

CONTROL_IMAGE="${CONTROL_IMAGE:-${LOCAL_REGISTRY_HOST}/nantian-controlplane:${RESOLVED_IMAGE_TAG}}"
DATAPLANE_IMAGE="${DATAPLANE_IMAGE:-${LOCAL_REGISTRY_HOST}/nantian-dataplane:${RESOLVED_IMAGE_TAG}}"
CONTROL_PUSH_IMAGE="${CONTROL_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/nantian-controlplane:${RESOLVED_IMAGE_TAG}}"
DATAPLANE_PUSH_IMAGE="${DATAPLANE_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/nantian-dataplane:${RESOLVED_IMAGE_TAG}}"
SMOKE_IMAGE="${SMOKE_IMAGE:-${SMOKE_IMAGE_REPO}:1.0.0}"
SMOKE_PUSH_IMAGE="${SMOKE_PUSH_IMAGE:-${SMOKE_PUSH_IMAGE_REPO}:1.0.0}"
SMOKE_ECHO_BASIC_IMAGE="${SMOKE_ECHO_BASIC_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-basic:smoke}"
SMOKE_COREDNS_IMAGE="${SMOKE_COREDNS_IMAGE:-${LOCAL_REGISTRY_HOST}/gateway-api-conformance/coredns:smoke}"
SMOKE_ECHO_BASIC_PUSH_IMAGE="${SMOKE_ECHO_BASIC_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/gateway-api-conformance/echo-basic:smoke}"
SMOKE_COREDNS_PUSH_IMAGE="${SMOKE_COREDNS_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/gateway-api-conformance/coredns:smoke}"

escape_sed_replacement() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

ensure_image_available() {
  local target="$1"
  shift

  if ! kind_image_sync_ensure_image_available "${target}" "$@"; then
    exit 1
  fi
}

ensure_registry_copy() {
  local source_image="$1"
  local target_image="$2"
  shift 2

  if ! kind_image_sync_ensure_registry_copy "${source_image}" "${target_image}" "$@"; then
    exit 1
  fi
}

registry_tags() {
  local repository="$1"

  curl -fsSL "http://127.0.0.1:${LOCAL_REGISTRY_PORT}/v2/${repository}/tags/list" \
    | jq -r '.tags[]?' | sort -u
}

registry_has_tag() {
  local repository="$1"
  local tag="$2"

  registry_tags "${repository}" | grep -qx "${tag}"
}

latest_common_runtime_tag() {
  local common

  common="$(registry_tags nantian-controlplane)"
  [[ -n "${common}" ]] || return 1
  common="$(comm -12 <(printf '%s\n' "${common}") <(registry_tags nantian-dataplane))"
  [[ -n "${common}" ]] || return 1

  printf '%s\n' "${common}" | tail -n 1
}

refresh_runtime_image_refs() {
  CONTROL_IMAGE="${LOCAL_REGISTRY_HOST}/nantian-controlplane:${RESOLVED_IMAGE_TAG}"
  DATAPLANE_IMAGE="${LOCAL_REGISTRY_HOST}/nantian-dataplane:${RESOLVED_IMAGE_TAG}"
  CONTROL_PUSH_IMAGE="${LOCAL_REGISTRY_PUSH_HOST}/nantian-controlplane:${RESOLVED_IMAGE_TAG}"
  DATAPLANE_PUSH_IMAGE="${LOCAL_REGISTRY_PUSH_HOST}/nantian-dataplane:${RESOLVED_IMAGE_TAG}"
  SMOKE_IMAGE="${SMOKE_IMAGE_REPO}:1.0.0"
  SMOKE_PUSH_IMAGE="${SMOKE_PUSH_IMAGE_REPO}:1.0.0"
  SMOKE_ECHO_BASIC_IMAGE="${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-basic:smoke"
  SMOKE_COREDNS_IMAGE="${LOCAL_REGISTRY_HOST}/gateway-api-conformance/coredns:smoke"
  SMOKE_ECHO_BASIC_PUSH_IMAGE="${LOCAL_REGISTRY_PUSH_HOST}/gateway-api-conformance/echo-basic:smoke"
  SMOKE_COREDNS_PUSH_IMAGE="${LOCAL_REGISTRY_PUSH_HOST}/gateway-api-conformance/coredns:smoke"
}

resolve_runtime_image_tag() {
  local fallback_tag

  if [[ "${SKIP_BUILD}" != "true" ]]; then
    refresh_runtime_image_refs
    return
  fi

  if registry_has_tag nantian-controlplane "${RESOLVED_IMAGE_TAG}" \
    && registry_has_tag nantian-dataplane "${RESOLVED_IMAGE_TAG}"; then
    refresh_runtime_image_refs
    return
  fi

  if [[ -n "${IMAGE_TAG:-}" ]]; then
    log "requested IMAGE_TAG=${RESOLVED_IMAGE_TAG} is not present for all runtime images in ${LOCAL_REGISTRY_HOST}"
    exit 1
  fi

  fallback_tag="$(latest_common_runtime_tag || true)"
  if [[ -z "${fallback_tag}" ]]; then
    log "unable to find a common runtime image tag in ${LOCAL_REGISTRY_HOST}; rerun without SKIP_BUILD=true"
    exit 1
  fi

  if [[ "${fallback_tag}" != "${RESOLVED_IMAGE_TAG}" ]]; then
    log "runtime image tag ${RESOLVED_IMAGE_TAG} is incomplete; falling back to common tag ${fallback_tag}"
    RESOLVED_IMAGE_TAG="${fallback_tag}"
  fi

  refresh_runtime_image_refs
}

cluster_exists() {
  kind get clusters | grep -qx "${CLUSTER_NAME}"
}

cluster_supports_local_registry() {
  local node_name="${CLUSTER_NAME}-control-plane"

  docker inspect "${node_name}" >/dev/null 2>&1 || return 1
  docker exec "${node_name}" grep -q "${LOCAL_REGISTRY_HOST}" /etc/containerd/config.toml
}

cluster_supports_port_mappings() {
  local node_name="${CLUSTER_NAME}-control-plane"

  docker inspect "${node_name}" >/dev/null 2>&1 || return 1
  docker inspect "${node_name}" | jq -e '
    .[0].HostConfig.PortBindings as $bindings
    | ($bindings["30080/tcp"][0].HostPort == "18080")
    and ($bindings["30443/tcp"][0].HostPort == "18443")
    and ($bindings["31300/udp"][0].HostPort == "5300")
    and ($bindings["31301/udp"][0].HostPort == "5301")
    and ($bindings["32000/tcp"][0].HostPort == "19000")
    and ($bindings["32001/tcp"][0].HostPort == "19001")
  ' >/dev/null
}

refresh_kind_kubeconfig() {
  kind export kubeconfig --name "${CLUSTER_NAME}" >/dev/null
}

connect_registry_to_kind_network() {
  docker network inspect kind >/dev/null 2>&1 || return 0

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

kind_registry_ip() {
  docker inspect "${LOCAL_REGISTRY_NAME}" \
    | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress'
}

ensure_kind_registry_hosts() {
  local registry_ip
  local node

  registry_ip="$(kind_registry_ip)"
  if [[ -z "${registry_ip}" || "${registry_ip}" == "null" ]]; then
    log "failed to determine kind network IP for ${LOCAL_REGISTRY_NAME}"
    return 1
  fi

  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    if docker exec "${node}" getent hosts "${LOCAL_REGISTRY_NAME}" >/dev/null 2>&1; then
      continue
    fi

    log "adding ${LOCAL_REGISTRY_NAME} host entry to ${node}"
    docker exec "${node}" sh -c \
      "printf '%s\t%s\n' '${registry_ip}' '${LOCAL_REGISTRY_NAME}' >> /etc/hosts"
  done
}

publish_local_registry_config() {
  kubectl --context "${KUBE_CONTEXT}" apply -f - >/dev/null <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "${LOCAL_REGISTRY_HOST}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

render_kind_config() {
  local output_path="$1"
  local worker_index

  sed \
    -e "s|^  ipFamily: .*|  ipFamily: ${KIND_IP_FAMILY}|" \
    "${ROOT_DIR}/deploy/kubernetes/overlays/kind/kind-config.yaml" >"${output_path}"
  if ! [[ "${KIND_WORKER_NODES}" =~ ^[0-9]+$ ]]; then
    log "KIND_WORKER_NODES must be a non-negative integer, got ${KIND_WORKER_NODES}"
    exit 1
  fi
  for ((worker_index = 0; worker_index < KIND_WORKER_NODES; worker_index++)); do
    cat >>"${output_path}" <<'EOF'
  - role: worker
EOF
  done
  cat >>"${output_path}" <<EOF
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."${LOCAL_REGISTRY_HOST}"]
      endpoint = ["http://${LOCAL_REGISTRY_NAME}:5000"]
EOF
}

gateway_api_module_dir() {
  local version="${1:-${GATEWAY_API_VERSION}}"
  local modcache

  modcache="$(cd "${ROOT_DIR}/controlplane" && go env GOMODCACHE 2>/dev/null || true)"
  if [[ -z "${modcache}" ]]; then
    return
  fi

  printf '%s/sigs.k8s.io/gateway-api@%s\n' "${modcache}" "${version}"
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

recreate_local_registry() {
  mkdir -p "${REGISTRY_DATA_DIR}"
  ensure_image_available \
    "${LOCAL_REGISTRY_IMAGE}" \
    "${LOCAL_REGISTRY_IMAGE}" \
    "${LOCAL_REGISTRY_MIRROR}" \
    "m.daocloud.io/docker.io/registry:2" \
    "docker.1ms.run/library/registry:2" \
    "docker.1ms.run/registry:2"

  docker rm -f "${LOCAL_REGISTRY_NAME}" >/dev/null 2>&1 || true
  docker run -d \
    --restart=always \
    -p "127.0.0.1:${LOCAL_REGISTRY_PORT}:5000" \
    -v "${REGISTRY_DATA_DIR}:/var/lib/registry" \
    --name "${LOCAL_REGISTRY_NAME}" \
    "${LOCAL_REGISTRY_IMAGE}" >/dev/null

  if ! wait_for_local_registry; then
    log "local registry ${LOCAL_REGISTRY_NAME} is not reachable on 127.0.0.1:${LOCAL_REGISTRY_PORT}"
    return 1
  fi
}

ensure_local_registry_storage() {
  if registry_storage_writable; then
    return
  fi

  log "local registry ${LOCAL_REGISTRY_NAME} storage is not writable; recreating registry"
  recreate_local_registry
  registry_storage_writable
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
    log "recreating local registry ${LOCAL_REGISTRY_NAME} because 127.0.0.1:${LOCAL_REGISTRY_PORT} is unreachable"
    recreate_local_registry
  fi

  if ! ensure_local_registry_storage; then
    log "local registry ${LOCAL_REGISTRY_NAME} storage is still not writable after recreation"
    return 1
  fi
}

ensure_kind_cluster() {
  mkdir -p "${KIND_CACHE_DIR}"
  ensure_image_available \
    "${KIND_NODE_IMAGE}" \
    "${KIND_NODE_MIRROR}" \
    "${KIND_NODE_IMAGE}" \
    "docker.1ms.run/kindest/node:v1.34.0"

  if cluster_exists; then
    if [[ "${RECREATE_CLUSTER}" == "true" ]] || ! cluster_supports_local_registry || ! cluster_supports_port_mappings; then
      log "recreating kind cluster ${CLUSTER_NAME}"
      kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
    else
      log "reusing existing kind cluster ${CLUSTER_NAME}"
      refresh_kind_kubeconfig
      connect_registry_to_kind_network
      ensure_kind_registry_hosts
      publish_local_registry_config
      ensure_kindnet_resources_unlimited
      return
    fi
  fi

  local rendered_config="${KIND_CACHE_DIR}/kind-config.rendered.yaml"
  render_kind_config "${rendered_config}"

  log "creating kind cluster ${CLUSTER_NAME}"
  kind create cluster --name "${CLUSTER_NAME}" --config "${rendered_config}" --image "${KIND_NODE_IMAGE}" >/dev/null
  refresh_kind_kubeconfig
  connect_registry_to_kind_network
  ensure_kind_registry_hosts
  publish_local_registry_config
  ensure_kindnet_resources_unlimited
}

ensure_kindnet_resources_unlimited() {
  local current_resources

  if ! current_resources="$(kubectl --context "${KUBE_CONTEXT}" -n kube-system get daemonset kindnet -o jsonpath='{.spec.template.spec.containers[0].resources}' 2>/dev/null)"; then
    log "kindnet daemonset not found; skipping resource limit removal"
    return
  fi

  if [[ "${current_resources}" != *limits* && "${current_resources}" != *requests* ]]; then
    log "kindnet resource requests/limits already absent"
    return
  fi

  log "removing kindnet resource requests/limits for performance tests"
  kubectl --context "${KUBE_CONTEXT}" -n kube-system patch daemonset kindnet \
    --type=json \
    -p='[{"op":"remove","path":"/spec/template/spec/containers/0/resources"}]' >/dev/null
  kubectl --context "${KUBE_CONTEXT}" -n kube-system rollout status daemonset/kindnet --timeout=120s
}

ensure_build_base_images() {
  ensure_image_available \
    "${CONTROLPLANE_GO_IMAGE}" \
    "${CONTROLPLANE_GO_IMAGE}" \
    "m.daocloud.io/docker.io/golang:1.26-bookworm" \
    "docker.1ms.run/library/golang:1.26-bookworm" \
    "docker.1ms.run/golang:1.26-bookworm" \
    "golang:1.26-bookworm"

  ensure_image_available \
    "${DATAPLANE_RUST_IMAGE}" \
    "${DATAPLANE_RUST_IMAGE}" \
    "m.daocloud.io/docker.io/rust:1.88-bookworm" \
    "docker.1ms.run/library/rust:1.88-bookworm" \
    "docker.1ms.run/rust:1.88-bookworm" \
    "rust:1.88-bookworm"

  ensure_image_available \
    "${RUNTIME_IMAGE}" \
    "${RUNTIME_IMAGE}" \
    "m.daocloud.io/docker.io/debian:bookworm-slim" \
    "docker.1ms.run/library/debian:bookworm-slim" \
    "docker.1ms.run/debian:bookworm-slim" \
    "debian:bookworm-slim"
}

ensure_gateway_api_crds() {
  mkdir -p "${CRD_CACHE_DIR}"
  local module_dir
  local backendlb_module_dir
  local backendlb_target
  module_dir="$(gateway_api_module_dir "${GATEWAY_API_VERSION}")"
  backendlb_module_dir="$(gateway_api_module_dir "${BACKENDLBPOLICY_CRD_VERSION}")"

  while IFS='|' read -r cache_name track source_name; do
    local target="${CRD_CACHE_DIR}/${cache_name}"
    if [[ -n "${module_dir}" && -f "${module_dir}/config/crd/${track}/${source_name}" ]]; then
      log "copying ${source_name} from local gateway-api module cache"
      cp "${module_dir}/config/crd/${track}/${source_name}" "${target}"
      continue
    fi

    log "downloading ${source_name} from gateway-api ${GATEWAY_API_VERSION}"
    curl -fsSL \
      "https://gh-proxy.com/https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/crd/${track}/${source_name}" \
      -o "${target}"
  done <<'EOF'
gatewayclasses.yaml|experimental|gateway.networking.k8s.io_gatewayclasses.yaml
gateways.yaml|experimental|gateway.networking.k8s.io_gateways.yaml
httproutes.yaml|experimental|gateway.networking.k8s.io_httproutes.yaml
grpcroutes.yaml|experimental|gateway.networking.k8s.io_grpcroutes.yaml
backendtlspolicies.yaml|experimental|gateway.networking.k8s.io_backendtlspolicies.yaml
referencegrants.yaml|experimental|gateway.networking.k8s.io_referencegrants.yaml
listenersets.yaml|experimental|gateway.networking.k8s.io_listenersets.yaml
tcproutes.yaml|experimental|gateway.networking.k8s.io_tcproutes.yaml
tlsroutes.yaml|experimental|gateway.networking.k8s.io_tlsroutes.yaml
udproutes.yaml|experimental|gateway.networking.k8s.io_udproutes.yaml
EOF

  backendlb_target="${CRD_CACHE_DIR}/backendlbpolicies.yaml"
  if [[ -n "${backendlb_module_dir}" && -f "${backendlb_module_dir}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml" ]]; then
    log "copying gateway.networking.k8s.io_backendlbpolicies.yaml from local gateway-api module cache ${BACKENDLBPOLICY_CRD_VERSION}"
    cp \
      "${backendlb_module_dir}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml" \
      "${backendlb_target}"
  else
    log "downloading gateway.networking.k8s.io_backendlbpolicies.yaml from gateway-api ${BACKENDLBPOLICY_CRD_VERSION}"
    curl -fsSL \
      "https://gh-proxy.com/https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${BACKENDLBPOLICY_CRD_VERSION}/config/crd/experimental/gateway.networking.k8s.io_backendlbpolicies.yaml" \
      -o "${backendlb_target}"
  fi
  strip_gateway_api_bundle_annotations "${backendlb_target}"
}

render_manifest() {
  local source_path="$1"
  local output_file="$2"

  if [[ -d "${source_path}" ]]; then
    kubectl kustomize "${source_path}" | sed \
      -e "s|nantian-controlplane:dev|$(escape_sed_replacement "${CONTROL_IMAGE}")|g" \
      -e "s|nantian-dataplane:dev|$(escape_sed_replacement "${DATAPLANE_IMAGE}")|g" \
      -e "s|m.daocloud.io/docker.io/hashicorp/http-echo:1.0.0|$(escape_sed_replacement "${SMOKE_IMAGE}")|g" \
      -e "s|localhost:5001/gateway-api-conformance/echo-basic:smoke|$(escape_sed_replacement "${SMOKE_ECHO_BASIC_IMAGE}")|g" \
      -e "s|localhost:5001/gateway-api-conformance/coredns:smoke|$(escape_sed_replacement "${SMOKE_COREDNS_IMAGE}")|g" \
      >"${output_file}"
    return
  fi

  sed \
    -e "s|nantian-controlplane:dev|$(escape_sed_replacement "${CONTROL_IMAGE}")|g" \
    -e "s|nantian-dataplane:dev|$(escape_sed_replacement "${DATAPLANE_IMAGE}")|g" \
    -e "s|m.daocloud.io/docker.io/hashicorp/http-echo:1.0.0|$(escape_sed_replacement "${SMOKE_IMAGE}")|g" \
    -e "s|localhost:5001/gateway-api-conformance/echo-basic:smoke|$(escape_sed_replacement "${SMOKE_ECHO_BASIC_IMAGE}")|g" \
    -e "s|localhost:5001/gateway-api-conformance/coredns:smoke|$(escape_sed_replacement "${SMOKE_COREDNS_IMAGE}")|g" \
    "${source_path}" >"${output_file}"
}

build_and_push_images() {
  if [[ "${SKIP_BUILD}" == "true" ]]; then
    log "skipping image build; using tag ${RESOLVED_IMAGE_TAG}"
    return
  fi

  ensure_build_base_images

  log "building controlplane image ${CONTROL_PUSH_IMAGE}"
  docker build \
    --network "${DOCKER_BUILD_NETWORK}" \
    --build-arg "GO_IMAGE=${CONTROLPLANE_GO_IMAGE}" \
    --build-arg "RUNTIME_IMAGE=${RUNTIME_IMAGE}" \
    -f "${ROOT_DIR}/controlplane/Dockerfile" \
    -t "${CONTROL_PUSH_IMAGE}" \
    "${ROOT_DIR}" >/dev/null
  log "pushing controlplane image ${CONTROL_PUSH_IMAGE}"
  docker push "${CONTROL_PUSH_IMAGE}" >/dev/null

  log "building dataplane image ${DATAPLANE_PUSH_IMAGE}"
  docker build \
    --network "${DOCKER_BUILD_NETWORK}" \
    --build-arg "RUST_IMAGE=${DATAPLANE_RUST_IMAGE}" \
    --build-arg "RUNTIME_IMAGE=${RUNTIME_IMAGE}" \
    --build-arg "DATAPLANE_CARGO_FEATURES=${DATAPLANE_CARGO_FEATURES}" \
    -f "${ROOT_DIR}/../dataplane/Dockerfile" \
    -t "${DATAPLANE_PUSH_IMAGE}" \
    "${ROOT_DIR}/.." >/dev/null
  log "pushing dataplane image ${DATAPLANE_PUSH_IMAGE}"
  docker push "${DATAPLANE_PUSH_IMAGE}" >/dev/null

  printf '%s' "${RESOLVED_IMAGE_TAG}" >"${LAST_TAG_FILE}"
}

preload_kind_runtime_images() {
  preload_kind_registry_images \
    "${CONTROL_IMAGE}" \
    "${DATAPLANE_IMAGE}"
}

preload_kind_smoke_images() {
  preload_kind_registry_images \
    "${SMOKE_IMAGE}" \
    "${SMOKE_ECHO_BASIC_IMAGE}" \
    "${SMOKE_COREDNS_IMAGE}"
}

ensure_smoke_source_images() {
  ensure_registry_copy \
    "${SMOKE_SOURCE_IMAGE}" \
    "${SMOKE_PUSH_IMAGE}" \
    "docker.1ms.run/hashicorp/http-echo:1.0.0" \
    "hashicorp/http-echo:1.0.0"

  ensure_registry_copy \
    "${SMOKE_ECHO_BASIC_SOURCE_IMAGE}" \
    "${SMOKE_ECHO_BASIC_PUSH_IMAGE}" \
    "gcr.io/k8s-staging-gateway-api/echo-basic:v20240412-v1.0.0-394-g40c666fd"

  ensure_registry_copy \
    "${SMOKE_COREDNS_SOURCE_IMAGE}" \
    "${SMOKE_COREDNS_PUSH_IMAGE}" \
    "docker.1ms.run/coredns/coredns:latest" \
    "coredns/coredns:latest"
}

preload_kind_registry_images() {
  local node
  local image

  log "preloading images into kind nodes via crictl"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    for image in "$@"; do
      docker exec "${node}" crictl pull "${image}" >/dev/null
    done
  done
}

if [[ "${RUN_KIND_SOURCE_ONLY:-false}" == "true" ]]; then
  return 0 2>/dev/null || exit 0
fi

ensure_local_registry
resolve_runtime_image_tag
ensure_kind_cluster
ensure_gateway_api_crds
build_and_push_images
preload_kind_runtime_images
if [[ "${SKIP_SMOKE}" == "false" ]]; then
  ensure_smoke_source_images
  preload_kind_smoke_images
fi
trap cleanup_transient_state EXIT

for crd in gatewayclasses gateways httproutes grpcroutes backendlbpolicies backendtlspolicies referencegrants listenersets tcproutes tlsroutes udproutes; do
  kubectl --context "${KUBE_CONTEXT}" apply --server-side --force-conflicts -f "${CRD_CACHE_DIR}/${crd}.yaml" >/dev/null
done

BASE_RENDERED="${KIND_CACHE_DIR}/base.rendered.yaml"
SMOKE_RENDERED="${KIND_CACHE_DIR}/smoke.rendered.yaml"
render_manifest "${ROOT_DIR}/deploy/kubernetes/overlays/kind" "${BASE_RENDERED}"

kubectl --context "${KUBE_CONTEXT}" apply -f "${BASE_RENDERED}" >/dev/null
# The local kind workflow relies on static ConfigMaps mounted into the pods.
# A plain apply updates the files on disk but does not restart the controlplane
# or dataplane processes, so config-only changes would otherwise never take
# effect during iterative runs.
kubectl --context "${KUBE_CONTEXT}" rollout restart deployment/nantian-controlplane -n nantian-gw >/dev/null
kubectl --context "${KUBE_CONTEXT}" rollout restart deployment/nantian-dataplane -n nantian-gw >/dev/null
kubectl --context "${KUBE_CONTEXT}" rollout status deployment/nantian-controlplane -n nantian-gw --timeout="${ROLLOUT_TIMEOUT}"
kubectl --context "${KUBE_CONTEXT}" rollout status deployment/nantian-dataplane -n nantian-gw --timeout="${ROLLOUT_TIMEOUT}"

if [[ "${SKIP_SMOKE}" == "false" ]]; then
  cleanup_smoke_resources
  prepare_smoke_tls_secret
  render_manifest "${ROOT_DIR}/tests/e2e/smoke.yaml" "${SMOKE_RENDERED}"
  kubectl --context "${KUBE_CONTEXT}" apply -f "${SMOKE_RENDERED}" >/dev/null
  kubectl --context "${KUBE_CONTEXT}" rollout status deployment/echo -n nantian-gw --timeout="${ROLLOUT_TIMEOUT}"
  kubectl --context "${KUBE_CONTEXT}" rollout status deployment/grpc-echo -n nantian-gw --timeout="${ROLLOUT_TIMEOUT}"
  kubectl --context "${KUBE_CONTEXT}" rollout status deployment/tls-backend -n nantian-gw --timeout="${ROLLOUT_TIMEOUT}"
  kubectl --context "${KUBE_CONTEXT}" rollout status deployment/coredns -n nantian-gw --timeout="${ROLLOUT_TIMEOUT}"
  ensure_tcp_smoke_relay
  ensure_failure_smoke_relays
  run_smoke_checks
  run_failure_checks
  log "e2e smoke test passed"
  exit 0
fi

cleanup_smoke_resources

log "kind cluster is ready"
log "controlplane image: ${CONTROL_IMAGE}"
log "dataplane image: ${DATAPLANE_IMAGE}"
log "smoke workload deployment skipped"
exit 0
