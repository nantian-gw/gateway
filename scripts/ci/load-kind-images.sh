#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:?CLUSTER_NAME is required}"
CONTROL_PLANE_IMAGE="${CONTROL_PLANE_IMAGE:-${CONTROLPLANE_IMAGE:-ghcr.io/nantian-gw/nantian-controlplane:latest}}"
DATA_PLANE_IMAGE="${DATA_PLANE_IMAGE:-${DATAPLANE_IMAGE:-ghcr.io/nantian-gw/dataplane:latest}}"
DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-ghcr.io/nantian-gw/dashboard:latest}"
CONFORMANCE_TEST_IMAGES="${CONFORMANCE_TEST_IMAGES:-}"

preload_image() {
  local image="$1"
  local image_archive

  for attempt in 1 2 3; do
    if docker image inspect "$image" >/dev/null 2>&1 || docker pull "$image"; then
      image_archive="$(mktemp "${TMPDIR:-/tmp}/nantian-kind-image.XXXXXX.tar")"
      if docker save --platform linux/amd64 -o "$image_archive" "$image" \
        && kind load image-archive --name "$CLUSTER_NAME" "$image_archive"; then
        rm -f "$image_archive"
        return
      fi
      rm -f "$image_archive"
    fi

    echo "Failed to preload $image on attempt $attempt; retrying in 10s..." >&2
    sleep 10
  done

  echo "Failed to preload $image after 3 attempts." >&2
  return 1
}

preload_image "$CONTROL_PLANE_IMAGE"
preload_image "$DATA_PLANE_IMAGE"
preload_image "$DASHBOARD_IMAGE"
for image in $CONFORMANCE_TEST_IMAGES; do
  preload_image "$image"
done

echo "Loaded images into kind cluster $CLUSTER_NAME:"
echo "  controlplane: $CONTROL_PLANE_IMAGE"
echo "  dataplane:    $DATA_PLANE_IMAGE"
echo "  dashboard:    $DASHBOARD_IMAGE"
if [[ -n "$CONFORMANCE_TEST_IMAGES" ]]; then
  echo "  conformance:  $CONFORMANCE_TEST_IMAGES"
fi
