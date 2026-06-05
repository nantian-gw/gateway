#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK_SCRIPT="${ROOT_DIR}/scripts/check-script-inventory.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[script-inventory-test] %s\n' "$*" >&2
  exit 1
}

[[ -f "${CHECK_SCRIPT}" ]] || fail "missing checker: ${CHECK_SCRIPT}"

"${CHECK_SCRIPT}" --help >"${TMP_DIR}/help.txt"
grep -q 'script inventory' "${TMP_DIR}/help.txt" \
  || fail "expected --help output to describe the script inventory contract"

"${CHECK_SCRIPT}" --repo-root "${ROOT_DIR}" >"${TMP_DIR}/pass.log"
grep -q 'script inventory aligned' "${TMP_DIR}/pass.log" \
  || fail "expected success summary for aligned script inventory"

FAKE_ROOT="${TMP_DIR}/repo"
mkdir -p "${FAKE_ROOT}/scripts/lib" "${FAKE_ROOT}/docs/developer"
cat >"${FAKE_ROOT}/scripts/known.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"${FAKE_ROOT}/scripts/unlisted.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"${FAKE_ROOT}/scripts/lib/common.sh" <<'EOF'
#!/usr/bin/env bash
return 0
EOF
cat >"${FAKE_ROOT}/scripts/script-inventory.yaml" <<'EOF'
scripts:
  - path: scripts/known.sh
    class: stable
    purpose: fixture stable script
    owner: test
    documented: true
  - path: scripts/lib/common.sh
    class: internal
    purpose: fixture helper
    owner: test
    documented: true
EOF
cat >"${FAKE_ROOT}/docs/developer/scripts.md" <<'EOF'
# Scripts

- scripts/known.sh
- scripts/lib/common.sh
EOF

if "${CHECK_SCRIPT}" --repo-root "${FAKE_ROOT}" \
  >"${TMP_DIR}/fail.stdout" 2>"${TMP_DIR}/fail.stderr"; then
  fail "expected unlisted top-level script to fail inventory check"
fi

grep -q 'scripts/unlisted.sh' "${TMP_DIR}/fail.stderr" \
  || fail "expected missing script path in failure output"

COMMON_LIB="${ROOT_DIR}/scripts/lib/common.sh"
[[ -f "${COMMON_LIB}" ]] || fail "missing common helper: ${COMMON_LIB}"

# shellcheck source=/dev/null
source "${COMMON_LIB}"

[[ "$(aeg_repo_root)" == "${ROOT_DIR}" ]] \
  || fail "expected aeg_repo_root to resolve repository root"

aeg_require_file "${COMMON_LIB}"
aeg_require_dir "${ROOT_DIR}/scripts"
aeg_require_command bash

PATTERN_FILE="${TMP_DIR}/pattern.txt"
printf 'nantian-gw\n' >"${PATTERN_FILE}"
aeg_require_pattern 'nantian-gw' "${PATTERN_FILE}"

HELPER_DIR="${TMP_DIR}/helper-dir"
aeg_safe_mkdir "${HELPER_DIR}"
[[ -d "${HELPER_DIR}" ]] || fail "expected aeg_safe_mkdir to create a directory"

if (aeg_usage_error "bad usage") >"${TMP_DIR}/usage.stdout" 2>"${TMP_DIR}/usage.stderr"; then
  fail "expected aeg_usage_error to exit non-zero"
fi
[[ "$(cat "${TMP_DIR}/usage.stdout")" == "" ]] \
  || fail "expected aeg_usage_error to write only to stderr"
grep -q 'bad usage' "${TMP_DIR}/usage.stderr" \
  || fail "expected aeg_usage_error message on stderr"

printf '[script-inventory-test] ok\n'
