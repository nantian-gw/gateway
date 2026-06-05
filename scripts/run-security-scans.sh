#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/tmp/security-scans/latest}"
SCAN_ROOT="${SCAN_ROOT:-${ROOT_DIR}}"
OSV_SCAN_EXCLUDES="${OSV_SCAN_EXCLUDES:-.git .worktrees .sisyphus tmp node_modules dashboard/node_modules dashboard/dist dashboard/dist-ssr dashboard/.vite dataplane/target controlplane/bin target bin dist coverage}"
GRYPE_SCAN_EXCLUDES="${GRYPE_SCAN_EXCLUDES:-**/.git/** **/.worktrees/** **/.sisyphus/** **/tmp/** **/node_modules/** **/dashboard/node_modules/** **/dashboard/dist/** **/dashboard/dist-ssr/** **/dashboard/.vite/** **/dataplane/target/** **/controlplane/bin/** **/target/** **/bin/** **/dist/** **/coverage/**}"
PRODUCTION_OVERLAY_DIR="${PRODUCTION_OVERLAY_DIR:-${ROOT_DIR}/deploy/kubernetes/overlays/production}"
PRODUCTION_RENDERED_MANIFEST="${PRODUCTION_RENDERED_MANIFEST:-${OUTPUT_DIR}/production-overlay.yaml}"
KUBESCAPE_EXCEPTIONS="${KUBESCAPE_EXCEPTIONS:-${ROOT_DIR}/tests/security/kubescape-exceptions.json}"
GRYPE_FAIL_ON="${GRYPE_FAIL_ON:-high}"
KUBESCAPE_SEVERITY_THRESHOLD="${KUBESCAPE_SEVERITY_THRESHOLD:-high}"
CARGO_AUDIT_LOCKFILE="${CARGO_AUDIT_LOCKFILE:-${ROOT_DIR}/dataplane/Cargo.lock}"
CARGO_AUDIT_IGNORE_IDS="${CARGO_AUDIT_IGNORE_IDS:-RUSTSEC-2024-0437}"
CARGO_AUDIT_NO_FETCH="${CARGO_AUDIT_NO_FETCH:-false}"
CARGO_AUDIT_STALE="${CARGO_AUDIT_STALE:-false}"
DEPENDABOT_ALERT_TRIAGE="${DEPENDABOT_ALERT_TRIAGE:-true}"
DEPENDABOT_ALERT_TRIAGE_ALERTS_JSON="${DEPENDABOT_ALERT_TRIAGE_ALERTS_JSON:-}"
DEPENDABOT_ALERT_TRIAGE_REPOSITORY="${DEPENDABOT_ALERT_TRIAGE_REPOSITORY:-${GITHUB_REPOSITORY:-}}"

