#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATAPLANE_DIR="${ROOT_DIR}/dataplane"
DATAPLANE_MANIFEST="${DATAPLANE_DIR}/Cargo.toml"
DENY_CONFIG="${DATAPLANE_DIR}/deny.toml"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/tmp/dataplane-guardrails/latest}"
LLVM_COV_SUMMARY="${LLVM_COV_SUMMARY:-${OUTPUT_DIR}/llvm-cov-summary.json}"

log() {
  printf '[dataplane-guardrails] %s\n' "$*"
}

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

require_cargo_subcommand() {
  local name="$1"
  if ! cargo "${name}" --version >/dev/null 2>&1; then
    log "missing required cargo subcommand: cargo-${name}"
    exit 1
  fi
}

configure_llvm_tools() {
  if [[ -n "${LLVM_COV:-}" && -n "${LLVM_PROFDATA:-}" ]]; then
    return
  fi

  if command -v llvm-cov >/dev/null 2>&1 && command -v llvm-profdata >/dev/null 2>&1; then
    export LLVM_COV="${LLVM_COV:-$(command -v llvm-cov)}"
    export LLVM_PROFDATA="${LLVM_PROFDATA:-$(command -v llvm-profdata)}"
  fi
}

write_versions() {
  {
    printf 'timestamp=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    cargo --version
    rustc --version
    cargo clippy --version
    cargo nextest --version
    cargo llvm-cov --version
    cargo deny --version
  } >"${OUTPUT_DIR}/versions.txt"
}

run_clippy() {
  log "running cargo clippy --all-targets"
  cargo clippy \
    --manifest-path "${DATAPLANE_MANIFEST}" \
    --workspace \
    --all-targets \
    -- \
    -D warnings \
    >"${OUTPUT_DIR}/clippy.log" 2>&1
}

run_allocator_feature_guardrail() {
  local feature

  for feature in allocator-mimalloc allocator-jemalloc; do
    log "running allocator feature guardrail for ${feature}"
    cargo clippy \
      --manifest-path "${DATAPLANE_MANIFEST}" \
      -p aeg-allocator \
      --all-targets \
      --features "${feature}" \
      -- \
      -D warnings \
      >"${OUTPUT_DIR}/allocator-${feature}-clippy.log" 2>&1
    cargo test \
      --manifest-path "${DATAPLANE_MANIFEST}" \
      -p aeg-allocator \
      --features "${feature}" \
      >"${OUTPUT_DIR}/allocator-${feature}-test.log" 2>&1
  done
}

run_unwrap_guardrail() {
  log "running non-test unwrap/expect guardrail"
  "${ROOT_DIR}/scripts/check-rust-unwraps.sh" >"${OUTPUT_DIR}/unwraps.log" 2>&1
}

run_protobuf_dependency_guardrail() {
  local tree_output normalized expected

  log "running protobuf dependency guardrail"
  tree_output="$(
    cargo tree \
      --manifest-path "${DATAPLANE_MANIFEST}" \
      -i protobuf \
      -e normal,build,dev \
      --prefix none
  )"
  printf '%s\n' "${tree_output}" >"${OUTPUT_DIR}/protobuf-tree.log"

  if ! grep -Eq '^protobuf v2\.' "${OUTPUT_DIR}/protobuf-tree.log"; then
    log "protobuf 2.x is no longer present; remove SEC-RA-004 tracking and the deny exception before continuing"
    exit 1
  fi

  normalized="$(
    sed -E 's/ \(\*\)$//' "${OUTPUT_DIR}/protobuf-tree.log" \
      | sed -E 's/ \([^)]*\/dataplane\/crates\/[^)]*\)$//' \
      | sed '/^$/d' \
      | sort -u
  )"
  printf '%s\n' "${normalized}" >"${OUTPUT_DIR}/protobuf-tree-normalized.log"

  read -r -d '' expected <<'EOF' || true
aeg-app v0.1.0
aeg-bench v0.1.0
aeg-config v0.1.0
aeg-http v0.1.0
aeg-shared-tls v0.1.0
aether v0.8.0
aether-cache v0.8.0
aether-core v0.8.0
aether-proxy v0.8.0
prometheus v0.13.4
protobuf v2.28.0
EOF

  if [[ "${normalized}" != "${expected}" ]]; then
    log "protobuf dependency graph changed; review SEC-RA-004, the deny exception, and the upstream upgrade path"
    log "expected normalized graph saved in-memory; actual graph written to ${OUTPUT_DIR}/protobuf-tree-normalized.log"
    exit 1
  fi
}

run_nextest() {
  log "running cargo nextest"
  cargo nextest run \
    --manifest-path "${DATAPLANE_MANIFEST}" \
    --workspace \
    --no-fail-fast \
    >"${OUTPUT_DIR}/nextest.log" 2>&1
}

run_llvm_cov() {
  log "running cargo llvm-cov"
  cargo llvm-cov \
    --manifest-path "${DATAPLANE_MANIFEST}" \
    --workspace \
    --json \
    --summary-only \
    --output-path "${LLVM_COV_SUMMARY}" \
    >"${OUTPUT_DIR}/llvm-cov.log" 2>&1
}

run_cargo_deny() {
  log "running cargo deny"
  (
    cd "${DATAPLANE_DIR}"
    cargo deny check --config "${DENY_CONFIG}" all
  ) >"${OUTPUT_DIR}/cargo-deny.log" 2>&1
}

main() {
  require_command cargo
  require_command rustc
  require_cargo_subcommand clippy
  require_cargo_subcommand nextest
  require_cargo_subcommand llvm-cov
  require_cargo_subcommand deny

  mkdir -p "${OUTPUT_DIR}"

  configure_llvm_tools
  write_versions
  run_protobuf_dependency_guardrail
  run_clippy
  run_allocator_feature_guardrail
  run_unwrap_guardrail
  run_nextest
  run_llvm_cov
  run_cargo_deny

  log "dataplane guardrails completed successfully"
  log "artifacts: ${OUTPUT_DIR}"
}

main "$@"
