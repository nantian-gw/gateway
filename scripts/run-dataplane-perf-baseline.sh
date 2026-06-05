#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="${RUN_ID:-$(date +%Y-%m-%d-%H%M%S)-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)-dataplane-perf-baseline}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/tmp/dataplane-perf-baseline/${RUN_ID}}"
ARCHIVE_REPORTS="${ARCHIVE_REPORTS:-false}"
ARCHIVE_ROOT="${ARCHIVE_ROOT:-${ROOT_DIR}/reports/performance/runs}"
ARCHIVE_DIR="${ARCHIVE_DIR:-${ARCHIVE_ROOT}/${RUN_ID}}"
RELOAD_BENCH_SCRIPT="${RELOAD_BENCH_SCRIPT:-${ROOT_DIR}/scripts/run-dataplane-reload-bench.sh}"
PERF_TOOL="${PERF_TOOL:-perf}"
PERF_STAT_EVENTS="${PERF_STAT_EVENTS:-task-clock,cycles,instructions,branches,branch-misses,context-switches,cpu-migrations,page-faults}"
STACKCOLLAPSE_TOOL="${STACKCOLLAPSE_TOOL:-stackcollapse-perf.pl}"
FLAMEGRAPH_TOOL="${FLAMEGRAPH_TOOL:-flamegraph.pl}"
PERF_RECORD_FREQUENCY="${PERF_RECORD_FREQUENCY:-199}"
PERF_REPLAY_DIR="${OUTPUT_DIR}/perf-replay"
BENCH_OUTPUT="${OUTPUT_DIR}/bench.json"
METADATA_OUTPUT="${OUTPUT_DIR}/metadata.txt"
BENCH_SUMMARY_OUTPUT="${OUTPUT_DIR}/bench-summary.md"
SUMMARY_OUTPUT="${OUTPUT_DIR}/summary.md"
PERF_RECORD_OUTPUT="${OUTPUT_DIR}/perf.data"
PERF_STAT_OUTPUT="${OUTPUT_DIR}/perf-stat.txt"
FLAMEGRAPH_OUTPUT="${OUTPUT_DIR}/flamegraph.svg"
FLAMEGRAPH_GENERATED="false"
PERF_VERSION=""

log() {
  printf '[dataplane-perf-baseline] %s\n' "$*"
}

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

run_reload_bench() {
  local run_id="$1"
  local output_dir="$2"
  env \
    RUN_ID="${run_id}" \
    OUTPUT_DIR="${output_dir}" \
    ARCHIVE_REPORTS=false \
    CARGO_PROFILE="${CARGO_PROFILE:-release}" \
    CARGO_TOOLCHAIN="${CARGO_TOOLCHAIN:-}" \
    ALLOCATOR="${ALLOCATOR:-system}" \
    ITERATIONS="${ITERATIONS:-25}" \
    SNAPSHOT_LISTENERS="${SNAPSHOT_LISTENERS:-24}" \
    ROUTES_PER_LISTENER="${ROUTES_PER_LISTENER:-16}" \
    BACKENDS_PER_ROUTE="${BACKENDS_PER_ROUTE:-4}" \
    ENDPOINTS_PER_BACKEND="${ENDPOINTS_PER_BACKEND:-4}" \
    TLS_LISTENERS="${TLS_LISTENERS:-32}" \
    TLS_CA_BUNDLE_VARIANTS="${TLS_CA_BUNDLE_VARIANTS:-4}" \
    "${RELOAD_BENCH_SCRIPT}"
}

