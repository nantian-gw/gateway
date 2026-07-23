#!/usr/bin/env bash
set -euo pipefail

KIND_VERSION="${KIND_VERSION:-v0.27.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-v1.32.2}"
KUSTOMIZE_VERSION="${KUSTOMIZE_VERSION:-v5.7.1}"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

sudo apt-get update
sudo apt-get install -y --no-install-recommends ca-certificates curl jq

# curl with retry for transient network errors
curl_with_retry() {
  local url="$1" out="$2"
  curl -fsSL --retry 3 --retry-delay 5 --retry-max-time 60 "$url" -o "$out"
}

curl_with_retry "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64" "$tmpdir/kind"
chmod +x "$tmpdir/kind"
sudo install -m 0755 "$tmpdir/kind" "$BIN_DIR/kind"

curl_with_retry "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" "$tmpdir/kubectl"
chmod +x "$tmpdir/kubectl"
sudo install -m 0755 "$tmpdir/kubectl" "$BIN_DIR/kubectl"

curl_with_retry "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz" "$tmpdir/kustomize.tar.gz"
tar -xzf "$tmpdir/kustomize.tar.gz" -C "$tmpdir"
chmod +x "$tmpdir/kustomize"
sudo install -m 0755 "$tmpdir/kustomize" "$BIN_DIR/kustomize"

kind version
kubectl version --client=true
kustomize version
jq --version
