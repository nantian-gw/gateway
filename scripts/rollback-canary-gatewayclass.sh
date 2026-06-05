#!/usr/bin/env bash
set -euo pipefail

STABLE_GATEWAY_CLASS="${STABLE_GATEWAY_CLASS:-nantian}"
CANARY_GATEWAY_CLASS="${CANARY_GATEWAY_CLASS:-nantian-canary}"
DELETE_CANARY_CLASS="${DELETE_CANARY_CLASS:-true}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-180s}"

log() {
  printf '[rollback-canary-gatewayclass] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

list_canary_gateways() {
  kubectl get gateways.gateway.networking.k8s.io -A \
    -o go-template='{{range .items}}{{if eq .spec.gatewayClassName "'"${CANARY_GATEWAY_CLASS}"'"}}{{.metadata.namespace}}{{"\t"}}{{.metadata.name}}{{"\n"}}{{end}}{{end}}'
}

wait_gateway_ready() {
  local namespace="$1"
  local name="$2"

  kubectl wait --namespace "${namespace}" --for=condition=Accepted "gateway/${name}" --timeout="${WAIT_TIMEOUT}" >/dev/null
  kubectl wait --namespace "${namespace}" --for=condition=Programmed "gateway/${name}" --timeout="${WAIT_TIMEOUT}" >/dev/null
}

main() {
  require_command kubectl

  if ! kubectl get gatewayclass.gateway.networking.k8s.io "${STABLE_GATEWAY_CLASS}" >/dev/null 2>&1; then
    log "stable GatewayClass ${STABLE_GATEWAY_CLASS} not found"
    exit 1
  fi

  local gateways
  gateways="$(list_canary_gateways)"
  if [[ -z "${gateways}" ]]; then
    log "no Gateways currently reference ${CANARY_GATEWAY_CLASS}"
  else
    while IFS=$'\t' read -r namespace name; do
      [[ -n "${namespace}" ]] || continue
      log "switching gateway ${namespace}/${name} back to ${STABLE_GATEWAY_CLASS}"
      kubectl patch "gateway.gateway.networking.k8s.io/${name}" \
        --namespace "${namespace}" \
        --type=merge \
        -p "{\"spec\":{\"gatewayClassName\":\"${STABLE_GATEWAY_CLASS}\"}}" >/dev/null
      wait_gateway_ready "${namespace}" "${name}"
    done <<<"${gateways}"
  fi

  if [[ "${DELETE_CANARY_CLASS}" == "true" ]]; then
    kubectl delete gatewayclass.gateway.networking.k8s.io "${CANARY_GATEWAY_CLASS}" --ignore-not-found >/dev/null
    log "deleted canary GatewayClass ${CANARY_GATEWAY_CLASS}"
  else
    log "kept canary GatewayClass ${CANARY_GATEWAY_CLASS}"
  fi
}

main "$@"
