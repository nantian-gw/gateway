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
run_capture nantian-images kubectl get pods -n nantian-gw -o json

if kubectl get pods -n nantian-gw -o json >"$ARTIFACT_DIR/nantian-images.json" 2>/dev/null; then
  if command -v jq >/dev/null 2>&1; then
    jq -r '.items[]? | .spec.containers[]? | "image \(.name): \(.image)"' "$ARTIFACT_DIR/nantian-images.json" >"$ARTIFACT_DIR/image-names.txt" 2>/dev/null || true
    jq -r '.items[]? | .status.containerStatuses[]? | "imageID \(.name): \(.imageID)"' "$ARTIFACT_DIR/nantian-images.json" >"$ARTIFACT_DIR/image-ids.txt" 2>/dev/null || true
    jq -r '.items[]? | .status.containerStatuses[]? | select(.imageID != "") | "digest \(.name): \(.imageID | sub("^.*@"; "") | sub("^.*://"; ""))"' "$ARTIFACT_DIR/nantian-images.json" >"$ARTIFACT_DIR/image-digests.txt" 2>/dev/null || true
  fi
fi

find "$ARTIFACT_DIR" -maxdepth 1 -type f -print | sort
