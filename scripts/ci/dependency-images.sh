#!/usr/bin/env bash

# Pinned dependency images for gateway kind validation.
# These digests currently back the shared v2026.06.0-rc1 dependency release.
# Use sha-<commit>-amd64 tags (not latest-amd64) to avoid timing issues where
# the Conformance workflow starts before the Docker workflow finishes updating
# the latest-amd64 tag.
DEFAULT_DATAPLANE_IMAGE="ghcr.io/nantian-gw/dataplane:sha-29beb12-amd64"
DEFAULT_DASHBOARD_IMAGE="ghcr.io/nantian-gw/dashboard@sha256:f913109dd5c964a48877de15797e1a2e9f08008e978c5ede53fc2ca9be8c601a"

sanitize_kind_tag_part() {
  local raw="${1:?tag component is required}"

  raw="${raw//\//-}"
  raw="${raw//:/-}"
  raw="${raw//[^[:alnum:]_.-]/-}"

  printf '%s' "$raw"
}

kind_runtime_image_ref() {
  local image_ref="${1:?image reference is required}"
  local repository
  local suffix

  if [[ "$image_ref" == *"@sha256:"* ]]; then
    repository="${image_ref%@sha256:*}"
    suffix="${image_ref##*@sha256:}"
    suffix="${suffix:0:12}"
  else
    repository="${image_ref%:*}"
    if [[ "$repository" == "$image_ref" ]]; then
      echo "image reference must include a tag or digest: $image_ref" >&2
      return 1
    fi
    suffix="$(sanitize_kind_tag_part "${image_ref##*:}")"
  fi

  printf '%s:kind-%s\n' "$repository" "$suffix"
}

DEFAULT_KIND_DATAPLANE_IMAGE="$(kind_runtime_image_ref "$DEFAULT_DATAPLANE_IMAGE")"
DEFAULT_KIND_DASHBOARD_IMAGE="$(kind_runtime_image_ref "$DEFAULT_DASHBOARD_IMAGE")"
