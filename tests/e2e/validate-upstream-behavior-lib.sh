log() {
  printf '[upstream-behavior] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "${name}" >/dev/null 2>&1; then
    log "missing required command: ${name}"
    exit 1
  fi
}

k() {
  kubectl --context "${KUBE_CONTEXT}" "$@"
}

kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

ensure_kind_cluster() {
  if kind_cluster_exists; then
    return
  fi
  if [[ "${ENSURE_KIND}" != "true" ]]; then
    log "kind cluster ${CLUSTER_NAME} does not exist; run ./tests/e2e/run-kind.sh first or rerun with ENSURE_KIND=true"
    exit 1
  fi

  log "bootstrapping kind cluster via tests/e2e/run-kind.sh"
  (
    cd "${ROOT_DIR}"
    SKIP_BUILD="${SKIP_BUILD:-true}" SKIP_SMOKE=true ./tests/e2e/run-kind.sh
  )
}

ensure_local_registry() {
  if ! docker inspect "${LOCAL_REGISTRY_NAME}" >/dev/null 2>&1; then
    log "local registry ${LOCAL_REGISTRY_NAME} is not running; run ./tests/e2e/run-kind.sh first"
    exit 1
  fi
}

sync_image_to_local_registry() {
  local source_image="$1"
  local target_image="$2"
  local push_image="${target_image}"

  if [[ "${target_image}" == "${LOCAL_REGISTRY_HOST}/"* ]]; then
    push_image="${LOCAL_REGISTRY_PUSH_HOST}/${target_image#${LOCAL_REGISTRY_HOST}/}"
  fi

  log "syncing ${source_image} -> ${push_image}"
  if ! docker image inspect "${source_image}" >/dev/null 2>&1; then
    docker pull "${source_image}" >/dev/null
  fi
  docker tag "${source_image}" "${push_image}"
  docker push "${push_image}" >/dev/null
}

preload_kind_images() {
  local node

  log "preloading validation image into kind nodes via crictl"
  for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    docker exec "${node}" crictl pull "${PYTHON_IMAGE}" >/dev/null
  done
}

sync_test_images() {
  ensure_local_registry
  sync_image_to_local_registry "${PYTHON_SOURCE_IMAGE}" "${PYTHON_IMAGE}"
  preload_kind_images
}

cleanup_namespace() {
  if ! k get namespace "${TEST_NAMESPACE}" >/dev/null 2>&1; then
    return
  fi

  log "cleaning namespace ${TEST_NAMESPACE}"
  k delete namespace "${TEST_NAMESPACE}" --wait=false >/dev/null 2>&1 || true
  if ! timeout 120 bash -c \
    "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${TEST_NAMESPACE}' >/dev/null 2>&1; do sleep 2; done"
  then
    log "forcing cleanup for namespace ${TEST_NAMESPACE}"
    k -n "${TEST_NAMESPACE}" delete pod --all --force --grace-period=0 >/dev/null 2>&1 || true
    k get namespace "${TEST_NAMESPACE}" -o json \
      | jq '{apiVersion, kind, metadata: {name: .metadata.name}, spec: {finalizers: []}}' \
      | kubectl --context "${KUBE_CONTEXT}" replace --raw "/api/v1/namespaces/${TEST_NAMESPACE}/finalize" -f - >/dev/null 2>&1 || true

    if ! timeout 30 bash -c \
      "until ! kubectl --context '${KUBE_CONTEXT}' get namespace '${TEST_NAMESPACE}' >/dev/null 2>&1; do sleep 2; done"
    then
      log "namespace ${TEST_NAMESPACE} is still terminating after force cleanup"
      exit 1
    fi
  fi
}

port_listening() {
  local port="$1"

  ss -H -ltn "( sport = :${port} )" 2>/dev/null | grep -q .
}

pick_admin_forward_port() {
  local candidate="${ADMIN_FORWARD_PORT}"

  while port_listening "${candidate}"; do
    candidate=$((candidate + 1))
  done

  ADMIN_FORWARD_PORT="${candidate}"
}

