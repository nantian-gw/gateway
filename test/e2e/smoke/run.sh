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
DATA_PLANE_DEPLOYMENT="nantian-gw-dataplane"
CONTROL_PLANE_IMAGE="${CONTROL_PLANE_IMAGE:-ghcr.io/nantian-gw/nantian-controlplane:latest}"
DATA_PLANE_IMAGE="${DATA_PLANE_IMAGE:-ghcr.io/nantian-gw/dataplane:latest}"
DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-ghcr.io/nantian-gw/dashboard:latest}"
DATA_PLANE_SELECTOR="app=nantian-gw-dataplane"
GATEWAY_CLASS_NAME="${GATEWAY_CLASS_NAME:-nantian-gw}"
ECHO_IMAGE="${ECHO_IMAGE:-registry.k8s.io/e2e-test-images/echoserver:2.5}"
ECHO_PORT=8080
LOCAL_HTTP_PORT="${LOCAL_HTTP_PORT:-10080}"
GATEWAY_HTTP_PORT="${GATEWAY_HTTP_PORT:-80}"
TIMEOUT="${TIMEOUT:-300}"
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

finish() {
    local exit_code=$?
    if (( exit_code != 0 )); then
        FAILED=true
    fi

    cleanup_cluster
    if $FAILED; then
        red "✗ Smoke test FAILED"
    else
        green "✓ Smoke test PASSED"
    fi
    exit "$exit_code"
}

stop_port_forward() {
    local pid="$1"
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
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
    GATEWAY_API_CHANNEL=experimental "$GATEWAY_ROOT/scripts/ci/install-gateway-api-crds.sh"
}

# ── Step 3: preload runtime images ──
preload_image() {
    local image="$1"
    local image_archive

    for attempt in 1 2 3; do
        if docker image inspect "$image" >/dev/null 2>&1 || docker pull "$image"; then
            image_archive="$(mktemp "${TMPDIR:-/tmp}/nantian-kind-image.XXXXXX.tar")"
            if docker save --platform linux/amd64 -o "$image_archive" "$image" \
                && kind load image-archive --name "$CLUSTER_NAME" "$image_archive"; then
                rm -f "$image_archive"
                return
            fi
            rm -f "$image_archive"
        fi

        echo "Failed to preload $image on attempt $attempt; retrying in 10s..." >&2
        sleep 10
    done

    echo "Failed to preload $image after 3 attempts." >&2
    return 1
}

preload_gateway_images() {
    echo "=== Preloading nantian-gw images ==="
    preload_image "$CONTROL_PLANE_IMAGE"
    preload_image "$DATA_PLANE_IMAGE"
    preload_image "$DASHBOARD_IMAGE"
}

# ── Step 4: deploy nantian-gw ──
deploy_gateway() {
    if kubectl get deployment -n "$CONTROL_PLANE_NS" "$CONTROL_PLANE_DEPLOYMENT" &>/dev/null; then
        yellow "nantian-gw already deployed, skipping"
        return
    fi
    echo "=== Deploying nantian-gw ==="
    kustomize build "$GATEWAY_ROOT/deploy/kubernetes/overlays/kind-conformance" --load-restrictor LoadRestrictionsNone | kubectl apply -f -
    kubectl wait --for=condition=available deployment/"$CONTROL_PLANE_DEPLOYMENT" -n "$CONTROL_PLANE_NS" --timeout="${TIMEOUT}s"
    kubectl wait --for=condition=available deployment/"$DATA_PLANE_DEPLOYMENT" -n "$CONTROL_PLANE_NS" --timeout="${TIMEOUT}s"
}

# ── Step 5: preload echo backend image ──
preload_echo_image() {
    echo "=== Preloading echo backend image ==="
    preload_image "$ECHO_IMAGE"
}

# ── Step 6: deploy echo backend ──
deploy_backend() {
    echo "=== Deploying echo backend ==="
    kubectl create namespace "$TEST_NS" --dry-run=client -o yaml | kubectl apply -f -

    kubectl apply -n "$TEST_NS" -f - <<YAML
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
        image: $ECHO_IMAGE
        ports:
        - containerPort: 8080
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
    targetPort: 8080
YAML

    kubectl wait --for=condition=ready pod -l app=echo -n "$TEST_NS" --timeout="${TIMEOUT}s"
    green "  echo backend ready"
}

# ── Step 7: create Gateway, ReferenceGrant, and HTTPRoute ──
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

# ── Step 8: port-forward and send request ──
send_request() {
    local dataplane_target="deployment/$DATA_PLANE_DEPLOYMENT"

    echo "=== Sending test request (port-forward $dataplane_target ${LOCAL_HTTP_PORT}:${GATEWAY_HTTP_PORT}) ==="

    # Start port-forward in background
    kubectl port-forward -n "$CONTROL_PLANE_NS" "$dataplane_target" "${LOCAL_HTTP_PORT}:${GATEWAY_HTTP_PORT}" &>/dev/null &
    PF_PID=$!

    local request_deadline=$((SECONDS + TIMEOUT))
    local response="000"
    while (( SECONDS < request_deadline )); do
        if ! kill -0 "$PF_PID" 2>/dev/null; then
            stop_port_forward "$PF_PID"
            fail "port-forward to $dataplane_target exited before request succeeded"
            return 1
        fi

        response=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:${LOCAL_HTTP_PORT}/echo" 2>/dev/null || echo "000")
        if [[ "$response" == "200" ]]; then
            stop_port_forward "$PF_PID"
            green "  PASS: GET /echo -> HTTP $response"
            return 0
        fi

        sleep 2
    done

    stop_port_forward "$PF_PID"
    fail "GET /echo -> HTTP $response (expected 200 within ${TIMEOUT}s)"
    return 1
}

# ── Main ──
main() {
    trap finish EXIT

    ensure_cluster
    install_gateway_api_crds
    preload_gateway_images
    deploy_gateway
    preload_echo_image
    deploy_backend
    create_gateway
    create_reference_grant
    create_route
    send_request
}

main "$@"
