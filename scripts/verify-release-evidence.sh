#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${repo_root}/scripts/lib/common.sh"

candidate_commit=""
conformance_metadata=""
performance_metadata=""
chaos_metadata=""
soak_metadata=""
allow_dirty_conformance="false"
allow_dirty_performance_code_tree="false"
allow_dirty_chaos_code_tree="false"
allow_dirty_soak_code_tree="false"
allowed_commits=()
required_soak_duration_seconds=86400

usage() {
  cat <<'EOF'
usage: verify-release-evidence.sh \
  --candidate <git-ref> \
  --conformance <metadata.yaml> \
  --performance <metadata.txt> \
  --chaos <metadata.txt> \
  --soak <metadata.txt> \
  [--allow-commit <git-ref>]... \
  [--allow-dirty-conformance] \
  [--allow-dirty-performance-code-tree] \
  [--allow-dirty-chaos-code-tree] \
  [--allow-dirty-soak-code-tree]

Validates that release-candidate evidence points at the candidate commit or an
explicitly allowed commit window. Chaos evidence must have
conclusions/summary.json with release_gate_status=pass and traffic/summary.json
with slo_gate.status=pass. Soak evidence must record duration_seconds>=86400 in
metadata.txt and traffic/summary.json with slo_gate.status=pass. Performance
evidence must include throughput-report.json with complete protocol, scenario,
and reload live-traffic coverage plus source kind A4 slo-gate.json with
status=pass. Performance, chaos, and soak evidence must record
code_tree_state=clean unless explicitly risk-accepted.
EOF
}

log() {
  printf '[release-evidence] %s\n' "$*"
}

strip_dirty_suffix() {
  printf '%s' "$1" | sed 's/-dirty$//'
}

is_dirty_ref() {
  [[ "$1" == *-dirty ]]
}

commit_matches() {
  local left right
  left="$(strip_dirty_suffix "$1")"
  right="$(strip_dirty_suffix "$2")"

  [[ -n "${left}" && -n "${right}" ]] || return 1

  [[ "${left}" == "${right}" ]] \
    || [[ "${left}" == "${right}"* ]] \
    || [[ "${right}" == "${left}"* ]]
}

commit_allowed() {
  local actual="$1"
  shift

  local expected
  for expected in "$@"; do
    if commit_matches "${actual}" "${expected}"; then
      return 0
    fi
  done

  return 1
}

extract_conformance_impl_version() {
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

extract_metadata_commit() {
  local file="$1"

  awk -F= '
    $1 == "git_commit" {
      print $2
      exit
    }
  ' "${file}"
}

extract_metadata_value() {
  local key="$1"
  local file="$2"

  awk -F= -v wanted="${key}" '
    $1 == wanted {
      print $2
      exit
    }
  ' "${file}"
}

verify_conformance() {
  local actual_commit="$1"
  shift

  if is_dirty_ref "${actual_commit}" && [[ "${allow_dirty_conformance}" != "true" ]]; then
    aeg_fail "conformance implementationVersion ${actual_commit} is dirty; release candidates require a clean full-suite baseline"
  fi

  if ! commit_allowed "${actual_commit}" "$@"; then
    aeg_fail "conformance implementationVersion ${actual_commit} does not match candidate or allowed commits"
  fi

  log "conformance implementationVersion: ${actual_commit}"
}

verify_benchmark_metadata() {
  local label="$1"
  local actual_commit="$2"
  shift 2

  if ! commit_allowed "${actual_commit}" "$@"; then
    aeg_fail "${label} evidence git_commit ${actual_commit} does not match candidate or allowed commits"
  fi

  log "${label} git_commit: ${actual_commit}"
}

verify_code_tree_state() {
  local label="$1"
  local state="$2"
  local allow_dirty="$3"

  if [[ -z "${state}" ]]; then
    if [[ "${allow_dirty}" == "true" ]]; then
      log "${label} code_tree_state: missing (risk-accepted)"
      return
    fi
    aeg_fail "${label} evidence is missing code_tree_state; rerun evidence with current scripts or explicitly accept the risk"
  fi

  if [[ "${state}" != "clean" ]]; then
    if [[ "${allow_dirty}" == "true" ]]; then
      log "${label} code_tree_state: ${state} (risk-accepted)"
      return
    fi
    aeg_fail "${label} evidence code_tree_state ${state} is not clean; release candidates require clean code-tree evidence"
  fi

  log "${label} code_tree_state: ${state}"
}

verify_performance_coverage() {
  local metadata="$1"
  local report output

  report="$(dirname "${metadata}")/throughput-report.json"
  [[ -f "${report}" ]] || aeg_fail "performance evidence is missing throughput report: ${report}"

  if ! output="$(python3 - "${report}" <<'PY' 2>&1
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
payload = json.loads(path.read_text(encoding="utf-8"))

checks = [
    (("coverage", "missing_protocols"), "coverage missing_protocols"),
    (("coverage", "missing_scenarios"), "coverage missing_scenarios"),
    (
        ("reload", "live_traffic", "missing_protocols"),
        "reload live traffic missing_protocols",
    ),
    (
        ("reload", "live_traffic", "missing_mutations"),
        "reload live traffic missing_mutations",
    ),
]

for keys, label in checks:
    current = payload
    for key in keys:
        if not isinstance(current, dict) or key not in current:
            raise SystemExit(f"performance evidence {label} is missing")
        current = current[key]
    if not isinstance(current, list):
        raise SystemExit(f"performance evidence {label} is not an array")
    if current:
        missing = ", ".join(str(item) for item in current)
        raise SystemExit(
            f"performance evidence {label} is not empty: {missing}"
        )
PY
  )"; then
    aeg_fail "${output}"
  fi

  log "performance throughput coverage: complete"
}

