#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATAPLANE_CARGO_TOML="${ROOT_DIR}/dataplane/Cargo.toml"
DATAPLANE_CARGO_LOCK="${ROOT_DIR}/dataplane/Cargo.lock"
THIRD_PARTY_DIR="${ROOT_DIR}/dataplane/third_party"

log() {
  printf '[audit-vendored-upstream] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

require_file() {
  local path="$1"

  if [[ ! -f "${path}" ]]; then
    log "missing required file: ${path}"
    exit 1
  fi
}

assert_not_contains() {
  local path="$1"
  local needle="$2"
  local description="$3"

  if grep -Fq -- "${needle}" "${path}"; then
    log "unexpected ${description} in ${path}"
    exit 1
  fi
}

workspace_upstream_version() {
  awk '
    /^\[workspace\.dependencies\]/ { in_section=1; next }
    /^\[/ && $0 !~ /^\[workspace\.dependencies\]/ { in_section=0 }
    in_section && $1 == "upstream" {
      if (match($0, /version = "([^"]+)"/, found)) {
        print found[1]
        exit
      }
    }
  ' "${DATAPLANE_CARGO_TOML}"
}

lockfile_upstream_version() {
  awk '
    /^\[\[package\]\]/ {
      in_package=1
      name=""
      version=""
      next
    }
    in_package && $1 == "name" && $3 == "\"upstream\"" {
      name="upstream"
      next
    }
    in_package && $1 == "version" && name == "upstream" {
      gsub(/"/, "", $3)
      print $3
      exit
    }
  ' "${DATAPLANE_CARGO_LOCK}"
}

main() {
  local expected_version
  local locked_version

  require_command git
  require_command grep
  require_command awk
  require_file "${DATAPLANE_CARGO_TOML}"
  require_file "${DATAPLANE_CARGO_LOCK}"

  assert_not_contains \
    "${DATAPLANE_CARGO_TOML}" \
    'upstream-core = { path = "third_party/upstream-core" }' \
    "upstream-core vendored patch"
  assert_not_contains \
    "${DATAPLANE_CARGO_TOML}" \
    'upstream-proxy = { path = "third_party/upstream-proxy" }' \
    "upstream-proxy vendored patch"

  if [[ -e "${THIRD_PARTY_DIR}" ]]; then
    log "unexpected vendored directory present: ${THIRD_PARTY_DIR}"
    exit 1
  fi

  expected_version="$(workspace_upstream_version)"
  if [[ -z "${expected_version}" ]]; then
    log "failed to determine workspace upstream version"
    exit 1
  fi

  locked_version="$(lockfile_upstream_version)"
  if [[ -z "${locked_version}" ]]; then
    log "failed to determine lockfile upstream version"
    exit 1
  fi

  if [[ "${expected_version}" != "${locked_version}" ]]; then
    log "workspace upstream version ${expected_version} does not match lockfile ${locked_version}"
    exit 1
  fi

  log "upstream-only Upstream audit passed"
  printf 'workspace version\t%s\n' "${expected_version}"
  printf 'lockfile version\t%s\n' "${locked_version}"

  printf '\nrecent migration commits:\n'
  git -C "${ROOT_DIR}" log --oneline --max-count=8 -- \
    dataplane/Cargo.toml \
    dataplane/Cargo.lock \
    docs/developer/third-party.md \
    docs/developer/upstream-upstream-migration.md \
    scripts/audit-vendored-upstream.sh

  printf '\nrecommended regression commands:\n'
  printf 'cd controlplane && go test ./internal/backendtls ./internal/translator ./internal/status\n'
  printf 'cargo test --manifest-path dataplane/Cargo.toml -p aeg-http\n'
  printf 'cargo test --manifest-path dataplane/Cargo.toml --workspace\n'
}

main "$@"
