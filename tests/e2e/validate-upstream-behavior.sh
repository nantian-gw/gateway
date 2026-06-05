#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-aether-gateway}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
LOCAL_REGISTRY_NAME="${LOCAL_REGISTRY_NAME:-kind-registry}"
LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
LOCAL_REGISTRY_HOST="${LOCAL_REGISTRY_HOST:-localhost:${LOCAL_REGISTRY_PORT}}"
LOCAL_REGISTRY_PUSH_HOST="${LOCAL_REGISTRY_PUSH_HOST:-127.0.0.1:${LOCAL_REGISTRY_PORT}}"
TEST_NAMESPACE="${TEST_NAMESPACE:-aether-upstream-behavior}"
POOL_HOST="${POOL_HOST:-pool.example.com}"
STREAM_HOST="${STREAM_HOST:-stream.example.com}"
RETRY_HOST="${RETRY_HOST:-retry.example.com}"
TIMEOUT_HOST="${TIMEOUT_HOST:-timeout.example.com}"
WEIGHT_HOST="${WEIGHT_HOST:-weight.example.com}"
RECOVER_HOST="${RECOVER_HOST:-recover.example.com}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
ADMIN_FORWARD_PORT="${ADMIN_FORWARD_PORT:-29080}"
STREAM_IDLE_GAP_MS="${STREAM_IDLE_GAP_MS:-1500}"
STREAM_CURL_MAX_TIME_SEC="${STREAM_CURL_MAX_TIME_SEC:-8}"
PROFILE_OUTPUT_DIR="${PROFILE_OUTPUT_DIR:-}"
STREAM_PROFILE_REQUESTS="${STREAM_PROFILE_REQUESTS:-6}"
STREAM_PROFILE_CONCURRENCY="${STREAM_PROFILE_CONCURRENCY:-3}"
STREAM_PROFILE_REQUEST_TIMEOUT="${STREAM_PROFILE_REQUEST_TIMEOUT:-${STREAM_CURL_MAX_TIME_SEC}}"
ENSURE_KIND="${ENSURE_KIND:-false}"
KEEP_RESOURCES="${KEEP_RESOURCES:-false}"
DATAPLANE_NAMESPACE="${DATAPLANE_NAMESPACE:-aether-gateway}"
DATAPLANE_DEPLOYMENT="${DATAPLANE_DEPLOYMENT:-aether-gateway-dataplane}"
PYTHON_SOURCE_IMAGE="${PYTHON_SOURCE_IMAGE:-m.daocloud.io/docker.io/library/python:3.12-slim-bookworm}"
PYTHON_IMAGE="${PYTHON_IMAGE:-${LOCAL_REGISTRY_HOST}/aether-gateway-validation/python-ws:3.12-slim-bookworm}"

TMP_DIR=""
PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""
ORIGINAL_DATAPLANE_REPLICAS=""
SUCCESS="false"


source "${ROOT_DIR}/tests/e2e/validate-upstream-behavior-lib.sh"
source "${ROOT_DIR}/tests/e2e/validate-upstream-behavior-streaming-lib.sh"
source "${ROOT_DIR}/tests/e2e/validate-upstream-behavior-resources.sh"

main() {
  local before_summary
  local before_traffic
  local after_summary
  local after_traffic

  require_command curl
  require_command docker
  require_command jq
  require_command kind
  require_command kubectl
  require_command python3

  ensure_kind_cluster
  sync_test_images
  TMP_DIR="$(mktemp -d "${ROOT_DIR}/tmp/upstream-behavior.XXXXXX")"
  trap cleanup EXIT

  cleanup_namespace
  apply_resources
  wait_for_deployment pool-backend
  wait_for_deployment streaming-backend
  wait_for_deployment retry-failing
  wait_for_deployment retry-healthy
  wait_for_deployment weighted-a
  wait_for_deployment weighted-b
  wait_for_deployment recover-a
  wait_for_deployment recover-b
  wait_for_deployment slow-backend
  wait_for_deployment fast-backend
  wait_for_service_endpoints pool-backend 1
  wait_for_service_endpoints streaming-backend 1
  wait_for_service_endpoints retry-failing 1
  wait_for_service_endpoints retry-healthy 1
  wait_for_service_endpoints weighted-a 1
  wait_for_service_endpoints weighted-b 1
  wait_for_service_endpoints recover-a 1
  wait_for_service_endpoints recover-b 1
  wait_for_service_endpoints slow-backend 1
  wait_for_service_endpoints fast-backend 1
  wait_for_gateway_ready
  wait_for_route_acceptance pool-route
  wait_for_route_acceptance streaming-route
  wait_for_route_acceptance retry-route
  wait_for_route_acceptance timeout-route
  wait_for_route_acceptance weighted-route
  wait_for_route_acceptance recover-route
  ensure_single_dataplane_replica
  start_admin_port_forward

  before_summary="$(summary_json)"
  before_traffic="$(traffic_json)"

  validate_keepalive_reuse
  validate_streaming_http
  write_streaming_profiles
  validate_retry_failover
  validate_timeout_failover
  validate_weighted_distribution
  validate_weight_convergence
  validate_backend_recovery
  validate_metrics_endpoint

  after_summary="$(summary_json)"
  after_traffic="$(traffic_json)"
  print_derived_metrics "${before_summary}" "${after_summary}" "${before_traffic}" "${after_traffic}"

  SUCCESS="true"
  log "upstream behavior validation passed"
}

main "$@"
