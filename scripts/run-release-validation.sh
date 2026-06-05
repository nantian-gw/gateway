#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.5.1}"
REPORT_DIR="${REPORT_DIR:-${ROOT_DIR}/tmp/conformance}"
REPORT_OUTPUT="${REPORT_OUTPUT:-${REPORT_DIR}/report-${GATEWAY_API_VERSION}.yaml}"
LOG_PATH="${LOG_PATH:-${REPORT_DIR}/run.log}"
IMPLEMENTATION_VERSION="${IMPLEMENTATION_VERSION:-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)}"
ARCHIVE_REPORT_ID="${ARCHIVE_REPORT_ID:-}"
SKIP_PROTO="${SKIP_PROTO:-false}"
SKIP_SKEW_VALIDATION="${SKIP_SKEW_VALIDATION:-false}"
SKIP_SECURITY_SCANS="${SKIP_SECURITY_SCANS:-false}"
SKIP_CONTROLPLANE_TESTS="${SKIP_CONTROLPLANE_TESTS:-false}"
SKIP_DATAPLANE_TESTS="${SKIP_DATAPLANE_TESTS:-false}"
SKIP_KIND="${SKIP_KIND:-false}"
SKIP_SUPPORT_MATRIX_CHECK="${SKIP_SUPPORT_MATRIX_CHECK:-false}"
SKIP_METRICS_SURFACES="${SKIP_METRICS_SURFACES:-false}"
SKIP_STATUS_SURFACES="${SKIP_STATUS_SURFACES:-false}"
SKIP_ADMIN_TOKEN_ROTATION="${SKIP_ADMIN_TOKEN_ROTATION:-false}"
SKIP_XDS_MTLS_ROTATION="${SKIP_XDS_MTLS_ROTATION:-false}"
SKIP_TLS_ASSET_ROTATION="${SKIP_TLS_ASSET_ROTATION:-false}"
SKIP_BACKEND_PROTOCOLS="${SKIP_BACKEND_PROTOCOLS:-false}"
SKIP_GATEWAY_CERT_REFS="${SKIP_GATEWAY_CERT_REFS:-false}"
SKIP_GRPC_REFERENCE_GRANTS="${SKIP_GRPC_REFERENCE_GRANTS:-false}"
SKIP_REFERENCE_GRANTS="${SKIP_REFERENCE_GRANTS:-false}"
SKIP_UPSTREAM_BEHAVIOR="${SKIP_UPSTREAM_BEHAVIOR:-false}"
SKIP_SESSION_PERSISTENCE="${SKIP_SESSION_PERSISTENCE:-false}"
SKIP_HTTP_SECURITY="${SKIP_HTTP_SECURITY:-false}"
SKIP_MESH_FRONTENDS="${SKIP_MESH_FRONTENDS:-false}"
SKIP_CONFORMANCE="${SKIP_CONFORMANCE:-false}"
REQUIRE_RELEASE_EVIDENCE="${REQUIRE_RELEASE_EVIDENCE:-false}"
RELEASE_EVIDENCE_CANDIDATE="${RELEASE_EVIDENCE_CANDIDATE:-}"
RELEASE_EVIDENCE_ALLOW_COMMITS="${RELEASE_EVIDENCE_ALLOW_COMMITS:-}"
RELEASE_EVIDENCE_CONFORMANCE_RUN="${RELEASE_EVIDENCE_CONFORMANCE_RUN:-}"
RELEASE_EVIDENCE_PERFORMANCE_RUN="${RELEASE_EVIDENCE_PERFORMANCE_RUN:-}"
RELEASE_EVIDENCE_CHAOS_RUN="${RELEASE_EVIDENCE_CHAOS_RUN:-}"
RELEASE_EVIDENCE_SOAK_RUN="${RELEASE_EVIDENCE_SOAK_RUN:-}"
SKIP_RELEASE_EVIDENCE_ALIGNMENT="${SKIP_RELEASE_EVIDENCE_ALIGNMENT:-false}"
ALL_FEATURES="${ALL_FEATURES:-true}"
BACKEND_PROTOCOLS_ENSURE_KIND="${BACKEND_PROTOCOLS_ENSURE_KIND:-false}"
GATEWAY_CERT_REFS_ENSURE_KIND="${GATEWAY_CERT_REFS_ENSURE_KIND:-false}"
GRPC_REFERENCE_GRANTS_ENSURE_KIND="${GRPC_REFERENCE_GRANTS_ENSURE_KIND:-false}"
REFERENCE_GRANTS_ENSURE_KIND="${REFERENCE_GRANTS_ENSURE_KIND:-false}"
UPSTREAM_BEHAVIOR_ENSURE_KIND="${UPSTREAM_BEHAVIOR_ENSURE_KIND:-false}"
SESSION_PERSISTENCE_ENSURE_KIND="${SESSION_PERSISTENCE_ENSURE_KIND:-false}"
XDS_MTLS_ROTATION_ENSURE_KIND="${XDS_MTLS_ROTATION_ENSURE_KIND:-false}"
TLS_ASSET_ROTATION_ENSURE_KIND="${TLS_ASSET_ROTATION_ENSURE_KIND:-false}"
SKEW_VALIDATION_BASE_REF="${SKEW_VALIDATION_BASE_REF:-}"
RAN_CONFORMANCE="false"

