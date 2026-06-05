#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPAT_BASE_REF="${COMPAT_BASE_REF:-}"
SKIP_PROTO="${SKIP_PROTO:-false}"
SKIP_PROTO_COMPAT="${SKIP_PROTO_COMPAT:-false}"
SKIP_CONTROLPLANE_TESTS="${SKIP_CONTROLPLANE_TESTS:-false}"
SKIP_DATAPLANE_TESTS="${SKIP_DATAPLANE_TESTS:-false}"
MIXED_VERSION_VALIDATE="${MIXED_VERSION_VALIDATE:-false}"
MIXED_VERSION_BASE_REF="${MIXED_VERSION_BASE_REF:-}"
SKEW_WORKTREE_PARENT="${SKEW_WORKTREE_PARENT:-${ROOT_DIR}/tmp}"

log() {
  printf '[skew-validation] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

run_proto() {
  require_command make
  log "generating proto bindings"
  (
    cd "${ROOT_DIR}"
    make proto
  )
}

run_proto_compat() {
  require_command go
  require_command git
  require_command protoc

  log "checking gateway/control/v1 proto compatibility"
  (
    cd "${ROOT_DIR}/controlplane"
    if [[ -n "${COMPAT_BASE_REF}" ]]; then
      GOWORK=off go run ./cmd/proto-compat-check \
        -repo-root "${ROOT_DIR}" \
        -base-ref "${COMPAT_BASE_REF}"
    else
      GOWORK=off go run ./cmd/proto-compat-check \
        -repo-root "${ROOT_DIR}"
    fi
  )
}

run_controlplane_tests() {
  require_command go
  log "running controlplane tests"
  (
    cd "${ROOT_DIR}/controlplane"
    go test ./...
  )
}

run_dataplane_tests() {
  require_command cargo
  log "running dataplane workspace tests"
  (
    cd "${ROOT_DIR}"
    cargo test --manifest-path dataplane/Cargo.toml --workspace
  )
}

detect_adjacent_ref() {
  local ref

  if [[ -n "${MIXED_VERSION_BASE_REF}" ]]; then
    echo "${MIXED_VERSION_BASE_REF}"
    return 0
  fi

  ref="$(git -C "${ROOT_DIR}" describe --tags --abbrev=0 --match 'v*' 2>/dev/null)" || true
  if [[ -n "${ref}" ]]; then
    echo "${ref}"
    return 0
  fi

  ref="$(git -C "${ROOT_DIR}" rev-parse HEAD^ 2>/dev/null)" || true
  if [[ -n "${ref}" ]]; then
    echo "${ref}"
    return 0
  fi

  return 1
}

run_mixed_version_validate() {
  require_command git
  require_command go
  require_command cargo
  require_command make

  local base_ref
  base_ref="$(detect_adjacent_ref)" || {
    log "mixed-version: no adjacent ref found, skipping"
    return 0
  }

  local current_ref
  current_ref="$(git -C "${ROOT_DIR}" rev-parse HEAD)"

  if [[ "${base_ref}" == "${current_ref}" ]]; then
    log "mixed-version: base ref equals current ref, no skew to validate"
    return 0
  fi

  log "mixed-version: validating adjacent skew (current=${current_ref:0:7}, base=${base_ref})"

  mkdir -p "${SKEW_WORKTREE_PARENT}"

  local worktree_dir
  worktree_dir="$(mktemp -d "${SKEW_WORKTREE_PARENT}/.skew-wt-XXXXXX")"

  log "mixed-version: creating worktree at base ref"
  if ! git -C "${ROOT_DIR}" worktree add --detach "${worktree_dir}" "${base_ref}" >/dev/null 2>&1; then
    log "mixed-version: failed to create worktree, skipping"
    rm -rf "${worktree_dir}"
    return 0
  fi

  local proto_compat_ok=true

  log "mixed-version: checking proto compatibility current->base direction"
  if ! (
    cd "${ROOT_DIR}/controlplane"
    GOWORK=off go run ./cmd/proto-compat-check \
      -repo-root "${ROOT_DIR}" \
      -base-ref "${base_ref}" >/dev/null 2>&1
  ); then
    log "mixed-version: FAIL proto compatibility current->base"
    proto_compat_ok=false
  fi

  if [[ -d "${worktree_dir}/controlplane/cmd/proto-compat-check" ]]; then
    log "mixed-version: checking proto compatibility base->current direction"
    if ! (
      cd "${worktree_dir}/controlplane"
      GOWORK=off go run ./cmd/proto-compat-check \
        -repo-root "${worktree_dir}" \
        -base-ref "${current_ref}" >/dev/null 2>&1
    ); then
      log "mixed-version: FAIL proto compatibility base->current"
      proto_compat_ok=false
    fi
  else
    log "mixed-version: base ref has no proto-compat-check; skipping base->current direction"
  fi

  if [[ "${proto_compat_ok}" != "true" ]]; then
    git -C "${ROOT_DIR}" worktree remove --force "${worktree_dir}" 2>/dev/null || true
    rm -rf "${worktree_dir}"
    log "mixed-version: proto compatibility check failed"
    return 1
  fi

  log "mixed-version: generating proto bindings at base ref"
  if ! (
    cd "${worktree_dir}"
    make proto
  ); then
    git -C "${ROOT_DIR}" worktree remove --force "${worktree_dir}" 2>/dev/null || true
    rm -rf "${worktree_dir}"
    log "mixed-version: FAIL proto generation at base ref"
    return 1
  fi

  log "mixed-version: validating controlplane builds at base ref"
  if ! (
    cd "${worktree_dir}/controlplane"
    go build ./cmd/manager >/dev/null 2>&1
  ); then
    git -C "${ROOT_DIR}" worktree remove --force "${worktree_dir}" 2>/dev/null || true
    rm -rf "${worktree_dir}"
    log "mixed-version: FAIL controlplane build at base ref"
    return 1
  fi

  log "mixed-version: validating dataplane builds at base ref"
  if ! (
    cd "${worktree_dir}"
    cargo build --manifest-path dataplane/Cargo.toml --workspace --quiet 2>/dev/null
  ); then
    git -C "${ROOT_DIR}" worktree remove --force "${worktree_dir}" 2>/dev/null || true
    rm -rf "${worktree_dir}"
    log "mixed-version: FAIL dataplane build at base ref"
    return 1
  fi

  git -C "${ROOT_DIR}" worktree remove --force "${worktree_dir}" 2>/dev/null || true
  rm -rf "${worktree_dir}"
  log "mixed-version: adjacent-version validation passed"
  return 0
}

main() {
  if [[ "${SKIP_PROTO}" != "true" ]]; then
    run_proto
  fi

  if [[ "${SKIP_PROTO_COMPAT}" != "true" ]]; then
    run_proto_compat
  fi

  if [[ "${SKIP_CONTROLPLANE_TESTS}" != "true" ]]; then
    run_controlplane_tests
  fi

  if [[ "${SKIP_DATAPLANE_TESTS}" != "true" ]]; then
    run_dataplane_tests
  fi

  if [[ "${MIXED_VERSION_VALIDATE}" == "true" ]]; then
    run_mixed_version_validate
  fi

  log "skew validation passed"
}

main "$@"
