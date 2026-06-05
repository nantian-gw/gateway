#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/scripts/lib/kind-image-sync.sh"

fail() {
  printf '[kind-image-sync-test] %s\n' "$*" >&2
  exit 1
}

assert_eq() {
  local actual="$1"
  local expected="$2"
  local label="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    fail "${label}: expected '${expected}', got '${actual}'"
  fi
}

assert_file_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if ! grep -Fq "${pattern}" "${path}"; then
    fail "${label}: expected '${pattern}' in ${path}"
  fi
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "${TMP_ROOT}"' EXIT

DOCKER_LOG="${TMP_ROOT}/docker.log"
KIND_IMAGE_SYNC_LOCAL_REGISTRY="127.0.0.1:5001"

sleep() {
  :
}

docker() {
  printf '%s\n' "$*" >>"${DOCKER_LOG}"

  case "$1 $2" in
    "image inspect")
      if [[ "$3" == "cached/source:1.0.0" ]]; then
        return 0
      fi
      return 1
      ;;
    "pull remote/fail:1.0.0")
      return 1
      ;;
    "pull remote/retry:1.0.0")
      local state_file="${TMP_ROOT}/pull.retry.count"
      local count=0
      if [[ -f "${state_file}" ]]; then
        count="$(cat "${state_file}")"
      fi
      count=$((count + 1))
      printf '%s' "${count}" >"${state_file}"
      if (( count < 3 )); then
        return 1
      fi
      return 0
      ;;
    "pull "*)
      return 0
      ;;
    "tag "*)
      return 0
      ;;
    "push 127.0.0.1:5001/project/image:smoke")
      return 0
      ;;
    "push 127.0.0.1:5001/project/retry:smoke")
      local state_file="${TMP_ROOT}/push.retry.count"
      local count=0
      if [[ -f "${state_file}" ]]; then
        count="$(cat "${state_file}")"
      fi
      count=$((count + 1))
      printf '%s' "${count}" >"${state_file}"
      if (( count < 2 )); then
        return 1
      fi
      return 0
      ;;
  esac

  return 1
}

kind_image_sync_registry_has_tag() {
  local repository="$1"
  local tag="$2"

  if [[ "${repository}" == "project/already-there" && "${tag}" == "smoke" ]]; then
    return 0
  fi
  return 1
}

: >"${DOCKER_LOG}"
kind_image_sync_ensure_registry_copy \
  "remote/source:1.0.0" \
  "127.0.0.1:5001/project/already-there:smoke"

assert_eq "$(wc -l <"${DOCKER_LOG}" | tr -d ' ')" "0" \
  "skip docker operations when target already exists in local registry"

: >"${DOCKER_LOG}"
kind_image_sync_ensure_registry_copy \
  "cached/source:1.0.0" \
  "127.0.0.1:5001/project/image:smoke" \
  "remote/fail:1.0.0"

assert_file_contains "${DOCKER_LOG}" \
  "image inspect cached/source:1.0.0" \
  "inspect cached source image"
assert_file_contains "${DOCKER_LOG}" \
  "tag cached/source:1.0.0 127.0.0.1:5001/project/image:smoke" \
  "tag cached source into local registry target"
assert_file_contains "${DOCKER_LOG}" \
  "push 127.0.0.1:5001/project/image:smoke" \
  "push tagged image into local registry"

: >"${DOCKER_LOG}"
kind_image_sync_ensure_registry_copy \
  "remote/retry:1.0.0" \
  "127.0.0.1:5001/project/retry:smoke" \
  "remote/retry:1.0.0"

assert_eq "$(cat "${TMP_ROOT}/pull.retry.count")" "3" "retry remote pulls"
assert_eq "$(cat "${TMP_ROOT}/push.retry.count")" "2" "retry local registry pushes"
assert_file_contains "${DOCKER_LOG}" \
  "pull remote/retry:1.0.0" \
  "pull remote image"
assert_file_contains "${DOCKER_LOG}" \
  "push 127.0.0.1:5001/project/retry:smoke" \
  "push image after successful pull"

printf '[kind-image-sync-test] ok\n'
