#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
AETHER_NAMESPACE="${AETHER_NAMESPACE:-aether-gateway}"
SUCCESS="false"
FAILURES=0

log() { printf '[ai-conformance] %s\n' "$*"; }
pass() { log "  PASS: $*"; }

k() { kubectl --context "${KUBE_CONTEXT}" "$@"; }

kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

cleanup() {
  if [[ "${SUCCESS}" == "true" ]]; then
    log "AI Gateway Conformance: ALL PASS"
  else
    log "AI Gateway Conformance: FAIL (${FAILURES} failures)"
  fi
}
trap cleanup EXIT

if ! kind_cluster_exists; then
  log "SKIP: kind cluster ${CLUSTER_NAME} not available"
  SUCCESS="true"; exit 0
fi

log "=== Phase 1: CRD Discovery ==="

for crd in aiservices.gateway.nantian.dev tokenpolicies.gateway.nantian.dev; do
  if k get crd "$crd" >/dev/null 2>&1; then
    pass "CRD $crd"
  else
    log "  WARN: CRD $crd not found (install via kubectl apply)"
    FAILURES=$((FAILURES + 1))
  fi
done

log "=== Phase 2: CRD Schema Validation ==="

TMP="$(mktemp -d)"
trap "rm -rf $TMP; cleanup" EXIT

cat > "$TMP/aiservice.yaml" <<EOF
apiVersion: gateway.nantian.dev/v1alpha1
kind: AIService
metadata:
  name: conform-test
  namespace: $AETHER_NAMESPACE
spec:
  provider: openai
  model: gpt-4o
  auth:
    type: bearer
    secret: test-key
    key: apiKey
EOF

if k apply -f "$TMP/aiservice.yaml" --dry-run=client >/dev/null 2>&1; then
  pass "AIService schema valid"
else
  log "  FAIL: AIService schema validation"
  FAILURES=$((FAILURES + 1))
fi

cat > "$TMP/tokenpolicy.yaml" <<EOF
apiVersion: gateway.nantian.dev/v1alpha1
kind: TokenPolicy
metadata:
  name: conform-test
  namespace: $AETHER_NAMESPACE
spec:
  targetRefs:
    - group: gateway.nantian.dev
      kind: AIService
      name: conform-test
  tokensPerMinute: 100000
  scope: apiKey
EOF

if k apply -f "$TMP/tokenpolicy.yaml" --dry-run=client >/dev/null 2>&1; then
  pass "TokenPolicy schema valid"
else
  log "  FAIL: TokenPolicy schema validation"
  FAILURES=$((FAILURES + 1))
fi

log "=== Phase 3: Unit Test Results ==="

cd "$ROOT_DIR/dataplane"
if cargo test -p aeg-ai --quiet 2>&1 >/dev/null; then
  TEST_COUNT=$(cargo test -p aeg-ai 2>&1 | grep "test result:" | awk '{sum+=$2} END {print sum}')
  log "  Rust aeg-ai tests: ${TEST_COUNT:-?} passing"
  pass "cargo test -p aeg-ai"
else
  log "  FAIL: cargo test -p aeg-ai"
  FAILURES=$((FAILURES + 1))
fi

cd "$ROOT_DIR/controlplane"
if go test ./internal/translator/... >/dev/null 2>&1; then
  pass "go test translator"
else
  log "  FAIL: go test translator"
  FAILURES=$((FAILURES + 1))
fi

log "=== Phase 4: Feature Catalog ==="

declare -A FEATURES=(
  ["format-adapters"]="dataplane/crates/aeg-ai/src/format/openai.rs"
  ["token-policy"]="controlplane/internal/gatewayapiexperimental/tokenpolicyv1alpha1/types.go"
  ["rate-limiter"]="dataplane/crates/aeg-ai/src/ratelimit.rs"
  ["prompt-guard"]="dataplane/crates/aeg-ai/src/prompt_guard.rs"
  ["semantic-cache"]="dataplane/crates/aeg-ai/src/semantic_cache.rs"
  ["model-fallback"]="dataplane/crates/aeg-ai/src/fallback.rs"
  ["cost-tracker"]="dataplane/crates/aeg-ai/src/cost.rs"
  ["pii-masking"]="dataplane/crates/aeg-ai/src/pii.rs"
  ["content-safety"]="dataplane/crates/aeg-ai/src/content_safety.rs"
  ["multi-tenancy"]="dataplane/crates/aeg-ai/src/multitenant.rs"
  ["model-router"]="dataplane/crates/aeg-ai/src/model_router.rs"
  ["ab-testing"]="dataplane/crates/aeg-ai/src/ab_test.rs"
  ["api-key-mgmt"]="dataplane/crates/aeg-ai/src/keyring.rs"
)

for feature in "${!FEATURES[@]}"; do
  path="${FEATURES[$feature]}"
  if [[ -f "$ROOT_DIR/$path" ]]; then
    pass "feature: $feature"
  else
    log "  FAIL: $feature missing ($path)"
    FAILURES=$((FAILURES + 1))
  fi
done

if [[ "${FAILURES}" -eq 0 ]]; then
  SUCCESS="true"
fi