wait_for_dataplane_ready_pods() {
  local expected="$1"

  for _ in $(seq 1 60); do
    if [[ "$(
      k -n "${DATAPLANE_NAMESPACE}" get pod -l app=nantian-dataplane -o json \
        | jq '[.items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | length'
    )" -eq "${expected}" ]]; then
      return
    fi
    sleep 2
  done

  log "dataplane ready pod count did not converge to ${expected}"
  k -n "${DATAPLANE_NAMESPACE}" get pod -l app=nantian-dataplane -o wide >&2 || true
  exit 1
}

ensure_single_dataplane_replica() {
  ORIGINAL_DATAPLANE_REPLICAS="$(
    k -n "${DATAPLANE_NAMESPACE}" get deployment "${DATAPLANE_DEPLOYMENT}" -o jsonpath='{.spec.replicas}'
  )"

  if [[ -z "${ORIGINAL_DATAPLANE_REPLICAS}" ]]; then
    log "failed to determine current dataplane replica count"
    exit 1
  fi

  if [[ "${ORIGINAL_DATAPLANE_REPLICAS}" == "1" ]]; then
    wait_for_dataplane_ready_pods 1
    return
  fi

  log "scaling ${DATAPLANE_NAMESPACE}/${DATAPLANE_DEPLOYMENT} to 1 replica for deterministic upstream validation"
  k -n "${DATAPLANE_NAMESPACE}" scale deployment "${DATAPLANE_DEPLOYMENT}" --replicas=1 >/dev/null
  k -n "${DATAPLANE_NAMESPACE}" rollout status deployment/"${DATAPLANE_DEPLOYMENT}" --timeout=180s >/dev/null
  wait_for_dataplane_ready_pods 1
}

restore_dataplane_replicas() {
  if [[ -z "${ORIGINAL_DATAPLANE_REPLICAS}" || "${ORIGINAL_DATAPLANE_REPLICAS}" == "1" ]]; then
    return
  fi

  log "restoring ${DATAPLANE_NAMESPACE}/${DATAPLANE_DEPLOYMENT} to ${ORIGINAL_DATAPLANE_REPLICAS} replicas"
  k -n "${DATAPLANE_NAMESPACE}" scale deployment "${DATAPLANE_DEPLOYMENT}" --replicas="${ORIGINAL_DATAPLANE_REPLICAS}" >/dev/null || true
  k -n "${DATAPLANE_NAMESPACE}" rollout status deployment/"${DATAPLANE_DEPLOYMENT}" --timeout=180s >/dev/null || true
}

start_admin_port_forward() {
  pick_admin_forward_port
  PORT_FORWARD_LOG="${TMP_DIR}/port-forward.log"
  k -n "${DATAPLANE_NAMESPACE}" port-forward service/nantian-dataplane-admin "${ADMIN_FORWARD_PORT}:19080" \
    >"${PORT_FORWARD_LOG}" 2>&1 &
  PORT_FORWARD_PID="$!"

  for _ in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/livez" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  log "timed out waiting for dataplane admin port-forward"
  cat "${PORT_FORWARD_LOG}" >&2 || true
  exit 1
}

stop_admin_port_forward() {
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    kill "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  fi
}

summary_json() {
  curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/summary"
}

traffic_json() {
  curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/v1/traffic"
}

metrics_text() {
  curl -fsS "http://127.0.0.1:${ADMIN_FORWARD_PORT}/metrics"
}

dump_debug_state() {
  set +e
  printf '\n[upstream-behavior] debug: gateway\n' >&2
  k -n "${TEST_NAMESPACE}" get gateway upstream-edge -o yaml >&2
  printf '\n[upstream-behavior] debug: routes\n' >&2
  k -n "${TEST_NAMESPACE}" get httproute -o yaml >&2
  printf '\n[upstream-behavior] debug: services\n' >&2
  k -n "${TEST_NAMESPACE}" get service -o wide >&2
  printf '\n[upstream-behavior] debug: deployments\n' >&2
  k -n "${TEST_NAMESPACE}" get deploy -o wide >&2
  printf '\n[upstream-behavior] debug: endpointslices\n' >&2
  k -n "${TEST_NAMESPACE}" get endpointslices -o wide >&2
  if [[ -n "${PORT_FORWARD_PID}" ]]; then
    printf '\n[upstream-behavior] debug: dataplane summary\n' >&2
    summary_json | jq '.' >&2
    printf '\n[upstream-behavior] debug: dataplane traffic\n' >&2
    traffic_json | jq '.' >&2
  fi
  if [[ -f "${PORT_FORWARD_LOG}" ]]; then
    printf '\n[upstream-behavior] debug: port-forward log\n' >&2
    cat "${PORT_FORWARD_LOG}" >&2
  fi
  set -e
}

