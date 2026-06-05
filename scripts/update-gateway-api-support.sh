#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOC_PATH="${ROOT_DIR}/docs/gateway-api-support.md"
MODE="${1:-write}"
GENERATED_FILE="$(mktemp)"
UPDATED_FILE="$(mktemp)"

cleanup() {
  rm -f "${GENERATED_FILE}" "${UPDATED_FILE}"
}
trap cleanup EXIT

log() {
  printf '[update-gateway-api-support] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

case "${MODE}" in
  write|--write)
    MODE="write"
    ;;
  check|--check)
    MODE="check"
    ;;
  *)
    log "usage: $0 [--check]"
    exit 2
    ;;
esac

require_command go
require_command python3

(
  cd "${ROOT_DIR}/controlplane"
  go run ./cmd/gateway-api-support -format markdown
) >"${GENERATED_FILE}"

python3 - "${DOC_PATH}" "${GENERATED_FILE}" "${UPDATED_FILE}" <<'PY'
from pathlib import Path
import sys

doc_path = Path(sys.argv[1])
generated_path = Path(sys.argv[2])
output_path = Path(sys.argv[3])

begin = "<!-- BEGIN GENERATED SUPPORTED FEATURES -->"
end = "<!-- END GENERATED SUPPORTED FEATURES -->"

doc = doc_path.read_text(encoding="utf-8")
generated = generated_path.read_text(encoding="utf-8").strip()

start = doc.find(begin)
stop = doc.find(end)
if start == -1 or stop == -1 or stop < start:
    raise SystemExit("generated supported-features markers were not found in docs/gateway-api-support.md")

stop += len(end)
updated = doc[:start] + generated + doc[stop:]
if not updated.endswith("\n"):
    updated += "\n"
output_path.write_text(updated, encoding="utf-8")
PY

if [[ "${MODE}" == "check" ]]; then
  if cmp -s "${DOC_PATH}" "${UPDATED_FILE}"; then
    log "docs/gateway-api-support.md is up to date"
    exit 0
  fi

  log "docs/gateway-api-support.md is stale; run scripts/update-gateway-api-support.sh"
  diff -u "${DOC_PATH}" "${UPDATED_FILE}" || true
  exit 1
fi

mv "${UPDATED_FILE}" "${DOC_PATH}"
log "updated ${DOC_PATH}"
