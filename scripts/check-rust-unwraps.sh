#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATAPLANE_MANIFEST="${ROOT_DIR}/dataplane/Cargo.toml"

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    echo "missing required command: ${name}" >&2
    exit 1
  fi
}

run_workspace_check() {
  cargo clippy \
    --manifest-path "${DATAPLANE_MANIFEST}" \
    --workspace \
    --lib \
    --bins \
    -- \
    -D clippy::unwrap_used \
    -D clippy::expect_used

  local feature
  for feature in allocator-mimalloc allocator-jemalloc; do
    cargo clippy \
      --manifest-path "${DATAPLANE_MANIFEST}" \
      -p aeg-allocator \
      --features "${feature}" \
      --lib \
      -- \
      -D clippy::unwrap_used \
      -D clippy::expect_used
  done
}

run_fixture_check() {
  local file="$1"
  local tmp_dir manifest_dir

  if [[ ! -f "${file}" ]]; then
    echo "not a Rust source file: ${file}" >&2
    exit 1
  fi

  tmp_dir="$(mktemp -d)"
  manifest_dir="${tmp_dir}/fixture"
  mkdir -p "${manifest_dir}/src"

  cat >"${manifest_dir}/Cargo.toml" <<'EOF'
[package]
name = "unwrap-guardrail-fixture"
version = "0.1.0"
edition = "2021"

[lib]
path = "src/lib.rs"
EOF

  cp "${file}" "${manifest_dir}/src/lib.rs"

  if ! cargo clippy \
    --quiet \
    --manifest-path "${manifest_dir}/Cargo.toml" \
    --lib \
    -- \
    -D clippy::unwrap_used \
    -D clippy::expect_used; then
    echo "non-test unwrap/expect usage detected in ${file}" >&2
    rm -rf "${tmp_dir}"
    return 1
  fi

  rm -rf "${tmp_dir}"
}

main() {
  require_command cargo

  if [[ $# -eq 0 ]]; then
    run_workspace_check
    return
  fi

  local failed=0
  local file
  for file in "$@"; do
    if ! run_fixture_check "${file}"; then
      failed=1
    fi
  done

  if [[ ${failed} -ne 0 ]]; then
    exit 1
  fi
}

main "$@"
