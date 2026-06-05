#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${repo_root}/scripts/lib/common.sh"

usage() {
  cat <<'EOF' >&2
usage: check-evidence-reference-alignment.sh [--repo-root <path>]

Verifies that the key conformance evidence documents still point at the current
reports/conformance/latest run and the most recent clean full-suite run.
EOF
}

log() {
  printf '[evidence-alignment] %s\n' "$*"
}

extract_scalar() {
  local key="$1"
  local file="$2"

  awk -F': ' -v wanted="${key}" '
    $1 == wanted {
      value = $2
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
  ' "${file}"
}

extract_implementation_version() {
  local file="$1"

  awk '
    $1 == "report:" {
      in_report = 1
      next
    }
    in_report && $1 == "implementationVersion:" {
      value = $2
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
    in_report && /^[^[:space:]]/ {
      in_report = 0
    }
  ' "${file}"
}

extract_result() {
  extract_scalar "result" "$1"
}

extract_run_date() {
  printf '%s' "$1" | cut -d- -f1-3
}

require_contains() {
  local file="$1"
  local expected="$2"
  local label="$3"

  aeg_require_file "${file}"
  if ! grep -Fq -- "${expected}" "${file}"; then
    aeg_fail "${file} is missing ${label}: ${expected}"
  fi
}

discover_latest_clean_metadata() {
  local runs_root="$1"
  local latest_path=""
  local candidate
  local result
  local run_id
  local implementation_version

  while IFS= read -r candidate; do
    result="$(extract_result "${candidate}")"
    run_id="$(extract_scalar "id" "${candidate}")"
    implementation_version="$(extract_implementation_version "${candidate}")"
    if [[ "${result}" == "passed" \
      && -n "${run_id}" \
      && "${run_id}" == *-full \
      && -n "${implementation_version}" \
      && "${implementation_version}" != *-dirty ]]; then
      latest_path="${candidate}"
    fi
  done < <(find "${runs_root}" -mindepth 2 -maxdepth 2 -path '*/metadata.yaml' | sort)

  [[ -n "${latest_path}" ]] || aeg_fail "failed to discover latest clean passed full-suite conformance metadata under ${runs_root}"
  printf '%s\n' "${latest_path}"
}

extract_first_run_ref() {
  local file="$1"

  python3 - "$file" <<'PY'
from pathlib import Path
import re
import sys

text = Path(sys.argv[1]).read_text(encoding="utf-8")
for match in re.finditer(r"runs/([^/]+)/", text):
    run_id = match.group(1)
    if "<" in run_id or ">" in run_id:
        continue
    print(run_id)
    raise SystemExit(0)
raise SystemExit(f"missing run reference in {sys.argv[1]}")
PY
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --repo-root"
      }
      repo_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      aeg_usage_error "unknown argument: $1"
      ;;
  esac
done

latest_metadata="${repo_root}/reports/conformance/latest/metadata.yaml"
runs_root="${repo_root}/reports/conformance/runs"
readme_path="${repo_root}/reports/conformance/README.md"
performance_path="${repo_root}/reports/performance/README.md"
chaos_path="${repo_root}/reports/chaos/README.md"
soak_path="${repo_root}/reports/soak/README.md"
baseline_path="${repo_root}/docs/test/latest-baseline.md"
support_path="${repo_root}/docs/gateway-api-support.md"
community_path="${repo_root}/docs/community-readiness.md"
changelog_path="${repo_root}/CHANGELOG.md"

aeg_require_file "${latest_metadata}"
aeg_require_file "${readme_path}"
aeg_require_file "${performance_path}"
aeg_require_file "${chaos_path}"
aeg_require_file "${soak_path}"
aeg_require_file "${baseline_path}"
aeg_require_file "${support_path}"
aeg_require_file "${community_path}"
aeg_require_file "${changelog_path}"

clean_metadata="$(discover_latest_clean_metadata "${runs_root}")"

latest_id="$(extract_scalar "id" "${latest_metadata}")"
latest_commit="$(extract_implementation_version "${latest_metadata}")"
clean_id="$(extract_scalar "id" "${clean_metadata}")"
clean_commit="$(extract_implementation_version "${clean_metadata}")"

[[ -n "${latest_id}" ]] || aeg_fail "failed to read latest report id from ${latest_metadata}"
[[ -n "${latest_commit}" ]] || aeg_fail "failed to read latest implementationVersion from ${latest_metadata}"
[[ -n "${clean_id}" ]] || aeg_fail "failed to read clean report id from ${clean_metadata}"
[[ -n "${clean_commit}" ]] || aeg_fail "failed to read clean implementationVersion from ${clean_metadata}"

latest_date="$(extract_run_date "${latest_id}")"
clean_date="$(extract_run_date "${clean_id}")"
performance_run_id="$(extract_first_run_ref "${performance_path}")"
chaos_run_id="$(extract_first_run_ref "${chaos_path}")"
soak_run_id="$(extract_first_run_ref "${soak_path}")"

require_contains "${readme_path}" "runs/${latest_id}/" "latest archived run reference"
require_contains "${readme_path}" "runs/${clean_id}/" "latest clean run reference"

require_contains "${baseline_path}" "${latest_id}" "latest archived run reference"
require_contains "${baseline_path}" "${clean_id}" "latest clean run reference"

require_contains "${support_path}" "${latest_date}" "latest archived run date"
require_contains "${support_path}" "${latest_commit}" "latest archived commit"
require_contains "${support_path}" "${clean_date}" "latest clean run date"
require_contains "${support_path}" "${clean_commit}" "latest clean commit"

require_contains "${community_path}" "${latest_date}" "latest archived run date"
require_contains "${community_path}" "${latest_commit}" "latest archived commit"
require_contains "${community_path}" "${clean_date}" "latest clean run date"
require_contains "${community_path}" "${clean_commit}" "latest clean commit"

require_contains "${changelog_path}" "${latest_date}" "latest archived run date"
require_contains "${changelog_path}" "${latest_commit}" "latest archived commit"
require_contains "${changelog_path}" "${clean_date}" "latest clean run date"
require_contains "${changelog_path}" "${clean_commit}" "latest clean commit"
require_contains "${changelog_path}" "${performance_run_id}" "performance run reference"
require_contains "${changelog_path}" "${chaos_run_id}" "chaos run reference"
require_contains "${changelog_path}" "${soak_run_id}" "soak run reference"

log "latest archived run: ${latest_id} (${latest_commit})"
log "latest clean run: ${clean_id} (${clean_commit})"
log "performance run: ${performance_run_id}"
log "chaos run: ${chaos_run_id}"
log "soak run: ${soak_run_id}"
log "evidence references aligned"
