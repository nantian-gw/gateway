#!/usr/bin/env bash
# e2e test runner — creates a Kind cluster, deploys nantian-gw, runs e2e tests, cleans up.
# Usage: ./run.sh                    # full cycle (bootstrap → test → cleanup)
#        ./run.sh --no-cleanup       # keep cluster running for debugging
#        ./run.sh --skip-bootstrap   # skip cluster creation and deployment (already running)
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATEWAY_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-nantian-e2e}"
KIND_CONFIG="${KIND_CONFIG:-$GATEWAY_ROOT/scripts/ci/kind-ci-config.yaml}"
CLEANUP="true"
BOOTSTRAP="true"

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

export GATEWAY_ROOT
export CLUSTER_NAME
export KIND_CONFIG

if [[ "$BOOTSTRAP" == "true" ]]; then
    if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        echo "=== Creating kind cluster: ${CLUSTER_NAME} ==="
        if [[ ! -f "$KIND_CONFIG" ]]; then
            red "kind config not found: $KIND_CONFIG"
            exit 1
        fi
        kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG" --wait 5m
        kubectl wait --for=condition=ready node --all --timeout=2m
    else
        yellow "Cluster ${CLUSTER_NAME} already exists, using it"
    fi

    if ! kubectl get crd gatewayclasses.gateway.networking.k8s.io &>/dev/null; then
        echo "=== Installing Gateway API CRDs ==="
        BASE="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1"
        kubectl apply -f "$BASE/standard-install.yaml"
        kubectl apply -f "$BASE/experimental-install.yaml" || true
        kubectl wait --for=condition=established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
    else
        yellow "Gateway API CRDs already installed"
    fi

    if ! kubectl get deployment -n nantian-gw nantian-gw-controlplane &>/dev/null; then
        echo "=== Deploying nantian-gw ==="
        kustomize build "$GATEWAY_ROOT/deploy/kubernetes/overlays/kind-conformance" --load-restrictor LoadRestrictionsNone | kubectl apply -f -
        kubectl wait --for=condition=ready pod --all -n nantian-gw --timeout=180s
    else
        yellow "nantian-gw already deployed"
    fi
fi

export SKIP_SETUP=true

echo "=== Running e2e tests ==="
exit_code=0
cd "$GATEWAY_ROOT"
go test -tags=e2e -count=1 -v -timeout 30m ./test/e2e/... || exit_code=$?

if [[ "$CLEANUP" == "true" ]]; then
    echo "=== Cleaning up ==="
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
else
    yellow "Skipping cleanup (--no-cleanup)"
fi

if [[ "$exit_code" -eq 0 ]]; then
    green "✓ e2e tests PASSED"
else
    red "✗ e2e tests FAILED"
fi

exit "$exit_code"
