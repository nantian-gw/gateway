#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${ROOT_DIR}/scripts/lib/common.sh"

RUN_ID="${RUN_ID:-$(date +%Y-%m-%d-%H%M%S)-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)-dataplane-reload-bench}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/tmp/dataplane-reload-bench/${RUN_ID}}"
ARCHIVE_REPORTS="${ARCHIVE_REPORTS:-false}"
ARCHIVE_ROOT="${ARCHIVE_ROOT:-${ROOT_DIR}/reports/performance/runs}"
ARCHIVE_DIR="${ARCHIVE_DIR:-${ARCHIVE_ROOT}/${RUN_ID}}"
ITERATIONS="${ITERATIONS:-25}"
CARGO_PROFILE="${CARGO_PROFILE:-release}"
CARGO_TOOLCHAIN="${CARGO_TOOLCHAIN:-}"
ALLOCATOR="${ALLOCATOR:-system}"
SNAPSHOT_LISTENERS="${SNAPSHOT_LISTENERS:-24}"
ROUTES_PER_LISTENER="${ROUTES_PER_LISTENER:-16}"
BACKENDS_PER_ROUTE="${BACKENDS_PER_ROUTE:-4}"
ENDPOINTS_PER_BACKEND="${ENDPOINTS_PER_BACKEND:-4}"
TLS_LISTENERS="${TLS_LISTENERS:-32}"
TLS_CA_BUNDLE_VARIANTS="${TLS_CA_BUNDLE_VARIANTS:-4}"
BENCH_OUTPUT="${OUTPUT_DIR}/bench.json"
SUMMARY_OUTPUT="${OUTPUT_DIR}/summary.md"
BENCH_ALLOCATOR=""

log() {
  printf '[dataplane-reload-bench] %s\n' "$*"
}

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

validate_allocator() {
  case "${ALLOCATOR}" in
    system|mimalloc|jemalloc)
      ;;
    *)
      log "unsupported allocator: ${ALLOCATOR} (expected system, mimalloc, or jemalloc)"
      exit 1
      ;;
  esac
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
    printf 'iterations=%s\n' "${ITERATIONS}"
    printf 'cargo_profile=%s\n' "${CARGO_PROFILE}"
    printf 'cargo_toolchain=%s\n' "${CARGO_TOOLCHAIN:-default}"
    printf 'allocator_requested=%s\n' "${ALLOCATOR}"
    printf 'allocator_observed=%s\n' "${BENCH_ALLOCATOR}"
    printf 'snapshot_listeners=%s\n' "${SNAPSHOT_LISTENERS}"
    printf 'routes_per_listener=%s\n' "${ROUTES_PER_LISTENER}"
    printf 'backends_per_route=%s\n' "${BACKENDS_PER_ROUTE}"
    printf 'endpoints_per_backend=%s\n' "${ENDPOINTS_PER_BACKEND}"
    printf 'tls_listeners=%s\n' "${TLS_LISTENERS}"
    printf 'tls_ca_bundle_variants=%s\n' "${TLS_CA_BUNDLE_VARIANTS}"
    printf 'scenario_names=%s\n' "http_route_selection,grpc_route_selection,stream_route_selection,xds_snapshot_parse,large_snapshot_switch,request_meta_header_heavy,request_view_header_heavy,snapshot_read_rwlock,snapshot_read_arc_swap,runtime_index_rebuild_route_only,runtime_index_rebuild_endpoint_only,runtime_index_rebuild_secret_only,header_filter_chain,session_persistence,access_log_disabled_path,access_log_sampled_out_path,access_log_write_path,traffic_observe_reused_topology,traffic_observe_no_route,traffic_observe_backend_topology_4_shards,traffic_observe_backend_topology_64_shards,http_capacity_matrix,stream_tcp_buffer_matrix,stream_udp_dispatcher_distribution,stream_udp_payload_copy,tls_asset_rotation,high_frequency_apply,last_good_fallback"
    printf 'cargo_version=%s\n' "$(cargo --version)"
    printf 'rustc_version=%s\n' "$(rustc --version)"
    printf 'kernel=%s\n' "$(uname -srmo)"
    printf 'cpu_count=%s\n' "$(nproc)"
    printf 'memory_kib=%s\n' "$(awk '/MemTotal:/ {print $2}' /proc/meminfo)"
  } >"${metadata}"
}

