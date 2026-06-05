#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${ROOT_DIR}/scripts/lib/common.sh"

RUN_ID="${RUN_ID:-$(date +%Y-%m-%d-%H%M%S)-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)-controlplane-status-bench}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/tmp/controlplane-status-bench/${RUN_ID}}"
ARCHIVE_REPORTS="${ARCHIVE_REPORTS:-false}"
ARCHIVE_ROOT="${ARCHIVE_ROOT:-${ROOT_DIR}/reports/performance/runs}"
ARCHIVE_DIR="${ARCHIVE_DIR:-${ARCHIVE_ROOT}/${RUN_ID}}"
BENCH_COUNT="${BENCH_COUNT:-3}"
BENCH_TIME="${BENCH_TIME:-3x}"
BENCH_CPU="${BENCH_CPU:-1}"
STATUS_BENCH_PATTERN="${STATUS_BENCH_PATTERN:-Benchmark(ReconcileFullStatus(RouteFanout|AttachDetachStorm)|EvaluateRoutesRouteFanout|EvaluateGatewaysGatewayFleet|MergeRouteParents|MergeListenerStatuses|EvaluateBackendPolicyFanout)$}"
CONTROLLER_BENCH_PATTERN="${CONTROLLER_BENCH_PATTERN:-Benchmark(PublishSnapshotRouteFanout|PublishSnapshotAttachDetachStorm|SnapshotInputStatusStorm|ReconcileEndpointSliceBackendStorm)$}"
TRANSLATOR_BENCH_PATTERN="${TRANSLATOR_BENCH_PATTERN:-Benchmark(BuildSnapshotRouteFanout|BuildSnapshotAttachDetachStorm|BuildBackendsForSnapshotEndpointSliceStorm)$}"
INFRASTRUCTURE_BENCH_PATTERN="${INFRASTRUCTURE_BENCH_PATTERN:-Benchmark(InspectInfrastructureRouteFanout|InspectInfrastructureAttachDetachStorm|ReconcileMeshServicesRouteFanout|ReconcileMeshServicesAttachDetachStorm)$}"
ADMIN_BENCH_PATTERN="${ADMIN_BENCH_PATTERN:-Benchmark(FilterRoutesQueryRouteFanout|FilterBackendsQueryRouteFanout|ListResourcesQueryPaths|ListServiceCatalogQueryPaths)$}"
STATUS_RAW_OUTPUT="${OUTPUT_DIR}/status-bench.txt"
CONTROLLER_RAW_OUTPUT="${OUTPUT_DIR}/controller-bench.txt"
TRANSLATOR_RAW_OUTPUT="${OUTPUT_DIR}/translator-bench.txt"
INFRASTRUCTURE_RAW_OUTPUT="${OUTPUT_DIR}/infrastructure-bench.txt"
ADMIN_RAW_OUTPUT="${OUTPUT_DIR}/admin-bench.txt"
RAW_OUTPUT="${OUTPUT_DIR}/bench.txt"
SUMMARY_OUTPUT="${OUTPUT_DIR}/summary.md"

log() {
  printf '[controlplane-status-bench] %s\n' "$*"
}

write_combined_output() {
  cat >"${RAW_OUTPUT}" <<EOF
=== internal/status ===
$(cat "${STATUS_RAW_OUTPUT}")

=== internal/controller ===
$(cat "${CONTROLLER_RAW_OUTPUT}")

=== internal/translator ===
$(cat "${TRANSLATOR_RAW_OUTPUT}")

=== internal/infrastructure ===
$(cat "${INFRASTRUCTURE_RAW_OUTPUT}")

=== internal/admin ===
$(cat "${ADMIN_RAW_OUTPUT}")
EOF
}

write_metadata() {
  local metadata="${OUTPUT_DIR}/metadata.txt"
  local git_tree_state
  local code_tree_state
  git_tree_state="$(aeg_git_tree_state "${ROOT_DIR}")"
  code_tree_state="$(aeg_code_tree_state "${ROOT_DIR}")"
  {
    printf 'captured_at=%s\n' "$(date --iso-8601=seconds)"
    printf 'git_commit=%s\n' "$(git -C "${ROOT_DIR}" rev-parse HEAD)"
    printf 'git_tree_state=%s\n' "${git_tree_state}"
    printf 'code_tree_state=%s\n' "${code_tree_state}"
    printf 'run_id=%s\n' "${RUN_ID}"
    printf 'archive_reports=%s\n' "${ARCHIVE_REPORTS}"
    printf 'go_version=%s\n' "$(go version)"
    printf 'bench_count=%s\n' "${BENCH_COUNT}"
    printf 'bench_time=%s\n' "${BENCH_TIME}"
    printf 'bench_cpu=%s\n' "${BENCH_CPU}"
    printf 'status_bench_pattern=%s\n' "${STATUS_BENCH_PATTERN}"
    printf 'controller_bench_pattern=%s\n' "${CONTROLLER_BENCH_PATTERN}"
    printf 'translator_bench_pattern=%s\n' "${TRANSLATOR_BENCH_PATTERN}"
    printf 'infrastructure_bench_pattern=%s\n' "${INFRASTRUCTURE_BENCH_PATTERN}"
    printf 'admin_bench_pattern=%s\n' "${ADMIN_BENCH_PATTERN}"
    printf 'kernel=%s\n' "$(uname -srmo)"
    printf 'cpu_count=%s\n' "$(nproc)"
    printf 'memory_kib=%s\n' "$(awk '/MemTotal:/ {print $2}' /proc/meminfo)"
  } >"${metadata}"
}

