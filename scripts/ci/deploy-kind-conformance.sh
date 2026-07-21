#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=./dependency-images.sh
source "$SCRIPT_DIR/dependency-images.sh"

CONTROLPLANE_IMAGE="${CONTROLPLANE_IMAGE:?CONTROLPLANE_IMAGE is required}"
DATAPLANE_IMAGE="${DATAPLANE_IMAGE:-$DEFAULT_DATAPLANE_IMAGE}"
DASHBOARD_IMAGE="${DASHBOARD_IMAGE:-$DEFAULT_DASHBOARD_IMAGE}"
KIND_DATAPLANE_IMAGE="${KIND_DATAPLANE_IMAGE:-$(kind_runtime_image_ref "$DATAPLANE_IMAGE")}"
KIND_DASHBOARD_IMAGE="${KIND_DASHBOARD_IMAGE:-$(kind_runtime_image_ref "$DASHBOARD_IMAGE")}"
CONFORMANCE_EXPERIMENTAL="${CONFORMANCE_EXPERIMENTAL:-${ALL_FEATURES:-false}}"
TIMEOUT="${TIMEOUT:-600s}"

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
  kustomize edit set image "nantian-controlplane=$CONTROLPLANE_IMAGE"
  kustomize edit set image "nantian-dataplane=$KIND_DATAPLANE_IMAGE"
  kustomize edit set image "nantian-gw-dashboard=$KIND_DASHBOARD_IMAGE"
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

for expected in \
  "image: $CONTROLPLANE_IMAGE" \
  "image: $KIND_DATAPLANE_IMAGE" \
  "image: $KIND_DASHBOARD_IMAGE"; do
  if ! grep -F "$expected" "$rendered" >/dev/null; then
    echo "rendered manifests do not use expected image: $expected" >&2
    exit 1
  fi
done

kubectl apply -f "$rendered"
kubectl wait --for=condition=ready pod --all -n nantian-gw --timeout="$TIMEOUT"
