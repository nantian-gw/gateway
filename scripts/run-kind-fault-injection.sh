#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/common.sh"
source "${ROOT_DIR}/scripts/lib/kind-evidence.sh"
RUN_ID="${RUN_ID:-$(date +%Y-%m-%d-%H%M%S)-$(git -C "${ROOT_DIR}" rev-parse --short HEAD)-kind-faults}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/reports/chaos/runs/${RUN_ID}}"
GATEWAY_HOST_PORT="${GATEWAY_HOST_PORT:-18080}"
HTTP_HOST="${HTTP_HOST:-example.com}"
HTTP_CLIENT="${ROOT_DIR}/tests/e2e/http_concurrency_client.py"
RUN_KIND_SCRIPT="${RUN_KIND_SCRIPT:-${ROOT_DIR}/tests/e2e/run-kind.sh}"
FAULT_HTTP_BACKEND_REPLICAS="${FAULT_HTTP_BACKEND_REPLICAS:-2}"
TRAFFIC_BATCH_REQUESTS="${TRAFFIC_BATCH_REQUESTS:-200}"
TRAFFIC_BATCH_CONCURRENCY="${TRAFFIC_BATCH_CONCURRENCY:-16}"
SUMMARY_ONLY="${SUMMARY_ONLY:-false}"
REQUIRED_CONCLUSION_SCENARIOS="${REQUIRED_CONCLUSION_SCENARIOS:-controlplane-leader-switch dataplane-pod-restart node-drain apiserver-watch-disruption}"
REQUIRE_RELEASE_GATE_CONCLUSIONS="${REQUIRE_RELEASE_GATE_CONCLUSIONS:-false}"
SKIP_NODE_DRAIN="${SKIP_NODE_DRAIN:-true}"
SKIP_APISERVER_WATCH_DISRUPTION="${SKIP_APISERVER_WATCH_DISRUPTION:-true}"
NODE_DRAIN_KIND_WORKER_NODES="${NODE_DRAIN_KIND_WORKER_NODES:-${KIND_WORKER_NODES:-2}}"
NODE_DRAIN_TIMEOUT="${NODE_DRAIN_TIMEOUT:-180s}"
APISERVER_WATCH_CHURN_ITERATIONS="${APISERVER_WATCH_CHURN_ITERATIONS:-3}"
APISERVER_WATCH_CHURN_ANNOTATION="${APISERVER_WATCH_CHURN_ANNOTATION:-nantian.dev/fault-watch-churn}"
MIN_SUCCESS_RATE="${MIN_SUCCESS_RATE:-1.0}"
MAX_ERRORS="${MAX_ERRORS:-0}"
MAX_P99_MS="${MAX_P99_MS:-5000}"
MAX_LATENCY_MS="${MAX_LATENCY_MS:-30000}"
SLO_GATE_RISK_ACCEPTED="${SLO_GATE_RISK_ACCEPTED:-false}"
TRAFFIC_PID=""
FAILURES=0

log() {
  aeg_kind_log "kind-fault-injection" "$*"
}

require_command() {
  aeg_require_command "kind-fault-injection" "$1"
}

ensure_stack() {
  aeg_kind_ensure_stack "kind-fault-injection" "${ROOT_DIR}" "${HTTP_HOST}" "${GATEWAY_HOST_PORT}"
}

metadata_value() {
  local key="$1"
  local metadata="${OUTPUT_DIR}/metadata.txt"

  if [[ ! -f "${metadata}" ]]; then
    return
  fi

  awk -F= -v key="${key}" '
    $1 == key {
      sub(/^[^=]*=/, "")
      print
      exit
    }
  ' "${metadata}"
}

metadata_or_default() {
  local key="$1"
  local default_value="$2"
  local value
  value="$(metadata_value "${key}")"
  if [[ -n "${value}" ]]; then
    printf '%s\n' "${value}"
  else
    printf '%s\n' "${default_value}"
  fi
}

