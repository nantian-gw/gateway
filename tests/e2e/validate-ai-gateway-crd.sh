#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-gw}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
AETHER_NAMESPACE="${AETHER_NAMESPACE:-nantian-gw}"

TMP_DIR=""
SUCCESS="false"

log() {
  printf '[ai-gateway-crd] %s\n' "$*"
}

k() {
  kubectl --context "${KUBE_CONTEXT}" "$@"
}

kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

cleanup() {
  local exit_code=$?
  rm -rf "${TMP_DIR:-}"
  if [[ "${exit_code}" -eq 0 && "${SUCCESS}" == "true" ]]; then
    log "PASS"
  else
    log "FAIL (exit=${exit_code})"
  fi
}

trap cleanup EXIT
TMP_DIR="$(mktemp -d)"

# ── preflight ────────────────────────────────────────────────
if ! kind_cluster_exists; then
  log "SKIP: kind cluster ${CLUSTER_NAME} does not exist"
  SUCCESS="true"
  exit 0
fi

log "checking CRD installed"
if ! k get crd aiservices.gateway.nantian.dev >/dev/null 2>&1; then
  log "installing AIService CRD"
  k apply -f "${ROOT_DIR}/controlplane/config/crd/bases/gateway.nantian.dev_aiservices.yaml"
fi

# ── 1. CRD discovery ─────────────────────────────────────────
log "test 1: CRD discovery"
if ! k get crd aiservices.gateway.nantian.dev >/dev/null 2>&1; then
  log "FAIL: CRD not found"
  exit 1
fi

CRD_SCOPE="$(k get crd aiservices.gateway.nantian.dev -o jsonpath='{.spec.scope}')"
if [[ "${CRD_SCOPE}" != "Namespaced" ]]; then
  log "FAIL: expected Namespaced scope, got ${CRD_SCOPE}"
  exit 1
fi
log "  scope=${CRD_SCOPE} OK"

# ── 2. AIService resource lifecycle ──────────────────────────
log "test 2: AIService resource CRUD"

AISERVICE_YAML="${TMP_DIR}/test-aiservice.yaml"
cat > "${AISERVICE_YAML}" <<'EOF'
apiVersion: gateway.nantian.dev/v1alpha1
kind: AIService
metadata:
  name: test-openai
  namespace: nantian-gw
spec:
  provider: openai
  format: openai
  model: gpt-4o
  auth:
    type: bearer
    secret: test-openai-key
    key: apiKey
  timeout: 30s
  observability:
    langfuse:
      host: https://cloud.langfuse.com
      publicKey: pk-test
      secretKey: sk-test
EOF

# create
k apply -f "${AISERVICE_YAML}"
sleep 1

# read
SVC="$(k get aiservice test-openai -n nantian-gw -o json 2>&1)"
PROVIDER="$(echo "${SVC}" | python3 -c "import sys,json; print(json.load(sys.stdin)['spec']['provider'])" 2>/dev/null || echo "")"
if [[ "${PROVIDER}" != "openai" ]]; then
  log "FAIL: expected provider=openai, got '${PROVIDER}'"
  exit 1
fi
log "  provider=${PROVIDER} OK"

MODEL="$(echo "${SVC}" | python3 -c "import sys,json; print(json.load(sys.stdin)['spec']['model'])" 2>/dev/null || echo "")"
if [[ "${MODEL}" != "gpt-4o" ]]; then
  log "FAIL: expected model=gpt-4o, got '${MODEL}'"
  exit 1
fi
log "  model=${MODEL} OK"

# list
COUNT="$(k get aiservice -n nantian-gw -o json 2>&1 | python3 -c "import sys,json; print(len(json.load(sys.stdin)['items']))" 2>/dev/null || echo "0")"
if [[ "${COUNT}" -lt 1 ]]; then
  log "FAIL: expected at least 1 AIService"
  exit 1
fi
log "  count=${COUNT} OK"

# delete
k delete aiservice test-openai -n nantian-gw
sleep 1
if k get aiservice test-openai -n nantian-gw >/dev/null 2>&1; then
  log "FAIL: AIService not deleted"
  exit 1
fi
log "  delete OK"

# ── 3. Schema validation: reject invalid ─────────────────────
log "test 3: schema validation rejects missing required fields"

INVALID_YAML="${TMP_DIR}/invalid-aiservice.yaml"
cat > "${INVALID_YAML}" <<'EOF'
apiVersion: gateway.nantian.dev/v1alpha1
kind: AIService
metadata:
  name: test-invalid
  namespace: nantian-gw
spec:
  format: openai
EOF

if k apply -f "${INVALID_YAML}" --dry-run=server 2>&1; then
  log "FAIL: expected schema validation to reject missing provider"
  exit 1
fi
log "  schema validation OK"

log "all tests passed"
SUCCESS="true"
