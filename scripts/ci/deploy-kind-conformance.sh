#!/usr/bin/env bash
set -euo pipefail

CONTROLPLANE_IMAGE="${CONTROLPLANE_IMAGE:?CONTROLPLANE_IMAGE is required}"
DATAPLANE_IMAGE="${DATAPLANE_IMAGE:-ghcr.io/nantian-gw/dataplane:latest}"
DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-ghcr.io/nantian-gw/dashboard:latest}"
TIMEOUT="${TIMEOUT:-300s}"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

cp -a configs "$tmpdir/configs"
cp -a deploy "$tmpdir/deploy"
overlay="$tmpdir/deploy/kubernetes/overlays/kind-conformance"
rendered="$tmpdir/kind-conformance.yaml"

(
  cd "$overlay"
  kustomize edit set image "nantian-controlplane=$CONTROLPLANE_IMAGE"
  kustomize edit set image "nantian-dataplane=$DATAPLANE_IMAGE"
  kustomize edit set image "nantian-gw-dashboard=$DASHBOARD_IMAGE"
)

kustomize build "$overlay" --load-restrictor LoadRestrictionsNone >"$rendered"
if ! grep -F "image: $CONTROLPLANE_IMAGE" "$rendered" >/dev/null; then
  echo "rendered manifests do not use expected control-plane image: $CONTROLPLANE_IMAGE" >&2
  exit 1
fi

kubectl apply -f "$rendered"
kubectl wait --for=condition=ready pod --all -n nantian-gw --timeout="$TIMEOUT"
