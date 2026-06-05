#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT_DIR}/scripts/run-kind-a4-baseline.sh"

fail() {
  printf '[run-kind-a4-fault-scenarios-test] %s\n' "$*" >&2
  exit 1
}

bash -n "${SCRIPT}"

required_profiles=(
  "backend-error"
  "backend-slow-read"
  "backend-slow-write"
  "endpoint-flapping"
)

for profile in "${required_profiles[@]}"; do
  grep -q "${profile}" "${SCRIPT}" \
    || fail "expected run-kind-a4-baseline.sh to define ${profile}"
done

required_functions=(
  "ensure_a4_fault_scenario_resources"
  "run_backend_error_profile"
  "run_backend_slow_read_profile"
  "run_backend_slow_write_profile"
  "run_endpoint_flapping_profile"
  "run_fault_scenario_profiles"
  "annotate_fault_profile"
)

for function_name in "${required_functions[@]}"; do
  grep -q "${function_name}" "${SCRIPT}" \
    || fail "expected run-kind-a4-baseline.sh to define ${function_name}"
done

grep -q '## Fault Scenario Profiles' "${SCRIPT}" \
  || fail "expected kind A4 summary to include fault scenario profiles"

grep -q 'flap_backend' "${SCRIPT}" \
  || fail "expected endpoint-flapping profile to record the flapped backend"

grep -q 'request_queue_size = 1024' "${SCRIPT}" \
  || fail "expected A4 fault backend server to raise its TCP accept backlog"

printf '[run-kind-a4-fault-scenarios-test] ok\n'
