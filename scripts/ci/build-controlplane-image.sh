#!/usr/bin/env bash
set -euo pipefail

CONTROLPLANE_IMAGE="${CONTROLPLANE_IMAGE:?CONTROLPLANE_IMAGE is required}"
CONTROLPLANE_LATEST_IMAGE="${CONTROLPLANE_LATEST_IMAGE:-ghcr.io/nantian-gw/nantian-controlplane:latest}"

docker build -t "$CONTROLPLANE_IMAGE" .
docker tag "$CONTROLPLANE_IMAGE" "$CONTROLPLANE_LATEST_IMAGE"
docker image inspect "$CONTROLPLANE_IMAGE" >/dev/null
echo "Built control-plane image: $CONTROLPLANE_IMAGE"
