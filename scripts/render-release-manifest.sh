#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
install_profile="kind-dev"

usage() {
  local status="${1:-2}"
  local stream="/dev/stderr"

  if [[ "${status}" == "0" ]]; then
    stream="/dev/stdout"
  fi

  cat >"${stream}" <<EOF
usage: ${0##*/} [--repo-root <path>] [--profile <name>] <controlplane-image> <dataplane-image> <output-file>

profiles:
  kind-dev
  kind-hostnetwork-perf
  single-cluster-prod
  multi-replica-prod
  observability-enabled
EOF
  exit "${status}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h)
      usage 0
      ;;
    --repo-root)
      [[ $# -ge 2 ]] || usage
      repo_root="$2"
      shift 2
      ;;
    --profile)
      [[ $# -ge 2 ]] || usage
      install_profile="$2"
      shift 2
      ;;
    --*)
      printf 'unknown option: %s\n' "$1" >&2
      usage
      ;;
    *)
      break
      ;;
  esac
done

if [[ $# -ne 3 ]]; then
  usage
fi

CONTROL_IMAGE="$1"
DATAPLANE_IMAGE="$2"
OUTPUT_FILE="$3"

profile_source_rel() {
  local profile="$1"

  case "${profile}" in
    kind-dev)
      printf 'deploy/kubernetes/overlays/kind\n'
      ;;
    kind-hostnetwork-perf)
      printf 'deploy/kubernetes/overlays/kind-hostnetwork\n'
      ;;
    single-cluster-prod|multi-replica-prod|observability-enabled)
      printf 'deploy/kubernetes/overlays/production\n'
      ;;
    *)
      printf 'unknown install profile: %s\n' "${profile}" >&2
      printf 'supported profiles: kind-dev, kind-hostnetwork-perf, single-cluster-prod, multi-replica-prod, observability-enabled\n' >&2
      exit 2
      ;;
  esac
}

profile_note() {
  local profile="$1"

  case "${profile}" in
    kind-dev)
      printf 'kind-dev is intended for local and Kind-oriented bring-up; update statusAddress before non-local deployment.\n'
      ;;
    kind-hostnetwork-perf)
      printf 'kind-hostnetwork-perf renders the Kind hostNetwork dataplane profile for controlled performance tests; target node IPs directly instead of ClusterIP Services.\n'
      ;;
    single-cluster-prod)
      printf 'single-cluster-prod renders the production overlay; create real production Secrets before applying.\n'
      ;;
    multi-replica-prod)
      printf 'multi-replica-prod renders the production overlay with HPA/PDB defaults; tune replica and capacity limits for the target cluster.\n'
      ;;
    observability-enabled)
      printf 'observability-enabled renders the production overlay with metrics Services; apply optional PrometheusRule assets only when the CRD exists.\n'
      ;;
  esac
}

SOURCE_REL="$(profile_source_rel "${install_profile}")"
SOURCE_DIR="${repo_root}/${SOURCE_REL}"

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "${name}" >&2
    exit 1
  fi
}

escape_sed_replacement() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

mkdir -p "$(dirname "${OUTPUT_FILE}")"
require_command kubectl

{
  printf '# Aether Gateway release install manifest\n'
  printf '# Install profile: %s\n' "${install_profile}"
  printf '# Rendered from %s/.\n' "${SOURCE_REL}"
  printf '#\n'
  printf '# Images:\n'
  printf '#   controlplane: %s\n' "${CONTROL_IMAGE}"
  printf '#   dataplane:    %s\n' "${DATAPLANE_IMAGE}"
  printf '#\n'
  printf '# Note:\n'
  printf '#   %s\n' "$(profile_note "${install_profile}")"
  kubectl kustomize "${SOURCE_DIR}" | sed \
    -e "s|nantian-controlplane:dev|$(escape_sed_replacement "${CONTROL_IMAGE}")|g" \
    -e "s|nantian-dataplane:dev|$(escape_sed_replacement "${DATAPLANE_IMAGE}")|g"
} >"${OUTPUT_FILE}"