cleanup() {
  local exit_code="$?"

  if [[ "${SUCCESS}" != "true" ]]; then
    dump_debug_state
  fi
  stop_admin_port_forward
  restore_dataplane_replicas

  if [[ "${KEEP_RESOURCES}" != "true" ]]; then
    cleanup_namespace
  else
    log "keeping namespace ${TEST_NAMESPACE}"
  fi

  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi

  exit "${exit_code}"
}

apply_resources() {
  render_resources
  k apply -f "${TMP_DIR}/resources.yaml" >/dev/null
}

wait_for_deployment() {
  local name="$1"
  k -n "${TEST_NAMESPACE}" rollout status deployment/"${name}" --timeout=180s >/dev/null
}

wait_for_gateway_ready() {
  for _ in $(seq 1 60); do
    if k -n "${TEST_NAMESPACE}" get gateway upstream-edge -o json \
      | jq -e '
          ( [.status.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0 )
          and
          ( [.status.conditions[]? | select(.type=="Programmed" and .status=="True")] | length > 0 )
        ' >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "gateway ${TEST_NAMESPACE}/upstream-edge did not become ready"
  k -n "${TEST_NAMESPACE}" get gateway upstream-edge -o yaml >&2
  exit 1
}

wait_for_route_acceptance() {
  local name="$1"

  for _ in $(seq 1 60); do
    if k -n "${TEST_NAMESPACE}" get httproute "${name}" -o json \
      | jq -e '[.status.parents[]?.conditions[]? | select(.type=="Accepted" and .status=="True")] | length > 0' \
      >/dev/null 2>&1
    then
      return
    fi
    sleep 2
  done

  log "route ${TEST_NAMESPACE}/${name} did not become accepted"
  k -n "${TEST_NAMESPACE}" get httproute "${name}" -o yaml >&2
  exit 1
}

wait_for_service_endpoints() {
  local service="$1"
  local minimum="${2:-1}"

  for _ in $(seq 1 60); do
    if [[ "$(
      k -n "${TEST_NAMESPACE}" get endpointslice \
        -l "kubernetes.io/service-name=${service}" -o json 2>/dev/null \
        | jq '[.items[].endpoints[]? | select(.conditions.ready != false)] | length'
    )" -ge "${minimum}" ]]; then
      return
    fi
    sleep 2
  done

  log "service ${TEST_NAMESPACE}/${service} did not expose ${minimum} ready endpoints"
  k -n "${TEST_NAMESPACE}" get endpointslice -l "kubernetes.io/service-name=${service}" -o yaml >&2
  exit 1
}

request_body() {
  local host="$1"
  local path="$2"

  curl -fsS \
    --http1.1 \
    --noproxy '*' \
    --resolve "${host}:${GATEWAY_HOST_PORT}:127.0.0.1" \
    -H 'Connection: close' \
    "http://${host}:${GATEWAY_HOST_PORT}${path}"
}

request_headers_and_body() {
  local host="$1"
  local path="$2"
  local headers_file="$3"
  local body_file="$4"

  curl -fsS \
    --http1.1 \
    --noproxy '*' \
    --resolve "${host}:${GATEWAY_HOST_PORT}:127.0.0.1" \
    -H 'Connection: close' \
    -D "${headers_file}" \
    -o "${body_file}" \
    "http://${host}:${GATEWAY_HOST_PORT}${path}" >/dev/null
}

wait_for_expected_body() {
  local host="$1"
  local path="$2"
  local expected="$3"
  local body=""

  for _ in $(seq 1 45); do
    body="$(request_body "${host}" "${path}" || true)"
    if [[ "${body}" == "${expected}" ]]; then
      return 0
    fi
    sleep 2
  done

  log "did not receive expected response body for ${host}${path}"
  printf 'expected=%s last_body=%s\n' "${expected}" "${body}" >&2
  exit 1
}

wait_for_any_expected_body() {
  local host="$1"
  local path="$2"
  shift 2
  local body=""
  local expected

  for _ in $(seq 1 45); do
    body="$(request_body "${host}" "${path}" || true)"
    for expected in "$@"; do
      if [[ "${body}" == "${expected}" ]]; then
        return 0
      fi
    done
    sleep 2
  done

  log "did not receive any expected response body for ${host}${path}"
  printf 'expected=%s last_body=%s\n' "$*" "${body}" >&2
  exit 1
}

header_value() {
  local file="$1"
  local name="$2"

  awk -F': *' -v key="$(printf '%s' "${name}" | tr '[:upper:]' '[:lower:]')" '
    tolower($1) == key { gsub(/\r$/, "", $2); print $2; exit }
  ' "${file}"
}

wait_for_admin_route_edges() {
  local route_name="$1"
  local minimum="${2:-1}"
  local route_node="route:HTTPRoute:${TEST_NAMESPACE}/${route_name}"

  for _ in $(seq 1 60); do
    if jq -e --arg route "${route_node}" --argjson minimum "${minimum}" '
      [.edges[] | select(.source == $route)] | map(.events) | add >= $minimum
    ' <<<"$(traffic_json)" >/dev/null 2>&1
    then
      return
    fi
    sleep 1
  done

  log "traffic view did not report route edges for ${route_name}"
  traffic_json | jq '.' >&2
  exit 1
}

weighted_counts() {
  local host="$1"
  local path="$2"
  local requests="$3"
  local a_count=0
  local b_count=0
  local body

  for _ in $(seq 1 "${requests}"); do
    body="$(request_body "${host}" "${path}")"
    case "${body}" in
      weighted-a|recover-a)
        a_count=$((a_count + 1))
        ;;
      weighted-b|recover-b)
        b_count=$((b_count + 1))
        ;;
      *)
        log "unexpected weighted response body: ${body}"
        exit 1
        ;;
    esac
  done

  printf '%s %s\n' "${a_count}" "${b_count}"
}

