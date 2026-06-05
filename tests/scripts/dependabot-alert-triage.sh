#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK_SCRIPT="${ROOT_DIR}/scripts/check-dependabot-alert-triage.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[dependabot-alert-triage-test] %s\n' "$*" >&2
  exit 1
}

write_fixture_repo() {
  local repo="$1"

  mkdir -p "${repo}/dashboard" "${repo}/dataplane" "${repo}/docs/security"
  cat >"${repo}/dataplane/Cargo.lock" <<'EOF'
[[package]]
name = "openssl"
version = "0.10.80"

[[package]]
name = "openssl-sys"
version = "0.9.116"
EOF
  cat >"${repo}/dashboard/package-lock.json" <<'EOF'
{
  "name": "dashboard",
  "lockfileVersion": 3,
  "packages": {
    "node_modules/dompurify": {
      "version": "3.4.0"
    },
    "node_modules/next/node_modules/postcss": {
      "version": "8.5.14"
    },
    "node_modules/postcss": {
      "version": "8.5.14"
    }
  }
}
EOF
  cat >"${repo}/dataplane/deny.toml" <<'EOF'
[advisories]
ignore = [
  { id = "RUSTSEC-2024-0437", reason = "Tracked as SEC-RA-004" },
]
EOF
  cat >"${repo}/docs/security/risk-register.md" <<'EOF'
| `SEC-RA-004` | `Accepted` | dataplane dependency graph still contains `protobuf < 3.7.2` |
EOF
}

write_current_alerts() {
  local path="$1"

  cat >"${path}" <<'JSON'
[
  {
    "number": 36,
    "state": "open",
    "dependency": {"package": {"ecosystem": "rust", "name": "openssl"}},
    "security_vulnerability": {
      "severity": "medium",
      "vulnerable_version_range": ">= 0.10.50, < 0.10.80",
      "first_patched_version": {"identifier": "0.10.80"}
    }
  },
  {
    "number": 35,
    "state": "open",
    "dependency": {"package": {"ecosystem": "npm", "name": "dompurify"}},
    "security_vulnerability": {
      "severity": "medium",
      "vulnerable_version_range": "< 3.4.0",
      "first_patched_version": {"identifier": "3.4.0"}
    }
  },
  {
    "number": 25,
    "state": "open",
    "dependency": {"package": {"ecosystem": "npm", "name": "postcss"}},
    "security_vulnerability": {
      "severity": "medium",
      "vulnerable_version_range": "< 8.5.10",
      "first_patched_version": {"identifier": "8.5.10"}
    }
  },
  {
    "number": 7,
    "state": "open",
    "dependency": {"package": {"ecosystem": "rust", "name": "protobuf"}},
    "security_vulnerability": {"severity": "medium", "vulnerable_version_range": "< 3.7.2"}
  }
]
JSON
}

[[ -f "${CHECK_SCRIPT}" ]] || fail "missing checker: ${CHECK_SCRIPT}"

"${CHECK_SCRIPT}" --help >"${TMP_DIR}/help.txt"
grep -q 'Dependabot alert triage' "${TMP_DIR}/help.txt" \
  || fail "expected help output to describe Dependabot alert triage"

PASS_REPO="${TMP_DIR}/pass-repo"
PASS_ALERTS="${TMP_DIR}/alerts-current.json"
PASS_OUT="${TMP_DIR}/pass-out"
write_fixture_repo "${PASS_REPO}"
write_current_alerts "${PASS_ALERTS}"

"${CHECK_SCRIPT}" \
  --repo-root "${PASS_REPO}" \
  --alerts-json "${PASS_ALERTS}" \
  --output-dir "${PASS_OUT}" >"${TMP_DIR}/pass.log"

grep -q 'Dependabot alert triage passed' "${TMP_DIR}/pass.log" \
  || fail "expected pass summary"
grep -q 'fixed-awaiting-platform-refresh' "${PASS_OUT}/summary.tsv" \
  || fail "expected fixed alerts to be classified as fixed awaiting refresh"
grep -q $'35\topen\tnpm\tdompurify' "${PASS_OUT}/summary.tsv" \
  || fail "expected DOMPurify alert in summary"
grep -q $'25\topen\tnpm\tpostcss' "${PASS_OUT}/summary.tsv" \
  || fail "expected PostCSS alert in summary"
grep -q 'risk-accepted:SEC-RA-004' "${PASS_OUT}/summary.tsv" \
  || fail "expected protobuf alert to be classified with SEC-RA-004"

UNKNOWN_ALERTS="${TMP_DIR}/alerts-unknown.json"
cat >"${UNKNOWN_ALERTS}" <<'JSON'
[
  {
    "number": 99,
    "state": "open",
    "dependency": {"package": {"ecosystem": "rust", "name": "h2"}},
    "security_vulnerability": {"severity": "high", "vulnerable_version_range": "< 0.4.12"}
  }
]
JSON

if "${CHECK_SCRIPT}" \
  --repo-root "${PASS_REPO}" \
  --alerts-json "${UNKNOWN_ALERTS}" \
  --output-dir "${TMP_DIR}/unknown-out" \
  >"${TMP_DIR}/unknown.stdout" 2>"${TMP_DIR}/unknown.stderr"; then
  fail "expected unknown high alert to fail"
fi
grep -q 'unreviewed Dependabot alerts: 1' "${TMP_DIR}/unknown.stderr" \
  || fail "expected unreviewed alert failure"

STALE_REPO="${TMP_DIR}/stale-repo"
STALE_OUT="${TMP_DIR}/stale-out"
write_fixture_repo "${STALE_REPO}"
python3 - "${STALE_REPO}/dataplane/Cargo.lock" <<'PY'
from pathlib import Path
path = Path(__import__("sys").argv[1])
text = path.read_text(encoding="utf-8")
path.write_text(text.replace("0.10.80", "0.10.79"), encoding="utf-8")
PY

if "${CHECK_SCRIPT}" \
  --repo-root "${STALE_REPO}" \
  --alerts-json "${PASS_ALERTS}" \
  --output-dir "${STALE_OUT}" \
  >"${TMP_DIR}/stale.stdout" 2>"${TMP_DIR}/stale.stderr"; then
  fail "expected stale openssl lockfile to fail"
fi
grep -q 'openssl alert remains open and local Cargo.lock is still vulnerable' "${TMP_DIR}/stale.stderr" \
  || fail "expected stale openssl failure reason"

printf '[dependabot-alert-triage-test] ok\n'
