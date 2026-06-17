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
DATA_PLANE_SELECTOR="app=nantian-gw-dataplane"
GATEWAY_CLASS_NAME="${GATEWAY_CLASS_NAME:-nantian-gw}"
GATEWAY_NAME="${GATEWAY_NAME:-nantian-gw}"
GATEWAY_SERVICE="nantian-gw-$GATEWAY_NAME"
SMOKE_CLIENT_POD="smoke-client"
SMOKE_CLIENT_IMAGE="${SMOKE_CLIENT_IMAGE:-curlimages/curl:8.16.0}"
SMOKE_URL="http://${GATEWAY_SERVICE}.${CONTROL_PLANE_NS}.svc.cluster.local/echo"
BACKEND_DIRECT_URL="http://echo.${TEST_NS}.svc.cluster.local/echo"
ECHO_PORT=8080
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

trim_response_detail() {
    local value="$1"
    value="${value//$'\r'/}"
    value="$(printf '%s' "$value" | sed 's/[[:space:]]\+/ /g')"
    printf '%.240s' "$value"
}

probe_from_smoke_client() {
    local url="$1"
    local request_timeout="$2"
    local mode="${3:-plain}"

    kubectl exec -n "$TEST_NS" "$SMOKE_CLIENT_POD" -- \
        sh -c '
            body_file=/tmp/smoke-response-body.txt
            rm -f "$body_file"

            curl_flags=""
            if [ "$3" = "insecure" ]; then
                curl_flags="-k"
            fi

            if ! status="$(curl -sS ${curl_flags} --connect-timeout "$1" --max-time "$1" \
                -o "$body_file" -w "%{http_code}" "$2")"; then
                rc=$?
                printf "__CURL_EXIT__%s\n" "$rc"
                [ -f "$body_file" ] && cat "$body_file"
                exit "$rc"
            fi

            printf "__STATUS__%s\n" "$status"
            cat "$body_file"
        ' sh "$request_timeout" "$url" "$mode" 2>&1
}

probe_output_code() {
    printf '%s\n' "$1" | awk 'NR == 1 {sub(/^__STATUS__/, "", $0); print; exit}'
}

probe_output_body() {
    printf '%s\n' "$1" | tail -n +2
}

probe_and_capture() {
    local url="$1"
    local request_timeout="$2"
    local mode="$3"
    local -n code_ref="$4"
    local -n body_ref="$5"
    local -n error_ref="$6"
    local output=""

    code_ref=""
    body_ref=""
    error_ref=""

    if ! output="$(probe_from_smoke_client "$url" "$request_timeout" "$mode")"; then
        error_ref="$output"
        return 1
    fi

    code_ref="$(probe_output_code "$output")"
    body_ref="$(probe_output_body "$output")"
    return 0
}

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

gateway_service_exists() {
    kubectl get service -n "$CONTROL_PLANE_NS" "$GATEWAY_SERVICE" >/dev/null 2>&1
}

gateway_frontend_endpoints_ready() {
    kubectl get endpointslice -n "$CONTROL_PLANE_NS" \
        -l "kubernetes.io/service-name=$GATEWAY_SERVICE" \
        -o jsonpath='{range .items[*].endpoints[*]}{range .addresses[*]}{.}{" "}{end}{.conditions.ready}{"\n"}{end}' \
        2>/dev/null \
        | awk 'NF >= 1 && $NF != "false" {found=1} END {exit(found ? 0 : 1)}'
}

wait_for_gateway_frontend_endpoints() {
    local deadline="$1"

    while (( SECONDS < deadline )); do
        if ! gateway_service_exists; then
            sleep 2
            continue
        fi

        if gateway_frontend_endpoints_ready; then
            return 0
        fi

        sleep 2
    done

    return 1
}

