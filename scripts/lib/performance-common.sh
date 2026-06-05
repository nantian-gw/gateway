#!/usr/bin/env bash
# shellcheck shell=bash

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  printf '[pgw] scripts/lib/performance-common.sh is a helper library; source it from another script\n' >&2
  exit 2
fi

# shellcheck source=scripts/lib/common.sh
source "${PGW_COMMON_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}/scripts/lib/common.sh"

aeg_perf_log() {
  local prefix="${1:-perf}"
  shift
  printf '[%s] %s\n' "${prefix}" "$*"
}

aeg_perf_require_command() {
  local prefix="$1"
  local name="$2"
  if ! command -v "${name}" >/dev/null 2>&1; then
    aeg_perf_log "${prefix}" "missing required command: ${name}"
    exit 1
  fi
}

aeg_perf_run_id() {
  local suffix="$1"
  local root_dir="${2:-${PGW_COMMON_ROOT}}"
  printf '%s-%s-%s\n' "$(date +%Y-%m-%d-%H%M%S)" "$(git -C "${root_dir}" rev-parse --short HEAD)" "${suffix}"
}

aeg_perf_metadata_common() {
  local root_dir="$1"
  local run_id="$2"
  local git_tree_state
  local code_tree_state
  git_tree_state="$(aeg_git_tree_state "${root_dir}")"
  code_tree_state="$(aeg_code_tree_state "${root_dir}")"
  printf 'captured_at=%s\n' "$(date --iso-8601=seconds)"
  printf 'git_commit=%s\n' "$(git -C "${root_dir}" rev-parse HEAD)"
  printf 'git_tree_state=%s\n' "${git_tree_state}"
  printf 'code_tree_state=%s\n' "${code_tree_state}"
  printf 'run_id=%s\n' "${run_id}"
  printf 'kernel=%s\n' "$(uname -srmo)"
  printf 'cpu_count=%s\n' "$(nproc)"
  printf 'memory_kib=%s\n' "$(awk '/MemTotal:/ {print $2}' /proc/meminfo)"
}