write_metadata() {
  local metadata="${OUTPUT_DIR}/metadata.txt"
  local tmp="${metadata}.tmp"

  mkdir -p "${OUTPUT_DIR}"
  {
    printf 'captured_at=%s\n' "$(metadata_or_default "captured_at" "$(date --iso-8601=seconds)")"
    printf 'git_commit=%s\n' "$(metadata_or_default "git_commit" "$(git -C "${ROOT_DIR}" rev-parse HEAD)")"
    printf 'git_tree_state=%s\n' "$(metadata_or_default "git_tree_state" "$(aeg_git_tree_state "${ROOT_DIR}")")"
    printf 'code_tree_state=%s\n' "$(metadata_or_default "code_tree_state" "$(aeg_code_tree_state "${ROOT_DIR}")")"
    printf 'run_id=%s\n' "$(metadata_or_default "run_id" "${RUN_ID}")"
    printf 'output_dir=%s\n' "$(metadata_or_default "output_dir" "${OUTPUT_DIR}")"
    printf 'gateway_host_port=%s\n' "$(metadata_or_default "gateway_host_port" "${GATEWAY_HOST_PORT}")"
    printf 'http_host=%s\n' "$(metadata_or_default "http_host" "${HTTP_HOST}")"
    printf 'traffic_batch_requests=%s\n' "$(metadata_or_default "traffic_batch_requests" "${TRAFFIC_BATCH_REQUESTS}")"
    printf 'traffic_batch_concurrency=%s\n' "$(metadata_or_default "traffic_batch_concurrency" "${TRAFFIC_BATCH_CONCURRENCY}")"
    printf 'required_conclusion_scenarios=%s\n' "$(metadata_or_default "required_conclusion_scenarios" "${REQUIRED_CONCLUSION_SCENARIOS}")"
    printf 'require_release_gate_conclusions=%s\n' "${REQUIRE_RELEASE_GATE_CONCLUSIONS}"
    printf 'skip_node_drain=%s\n' "$(metadata_or_default "skip_node_drain" "${SKIP_NODE_DRAIN}")"
    printf 'skip_apiserver_watch_disruption=%s\n' "$(metadata_or_default "skip_apiserver_watch_disruption" "${SKIP_APISERVER_WATCH_DISRUPTION}")"
    printf 'min_success_rate=%s\n' "${MIN_SUCCESS_RATE}"
    printf 'max_errors=%s\n' "${MAX_ERRORS}"
    printf 'max_p99_ms=%s\n' "${MAX_P99_MS}"
    printf 'max_latency_ms=%s\n' "${MAX_LATENCY_MS}"
    printf 'slo_gate_risk_accepted=%s\n' "${SLO_GATE_RISK_ACCEPTED}"
  } >"${tmp}"
  mv "${tmp}" "${metadata}"
}

collect_admin() {
  aeg_kind_collect_admin_snapshots "${ROOT_DIR}" "${OUTPUT_DIR}" "$1"
}

record_event() {
  local message="$1"
  printf '%s %s\n' "$(date --iso-8601=seconds)" "${message}" | tee -a "${OUTPUT_DIR}/events.log" >/dev/null
}

