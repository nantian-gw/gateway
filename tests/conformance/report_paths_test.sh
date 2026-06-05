#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/scripts/lib/conformance-report.sh"

fail() {
  printf '[report-paths-test] %s\n' "$*" >&2
  exit 1
}

assert_eq() {
  local actual="$1"
  local expected="$2"
  local label="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    fail "${label}: expected '${expected}', got '${actual}'"
  fi
}

assert_dir() {
  local path="$1"
  local label="$2"

  if [[ ! -d "${path}" ]]; then
    fail "${label}: expected directory ${path}"
  fi
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "${TMP_ROOT}"' EXIT

FAKE_ROOT="${TMP_ROOT}/repo"
mkdir -p "${FAKE_ROOT}"

conformance_prepare_report_artifacts \
  "${FAKE_ROOT}" \
  "tmp/conformance" \
  "reports/release/report.yaml" \
  "tmp/conformance/run.log" \
  "reports/release/report.metadata.txt"

assert_eq "${REPORT_DIR}" "${FAKE_ROOT}/tmp/conformance" "relative report dir"
assert_eq "${REPORT_OUTPUT}" "${FAKE_ROOT}/reports/release/report.yaml" "relative report output"
assert_eq "${REPORT_OUTPUT_DIR}" "${FAKE_ROOT}/reports/release" "relative report output dir"
assert_eq "${CONFORMANCE_LOG_PATH}" "${FAKE_ROOT}/tmp/conformance/run.log" "relative log path"
assert_eq "${REPORT_METADATA_PATH}" "${FAKE_ROOT}/reports/release/report.metadata.txt" "relative metadata path"
assert_dir "${FAKE_ROOT}/tmp/conformance" "report dir creation"
assert_dir "${FAKE_ROOT}/reports/release" "report output dir creation"

ABS_ROOT="${TMP_ROOT}/abs"
mkdir -p "${ABS_ROOT}"

conformance_prepare_report_artifacts \
  "${FAKE_ROOT}" \
  "${ABS_ROOT}/report-dir" \
  "${ABS_ROOT}/nested/report.yaml"

assert_eq "${REPORT_DIR}" "${ABS_ROOT}/report-dir" "absolute report dir"
assert_eq "${REPORT_OUTPUT}" "${ABS_ROOT}/nested/report.yaml" "absolute report output"
assert_eq "${CONFORMANCE_LOG_PATH}" "${ABS_ROOT}/nested/report.log" "default log path"
assert_eq "${REPORT_METADATA_PATH}" "${ABS_ROOT}/nested/report.metadata.txt" "default metadata path"
assert_dir "${ABS_ROOT}/report-dir" "absolute report dir creation"
assert_dir "${ABS_ROOT}/nested" "absolute output dir creation"

printf '[report-paths-test] ok\n'
