#!/usr/bin/env bash
# e2e smoke test — deploy, route, request, verify, cleanup.
# Usage: ./run.sh              # full cycle (deploy → test → cleanup)
#        ./run.sh --no-cleanup # keep cluster running for debugging
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATEWAY_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-e2e}"
CONTROL_PLANE_NS="nantian-gw"
TEST_NS="nantian-e2e"
DATA_PLANE_SVC="nantian-dataplane"
ECHO_PORT=8080
TIMEOUT="${TIMEOUT:-180}"
CLEANUP="${1:-}"
FAILED=false

red()   { echo -e "\033[31m$*\033[0m"; }
green() { echo -e "\033[32m$*\033[0m"; }
yellow(){ echo -e "\033[33m$*\033[0m"; }

fail() {
    red "FAIL: $*"
    FAILED=true
}

cleanup_cluster() {
    if [[ "$CLEANUP" == "--no-cleanup" ]]; then
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
    kind create cluster --name "$CLUSTER_NAME" --image kindest/node:v1.31.0 --wait 5m
    kubectl wait --for=condition=ready node --all --timeout=2m
}

# ── Step 2: deploy Gateway API CRDs ──
install_gateway_api_crds() {
    if kubectl get crd gatewayclasses.gateway.networking.k8s.io &>/dev/null; then
        yellow "Gateway API CRDs already installed"
        return
    fi
    echo "=== Installing Gateway API CRDs ==="
    kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/experimental-install.yaml
    kubectl wait --for=condition=established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
}

# ── Step 3: deploy nantian-gw ──
deploy_gateway() {
    if kubectl get deployment -n "$CONTROL_PLANE_NS" nantian-controlplane &>/dev/null; then
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

# ── Step 5: create HTTPRoute ──
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

# ── Step 6: port-forward and send request ──
send_request() {
    local dataplane_pod
    dataplane_pod=$(kubectl get pod -n "$CONTROL_PLANE_NS" -l app=nantian-dataplane -o jsonpath='{.items[0].metadata.name}')
    if [[ -z "$dataplane_pod" ]]; then
        fail "no data plane pod found"
        return 1
    fi

    echo "=== Sending test request (port-forward $dataplane_pod:10080) ==="

    # Start port-forward in background
    kubectl port-forward -n "$CONTROL_PLANE_NS" "pod/$dataplane_pod" 10080:10080 &>/dev/null &
    PF_PID=$!
    sleep 2

    # Send request and capture response
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:10080/echo 2>/dev/null || echo "000")
    local status=$?

    kill $PF_PID 2>/dev/null || true

    if [[ "$response" == "200" ]]; then
        green "  PASS: GET /echo → HTTP $response"
        return 0
    else
        fail "GET /echo → HTTP $response (expected 200)"
        return 1
    fi
}

# ── Main ──
main() {
    trap 'cleanup_cluster; if $FAILED; then red "✗ Smoke test FAILED"; else green "✓ Smoke test PASSED"; fi' EXIT

    ensure_cluster
    install_gateway_api_crds
    deploy_gateway
    deploy_backend
    create_route
    send_request
}

main "$@"
