#!/usr/bin/env bash
set -euo pipefail

GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.5.1}"
GATEWAY_API_CHANNEL="${GATEWAY_API_CHANNEL:-experimental}"
BASE_URL="https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATEWAY_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
HELM_CHARTS_ROOT="${HELM_CHARTS_ROOT:-$GATEWAY_ROOT/../helm-charts}"
BUNDLED_STANDARD_CRDS="$HELM_CHARTS_ROOT/charts/nantian-gw/charts/gateway-api-crds-standard/crds"

case "$GATEWAY_API_CHANNEL" in
  standard | experimental)
    ;;
  *)
    echo "GATEWAY_API_CHANNEL must be standard or experimental, got: $GATEWAY_API_CHANNEL" >&2
    exit 1
    ;;
esac

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

apply_with_retries() {
  local manifest="$1"

  for attempt in 1 2 3; do
    if kubectl apply --server-side -f "$manifest"; then
      return 0
    fi

    echo "Failed to apply $manifest on attempt $attempt; retrying in 10s..." >&2
    sleep 10
  done

  echo "Failed to apply $manifest after 3 attempts." >&2
  return 1
}

wait_for_standard_crds() {
  kubectl wait --for=condition=established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
  kubectl wait --for=condition=established crd/gateways.gateway.networking.k8s.io --timeout=60s
  kubectl wait --for=condition=established crd/httproutes.gateway.networking.k8s.io --timeout=60s
  kubectl wait --for=condition=established crd/referencegrants.gateway.networking.k8s.io --timeout=60s
}

if [[ -d "$BUNDLED_STANDARD_CRDS" ]]; then
  echo "Installing bundled Gateway API CRDs from $BUNDLED_STANDARD_CRDS"
  apply_with_retries "$BUNDLED_STANDARD_CRDS"
  wait_for_standard_crds
  exit 0
fi

manifest_file="$tmpdir/${GATEWAY_API_CHANNEL}-install.yaml"
curl -fsSL --connect-timeout 15 --max-time 120 --retry 3 --retry-delay 5 --retry-all-errors \
  "${BASE_URL}/${GATEWAY_API_CHANNEL}-install.yaml" \
  -o "$manifest_file"

apply_with_retries "$manifest_file"
wait_for_standard_crds

if [[ "$GATEWAY_API_CHANNEL" != "experimental" ]]; then
  exit 0
fi

kubectl wait --for=condition=established crd/tcproutes.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/udproutes.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/tlsroutes.gateway.networking.k8s.io --timeout=60s