scenario_row() {
  local name="$1"
  jq -r --arg name "${name}" '
    .scenarios[]
    | select(.name == $name)
    | "| \(.name) | \(.iterations) | \(((.timing.average_ms * 1000)|round/1000)|tostring) | \(((.timing.p95_ms * 1000)|round/1000)|tostring) | \(((.timing.max_ms * 1000)|round/1000)|tostring) | \((.resource_delta.fd_count // 0)|tostring) | \((.resource_delta.rss_kib // 0)|tostring) |"
  ' "${BENCH_OUTPUT}"
}

write_summary() {
  cat >"${SUMMARY_OUTPUT}" <<EOF
# Dataplane Microbenchmark

- Run ID: \`${RUN_ID}\`
- Git commit: \`$(git -C "${ROOT_DIR}" rev-parse --short HEAD)\`
- Cargo profile: \`${CARGO_PROFILE}\`
- Allocator: \`${BENCH_ALLOCATOR}\`
- Iterations: \`${ITERATIONS}\`
- Snapshot scale: \`${SNAPSHOT_LISTENERS}\` listeners, \`${ROUTES_PER_LISTENER}\` routes/listener, \`${BACKENDS_PER_ROUTE}\` backends/route, \`${ENDPOINTS_PER_BACKEND}\` endpoints/backend
- TLS scale: \`${TLS_LISTENERS}\` listeners, \`${TLS_CA_BUNDLE_VARIANTS}\` CA bundle variants

## Scenario Summary

| Scenario | Iterations | Avg ms | p95 ms | Max ms | FD delta | RSS KiB delta |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
$(scenario_row large_snapshot_switch)
$(scenario_row http_route_selection)
$(scenario_row grpc_route_selection)
$(scenario_row stream_route_selection)
$(scenario_row xds_snapshot_parse)
$(scenario_row request_meta_header_heavy)
$(scenario_row request_view_header_heavy)
$(scenario_row snapshot_read_rwlock)
$(scenario_row snapshot_read_arc_swap)
$(scenario_row runtime_index_rebuild_route_only)
$(scenario_row runtime_index_rebuild_endpoint_only)
$(scenario_row runtime_index_rebuild_secret_only)
$(scenario_row header_filter_chain)
$(scenario_row session_persistence)
$(scenario_row access_log_disabled_path)
$(scenario_row access_log_sampled_out_path)
$(scenario_row access_log_write_path)
$(scenario_row traffic_observe_reused_topology)
$(scenario_row traffic_observe_no_route)
$(scenario_row traffic_observe_backend_topology_4_shards)
$(scenario_row traffic_observe_backend_topology_64_shards)
$(scenario_row http_capacity_matrix)
$(scenario_row stream_tcp_buffer_matrix)
$(scenario_row stream_udp_dispatcher_distribution)
$(scenario_row stream_udp_payload_copy)
$(scenario_row tls_asset_rotation)
$(scenario_row high_frequency_apply)
$(scenario_row last_good_fallback)

## Coverage

- \`http_route_selection\`: HTTP hostname/path match plus backend resolution on a large route table
- \`grpc_route_selection\`: gRPC service/method match plus backend resolution on a large route table
- \`stream_route_selection\`: stream SNI/listener match plus backend resolution on a large route table
- \`xds_snapshot_parse\`: proto \`ConfigSnapshot\` decode into runtime IR with index rebuild
- \`large_snapshot_switch\`: large snapshot clone + runtime-state inheritance + backend selection probe
- \`request_meta_header_heavy\`: owned request metadata materialization from a header-heavy request
- \`request_view_header_heavy\`: request-context capture through the lazy \`RequestView\` path for the same header-heavy request shape
- \`snapshot_read_rwlock\`: shared snapshot read-only hot path through \`Arc<RwLock<Snapshot>>::read\`
- \`snapshot_read_arc_swap\`: experimental shared snapshot read-only hot path through \`ArcSwap<Snapshot>::load\`
- \`runtime_index_rebuild_route_only\`: runtime index rebuild when route inputs change but endpoint/secret inputs remain stable
- \`runtime_index_rebuild_endpoint_only\`: runtime index rebuild when endpoint inputs change but route/secret inputs remain stable
- \`runtime_index_rebuild_secret_only\`: runtime index rebuild when secret inputs change but route/endpoint inputs remain stable
- \`header_filter_chain\`: request/response header modifier chain application on realistic headers
- \`session_persistence\`: session token encode/decode round trip through the real cookie path
- \`access_log_disabled_path\`: disabled access-log fast path without rendering or writer enqueue
- \`access_log_sampled_out_path\`: sampled-out access-log fast path without rendering or writer enqueue
- \`access_log_write_path\`: access log render plus background writer enqueue/flush path
- \`traffic_observe_reused_topology\`: traffic stats hot path with reused topology labels and request histogram updates
- \`traffic_observe_no_route\`: traffic stats hot path for unmatched requests and fallback labels
- \`traffic_observe_backend_topology_4_shards\`: traffic stats hot path with backend topology and 4 shards
- \`traffic_observe_backend_topology_64_shards\`: traffic stats hot path with backend topology and 64 shards
- \`http_capacity_matrix\`: capacity derivation matrix for worker threads, accept concurrency, keepalive pool size, and reuse_port
- \`stream_tcp_buffer_matrix\`: TCP proxy buffer normalization matrix across default and clamp bounds
- \`stream_udp_dispatcher_distribution\`: UDP dispatcher worker and session shard distribution across client keys
- \`stream_udp_payload_copy\`: UDP payload copy hot path with a representative datagram size
- \`tls_asset_rotation\`: repeated TLS asset materialization, CA bundle rotation, and stale asset cleanup
- \`high_frequency_apply\`: repeated xDS apply-success path and status report generation
- \`last_good_fallback\`: repeated current-version rejection while preserving last-good readiness semantics

## Artifact Contract

- \`metadata.txt\`: run metadata, scale knobs, git commit, tree state and environment summary
- \`bench.json\`: machine-readable allocator selection, scenario timings and resource deltas
- \`summary.md\`: operator-facing benchmark summary
EOF
}

load_bench_allocator() {
  BENCH_ALLOCATOR="$(jq -r '.allocator // empty' "${BENCH_OUTPUT}")"
  if [[ -z "${BENCH_ALLOCATOR}" ]]; then
    log "benchmark report did not expose allocator"
    exit 1
  fi
}

verify_bench_allocator() {
  if [[ "${BENCH_ALLOCATOR}" != "${ALLOCATOR}" ]]; then
    log "allocator mismatch: requested ${ALLOCATOR}, observed ${BENCH_ALLOCATOR}"
    exit 1
  fi
}

verify_outputs() {
  local required=("${OUTPUT_DIR}/metadata.txt" "${BENCH_OUTPUT}" "${SUMMARY_OUTPUT}")
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
  cp "${BENCH_OUTPUT}" "${ARCHIVE_DIR}/bench.json"
  cp "${SUMMARY_OUTPUT}" "${ARCHIVE_DIR}/summary.md"
}

main() {
  local -a cargo_profile_args
  local -a cargo_feature_args
  local -a cargo_cmd

  require_command cargo
  require_command jq
  validate_allocator
  cargo_cmd=(cargo)
  if [[ -n "${CARGO_TOOLCHAIN}" ]]; then
    cargo_cmd+=("+${CARGO_TOOLCHAIN}")
  fi

  if [[ "${CARGO_PROFILE}" == "release" ]]; then
    cargo_profile_args=(--release)
  else
    cargo_profile_args=(--profile "${CARGO_PROFILE}")
  fi

  case "${ALLOCATOR}" in
    mimalloc)
      cargo_feature_args=(--features allocator-jemalloc)
      ;;
    jemalloc)
      cargo_feature_args=(--features allocator-jemalloc)
      ;;
    *)
      cargo_feature_args=()
      ;;
  esac

  mkdir -p "${OUTPUT_DIR}"

  log "running dataplane microbenchmark suite"
  (
    cd "${ROOT_DIR}"
    "${cargo_cmd[@]}" run --manifest-path dataplane/Cargo.toml -p aeg-bench "${cargo_profile_args[@]}" "${cargo_feature_args[@]}" -- \
      --output "${BENCH_OUTPUT}" \
      --iterations "${ITERATIONS}" \
      --snapshot-listeners "${SNAPSHOT_LISTENERS}" \
      --routes-per-listener "${ROUTES_PER_LISTENER}" \
      --backends-per-route "${BACKENDS_PER_ROUTE}" \
      --endpoints-per-backend "${ENDPOINTS_PER_BACKEND}" \
      --tls-listeners "${TLS_LISTENERS}" \
      --tls-ca-bundle-variants "${TLS_CA_BUNDLE_VARIANTS}"
  )

  load_bench_allocator
  verify_bench_allocator
  write_metadata
  write_summary
  verify_outputs
  archive_outputs
  log "benchmark evidence written to ${OUTPUT_DIR}"
  if [[ "${ARCHIVE_REPORTS}" == "true" ]]; then
    log "archived benchmark evidence written to ${ARCHIVE_DIR}"
  fi
}

main "$@"
