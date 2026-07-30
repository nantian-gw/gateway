#!/usr/bin/env bash
set -euo pipefail

GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.5.1}"
GATEWAY_API_CHANNEL="${GATEWAY_API_CHANNEL:-experimental}"
BASE_URL="https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}"

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

manifest_file="$tmpdir/${GATEWAY_API_CHANNEL}-install.yaml"
curl -fsSL --connect-timeout 15 --max-time 120 --retry 3 --retry-delay 5 --retry-all-errors \
  "${BASE_URL}/${GATEWAY_API_CHANNEL}-install.yaml" \
  -o "$manifest_file"

apply_with_retries() {
  local manifest="$1"

  # Remove stale CRDs that may lack api-approved annotation
  kubectl delete crd backendlbpolicies.gateway.networking.k8s.io --ignore-not-found 2>/dev/null || true

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

apply_with_retries "$manifest_file"
kubectl wait --for=condition=established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/gateways.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/httproutes.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/referencegrants.gateway.networking.k8s.io --timeout=60s

if [[ "$GATEWAY_API_CHANNEL" == "experimental" ]]; then
  kubectl wait --for=condition=established crd/tcproutes.gateway.networking.k8s.io --timeout=60s
  kubectl wait --for=condition=established crd/udproutes.gateway.networking.k8s.io --timeout=60s
  kubectl wait --for=condition=established crd/tlsroutes.gateway.networking.k8s.io --timeout=60s
fi
