#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
alerts_json=""
output_dir="${OUTPUT_DIR:-${repo_root}/tmp/dependabot-alert-triage/latest}"
github_repository="${GITHUB_REPOSITORY:-}"

usage() {
  cat <<'EOF'
usage: check-dependabot-alert-triage.sh [--repo-root <path>] [--alerts-json <path>] [--output-dir <path>] [--github-repository <owner/repo>]

Dependabot alert triage gate.

Fetches open GitHub Dependabot alerts, or reads a provided alerts JSON file,
then verifies that every open alert is either already fixed in the local tree
and waiting for GitHub to refresh, or has an explicit repository risk
acceptance entry.
EOF
}

log() {
  printf '[dependabot-alert-triage] %s\n' "$*"
}

fail() {
  printf '[dependabot-alert-triage] %s\n' "$*" >&2
  exit 1
}

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    fail "missing required command: ${name}"
  fi
}

normalize_remote_repository() {
  local remote_url

  remote_url="$(git -C "${repo_root}" remote get-url origin 2>/dev/null || true)"
  if [[ -z "${remote_url}" ]]; then
    return
  fi

  python3 - "${remote_url}" <<'PY'
import re
import sys

remote = sys.argv[1].strip()
patterns = [
    r"^git@github\.com:(?P<repo>[^/]+/[^/]+?)(?:\.git)?$",
    r"^https://github\.com/(?P<repo>[^/]+/[^/]+?)(?:\.git)?$",
    r"^ssh://git@github\.com/(?P<repo>[^/]+/[^/]+?)(?:\.git)?$",
]
for pattern in patterns:
    match = re.match(pattern, remote)
    if match:
        print(match.group("repo"))
        break
PY
}