log() {
  printf '[release-validation] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

run_proto() {
  require_command make
  log "generating proto bindings"
  (
    cd "${ROOT_DIR}"
    make proto
  )
}

run_controlplane_tests() {
  require_command go
  log "running controlplane tests"
  (
    cd "${ROOT_DIR}/controlplane"
    go test ./...
  )
}

run_security_scans() {
  require_command bash
  require_command cargo-audit
  require_command grype
  require_command kubectl
  require_command kubescape
  require_command osv-scanner
  log "running security scan bundle"
  (
    cd "${ROOT_DIR}"
    ./scripts/run-security-scans.sh
  )
}

run_skew_validation() {
  require_command bash
  require_command git
  require_command go
  require_command protoc
  log "running controlplane/dataplane/proto skew validation"
  (
    cd "${ROOT_DIR}"
    SKIP_PROTO=true \
    SKIP_CONTROLPLANE_TESTS=true \
    SKIP_DATAPLANE_TESTS=true \
    COMPAT_BASE_REF="${SKEW_VALIDATION_BASE_REF}" \
    MIXED_VERSION_VALIDATE=true \
    MIXED_VERSION_BASE_REF="${SKEW_VALIDATION_BASE_REF}" \
    ./scripts/run-skew-validation.sh
  )
}

run_dataplane_tests() {
  require_command cargo
  log "running dataplane workspace tests"
  (
    cd "${ROOT_DIR}"
    cargo test --manifest-path dataplane/Cargo.toml --workspace
  )
}

run_kind_smoke() {
  require_command kubectl
  require_command kind
  require_command docker
  require_command jq
  require_command socat
  log "running kind smoke validation"
  (
    cd "${ROOT_DIR}"
    ./tests/e2e/run-kind.sh
  )
}

run_support_matrix_check() {
  require_command bash
  require_command go
  require_command python3
  log "checking generated gateway api support matrix"
  (
    cd "${ROOT_DIR}"
    ./scripts/update-gateway-api-support.sh --check
  )
}

run_status_surface_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command jq
  require_command ss
  log "running status and admin surface validation"
  (
    cd "${ROOT_DIR}"
    ./tests/e2e/validate-status-surfaces.sh
  )
}

run_admin_token_rotation_validation() {
  require_command curl
  require_command diff
  require_command jq
  require_command kind
  require_command kubectl
  require_command ss
  log "running admin token rotation validation"
  (
    cd "${ROOT_DIR}"
    ./tests/e2e/validate-admin-token-rotation.sh
  )
}

run_xds_mtls_rotation_validation() {
  require_command awk
  require_command curl
  require_command diff
  require_command jq
  require_command kind
  require_command kubectl
  require_command openssl
  require_command sha256sum
  require_command ss
  log "running xDS mTLS rotation validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${XDS_MTLS_ROTATION_ENSURE_KIND}" \
    ./tests/e2e/validate-xds-mtls-rotation.sh
  )
}

run_tls_asset_rotation_validation() {
  require_command curl
  require_command diff
  require_command docker
  require_command jq
  require_command kind
  require_command kubectl
  require_command openssl
  log "running TLS asset rotation validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${TLS_ASSET_ROTATION_ENSURE_KIND}" \
    ./tests/e2e/validate-tls-asset-rotation.sh
  )
}

run_metrics_surface_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command jq
  require_command python3
  require_command ss
  log "running live metrics surface validation"
  (
    cd "${ROOT_DIR}"
    ./tests/e2e/validate-metrics-surfaces.sh
  )
}

run_backend_protocol_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command docker
  require_command go
  require_command jq
  require_command python3
  log "running backend protocol validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${BACKEND_PROTOCOLS_ENSURE_KIND}" \
    ./tests/e2e/validate-backend-protocols.sh
  )
}

