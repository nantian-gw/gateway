#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=./dependency-images.sh
source "$SCRIPT_DIR/dependency-images.sh"

CLUSTER_NAME="${CLUSTER_NAME:?CLUSTER_NAME is required}"
CONTROLPLANE_IMAGE="${CONTROLPLANE_IMAGE:?CONTROLPLANE_IMAGE is required}"
DATAPLANE_IMAGE="${DATAPLANE_IMAGE:-$DEFAULT_DATAPLANE_IMAGE}"
DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-$DEFAULT_DASHBOARD_IMAGE}"
KIND_DATAPLANE_IMAGE="${KIND_DATAPLANE_IMAGE:-$(kind_runtime_image_ref "$DATAPLANE_IMAGE")}"
KIND_DASHBOARD_IMAGE="${KIND_DASHBOARD_IMAGE:-$(kind_runtime_image_ref "$DASHBOARD_IMAGE")}"
CONTROLPLANE_LATEST_IMAGE="${CONTROLPLANE_LATEST_IMAGE:-ghcr.io/nantian-gw/nantian-controlplane:latest}"

docker pull "$DATAPLANE_IMAGE"
docker pull "$DASHBOARD_IMAGE"

CONFORMANCE_ECHO_IMAGE="registry.k8s.io/gateway-api/conformance/echo-basic:v0.1.0"
docker pull "$CONFORMANCE_ECHO_IMAGE" 2>/dev/null || true

dataplane_image_id="$(docker image inspect --format '{{.Id}}' "$DATAPLANE_IMAGE")"
dashboard_image_id="$(docker image inspect --format '{{.Id}}' "$DASHBOARD_IMAGE")"

# kind does not preserve digest-only refs as runnable names inside the node, so
# retag pinned content to deterministic local refs before loading it.
docker tag "$dataplane_image_id" "$KIND_DATAPLANE_IMAGE"
docker tag "$dashboard_image_id" "$KIND_DASHBOARD_IMAGE"

kind load docker-image "$CONTROLPLANE_IMAGE" --name "$CLUSTER_NAME"
kind load docker-image "$CONTROLPLANE_LATEST_IMAGE" --name "$CLUSTER_NAME"
kind load docker-image "$KIND_DATAPLANE_IMAGE" --name "$CLUSTER_NAME"
kind load docker-image "$KIND_DASHBOARD_IMAGE" --name "$CLUSTER_NAME"
kind load docker-image "$CONFORMANCE_ECHO_IMAGE" --name "$CLUSTER_NAME" 2>/dev/null || true

# Force restart control plane pods to pick up the latest image if they exist
kubectl rollout restart deployment/nantian-gw-controlplane -n nantian-gw --ignore-not-found=true 2>/dev/null || true
kubectl rollout restart deployment/nantian-gw-dataplane -n nantian-gw --ignore-not-found=true 2>/dev/null || true
kubectl rollout restart deployment/nantian-gw-dashboard -n nantian-gw --ignore-not-found=true 2>/dev/null || true

if [[ -n "${GITHUB_ENV:-}" ]]; then
  {
    echo "KIND_DATAPLANE_IMAGE=$KIND_DATAPLANE_IMAGE"
    echo "KIND_DASHBOARD_IMAGE=$KIND_DASHBOARD_IMAGE"
  } >>"$GITHUB_ENV"
fi

echo "Loaded images into kind cluster $CLUSTER_NAME:"
echo "  controlplane: $CONTROLPLANE_IMAGE"
echo "  dataplane source:  $DATAPLANE_IMAGE"
echo "  dataplane kind:    $KIND_DATAPLANE_IMAGE"
echo "  dashboard source:  $DASHBOARD_IMAGE"
echo "  dashboard kind:    $KIND_DASHBOARD_IMAGE"
