#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${repo_root}/scripts/lib/common.sh"

usage() {
  cat <<'EOF' >&2
usage: check-stream-route-test-coverage.sh [--repo-root <path>]

Verifies that stream Route coverage evidence is explicit:
- UDPRoute is covered by the archived Gateway API conformance report.
- TCPRoute is covered by supplemental controlplane/dataplane tests and kind smoke.
- UDPRoute is covered by the supplemental kind smoke suite.
- The conformance README documents the TCPRoute upstream conformance gap.
EOF
}

log() {
  printf '[stream-route-coverage] %s\n' "$*"
}

require_contains() {
  local file="$1"
  local expected="$2"
  local label="$3"

  aeg_require_file "${file}"
  if ! grep -Fq -- "${expected}" "${file}"; then
    aeg_fail "${file} is missing ${label}: ${expected}"
  fi
}

require_matches() {
  local file="$1"
  local pattern="$2"
  local label="$3"

  aeg_require_file "${file}"
  if ! grep -Eq -- "${pattern}" "${file}"; then
    aeg_fail "${file} is missing ${label}: ${pattern}"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --repo-root"
      }
      repo_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      aeg_usage_error "unknown argument: $1"
      ;;
  esac
done

conformance_log="${repo_root}/reports/conformance/latest/run.log"
conformance_report="${repo_root}/reports/conformance/latest/report.yaml"
conformance_readme="${repo_root}/reports/conformance/README.md"
kind_smoke_manifest="${repo_root}/tests/e2e/smoke.yaml"
kind_smoke_script="${repo_root}/tests/e2e/run-kind.sh"
tcproute_dataplane_tests="${repo_root}/dataplane/crates/aeg-ir/src/tests_selection/stream_and_fallback/stream_routes/tcp.rs"
tcproute_translator_tests="${repo_root}/controlplane/internal/translator/tcproute_conformance_test.go"
tcproute_status_tests="${repo_root}/controlplane/internal/status/tcproute_conformance_test.go"

aeg_require_file "${conformance_log}"
aeg_require_file "${conformance_report}"
aeg_require_file "${conformance_readme}"
aeg_require_file "${kind_smoke_manifest}"
aeg_require_file "${kind_smoke_script}"
aeg_require_file "${tcproute_dataplane_tests}"
aeg_require_file "${tcproute_translator_tests}"
aeg_require_file "${tcproute_status_tests}"

require_matches \
  "${conformance_log}" \
  '^[[:space:]]*--- PASS: TestGatewayAPIConformance/UDPRoute([[:space:]/(]|$)' \
  "official UDPRoute conformance PASS evidence"
require_contains \
  "${conformance_report}" \
  "- UDPRoute" \
  "UDPRoute conformance report entry"

require_contains "${kind_smoke_manifest}" "protocol: TCP" "TCP listener smoke manifest"
require_contains "${kind_smoke_manifest}" "kind: TCPRoute" "TCPRoute smoke manifest"
require_contains "${kind_smoke_manifest}" "name: tcp-echo" "TCPRoute success smoke route"
require_contains "${kind_smoke_manifest}" "name: tcp-missing" "TCPRoute missing-backend smoke route"
require_contains "${kind_smoke_manifest}" "protocol: UDP" "UDP listener smoke manifest"
require_contains "${kind_smoke_manifest}" "kind: UDPRoute" "UDPRoute smoke manifest"
require_contains "${kind_smoke_manifest}" "name: udp-coredns" "UDPRoute success smoke route"
require_contains "${kind_smoke_manifest}" "name: udp-missing" "UDPRoute missing-backend smoke route"

require_contains "${kind_smoke_script}" "probe_tcp()" "TCP success probe function"
require_contains "${kind_smoke_script}" "probe_udp()" "UDP success probe function"
require_contains "${kind_smoke_script}" "probe_tcp_missing_backend()" "TCP missing-backend probe function"
require_contains "${kind_smoke_script}" "probe_udp_missing_backend()" "UDP missing-backend probe function"
require_contains "${kind_smoke_script}" 'retry_probe "tcp" 30 2 probe_tcp' "TCP success smoke execution"
require_contains "${kind_smoke_script}" 'retry_probe "udp" 30 2 probe_udp' "UDP success smoke execution"
require_contains "${kind_smoke_script}" 'retry_probe "tcp missing backend" 10 1 probe_tcp_missing_backend' "tcp missing backend smoke execution"
require_contains "${kind_smoke_script}" 'retry_probe "udp missing backend" 10 1 probe_udp_missing_backend' "udp missing backend smoke execution"

require_contains "${tcproute_dataplane_tests}" "tcproute_selects_backend_by_attached_tcp_listener" "TCPRoute dataplane selection test"
require_contains "${tcproute_dataplane_tests}" "tcproute_does_not_match_udp_listener_even_when_attached" "TCPRoute protocol isolation test"
require_contains "${tcproute_dataplane_tests}" "tcproute_rule_port_must_match_listener_port" "TCPRoute listener port match test"
require_contains "${tcproute_dataplane_tests}" "tcproute_returns_none_for_missing_backend_cluster" "TCPRoute missing backend dataplane test"
require_contains "${tcproute_dataplane_tests}" "tcproute_selects_only_healthy_backend_endpoint" "TCPRoute healthy endpoint dataplane test"

require_contains "${tcproute_translator_tests}" "TestBuildMarksTCPRouteCrossNamespaceBackendWithoutGrant" "TCPRoute translator ReferenceGrant rejection test"
require_contains "${tcproute_translator_tests}" "TestBuildAllowsTCPRouteCrossNamespaceBackendWithReferenceGrant" "TCPRoute translator ReferenceGrant allow test"
require_contains "${tcproute_status_tests}" "TestReconcileTCPRouteMarksCrossNamespaceBackendRefAsNotPermitted" "TCPRoute status ReferenceGrant rejection test"

require_contains \
  "${conformance_readme}" \
  "official Gateway API v1.5.1 conformance harness covers UDPRoute" \
  "UDPRoute official conformance documentation"
require_contains \
  "${conformance_readme}" \
  "TCPRoute has no upstream conformance feature in v1.5.1" \
  "TCPRoute upstream conformance gap documentation"
require_contains \
  "${conformance_readme}" \
  "supplemental kind smoke" \
  "supplemental kind smoke documentation"
require_contains \
  "${conformance_readme}" \
  "supplemental TCPRoute conformance-style" \
  "supplemental TCPRoute conformance-style documentation"

log "official conformance UDPRoute evidence: ${conformance_log}"
log "supplemental TCPRoute dataplane evidence: ${tcproute_dataplane_tests}"
log "supplemental TCPRoute controlplane evidence: ${tcproute_translator_tests}, ${tcproute_status_tests}"
log "supplemental TCPRoute/UDPRoute kind smoke evidence: ${kind_smoke_script}"
log "stream route test coverage evidence verified"