validate_keepalive_reuse() {
  local first_headers="${TMP_DIR}/pool-first.headers"
  local first_body="${TMP_DIR}/pool-first.body"
  local second_headers="${TMP_DIR}/pool-second.headers"
  local second_body="${TMP_DIR}/pool-second.body"
  local first_conn
  local second_conn
  local first_req
  local second_req

  wait_for_expected_body "${POOL_HOST}" "/pool" "pool-backend"

  curl -fsS \
    --http1.1 \
    --noproxy '*' \
    --resolve "${POOL_HOST}:${GATEWAY_HOST_PORT}:127.0.0.1" \
    -D "${first_headers}" \
    -o "${first_body}" \
    "http://${POOL_HOST}:${GATEWAY_HOST_PORT}/pool" \
    --next \
    --http1.1 \
    --noproxy '*' \
    --resolve "${POOL_HOST}:${GATEWAY_HOST_PORT}:127.0.0.1" \
    -D "${second_headers}" \
    -o "${second_body}" \
    "http://${POOL_HOST}:${GATEWAY_HOST_PORT}/pool" >/dev/null

  [[ "$(cat "${first_body}")" == "pool-backend" ]] || {
    log "unexpected first keepalive body"
    cat "${first_body}" >&2
    exit 1
  }
  [[ "$(cat "${second_body}")" == "pool-backend" ]] || {
    log "unexpected second keepalive body"
    cat "${second_body}" >&2
    exit 1
  }

  first_conn="$(header_value "${first_headers}" "X-Backend-Connection-Id")"
  second_conn="$(header_value "${second_headers}" "X-Backend-Connection-Id")"
  first_req="$(header_value "${first_headers}" "X-Backend-Connection-Request")"
  second_req="$(header_value "${second_headers}" "X-Backend-Connection-Request")"

  if [[ -z "${first_conn}" || -z "${second_conn}" ]]; then
    log "keepalive headers were not present"
    cat "${first_headers}" >&2
    cat "${second_headers}" >&2
    exit 1
  fi
  if [[ "${first_conn}" != "${second_conn}" ]]; then
    log "expected upstream keepalive reuse within one persistent downstream connection"
    printf 'first_conn=%s second_conn=%s\n' "${first_conn}" "${second_conn}" >&2
    exit 1
  fi
  if ! [[ "${first_req}" =~ ^[0-9]+$ && "${second_req}" =~ ^[0-9]+$ ]]; then
    log "expected numeric backend request counters on keepalive response headers"
    printf 'first_req=%s second_req=%s\n' "${first_req}" "${second_req}" >&2
    exit 1
  fi
  if (( second_req != first_req + 1 )); then
    log "expected the second keepalive request to increment the backend request counter by one"
    printf 'first_req=%s second_req=%s\n' "${first_req}" "${second_req}" >&2
    exit 1
  fi

  log "upstream keepalive reuse verified on a persistent downstream connection via backend connection id ${first_conn}"
}

