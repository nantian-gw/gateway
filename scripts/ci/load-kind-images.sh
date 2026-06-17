#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=./dependency-images.sh
source "$SCRIPT_DIR/dependency-images.sh"

CLUSTER_NAME="${CLUSTER_NAME:?CLUSTER_NAME is required}"
CONTROLPLANE_IMAGE="${CONTROLPLANE_IMAGE:?CONTROLPLANE_IMAGE is required}"
DATAPLANE_IMAGE="${DATAPLANE_IMAGE:-$DEFAULT_DATAPLANE_IMAGE}"
DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-$DEFAULT_DASHBOARD_IMAGE}"

docker pull "$DATAPLANE_IMAGE"
docker pull "$DASHBOARD_IMAGE"

kind load docker-image "$CONTROLPLANE_IMAGE" --name "$CLUSTER_NAME"
kind load docker-image "$DATAPLANE_IMAGE" --name "$CLUSTER_NAME"
kind load docker-image "$DASHBOARD_IMAGE" --name "$CLUSTER_NAME"

echo "Loaded images into kind cluster $CLUSTER_NAME:"
echo "  controlplane: $CONTROLPLANE_IMAGE"
echo "  dataplane:    $DATAPLANE_IMAGE"
echo "  dashboard:    $DASHBOARD_IMAGE"
