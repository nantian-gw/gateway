#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=scripts/lib/conformance-report.sh
source "${ROOT_DIR}/scripts/lib/conformance-report.sh"

REPORTS_ROOT="${REPORTS_ROOT:-${ROOT_DIR}/reports/conformance}"
REPORT_SCOPE="${REPORT_SCOPE:-runs}"
RESULT_STATUS="${RESULT_STATUS:-passed}"
RELEASE_TAG="${RELEASE_TAG:-}"
SOURCE_COMMAND="${SOURCE_COMMAND:-}"
SOURCE_RUN_URL="${SOURCE_RUN_URL:-}"
SOURCE_REF="${SOURCE_REF:-${GITHUB_REF_NAME:-}}"
ARCHIVED_AT="${ARCHIVED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

usage() {
  cat <<'EOF' >&2
usage: archive-conformance-report.sh <report-id> <report-path> [run-log-path]

Environment variables:
  REPORTS_ROOT    Repository path that stores archived reports.
  REPORT_SCOPE    Logical grouping under reports/conformance, for example runs or releases.
  RESULT_STATUS   passed or failed. Defaults to passed.
  RELEASE_TAG     Optional release tag associated with this report.
  SOURCE_COMMAND  Optional command line used to generate the report.
  SOURCE_RUN_URL  Optional CI run URL.
  SOURCE_REF      Optional git ref name.
EOF
  exit 1
}

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage
fi

REPORT_ID_RAW="$1"
REPORT_SOURCE="$2"
LOG_SOURCE="${3:-}"

if [[ ! -f "${REPORT_SOURCE}" ]]; then
  printf 'report file not found: %s\n' "${REPORT_SOURCE}" >&2
  exit 1
fi

if [[ -n "${LOG_SOURCE}" && ! -f "${LOG_SOURCE}" ]]; then
  printf 'run log file not found: %s\n' "${LOG_SOURCE}" >&2
  exit 1
fi

if [[ -z "${SOURCE_RUN_URL}" && -n "${GITHUB_SERVER_URL:-}" && -n "${GITHUB_REPOSITORY:-}" && -n "${GITHUB_RUN_ID:-}" ]]; then
  SOURCE_RUN_URL="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}"
fi

sanitize_id() {
  printf '%s' "$1" | tr -cs 'A-Za-z0-9._-' '-'
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
    $1 == "implementation:" {
      in_impl = 1
      next
    }
    in_impl && $1 == "version:" {
      value = $2
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
    in_impl && /^[^[:space:]]/ {
      in_impl = 0
    }
  ' "${file}"
}

copy_payload() {
  local target_dir="$1"

  mkdir -p "${target_dir}"
  cp "${REPORT_SOURCE}" "${target_dir}/report.yaml"
  if [[ -n "${LOG_SOURCE}" ]]; then
    cp "${LOG_SOURCE}" "${target_dir}/run.log"
  else
    rm -f "${target_dir}/run.log"
  fi
}

render_log_summary() {
  local target_dir="$1"

  if [[ -f "${target_dir}/run.log" ]]; then
    conformance_summarize_log \
      "${target_dir}/run.log" \
      "${target_dir}/log-summary.json" \
      "${target_dir}/summary.md"
  else
    rm -f "${target_dir}/log-summary.json" "${target_dir}/summary.md"
  fi
}

json_scalar() {
  local file="$1"
  local key="$2"

  python3 - "${file}" "${key}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
value = payload.get(sys.argv[2], "")
print(value)
PY
}

render_metadata() {
  local target_dir="$1"
  local report_date gateway_api_channel gateway_api_version implementation_version mode log_file
  local release_tag source_ref source_run_url
  local log_summary_file summary_file final_status stability_signal transient_retry_count expected_negative_path_error_count final_fail_count

  report_date="$(extract_scalar "date" "${REPORT_SOURCE}")"
  gateway_api_channel="$(extract_scalar "gatewayAPIChannel" "${REPORT_SOURCE}")"
  gateway_api_version="$(extract_scalar "gatewayAPIVersion" "${REPORT_SOURCE}")"
  implementation_version="$(extract_implementation_version "${REPORT_SOURCE}")"
  mode="$(extract_scalar "mode" "${REPORT_SOURCE}")"
  log_file=""
  log_summary_file=""
  summary_file=""
  final_status=""
  stability_signal=""
  transient_retry_count=0
  expected_negative_path_error_count=0
  final_fail_count=0
  release_tag="${RELEASE_TAG:-}"
  source_ref="${SOURCE_REF:-}"
  source_run_url="${SOURCE_RUN_URL:-}"

  if [[ -n "${LOG_SOURCE}" ]]; then
    log_file="run.log"
  fi
  if [[ -f "${target_dir}/log-summary.json" ]]; then
    log_summary_file="log-summary.json"
    summary_file="summary.md"
    final_status="$(json_scalar "${target_dir}/log-summary.json" "final_status")"
    stability_signal="$(json_scalar "${target_dir}/log-summary.json" "stability_signal")"
    transient_retry_count="$(json_scalar "${target_dir}/log-summary.json" "transient_retry_count")"
    expected_negative_path_error_count="$(json_scalar "${target_dir}/log-summary.json" "expected_negative_path_error_count")"
    final_fail_count="$(json_scalar "${target_dir}/log-summary.json" "final_fail_count")"
  fi

  cat >"${target_dir}/metadata.yaml" <<EOF
id: ${REPORT_ID}
scope: ${REPORT_SCOPE}
result: ${RESULT_STATUS}
archivedAt: ${ARCHIVED_AT}
releaseTag: "${release_tag}"
sourceRef: "${source_ref}"
sourceRunURL: "${source_run_url}"
sourceCommand: |
  ${SOURCE_COMMAND:-n/a}
report:
  file: report.yaml
  logFile: "${log_file}"
  reportDate: "${report_date}"
  gatewayAPIChannel: "${gateway_api_channel}"
  gatewayAPIVersion: "${gateway_api_version}"
  implementationVersion: "${implementation_version}"
  mode: "${mode}"
logSummary:
  file: "${log_summary_file}"
  summaryFile: "${summary_file}"
  finalStatus: "${final_status}"
  stabilitySignal: "${stability_signal}"
  transientRetryCount: ${transient_retry_count}
  expectedNegativePathErrorCount: ${expected_negative_path_error_count}
  finalFailCount: ${final_fail_count}
EOF
}

REPORT_ID="$(sanitize_id "${REPORT_ID_RAW}")"
DEST_DIR="${REPORTS_ROOT}/${REPORT_SCOPE}/${REPORT_ID}"
LATEST_DIR="${REPORTS_ROOT}/latest"

mkdir -p "${REPORTS_ROOT}/${REPORT_SCOPE}"
copy_payload "${DEST_DIR}"
render_log_summary "${DEST_DIR}"
render_metadata "${DEST_DIR}"
copy_payload "${LATEST_DIR}"
render_log_summary "${LATEST_DIR}"
render_metadata "${LATEST_DIR}"

printf '%s\n' "${DEST_DIR}"