performance_source_slo_gate_path() {
  local metadata="$1"
  local metadata_dir

  metadata_dir="$(dirname "${metadata}")"
  if [[ -f "${metadata_dir}/source-kind-a4/slo-gate.json" ]]; then
    printf '%s\n' "${metadata_dir}/source-kind-a4/slo-gate.json"
    return 0
  fi
  if [[ -f "${metadata_dir}/slo-gate.json" ]]; then
    printf '%s\n' "${metadata_dir}/slo-gate.json"
    return 0
  fi

  return 1
}

verify_performance_source_slo_gate() {
  local metadata="$1"
  local summary output

  if ! summary="$(performance_source_slo_gate_path "${metadata}")"; then
    aeg_fail "performance evidence is missing source SLO gate: $(dirname "${metadata}")/source-kind-a4/slo-gate.json"
  fi

  if ! output="$(python3 - "${summary}" <<'PY' 2>&1
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
payload = json.loads(path.read_text(encoding="utf-8"))
status = str(payload.get("status", "")).strip()
if not status:
    raise SystemExit(f"performance evidence is missing source SLO gate status in {path}")
if status != "pass":
    raise SystemExit(f"performance evidence source SLO gate {status} is not pass")

profiles = payload.get("profiles", {})
if not isinstance(profiles, dict):
    raise SystemExit("performance evidence source SLO gate profiles is not an object")
failed_profiles = []
for name, profile in sorted(profiles.items()):
    if not isinstance(profile, dict):
        failed_profiles.append(str(name))
        continue
    if str(profile.get("status", "")).strip() != "pass":
        failed_profiles.append(str(name))
if failed_profiles:
    raise SystemExit(
        "performance evidence source SLO gate has failed profiles: "
        + ", ".join(failed_profiles)
    )
PY
  )"; then
    aeg_fail "${output}"
  fi

  log "performance source SLO gate: pass"
}

verify_chaos_release_gate() {
  local metadata="$1"
  local summary status

  summary="$(dirname "${metadata}")/conclusions/summary.json"
  aeg_require_file "${summary}"

  status="$(python3 - "${summary}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
print(str(payload.get("release_gate_status", "")).strip())
PY
)"

  [[ -n "${status}" ]] || aeg_fail "chaos evidence is missing release_gate_status in ${summary}"
  if [[ "${status}" != "pass" ]]; then
    aeg_fail "chaos evidence release_gate_status ${status} is not pass"
  fi

  log "chaos release_gate_status: ${status}"
}

verify_chaos_traffic_slo_gate() {
  local metadata="$1"
  local summary status

  summary="$(dirname "${metadata}")/traffic/summary.json"
  [[ -f "${summary}" ]] || aeg_fail "chaos evidence is missing traffic SLO summary: ${summary}"

  status="$(python3 - "${summary}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
print(str((payload.get("slo_gate") or {}).get("status", "")).strip())
PY
)"

  [[ -n "${status}" ]] || aeg_fail "chaos evidence is missing traffic SLO gate status in ${summary}"
  if [[ "${status}" != "pass" ]]; then
    aeg_fail "chaos evidence traffic SLO gate ${status} is not pass"
  fi

  log "chaos traffic SLO gate: ${status}"
}

verify_soak_traffic_slo_gate() {
  local metadata="$1"
  local summary status

  summary="$(dirname "${metadata}")/traffic/summary.json"
  [[ -f "${summary}" ]] || aeg_fail "soak evidence is missing traffic SLO summary: ${summary}"

  status="$(python3 - "${summary}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
print(str((payload.get("slo_gate") or {}).get("status", "")).strip())
PY
)"

  [[ -n "${status}" ]] || aeg_fail "soak evidence is missing traffic SLO gate status in ${summary}"
  if [[ "${status}" != "pass" ]]; then
    aeg_fail "soak evidence traffic SLO gate ${status} is not pass"
  fi

  log "soak traffic SLO gate: ${status}"
}