run_gateway_cert_ref_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command jq
  require_command openssl
  log "running gateway certificate reference validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${GATEWAY_CERT_REFS_ENSURE_KIND}" \
    ./tests/e2e/validate-gateway-cross-namespace-certs.sh
  )
}

run_grpc_reference_grants_validation() {
  require_command kubectl
  require_command kind
  require_command docker
  require_command go
  require_command jq
  log "running grpc reference grant validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${GRPC_REFERENCE_GRANTS_ENSURE_KIND}" \
    ./tests/e2e/validate-grpc-reference-grants.sh
  )
}

run_upstream_behavior_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command docker
  require_command jq
  log "running upstream behavior validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${UPSTREAM_BEHAVIOR_ENSURE_KIND}" \
    ./tests/e2e/validate-upstream-behavior.sh
  )
}

run_reference_grants_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command docker
  require_command jq
  log "running reference grant validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${REFERENCE_GRANTS_ENSURE_KIND}" \
    ./tests/e2e/validate-reference-grants.sh
  )
}

run_session_persistence_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command jq
  log "running session persistence validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND="${SESSION_PERSISTENCE_ENSURE_KIND}" \
    ./tests/e2e/validate-session-persistence.sh
  )
}

run_http_security_validation() {
  require_command awk
  require_command curl
  require_command docker
  require_command jq
  require_command kind
  require_command kubectl
  require_command python3
  require_command socat
  require_command ss
  log "running http security validation"
  (
    cd "${ROOT_DIR}"
    ./tests/e2e/validate-http-security.sh
  )
}

run_mesh_frontend_validation() {
  require_command kubectl
  require_command kind
  require_command curl
  require_command docker
  require_command jq
  log "running mesh frontend validation"
  (
    cd "${ROOT_DIR}"
    ENSURE_KIND=false \
    ./tests/e2e/validate-mesh-frontends.sh
  )
}

run_release_evidence_validation() {
  require_command bash
  require_command python3

  local candidate="${RELEASE_EVIDENCE_CANDIDATE:-${IMPLEMENTATION_VERSION}}"
  local refresh_args=(
    --candidate "${candidate}"
    --check-only
  )

  if [[ -n "${RELEASE_EVIDENCE_ALLOW_COMMITS}" ]]; then
    local allow_commit
    local allow_commits=()
    read -r -a allow_commits <<<"${RELEASE_EVIDENCE_ALLOW_COMMITS}"
    for allow_commit in "${allow_commits[@]}"; do
      [[ -n "${allow_commit}" ]] || continue
      refresh_args+=(--allow-commit "${allow_commit}")
    done
  fi

  if [[ -n "${RELEASE_EVIDENCE_CONFORMANCE_RUN}" ]]; then
    refresh_args+=(--conformance-run "${RELEASE_EVIDENCE_CONFORMANCE_RUN}")
  fi
  if [[ -n "${RELEASE_EVIDENCE_PERFORMANCE_RUN}" ]]; then
    refresh_args+=(--performance-run "${RELEASE_EVIDENCE_PERFORMANCE_RUN}")
  fi
  if [[ -n "${RELEASE_EVIDENCE_CHAOS_RUN}" ]]; then
    refresh_args+=(--chaos-run "${RELEASE_EVIDENCE_CHAOS_RUN}")
  fi
  if [[ -n "${RELEASE_EVIDENCE_SOAK_RUN}" ]]; then
    refresh_args+=(--soak-run "${RELEASE_EVIDENCE_SOAK_RUN}")
  fi

  log "running release evidence validation for candidate ${candidate}"
  (
    cd "${ROOT_DIR}"
    ./scripts/refresh-release-evidence.sh "${refresh_args[@]}"
  )

  if [[ "${SKIP_RELEASE_EVIDENCE_ALIGNMENT}" != "true" ]]; then
    log "checking release evidence reference alignment"
    (
      cd "${ROOT_DIR}"
      ./scripts/check-evidence-reference-alignment.sh
    )
  fi
}

archive_conformance_report() {
  local result_status="$1"

  if [[ -z "${ARCHIVE_REPORT_ID}" || ! -f "${REPORT_OUTPUT}" ]]; then
    return
  fi

  log "archiving conformance report as ${ARCHIVE_REPORT_ID}"
  (
    cd "${ROOT_DIR}"
    RESULT_STATUS="${result_status}" \
    SOURCE_COMMAND="ALL_FEATURES=${ALL_FEATURES} IMPLEMENTATION_VERSION=${IMPLEMENTATION_VERSION} REPORT_OUTPUT=${REPORT_OUTPUT} ./tests/conformance/run.sh" \
    scripts/archive-conformance-report.sh \
      "${ARCHIVE_REPORT_ID}" \
      "${REPORT_OUTPUT}" \
      "${LOG_PATH}"
  )
}