# ── Step 6: probe the derived Gateway Service from inside the cluster ──
send_request() {
    echo "=== Sending test request via derived Gateway Service ($SMOKE_URL) ==="

    ensure_smoke_client

    local request_deadline=$((SECONDS + TIMEOUT))
    local request_timeout="${SMOKE_REQUEST_TIMEOUT_SEC:-5}"
    local last_request_error=""
    local last_response_code=""
    local last_response_body=""
    local https_fallback_url="https://${GATEWAY_SERVICE}.${CONTROL_PLANE_NS}.svc.cluster.local/echo"
    local backend_direct_url="$BACKEND_DIRECT_URL"
    local last_https_fallback_error=""
    local last_https_fallback_code=""
    local last_https_fallback_body=""
    local last_backend_direct_error=""
    local last_backend_direct_code=""
    local last_backend_direct_body=""
    local output=""
    local response_code=""
    local response_body=""

    if ! wait_for_gateway_frontend_endpoints "$request_deadline"; then
        if ! gateway_service_exists; then
            fail "derived Gateway Service $GATEWAY_SERVICE was not created within ${TIMEOUT}s"
            return 1
        fi

        fail "derived Gateway Service $GATEWAY_SERVICE did not expose ready frontend endpoints within ${TIMEOUT}s"
        return 1
    fi

    while (( SECONDS < request_deadline )); do
        output="$(kubectl exec -n "$TEST_NS" "$SMOKE_CLIENT_POD" -- \
            sh -c '
                body_file=/tmp/smoke-response-body.txt
                rm -f "$body_file"
                status="$(curl -sS --connect-timeout "$1" --max-time "$1" \
                    -o "$body_file" -w "%{http_code}" "$2")"
                printf "__STATUS__%s\n" "$status"
                cat "$body_file"
            ' sh "$request_timeout" "$SMOKE_URL" 2>&1)" || {
            last_request_error="$output"
            sleep 2
            continue
        }

        response_code="$(printf '%s\n' "$output" | awk 'NR == 1 {sub(/^__STATUS__/, "", $0); print; exit}')"
        response_body="$(printf '%s\n' "$output" | tail -n +2)"

        last_response_code="$response_code"
        last_response_body="$response_body"

        if [[ "$response_code" == "200" ]]; then
            green "  PASS: GET /echo via $GATEWAY_SERVICE -> HTTP 200"
            return 0
        fi

        last_request_error="HTTP ${response_code}"
        sleep 2
    done

    probe_and_capture \
        "$https_fallback_url" \
        "$request_timeout" \
        insecure \
        last_https_fallback_code \
        last_https_fallback_body \
        last_https_fallback_error || true

    probe_and_capture \
        "$backend_direct_url" \
        "$request_timeout" \
        plain \
        last_backend_direct_code \
        last_backend_direct_body \
        last_backend_direct_error || true

    local detail=""
    if [[ -n "$last_request_error" ]]; then
        detail=$'\nlast request error: '"$last_request_error"
    fi
    if [[ -n "$last_response_code" ]]; then
        detail+=$'\nlast response code: '"$last_response_code"
    fi
    if [[ -n "$last_response_body" ]]; then
        detail+=$'\nlast response body: '"$(trim_response_detail "$last_response_body")"
    fi
    if [[ -n "$last_https_fallback_error" ]]; then
        detail+=$'\nlast https fallback error: '"$(trim_response_detail "$last_https_fallback_error")"
    fi
    if [[ -n "$last_https_fallback_code" ]]; then
        detail+=$'\nlast https fallback code: '"$last_https_fallback_code"
    fi
    if [[ -n "$last_https_fallback_body" ]]; then
        detail+=$'\nlast https fallback body: '"$(trim_response_detail "$last_https_fallback_body")"
    fi
    if [[ -n "$last_backend_direct_error" ]]; then
        detail+=$'\nlast backend direct error: '"$(trim_response_detail "$last_backend_direct_error")"
    fi
    if [[ -n "$last_backend_direct_code" ]]; then
        detail+=$'\nlast backend direct code: '"$last_backend_direct_code"
    fi
    if [[ -n "$last_backend_direct_body" ]]; then
        detail+=$'\nlast backend direct body: '"$(trim_response_detail "$last_backend_direct_body")"
    fi

    fail "GET /echo via $GATEWAY_SERVICE did not succeed within ${TIMEOUT}s${detail}"
    return 1
}

# ── Main ──
main() {
    trap 'exit_code=$?; if [[ "$exit_code" -ne 0 ]]; then FAILED=true; fi; cleanup_cluster; if $FAILED; then red "✗ Smoke test FAILED"; else green "✓ Smoke test PASSED"; fi; exit "$exit_code"' EXIT

    if [[ "$BOOTSTRAP" == "true" ]]; then
        ensure_cluster
        install_gateway_api_crds
        deploy_gateway
    fi
    deploy_backend
    create_gateway
    create_reference_grant
    create_route
    send_request
}

main "$@"