verify_soak_duration() {
  local metadata="$1"
  local duration

  duration="$(extract_metadata_value "duration_seconds" "${metadata}")"
  [[ -n "${duration}" ]] || aeg_fail "soak evidence is missing duration_seconds; release candidates require a real 24h soak"

  if ! python3 -c '
import sys

try:
    actual = float(sys.argv[1])
except ValueError:
    raise SystemExit(1)
required = float(sys.argv[2])
if actual < required:
    raise SystemExit(1)
' "${duration}" "${required_soak_duration_seconds}"; then
    aeg_fail "soak evidence duration_seconds ${duration} is less than required ${required_soak_duration_seconds}"
  fi

  log "soak duration_seconds: ${duration}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --candidate)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --candidate"
      }
      candidate_commit="$2"
      shift 2
      ;;
    --conformance)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --conformance"
      }
      conformance_metadata="$2"
      shift 2
      ;;
    --performance)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --performance"
      }
      performance_metadata="$2"
      shift 2
      ;;
    --chaos)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --chaos"
      }
      chaos_metadata="$2"
      shift 2
      ;;
    --soak)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --soak"
      }
      soak_metadata="$2"
      shift 2
      ;;
    --allow-commit)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --allow-commit"
      }
      allowed_commits+=("$2")
      shift 2
      ;;
    --allow-dirty-conformance)
      allow_dirty_conformance="true"
      shift
      ;;
    --allow-dirty-performance-code-tree)
      allow_dirty_performance_code_tree="true"
      shift
      ;;
    --allow-dirty-chaos-code-tree)
      allow_dirty_chaos_code_tree="true"
      shift
      ;;
    --allow-dirty-soak-code-tree)
      allow_dirty_soak_code_tree="true"
      shift
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

[[ -n "${candidate_commit}" ]] || {
  usage >&2
  aeg_usage_error "missing required argument: --candidate"
}
[[ -n "${conformance_metadata}" ]] || {
  usage >&2
  aeg_usage_error "missing required argument: --conformance"
}
[[ -n "${performance_metadata}" ]] || {
  usage >&2
  aeg_usage_error "missing required argument: --performance"
}
[[ -n "${chaos_metadata}" ]] || {
  usage >&2
  aeg_usage_error "missing required argument: --chaos"
}
[[ -n "${soak_metadata}" ]] || {
  usage >&2
  aeg_usage_error "missing required argument: --soak"
}

aeg_require_file "${conformance_metadata}"
aeg_require_file "${performance_metadata}"
aeg_require_file "${chaos_metadata}"
aeg_require_file "${soak_metadata}"

allowed_refs=("${candidate_commit}")
if [[ ${#allowed_commits[@]} -gt 0 ]]; then
  allowed_refs+=("${allowed_commits[@]}")
fi

conformance_commit="$(extract_conformance_impl_version "${conformance_metadata}")"
performance_commit="$(extract_metadata_commit "${performance_metadata}")"
chaos_commit="$(extract_metadata_commit "${chaos_metadata}")"
soak_commit="$(extract_metadata_commit "${soak_metadata}")"
performance_code_tree_state="$(extract_metadata_value "code_tree_state" "${performance_metadata}")"
chaos_code_tree_state="$(extract_metadata_value "code_tree_state" "${chaos_metadata}")"
soak_code_tree_state="$(extract_metadata_value "code_tree_state" "${soak_metadata}")"

[[ -n "${conformance_commit}" ]] || aeg_fail "failed to read implementationVersion from ${conformance_metadata}"
[[ -n "${performance_commit}" ]] || aeg_fail "failed to read git_commit from ${performance_metadata}"
[[ -n "${chaos_commit}" ]] || aeg_fail "failed to read git_commit from ${chaos_metadata}"
[[ -n "${soak_commit}" ]] || aeg_fail "failed to read git_commit from ${soak_metadata}"

log "candidate commit: ${candidate_commit}"
if [[ ${#allowed_commits[@]} -gt 0 ]]; then
  log "allowed commits: ${allowed_refs[*]}"
fi

verify_conformance "${conformance_commit}" "${allowed_refs[@]}"
verify_benchmark_metadata "performance" "${performance_commit}" "${allowed_refs[@]}"
verify_code_tree_state "performance" "${performance_code_tree_state}" "${allow_dirty_performance_code_tree}"
verify_performance_coverage "${performance_metadata}"
verify_performance_source_slo_gate "${performance_metadata}"
verify_benchmark_metadata "chaos" "${chaos_commit}" "${allowed_refs[@]}"
verify_code_tree_state "chaos" "${chaos_code_tree_state}" "${allow_dirty_chaos_code_tree}"
verify_chaos_release_gate "${chaos_metadata}"
verify_chaos_traffic_slo_gate "${chaos_metadata}"
verify_benchmark_metadata "soak" "${soak_commit}" "${allowed_refs[@]}"
verify_code_tree_state "soak" "${soak_code_tree_state}" "${allow_dirty_soak_code_tree}"
verify_soak_duration "${soak_metadata}"
verify_soak_traffic_slo_gate "${soak_metadata}"

log "release evidence verified"
