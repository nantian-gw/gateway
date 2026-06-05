#!/usr/bin/env bash
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLEAN_ARTIFACTS_ROOT="${CLEAN_ARTIFACTS_ROOT:-${SCRIPT_ROOT}}"
DRY_RUN="${DRY_RUN:-false}"
usage() {
  cat <<'EOF'
Usage:
  scripts/clean-artifacts.sh

Safely removes local generated artifacts while preserving committed reports.

Environment:
  CLEAN_ARTIFACTS_ROOT=<path>  Repository root to clean. Defaults to this repo.
  DRY_RUN=true                 Print what would be removed without deleting anything.

Removed by default:
  tmp/
  target/
  dataplane/target/
  dashboard/.next/

Preserved:
  reports/
  docs/
  source files and dependency lock files
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "$#" -ne 0 ]]; then
  usage >&2
  exit 2
fi

log() {
  printf '[clean-artifacts] %s\n' "$*"
}

repo_root="$(cd "${CLEAN_ARTIFACTS_ROOT}" && pwd -P)"

if [[ -z "${repo_root}" || "${repo_root}" == "/" ]]; then
  log "refusing to clean unsafe root: ${repo_root:-<empty>}"
  exit 2
fi
if [[ ! -f "${repo_root}/Makefile" || ! -f "${repo_root}/dataplane/Cargo.toml" ]]; then
  log "refusing to clean ${repo_root}: expected Makefile and dataplane/Cargo.toml markers"
  exit 2
fi

declare -a paths=(
  "tmp"
  "target"
  "dataplane/target"
  "dashboard/.next"
)

remove_path() {
  local rel="$1"
  local abs="${repo_root}/${rel}"

  if [[ ! -e "${abs}" ]]; then
    log "skipped ${rel}: not present"
    return
  fi
  if [[ "${rel}" == reports || "${rel}" == reports/* ]]; then
    log "refusing to remove reports path: ${rel}"
    exit 2
  fi
  if [[ "${DRY_RUN}" == "true" ]]; then
    log "would remove ${rel}"
    return
  fi

  rm -rf -- "${abs}"
  log "removed ${rel}"
}

log "repo: ${repo_root}"
for rel in "${paths[@]}"; do
  remove_path "${rel}"
done
log "preserved reports/"