run_conformance() {
  local status

  require_command go
  require_command kubectl
  require_command docker
  require_command jq
  mkdir -p "${REPORT_DIR}"

  log "running conformance with ALL_FEATURES=${ALL_FEATURES}"
  RAN_CONFORMANCE="true"
  set +e
  (
    cd "${ROOT_DIR}"
    ALL_FEATURES="${ALL_FEATURES}" \
    IMPLEMENTATION_VERSION="${IMPLEMENTATION_VERSION}" \
    REPORT_OUTPUT="${REPORT_OUTPUT}" \
    ./tests/conformance/run.sh
  ) 2>&1 | tee "${LOG_PATH}"
  status=${PIPESTATUS[0]}
  set -e

  if [[ ${status} -eq 0 ]]; then
    archive_conformance_report "passed"
  else
    archive_conformance_report "failed"
  fi

  return "${status}"
}

main() {
  if [[ "${SKIP_PROTO}" != "true" ]]; then
    run_proto
  fi
  if [[ "${SKIP_SECURITY_SCANS}" != "true" ]]; then
    run_security_scans
  fi
  if [[ "${SKIP_CONTROLPLANE_TESTS}" != "true" ]]; then
    run_controlplane_tests
  fi
  if [[ "${SKIP_DATAPLANE_TESTS}" != "true" ]]; then
    run_dataplane_tests
  fi
  if [[ "${SKIP_SKEW_VALIDATION}" != "true" ]]; then
    run_skew_validation
  fi
  if [[ "${SKIP_KIND}" != "true" ]]; then
    run_kind_smoke
  fi
  if [[ "${SKIP_SUPPORT_MATRIX_CHECK}" != "true" ]]; then
    run_support_matrix_check
  fi
  if [[ "${SKIP_METRICS_SURFACES}" != "true" ]]; then
    run_metrics_surface_validation
  fi
  if [[ "${SKIP_STATUS_SURFACES}" != "true" ]]; then
    run_status_surface_validation
  fi
  if [[ "${SKIP_ADMIN_TOKEN_ROTATION}" != "true" ]]; then
    run_admin_token_rotation_validation
  fi
  if [[ "${SKIP_XDS_MTLS_ROTATION}" != "true" ]]; then
    run_xds_mtls_rotation_validation
  fi
  if [[ "${SKIP_TLS_ASSET_ROTATION}" != "true" ]]; then
    run_tls_asset_rotation_validation
  fi
  if [[ "${SKIP_BACKEND_PROTOCOLS}" != "true" ]]; then
    run_backend_protocol_validation
  fi
  if [[ "${SKIP_GATEWAY_CERT_REFS}" != "true" ]]; then
    run_gateway_cert_ref_validation
  fi
  if [[ "${SKIP_GRPC_REFERENCE_GRANTS}" != "true" ]]; then
    run_grpc_reference_grants_validation
  fi
  if [[ "${SKIP_REFERENCE_GRANTS}" != "true" ]]; then
    run_reference_grants_validation
  fi
  if [[ "${SKIP_UPSTREAM_BEHAVIOR}" != "true" ]]; then
    run_upstream_behavior_validation
  fi
  if [[ "${SKIP_SESSION_PERSISTENCE}" != "true" ]]; then
    run_session_persistence_validation
  fi
  if [[ "${SKIP_HTTP_SECURITY}" != "true" ]]; then
    run_http_security_validation
  fi
  if [[ "${SKIP_MESH_FRONTENDS}" != "true" ]]; then
    run_mesh_frontend_validation
  fi
  if [[ "${SKIP_CONFORMANCE}" != "true" ]]; then
    run_conformance
  fi
  if [[ "${REQUIRE_RELEASE_EVIDENCE}" == "true" ]]; then
    run_release_evidence_validation
  fi

  log "release validation completed"
  if [[ "${RAN_CONFORMANCE}" == "true" && -f "${REPORT_OUTPUT}" ]]; then
    log "conformance report: ${REPORT_OUTPUT}"
  fi
  if [[ "${RAN_CONFORMANCE}" == "true" && -f "${LOG_PATH}" ]]; then
    log "conformance log: ${LOG_PATH}"
  fi
}

main "$@"
