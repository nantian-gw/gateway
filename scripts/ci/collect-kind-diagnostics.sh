#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_DIR="${ARTIFACT_DIR:-tmp/kind-diagnostics}"
mkdir -p "$ARTIFACT_DIR"

run_capture() {
  local name="$1"
  shift
  {
    echo "\$ $*"
    "$@"
  } >"$ARTIFACT_DIR/${name}.txt" 2>&1 || true
}

run_capture pods-all kubectl get pods -A -o wide
run_capture events-all kubectl get events -A --sort-by=.lastTimestamp
run_capture nantian-pods kubectl describe pods -n nantian-gw
run_capture nantian-logs kubectl logs -n nantian-gw -l app.kubernetes.io/part-of=nantian-gw --all-containers=true --tail=300 --prefix=true
run_capture service-topology kubectl get svc,endpoints,endpointslice -A -o yaml
run_capture gateway-resources kubectl get gateway,httproute -A -o yaml
run_capture listenerset-resources kubectl get listenersets.gateway.networking.k8s.io -A -o yaml
run_capture nantian-images kubectl get pods -n nantian-gw -o json

if kubectl get pods -n nantian-gw -o json >"$ARTIFACT_DIR/nantian-images.json" 2>/dev/null; then
  if command -v jq >/dev/null 2>&1; then
    jq -r '.items[]? | .spec.containers[]? | "image \(.name): \(.image)"' "$ARTIFACT_DIR/nantian-images.json" >"$ARTIFACT_DIR/image-names.txt" 2>/dev/null || true
    jq -r '.items[]? | .status.containerStatuses[]? | "imageID \(.name): \(.imageID)"' "$ARTIFACT_DIR/nantian-images.json" >"$ARTIFACT_DIR/image-ids.txt" 2>/dev/null || true
    jq -r '.items[]? | .status.containerStatuses[]? | select(.imageID != "") | "digest \(.name): \(.imageID | sub("^.*@"; "") | sub("^.*://"; ""))"' "$ARTIFACT_DIR/nantian-images.json" >"$ARTIFACT_DIR/image-digests.txt" 2>/dev/null || true
  fi
fi

admin_token() {
  local secret_name="$1"

  kubectl get secret -n nantian-gw "$secret_name" -o jsonpath='{.data.token}' 2>/dev/null | base64 -d 2>/dev/null || true
}

capture_admin_endpoint() {
  local prefix="$1"
  local service_name="$2"
  local service_port="$3"
  local local_port="$4"
  local token="$5"
  shift 5

  local port_forward_log="$ARTIFACT_DIR/${prefix}-admin-port-forward.txt"
  kubectl -n nantian-gw port-forward --address 127.0.0.1 "svc/${service_name}" "${local_port}:${service_port}" >"$port_forward_log" 2>&1 &
  local port_forward_pid=$!

  local ready=false
  for _ in $(seq 1 40); do
    if curl -fsS --max-time 2 "http://127.0.0.1:${local_port}/livez" >/dev/null 2>&1; then
      ready=true
      break
    fi
    sleep 0.25
  done

  if [[ "$ready" != true ]]; then
    echo "admin port-forward for ${service_name} did not become ready" >"$ARTIFACT_DIR/${prefix}-admin-unavailable.txt"
    kill "$port_forward_pid" >/dev/null 2>&1 || true
    wait "$port_forward_pid" >/dev/null 2>&1 || true
    return
  fi

  local curl_args=(-fsS --max-time 10)
  if [[ -n "$token" ]]; then
    curl_args+=(-H "Authorization: Bearer ${token}")
  fi

  local endpoint
  for endpoint in "$@"; do
    local safe="${endpoint#/}"
    safe="${safe//\//-}"
    safe="${safe//\?/-}"
    curl "${curl_args[@]}" "http://127.0.0.1:${local_port}${endpoint}" >"$ARTIFACT_DIR/${prefix}-${safe}.json" 2>"$ARTIFACT_DIR/${prefix}-${safe}.err" || true
  done

  kill "$port_forward_pid" >/dev/null 2>&1 || true
  wait "$port_forward_pid" >/dev/null 2>&1 || true
}

controlplane_token="$(admin_token nantian-gw-controlplane-admin-auth)"
dataplane_token="$(admin_token nantian-gw-dataplane-admin-auth)"
capture_admin_endpoint controlplane nantian-gw-controlplane-admin 18081 41801 "$controlplane_token" \
  /v1/summary \
  /v1/snapshot-sync \
  /v1/snapshot \
  /v1/listeners \
  /v1/routes \
  /v1/backends
capture_admin_endpoint dataplane nantian-gw-dataplane-admin 19080 41802 "$dataplane_token" \
  /v1/summary \
  /v1/snapshot \
  /v1/listeners \
  /v1/routes \
  /v1/backends

find "$ARTIFACT_DIR" -maxdepth 1 -type f -print | sort
