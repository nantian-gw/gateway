#!/usr/bin/env bash
# e2e smoke test — deploy, route, request, verify, cleanup.
# Usage: ./run.sh              # full cycle (deploy → test → cleanup)
#        ./run.sh --no-cleanup # keep cluster running for debugging
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATEWAY_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-e2e}"
CONTROL_PLANE_NS="nantian-gw"
TEST_NS="nantian-e2e"
CONTROL_PLANE_DEPLOYMENT="nantian-gw-controlplane"
GATEWAY_CLASS_NAME="${GATEWAY_CLASS_NAME:-nantian-gw}"
GATEWAY_HOST="${GATEWAY_HOST:-127.0.0.1}"
GATEWAY_HTTP_PORT="${GATEWAY_HTTP_PORT:-80}"
TIMEOUT="${TIMEOUT:-180}"
KIND_CONFIG="${KIND_CONFIG:-$GATEWAY_ROOT/scripts/ci/kind-ci-config.yaml}"
CLEANUP="true"
BOOTSTRAP="true"
FAILED=false

for arg in "$@"; do
    case "$arg" in
        --no-cleanup)
            CLEANUP="false"
            ;;
        --skip-bootstrap)
            BOOTSTRAP="false"
            ;;
        *)
            echo "unknown argument: $arg" >&2
            exit 1
            ;;
    esac
done

red()   { echo -e "\033[31m$*\033[0m"; }
green() { echo -e "\033[32m$*\033[0m"; }
yellow(){ echo -e "\033[33m$*\033[0m"; }

fail() {
    red "FAIL: $*"
    FAILED=true
}

cleanup_cluster() {
    if [[ "$CLEANUP" == "false" ]]; then
        yellow "Skipping cleanup (--no-cleanup)"
        return
    fi
    echo "=== Cleaning up ==="
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
}

# ── Step 1: ensure kind cluster ──
ensure_cluster() {
    if kind get clusters 2>/dev/null | grep -q "^$CLUSTER_NAME$"; then
        yellow "Cluster $CLUSTER_NAME already exists, using it"
        return
    fi
    echo "=== Creating kind cluster: $CLUSTER_NAME ==="
    if [[ ! -f "$KIND_CONFIG" ]]; then
        fail "kind config not found: $KIND_CONFIG"
        return 1
    fi
    kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG" --wait 5m
    kubectl wait --for=condition=ready node --all --timeout=2m
}

# ── Step 2: deploy Gateway API CRDs ──
install_gateway_api_crds() {
    if kubectl get crd gatewayclasses.gateway.networking.k8s.io &>/dev/null; then
        yellow "Gateway API CRDs already installed"
        return
    fi
    echo "=== Installing Gateway API CRDs ==="
    BASE="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1"
    kubectl apply -f "$BASE/standard-install.yaml"
    kubectl apply -f "$BASE/experimental-install.yaml" || true
    kubectl wait --for=condition=established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
}

# ── Step 3: deploy nantian-gw ──
deploy_gateway() {
    if kubectl get deployment -n "$CONTROL_PLANE_NS" "$CONTROL_PLANE_DEPLOYMENT" &>/dev/null; then
        yellow "nantian-gw already deployed, skipping"
        return
    fi
    echo "=== Deploying nantian-gw ==="
    kustomize build "$GATEWAY_ROOT/deploy/kubernetes/overlays/kind-conformance" --load-restrictor LoadRestrictionsNone | kubectl apply -f -
    kubectl wait --for=condition=ready pod --all -n "$CONTROL_PLANE_NS" --timeout="${TIMEOUT}s"
}

