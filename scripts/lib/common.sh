#!/usr/bin/env bash
# shellcheck shell=bash

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  printf '[pgw] scripts/lib/common.sh is a helper library; source it from another script\n' >&2
  exit 2
fi

PGW_COMMON_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

aeg_repo_root() {
  printf '%s\n' "${PGW_COMMON_ROOT}"
}

aeg_log() {
  printf '[pgw] %s\n' "$*"
}

aeg_fail() {
  printf '[pgw] %s\n' "$*" >&2
  exit 1
}

aeg_usage_error() {
  printf '[pgw] %s\n' "$*" >&2
  exit 2
}

aeg_require_file() {
  local path="$1"
  [[ -f "${path}" ]] || aeg_fail "required file not found: ${path}"
}

aeg_require_dir() {
  local path="$1"
  [[ -d "${path}" ]] || aeg_fail "required directory not found: ${path}"
}

aeg_require_command() {
  local command_name="$1"
  command -v "${command_name}" >/dev/null 2>&1 \
    || aeg_fail "required command not found: ${command_name}"
}

aeg_require_pattern() {
  local pattern="$1"
  local path="$2"
  aeg_require_file "${path}"
  grep -Eq "${pattern}" "${path}" \
    || aeg_fail "required pattern not found in ${path}: ${pattern}"
}

aeg_safe_mkdir() {
  local path="$1"
  [[ -n "${path}" && "${path}" != "/" ]] \
    || aeg_usage_error "refusing to create unsafe directory: ${path}"
  mkdir -p "${path}"
}

aeg_git_tree_state() {
  local root="${1:-${PGW_COMMON_ROOT}}"

  if [[ -n "$(git -C "${root}" status --porcelain --untracked-files=all)" ]]; then
    printf 'dirty\n'
    return
  fi

  printf 'clean\n'
}

aeg_code_tree_state() {
  local root="${1:-${PGW_COMMON_ROOT}}"

  if [[ -n "$(git -C "${root}" status --porcelain --untracked-files=all -- . ':(exclude)reports/**' ':(exclude)tmp/**')" ]]; then
    printf 'dirty\n'
    return
  fi

  printf 'clean\n'
}
