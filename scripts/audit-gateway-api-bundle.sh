#!/usr/bin/env bash
set -euo pipefail

EXPECTED_BUNDLE_VERSION="${EXPECTED_BUNDLE_VERSION:-v1.5.1}"
GATEWAY_CLASS_NAME="${GATEWAY_CLASS_NAME:-aether}"

log() {
  printf '[audit-gateway-api-bundle] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

crd_bundle_version() {
  local crd_name="$1"

  kubectl get crd "${crd_name}" -o jsonpath='{.metadata.annotations.gateway\.networking\.k8s\.io/bundle-version}'
}

print_crd_versions() {
  local crd
  local version
  local mismatch="false"

  for crd in \
    gatewayclasses.gateway.networking.k8s.io \
    gateways.gateway.networking.k8s.io \
    httproutes.gateway.networking.k8s.io \
    grpcroutes.gateway.networking.k8s.io \
    referencegrants.gateway.networking.k8s.io
  do
    if ! kubectl get crd "${crd}" >/dev/null 2>&1; then
      log "CRD ${crd} is not installed"
      mismatch="true"
      continue
    fi

    version="$(crd_bundle_version "${crd}")"
    printf '%s\t%s\n' "${crd}" "${version}"
    if [[ -z "${version}" || "${version}" != "${EXPECTED_BUNDLE_VERSION}" ]]; then
      mismatch="true"
    fi
  done

  if [[ "${mismatch}" == "true" ]]; then
    log "bundle-version mismatch detected; run docs/gateway-api-version-audit.md before changing support claims"
    exit 1
  fi
}

print_gatewayclass_status() {
  if ! kubectl get gatewayclass.gateway.networking.k8s.io "${GATEWAY_CLASS_NAME}" >/dev/null 2>&1; then
    log "GatewayClass ${GATEWAY_CLASS_NAME} not found; skipping status dump"
    return
  fi

  printf '\nGatewayClass: %s\n' "${GATEWAY_CLASS_NAME}"
  kubectl get gatewayclass.gateway.networking.k8s.io "${GATEWAY_CLASS_NAME}" -o json \
    | jq '{
        controllerName: .spec.controllerName,
        supportedFeatures: (.status.supportedFeatures // [] | map(.name)),
        conditions: (.status.conditions // [] | map({
          type,
          status,
          reason,
          observedGeneration,
          message
        }))
      }'
}

main() {
  require_command jq
  require_command kubectl

  log "expecting Gateway API bundle-version ${EXPECTED_BUNDLE_VERSION}"
  print_crd_versions
  print_gatewayclass_status
  log "bundle audit passed"
}

main "$@"