validate_retry_failover() {
  wait_for_expected_body "${RETRY_HOST}" "/retry" "retry-healthy"
  wait_for_admin_route_edges retry-route 1
  log "503 retry failover returned healthy backend"
}

validate_timeout_failover() {
  local body
  local elapsed

  wait_for_expected_body "${TIMEOUT_HOST}" "/timeout" "fast-backend"

  elapsed="$(
    curl -fsS \
      --http1.1 \
      --noproxy '*' \
      --resolve "${TIMEOUT_HOST}:${GATEWAY_HOST_PORT}:127.0.0.1" \
      -H 'Connection: close' \
      -o "${TMP_DIR}/timeout.body" \
      -w '%{time_total}' \
      "http://${TIMEOUT_HOST}:${GATEWAY_HOST_PORT}/timeout"
  )"
  body="$(cat "${TMP_DIR}/timeout.body")"
  if [[ "${body}" != "fast-backend" ]]; then
    log "expected timeout route to fail over to fast backend"
    printf 'body=%s\n' "${body}" >&2
    exit 1
  fi
  awk -v value="${elapsed}" 'BEGIN { exit !(value < 1.2) }' || {
    log "timeout failover took longer than expected"
    printf 'elapsed=%s\n' "${elapsed}" >&2
    exit 1
  }

  wait_for_admin_route_edges timeout-route 1
  log "timeout failover returned fast backend in ${elapsed}s"
}

validate_weighted_distribution() {
  local counts
  local a_count
  local b_count

  wait_for_any_expected_body "${WEIGHT_HOST}" "/weight" "weighted-a" "weighted-b"
  counts="$(weighted_counts "${WEIGHT_HOST}" "/weight" 40)"
  a_count="${counts%% *}"
  b_count="${counts##* }"

  if [[ "${a_count}" -ne 10 || "${b_count}" -ne 30 ]]; then
    log "unexpected 1:3 weighted distribution"
    printf 'weighted-a=%s weighted-b=%s\n' "${a_count}" "${b_count}" >&2
    exit 1
  fi

  wait_for_admin_route_edges weighted-route 40
  log "weighted distribution matched 1:3 ratio (a=${a_count}, b=${b_count})"
}

validate_weight_convergence() {
  local start_ts
  local elapsed
  local counts
  local a_count
  local b_count

  start_ts="$(date +%s)"
  k -n "${TEST_NAMESPACE}" patch httproute weighted-route --type merge -p '{
    "spec": {
      "rules": [{
        "matches": [{"path": {"type": "PathPrefix", "value": "/weight"}}],
        "backendRefs": [
          {"name": "weighted-a", "port": 8080, "weight": 3},
          {"name": "weighted-b", "port": 8080, "weight": 1}
        ]
      }]
    }
  }' >/dev/null

  for _ in $(seq 1 30); do
    counts="$(weighted_counts "${WEIGHT_HOST}" "/weight" 8)"
    a_count="${counts%% *}"
    b_count="${counts##* }"
    if [[ "${a_count}" -eq 6 && "${b_count}" -eq 2 ]]; then
      elapsed="$(( $(date +%s) - start_ts ))"
      log "weight change converged to 3:1 in ${elapsed}s"
      return
    fi
    sleep 1
  done

  log "weight change did not converge to 3:1 in time"
  printf 'last_counts=%s\n' "${counts}" >&2
  exit 1
}

