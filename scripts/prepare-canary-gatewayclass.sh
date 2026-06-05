#!/usr/bin/env bash
set -euo pipefail

STABLE_GATEWAY_CLASS="${STABLE_GATEWAY_CLASS:-nantian}"
CANARY_GATEWAY_CLASS="${CANARY_GATEWAY_CLASS:-nantian-canary}"
APPLY="${APPLY:-true}"

log() {
  printf '[prepare-canary-gatewayclass] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

fetch_jsonpath() {
  local resource="$1"
  local jsonpath="$2"

  kubectl get "${resource}" -o "jsonpath=${jsonpath}"
}

main() {
  require_command kubectl

  if ! kubectl get gatewayclass.gateway.networking.k8s.io "${STABLE_GATEWAY_CLASS}" >/dev/null 2>&1; then
    log "stable GatewayClass ${STABLE_GATEWAY_CLASS} not found"
    exit 1
  fi

  local controller_name
  local parameters_group
  local parameters_kind
  local parameters_name
  local parameters_namespace

  controller_name="$(fetch_jsonpath "gatewayclass.gateway.networking.k8s.io/${STABLE_GATEWAY_CLASS}" '{.spec.controllerName}')"
  if [[ -z "${controller_name}" ]]; then
    log "stable GatewayClass ${STABLE_GATEWAY_CLASS} does not expose spec.controllerName"
    exit 1
  fi

  parameters_group="$(fetch_jsonpath "gatewayclass.gateway.networking.k8s.io/${STABLE_GATEWAY_CLASS}" '{.spec.parametersRef.group}')"
  parameters_kind="$(fetch_jsonpath "gatewayclass.gateway.networking.k8s.io/${STABLE_GATEWAY_CLASS}" '{.spec.parametersRef.kind}')"
  parameters_name="$(fetch_jsonpath "gatewayclass.gateway.networking.k8s.io/${STABLE_GATEWAY_CLASS}" '{.spec.parametersRef.name}')"
  parameters_namespace="$(fetch_jsonpath "gatewayclass.gateway.networking.k8s.io/${STABLE_GATEWAY_CLASS}" '{.spec.parametersRef.namespace}')"

  local manifest
  manifest="$(cat <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: ${CANARY_GATEWAY_CLASS}
  labels:
    app.kubernetes.io/managed-by: nantian-gw
    gateway.nantian.dev/release-channel: canary
  annotations:
    gateway.nantian.dev/canary-of: ${STABLE_GATEWAY_CLASS}
spec:
  controllerName: ${controller_name}
EOF
)"

  if [[ -n "${parameters_kind}" && -n "${parameters_name}" ]]; then
    manifest="${manifest}"$'\n'"  parametersRef:"$'\n'
    if [[ -n "${parameters_group}" ]]; then
      manifest="${manifest}    group: ${parameters_group}"$'\n'
    fi
    manifest="${manifest}    kind: ${parameters_kind}"$'\n'
    manifest="${manifest}    name: ${parameters_name}"$'\n'
    if [[ -n "${parameters_namespace}" ]]; then
      manifest="${manifest}    namespace: ${parameters_namespace}"$'\n'
    fi
  fi

  if [[ "${APPLY}" == "true" ]]; then
    printf '%s\n' "${manifest}" | kubectl apply -f -
    log "prepared canary GatewayClass ${CANARY_GATEWAY_CLASS} from ${STABLE_GATEWAY_CLASS}"
  else
    printf '%s\n' "${manifest}"
  fi
}

main "$@"
