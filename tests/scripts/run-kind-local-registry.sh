#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT_DIR}/tests/e2e/run-kind.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

DOCKER_LOG="${TMP_DIR}/docker.log"
KUBECTL_LOG="${TMP_DIR}/kubectl.log"

fail() {
  printf '[run-kind-local-registry-test] %s\n' "$*" >&2
  exit 1
}

assert_file_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if ! grep -Fq "${pattern}" "${path}"; then
    fail "${label}: expected '${pattern}' in ${path}"
  fi
}

assert_file_not_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Fq "${pattern}" "${path}"; then
    fail "${label}: did not expect '${pattern}' in ${path}"
  fi
}

sleep() {
  :
}

curl() {
  printf 'curl %s\n' "$*" >>"${DOCKER_LOG}"
  return 0
}

docker() {
  printf 'docker %s\n' "$*" >>"${DOCKER_LOG}"

  case "$*" in
    "image inspect registry:2")
      return 0
      ;;
    "inspect kind-registry")
      return 0
      ;;
    "inspect -f {{.State.Running}} kind-registry")
      printf 'true\n'
      return 0
      ;;
    "exec kind-registry sh -c "*)
      if [[ -f "${TMP_DIR}/registry.recreated" ]]; then
        return 0
      fi
      return 1
      ;;
    "rm -f kind-registry")
      return 0
      ;;
    "run -d "*)
      : >"${TMP_DIR}/registry.recreated"
      printf 'registry-container-id\n'
      return 0
      ;;
  esac

  return 1
}

kubectl() {
  printf 'kubectl %s\n' "$*" >>"${KUBECTL_LOG}"

  case "$*" in
    "--context kind-nantian-gw -n kube-system get daemonset kindnet -o jsonpath={.spec.template.spec.containers[0].resources}")
      printf '{"limits":{"cpu":"100m","memory":"50Mi"},"requests":{"cpu":"100m","memory":"50Mi"}}'
      return 0
      ;;
    "--context kind-nantian-gw -n kube-system patch daemonset kindnet --type=json -p="*)
      return 0
      ;;
    "--context kind-nantian-gw -n kube-system rollout status daemonset/kindnet --timeout=120s")
      return 0
      ;;
  esac

  return 1
}

: >"${DOCKER_LOG}"
if ! RUN_KIND_SOURCE_ONLY=true source "${SCRIPT}" >"${TMP_DIR}/source.log" 2>&1; then
  cat "${TMP_DIR}/source.log" >&2
  fail "expected run-kind.sh to support source-only loading for shell tests"
fi

assert_file_not_contains "${DOCKER_LOG}" "docker " \
  "source-only mode should not execute docker operations"

KIND_CONFIG="${ROOT_DIR}/deploy/kubernetes/overlays/kind/kind-config.yaml"
BASE_CONTROLPLANE_CONFIG="${ROOT_DIR}/deploy/kubernetes/base/controlplane-config.yaml"
assert_file_contains "${KIND_CONFIG}" "containerPort: 32000" \
  "kind config should expose TCPRoute success NodePort"
assert_file_contains "${KIND_CONFIG}" "hostPort: 19000" \
  "kind config should expose TCPRoute success host port"
assert_file_contains "${KIND_CONFIG}" "containerPort: 32001" \
  "kind config should expose TCPRoute failure NodePort"
assert_file_contains "${KIND_CONFIG}" "hostPort: 19001" \
  "kind config should expose TCPRoute failure host port"
assert_file_contains "${KIND_CONFIG}" "containerPort: 31301" \
  "kind config should expose UDPRoute failure NodePort"
assert_file_contains "${KIND_CONFIG}" "hostPort: 5301" \
  "kind config should expose UDPRoute failure host port"
assert_file_not_contains "${BASE_CONTROLPLANE_CONFIG}" 'statusAddress: "127.0.0.1"' \
  "base controlplane config should not publish loopback statusAddress"
assert_file_contains "${SCRIPT}" '$bindings["32000/tcp"][0].HostPort == "19000"' \
  "cluster reuse check should require TCPRoute success mapping"
assert_file_contains "${SCRIPT}" '$bindings["32001/tcp"][0].HostPort == "19001"' \
  "cluster reuse check should require TCPRoute failure mapping"
assert_file_contains "${SCRIPT}" '$bindings["31301/udp"][0].HostPort == "5301"' \
  "cluster reuse check should require UDPRoute failure mapping"

: >"${DOCKER_LOG}"
if ! ensure_local_registry >"${TMP_DIR}/ensure.log" 2>&1; then
  cat "${TMP_DIR}/ensure.log" >&2
  fail "expected ensure_local_registry to repair unwritable registry storage"
fi

assert_file_contains "${DOCKER_LOG}" \
  "docker exec kind-registry sh -c" \
  "check registry storage writability"
assert_file_contains "${DOCKER_LOG}" \
  "docker rm -f kind-registry" \
  "remove registry with broken storage"
assert_file_contains "${DOCKER_LOG}" \
  "docker run -d" \
  "create replacement registry"
assert_file_contains "${TMP_DIR}/ensure.log" \
  "storage is not writable; recreating registry" \
  "log registry storage repair"

: >"${KUBECTL_LOG}"
if ! ensure_kindnet_resources_unlimited >"${TMP_DIR}/kindnet.log" 2>&1; then
  cat "${TMP_DIR}/kindnet.log" >&2
  fail "expected run-kind.sh to remove kindnet resource limits"
fi

assert_file_contains "${KUBECTL_LOG}" \
  "kubectl --context kind-nantian-gw -n kube-system get daemonset kindnet -o jsonpath={.spec.template.spec.containers[0].resources}" \
  "inspect kindnet resources"
assert_file_contains "${KUBECTL_LOG}" \
  "kubectl --context kind-nantian-gw -n kube-system patch daemonset kindnet --type=json -p=" \
  "patch kindnet resources"
assert_file_contains "${KUBECTL_LOG}" \
  "kubectl --context kind-nantian-gw -n kube-system rollout status daemonset/kindnet --timeout=120s" \
  "wait for kindnet rollout"
assert_file_contains "${TMP_DIR}/kindnet.log" \
  "removing kindnet resource requests/limits for performance tests" \
  "log kindnet resource patch"

printf '[run-kind-local-registry-test] ok\n'
