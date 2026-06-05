#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_SCRIPT="${ROOT_DIR}/tests/conformance/run.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[conformance-image-rewrite-test] %s\n' "$*" >&2
  exit 1
}

assert_file_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if ! grep -Fq "${pattern}" "${path}"; then
    fail "${label}: expected '${pattern}' in ${path}"
  fi
}

assert_file_not_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Fq "${pattern}" "${path}"; then
    fail "${label}: unexpected '${pattern}' in ${path}"
  fi
}

FIXTURE_WORK_DIR="${TMP_DIR}/gateway-api"
WORK_DIR="${FIXTURE_WORK_DIR}"
mkdir -p \
  "${WORK_DIR}/conformance/base" \
  "${WORK_DIR}/conformance/mesh" \
  "${WORK_DIR}/conformance/tests"

cat >"${WORK_DIR}/conformance/base/manifests.yaml" <<'EOF'
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: old-basic
        image: gcr.io/k8s-staging-gateway-api/echo-basic:v20240412-v1.0.0-394-g40c666fd
      - name: dns
        image: registry.k8s.io/coredns/coredns:1.11.3
EOF

cat >"${WORK_DIR}/conformance/mesh/manifests.yaml" <<'EOF'
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: advanced
        image: gcr.io/k8s-staging-gateway-api/echo-advanced:v20240412-v1.0.0-394-g40c666fd
EOF

cat >"${WORK_DIR}/conformance/tests/gateway-tls-backend-client-certificate.yaml" <<'EOF'
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: mtls-basic
        image: gcr.io/k8s-staging-gateway-api/echo-basic:v20260204-monthly-2026.01-60-g28382302
EOF

CONFORMANCE_RUN_SH_SOURCE_ONLY=true source "${RUN_SCRIPT}"

WORK_DIR="${FIXTURE_WORK_DIR}"
LOCAL_REGISTRY_HOST="localhost:5001"
ECHO_BASIC_IMAGE_REPOSITORY="${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-basic"
ECHO_ADVANCED_IMAGE_REPOSITORY="${LOCAL_REGISTRY_HOST}/gateway-api-conformance/echo-advanced"
COREDNS_IMAGE="${LOCAL_REGISTRY_HOST}/gateway-api-conformance/coredns:conformance"

mapfile -t echo_basic_tags < <(discover_gateway_api_image_tags "echo-basic")
if [[ "${echo_basic_tags[*]}" != *"v20240412-v1.0.0-394-g40c666fd"* ]]; then
  fail "expected old echo-basic tag to be discovered"
fi
if [[ "${echo_basic_tags[*]}" != *"v20260204-monthly-2026.01-60-g28382302"* ]]; then
  fail "expected v1.5 backend client certificate echo-basic tag to be discovered"
fi

rewrite_conformance_images

assert_file_contains \
  "${WORK_DIR}/conformance/base/manifests.yaml" \
  "image: localhost:5001/gateway-api-conformance/echo-basic:v20240412-v1.0.0-394-g40c666fd" \
  "base echo-basic image rewritten with original tag"
assert_file_contains \
  "${WORK_DIR}/conformance/base/manifests.yaml" \
  "image: localhost:5001/gateway-api-conformance/coredns:conformance" \
  "base coredns image rewritten"
assert_file_contains \
  "${WORK_DIR}/conformance/mesh/manifests.yaml" \
  "image: localhost:5001/gateway-api-conformance/echo-advanced:v20240412-v1.0.0-394-g40c666fd" \
  "mesh echo-advanced image rewritten"
assert_file_contains \
  "${WORK_DIR}/conformance/tests/gateway-tls-backend-client-certificate.yaml" \
  "image: localhost:5001/gateway-api-conformance/echo-basic:v20260204-monthly-2026.01-60-g28382302" \
  "test-specific echo-basic image rewritten with v1.5 tag"

assert_file_not_contains \
  "${WORK_DIR}/conformance/tests/gateway-tls-backend-client-certificate.yaml" \
  "gcr.io/k8s-staging-gateway-api" \
  "test manifest no longer references upstream gcr.io image"

printf '[conformance-image-rewrite-test] ok\n'
