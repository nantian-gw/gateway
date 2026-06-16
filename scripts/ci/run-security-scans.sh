#!/usr/bin/env bash
set -euo pipefail

SCAN_ROOT="${SCAN_ROOT:-${1:-.}}"
ARTIFACT_DIR="${ARTIFACT_DIR:-tmp/security-scans/latest}"
WORKSPACE_ROOT="$(pwd)"

case "$ARTIFACT_DIR" in
  /*)
    ;;
  *)
    ARTIFACT_DIR="$WORKSPACE_ROOT/$ARTIFACT_DIR"
    ;;
esac

mkdir -p "$ARTIFACT_DIR"
cd "$SCAN_ROOT"

osv-scanner scan source -r . --format json --output-file "$ARTIFACT_DIR/osv-scanner.json"
grype dir:. -o json --file "$ARTIFACT_DIR/grype-dir.json"
kubescape scan framework nsa \
  --format json \
  --format-version v2 \
  --output "$ARTIFACT_DIR/kubescape-nsa.json" \
  deploy/kubernetes/overlays/kind-conformance

{
  echo "Generated security scan artifacts in $ARTIFACT_DIR"
  echo "scan root: $(pwd)"
  echo "osv-scanner: $(command -v osv-scanner)"
  echo "grype: $(command -v grype)"
  echo "kubescape: $(command -v kubescape)"
} >"$ARTIFACT_DIR/summary.txt"
