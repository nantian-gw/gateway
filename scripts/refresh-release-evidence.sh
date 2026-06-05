#!/usr/bin/env bash
set -euo pipefail

candidate_commit=""
tool_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_root="${tool_root}"
check_only="false"
allow_commits=()
conformance_run=""
performance_run=""
chaos_run=""
soak_run=""

usage() {
  cat <<'EOF' >&2
usage: refresh-release-evidence.sh \
  --candidate <commit> \
  [--allow-commit <commit>]... \
  [--conformance-run <run-id-or-path>] \
  [--performance-run <run-id-or-path>] \
  [--chaos-run <run-id-or-path>] \
  [--soak-run <run-id-or-path>] \
  [--check-only]

Selects archived evidence runs for a release candidate, verifies the evidence
window, and refreshes summary docs. Auto-discovery only selects performance,
chaos, and soak evidence that records code_tree_state=clean. Performance
evidence also must include throughput-report.json with complete protocol,
scenario, reload live-traffic coverage, and source kind A4 slo-gate.json with
status=pass. Chaos evidence also must have release_gate_status=pass and traffic
slo_gate.status=pass, and soak evidence also must have duration_seconds>=86400
and traffic slo_gate.status=pass. In check-only mode it prints the selected
metadata files without mutating documents.
EOF
  exit 1
}

log() {
  printf '[refresh-release-evidence] %s\n' "$*"
}

fail() {
  printf '[refresh-release-evidence] %s\n' "$*" >&2
  exit 1
}

