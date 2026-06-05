#!/usr/bin/env bash

kind_image_sync_log() {
  if declare -F log >/dev/null 2>&1; then
    log "$*"
    return
  fi

  printf '[kind-image-sync] %s\n' "$*" >&2
}

kind_image_sync_retry() {
  local attempts="$1"
  local delay_seconds="$2"
  shift 2

  local try
  for try in $(seq 1 "${attempts}"); do
    if "$@"; then
      return 0
    fi
    if (( try < attempts )); then
      sleep "${delay_seconds}"
    fi
  done

  return 1
}

kind_image_sync_split_ref() {
  local image="$1"
  local remainder="${image}"
  local first_component

  if [[ "${remainder}" == */* ]]; then
    first_component="${remainder%%/*}"
    if [[ "${first_component}" == *.* || "${first_component}" == *:* || "${first_component}" == "localhost" ]]; then
      remainder="${remainder#*/}"
    fi
  fi

  KIND_IMAGE_SYNC_REPOSITORY="${remainder}"
  KIND_IMAGE_SYNC_TAG="latest"
  if [[ "${remainder}" == *:* ]]; then
    KIND_IMAGE_SYNC_REPOSITORY="${remainder%:*}"
    KIND_IMAGE_SYNC_TAG="${remainder##*:}"
  fi
}

kind_image_sync_registry_tags() {
  local repository="$1"
  local registry_host="${KIND_IMAGE_SYNC_LOCAL_REGISTRY:-127.0.0.1:5001}"

  curl -fsSL "http://${registry_host}/v2/${repository}/tags/list" \
    | jq -r '.tags[]?' | sort -u
}

kind_image_sync_registry_has_tag() {
  local repository="$1"
  local tag="$2"

  kind_image_sync_registry_tags "${repository}" | grep -qx "${tag}"
}

kind_image_sync_ensure_image_available() {
  local target="$1"
  shift

  local candidate
  local pull_attempts="${KIND_IMAGE_SYNC_PULL_ATTEMPTS:-3}"
  local retry_delay_seconds="${KIND_IMAGE_SYNC_RETRY_DELAY_SECONDS:-2}"

  if docker image inspect "${target}" >/dev/null 2>&1; then
    return 0
  fi

  for candidate in "$@"; do
    [[ -z "${candidate}" ]] && continue
    if docker image inspect "${candidate}" >/dev/null 2>&1; then
      if [[ "${candidate}" != "${target}" ]]; then
        docker tag "${candidate}" "${target}" >/dev/null
      fi
      return 0
    fi
  done

  for candidate in "$@"; do
    [[ -z "${candidate}" ]] && continue
    kind_image_sync_log "pulling ${candidate}"
    if kind_image_sync_retry "${pull_attempts}" "${retry_delay_seconds}" docker pull "${candidate}" >/dev/null; then
      if [[ "${candidate}" != "${target}" ]]; then
        docker tag "${candidate}" "${target}" >/dev/null
      fi
      return 0
    fi
    kind_image_sync_log "failed to pull ${candidate}; trying next mirror"
  done

  kind_image_sync_log "unable to prepare image ${target}"
  return 1
}

kind_image_sync_ensure_registry_copy() {
  local source_image="$1"
  local target_image="$2"
  shift 2

  local repository
  local tag
  local push_attempts="${KIND_IMAGE_SYNC_PUSH_ATTEMPTS:-3}"
  local retry_delay_seconds="${KIND_IMAGE_SYNC_RETRY_DELAY_SECONDS:-2}"

  kind_image_sync_split_ref "${target_image}"
  repository="${KIND_IMAGE_SYNC_REPOSITORY}"
  tag="${KIND_IMAGE_SYNC_TAG}"

  if kind_image_sync_registry_has_tag "${repository}" "${tag}"; then
    kind_image_sync_log "local registry already has ${target_image}; skipping sync"
    return 0
  fi

  kind_image_sync_log "syncing ${source_image} -> ${target_image}"
  kind_image_sync_ensure_image_available "${source_image}" "${source_image}" "$@" || return 1

  if [[ "${source_image}" != "${target_image}" ]]; then
    docker tag "${source_image}" "${target_image}" >/dev/null
  fi

  if ! kind_image_sync_retry "${push_attempts}" "${retry_delay_seconds}" docker push "${target_image}" >/dev/null; then
    kind_image_sync_log "failed to push ${target_image} into local registry"
    return 1
  fi

  return 0
}
