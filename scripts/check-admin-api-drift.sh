#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
FAILED="false"

log() {
  printf '[admin-api-drift] %s\n' "$*"
}

check_go_contract() {
  log "running controlplane admin route contract test"
  if ! (cd "${ROOT_DIR}/controlplane" && go test ./internal/admin -run "TestControlplaneAdminRouteContractMatchesMachineReadableSurfaceDoc" -count=1 >/dev/null 2>&1); then
    log "FAIL: controlplane admin route contract mismatch between docs/contracts/admin-api-surface.json and controlplane/internal/admin/server.go"
    FAILED="true"
  else
    log "PASS: controlplane admin route contract matches surface doc"
  fi
}

check_surface_metadata() {
  log "running surface contract metadata validation"
  if ! (cd "${ROOT_DIR}/controlplane" && go test ./internal/admin -run "TestAdminSurfaceContractDocumentsVersioningMetadata" -count=1 >/dev/null 2>&1); then
    log "FAIL: admin API surface contract missing required metadata (displayName/basePath/defaultAuth/stability/versionPolicy)"
    FAILED="true"
  else
    log "PASS: surface contract metadata valid"
  fi
}

check_user_doc_endpoints() {
  local user_doc="${ROOT_DIR}/docs/user/admin-api.md"
  local surface_json="${ROOT_DIR}/docs/contracts/admin-api-surface.json"

  if [[ ! -f "${user_doc}" ]]; then
    log "SKIP: docs/user/admin-api.md not found"
    return
  fi

  log "checking documented endpoints against surface contract"
  local missing=""

  while IFS= read -r endpoint; do
    if ! grep -qF "\"path\": \"${endpoint}\"" "${surface_json}"; then
      missing="${missing}  ${endpoint}\n"
    fi
  done < <(grep -oP '`(GET|POST|PUT|DELETE) [^`]+`' "${user_doc}" | sed 's/`//g' | awk '{print $2}' | sort -u)

  if [[ -n "${missing}" ]]; then
    log "FAIL: user doc references endpoints not in surface contract:"
    printf '%b' "${missing}"
    FAILED="true"
  else
    log "PASS: all documented endpoints present in surface contract"
  fi
}

main() {
  check_go_contract
  check_surface_metadata
  check_user_doc_endpoints

  if [[ "${FAILED}" == "true" ]]; then
    log "admin API drift check FAILED"
    exit 1
  fi

  log "admin API drift check PASSED"
}

main "$@"