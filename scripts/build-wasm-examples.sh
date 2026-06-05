#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATAPLANE_DIR="${ROOT_DIR}/dataplane/crates/aeg-wasm"
PREBUILT_DIR="${DATAPLANE_DIR}/prebuilt"

echo "=== Building Wasm examples ==="

# Install wasm32-wasi target
rustup target add wasm32-wasi 2>/dev/null || {
    echo "SKIP: wasm32-wasi target not available"
    exit 0
}

# Build example plugins
echo "Building hello-plugin.wasm..."
cargo build --manifest-path "${ROOT_DIR}/dataplane/Cargo.toml" \
    -p aeg-wasm --example hello-plugin \
    --target wasm32-wasi --release 2>&1

echo "Building auth-plugin.wasm..."
cargo build --manifest-path "${ROOT_DIR}/dataplane/Cargo.toml" \
    -p aeg-wasm --example auth-plugin \
    --target wasm32-wasi --release 2>&1

# Build prebuilt modules
echo "Building prebuilt tokenizer-simple.wasm..."
if [ -f "${PREBUILT_DIR}/tokenizer-simple.rs" ]; then
    rustc --target wasm32-wasi -C opt-level=s --edition 2021 \
        -o "${PREBUILT_DIR}/tokenizer-simple.wasm" \
        "${PREBUILT_DIR}/tokenizer-simple.rs"
fi

echo "Building prebuilt embedder-stub.wasm..."
if [ -f "${PREBUILT_DIR}/embedder-stub.rs" ]; then
    rustc --target wasm32-wasi -C opt-level=s --edition 2021 \
        -o "${PREBUILT_DIR}/embedder-stub.wasm" \
        "${PREBUILT_DIR}/embedder-stub.rs"
fi

echo "=== Wasm build complete ==="
ls -la "${ROOT_DIR}"/dataplane/target/wasm32-wasi/release/examples/*.wasm 2>/dev/null || echo "  (examples not found)"
ls -la "${PREBUILT_DIR}"/*.wasm 2>/dev/null || echo "  (prebuilt not found)"
