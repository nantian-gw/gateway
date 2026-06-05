#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT_DIR}/scripts/run-kind-a4-baseline.sh"

fail() {
  printf '[run-kind-a4-live-reload-test] %s\n' "$*" >&2
  exit 1
}

bash -n "${SCRIPT}"

required_profiles=(
  "live-reload-route-only"
  "live-reload-backend-only"
  "live-reload-endpoint-only"
  "live-reload-secret-only"
  "live-reload-tls-asset-rotation"
  "live-reload-listener-add-remove"
)

for profile in "${required_profiles[@]}"; do
  grep -q "${profile}" "${SCRIPT}" \
    || fail "expected run-kind-a4-baseline.sh to define ${profile}"
done

grep -q 'run_live_reload_profiles' "${SCRIPT}" \
  || fail "expected run-kind-a4-baseline.sh to run live reload profiles"
grep -q '## Live Reload Profiles' "${SCRIPT}" \
  || fail "expected kind A4 summary to include live reload profiles"
grep -q 'reload-under-load' "${SCRIPT}" \
  || fail "expected live reload profiles to mark the reload-under-load scenario"

printf '[run-kind-a4-live-reload-test] ok\n'
