#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RBAC_FILE="${RBAC_FILE:-${ROOT_DIR}/deploy/kubernetes/base/rbac.yaml}"
REQUIRED_FILE="${REQUIRED_FILE:-${ROOT_DIR}/docs/security/controlplane-rbac-required.json}"

usage() {
  cat <<'EOF'
Usage: scripts/audit-controlplane-rbac.sh [--check]

Audits the controlplane ClusterRole against docs/security/controlplane-rbac-required.json
and verifies watched resources discovered from controller setup have get/list/watch
permissions documented and granted.

Environment:
  RBAC_FILE       Path to the rendered or source RBAC YAML.
  REQUIRED_FILE   Path to the machine-readable required permission baseline.
EOF
}

case "${1:---check}" in
  --check)
    ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

python3 - "${ROOT_DIR}" "${RBAC_FILE}" "${REQUIRED_FILE}" <<'PY'
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
rbac_file = Path(sys.argv[2])
required_file = Path(sys.argv[3])

WATCH_VERBS = {"get", "list", "watch"}
GO_TYPE_TO_RBAC = {
    "corev1.Service": ("", "services"),
    "corev1.Pod": ("", "pods"),
    "corev1.Namespace": ("", "namespaces"),
    "corev1.Secret": ("", "secrets"),
    "corev1.ConfigMap": ("", "configmaps"),
    "discoveryv1.EndpointSlice": ("discovery.k8s.io", "endpointslices"),
    "gatewayv1.GatewayClass": ("gateway.networking.k8s.io", "gatewayclasses"),
    "gatewayv1.Gateway": ("gateway.networking.k8s.io", "gateways"),
    "gatewayv1.HTTPRoute": ("gateway.networking.k8s.io", "httproutes"),
    "gatewayv1.GRPCRoute": ("gateway.networking.k8s.io", "grpcroutes"),
    "gatewayv1.ListenerSet": ("gateway.networking.k8s.io", "listenersets"),
    "gatewayv1alpha2.TCPRoute": ("gateway.networking.k8s.io", "tcproutes"),
    "gatewayv1alpha2.UDPRoute": ("gateway.networking.k8s.io", "udproutes"),
    "gatewayv1alpha2.TLSRoute": ("gateway.networking.k8s.io", "tlsroutes"),
    "gatewayv1beta1.ReferenceGrant": ("gateway.networking.k8s.io", "referencegrants"),
    "backendlbv1alpha2.BackendLBPolicy": ("gateway.networking.k8s.io", "backendlbpolicies"),
    "mcsv1alpha1.ServiceImport": ("multicluster.x-k8s.io", "serviceimports"),
}
DYNAMIC_WATCH_MARKERS = {
    "gatewayapi.NewBackendTLSPolicyV1Object()": ("gateway.networking.k8s.io", "backendtlspolicies"),
}


def parse_inline_list(raw: str) -> list[str]:
    try:
        value = json.loads(raw.strip())
    except json.JSONDecodeError as err:
        raise SystemExit(f"failed to parse RBAC inline list {raw!r}: {err}") from err
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise SystemExit(f"RBAC inline list must contain only strings: {raw!r}")
    return value


def parse_rbac_rules(path: Path) -> dict[tuple[str, str], set[str]]:
    text = path.read_text()
    rules: dict[tuple[str, str], set[str]] = {}
    pattern = re.compile(
        r"  - apiGroups: (?P<api_groups>\[[^\n]+\])\n"
        r"    resources: (?P<resources>\[[^\n]+\])\n"
        r"    verbs: (?P<verbs>\[[^\n]+\])"
    )
    for match in pattern.finditer(text):
        api_groups = parse_inline_list(match.group("api_groups"))
        resources = parse_inline_list(match.group("resources"))
        verbs = set(parse_inline_list(match.group("verbs")))
        for api_group in api_groups:
            for resource in resources:
                rules.setdefault((api_group, resource), set()).update(verbs)
    return rules