log() {
  printf '[security-scans] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

record_result() {
  local name="$1"
  local status="$2"

  printf '%s=%s\n' "${name}" "${status}" >>"${OUTPUT_DIR}/summary.txt"
}

run_cargo_audit() {
  local status=0
  local ignore_id
  local -a args

  require_command cargo-audit

  args=(
    audit
    --file "${CARGO_AUDIT_LOCKFILE}"
    --format json
  )
  if [[ "${CARGO_AUDIT_NO_FETCH}" == "true" ]]; then
    args+=(--no-fetch)
  fi
  if [[ "${CARGO_AUDIT_STALE}" == "true" ]]; then
    args+=(--stale)
  fi
  for ignore_id in ${CARGO_AUDIT_IGNORE_IDS//,/ }; do
    if [[ -n "${ignore_id}" ]]; then
      args+=(--ignore "${ignore_id}")
    fi
  done

  log "running cargo audit"
  if cargo-audit "${args[@]}" \
    >"${OUTPUT_DIR}/cargo-audit.json" \
    2>"${OUTPUT_DIR}/cargo-audit.log"; then
    status=0
  else
    status=$?
  fi

  if [[ ${status} -eq 0 ]]; then
    record_result cargo_audit passed
  else
    record_result cargo_audit "failed:${status}"
  fi

  return "${status}"
}

run_osv_scanner() {
  local status=0
  local exclude
  local -a excludes
  local -a args

  require_command osv-scanner

  args=(
    scan
    source
    -r "${SCAN_ROOT}"
    --format json
    --output "${OUTPUT_DIR}/osv-scanner.json"
  )
  read -r -a excludes <<<"${OSV_SCAN_EXCLUDES}"
  for exclude in "${excludes[@]}"; do
    if [[ -n "${exclude}" ]]; then
      args+=(--experimental-exclude "${exclude}")
    fi
  done

  log "running osv-scanner"
  if osv-scanner "${args[@]}" \
    >"${OUTPUT_DIR}/osv-scanner.log" \
    2>&1; then
    status=0
  else
    status=$?
  fi

  if [[ ${status} -eq 0 ]]; then
    record_result osv_scanner passed
  else
    record_result osv_scanner "failed:${status}"
  fi

  return "${status}"
}

run_grype() {
  local status=0
  local exclude
  local -a excludes
  local -a args

  require_command grype

  args=(
    "dir:${SCAN_ROOT}"
    --fail-on "${GRYPE_FAIL_ON}"
    -o json
  )
  read -r -a excludes <<<"${GRYPE_SCAN_EXCLUDES}"
  for exclude in "${excludes[@]}"; do
    if [[ -n "${exclude}" ]]; then
      args+=(--exclude "${exclude}")
    fi
  done

  log "running grype"
  if grype "${args[@]}" \
    >"${OUTPUT_DIR}/grype.json" \
    2>"${OUTPUT_DIR}/grype.log"; then
    status=0
  else
    status=$?
  fi

  if [[ ${status} -eq 0 ]]; then
    record_result grype passed
  else
    record_result grype "failed:${status}"
  fi

  return "${status}"
}

run_controlplane_rbac_audit() {
  local status=0

  require_command python3

  log "running controlplane RBAC audit"
  if "${ROOT_DIR}/scripts/audit-controlplane-rbac.sh" --check \
    >"${OUTPUT_DIR}/controlplane-rbac-audit.log" \
    2>&1; then
    status=0
  else
    status=$?
  fi

  if [[ ${status} -eq 0 ]]; then
    record_result controlplane_rbac_audit passed
  else
    record_result controlplane_rbac_audit "failed:${status}"
  fi

  return "${status}"
}

render_production_overlay() {
  require_command kubectl

  log "rendering production overlay"
  kubectl kustomize "${PRODUCTION_OVERLAY_DIR}" >"${PRODUCTION_RENDERED_MANIFEST}"
}

run_kubescape() {
  local status=0

  require_command kubescape

  if [[ ! -f "${KUBESCAPE_EXCEPTIONS}" ]]; then
    log "missing kubescape exceptions file: ${KUBESCAPE_EXCEPTIONS}"
    exit 1
  fi

  render_production_overlay

  log "running kubescape"
  if kubescape scan \
    "${PRODUCTION_RENDERED_MANIFEST}" \
    --exceptions "${KUBESCAPE_EXCEPTIONS}" \
    --severity-threshold "${KUBESCAPE_SEVERITY_THRESHOLD}" \
    --format json \
    --output "${OUTPUT_DIR}/kubescape.json" \
    >"${OUTPUT_DIR}/kubescape.log" \
    2>&1; then
    status=0
  else
    status=$?
  fi

  if [[ ${status} -eq 0 ]]; then
    record_result kubescape passed
  else
    record_result kubescape "failed:${status}"
  fi

  return "${status}"
}

run_dependabot_alert_triage() {
  local status=0
  local -a args

  if [[ "${DEPENDABOT_ALERT_TRIAGE}" != "true" ]]; then
    log "skipping Dependabot alert triage because DEPENDABOT_ALERT_TRIAGE=${DEPENDABOT_ALERT_TRIAGE}"
    record_result dependabot_alert_triage skipped
    return 0
  fi

  args=(
    --repo-root "${ROOT_DIR}"
    --output-dir "${OUTPUT_DIR}/dependabot-alert-triage"
  )
  if [[ -n "${DEPENDABOT_ALERT_TRIAGE_ALERTS_JSON}" ]]; then
    args+=(--alerts-json "${DEPENDABOT_ALERT_TRIAGE_ALERTS_JSON}")
  fi
  if [[ -n "${DEPENDABOT_ALERT_TRIAGE_REPOSITORY}" ]]; then
    args+=(--github-repository "${DEPENDABOT_ALERT_TRIAGE_REPOSITORY}")
  fi

  log "running Dependabot alert triage"
  if "${ROOT_DIR}/scripts/check-dependabot-alert-triage.sh" "${args[@]}" \
    >"${OUTPUT_DIR}/dependabot-alert-triage.log" \
    2>&1; then
    status=0
  else
    status=$?
  fi

  if [[ ${status} -eq 0 ]]; then
    record_result dependabot_alert_triage passed
  else
    record_result dependabot_alert_triage "failed:${status}"
  fi

  return "${status}"
}

write_versions() {
  {
    printf 'timestamp=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'scan_root=%s\n' "${SCAN_ROOT}"
    printf 'osv_scan_excludes=%s\n' "${OSV_SCAN_EXCLUDES}"
    printf 'grype_scan_excludes=%s\n' "${GRYPE_SCAN_EXCLUDES}"
    printf 'production_overlay_dir=%s\n' "${PRODUCTION_OVERLAY_DIR}"
    cargo-audit --version
    osv-scanner --version
    grype version
    kubescape version
    kubectl version --client
    python3 --version
  } >"${OUTPUT_DIR}/versions.txt" 2>&1
}

main() {
  local failed=0

  mkdir -p "${OUTPUT_DIR}"
  : >"${OUTPUT_DIR}/summary.txt"

  require_command cargo-audit
  require_command osv-scanner
  require_command grype
  require_command kubectl
  require_command kubescape
  require_command python3

  write_versions

  if ! run_controlplane_rbac_audit; then
    failed=1
  fi
  if ! run_cargo_audit; then
    failed=1
  fi
  if ! run_dependabot_alert_triage; then
    failed=1
  fi
  if ! run_osv_scanner; then
    failed=1
  fi
  if ! run_grype; then
    failed=1
  fi
  if ! run_kubescape; then
    failed=1
  fi

  if [[ ${failed} -ne 0 ]]; then
    log "one or more security scans failed; see ${OUTPUT_DIR}"
    exit 1
  fi

  log "security scans completed successfully"
  log "artifacts: ${OUTPUT_DIR}"
}

main "$@"
