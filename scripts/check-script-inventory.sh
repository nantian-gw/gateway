#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'EOF'
usage: check-script-inventory.sh [--repo-root <path>]

Verifies that the script inventory covers every scripts/*.sh and
scripts/lib/*.sh file. Each entry must be listed in
scripts/script-inventory.yaml with a supported class, owner, purpose, and
documentation flag.
EOF
}

log() {
  printf '[script-inventory] %s\n' "$*"
}

fail() {
  printf '[script-inventory] %s\n' "$*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || {
        usage >&2
        exit 2
      }
      repo_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

inventory="${repo_root}/scripts/script-inventory.yaml"
docs_path="${repo_root}/docs/developer/scripts.md"

[[ -f "${inventory}" ]] || fail "required file not found: ${inventory}"
[[ -f "${docs_path}" ]] || fail "required file not found: ${docs_path}"

python3 - "${repo_root}" "${inventory}" "${docs_path}" <<'PY'
from pathlib import Path
import re
import sys

repo = Path(sys.argv[1])
inventory_path = Path(sys.argv[2])
docs_path = Path(sys.argv[3])

valid_classes = {"stable", "check", "audit", "evidence", "internal", "candidate", "deprecated"}
inventory_text = inventory_path.read_text(encoding="utf-8")
docs_text = docs_path.read_text(encoding="utf-8")

entries = []
current = None
for raw_line in inventory_text.splitlines():
    line = raw_line.rstrip()
    if not line or line.lstrip().startswith("#") or line.strip() == "scripts:":
        continue
    match = re.match(r"\s*-\s+path:\s+(.+)\s*$", line)
    if match:
        if current:
            entries.append(current)
        current = {"path": match.group(1).strip().strip('"')}
        continue
    match = re.match(r"\s+([a-zA-Z_]+):\s+(.+)\s*$", line)
    if match and current is not None:
        current[match.group(1)] = match.group(2).strip().strip('"')
        continue
    raise SystemExit(f"{inventory_path}: unsupported inventory line: {raw_line}")
if current:
    entries.append(current)

errors = []
seen = set()
for entry in entries:
    path = entry.get("path", "")
    if path in seen:
        errors.append(f"duplicate inventory entry: {path}")
    seen.add(path)
    if not path.startswith("scripts/") or not path.endswith(".sh"):
        errors.append(f"invalid script path: {path}")
    if entry.get("class") not in valid_classes:
        errors.append(f"{path}: invalid class {entry.get('class')!r}")
    if not entry.get("purpose"):
        errors.append(f"{path}: missing purpose")
    if not entry.get("owner"):
        errors.append(f"{path}: missing owner")
    documented = entry.get("documented")
    if documented not in {"true", "false"}:
        errors.append(f"{path}: documented must be true or false")
    if documented == "true" and path not in docs_text:
        errors.append(f"{path}: documented=true but path is missing from {docs_path}")
    if not (repo / path).is_file():
        errors.append(f"{path}: inventory entry points to missing file")

actual_scripts = {
    str(path.relative_to(repo))
    for path in (repo / "scripts").glob("*.sh")
}
actual_scripts.update(
    str(path.relative_to(repo))
    for path in (repo / "scripts" / "lib").glob("*.sh")
)

missing = sorted(actual_scripts - seen)
extra = sorted(seen - actual_scripts)
for path in missing:
    errors.append(f"{path}: script file is missing from inventory")
for path in extra:
    errors.append(f"{path}: inventory entry has no matching file")

if errors:
    for error in errors:
        print(f"[script-inventory] {error}", file=sys.stderr)
    raise SystemExit(1)

print(f"[script-inventory] checked {len(entries)} inventory entries")
PY

log "script inventory aligned"
