#!/usr/bin/env bash

conformance_report_log() {
  if declare -F log >/dev/null 2>&1; then
    log "$*"
    return
  fi

  printf '[conformance] %s\n' "$*" >&2
}

conformance_resolve_path() {
  local root_dir="$1"
  local path="$2"

  if [[ "${path}" == /* ]]; then
    printf '%s\n' "${path}"
    return
  fi

  printf '%s/%s\n' "${root_dir}" "${path}"
}

conformance_prepare_report_artifacts() {
  local root_dir="$1"
  local report_dir_input="$2"
  local report_output_input="$3"
  local log_path_input="${4:-}"
  local metadata_path_input="${5:-}"

  REPORT_DIR="$(conformance_resolve_path "${root_dir}" "${report_dir_input}")"
  REPORT_OUTPUT="$(conformance_resolve_path "${root_dir}" "${report_output_input}")"
  REPORT_OUTPUT_DIR="$(dirname "${REPORT_OUTPUT}")"

  if [[ -z "${log_path_input}" ]]; then
    log_path_input="${REPORT_OUTPUT%.*}.log"
  fi
  CONFORMANCE_LOG_PATH="$(conformance_resolve_path "${root_dir}" "${log_path_input}")"

  if [[ -z "${metadata_path_input}" ]]; then
    metadata_path_input="${REPORT_OUTPUT%.*}.metadata.txt"
  fi
  REPORT_METADATA_PATH="$(conformance_resolve_path "${root_dir}" "${metadata_path_input}")"

  mkdir -p "${REPORT_DIR}" "${REPORT_OUTPUT_DIR}" \
    "$(dirname "${CONFORMANCE_LOG_PATH}")" \
    "$(dirname "${REPORT_METADATA_PATH}")"

  if [[ -d "${REPORT_OUTPUT}" ]]; then
    conformance_report_log "REPORT_OUTPUT points to a directory: ${REPORT_OUTPUT}"
    return 1
  fi

  if [[ ! -d "${REPORT_OUTPUT_DIR}" || ! -w "${REPORT_OUTPUT_DIR}" ]]; then
    conformance_report_log "report output directory is not writable: ${REPORT_OUTPUT_DIR}"
    return 1
  fi

  if [[ ! -d "${REPORT_DIR}" || ! -w "${REPORT_DIR}" ]]; then
    conformance_report_log "report directory is not writable: ${REPORT_DIR}"
    return 1
  fi

  if [[ ! -d "$(dirname "${CONFORMANCE_LOG_PATH}")" || ! -w "$(dirname "${CONFORMANCE_LOG_PATH}")" ]]; then
    conformance_report_log "conformance log directory is not writable: $(dirname "${CONFORMANCE_LOG_PATH}")"
    return 1
  fi

  if [[ ! -d "$(dirname "${REPORT_METADATA_PATH}")" || ! -w "$(dirname "${REPORT_METADATA_PATH}")" ]]; then
    conformance_report_log "report metadata directory is not writable: $(dirname "${REPORT_METADATA_PATH}")"
    return 1
  fi
}

conformance_write_metadata() {
  local status="$1"
  local note="${2:-}"

  cat >"${REPORT_METADATA_PATH}" <<EOF
status=${status}
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
root_dir=${ROOT_DIR:-}
report_dir=${REPORT_DIR:-}
report_output=${REPORT_OUTPUT:-}
report_output_dir=${REPORT_OUTPUT_DIR:-}
log_path=${CONFORMANCE_LOG_PATH:-}
gateway_api_version=${GATEWAY_API_VERSION:-}
implementation_version=${IMPLEMENTATION_VERSION:-}
note=${note}
EOF
}

conformance_summarize_log() {
  local log_path="$1"
  local json_output="$2"
  local markdown_output="$3"

  python3 - "${log_path}" "${json_output}" "${markdown_output}" <<'PY'
import json
import os
import re
import sys
from pathlib import Path

log_path = Path(sys.argv[1])
json_output = Path(sys.argv[2])
markdown_output = Path(sys.argv[3])

signals = {
    "not_ready_yet": re.compile(r"\bnot ready yet\b", re.I),
    "response_expectation_failed": re.compile(r"Response expectation failed", re.I),
    "io_timeout": re.compile(r"\bi/o timeout\b", re.I),
    "context_deadline_exceeded": re.compile(r"context deadline exceeded", re.I),
    "timed_out_waiting": re.compile(r"timed out waiting", re.I),
    "rpc_finished_with_error": re.compile(r"RPC finished with error", re.I),
    "udp_query_error": re.compile(r"failed to perform a UDP query", re.I),
}
transient_signal_names = {
    "response_expectation_failed",
    "io_timeout",
    "context_deadline_exceeded",
    "timed_out_waiting",
}
readiness_wait_signal_names = {
    "not_ready_yet",
}
expected_negative_path_names = {
    "rpc_finished_with_error",
    "udp_query_error",
}
per_test_warn_threshold = int(os.environ.get("CONFORMANCE_TRANSIENT_RETRY_WARN_THRESHOLD", "100"))
total_warn_threshold = int(
    os.environ.get(
        "CONFORMANCE_TRANSIENT_RETRY_TOTAL_WARN_THRESHOLD",
        str(per_test_warn_threshold),
    )
)

signal_counts = {name: 0 for name in signals}
transient_by_test = {}
final_fail_count = 0
pass_marker_count = 0
fail_examples = []
current_test = "(unknown)"

for line_number, raw in enumerate(log_path.read_text(encoding="utf-8", errors="replace").splitlines(), 1):
    line = raw.strip()
    if line.startswith("=== RUN"):
        current_test = line.removeprefix("=== RUN").strip() or current_test

    line_transient_count = 0
    for name, pattern in signals.items():
        if pattern.search(line):
            signal_counts[name] += 1
            if name in transient_signal_names:
                line_transient_count += 1
    if line_transient_count:
        transient_by_test[current_test] = transient_by_test.get(current_test, 0) + line_transient_count

    if line.startswith("--- FAIL:") or line == "FAIL" or line.startswith("FAIL\t"):
        final_fail_count += 1
        if len(fail_examples) < 5:
            fail_examples.append({"line": line_number, "text": line})
    if line.startswith("--- PASS:") or line == "PASS" or line.startswith("PASS\t"):
        pass_marker_count += 1

transient_retry_count = sum(signal_counts[name] for name in transient_signal_names)
readiness_wait_count = sum(signal_counts[name] for name in readiness_wait_signal_names)
expected_negative_path_error_count = sum(signal_counts[name] for name in expected_negative_path_names)
if final_fail_count:
    final_status = "fail"
elif pass_marker_count:
    final_status = "pass"
else:
    final_status = "unknown"
top_transient_retry_tests = [
    {"test": test, "transient_retry_count": count}
    for test, count in sorted(transient_by_test.items(), key=lambda item: (-item[1], item[0]))[:10]
]
stability_reasons = []
if transient_retry_count > total_warn_threshold:
    stability_reasons.append("total_transient_retry_count")
if any(item["transient_retry_count"] > per_test_warn_threshold for item in top_transient_retry_tests):
    stability_reasons.append("per_test_transient_retry_count")
stability_signal = "elevated" if stability_reasons else "normal"

summary = {
    "final_status": final_status,
    "stability_signal": stability_signal,
    "stability_reasons": stability_reasons,
    "transient_retry_count": transient_retry_count,
    "readiness_wait_count": readiness_wait_count,
    "transient_retry_warn_threshold": per_test_warn_threshold,
    "transient_retry_per_test_warn_threshold": per_test_warn_threshold,
    "transient_retry_total_warn_threshold": total_warn_threshold,
    "expected_negative_path_error_count": expected_negative_path_error_count,
    "final_fail_count": final_fail_count,
    "pass_marker_count": pass_marker_count,
    "signals": {name: signal_counts[name] for name in sorted(signal_counts)},
    "top_transient_retry_tests": top_transient_retry_tests,
    "final_fail_examples": fail_examples,
}

json_output.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")

lines = [
    "# Conformance Log Summary",
    "",
    f"- final status: `{final_status}`",
    f"- stability signal: `{stability_signal}`",
    f"- stability reasons: `{', '.join(stability_reasons) if stability_reasons else 'none'}`",
    f"- transient retry count: `{transient_retry_count}`",
    f"- readiness wait count: `{readiness_wait_count}`",
    f"- transient retry warning threshold/total: `{total_warn_threshold}`",
    f"- transient retry warning threshold/test: `{per_test_warn_threshold}`",
    f"- expected negative-path error count: `{expected_negative_path_error_count}`",
    f"- final FAIL count: `{final_fail_count}`",
    "",
    "## Signals",
    "",
    "| Signal | Count |",
    "| --- | ---: |",
]
for name in sorted(signal_counts):
    lines.append(f"| `{name}` | {signal_counts[name]} |")

if top_transient_retry_tests:
    lines.extend([
        "",
        "## Top Transient Retry Tests",
        "",
        "| Test | Transient Retry Count |",
        "| --- | ---: |",
    ])
    for item in top_transient_retry_tests:
        lines.append(f"| `{item['test']}` | {item['transient_retry_count']} |")

if fail_examples:
    lines.extend(["", "## Final Fail Examples", ""])
    for item in fail_examples:
        lines.append(f"- line {item['line']}: `{item['text']}`")

markdown_output.write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
}