validate_backend_recovery() {
  local initial
  local after_drain
  local after_recover

  wait_for_any_expected_body "${RECOVER_HOST}" "/recover" "recover-a" "recover-b"
  initial="$(weighted_counts "${RECOVER_HOST}" "/recover" 4)"
  if [[ "${initial%% *}" -eq 0 || "${initial##* }" -eq 0 ]]; then
    log "expected both recover backends to receive traffic before drain"
    printf 'initial=%s\n' "${initial}" >&2
    exit 1
  fi

  k -n "${TEST_NAMESPACE}" scale deployment recover-a --replicas=0 >/dev/null
  k -n "${TEST_NAMESPACE}" rollout status deployment/recover-a --timeout=180s >/dev/null || true
  for _ in $(seq 1 60); do
    ready_endpoints="$(
      k -n "${TEST_NAMESPACE}" get endpointslice \
        -l 'kubernetes.io/service-name=recover-a' -o json 2>/dev/null \
        | jq '[.items[].endpoints[]? | select(.conditions.ready != false)] | length'
    )"
    if [[ "${ready_endpoints}" -eq 0 ]]; then
      break
    fi
    sleep 2
  done

  for _ in $(seq 1 30); do
    after_drain="$(weighted_counts "${RECOVER_HOST}" "/recover" 4)"
    if [[ "${after_drain%% *}" -eq 0 && "${after_drain##* }" -eq 4 ]]; then
      break
    fi
    sleep 1
  done
  if [[ "${after_drain%% *}" -ne 0 || "${after_drain##* }" -ne 4 ]]; then
    log "expected recover-a to be drained from rotation after convergence"
    printf 'after_drain=%s\n' "${after_drain}" >&2
    exit 1
  fi

  k -n "${TEST_NAMESPACE}" scale deployment recover-a --replicas=1 >/dev/null
  wait_for_deployment recover-a
  wait_for_service_endpoints recover-a 1

  for _ in $(seq 1 20); do
    after_recover="$(weighted_counts "${RECOVER_HOST}" "/recover" 8)"
    if [[ "${after_recover%% *}" -gt 0 && "${after_recover##* }" -gt 0 ]]; then
      log "backend recovery returned drained backend to rotation (${after_recover})"
      return
    fi
    sleep 1
  done

  log "recover-a did not re-enter rotation"
  exit 1
}

print_derived_metrics() {
  local before_summary="$1"
  local after_summary="$2"
  local before_traffic="$3"
  local after_traffic="$4"
  local total_events
  local retried_events
  local retry_attempts
  local retried_success_events
  local total_latency
  local max_latency
  local retry_rate
  local average_latency
  local failover_success_rate
  local weighted_edge_a
  local weighted_edge_b
  local imbalance
  local weighted_edge_a_before
  local weighted_edge_a_after
  local weighted_edge_b_before
  local weighted_edge_b_after

  total_events="$(
    jq -n \
      --argjson before "$(jq '.total_events' <<<"${before_traffic}")" \
      --argjson after "$(jq '.total_events' <<<"${after_traffic}")" \
      '$after - $before'
  )"
  retried_events="$(
    jq -n \
      --argjson before "$(jq '.trafficTotalRetriedEvents' <<<"${before_summary}")" \
      --argjson after "$(jq '.trafficTotalRetriedEvents' <<<"${after_summary}")" \
      '$after - $before'
  )"
  retry_attempts="$(
    jq -n \
      --argjson before "$(jq '.trafficTotalRetryAttempts' <<<"${before_summary}")" \
      --argjson after "$(jq '.trafficTotalRetryAttempts' <<<"${after_summary}")" \
      '$after - $before'
  )"
  retried_success_events="$(
    jq -n \
      --argjson before "$(jq '.trafficRetriedSuccessEvents' <<<"${before_summary}")" \
      --argjson after "$(jq '.trafficRetriedSuccessEvents' <<<"${after_summary}")" \
      '$after - $before'
  )"
  total_latency="$(
    jq -n \
      --argjson before "$(jq '.total_latency_ms' <<<"${before_traffic}")" \
      --argjson after "$(jq '.total_latency_ms' <<<"${after_traffic}")" \
      '$after - $before'
  )"
  max_latency="$(jq '.trafficMaxLatencyMs' <<<"${after_summary}")"
  average_latency="$(
    jq -n --argjson total "${total_latency}" --argjson events "${total_events}" '
      if $events == 0 then 0 else ($total / $events) end
    '
  )"
  retry_rate="$(
    jq -n --argjson retried "${retried_events}" --argjson events "${total_events}" '
      if $events == 0 then 0 else ($retried / $events) end
    '
  )"
  failover_success_rate="$(
    jq -n --argjson successes "${retried_success_events}" --argjson retried "${retried_events}" '
      if $retried == 0 then 0 else ($successes / $retried) end
    '
  )"
  weighted_edge_a_before="$(
    jq -r --arg edge "edge:route:HTTPRoute:${TEST_NAMESPACE}/weighted-route:backend:${TEST_NAMESPACE}/weighted-a:8080" '
      [.edges[] | select(.edge_id == $edge) | .events] | add // 0
    ' <<<"${before_traffic}"
  )"
  weighted_edge_a_after="$(
    jq -r --arg edge "edge:route:HTTPRoute:${TEST_NAMESPACE}/weighted-route:backend:${TEST_NAMESPACE}/weighted-a:8080" '
      [.edges[] | select(.edge_id == $edge) | .events] | add // 0
    ' <<<"${after_traffic}"
  )"
  weighted_edge_b_before="$(
    jq -r --arg edge "edge:route:HTTPRoute:${TEST_NAMESPACE}/weighted-route:backend:${TEST_NAMESPACE}/weighted-b:8080" '
      [.edges[] | select(.edge_id == $edge) | .events] | add // 0
    ' <<<"${before_traffic}"
  )"
  weighted_edge_b_after="$(
    jq -r --arg edge "edge:route:HTTPRoute:${TEST_NAMESPACE}/weighted-route:backend:${TEST_NAMESPACE}/weighted-b:8080" '
      [.edges[] | select(.edge_id == $edge) | .events] | add // 0
    ' <<<"${after_traffic}"
  )"
  weighted_edge_a="$(
    jq -n --argjson before "${weighted_edge_a_before}" --argjson after "${weighted_edge_a_after}" '
      $after - $before
    '
  )"
  weighted_edge_b="$(
    jq -n --argjson before "${weighted_edge_b_before}" --argjson after "${weighted_edge_b_after}" '
      $after - $before
    '
  )"
  imbalance="$(
    jq -n --argjson a "${weighted_edge_a}" --argjson b "${weighted_edge_b}" '
      if ($a + $b) == 0 then 0
      else (((($b / ($a + $b)) - 0.75) | if . < 0 then -. else . end))
      end
    '
  )"

  printf '[upstream-behavior] derived metrics: total_events=%s retried_events=%s retry_attempts=%s retried_success_events=%s retry_rate=%s average_latency_ms=%s max_latency_ms=%s failover_success_rate=%s backend_imbalance=%s\n' \
    "${total_events}" "${retried_events}" "${retry_attempts}" "${retried_success_events}" "${retry_rate}" "${average_latency}" "${max_latency}" "${failover_success_rate}" "${imbalance}"
}