capture_perf_record() {
  mkdir -p "${PERF_REPLAY_DIR}"
  "${PERF_TOOL}" record \
    -F "${PERF_RECORD_FREQUENCY}" \
    -g \
    -o "${PERF_RECORD_OUTPUT}" \
    -- \
    env \
      RUN_ID="${RUN_ID}-perf-record" \
      OUTPUT_DIR="${PERF_REPLAY_DIR}" \
      ARCHIVE_REPORTS=false \
      CARGO_PROFILE="${CARGO_PROFILE:-release}" \
      CARGO_TOOLCHAIN="${CARGO_TOOLCHAIN:-}" \
      ALLOCATOR="${ALLOCATOR:-system}" \
      ITERATIONS="${ITERATIONS:-25}" \
      SNAPSHOT_LISTENERS="${SNAPSHOT_LISTENERS:-24}" \
      ROUTES_PER_LISTENER="${ROUTES_PER_LISTENER:-16}" \
      BACKENDS_PER_ROUTE="${BACKENDS_PER_ROUTE:-4}" \
      ENDPOINTS_PER_BACKEND="${ENDPOINTS_PER_BACKEND:-4}" \
      TLS_LISTENERS="${TLS_LISTENERS:-32}" \
      TLS_CA_BUNDLE_VARIANTS="${TLS_CA_BUNDLE_VARIANTS:-4}" \
      "${RELOAD_BENCH_SCRIPT}"
}

capture_perf_stat() {
  mkdir -p "${PERF_REPLAY_DIR}"
  "${PERF_TOOL}" stat \
    -e "${PERF_STAT_EVENTS}" \
    -o "${PERF_STAT_OUTPUT}" \
    -- \
    env \
      RUN_ID="${RUN_ID}-perf-stat" \
      OUTPUT_DIR="${PERF_REPLAY_DIR}" \
      ARCHIVE_REPORTS=false \
      CARGO_PROFILE="${CARGO_PROFILE:-release}" \
      CARGO_TOOLCHAIN="${CARGO_TOOLCHAIN:-}" \
      ALLOCATOR="${ALLOCATOR:-system}" \
      ITERATIONS="${ITERATIONS:-25}" \
      SNAPSHOT_LISTENERS="${SNAPSHOT_LISTENERS:-24}" \
      ROUTES_PER_LISTENER="${ROUTES_PER_LISTENER:-16}" \
      BACKENDS_PER_ROUTE="${BACKENDS_PER_ROUTE:-4}" \
      ENDPOINTS_PER_BACKEND="${ENDPOINTS_PER_BACKEND:-4}" \
      TLS_LISTENERS="${TLS_LISTENERS:-32}" \
      TLS_CA_BUNDLE_VARIANTS="${TLS_CA_BUNDLE_VARIANTS:-4}" \
      "${RELOAD_BENCH_SCRIPT}"
}

generate_flamegraph() {
  if ! command -v "${STACKCOLLAPSE_TOOL}" >/dev/null 2>&1; then
    log "stack collapse tool not found, skipping flamegraph generation"
    return
  fi
  if ! command -v "${FLAMEGRAPH_TOOL}" >/dev/null 2>&1; then
    log "flamegraph renderer not found, skipping flamegraph generation"
    return
  fi

  "${PERF_TOOL}" script -i "${PERF_RECORD_OUTPUT}" \
    | "${STACKCOLLAPSE_TOOL}" \
    | "${FLAMEGRAPH_TOOL}" >"${FLAMEGRAPH_OUTPUT}"
  FLAMEGRAPH_GENERATED="true"
}

append_perf_metadata() {
  PERF_VERSION="$("${PERF_TOOL}" --version | head -n1 || true)"
  {
    printf 'perf_run_id=%s\n' "${RUN_ID}"
    printf 'perf_record_frequency=%s\n' "${PERF_RECORD_FREQUENCY}"
    printf 'perf_tool=%s\n' "${PERF_TOOL}"
    printf 'perf_version=%s\n' "${PERF_VERSION}"
    printf 'flamegraph_generated=%s\n' "${FLAMEGRAPH_GENERATED}"
    printf 'stackcollapse_tool=%s\n' "${STACKCOLLAPSE_TOOL}"
    printf 'flamegraph_tool=%s\n' "${FLAMEGRAPH_TOOL}"
  } >>"${METADATA_OUTPUT}"
}

