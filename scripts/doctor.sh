#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STRICT="${STRICT:-false}"
CHECK_LOCAL_REGISTRY="${CHECK_LOCAL_REGISTRY:-true}"

usage() {
  cat <<'EOF'
Usage:
  scripts/doctor.sh

Checks local development prerequisites and prints actionable status lines.

Environment:
  STRICT=true                  Exit non-zero when required or recommended checks fail.
  CHECK_LOCAL_REGISTRY=false   Skip the kind-registry container status check.

Checks:
  Required: go, cargo, rustc, kubectl, kind, docker
  Recommended: perf, flamegraph or flamegraph.pl, kustomize or kubectl kustomize
  Local state: kind-registry container when Docker is available
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
  printf '[doctor] %s\n' "$*"
}

missing_required=0
missing_recommended=0

check_command() {
  local name="$1"
  local required="$2"
  local version_command="${3:-}"

  if ! command -v "${name}" >/dev/null 2>&1; then
    if [[ "${required}" == "required" ]]; then
      log "missing required command: ${name}"
      missing_required=1
    else
      log "missing recommended command: ${name}"
      missing_recommended=1
    fi
    return
  fi

  if [[ -n "${version_command}" ]]; then
    log "found ${name}: $(${version_command} 2>/dev/null | head -n 1 || printf 'version unavailable')"
  else
    log "found ${name}: $(command -v "${name}")"
  fi
}

check_kustomize() {
  if command -v kustomize >/dev/null 2>&1; then
    log "found kustomize: $(kustomize version 2>/dev/null | head -n 1 || printf 'version unavailable')"
    return
  fi
  if command -v kubectl >/dev/null 2>&1 && kubectl kustomize --help >/dev/null 2>&1; then
    log "found kustomize: kubectl kustomize"
    return
  fi

  log "missing recommended command: kustomize or kubectl kustomize"
  missing_recommended=1
}

check_flamegraph() {
  if command -v flamegraph >/dev/null 2>&1; then
    log "found flamegraph: $(command -v flamegraph)"
    return
  fi
  if command -v flamegraph.pl >/dev/null 2>&1; then
    log "found flamegraph: $(command -v flamegraph.pl)"
    return
  fi

  log "missing recommended command: flamegraph or flamegraph.pl"
  missing_recommended=1
}

check_local_registry() {
  if [[ "${CHECK_LOCAL_REGISTRY}" != "true" ]]; then
    log "skipped local registry check: CHECK_LOCAL_REGISTRY=false"
    return
  fi
  if ! command -v docker >/dev/null 2>&1; then
    log "skipped local registry check: docker is unavailable"
    return
  fi
  if docker ps --format '{{.Names}}' 2>/dev/null | grep -Fxq kind-registry; then
    log "found local registry: kind-registry is running"
    return
  fi

  log "local registry warning: kind-registry is not running"
  missing_recommended=1
}

log "repo: ${ROOT_DIR}"
check_command go required "go version"
check_command cargo required "cargo --version"
check_command rustc required "rustc --version"
check_command kubectl required "kubectl version --client=true"
check_command kind required "kind version"
check_command docker required "docker --version"
check_command perf recommended "perf --version"
check_flamegraph
check_kustomize
check_local_registry

if [[ "${missing_required}" -ne 0 ]]; then
  log "required prerequisite check failed"
  exit 1
fi
if [[ "${STRICT}" == "true" && "${missing_recommended}" -ne 0 ]]; then
  log "recommended prerequisite check failed in STRICT=true mode"
  exit 1
fi

log "doctor completed"
