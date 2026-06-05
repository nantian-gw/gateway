#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
EXPECTED_VERSION="${EXPECTED_VERSION:-v1.5.1}"
EXPECTED_KIND_NODE_IMAGE="${EXPECTED_KIND_NODE_IMAGE:-kindest/node:v1.34.0}"
FAILED="false"

log() {
  printf '[gateway-api-version-alignment] %s\n' "$*"
}

check_literal() {
  local file="$1"
  local literal="$2"

  if grep -Fq "${literal}" "${ROOT_DIR}/${file}"; then
    return
  fi

  log "missing ${literal} in ${file}"
  FAILED="true"
}

check_literal "controlplane/go.mod" "sigs.k8s.io/gateway-api ${EXPECTED_VERSION}"
check_literal "tests/conformance-harness/go.mod" "sigs.k8s.io/gateway-api ${EXPECTED_VERSION}"
check_literal "tests/conformance/run.sh" "GATEWAY_API_VERSION=\"\${GATEWAY_API_VERSION:-${EXPECTED_VERSION}}\""
check_literal "tests/e2e/run-kind.sh" "GATEWAY_API_VERSION=\"\${GATEWAY_API_VERSION:-${EXPECTED_VERSION}}\""
check_literal "scripts/run-release-validation.sh" "GATEWAY_API_VERSION=\"\${GATEWAY_API_VERSION:-${EXPECTED_VERSION}}\""
check_literal "scripts/audit-gateway-api-bundle.sh" "EXPECTED_BUNDLE_VERSION=\"\${EXPECTED_BUNDLE_VERSION:-${EXPECTED_VERSION}}\""
check_literal "controlplane/internal/status/gatewayclass_status.go" "minSupportedGatewayAPIVersion     = \"${EXPECTED_VERSION}\""
check_literal "controlplane/internal/status/gatewayclass_status.go" "maxSupportedGatewayAPIVersion     = \"${EXPECTED_VERSION}\""
check_literal "tests/e2e/run-kind.sh" "KIND_NODE_IMAGE=\"\${KIND_NODE_IMAGE:-${EXPECTED_KIND_NODE_IMAGE}}\""
check_literal "tests/e2e/run-kind.sh" "KIND_NODE_MIRROR=\"\${KIND_NODE_MIRROR:-m.daocloud.io/docker.io/${EXPECTED_KIND_NODE_IMAGE}}\""

if [[ "${FAILED}" == "true" ]]; then
  exit 1
fi

log "Gateway API version defaults aligned to ${EXPECTED_VERSION}; kind node image default aligned to ${EXPECTED_KIND_NODE_IMAGE}"