validate_metrics_endpoint() {
  local metrics
  local summary

  metrics="$(metrics_text)"
  grep -q 'nantian_gateway_dataplane_traffic_retry_attempts_total' <<<"${metrics}" || {
    log "metrics endpoint is missing retry attempts counter"
    exit 1
  }
  grep -q 'nantian_gateway_dataplane_traffic_retry_rate' <<<"${metrics}" || {
    log "metrics endpoint is missing retry rate gauge"
    exit 1
  }
  grep -q 'nantian_gateway_dataplane_traffic_failover_success_rate' <<<"${metrics}" || {
    log "metrics endpoint is missing failover success rate gauge"
    exit 1
  }
  grep -q 'nantian_gateway_dataplane_traffic_latency_ms_max' <<<"${metrics}" || {
    log "metrics endpoint is missing max latency gauge"
    exit 1
  }
  grep -q 'nantian_gateway_dataplane_traffic_upstream_pool_hit_ratio' <<<"${metrics}" || {
    log "metrics endpoint is missing upstream pool hit ratio gauge"
    exit 1
  }
  grep -q 'nantian_gateway_dataplane_traffic_upstream_connect_latency_ms_average' <<<"${metrics}" || {
    log "metrics endpoint is missing upstream connect latency average gauge"
    exit 1
  }
  grep -q 'nantian_gateway_dataplane_traffic_upstream_connect_latency_ms_max' <<<"${metrics}" || {
    log "metrics endpoint is missing upstream connect latency max gauge"
    exit 1
  }

  summary="$(summary_json)"
  jq -e '
    (.trafficRetriedSuccessEvents >= 1)
    and (.trafficRetryRate > 0)
    and (.trafficFailoverSuccessRate > 0)
    and (.trafficUpstreamPoolHits >= 1)
    and (.trafficUpstreamPoolMisses >= 1)
    and (.trafficUpstreamPoolHitRatio > 0)
    and (.trafficUpstreamConnectLatencyMsAvg >= 0)
    and (.trafficUpstreamConnectLatencyMsMax >= 0)
  ' <<<"${summary}" >/dev/null 2>&1 || {
    log "summary view is missing upstream retry/failover or pool/connect observability"
    jq '.' <<<"${summary}" >&2
    exit 1
  }
}