write_summary() {
  cat >"${SUMMARY_OUTPUT}" <<EOF
# Controlplane Benchmark Evidence

- Run ID: \`${RUN_ID}\`
- Git commit: \`$(git -C "${ROOT_DIR}" rev-parse --short HEAD)\`
- Go version: \`$(go version)\`
- Benchmark count: \`${BENCH_COUNT}\`
- Benchmark time: \`${BENCH_TIME}\`
- Benchmark CPU: \`${BENCH_CPU}\`
- Status benchmark pattern: \`${STATUS_BENCH_PATTERN}\`
- Controller benchmark pattern: \`${CONTROLLER_BENCH_PATTERN}\`
- Translator benchmark pattern: \`${TRANSLATOR_BENCH_PATTERN}\`
- Infrastructure benchmark pattern: \`${INFRASTRUCTURE_BENCH_PATTERN}\`
- Admin benchmark pattern: \`${ADMIN_BENCH_PATTERN}\`

## Coverage

- \`BenchmarkReconcileFullStatusRouteFanout\`: full controlplane status reconcile with a growing HTTPRoute set.
- \`BenchmarkReconcileFullStatusAttachDetachStorm\`: repeated attach/detach flips across the same HTTPRoute set to emulate status churn.
- \`BenchmarkEvaluateRoutesRouteFanout\`: pure route status evaluator cost for a preloaded cluster state, excluding fake client, RESTMapper and status update writes.
- \`BenchmarkEvaluateGatewaysGatewayFleet\`: pure Gateway listener/address/convergence evaluator cost for a preloaded Gateway fleet.
- \`BenchmarkMergeRouteParents\`: pure Route parent status merge cost with stale condition cleanup and stable parent ordering.
- \`BenchmarkMergeListenerStatuses\`: pure Gateway listener status merge cost with stale conflict cleanup and stable listener ordering.
- \`BenchmarkEvaluateBackendPolicyFanout\`: pure BackendTLSPolicy and BackendLBPolicy evaluator cost after route evaluation has produced backend ancestors.
- \`BenchmarkPublishSnapshotRouteFanout\`: full snapshot publish path with a growing HTTPRoute set.
- \`BenchmarkPublishSnapshotAttachDetachStorm\`: repeated route attach/detach flips across the same HTTPRoute set through the snapshot publish path.
- \`BenchmarkSnapshotInputStatusStorm\`: status-only route update storms filtered at the snapshot input predicate without rebuilding snapshots.
- \`BenchmarkReconcileEndpointSliceBackendStorm\`: repeated \`EndpointSlice\` backend address flips reconciled through the keyed backend-only snapshot path under route fanout.
- \`BenchmarkBuildSnapshotRouteFanout\`: isolated translator snapshot build cost with a growing HTTPRoute and backend set.
- \`BenchmarkBuildSnapshotAttachDetachStorm\`: repeated route attach/detach flips across the same HTTPRoute and backend set through the translator full build path.
- \`BenchmarkBuildBackendsForSnapshotEndpointSliceStorm\`: isolated translator partial backend rebuild cost for a referenced \`EndpointSlice\` storm under route fanout.
- \`BenchmarkInspectInfrastructureRouteFanout\`: isolated infrastructure inspection cost with a growing mesh-parent HTTPRoute and Service set.
- \`BenchmarkInspectInfrastructureAttachDetachStorm\`: repeated mesh-parent route attach/detach flips across the same HTTPRoute and Service set through the infrastructure inspect path.
- \`BenchmarkReconcileMeshServicesRouteFanout\`: isolated mesh Service infrastructure reconcile cost with a growing mesh-parent HTTPRoute, Service and source EndpointSlice set.
- \`BenchmarkReconcileMeshServicesAttachDetachStorm\`: repeated mesh Service attach/detach flips across the same HTTPRoute and Service set through the infrastructure reconcile path.
- \`BenchmarkFilterRoutesQueryRouteFanout\`: snapshot-backed admin route query cost with sorting and pagination under growing HTTPRoute fanout.
- \`BenchmarkFilterBackendsQueryRouteFanout\`: snapshot-backed admin backend query cost, including referenced-backend filtering, sorting and pagination under route fanout.
- \`BenchmarkListResourcesQueryPaths\`: resource-manager-backed admin resource query cost for kind-scoped cache miss, cache hit and exact-match fast path.
- \`BenchmarkListServiceCatalogQueryPaths\`: service-catalog query cost for namespace-scoped cache miss, cache hit and exact-match direct-Get path.

## Artifact Contract

- \`metadata.txt\`: run metadata, git commit/tree state and benchmark knobs.
- \`status-bench.txt\`: raw \`internal/status\` benchmark output.
- \`controller-bench.txt\`: raw \`internal/controller\` benchmark output.
- \`translator-bench.txt\`: raw \`internal/translator\` benchmark output.
- \`infrastructure-bench.txt\`: raw \`internal/infrastructure\` benchmark output.
- \`admin-bench.txt\`: raw \`internal/admin\` benchmark output.
- \`bench.txt\`: combined raw output for quick sharing.
- \`summary.md\`: operator-facing summary.
EOF
}

verify_outputs() {
  local required=(
    "${OUTPUT_DIR}/metadata.txt"
    "${STATUS_RAW_OUTPUT}"
    "${CONTROLLER_RAW_OUTPUT}"
    "${TRANSLATOR_RAW_OUTPUT}"
    "${INFRASTRUCTURE_RAW_OUTPUT}"
    "${ADMIN_RAW_OUTPUT}"
    "${RAW_OUTPUT}"
    "${SUMMARY_OUTPUT}"
  )
  local path
  for path in "${required[@]}"; do
    if [[ ! -f "${path}" ]]; then
      log "missing expected artifact: ${path}"
      exit 1
    fi
  done
}

archive_outputs() {
  if [[ "${ARCHIVE_REPORTS}" != "true" ]]; then
    return
  fi

  mkdir -p "${ARCHIVE_DIR}"
  cp "${OUTPUT_DIR}/metadata.txt" "${ARCHIVE_DIR}/metadata.txt"
  cp "${STATUS_RAW_OUTPUT}" "${ARCHIVE_DIR}/status-bench.txt"
  cp "${CONTROLLER_RAW_OUTPUT}" "${ARCHIVE_DIR}/controller-bench.txt"
  cp "${TRANSLATOR_RAW_OUTPUT}" "${ARCHIVE_DIR}/translator-bench.txt"
  cp "${INFRASTRUCTURE_RAW_OUTPUT}" "${ARCHIVE_DIR}/infrastructure-bench.txt"
  cp "${ADMIN_RAW_OUTPUT}" "${ARCHIVE_DIR}/admin-bench.txt"
  cp "${RAW_OUTPUT}" "${ARCHIVE_DIR}/bench.txt"
  cp "${SUMMARY_OUTPUT}" "${ARCHIVE_DIR}/summary.md"
}

main() {
  mkdir -p "${OUTPUT_DIR}"
  write_metadata

  log "running controlplane status benchmarks"
  (
    cd "${ROOT_DIR}/controlplane"
    go test ./internal/status \
      -run '^$' \
      -bench "${STATUS_BENCH_PATTERN}" \
      -benchmem \
      -count "${BENCH_COUNT}" \
      -benchtime "${BENCH_TIME}" \
      -cpu "${BENCH_CPU}"
  ) | tee "${STATUS_RAW_OUTPUT}"

  log "running controlplane snapshot benchmarks"
  (
    cd "${ROOT_DIR}/controlplane"
    go test ./internal/controller \
      -run '^$' \
      -bench "${CONTROLLER_BENCH_PATTERN}" \
      -benchmem \
      -count "${BENCH_COUNT}" \
      -benchtime "${BENCH_TIME}" \
      -cpu "${BENCH_CPU}"
  ) | tee "${CONTROLLER_RAW_OUTPUT}"

  log "running controlplane translator benchmarks"
  (
    cd "${ROOT_DIR}/controlplane"
    go test ./internal/translator \
      -run '^$' \
      -bench "${TRANSLATOR_BENCH_PATTERN}" \
      -benchmem \
      -count "${BENCH_COUNT}" \
      -benchtime "${BENCH_TIME}" \
      -cpu "${BENCH_CPU}"
  ) | tee "${TRANSLATOR_RAW_OUTPUT}"

  log "running controlplane infrastructure benchmarks"
  (
    cd "${ROOT_DIR}/controlplane"
    go test ./internal/infrastructure \
      -run '^$' \
      -bench "${INFRASTRUCTURE_BENCH_PATTERN}" \
      -benchmem \
      -count "${BENCH_COUNT}" \
      -benchtime "${BENCH_TIME}" \
      -cpu "${BENCH_CPU}"
  ) | tee "${INFRASTRUCTURE_RAW_OUTPUT}"

  log "running controlplane admin benchmarks"
  (
    cd "${ROOT_DIR}/controlplane"
    go test ./internal/admin \
      -run '^$' \
      -bench "${ADMIN_BENCH_PATTERN}" \
      -benchmem \
      -count "${BENCH_COUNT}" \
      -benchtime "${BENCH_TIME}" \
      -cpu "${BENCH_CPU}"
  ) | tee "${ADMIN_RAW_OUTPUT}"

  write_combined_output
  write_summary
  verify_outputs
  archive_outputs
  log "benchmark evidence written to ${OUTPUT_DIR}"
  if [[ "${ARCHIVE_REPORTS}" == "true" ]]; then
    log "archived benchmark evidence written to ${ARCHIVE_DIR}"
  fi
}

main "$@"
