#!/usr/bin/env bash
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT_DIR="${REPO_ROOT:-${SCRIPT_ROOT}}"
source "${SCRIPT_ROOT}/scripts/lib/kind-image-sync.sh"

KIND_CACHE_DIR="${KIND_CACHE_DIR:-${ROOT_DIR}/tmp/kind}"
LAST_TAG_FILE="${LAST_TAG_FILE:-${KIND_CACHE_DIR}/last-image-tag}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:5001}"
KIND_IMAGE_SYNC_LOCAL_REGISTRY="${KIND_IMAGE_SYNC_LOCAL_REGISTRY:-${LOCAL_REGISTRY_PUSH_HOST}}"
IMAGE_TAG="${IMAGE_TAG:-$(date +%Y%m%d%H%M%S)}"
RUNTIME_IMAGE="${RUNTIME_IMAGE:-m.daocloud.io/docker.io/library/debian:trixie-slim}"
RUNTIME_IMAGE_MIRROR="${RUNTIME_IMAGE_MIRROR:-docker.1ms.run/library/debian:trixie-slim}"
DOCKER_BUILD_NETWORK="${DOCKER_BUILD_NETWORK:-}"
LOCAL_IMAGE_BUILD_DIR="${LOCAL_IMAGE_BUILD_DIR:-${KIND_CACHE_DIR}/local-image-build/${IMAGE_TAG}}"
CA_CERT_BUNDLE="${CA_CERT_BUNDLE:-/etc/ssl/certs/ca-certificates.crt}"

CONTROL_PUSH_IMAGE="${CONTROL_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/aether-gateway-controlplane:${IMAGE_TAG}}"
DATAPLANE_PUSH_IMAGE="${DATAPLANE_PUSH_IMAGE:-${LOCAL_REGISTRY_PUSH_HOST}/aether-gateway-dataplane:${IMAGE_TAG}}"
DATAPLANE_CARGO_FEATURES="${DATAPLANE_CARGO_FEATURES:-allocator-jemalloc}"

log() {
  printf '[local-runtime-images] %s\n' "$*"
}

if [[ -z "${DOCKER_BUILD_NETWORK}" ]]; then
  if grep -Eq '^[[:space:]]*nameserver[[:space:]]+127\.' /etc/resolv.conf 2>/dev/null; then
    DOCKER_BUILD_NETWORK="host"
  else
    DOCKER_BUILD_NETWORK="default"
  fi
fi

copy_ca_bundle() {
  local output_path="$1"

  if [[ -f "${CA_CERT_BUNDLE}" ]]; then
    cp "${CA_CERT_BUNDLE}" "${output_path}"
    return
  fi

  log "CA bundle ${CA_CERT_BUNDLE} not found; writing an empty bundle for local test image"
  : >"${output_path}"
}

build_controlplane_binary() {
  local output_dir="${LOCAL_IMAGE_BUILD_DIR}/controlplane"
  local output_path="${output_dir}/aether-gateway-controlplane"

  mkdir -p "${output_dir}"
  log "building controlplane binary with host Go cache"
  (
    cd "${ROOT_DIR}/controlplane"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${output_path}" ./cmd/manager
  )
  chmod +x "${output_path}"
  copy_ca_bundle "${output_dir}/ca-certificates.crt"
  cat >"${output_dir}/Dockerfile" <<'EOF'
ARG RUNTIME_IMAGE=debian:trixie-slim
FROM ${RUNTIME_IMAGE}

WORKDIR /app
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY aether-gateway-controlplane /usr/local/bin/aether-gateway-controlplane

ENTRYPOINT ["/usr/local/bin/aether-gateway-controlplane"]
EOF
}

build_dataplane_binary() {
  local output_dir="${LOCAL_IMAGE_BUILD_DIR}/dataplane"
  local output_path="${output_dir}/aeg-app"
  local -a cargo_feature_args=()

  if [[ -n "${DATAPLANE_CARGO_FEATURES}" ]]; then
    cargo_feature_args=(--features "${DATAPLANE_CARGO_FEATURES}")
  fi

  mkdir -p "${output_dir}"
  log "building dataplane binary with host Cargo cache"
  cargo build --release --manifest-path "${ROOT_DIR}/dataplane/Cargo.toml" -p aeg-app "${cargo_feature_args[@]}"
  cp "${ROOT_DIR}/dataplane/target/release/aeg-app" "${output_path}"
  chmod +x "${output_path}"
  copy_ca_bundle "${output_dir}/ca-certificates.crt"
  cat >"${output_dir}/Dockerfile" <<'EOF'
ARG RUNTIME_IMAGE=debian:trixie-slim
FROM ${RUNTIME_IMAGE}

WORKDIR /app
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY aeg-app /usr/local/bin/aeg-app

ENTRYPOINT ["/usr/local/bin/aeg-app"]
EOF
}

build_runtime_image() {
  local name="$1"
  local image="$2"
  local context_dir="${LOCAL_IMAGE_BUILD_DIR}/${name}"

  log "building ${name} runtime image ${image}"
  docker build \
    --network "${DOCKER_BUILD_NETWORK}" \
    --build-arg "RUNTIME_IMAGE=${RUNTIME_IMAGE}" \
    -f "${context_dir}/Dockerfile" \
    -t "${image}" \
    "${context_dir}" >/dev/null

  log "pushing ${name} runtime image ${image}"
  docker push "${image}" >/dev/null
}

mkdir -p "${KIND_CACHE_DIR}" "${LOCAL_IMAGE_BUILD_DIR}"

kind_image_sync_ensure_image_available \
  "${RUNTIME_IMAGE}" \
  "${RUNTIME_IMAGE}" \
  "${RUNTIME_IMAGE_MIRROR}" \
  "m.daocloud.io/docker.io/debian:trixie-slim" \
  "docker.1ms.run/debian:trixie-slim" \
  "debian:trixie-slim" \
  || exit 1

build_controlplane_binary
build_dataplane_binary
build_runtime_image "controlplane" "${CONTROL_PUSH_IMAGE}"
build_runtime_image "dataplane" "${DATAPLANE_PUSH_IMAGE}"

printf '%s' "${IMAGE_TAG}" >"${LAST_TAG_FILE}"
log "recorded image tag ${IMAGE_TAG} in ${LAST_TAG_FILE}"
