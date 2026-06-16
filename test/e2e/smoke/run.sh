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
CONTROL_PLANE_DEPLOYMENT="nantian-gw-controlplane"
DATA_PLANE_SELECTOR="app=nantian-gw-dataplane"
GATEWAY_CLASS_NAME="${GATEWAY_CLASS_NAME:-nantian-gw}"
GATEWAY_NAME="${GATEWAY_NAME:-nantian-gw}"
GATEWAY_SERVICE="nantian-gw-$GATEWAY_NAME"
SMOKE_CLIENT_POD="smoke-client"
SMOKE_CLIENT_IMAGE="${SMOKE_CLIENT_IMAGE:-docker.io/busybox:1.36.1}"
SMOKE_URL="http://${GATEWAY_SERVICE}.${CONTROL_PLANE_NS}.svc.cluster.local/echo"
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
    kind create cluster --name "$CLUSTER_NAME" --wait 5m
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
  name: $GATEWAY_NAME
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
  - name: $GATEWAY_NAME
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

ensure_smoke_client() {
    kubectl apply -n "$TEST_NS" -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: $SMOKE_CLIENT_POD
spec:
  restartPolicy: Always
  containers:
  - name: client
    image: $SMOKE_CLIENT_IMAGE
    command:
    - sh
    - -c
    - sleep 3600
YAML

    kubectl wait --for=condition=ready pod/$SMOKE_CLIENT_POD -n "$TEST_NS" --timeout="${TIMEOUT}s"
}

# ── Step 6: probe the derived Gateway Service from inside the cluster ──
send_request() {
    echo "=== Sending test request via derived Gateway Service ($SMOKE_URL) ==="

    ensure_smoke_client

    local request_deadline=$((SECONDS + TIMEOUT))
    while (( SECONDS < request_deadline )); do
        if ! kubectl get service -n "$CONTROL_PLANE_NS" "$GATEWAY_SERVICE" &>/dev/null; then
            sleep 2
            continue
        fi

        if kubectl exec -n "$TEST_NS" "$SMOKE_CLIENT_POD" -- wget -q -O - "$SMOKE_URL" >/dev/null 2>&1; then
            green "  PASS: GET /echo via $GATEWAY_SERVICE -> HTTP 200"
            return 0
        fi

        sleep 2
    done

    if ! kubectl get service -n "$CONTROL_PLANE_NS" "$GATEWAY_SERVICE" &>/dev/null; then
        fail "derived Gateway Service $GATEWAY_SERVICE was not created within ${TIMEOUT}s"
        return 1
    fi

    fail "GET /echo via $GATEWAY_SERVICE did not succeed within ${TIMEOUT}s"
    return 1
}

# ── Main ──
main() {
    trap 'exit_code=$?; if [[ "$exit_code" -ne 0 ]]; then FAILED=true; fi; cleanup_cluster; if $FAILED; then red "✗ Smoke test FAILED"; else green "✓ Smoke test PASSED"; fi; exit "$exit_code"' EXIT

    ensure_cluster
    install_gateway_api_crds
    deploy_gateway
    deploy_backend
    create_gateway
    create_reference_grant
    create_route
    send_request
}

main "$@"
