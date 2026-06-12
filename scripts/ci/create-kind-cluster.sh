#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:?CLUSTER_NAME is required}"
KIND_CONFIG="${KIND_CONFIG:-scripts/ci/kind-ci-config.yaml}"

if [[ ! -f "$KIND_CONFIG" ]]; then
  echo "kind config not found: $KIND_CONFIG" >&2
  exit 1
fi

kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG" --wait 5m
kubectl wait --for=condition=ready node --all --timeout=2m