# ── Step 4: deploy echo backend ──
deploy_backend() {
    echo "=== Deploying echo backend ==="
    kubectl create namespace "$TEST_NS" --dry-run=client -o yaml | kubectl apply -f -

    kubectl apply -n "$TEST_NS" -f - <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echo
  template:
    metadata:
      labels:
        app: echo
    spec:
      containers:
      - name: echo
        image: docker.io/ealen/echo-server:latest
        ports:
        - containerPort: 80
        env:
        - name: PORT
          value: "80"
---
apiVersion: v1
kind: Service
metadata:
  name: echo
spec:
  selector:
    app: echo
  ports:
  - port: 80
    targetPort: 80
YAML

    kubectl wait --for=condition=ready pod -l app=echo -n "$TEST_NS" --timeout="${TIMEOUT}s"
    green "  echo backend ready"
}

# ── Step 5: create Gateway, ReferenceGrant, and HTTPRoute ──
create_gateway() {
    echo "=== Creating Gateway ==="
    kubectl apply -n "$CONTROL_PLANE_NS" -f - <<YAML
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: nantian-gw
spec:
  gatewayClassName: $GATEWAY_CLASS_NAME
  listeners:
  - name: http
    protocol: HTTP
    port: 80
    allowedRoutes:
      namespaces:
        from: All
YAML

    # Allow gateway status to settle before attaching routes.
    sleep 5
    green "  Gateway created"
}

create_reference_grant() {
    echo "=== Creating ReferenceGrant ==="
    kubectl apply -n "$TEST_NS" -f - <<YAML
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-nantian-gw-routes
spec:
  from:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    namespace: $CONTROL_PLANE_NS
  to:
  - group: ""
    kind: Service
    name: echo
YAML

    green "  ReferenceGrant created"
}

create_route() {
    echo "=== Creating HTTPRoute ==="
    kubectl apply -n "$CONTROL_PLANE_NS" -f - <<YAML
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: echo
spec:
  parentRefs:
  - name: nantian-gw
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /echo
    backendRefs:
    - name: echo
      namespace: $TEST_NS
      port: 80
YAML

    # Allow route to propagate
    sleep 5
    green "  HTTPRoute created"
}

wait_for_gateway_programmed() {
    local gateway_name="${1:-nantian-gw}"

    echo "=== Waiting for Gateway ${gateway_name} to be Programmed ==="
    local request_deadline=$((SECONDS + TIMEOUT))
    while (( SECONDS < request_deadline )); do
        local programmed
        programmed="$(
            kubectl get gateway "$gateway_name" -n "$CONTROL_PLANE_NS" \
                -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}' 2>/dev/null || true
        )"
        if [[ "$programmed" == "True" ]]; then
            green "  Gateway ${gateway_name} is Programmed"
            return 0
        fi
        sleep 2
    done

    fail "Gateway ${gateway_name} did not become Programmed=True within ${TIMEOUT}s"
    return 1
}

# ── Step 6: send request through the Kind host port entry ──
send_request() {
    local endpoint="http://${GATEWAY_HOST}:${GATEWAY_HTTP_PORT}/echo"

    echo "=== Sending test request (${endpoint}) ==="
    local request_deadline=$((SECONDS + TIMEOUT))
    local response="000"
    while (( SECONDS < request_deadline )); do
        response=$(curl -s -o /dev/null -w "%{http_code}" "$endpoint" 2>/dev/null || echo "000")
        if [[ "$response" == "200" ]]; then
            green "  PASS: GET /echo -> HTTP $response"
            return 0
        fi

        sleep 2
    done

    fail "GET /echo via ${endpoint} -> HTTP $response (expected 200 within ${TIMEOUT}s)"
    return 1
}

# ── Main ──
main() {
    trap 'FAILED=true' ERR
    trap 'cleanup_cluster; if $FAILED; then red "✗ Smoke test FAILED"; else green "✓ Smoke test PASSED"; fi' EXIT

    if [[ "$BOOTSTRAP" == "true" ]]; then
        ensure_cluster
        install_gateway_api_crds
        deploy_gateway
    fi
    deploy_backend
    create_gateway
    create_reference_grant
    create_route
    wait_for_gateway_programmed
    send_request
}

main "$@"
