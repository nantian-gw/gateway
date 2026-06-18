#!/usr/bin/env bash
set -euo pipefail

CONTROL_PLANE_IMAGE="${CONTROL_PLANE_IMAGE:-${CONTROLPLANE_IMAGE:?CONTROL_PLANE_IMAGE is required}}"
DATA_PLANE_IMAGE="${DATA_PLANE_IMAGE:-${DATAPLANE_IMAGE:-ghcr.io/nantian-gw/dataplane:latest}}"
DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-ghcr.io/nantian-gw/dashboard:latest}"
CONFORMANCE_EXPERIMENTAL="${CONFORMANCE_EXPERIMENTAL:-${ALL_FEATURES:-false}}"
TIMEOUT="${TIMEOUT:-300s}"

case "$CONFORMANCE_EXPERIMENTAL" in
  true | false)
    ;;
  *)
    echo "CONFORMANCE_EXPERIMENTAL must be true or false, got: $CONFORMANCE_EXPERIMENTAL" >&2
    exit 1
    ;;
esac

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
  kustomize edit set image "nantian-controlplane=$CONTROL_PLANE_IMAGE"
  kustomize edit set image "nantian-dataplane=$DATA_PLANE_IMAGE"
  kustomize edit set image "nantian-gw-dashboard=$DASHBOARD_IMAGE"
)

if [[ "$CONFORMANCE_EXPERIMENTAL" == "true" ]]; then
  controlplane_config="$overlay/controlplane-config.yaml"
  tmp_config="${controlplane_config}.tmp"
  awk '
    /^[[:space:]]*enableExperimentalGateway:[[:space:]]*/ {
      next
    }
    /^features:[[:space:]]*$/ {
      print
      print "  enableExperimentalGateway: true"
      next
    }
    {
      print
    }
  ' "$overlay/controlplane-config.yaml" >"$tmp_config"
  mv "$tmp_config" "$overlay/controlplane-config.yaml"
fi

kustomize build "$overlay" --load-restrictor LoadRestrictionsNone >"$rendered"
if ! grep -F "image: $CONTROL_PLANE_IMAGE" "$rendered" >/dev/null; then
  echo "rendered manifests do not use expected control-plane image: $CONTROL_PLANE_IMAGE" >&2
  exit 1
fi

kubectl apply -f "$rendered"
kubectl wait --for=condition=available deployment/nantian-gw-controlplane -n nantian-gw --timeout="$TIMEOUT"
kubectl wait --for=condition=available deployment/nantian-gw-dataplane -n nantian-gw --timeout="$TIMEOUT"