write_conclusion() {
  local scenario="$1"
  local status="$2"
  local summary="$3"
  shift 3

  mkdir -p "${OUTPUT_DIR}/conclusions"
  python3 - "${OUTPUT_DIR}/conclusions/${scenario}.json" "${scenario}" "${status}" "${summary}" "$@" <<'PY'
import json
import sys
from pathlib import Path

dst = Path(sys.argv[1])
payload = {
    "scenario": sys.argv[2],
    "status": sys.argv[3],
    "summary": sys.argv[4],
    "evidence": list(sys.argv[5:]),
}
dst.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

start_background_traffic() {
  aeg_kind_start_background_http_traffic \
    TRAFFIC_PID \
    "${HTTP_CLIENT}" \
    "${OUTPUT_DIR}" \
    "${GATEWAY_HOST_PORT}" \
    "${HTTP_HOST}" \
    "${TRAFFIC_BATCH_REQUESTS}" \
    "${TRAFFIC_BATCH_CONCURRENCY}"
}

stop_background_traffic() {
  aeg_kind_stop_background_pid "${TRAFFIC_PID}"
}

cleanup() {
  local exit_code="$?"
  stop_background_traffic
  exit "${exit_code}"
}

current_leader_holder() {
  kubectl --context kind-aether-gateway -n aether-gateway get lease aether-gateway-controlplane-leader \
    -o jsonpath='{.spec.holderIdentity}'
}

wait_for_new_leader() {
  local old_holder="$1"
  local new_holder
  for _ in $(seq 1 40); do
    new_holder="$(current_leader_holder)"
    if [[ -n "${new_holder}" && "${new_holder}" != "${old_holder}" ]]; then
      printf '%s\n' "${new_holder}"
      return 0
    fi
    sleep 1
  done
  return 1
}

assert_http_ready() {
  if ! curl -fsS -H "Host: ${HTTP_HOST}" "http://127.0.0.1:${GATEWAY_HOST_PORT}/" >/dev/null; then
    FAILURES=$((FAILURES + 1))
    record_event "http readiness probe failed"
    return 1
  fi
  return 0
}

select_drain_node() {
  local nodes_json
  if ! nodes_json="$(kubectl --context kind-aether-gateway get nodes -o json 2>/dev/null)"; then
    return 0
  fi
  jq -r '
    [
      .items[]
      | select(
          ((.metadata.labels // {}) | (has("node-role.kubernetes.io/control-plane") or has("node-role.kubernetes.io/master"))) | not
        )
      | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
      | select((.spec.unschedulable // false) | not)
      | .metadata.name
    ][0] // empty
  ' <<<"${nodes_json}"
}

schedulable_worker_node_count() {
  local nodes_json
  if ! nodes_json="$(kubectl --context kind-aether-gateway get nodes -o json 2>/dev/null)"; then
    printf '0\n'
    return
  fi
  jq -r '
    [
      .items[]
      | select(
          ((.metadata.labels // {}) | (has("node-role.kubernetes.io/control-plane") or has("node-role.kubernetes.io/master"))) | not
        )
      | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
      | select((.spec.unschedulable // false) | not)
    ]
    | length
  ' <<<"${nodes_json}"
}

schedulable_worker_nodes() {
  local nodes_json
  if ! nodes_json="$(kubectl --context kind-aether-gateway get nodes -o json 2>/dev/null)"; then
    return
  fi
  jq -r '
    .items[]
    | select(
        ((.metadata.labels // {}) | (has("node-role.kubernetes.io/control-plane") or has("node-role.kubernetes.io/master"))) | not
      )
    | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
    | select((.spec.unschedulable // false) | not)
    | .metadata.name
  ' <<<"${nodes_json}"
}

ensure_node_drain_cluster_shape() {
  if [[ "${SKIP_NODE_DRAIN}" == "true" ]]; then
    return
  fi

  local node worker_count
  worker_count="$(schedulable_worker_node_count)"
  if [[ "${worker_count}" -ge 2 ]]; then
    return
  fi

  if ! [[ "${NODE_DRAIN_KIND_WORKER_NODES}" =~ ^[0-9]+$ ]] || [[ "${NODE_DRAIN_KIND_WORKER_NODES}" -lt 2 ]]; then
    log "node drain requested but NODE_DRAIN_KIND_WORKER_NODES must be at least 2: ${NODE_DRAIN_KIND_WORKER_NODES}"
    exit 1
  fi

  log "node drain requested but only ${worker_count} non-control-plane Ready schedulable node(s) are available; recreating kind stack with ${NODE_DRAIN_KIND_WORKER_NODES} worker node(s)"
  (
    cd "${ROOT_DIR}"
    RECREATE_CLUSTER=true \
      KIND_WORKER_NODES="${NODE_DRAIN_KIND_WORKER_NODES}" \
      SKIP_BUILD=true \
      "${RUN_KIND_SCRIPT}"
  )

  worker_count="$(schedulable_worker_node_count)"
  if [[ "${worker_count}" -lt 2 ]]; then
    log "node drain requested but only ${worker_count} non-control-plane Ready schedulable node(s) are available after kind refresh"
    exit 1
  fi
  node="$(select_drain_node)"
  log "node drain worker candidate available: ${node}"
}

prepare_http_traffic_backend() {
  if [[ "${SKIP_NODE_DRAIN}" == "true" ]]; then
    return
  fi

  if ! [[ "${FAULT_HTTP_BACKEND_REPLICAS}" =~ ^[0-9]+$ ]] || [[ "${FAULT_HTTP_BACKEND_REPLICAS}" -lt 2 ]]; then
    log "FAULT_HTTP_BACKEND_REPLICAS must be at least 2 when node drain traffic SLO is enabled: ${FAULT_HTTP_BACKEND_REPLICAS}"
    exit 1
  fi

  record_event "preparing HTTP backend echo with ${FAULT_HTTP_BACKEND_REPLICAS} replicas for node drain traffic"
  kubectl --context kind-aether-gateway -n aether-gateway delete pod \
    -l nantian.dev/fault-traffic-guard=true \
    --ignore-not-found \
    --wait=true >/dev/null 2>&1 || true
  kubectl --context kind-aether-gateway -n aether-gateway patch deployment/echo \
    --type=json \
    -p='[{"op":"remove","path":"/spec/template/spec/topologySpreadConstraints"}]' >/dev/null 2>&1 || true
  kubectl --context kind-aether-gateway -n aether-gateway scale deployment/echo \
    --replicas="${FAULT_HTTP_BACKEND_REPLICAS}" >/dev/null
  kubectl --context kind-aether-gateway -n aether-gateway rollout status deployment/echo --timeout=180s >/dev/null
  ensure_http_backend_spread_guard
  wait_for_http_backend_spread
  assert_http_ready
}

http_backend_ready_nodes() {
  kubectl --context kind-aether-gateway -n aether-gateway get pods -l app=echo -o json | jq -r '
    [
      .items[]
      | select(.status.phase == "Running")
      | select(any(.status.containerStatuses[]?; .ready == true))
      | .spec.nodeName
    ]
    | unique
    | .[]
  '
}

http_backend_ready_node_count() {
  http_backend_ready_nodes | sed '/^$/d' | wc -l | tr -d ' '
}

first_worker_without_ready_http_backend() {
  local ready_nodes
  local worker
  ready_nodes="$(http_backend_ready_nodes || true)"
  while IFS= read -r worker; do
    [[ -z "${worker}" ]] && continue
    if ! grep -Fxq "${worker}" <<<"${ready_nodes}"; then
      printf '%s\n' "${worker}"
      return
    fi
  done < <(schedulable_worker_nodes)
}

fault_guard_name_for_node() {
  local node="$1"
  local suffix
  suffix="$(tr '[:upper:]' '[:lower:]' <<<"${node}" \
    | sed -E 's/[^a-z0-9-]+/-/g; s/^-+//; s/-+$//' \
    | cut -c1-40)"
  printf 'echo-fault-traffic-%s\n' "${suffix:-worker}"
}

ensure_http_backend_spread_guard() {
  local required_nodes=2
  local ready_nodes
  local missing_node
  local guard_name
  local image

  if [[ "${FAULT_HTTP_BACKEND_REPLICAS}" -lt 2 ]]; then
    required_nodes="${FAULT_HTTP_BACKEND_REPLICAS}"
  fi

  ready_nodes="$(http_backend_ready_node_count)"
  if [[ "${ready_nodes}" -ge "${required_nodes}" ]]; then
    return
  fi

  missing_node="$(first_worker_without_ready_http_backend)"
  if [[ -z "${missing_node}" ]]; then
    return
  fi

  guard_name="$(fault_guard_name_for_node "${missing_node}")"
  image="$(kubectl --context kind-aether-gateway -n aether-gateway get deployment/echo \
    -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)"
  image="${image:-m.daocloud.io/docker.io/hashicorp/http-echo:1.0.0}"

  record_event "creating HTTP backend fault guard ${guard_name} on ${missing_node}"
  kubectl --context kind-aether-gateway -n aether-gateway apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: ${guard_name}
  namespace: aether-gateway
  labels:
    app: echo
    nantian.dev/fault-traffic-guard: "true"
spec:
  restartPolicy: Always
  nodeName: ${missing_node}
  containers:
    - name: echo
      image: ${image}
      imagePullPolicy: IfNotPresent
      args:
        - "-listen=:8080"
        - "-text=aether-gateway-ok"
      ports:
        - containerPort: 8080
YAML
}

wait_for_http_backend_spread() {
  local required_nodes=2
  local ready_nodes

  if [[ "${FAULT_HTTP_BACKEND_REPLICAS}" -lt 2 ]]; then
    required_nodes="${FAULT_HTTP_BACKEND_REPLICAS}"
  fi

  for _ in $(seq 1 60); do
    ready_nodes="$(http_backend_ready_node_count)"
    if [[ "${ready_nodes}" -ge "${required_nodes}" ]]; then
      record_event "HTTP backend echo ready on ${ready_nodes} node(s)"
      return
    fi
    sleep 2
  done

  log "HTTP backend echo did not become ready on ${required_nodes} node(s)"
  exit 1
}

dataplane_desired_replicas() {
  local replicas
  replicas="$(kubectl --context kind-aether-gateway -n aether-gateway get deploy/aether-gateway-dataplane \
    -o jsonpath='{.spec.replicas}' 2>/dev/null || true)"
  printf '%s\n' "${replicas:-1}"
}

dataplane_ready_non_terminating_count() {
  local deleted_pod="$1"
  kubectl --context kind-aether-gateway -n aether-gateway get pods \
    -l app=aether-gateway-dataplane \
    -o json | jq -r --arg deleted_pod "${deleted_pod}" '
      [
        .items[]
        | select(.metadata.name != $deleted_pod)
        | select((.metadata.deletionTimestamp // "") == "")
        | select(.status.phase == "Running")
        | select(any(.status.containerStatuses[]?; .ready == true))
      ]
      | length
    '
}

wait_for_dataplane_replacement() {
  local deleted_pod="$1"
  local desired
  local ready

  desired="$(dataplane_desired_replicas)"
  if ! [[ "${desired}" =~ ^[0-9]+$ ]] || [[ "${desired}" -lt 1 ]]; then
    desired=1
  fi

  for _ in $(seq 1 90); do
    ready="$(dataplane_ready_non_terminating_count "${deleted_pod}")"
    if [[ "${ready}" -ge "${desired}" ]]; then
      return 0
    fi
    sleep 2
  done

  return 1
}

run_node_drain_scenario() {
  if [[ "${SKIP_NODE_DRAIN}" == "true" ]]; then
    write_conclusion \
      "node-drain" \
      "not-run" \
      "node drain skipped by SKIP_NODE_DRAIN=true" \
      "events.log"
    return
  fi

  local node
  node="$(select_drain_node)"
  if [[ -z "${node}" ]]; then
    write_conclusion \
      "node-drain" \
      "not-run" \
      "node drain not run because no non-control-plane Ready node was available" \
      "events.log"
    return
  fi

  record_event "draining node ${node}"
  if ! kubectl --context kind-aether-gateway drain "${node}" \
    --ignore-daemonsets \
    --delete-emptydir-data \
    --force \
    --timeout="${NODE_DRAIN_TIMEOUT}" >/dev/null; then
    FAILURES=$((FAILURES + 1))
    record_event "node drain failed for ${node}"
    kubectl --context kind-aether-gateway uncordon "${node}" >/dev/null || true
    write_conclusion \
      "node-drain" \
      "fail" \
      "node drain failed for ${node}" \
      "events.log" \
      "logs/dataplane.log"
    return
  fi

  local recovered=true
  local ready=true
  if ! kubectl --context kind-aether-gateway -n aether-gateway rollout status deploy/aether-gateway-dataplane --timeout=180s >/dev/null; then
    recovered=false
    FAILURES=$((FAILURES + 1))
    record_event "dataplane deployment did not recover after draining ${node}"
  fi
  if ! assert_http_ready; then
    ready=false
  fi
  kubectl --context kind-aether-gateway uncordon "${node}" >/dev/null || true

  if [[ "${recovered}" == "true" && "${ready}" == "true" ]]; then
    record_event "node drain workload recovered on ${node}"
    write_conclusion \
      "node-drain" \
      "pass" \
      "node drain completed for ${node}; dataplane deployment recovered while traffic continued" \
      "events.log" \
      "logs/dataplane.log" \
      "admin-after/"
  else
    write_conclusion \
      "node-drain" \
      "fail" \
      "node drain completed for ${node}, but dataplane recovery or HTTP readiness did not finish" \
      "events.log" \
      "logs/dataplane.log"
  fi
}

first_gateway_resource() {
  kubectl --context kind-aether-gateway get gateways.gateway.networking.k8s.io -A -o json | jq -r '
    .items[0]? | select(. != null) | [.metadata.namespace, .metadata.name] | @tsv
  '
}

first_httproute_resource() {
  kubectl --context kind-aether-gateway get httproutes.gateway.networking.k8s.io -A -o json | jq -r '
    .items[0]? | select(. != null) | [.metadata.namespace, .metadata.name] | @tsv
  '
}

patch_watch_churn_resource() {
  local resource="$1"
  local namespace="$2"
  local name="$3"
  local iteration="$4"
  local patch
  patch="$(jq -nc \
    --arg key "${APISERVER_WATCH_CHURN_ANNOTATION}" \
    --arg value "${RUN_ID}-${iteration}" \
    '{metadata:{annotations:{($key):$value}}}')"
  kubectl --context kind-aether-gateway -n "${namespace}" patch "${resource}" "${name}" --type merge -p "${patch}" >/dev/null
}

run_apiserver_watch_disruption_scenario() {
  if [[ "${SKIP_APISERVER_WATCH_DISRUPTION}" == "true" ]]; then
    write_conclusion \
      "apiserver-watch-disruption" \
      "not-run" \
      "apiserver watch churn skipped by SKIP_APISERVER_WATCH_DISRUPTION=true" \
      "events.log"
    return
  fi

  if ! [[ "${APISERVER_WATCH_CHURN_ITERATIONS}" =~ ^[0-9]+$ ]] || [[ "${APISERVER_WATCH_CHURN_ITERATIONS}" -lt 1 ]]; then
    FAILURES=$((FAILURES + 1))
    write_conclusion \
      "apiserver-watch-disruption" \
      "fail" \
      "apiserver watch churn iteration count must be a positive integer" \
      "events.log"
    return
  fi

  local gateway_ref route_ref
  gateway_ref="$(first_gateway_resource)"
  route_ref="$(first_httproute_resource)"
  if [[ -z "${gateway_ref}" && -z "${route_ref}" ]]; then
    write_conclusion \
      "apiserver-watch-disruption" \
      "not-run" \
      "apiserver watch churn not run because no Gateway or HTTPRoute resource was available" \
      "events.log"
    return
  fi

  local gateway_namespace="" gateway_name="" route_namespace="" route_name=""
  if [[ -n "${gateway_ref}" ]]; then
    read -r gateway_namespace gateway_name <<<"${gateway_ref}"
  fi
  if [[ -n "${route_ref}" ]]; then
    read -r route_namespace route_name <<<"${route_ref}"
  fi

  record_event "apiserver watch churn started"
  local iteration
  local patched=true
  local ready=true
  for iteration in $(seq 1 "${APISERVER_WATCH_CHURN_ITERATIONS}"); do
    if [[ -n "${gateway_name}" ]]; then
      if ! patch_watch_churn_resource "gateways.gateway.networking.k8s.io" "${gateway_namespace}" "${gateway_name}" "${iteration}"; then
        patched=false
        FAILURES=$((FAILURES + 1))
        record_event "Gateway watch churn patch failed on iteration ${iteration}"
      fi
    fi
    if [[ -n "${route_name}" ]]; then
      if ! patch_watch_churn_resource "httproutes.gateway.networking.k8s.io" "${route_namespace}" "${route_name}" "${iteration}"; then
        patched=false
        FAILURES=$((FAILURES + 1))
        record_event "HTTPRoute watch churn patch failed on iteration ${iteration}"
      fi
    fi
    if ! assert_http_ready; then
      ready=false
    fi
  done
  record_event "apiserver watch churn completed"
  if [[ "${patched}" == "true" && "${ready}" == "true" ]]; then
    write_conclusion \
      "apiserver-watch-disruption" \
      "pass" \
      "apiserver watch churn patched Gateway/HTTPRoute resources for ${APISERVER_WATCH_CHURN_ITERATIONS} iterations while traffic continued" \
      "events.log" \
      "logs/controlplane.log" \
      "admin-after/"
  else
    write_conclusion \
      "apiserver-watch-disruption" \
      "fail" \
      "apiserver watch churn did not complete cleanly; patching or HTTP readiness failed" \
      "events.log" \
      "logs/controlplane.log"
  fi
}

aggregate_traffic() {
  local input="${OUTPUT_DIR}/traffic/http-batches.jsonl"
  local output="${OUTPUT_DIR}/traffic/summary.json"
  mkdir -p "$(dirname "${output}")"
  python3 - "${input}" "${output}" <<'PY'
import json
import sys
from pathlib import Path

src = Path(sys.argv[1])
dst = Path(sys.argv[2])
items = [json.loads(line) for line in src.read_text(encoding="utf-8").splitlines() if line.strip()]
if not items:
    raise SystemExit("no traffic batches captured")

summary = {
    "batches": len(items),
    "completed": sum(item["completed"] for item in items),
    "successes": sum(item["successes"] for item in items),
    "mean_success_rate": sum(item["success_rate"] for item in items) / len(items),
    "max_p95_ms": max(item["latency_ms"]["p95"] for item in items),
    "max_p99_ms": max(item["latency_ms"]["p99"] for item in items),
    "max_p999_ms": max(item["latency_ms"].get("p999", item["latency_ms"]["p99"]) for item in items),
    "max_latency_ms": max(item["latency_ms"]["max"] for item in items),
}
dst.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

apply_traffic_slo_gate() {
  local summary="${OUTPUT_DIR}/traffic/summary.json"

  aeg_kind_write_traffic_slo_gate \
    "${summary}" \
    "${summary}" \
    "${MIN_SUCCESS_RATE}" \
    "${MAX_ERRORS}" \
    "${MAX_P99_MS}" \
    "${MAX_LATENCY_MS}" \
    "${SLO_GATE_RISK_ACCEPTED}"
}

aggregate_conclusions() {
  local input_dir="${OUTPUT_DIR}/conclusions"
  local output="${OUTPUT_DIR}/conclusions/summary.json"
  mkdir -p "$(dirname "${output}")"
  python3 - "${input_dir}" "${output}" "${REQUIRED_CONCLUSION_SCENARIOS}" <<'PY'
import json
import re
import sys
from collections import Counter
from pathlib import Path

src = Path(sys.argv[1])
dst = Path(sys.argv[2])
required = [item for item in re.split(r"[\s,]+", sys.argv[3].strip()) if item]

scenarios = {}
if src.exists():
    for path in sorted(src.glob("*.json")):
        if path.name == "summary.json":
            continue
        payload = json.loads(path.read_text(encoding="utf-8"))
        scenario = str(payload.get("scenario", path.stem)).strip()
        if not scenario:
            raise SystemExit(f"missing scenario in {path}")
        status = str(payload.get("status", "")).strip()
        if status not in {"pass", "risk-accepted", "fail", "not-run"}:
            raise SystemExit(f"invalid status for {scenario}: {status}")
        summary = str(payload.get("summary", "")).strip()
        if not summary:
            raise SystemExit(f"missing summary for {scenario}")
        scenarios[scenario] = {
            "status": status,
            "summary": summary,
            "evidence": list(payload.get("evidence", [])),
        }

missing = [scenario for scenario in required if scenario not in scenarios]
status_counts = Counter(item["status"] for item in scenarios.values())
if any(item["status"] == "fail" for item in scenarios.values()):
    release_gate_status = "fail"
elif missing or any(item["status"] == "not-run" for item in scenarios.values()):
    release_gate_status = "incomplete"
elif any(item["status"] == "risk-accepted" for item in scenarios.values()):
    release_gate_status = "risk-accepted"
else:
    release_gate_status = "pass"

summary = {
    "release_gate_status": release_gate_status,
    "required_scenarios": required,
    "observed_scenarios": sorted(scenarios),
    "missing_required_scenarios": missing,
    "status_counts": dict(sorted(status_counts.items())),
    "scenarios": {name: scenarios[name] for name in sorted(scenarios)},
}

dst.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

write_summary() {
  local git_commit
  git_commit="$(git -C "${ROOT_DIR}" rev-parse --short HEAD)"
  python3 - "${OUTPUT_DIR}" "${RUN_ID}" "${git_commit}" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
run_id = sys.argv[2]
git_commit = sys.argv[3]


def load_text(path):
    return path.read_text(encoding="utf-8") if path.exists() else ""


def load_json(path, default):
    if path.exists():
        return json.loads(path.read_text(encoding="utf-8"))
    return default


def escape_cell(value):
    return str(value).replace("|", "\\|").replace("\n", " ")


def fmt_number(value):
    if value is None:
        return "n/a"
    numeric = float(value)
    if numeric.is_integer():
        return str(int(numeric))
    return f"{numeric:.3f}".rstrip("0").rstrip(".")


def fmt_percent(value):
    if value is None:
        return "n/a"
    return f"{fmt_number(float(value) * 100)}%"


def fmt_ms(value):
    return f"{fmt_number(value)}ms" if value is not None else "n/a"


traffic = load_json(root / "traffic" / "summary.json", {})
gate = traffic.get("slo_gate", {})
observed = gate.get("observed", {})
thresholds = gate.get("thresholds", {})
conclusions = load_json(root / "conclusions" / "summary.json", {"scenarios": {}})
events = load_text(root / "events.log").strip()

lines = [
    "# Kind Fault Injection",
    "",
    f"- Run ID: `{run_id}`",
    f"- Git commit: `{git_commit}`",
    f"- release gate status: `{conclusions.get('release_gate_status', 'incomplete')}`",
    f"- traffic SLO gate: `{gate.get('status', 'unknown')}`",
    "",
    "## Faults",
    "",
    "```text",
    events,
    "```",
    "",
    "## Continuous Traffic Summary",
    "",
    "- SLO: "
    f"success rate `{fmt_percent(observed.get('success_rate'))}` >= `{fmt_percent(thresholds.get('min_success_rate'))}`; "
    f"errors `{fmt_number(observed.get('errors'))}` <= `{fmt_number(thresholds.get('max_errors'))}`; "
    f"p99 `{fmt_ms(observed.get('p99_ms'))}` <= `{fmt_ms(thresholds.get('max_p99_ms'))}`; "
    f"max latency `{fmt_ms(observed.get('max_latency_ms'))}` <= `{fmt_ms(thresholds.get('max_latency_ms'))}`",
    "",
    "```json",
    json.dumps(traffic, indent=2, sort_keys=True),
    "```",
    "",
    "## Release Gate Conclusions",
    "",
    "| Scenario | Status | Conclusion |",
    "| --- | --- | --- |",
]

scenarios = conclusions.get("scenarios", {})
if scenarios:
    for name in sorted(scenarios):
        item = scenarios[name]
        lines.append(
            f"| {escape_cell(name)} | {escape_cell(item.get('status', ''))} | "
            f"{escape_cell(item.get('summary', ''))} |"
        )
else:
    lines.append("| n/a | incomplete | no conclusion artifacts captured |")

missing = conclusions.get("missing_required_scenarios", [])
if missing:
    lines.extend([
        "",
        "Missing required scenarios:",
        "",
        *[f"- `{item}`" for item in missing],
    ])

lines.extend([
    "",
    "```json",
    json.dumps(conclusions, indent=2, sort_keys=True),
    "```",
    "",
    "## Scoped Conclusion",
    "",
    "- This run keeps continuous HTTP traffic active while forcing selected controlplane or dataplane disruption scenarios.",
    "- Each release-gate scenario must have an explicit conclusion artifact under `conclusions/`; missing required scenarios keep the release-gate status `incomplete`.",
    "- Backend timeout/failover faults remain covered by `tests/e2e/validate-upstream-behavior.sh`, which should be paired with this run when assembling a full A4 bundle.",
])

(root / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
}

summarize_evidence() {
  aggregate_traffic
  apply_traffic_slo_gate
  aggregate_conclusions
  write_summary
  assert_traffic_slo_gate
}

assert_release_gate_conclusions() {
  local status
  status="$(python3 - "${OUTPUT_DIR}/conclusions/summary.json" <<'PY'
import json
import sys
from pathlib import Path

print(json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))["release_gate_status"])
PY
)"
  if [[ "${REQUIRE_RELEASE_GATE_CONCLUSIONS}" == "true" && "${status}" != "pass" && "${status}" != "risk-accepted" ]]; then
    log "release gate conclusions are ${status}"
    exit 1
  fi
}

assert_traffic_slo_gate() {
  local status
  status="$(aeg_kind_slo_status "${OUTPUT_DIR}/traffic/summary.json")"

  if [[ "${status}" == "fail" ]]; then
    log "traffic SLO gate failed"
    exit 1
  fi
  if [[ "${status}" == "risk-accepted" ]]; then
    log "traffic SLO gate is risk-accepted"
  fi
}

main() {
  trap cleanup EXIT

  require_command git
  require_command python3

  if [[ "${SUMMARY_ONLY}" == "true" ]]; then
    write_metadata
    summarize_evidence
    log "fault evidence summary written to ${OUTPUT_DIR}"
    return
  fi

  require_command curl
  require_command jq
  require_command kubectl

  ensure_node_drain_cluster_shape
  ensure_stack
  ensure_node_drain_cluster_shape
  write_metadata
  prepare_http_traffic_backend
  collect_admin admin-before
  mkdir -p "${OUTPUT_DIR}/logs" "${OUTPUT_DIR}/traffic"

  record_event "starting background http traffic"
  start_background_traffic
  sleep 5

  old_holder="$(current_leader_holder)"
  leader_pod="${old_holder%%_*}"
  record_event "deleting controlplane leader pod ${leader_pod}"
  kubectl --context kind-aether-gateway -n aether-gateway delete pod "${leader_pod}" --wait=false >/dev/null
  new_holder="$(wait_for_new_leader "${old_holder}")" || {
    FAILURES=$((FAILURES + 1))
    record_event "leader did not switch within timeout"
    new_holder=""
  }
  [[ -n "${new_holder}" ]] && record_event "new controlplane leader ${new_holder}"
  if [[ -n "${new_holder}" ]]; then
    write_conclusion \
      "controlplane-leader-switch" \
      "pass" \
      "controlplane leader moved from ${leader_pod} to ${new_holder} while traffic continued" \
      "events.log" \
      "admin-before/" \
      "admin-after/"
  else
    write_conclusion \
      "controlplane-leader-switch" \
      "fail" \
      "controlplane leader did not switch within timeout" \
      "events.log" \
      "logs/controlplane.log"
  fi
  kubectl --context kind-aether-gateway -n aether-gateway rollout status deploy/aether-gateway-controlplane --timeout=180s >/dev/null
  assert_http_ready || true

  dataplane_pod="$(kubectl --context kind-aether-gateway -n aether-gateway get pods -l app=aether-gateway-dataplane -o jsonpath='{.items[0].metadata.name}')"
  record_event "deleting dataplane pod ${dataplane_pod}"
  kubectl --context kind-aether-gateway -n aether-gateway delete pod "${dataplane_pod}" --wait=false >/dev/null
  dataplane_recovered=true
  if ! kubectl --context kind-aether-gateway -n aether-gateway rollout status deploy/aether-gateway-dataplane --timeout=180s >/dev/null; then
    dataplane_recovered=false
    FAILURES=$((FAILURES + 1))
    record_event "dataplane rollout did not report recovered after deleting ${dataplane_pod}"
  fi
  if ! wait_for_dataplane_replacement "${dataplane_pod}"; then
    dataplane_recovered=false
    FAILURES=$((FAILURES + 1))
    record_event "dataplane replacement did not become ready after deleting ${dataplane_pod}"
  fi
  if [[ "${dataplane_recovered}" == "true" ]]; then
    record_event "dataplane deployment recovered"
    write_conclusion \
      "dataplane-pod-restart" \
      "pass" \
      "dataplane deployment recovered after deleting ${dataplane_pod} while traffic continued" \
      "events.log" \
      "logs/dataplane.log" \
      "admin-after/"
  else
    write_conclusion \
      "dataplane-pod-restart" \
      "fail" \
      "dataplane replacement did not recover cleanly after deleting ${dataplane_pod}" \
      "events.log" \
      "logs/dataplane.log"
  fi
  assert_http_ready || true

  run_node_drain_scenario
  assert_http_ready || true

  run_apiserver_watch_disruption_scenario
  assert_http_ready || true

  sleep 5
  stop_background_traffic
  TRAFFIC_PID=""

  collect_admin admin-after
  kubectl --context kind-aether-gateway -n aether-gateway logs deploy/aether-gateway-controlplane --tail=200 \
    >"${OUTPUT_DIR}/logs/controlplane.log"
  kubectl --context kind-aether-gateway -n aether-gateway logs deploy/aether-gateway-dataplane --tail=200 \
    >"${OUTPUT_DIR}/logs/dataplane.log"
  summarize_evidence
  assert_release_gate_conclusions

  if [[ "${FAILURES}" -ne 0 ]]; then
    log "completed with ${FAILURES} failures"
    exit 1
  fi
  log "fault evidence written to ${OUTPUT_DIR}"
}

main "$@"
