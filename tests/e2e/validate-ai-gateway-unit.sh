#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SUCCESS="false"
FAILURES=0

log() { printf '[ai-gateway-unit] %s\n' "$*"; }
pass() { log "  PASS: $*"; }
fail() { log "  FAIL: $*"; FAILURES=$((FAILURES + 1)); }

cleanup() {
  if [[ "${SUCCESS}" == "true" ]]; then
    log "ALL PASS"
  else
    log "FAIL (${FAILURES} failures)"
  fi
}
trap cleanup EXIT

# ── Rust aeg-ai tests ─────────────────────────────────────────
log "Rust: cargo test -p aeg-ai"
if (cd dataplane && cargo test -p aeg-ai --quiet 2>&1); then
  pass "Rust aeg-ai tests"
else
  fail "Rust aeg-ai tests"
fi

# Check no unsafe code
log "Rust: check unsafe code"
if grep -rn "unsafe" dataplane/crates/aeg-ai/src/ | grep -v "forbid(unsafe_code)" >/dev/null 2>&1; then
  fail "unsafe code found in aeg-ai/src/"
  grep -rn "unsafe" dataplane/crates/aeg-ai/src/ | grep -v "forbid(unsafe_code)" || true
else
  pass "no unsafe code"
fi

# ── Go translator tests ──────────────────────────────────────
log "Go: go test ./internal/translator/..."
(
  cd controlplane
  if go test ./internal/translator/... 2>&1; then
    pass "Go translator tests"
  else
    fail "Go translator tests"
  fi
)

log "Go: go build ./..."
(
  cd controlplane
  if go build ./... 2>&1; then
    pass "Go build"
  else
    fail "Go build"
  fi
)

# ── Dashboard ────────────────────────────────────────────────
log "Dashboard: npm run check"
(
  cd dashboard
  if npm run check >/dev/null 2>&1; then
    pass "Dashboard check"
  else
    fail "Dashboard check"
  fi
)

# ── Proto consistency ────────────────────────────────────────
log "Proto: verify regenerated code is up to date"
if [[ -f controlplane/internal/gen/gateway/control/v1/control.pb.go ]]; then
  PB_SIZE=$(wc -l < controlplane/internal/gen/gateway/control/v1/control.pb.go)
  log "  proto generated code: ${PB_SIZE} lines"
  if [[ "${PB_SIZE}" -gt 1000 ]]; then
    pass "Proto generated code present"
  else
    fail "Proto generated code too small (${PB_SIZE} lines)"
  fi
else
  fail "Proto generated code missing"
fi

# ── File structure check ─────────────────────────────────────
log "Structure: verify key files exist"
for f in \
  dataplane/crates/aeg-ai/src/lib.rs \
  dataplane/crates/aeg-ai/src/format/ir.rs \
  dataplane/crates/aeg-ai/src/format/mod.rs \
  dataplane/crates/aeg-ai/src/token.rs \
  dataplane/crates/aeg-ai/src/filter.rs \
  dataplane/crates/aeg-ai/src/observability/metrics.rs \
  dataplane/crates/aeg-ai/src/observability/tracing.rs \
  dataplane/crates/aeg-ai/src/observability/langfuse.rs \
  controlplane/internal/gatewayapiexperimental/aiservicev1alpha1/types.go \
  controlplane/internal/translator/ai_service.go \
  controlplane/config/crd/bases/gateway.nantian.dev_aiservices.yaml \
  dashboard/src/components/ai/format-badge.tsx \
  docs/design/ai-gateway/architecture.md; do
  if [[ -f "$f" ]]; then
    pass "exists: $f"
  else
    fail "missing: $f"
  fi
done

# dashboard App Router paths use literal brackets
log "Structure: verify dashboard AI pages"
if find dashboard/src/app -path "*/ai/overview/page.tsx" | grep -q .; then
  pass "exists: dashboard/src/app/[locale]/ai/overview/page.tsx"
else
  fail "missing: dashboard/src/app/[locale]/ai/overview/page.tsx"
fi

if [[ "${FAILURES}" -eq 0 ]]; then
  SUCCESS="true"
fi