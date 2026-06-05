#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK_SCRIPT="${ROOT_DIR}/scripts/check-rust-unwraps.sh"
FIXTURE_DIR="${ROOT_DIR}/tests/scripts/fixtures/rust-unwraps"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

"${CHECK_SCRIPT}" \
  "${FIXTURE_DIR}/allowed_runtime.rs" \
  "${FIXTURE_DIR}/allowed_inline_tests.rs"

if "${CHECK_SCRIPT}" "${FIXTURE_DIR}/disallowed_runtime.rs" >"${TMP_DIR}/stderr.log" 2>&1; then
  echo "expected unwrap guardrail to reject runtime unwrap usage" >&2
  exit 1
fi

grep -q 'disallowed_runtime.rs' "${TMP_DIR}/stderr.log"
grep -q 'unwrap' "${TMP_DIR}/stderr.log"
