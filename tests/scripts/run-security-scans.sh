#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT_DIR}/scripts/run-security-scans.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[run-security-scans-test] %s\n' "$*" >&2
  exit 1
}

FAKE_BIN="${TMP_DIR}/bin"
mkdir -p "${FAKE_BIN}"

cat >"${FAKE_BIN}/cargo-audit" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--version" ]]; then
  printf 'cargo-audit 9.9.9\n'
  exit 0
fi
printf '{"ok":true}\n'
exit "${FAKE_CARGO_AUDIT_EXIT:-0}"
EOF
chmod +x "${FAKE_BIN}/cargo-audit"

cat >"${FAKE_BIN}/osv-scanner" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--version" ]]; then
  printf 'osv-scanner 9.9.9\n'
  exit 0
fi
if [[ -n "${FAKE_OSV_ARGS_FILE:-}" ]]; then
  printf '%s\n' "$*" >"${FAKE_OSV_ARGS_FILE}"
fi
output=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
[[ -n "${output}" ]] && printf '{"ok":true}\n' >"${output}"
exit "${FAKE_OSV_SCANNER_EXIT:-0}"
EOF
chmod +x "${FAKE_BIN}/osv-scanner"

cat >"${FAKE_BIN}/grype" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "version" ]]; then
  printf 'grype 9.9.9\n'
  exit 0
fi
if [[ -n "${FAKE_GRYPE_ARGS_FILE:-}" ]]; then
  printf '%s\n' "$*" >"${FAKE_GRYPE_ARGS_FILE}"
fi
printf '{"ok":true}\n'
exit "${FAKE_GRYPE_EXIT:-0}"
EOF
chmod +x "${FAKE_BIN}/grype"

cat >"${FAKE_BIN}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  version)
    printf 'kubectl fake client\n'
    ;;
  kustomize)
    printf 'apiVersion: v1\nkind: Namespace\nmetadata:\n  name: fake\n'
    ;;
  *)
    printf 'unexpected kubectl command: %s\n' "${1:-}" >&2
    exit 1
    ;;
esac
EOF
chmod +x "${FAKE_BIN}/kubectl"

cat >"${FAKE_BIN}/kubescape" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "version" ]]; then
  printf 'kubescape 9.9.9\n'
  exit 0
fi
output=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
[[ -n "${output}" ]] && printf '{"ok":true}\n' >"${output}"
exit "${FAKE_KUBESCAPE_EXIT:-0}"
EOF
chmod +x "${FAKE_BIN}/kubescape"

ALERTS_JSON="${TMP_DIR}/alerts.json"
cat >"${ALERTS_JSON}" <<'JSON'
[
  {
    "number": 26,
    "state": "open",
    "dependency": {"package": {"ecosystem": "rust", "name": "openssl"}},
    "security_vulnerability": {"severity": "high", "vulnerable_version_range": ">= 0.9.7, < 0.10.79"}
  },
  {
    "number": 7,
    "state": "open",
    "dependency": {"package": {"ecosystem": "rust", "name": "protobuf"}},
    "security_vulnerability": {"severity": "medium", "vulnerable_version_range": "< 3.7.2"}
  }
]
JSON

OUTPUT_DIR="${TMP_DIR}/pass"
PATH="${FAKE_BIN}:${PATH}" \
DEPENDABOT_ALERT_TRIAGE_ALERTS_JSON="${ALERTS_JSON}" \
FAKE_OSV_ARGS_FILE="${TMP_DIR}/osv.args" \
FAKE_GRYPE_ARGS_FILE="${TMP_DIR}/grype.args" \
OUTPUT_DIR="${OUTPUT_DIR}" \
"${SCRIPT}" >"${TMP_DIR}/pass.log"

grep -q 'dependabot_alert_triage=passed' "${OUTPUT_DIR}/summary.txt" \
  || fail "expected Dependabot alert triage to be part of security scan bundle"
[[ -f "${OUTPUT_DIR}/dependabot-alert-triage/summary.tsv" ]] \
  || fail "expected Dependabot triage summary artifact"
grep -Fq -- '--experimental-exclude .worktrees' "${TMP_DIR}/osv.args" \
  || fail "expected osv-scanner to exclude local worktrees"
grep -Fq -- '--experimental-exclude tmp' "${TMP_DIR}/osv.args" \
  || fail "expected osv-scanner to exclude tmp artifacts"
grep -Fq -- '--exclude **/.worktrees/**' "${TMP_DIR}/grype.args" \
  || fail "expected grype to exclude local worktrees"
grep -Fq -- '--exclude **/tmp/**' "${TMP_DIR}/grype.args" \
  || fail "expected grype to exclude tmp artifacts"

FAIL_OUTPUT_DIR="${TMP_DIR}/fail"
if PATH="${FAKE_BIN}:${PATH}" \
  DEPENDABOT_ALERT_TRIAGE_ALERTS_JSON="${ALERTS_JSON}" \
  OUTPUT_DIR="${FAIL_OUTPUT_DIR}" \
  FAKE_GRYPE_EXIT=42 \
  "${SCRIPT}" >"${TMP_DIR}/fail.stdout" 2>"${TMP_DIR}/fail.stderr"; then
  fail "expected scanner failure to propagate"
fi
grep -q 'grype=failed:42' "${FAIL_OUTPUT_DIR}/summary.txt" \
  || fail "expected original scanner exit code to be recorded"
grep -q 'one or more security scans failed' "${TMP_DIR}/fail.stdout" \
  || fail "expected aggregate security scan failure message"

printf '[run-security-scans-test] ok\n'
