#!/usr/bin/env bash
set -euo pipefail

GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.5.1}"
BASE_URL="https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}"

apply_with_retries() {
  local manifest="$1"

  for attempt in 1 2 3; do
    if kubectl apply -f "$manifest"; then
      return 0
    fi

    echo "Failed to apply $manifest on attempt $attempt; retrying in 10s..." >&2
    sleep 10
  done

  echo "Failed to apply $manifest after 3 attempts." >&2
  return 1
}

apply_with_retries "${BASE_URL}/standard-install.yaml"
apply_with_retries "${BASE_URL}/experimental-install.yaml"
kubectl wait --for=condition=established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/gateways.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/httproutes.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=established crd/referencegrants.gateway.networking.k8s.io --timeout=60s