fetch_open_alerts() {
  local output="$1"
  local jsonl

  if [[ -n "${alerts_json}" ]]; then
    cp "${alerts_json}" "${output}"
    return
  fi

  require_command gh
  if [[ -z "${github_repository}" ]]; then
    github_repository="$(normalize_remote_repository)"
  fi
  [[ -n "${github_repository}" ]] || fail "unable to determine GitHub repository; pass --github-repository owner/repo"

  jsonl="${output}.jsonl"
  gh api \
    --paginate \
    "repos/${github_repository}/dependabot/alerts?state=open&per_page=100" \
    --jq '.[] | @json' >"${jsonl}"
  python3 - "${jsonl}" "${output}" <<'PY'
import json
import sys
from pathlib import Path

src = Path(sys.argv[1])
dst = Path(sys.argv[2])
alerts = []
if src.exists():
    for line in src.read_text(encoding="utf-8").splitlines():
        if line.strip():
            alerts.append(json.loads(line))
dst.write_text(json.dumps(alerts, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || fail "missing value for --repo-root"
      repo_root="$2"
      shift 2
      ;;
    --alerts-json)
      [[ $# -ge 2 ]] || fail "missing value for --alerts-json"
      alerts_json="$2"
      shift 2
      ;;
    --output-dir)
      [[ $# -ge 2 ]] || fail "missing value for --output-dir"
      output_dir="$2"
      shift 2
      ;;
    --github-repository)
      [[ $# -ge 2 ]] || fail "missing value for --github-repository"
      github_repository="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      fail "unknown argument: $1"
      ;;
  esac
done

mkdir -p "${output_dir}"
open_alerts="${output_dir}/open-alerts.json"
summary_json="${output_dir}/summary.json"
summary_tsv="${output_dir}/summary.tsv"

fetch_open_alerts "${open_alerts}"

python3 - \
  "${repo_root}" \
  "${open_alerts}" \
  "${summary_json}" \
  "${summary_tsv}" <<'PY'
from __future__ import annotations

import csv
import json
import re
import sys
from pathlib import Path
from typing import Any

repo = Path(sys.argv[1])
alerts_path = Path(sys.argv[2])
summary_json = Path(sys.argv[3])
summary_tsv = Path(sys.argv[4])


def load_alerts(path: Path) -> list[dict[str, Any]]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, list):
        raise SystemExit(f"{path}: expected a JSON array")
    return [item for item in value if isinstance(item, dict)]


def file_contains(path: Path, *needles: str) -> bool:
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8", errors="replace")
    return all(needle in text for needle in needles)


def lock_version(package: str) -> str | None:
    path = repo / "dataplane" / "Cargo.lock"
    if not path.is_file():
        return None
    text = path.read_text(encoding="utf-8", errors="replace")
    for block in re.split(r"\n(?=\[\[package\]\])", text):
        if re.search(rf'^name = "{re.escape(package)}"$', block, re.MULTILINE):
            match = re.search(r'^version = "([^"]+)"$', block, re.MULTILINE)
            if match:
                return match.group(1)
    return None


def package_lock_versions(package: str) -> list[str]:
    path = repo / "dashboard" / "package-lock.json"
    if not path.is_file():
        return []
    payload = json.loads(path.read_text(encoding="utf-8"))
    packages = payload.get("packages")
    if not isinstance(packages, dict):
        return []

    versions: list[str] = []
    suffix = f"node_modules/{package}"
    for lock_path, metadata in packages.items():
        if not isinstance(lock_path, str) or not isinstance(metadata, dict):
            continue
        if lock_path == suffix or lock_path.endswith(f"/{suffix}"):
            version = metadata.get("version")
            if isinstance(version, str) and version:
                versions.append(version)
    return sorted(set(versions), key=version_tuple)


def version_tuple(value: str | None) -> tuple[int, ...]:
    if not value:
        return ()
    return tuple(int(part) for part in re.findall(r"\d+", value))


def alert_field(alert: dict[str, Any], *path: str) -> Any:
    value: Any = alert
    for item in path:
        if not isinstance(value, dict):
            return None
        value = value.get(item)
    return value


def classify(alert: dict[str, Any]) -> tuple[str, str, bool]:
    ecosystem = str(alert_field(alert, "dependency", "package", "ecosystem") or "")
    package = str(alert_field(alert, "dependency", "package", "name") or "")
    vulnerable_range = str(alert_field(alert, "security_vulnerability", "vulnerable_version_range") or "")
    first_patched = alert_field(alert, "security_vulnerability", "first_patched_version", "identifier")

    if ecosystem == "rust" and package == "openssl":
        openssl_version = lock_version("openssl")
        openssl_sys_version = lock_version("openssl-sys")
        patched_version = str(first_patched or "")
        if not patched_version:
            if "< 0.10.80" in vulnerable_range:
                patched_version = "0.10.80"
            elif "< 0.10.79" in vulnerable_range:
                patched_version = "0.10.79"
        if patched_version and version_tuple(openssl_version) >= version_tuple(patched_version):
            return (
                "fixed-awaiting-platform-refresh",
                f"local Cargo.lock has openssl {openssl_version} and openssl-sys {openssl_sys_version}",
                True,
            )
        return (
            "unreviewed",
            "openssl alert remains open and local Cargo.lock is still vulnerable",
            False,
        )

    if ecosystem == "npm" and package in {"dompurify", "postcss"}:
        patched_version = str(first_patched or "")
        versions = package_lock_versions(package)
        vulnerable_versions = [
            version for version in versions
            if not patched_version or version_tuple(version) < version_tuple(patched_version)
        ]
        if versions and not vulnerable_versions:
            return (
                "fixed-awaiting-platform-refresh",
                f"dashboard package-lock has {package} {', '.join(versions)}",
                True,
            )
        if versions:
            return (
                "unreviewed",
                f"dashboard package-lock still has vulnerable {package} versions: {', '.join(vulnerable_versions)}",
                False,
            )
        return (
            "unreviewed",
            f"dashboard package-lock does not contain {package}",
            False,
        )

    if ecosystem == "rust" and package == "protobuf" and "< 3.7.2" in vulnerable_range:
        risk_ok = file_contains(repo / "docs" / "security" / "risk-register.md", "SEC-RA-004", "protobuf < 3.7.2")
        deny_ok = file_contains(repo / "dataplane" / "deny.toml", "RUSTSEC-2024-0437", "SEC-RA-004")
        if risk_ok and deny_ok:
            return (
                "risk-accepted:SEC-RA-004",
                "protobuf alert is documented in risk register and deny.toml",
                True,
            )
        return (
            "unreviewed",
            "protobuf alert is missing SEC-RA-004 risk register or deny.toml evidence",
            False,
        )

    return ("unreviewed", "no repository triage policy matched this open alert", False)


alerts = [item for item in load_alerts(alerts_path) if item.get("state") == "open"]
rows = []
unreviewed = []
for alert in alerts:
    classification, reason, ok = classify(alert)
    row = {
        "number": alert.get("number"),
        "state": alert.get("state"),
        "ecosystem": alert_field(alert, "dependency", "package", "ecosystem"),
        "package": alert_field(alert, "dependency", "package", "name"),
        "severity": alert_field(alert, "security_vulnerability", "severity"),
        "vulnerable_range": alert_field(alert, "security_vulnerability", "vulnerable_version_range"),
        "classification": classification,
        "reason": reason,
    }
    rows.append(row)
    if not ok:
        unreviewed.append(row)

summary_json.write_text(json.dumps({"open_alert_count": len(alerts), "unreviewed_count": len(unreviewed), "alerts": rows}, indent=2, sort_keys=True) + "\n", encoding="utf-8")
with summary_tsv.open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=["number", "state", "ecosystem", "package", "severity", "vulnerable_range", "classification", "reason"], delimiter="\t")
    writer.writeheader()
    writer.writerows(rows)

if unreviewed:
    print(f"[dependabot-alert-triage] unreviewed Dependabot alerts: {len(unreviewed)}", file=sys.stderr)
    for row in unreviewed:
        print(
            f"[dependabot-alert-triage] #{row['number']} {row['ecosystem']}/{row['package']} {row['severity']}: {row['reason']}",
            file=sys.stderr,
        )
    raise SystemExit(1)

print(f"[dependabot-alert-triage] reviewed open alerts: {len(alerts)}")
PY

log "Dependabot alert triage passed"