def load_required(path: Path) -> dict[tuple[str, str], dict[str, object]]:
    data = json.loads(path.read_text())
    permissions = data.get("permissions")
    if not isinstance(permissions, list):
        raise SystemExit("required RBAC baseline must contain a permissions list")

    out: dict[tuple[str, str], dict[str, object]] = {}
    for index, item in enumerate(permissions):
        if not isinstance(item, dict):
            raise SystemExit(f"permission entry {index} must be an object")
        api_group = item.get("apiGroup")
        resource = item.get("resource")
        verbs = item.get("verbs")
        purpose = item.get("purpose")
        sources = item.get("sources")
        if not isinstance(api_group, str) or not isinstance(resource, str):
            raise SystemExit(f"permission entry {index} must define apiGroup and resource")
        if not isinstance(verbs, list) or not all(isinstance(verb, str) for verb in verbs):
            raise SystemExit(f"permission entry {api_group}/{resource} must define string verbs")
        if verbs != sorted(set(verbs)):
            raise SystemExit(f"permission entry {api_group}/{resource} verbs must be sorted and unique")
        if not isinstance(purpose, str) or not purpose.strip():
            raise SystemExit(f"permission entry {api_group}/{resource} must document purpose")
        if not isinstance(sources, list) or not sources or not all(isinstance(source, str) for source in sources):
            raise SystemExit(f"permission entry {api_group}/{resource} must document source files")
        key = (api_group, resource)
        if key in out:
            raise SystemExit(f"duplicate permission entry for {api_group}/{resource}")
        out[key] = {**item, "verbs": set(verbs)}
    return out


def discover_watched_resources(root: Path) -> set[tuple[str, str]]:
    watched: set[tuple[str, str]] = set()
    source_paths = [
        root / "controlplane/internal/controller/reconciler.go",
        root / "controlplane/internal/status/controllers.go",
    ]
    pattern = re.compile(r"(?:^|[.\s])(?:For|Watches)\(\s*&([A-Za-z0-9_\.]+)\{\}", re.S)
    for source_path in source_paths:
        text = source_path.read_text()
        for go_type in pattern.findall(text):
            if go_type not in GO_TYPE_TO_RBAC:
                raise SystemExit(f"unmapped watched Go type {go_type} in {source_path}")
            watched.add(GO_TYPE_TO_RBAC[go_type])
        for marker, resource in DYNAMIC_WATCH_MARKERS.items():
            if marker in text:
                watched.add(resource)
    return watched


actual = parse_rbac_rules(rbac_file)
required = load_required(required_file)
watched = discover_watched_resources(root)
errors: list[str] = []

for key, meta in sorted(required.items()):
    missing = set(meta["verbs"]) - actual.get(key, set())
    if missing:
        api_group, resource = key
        errors.append(
            f"missing RBAC verb for {api_group or 'core'}/{resource}: {', '.join(sorted(missing))}"
        )

for key, actual_verbs in sorted(actual.items()):
    api_group, resource = key
    if key not in required:
        errors.append(f"undocumented RBAC resource {api_group or 'core'}/{resource}")
        continue
    extra = actual_verbs - set(required[key]["verbs"])
    if extra:
        errors.append(
            f"undocumented RBAC verb for {api_group or 'core'}/{resource}: {', '.join(sorted(extra))}"
        )

for key in sorted(watched):
    api_group, resource = key
    required_verbs = set(required.get(key, {}).get("verbs", set()))
    actual_verbs = actual.get(key, set())
    missing_required = WATCH_VERBS - required_verbs
    missing_actual = WATCH_VERBS - actual_verbs
    if missing_required:
        errors.append(
            f"watched resource {api_group or 'core'}/{resource} lacks documented verbs: "
            f"{', '.join(sorted(missing_required))}"
        )
    if missing_actual:
        errors.append(
            f"watched resource {api_group or 'core'}/{resource} lacks granted verbs: "
            f"{', '.join(sorted(missing_actual))}"
        )

if errors:
    for error in errors:
        print(f"[controlplane-rbac-audit] {error}", file=sys.stderr)
    sys.exit(1)

print(
    "[controlplane-rbac-audit] controlplane RBAC audit passed "
    f"({len(required)} documented resources, {len(watched)} watched resources)"
)
PY