require_file() {
  local path="$1"
  local label="$2"

  [[ -f "${path}" ]] || fail "${label} file not found: ${path}"
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

extract_conformance_impl_version() {
  extract_conformance_report_scalar "implementationVersion" "$1"
}

extract_conformance_report_scalar() {
  local key="$1"
  local file="$2"

  awk -v wanted="${key}" '
    $1 == "report:" {
      in_report = 1
      next
    }
    in_report && $1 == wanted ":" {
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

extract_benchmark_run_id() {
  local file="$1"
  local run_id

  run_id="$(extract_metadata_value "run_id" "${file}")"
  if [[ -n "${run_id}" ]]; then
    printf '%s\n' "${run_id}"
    return 0
  fi

  basename "$(dirname "${file}")"
}

normalize_commit() {
  printf '%s' "${1%-dirty}"
}

commit_is_allowed() {
  local actual
  actual="$(normalize_commit "$1")"
  shift

  local expected
  for expected in "$@"; do
    expected="$(normalize_commit "${expected}")"
    if [[ -z "${expected}" ]]; then
      continue
    fi
    if [[ "${actual}" == "${expected}" \
      || "${actual}" == "${expected}"* \
      || "${expected}" == "${actual}"* ]]; then
      return 0
    fi
  done

  return 1
}

resolve_conformance_input() {
  local input="$1"

  if [[ -f "${input}" ]]; then
    printf '%s\n' "${input}"
    return 0
  fi

  if [[ -d "${input}" && -f "${input}/metadata.yaml" ]]; then
    printf '%s\n' "${input}/metadata.yaml"
    return 0
  fi

  if [[ -f "${repo_root}/${input}" ]]; then
    printf '%s\n' "${repo_root}/${input}"
    return 0
  fi

  if [[ -d "${repo_root}/${input}" && -f "${repo_root}/${input}/metadata.yaml" ]]; then
    printf '%s\n' "${repo_root}/${input}/metadata.yaml"
    return 0
  fi

  if [[ -f "${repo_root}/reports/conformance/runs/${input}/metadata.yaml" ]]; then
    printf '%s\n' "${repo_root}/reports/conformance/runs/${input}/metadata.yaml"
    return 0
  fi

  fail "unable to resolve conformance run input: ${input}"
}

resolve_benchmark_input() {
  local input="$1"
  local root_dir="$2"

  if [[ -f "${input}" ]]; then
    printf '%s\n' "${input}"
    return 0
  fi

  if [[ -d "${input}" && -f "${input}/metadata.txt" ]]; then
    printf '%s\n' "${input}/metadata.txt"
    return 0
  fi

  if [[ -f "${repo_root}/${input}" ]]; then
    printf '%s\n' "${repo_root}/${input}"
    return 0
  fi

  if [[ -d "${repo_root}/${input}" && -f "${repo_root}/${input}/metadata.txt" ]]; then
    printf '%s\n' "${repo_root}/${input}/metadata.txt"
    return 0
  fi

  if [[ -f "${repo_root}/${root_dir}/${input}/metadata.txt" ]]; then
    printf '%s\n' "${repo_root}/${root_dir}/${input}/metadata.txt"
    return 0
  fi

  fail "unable to resolve benchmark run input: ${input}"
}

discover_conformance_metadata() {
  local runs_root="$1"
  local candidate=""
  local latest=""

  while IFS= read -r candidate; do
    local result run_id implementation_version
    result="$(extract_scalar "result" "${candidate}")"
    run_id="$(extract_scalar "id" "${candidate}")"
    implementation_version="$(extract_conformance_impl_version "${candidate}")"

    if [[ "${result}" == "passed" \
      && -n "${run_id}" \
      && "${run_id}" == *-full \
      && -n "${implementation_version}" \
      && "${implementation_version}" != *-dirty ]] \
      && commit_is_allowed "${implementation_version}" "${candidate_commit}" "${allow_commits[@]}"; then
      latest="${candidate}"
    fi
  done < <(find "${runs_root}" -mindepth 2 -maxdepth 2 -path '*/metadata.yaml' | sort)

  [[ -n "${latest}" ]] || fail "no matching clean full-suite conformance evidence found"
  printf '%s\n' "${latest}"
}

discover_benchmark_metadata() {
  local runs_root="$1"
  local qualifier="$2"
  shift 2
  local patterns=("$@")
  local candidate=""
  local latest=""

  if [[ ${#patterns[@]} -eq 0 ]]; then
    patterns=("*")
  fi

  while IFS= read -r candidate; do
    local git_commit
    git_commit="$(extract_metadata_commit "${candidate}")"

    if [[ -n "${git_commit}" ]] \
      && commit_is_allowed "${git_commit}" "${candidate_commit}" "${allow_commits[@]}" \
      && benchmark_metadata_is_qualified "${candidate}" "${qualifier}"; then
      latest="${candidate}"
    fi
  done < <(
    for pattern in "${patterns[@]}"; do
      find "${runs_root}" -mindepth 2 -maxdepth 2 -path "*/${pattern}/metadata.txt"
    done | sort -u
  )

  [[ -n "${latest}" ]] || fail "no matching ${qualifier} evidence found under ${runs_root}/${patterns[*]}"
  printf '%s\n' "${latest}"
}

performance_coverage_is_complete() {
  local report="$1"

  [[ -f "${report}" ]] || return 1
  python3 - "${report}" <<'PY'
import json
import sys
from pathlib import Path

try:
    payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
except Exception:
    raise SystemExit(1)

checks = [
    ("coverage", "missing_protocols"),
    ("coverage", "missing_scenarios"),
    ("reload", "live_traffic", "missing_protocols"),
    ("reload", "live_traffic", "missing_mutations"),
]

for keys in checks:
    current = payload
    for key in keys:
        if not isinstance(current, dict) or key not in current:
            raise SystemExit(1)
        current = current[key]
    if not isinstance(current, list) or current:
        raise SystemExit(1)
raise SystemExit(0)
PY
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

performance_source_slo_gate_is_pass() {
  local metadata="$1"
  local summary

  summary="$(performance_source_slo_gate_path "${metadata}")" || return 1
  python3 - "${summary}" <<'PY'
import json
import sys
from pathlib import Path

try:
    payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
except Exception:
    raise SystemExit(1)

if payload.get("status") != "pass":
    raise SystemExit(1)
profiles = payload.get("profiles", {})
if not isinstance(profiles, dict):
    raise SystemExit(1)
for profile in profiles.values():
    if not isinstance(profile, dict) or profile.get("status") != "pass":
        raise SystemExit(1)
raise SystemExit(0)
PY
}

benchmark_metadata_is_qualified() {
  local metadata="$1"
  local qualifier="$2"

  case "${qualifier}" in
    any)
      return 0
      ;;
    clean-code-tree)
      local code_tree_state
      code_tree_state="$(extract_metadata_value "code_tree_state" "${metadata}")"
      [[ "${code_tree_state}" == "clean" ]]
      ;;
    performance-release-coverage)
      local code_tree_state report
      code_tree_state="$(extract_metadata_value "code_tree_state" "${metadata}")"
      [[ "${code_tree_state}" == "clean" ]] || return 1
      report="$(dirname "${metadata}")/throughput-report.json"
      performance_coverage_is_complete "${report}" || return 1
      performance_source_slo_gate_is_pass "${metadata}"
      ;;
    chaos-release-gate)
      local code_tree_state summary traffic_summary
      code_tree_state="$(extract_metadata_value "code_tree_state" "${metadata}")"
      [[ "${code_tree_state}" == "clean" ]] || return 1
      summary="$(dirname "${metadata}")/conclusions/summary.json"
      [[ -f "${summary}" ]] || return 1
      python3 - "${summary}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
raise SystemExit(0 if payload.get("release_gate_status") == "pass" else 1)
PY
      traffic_summary="$(dirname "${metadata}")/traffic/summary.json"
      [[ -f "${traffic_summary}" ]] || return 1
      python3 - "${traffic_summary}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
raise SystemExit(0 if (payload.get("slo_gate") or {}).get("status") == "pass" else 1)
PY
      ;;
    soak-24h)
      local code_tree_state duration traffic_summary
      code_tree_state="$(extract_metadata_value "code_tree_state" "${metadata}")"
      [[ "${code_tree_state}" == "clean" ]] || return 1
      duration="$(extract_metadata_value "duration_seconds" "${metadata}")"
      [[ -n "${duration}" ]] || return 1
      python3 - "${duration}" <<'PY'
import sys

try:
    duration = float(sys.argv[1])
except ValueError:
    raise SystemExit(1)
raise SystemExit(0 if duration >= 86400 else 1)
PY
      traffic_summary="$(dirname "${metadata}")/traffic/summary.json"
      [[ -f "${traffic_summary}" ]] || return 1
      python3 - "${traffic_summary}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
raise SystemExit(0 if (payload.get("slo_gate") or {}).get("status") == "pass" else 1)
PY
      ;;
    *)
      fail "unknown benchmark evidence qualifier: ${qualifier}"
      ;;
  esac
}

extract_table_field() {
  local file="$1"
  local label="$2"
  local column="$3"

  awk -F'|' -v wanted="${label}" -v col="${column}" '
    function trim(value) {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      return value
    }
    trim($2) == wanted {
      print trim($(col))
      exit
    }
  ' "${file}"
}

extract_json_field() {
  local file="$1"
  local field="$2"

  python3 - "$file" "$field" <<'PY'
from pathlib import Path
import json
import re
import sys

path = Path(sys.argv[1])
field = sys.argv[2]
text = path.read_text(encoding="utf-8")
match = re.search(r"```json\n(.*?)\n```", text, re.S)
if not match:
    raise SystemExit(f"missing json block in {path}")
payload = json.loads(match.group(1))
value = payload[field]
if isinstance(value, float):
    print(value)
else:
    print(value)
PY
}

format_percent() {
  python3 - "$1" <<'PY'
import sys

value = float(sys.argv[1]) * 100
print(f"{value:.2f}")
PY
}

format_float() {
  python3 - "$1" <<'PY'
import sys

value = float(sys.argv[1])
print(f"{value:.2f}")
PY
}

short_commit() {
  printf '%s' "$1" | cut -c1-7
}

extract_run_date() {
  printf '%s' "$1" | cut -d- -f1-3
}

ensure_parent_dir() {
  mkdir -p "$(dirname "$1")"
}

replace_marker_block() {
  local path="$1"
  local marker="$2"
  local replacement_file="$3"

  require_file "${path}" "document"
  require_file "${replacement_file}" "replacement"

  python3 - "$path" "$marker" "$replacement_file" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
marker = sys.argv[2]
replacement_path = Path(sys.argv[3])
start = f"<!-- release-evidence:{marker}:start -->"
end = f"<!-- release-evidence:{marker}:end -->"
text = path.read_text(encoding="utf-8")
if start not in text or end not in text:
    raise SystemExit(f"missing marker block {marker} in {path}")
before, rest = text.split(start, 1)
_, after = rest.split(end, 1)
replacement = replacement_path.read_text(encoding="utf-8").rstrip("\n")
path.write_text(before + start + "\n" + replacement + "\n" + end + after, encoding="utf-8")
PY
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --candidate)
      [[ $# -ge 2 ]] || usage
      candidate_commit="$2"
      shift 2
      ;;
    --repo-root)
      [[ $# -ge 2 ]] || usage
      repo_root="$2"
      shift 2
      ;;
    --check-only)
      check_only="true"
      shift
      ;;
    --allow-commit)
      [[ $# -ge 2 ]] || usage
      allow_commits+=("$2")
      shift 2
      ;;
    --conformance-run)
      [[ $# -ge 2 ]] || usage
      conformance_run="$2"
      shift 2
      ;;
    --performance-run)
      [[ $# -ge 2 ]] || usage
      performance_run="$2"
      shift 2
      ;;
    --chaos-run)
      [[ $# -ge 2 ]] || usage
      chaos_run="$2"
      shift 2
      ;;
    --soak-run)
      [[ $# -ge 2 ]] || usage
      soak_run="$2"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[[ -n "${candidate_commit}" ]] || usage

conformance_metadata="${conformance_run:+$(resolve_conformance_input "${conformance_run}")}"
performance_metadata="${performance_run:+$(resolve_benchmark_input "${performance_run}" "reports/performance/runs")}"
chaos_metadata="${chaos_run:+$(resolve_benchmark_input "${chaos_run}" "reports/chaos/runs")}"
soak_metadata="${soak_run:+$(resolve_benchmark_input "${soak_run}" "reports/soak/runs")}"

if [[ -z "${conformance_metadata}" ]]; then
  conformance_metadata="$(discover_conformance_metadata "${repo_root}/reports/conformance/runs")"
fi
if [[ -z "${performance_metadata}" ]]; then
  performance_metadata="$(discover_benchmark_metadata \
    "${repo_root}/reports/performance/runs" \
    "performance-release-coverage" \
    "*kind-a4*" \
    "*a4-profiles" \
    "*a4-scenarios")"
fi
if [[ -z "${chaos_metadata}" ]]; then
  chaos_metadata="$(discover_benchmark_metadata "${repo_root}/reports/chaos/runs" "chaos-release-gate" "*kind-faults")"
fi
if [[ -z "${soak_metadata}" ]]; then
  soak_metadata="$(discover_benchmark_metadata "${repo_root}/reports/soak/runs" "soak-24h" "*kind-soak*")"
fi

require_file "${conformance_metadata}" "conformance metadata"
require_file "${performance_metadata}" "performance metadata"
require_file "${chaos_metadata}" "chaos metadata"
require_file "${soak_metadata}" "soak metadata"

allow_commit_args=()
for candidate in "${allow_commits[@]}"; do
  allow_commit_args+=(--allow-commit "${candidate}")
done

"${tool_root}/scripts/verify-release-evidence.sh" \
  --candidate "${candidate_commit}" \
  "${allow_commit_args[@]}" \
  --conformance "${conformance_metadata}" \
  --performance "${performance_metadata}" \
  --chaos "${chaos_metadata}" \
  --soak "${soak_metadata}"

if [[ "${check_only}" == "true" ]]; then
  printf 'selected conformance metadata: %s\n' "${conformance_metadata}"
  printf 'selected performance metadata: %s\n' "${performance_metadata}"
  printf 'selected chaos metadata: %s\n' "${chaos_metadata}"
  printf 'selected soak metadata: %s\n' "${soak_metadata}"
  exit 0
fi

latest_conformance_metadata="${repo_root}/reports/conformance/latest/metadata.yaml"
require_file "${latest_conformance_metadata}" "latest conformance metadata"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

latest_id="$(extract_scalar "id" "${latest_conformance_metadata}")"
latest_commit="$(extract_conformance_impl_version "${latest_conformance_metadata}")"
latest_date="$(extract_run_date "${latest_id}")"
clean_id="$(extract_scalar "id" "${conformance_metadata}")"
clean_commit="$(extract_conformance_impl_version "${conformance_metadata}")"
clean_gateway_api_version="$(extract_conformance_report_scalar "gatewayAPIVersion" "${conformance_metadata}")"
clean_date="$(extract_run_date "${clean_id}")"

performance_run_id="$(extract_benchmark_run_id "${performance_metadata}")"
performance_commit_full="$(extract_metadata_commit "${performance_metadata}")"
performance_commit="$(short_commit "${performance_commit_full}")"
performance_date="$(extract_run_date "${performance_run_id}")"
performance_summary="${performance_metadata%metadata.txt}summary.md"
require_file "${performance_summary}" "performance summary"

steady_p99="$(extract_table_field "${performance_summary}" "steady" 7)"
burst_p99="$(extract_table_field "${performance_summary}" "burst" 7)"
ceiling_p99="$(extract_table_field "${performance_summary}" "ceiling" 7)"
grpc_p99="$(extract_table_field "${performance_summary}" "unary" 7)"

chaos_run_id="$(extract_benchmark_run_id "${chaos_metadata}")"
chaos_commit_full="$(extract_metadata_commit "${chaos_metadata}")"
chaos_commit="$(short_commit "${chaos_commit_full}")"
chaos_date="$(extract_run_date "${chaos_run_id}")"
chaos_summary="${chaos_metadata%metadata.txt}summary.md"
require_file "${chaos_summary}" "chaos summary"
chaos_completed="$(extract_json_field "${chaos_summary}" "completed")"
chaos_successes="$(extract_json_field "${chaos_summary}" "successes")"
chaos_success_rate="$(format_percent "$(extract_json_field "${chaos_summary}" "mean_success_rate")")"
chaos_p99="$(format_float "$(extract_json_field "${chaos_summary}" "max_p99_ms")")"

soak_run_id="$(extract_benchmark_run_id "${soak_metadata}")"
soak_commit_full="$(extract_metadata_commit "${soak_metadata}")"
soak_commit="$(short_commit "${soak_commit_full}")"
soak_date="$(extract_run_date "${soak_run_id}")"
soak_summary="${soak_metadata%metadata.txt}summary.md"
require_file "${soak_summary}" "soak summary"
soak_completed="$(extract_json_field "${soak_summary}" "completed")"
soak_successes="$(extract_json_field "${soak_summary}" "successes")"
soak_success_rate="$(format_percent "$(extract_json_field "${soak_summary}" "mean_success_rate")")"
soak_p99="$(format_float "$(extract_json_field "${soak_summary}" "max_p99_ms")")"
soak_type="kind soak sample"
if [[ "${soak_run_id}" == *10m-pilot* ]]; then
  soak_type="10m soak pilot"
fi

cat >"${tmp_dir}/baseline-current.md" <<EOF
- Most recent complete \`A1\` release-grade single-command automation sample: \`2026-03-27\`
- Most recent archived full-suite conformance: \`${latest_date}\`, commit \`${latest_commit}\`
- Most recent clean-commit full-suite conformance baseline: \`${clean_date}\`, commit \`${clean_commit}\`
- Most recent \`A4\` kind performance / chaos / soak evidence: \`${performance_date}\`, commit \`${performance_commit}\`
- Current document state: the above archive locations and \`latest\` pointers are uniformly aligned; however, a "full \`A1 + A4 + soak\` unified refresh on the same candidate commit" has not yet been formed
EOF

cat >"${tmp_dir}/baseline-refreshes.md" <<EOF
- full-suite conformance \`latest/\` has been refreshed to \`${latest_id}/\`, corresponding to [report.yaml](../../reports/conformance/latest/report.yaml), [metadata.yaml](../../reports/conformance/latest/metadata.yaml), and [run.log](../../reports/conformance/latest/run.log)
- Most recent clean-commit full-suite conformance baseline is \`${clean_id}/\`
- Most recent \`A4\` performance baseline, fault injection, and soak pilot have been refreshed to \`reports/performance/runs/${performance_run_id}/\`, \`reports/chaos/runs/${chaos_run_id}/\`, and \`reports/soak/runs/${soak_run_id}/\` respectively
- There are currently no additional \`A2\` scripts; mesh frontend has been merged into \`A1\`
EOF

cat >"${tmp_dir}/baseline-artifacts.md" <<EOF
- Latest full-suite conformance report: \`reports/conformance/latest/report.yaml\`
- Latest full-suite conformance metadata: \`reports/conformance/latest/metadata.yaml\`
- Latest full-suite conformance log: \`reports/conformance/latest/run.log\`
- Most recent clean-commit full-suite baseline: \`reports/conformance/runs/${clean_id}/\`

### 4.1 Most Recent A4 Kind Evidence

The following two sets of kind evidence were additionally run and archived for long-term retention:

- Performance and capacity baseline: \`reports/performance/runs/${performance_run_id}/\`
- Fault injection: \`reports/chaos/runs/${chaos_run_id}/\`

Key results this round:

- HTTP concurrency baseline:
- steady \`p99=${steady_p99}ms\`
- burst \`p99=${burst_p99}ms\`
- ceiling \`p99=${ceiling_p99}ms\`
- gRPC unary baseline: \`p99=${grpc_p99}ms\`
- upstream keepalive, retry, failover, weighted distribution, weight convergence, backend recovery: raw logs retained
- backend protocol selection plain/h2c/ws success and failure paths: raw logs retained
- During fault injection, continuous HTTP traffic completed \`${chaos_completed}\` requests, of which \`${chaos_successes}\` succeeded, average success rate approximately \`${chaos_success_rate}%\`, batch maximum \`p99≈${chaos_p99}ms\`
- \`scripts/run-kind-soak.sh\` has validated a short-duration pilot sample; most recent sample at \`reports/soak/runs/${soak_run_id}/\`; sample type is \`${soak_type}\`, completed \`${soak_completed}\` requests, of which \`${soak_successes}\` succeeded, average success rate approximately \`${soak_success_rate}%\`, batch maximum \`p99≈${soak_p99}ms\`. This sample is not a \`24h\` full conclusion, it is only used to anchor the soak automation entry point
EOF

cat >"${tmp_dir}/performance-kind-a4.md" <<EOF
- \`runs/${performance_run_id}/\`
  - generated at: \`$(extract_metadata_value "captured_at" "${performance_metadata}")\`
  - code commit: \`${performance_commit}\`
  - type: kind A4 baseline
  - result: pass
  - key findings: HTTP \`steady\` / \`burst\` / \`ceiling\` \`p99\` are \`${steady_p99}ms\` / \`${burst_p99}ms\` / \`${ceiling_p99}ms\` respectively, gRPC unary \`p99=${grpc_p99}ms\`; see \`summary.md\` in same directory
EOF

cat >"${tmp_dir}/chaos-summary.md" <<EOF
- \`runs/${chaos_run_id}/\`
  - generated at: \`$(extract_metadata_value "captured_at" "${chaos_metadata}")\`
  - code commit: \`${chaos_commit}\`
  - result: pass
  - fault sequence: controlplane leader pod deleted, leader re-election completed, then dataplane pod deleted, deployment recovery completed
  - continuous traffic summary: \`${chaos_completed}\` requests, \`${chaos_successes}\` succeeded, average success rate approximately \`${chaos_success_rate}%\`, max \`p99\` latency approximately \`${chaos_p99}ms\`
EOF

cat >"${tmp_dir}/soak-summary.md" <<EOF
- \`runs/${soak_run_id}/\`
  - generated at: \`$(extract_metadata_value "captured_at" "${soak_metadata}")\`
  - code commit: \`${soak_commit}\`
  - type: \`${soak_type}\`
  - result: pass
  - traffic summary: \`${soak_completed}\` requests, \`${soak_successes}\` succeeded, average success rate \`${soak_success_rate}%\`, max \`p99\` latency approximately \`${soak_p99}ms\`
  - note: this report is used for the soak summary refresh of the current candidate commit; it does not represent a completed \`24h\` baseline
EOF

cat >"${tmp_dir}/conformance-clean.md" <<EOF
- \`runs/${clean_id}/\`
  - generated at: \`$(extract_conformance_report_scalar "reportDate" "${conformance_metadata}")\`
  - code commit: \`${clean_commit}\`
  - Gateway API version: \`$(extract_conformance_report_scalar "gatewayAPIVersion" "${conformance_metadata}")\`
  - run mode: \`ALL_FEATURES=true ./tests/conformance/run.sh\`
  - result: pass
  - attached run log: \`run.log\`
  - note: if you need to reference the "most recent clean-commit full-suite" rather than \`latest/\`, prefer this archive
EOF

cat >"${tmp_dir}/conformance-clean-support.md" <<EOF
The most recent clean-commit full-suite baseline is \`${clean_date}\` with \`${clean_commit}\`. If a newer commit is to be used as the external reference baseline, a full-suite report should still be re-archived for that commit. Corresponding results are in [reports/conformance](../reports/conformance/README.md).
EOF

cat >"${tmp_dir}/conformance-clean-results.md" <<EOF
- Most recent clean-commit full-suite baseline:
  - [report.yaml](../reports/conformance/runs/${clean_id}/report.yaml)
  - [metadata.yaml](../reports/conformance/runs/${clean_id}/metadata.yaml)
  - [run.log](../reports/conformance/runs/${clean_id}/run.log)
EOF

cat >"${tmp_dir}/conformance-clean-community.md" <<EOF
- This project already has the technical foundation to advance as a Gateway API implementation, and has archived a full-suite conformance report based on Gateway API \`${clean_gateway_api_version}\`; the most recent archived result is \`${latest_date}\` with \`${latest_commit}\`, and the most recent clean-commit full-suite baseline is \`${clean_date}\` with \`${clean_commit}\`.
EOF

cat >"${tmp_dir}/changelog-summary.md" <<EOF
- release summary currently references archived evidence:
  - latest archived full-suite: \`runs/${latest_id}/\`, date \`${latest_date}\`, commit \`${latest_commit}\`
  - latest clean full-suite baseline: \`runs/${clean_id}/\`, date \`${clean_date}\`, commit \`${clean_commit}\`
  - performance: \`runs/${performance_run_id}/\`
  - chaos: \`runs/${chaos_run_id}/\`
  - soak: \`runs/${soak_run_id}/\`
- For detailed summaries, read \`docs/test/latest-baseline.md\` and the respective evidence READMEs first.
EOF

replace_marker_block "${repo_root}/docs/test/latest-baseline.md" "baseline-current" "${tmp_dir}/baseline-current.md"
replace_marker_block "${repo_root}/docs/test/latest-baseline.md" "baseline-refreshes" "${tmp_dir}/baseline-refreshes.md"
replace_marker_block "${repo_root}/docs/test/latest-baseline.md" "baseline-artifacts" "${tmp_dir}/baseline-artifacts.md"
replace_marker_block "${repo_root}/reports/performance/README.md" "performance-kind-a4" "${tmp_dir}/performance-kind-a4.md"
replace_marker_block "${repo_root}/reports/chaos/README.md" "chaos-summary" "${tmp_dir}/chaos-summary.md"
replace_marker_block "${repo_root}/reports/soak/README.md" "soak-summary" "${tmp_dir}/soak-summary.md"
replace_marker_block "${repo_root}/reports/conformance/README.md" "conformance-clean" "${tmp_dir}/conformance-clean.md"
replace_marker_block "${repo_root}/docs/gateway-api-support.md" "conformance-clean-support" "${tmp_dir}/conformance-clean-support.md"
replace_marker_block "${repo_root}/docs/gateway-api-support.md" "conformance-clean-results" "${tmp_dir}/conformance-clean-results.md"
replace_marker_block "${repo_root}/docs/community-readiness.md" "conformance-clean-community" "${tmp_dir}/conformance-clean-community.md"
replace_marker_block "${repo_root}/CHANGELOG.md" "changelog-summary" "${tmp_dir}/changelog-summary.md"

"${tool_root}/scripts/check-evidence-reference-alignment.sh" --repo-root "${repo_root}"

log "release evidence references refreshed"