write_summary() {
  cat >"${SUMMARY_OUTPUT}" <<EOF
# Dataplane Perf Baseline

- Run ID: \`${RUN_ID}\`
- Git commit: \`$(git -C "${ROOT_DIR}" rev-parse --short HEAD)\`
- Cargo profile: \`${CARGO_PROFILE:-release}\`
- Allocator: \`${ALLOCATOR:-system}\`
- Perf record frequency: \`${PERF_RECORD_FREQUENCY}\`
- Perf tool: \`${PERF_VERSION:-unknown}\`
- Flamegraph generated: \`${FLAMEGRAPH_GENERATED}\`

## Evidence Contract

- \`bench.json\`: dataplane reload microbenchmark baseline from \`run-dataplane-reload-bench.sh\`
- \`bench-summary.md\`: operator-facing reload benchmark summary preserved from the baseline run
- \`perf.data\`: raw \`${PERF_TOOL}\` sampling capture for hotspot drilling
- \`perf-stat.txt\`: top-level perf counter summary for CPU / context-switch / cache behavior comparison
- \`flamegraph.svg\`: folded-stack flamegraph rendered from \`perf.data\` when flamegraph tooling is available

## Decision Rule

- Before changing release profile, allocator, or runtime micro-optimizations, first compare `bench-summary.md`, `perf-stat.txt`, and `flamegraph.svg` together.
- Only when all three categories of evidence point to demonstrable benefit and results are stable should the optimization be promoted to default; if only complexity increases and the benefit is unstable, maintain status quo.
EOF
}

verify_outputs() {
  local required=(
    "${BENCH_OUTPUT}"
    "${METADATA_OUTPUT}"
    "${BENCH_SUMMARY_OUTPUT}"
    "${SUMMARY_OUTPUT}"
    "${PERF_RECORD_OUTPUT}"
    "${PERF_STAT_OUTPUT}"
  )
  local path
  for path in "${required[@]}"; do
    if [[ ! -f "${path}" ]]; then
      log "missing expected artifact: ${path}"
      exit 1
    fi
  done
  if [[ "${FLAMEGRAPH_GENERATED}" == "true" && ! -f "${FLAMEGRAPH_OUTPUT}" ]]; then
    log "expected flamegraph output at ${FLAMEGRAPH_OUTPUT}"
    exit 1
  fi
}

archive_outputs() {
  if [[ "${ARCHIVE_REPORTS}" != "true" ]]; then
    return
  fi

  mkdir -p "${ARCHIVE_DIR}"
  cp "${BENCH_OUTPUT}" "${ARCHIVE_DIR}/bench.json"
  cp "${METADATA_OUTPUT}" "${ARCHIVE_DIR}/metadata.txt"
  cp "${BENCH_SUMMARY_OUTPUT}" "${ARCHIVE_DIR}/bench-summary.md"
  cp "${SUMMARY_OUTPUT}" "${ARCHIVE_DIR}/summary.md"
  cp "${PERF_RECORD_OUTPUT}" "${ARCHIVE_DIR}/perf.data"
  cp "${PERF_STAT_OUTPUT}" "${ARCHIVE_DIR}/perf-stat.txt"
  if [[ "${FLAMEGRAPH_GENERATED}" == "true" ]]; then
    cp "${FLAMEGRAPH_OUTPUT}" "${ARCHIVE_DIR}/flamegraph.svg"
  fi
}

main() {
  require_command "${PERF_TOOL}"
  require_command "${RELOAD_BENCH_SCRIPT}"

  mkdir -p "${OUTPUT_DIR}"

  log "capturing dataplane reload benchmark baseline"
  run_reload_bench "${RUN_ID}" "${OUTPUT_DIR}"
  if [[ -f "${OUTPUT_DIR}/summary.md" ]]; then
    mv "${OUTPUT_DIR}/summary.md" "${BENCH_SUMMARY_OUTPUT}"
  fi

  log "capturing perf record baseline"
  capture_perf_record
  log "capturing perf stat baseline"
  capture_perf_stat
  generate_flamegraph
  append_perf_metadata
  write_summary
  verify_outputs
  archive_outputs

  log "perf baseline evidence written to ${OUTPUT_DIR}"
  if [[ "${ARCHIVE_REPORTS}" == "true" ]]; then
    log "archived perf baseline evidence written to ${ARCHIVE_DIR}"
  fi
}

main "$@"
