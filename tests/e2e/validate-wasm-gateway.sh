#!/usr/bin/env bash
set -euo pipefail

# validate-wasm-gateway.sh — Validate Wasm Gateway build and configuration
#
# This script validates:
# 1. aeg-wasm crate compiles and tests pass
# 2. aeg-ai wasm_filter integration compiles
# 3. Go WasmPlugin CRD compiles and tests pass
# 4. Wasm CRD exists in kustomize output
# 5. Dashboard builds with wasm pages
# 6. Helm lint passes with wasm RBAC

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

PASS=0
FAIL=0

check() {
    local name="$1"
    shift
    echo -n "  [....] ${name} ... "
    if "$@" > /dev/null 2>&1; then
        echo -e "\r  [PASS] ${name}"
        PASS=$((PASS + 1))
    else
        echo -e "\r  [FAIL] ${name}"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== Wasm Gateway E2E Validation ==="
echo ""

# --- Rust checks ---
echo "--- Rust Dataplane ---"
check "aeg-wasm cargo check" cargo check --manifest-path dataplane/Cargo.toml -p aeg-wasm
check "aeg-wasm cargo test" cargo test --manifest-path dataplane/Cargo.toml -p aeg-wasm
check "aeg-ai cargo check (with wasm_filter)" cargo check --manifest-path dataplane/Cargo.toml -p aeg-ai
check "aeg-ai cargo test (regression)" cargo test --manifest-path dataplane/Cargo.toml -p aeg-ai
check "aeg-ir cargo check" cargo check --manifest-path dataplane/Cargo.toml -p aeg-ir

# --- Wasm build ---
echo ""
echo "--- Wasm Build ---"
check "build-wasm script exists" bash -c "test -x scripts/build-wasm-examples.sh"

# --- Go checks ---
echo ""
echo "--- Go Controlplane ---"
check "Go build" bash -c "cd controlplane && go build ./..."
check "Go test WasmPlugin translator" bash -c "cd controlplane && go test ./internal/translator/ -run Wasm -v -count=1"
check "Go test all" bash -c "cd controlplane && go test ./... -count=1"

# --- Deploy checks ---
echo ""
echo "--- Deploy ---"
check "kustomize has wasmplugin CRD" bash -c "kubectl kustomize deploy/kubernetes/base/ | grep -q 'wasmplugins.gateway.nantian.dev'"
check "kustomize RBAC has wasmplugins" bash -c "kubectl kustomize deploy/kubernetes/base/ | grep -q 'wasmplugins'"
check "Helm lint" helm lint deploy/helm/aether-gateway/

# --- Dashboard check ---
echo ""
echo "--- Dashboard ---"
check "npm run check" bash -c "cd dashboard && npm run check"

# --- Summary ---
echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="

if [ "${FAIL}" -gt 0 ]; then
    exit 1
fi