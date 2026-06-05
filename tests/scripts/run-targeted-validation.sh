#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TARGET_SCRIPT="${ROOT_DIR}/scripts/run-targeted-validation.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[run-targeted-validation-test] %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if ! grep -Fq "${pattern}" "${path}"; then
    fail "${label}: expected '${pattern}' in ${path}"
  fi
}

assert_not_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Fq "${pattern}" "${path}"; then
    fail "${label}: did not expect '${pattern}' in ${path}"
  fi
}

PLAN_ONLY=true INCLUDE_KIND=false "${TARGET_SCRIPT}" \
  deploy/kubernetes/base/dataplane.yaml \
  >"${TMP_DIR}/deploy-plan.log"

assert_contains "${TMP_DIR}/deploy-plan.log" \
  '[targeted-validation] skipped validations:' \
  'print skipped validation section for kind-affecting changes'
assert_contains "${TMP_DIR}/deploy-plan.log" \
  'SKIP_BUILD=true ./tests/e2e/run-kind.sh' \
  'identify skipped kind smoke command'
assert_contains "${TMP_DIR}/deploy-plan.log" \
  'enable: rerun with INCLUDE_KIND=true' \
  'explain how to enable skipped kind smoke'
assert_not_contains "${TMP_DIR}/deploy-plan.log" \
  'Documentation-only changes do not require runtime validation.' \
  'do not classify deploy changes as documentation-only'

PLAN_ONLY=true INCLUDE_KIND=false "${TARGET_SCRIPT}" \
  controlplane/internal/admin/metrics.go \
  >"${TMP_DIR}/metrics-plan.log"

assert_contains "${TMP_DIR}/metrics-plan.log" \
  'cd controlplane && go test ./...' \
  'still select cheap controlplane validation'
assert_contains "${TMP_DIR}/metrics-plan.log" \
  './tests/e2e/validate-metrics-surfaces.sh' \
  'identify skipped live metrics validation'
assert_contains "${TMP_DIR}/metrics-plan.log" \
  'reason: metrics/admin surface validation requires live Kind services and INCLUDE_KIND=false' \
  'explain why live metrics validation is skipped'

PLAN_ONLY=true INCLUDE_KIND=false "${TARGET_SCRIPT}" \
  docs/contracts/metrics-cardinality.md \
  deploy/observability/grafana/aether-gateway-observability-dashboard.json \
  >"${TMP_DIR}/metrics-contract-plan.log"

assert_contains "${TMP_DIR}/metrics-contract-plan.log" \
  './scripts/check-metrics-cardinality-contract.sh' \
  'run metrics cardinality contract check for metrics docs and observability asset changes'
assert_contains "${TMP_DIR}/metrics-contract-plan.log" \
  'reason: metrics cardinality contract or observability assets changed' \
  'explain why metrics cardinality contract check is selected'

PLAN_ONLY=true INCLUDE_KIND=false "${TARGET_SCRIPT}" \
  README.md \
  SECURITY.md \
  docs/community-readiness.md \
  >"${TMP_DIR}/community-governance-plan.log"

assert_contains "${TMP_DIR}/community-governance-plan.log" \
  './scripts/check-community-governance-contract.sh' \
  'run community governance contract check for public community docs'
assert_contains "${TMP_DIR}/community-governance-plan.log" \
  'reason: community governance or public project contract changed' \
  'explain why community governance contract check is selected'

PLAN_ONLY=true INCLUDE_KIND=false "${TARGET_SCRIPT}" \
  scripts/script-inventory.yaml \
  scripts/lib/common.sh \
  docs/developer/scripts.md \
  .agents/skills/aether-repo-scripts/references/script-catalog.md \
  >"${TMP_DIR}/script-inventory-plan.log"

assert_contains "${TMP_DIR}/script-inventory-plan.log" \
  './scripts/check-script-inventory.sh' \
  'run script inventory check for script metadata, helpers, docs, and skill catalog changes'
assert_contains "${TMP_DIR}/script-inventory-plan.log" \
  'reason: script inventory, helper, or script docs changed' \
  'explain why script inventory contract check is selected'

PLAN_ONLY=true INCLUDE_KIND=false "${TARGET_SCRIPT}" \
  scripts/check-dependabot-alert-triage.sh \
  scripts/run-security-scans.sh \
  docs/security/risk-register.md \
  >"${TMP_DIR}/dependabot-triage-plan.log"

assert_contains "${TMP_DIR}/dependabot-triage-plan.log" \
  'bash tests/scripts/dependabot-alert-triage.sh' \
  'run Dependabot alert triage test for alert policy changes'
assert_contains "${TMP_DIR}/dependabot-triage-plan.log" \
  'bash tests/scripts/run-security-scans.sh' \
  'run security scan bundle test for alert gate integration changes'
assert_contains "${TMP_DIR}/dependabot-triage-plan.log" \
  'reason: Dependabot alert triage policy changed' \
  'explain why Dependabot alert triage test is selected'

PLAN_ONLY=true INCLUDE_KIND=false "${TARGET_SCRIPT}" \
  scripts/removed-validation-hook.sh \
  >"${TMP_DIR}/removed-shell-plan.log"

assert_not_contains "${TMP_DIR}/removed-shell-plan.log" \
  'bash -n scripts/removed-validation-hook.sh' \
  'do not run bash syntax checks for deleted shell scripts'
assert_contains "${TMP_DIR}/removed-shell-plan.log" \
  './scripts/check-script-inventory.sh' \
  'still check script inventory when a tracked shell script is deleted'

printf '[run-targeted-validation-test] ok\n'
