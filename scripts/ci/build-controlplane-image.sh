#!/usr/bin/env bash
set -euo pipefail

CONTROLPLANE_IMAGE="${CONTROLPLANE_IMAGE:?CONTROLPLANE_IMAGE is required}"

docker build -t "$CONTROLPLANE_IMAGE" .
docker image inspect "$CONTROLPLANE_IMAGE" >/dev/null
echo "Built control-plane image: $CONTROLPLANE_IMAGE"
