POD_HTTP_PORT_FORWARD_PID=""
POD_HTTP_PORT_FORWARD_PIDS=()

ready_dataplane_pods() {
  k -n "${DATAPLANE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o json \
    | jq -r '
      .items[]
      | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
      | .metadata.name
    ' \
    | sort
}

wait_for_ready_dataplane_pods() {
  local count

  for _ in $(seq 1 90); do
    count="$(ready_dataplane_pods | wc -l | tr -d ' ')"
    if [[ "${count}" -ge 1 ]]; then
      return
    fi
    sleep 2
  done

  log "dataplane did not have any ready pods"
  k -n "${DATAPLANE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o wide >&2 || true
  exit 1
}

capture_dataplane_pod_identity() {
  local output="$1"

  k -n "${DATAPLANE_NAMESPACE}" get pod -l "${DATAPLANE_SELECTOR}" -o json \
    | jq '
      [
        .items[]
        | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
        | {
            name: .metadata.name,
            uid: .metadata.uid,
            restarts: ([.status.containerStatuses[]?.restartCount] | add // 0)
          }
      ]
      | sort_by(.name)
    ' >"${output}"
}

assert_dataplane_pods_unchanged() {
  local before="$1"
  local after="${before}.after"

  capture_dataplane_pod_identity "${after}"
  if ! diff -u "${before}" "${after}" >"${after}.diff"; then
    cat "${after}.diff" >&2
    log "dataplane pod identity or restart count changed during session secret rotation"
    exit 1
  fi
}

start_pod_http_port_forward() {
  local pod="$1"
  local local_port="$2"
  local log_file="$3"
  local pid

  k -n "${DATAPLANE_NAMESPACE}" port-forward "pod/${pod}" "${local_port}:${DATAPLANE_HTTP_FORWARD_PORT}" \
    >"${log_file}" 2>&1 &
  pid="$!"

  for _ in $(seq 1 30); do
    if port_listening "${local_port}"; then
      POD_HTTP_PORT_FORWARD_PID="${pid}"
      POD_HTTP_PORT_FORWARD_PIDS+=("${pid}")
      return
    fi
    sleep 1
  done

  log "timed out waiting for dataplane HTTP port-forward for pod ${pod}"
  cat "${log_file}" >&2 || true
  kill "${pid}" >/dev/null 2>&1 || true
  wait "${pid}" >/dev/null 2>&1 || true
  exit 1
}

stop_port_forward_pid() {
  local pid="$1"

  if [[ -n "${pid}" ]]; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
}

stop_all_pod_http_port_forwards() {
  local pid

  for pid in "${POD_HTTP_PORT_FORWARD_PIDS[@]:-}"; do
    stop_port_forward_pid "${pid}"
  done
  POD_HTTP_PORT_FORWARD_PIDS=()
  POD_HTTP_PORT_FORWARD_PID=""
}

base64url_decode() {
  local value="$1"
  local remainder

  remainder=$((${#value} % 4))
  case "${remainder}" in
    0) ;;
    2) value="${value}==" ;;
    3) value="${value}=" ;;
    *) return 1 ;;
  esac

  printf '%s' "${value}" | tr '_-' '/+' | openssl base64 -d -A
}

token_signature_matches_secret() {
  local token="$1"
  local secret="$2"
  local encoded_body="${token%%.*}"
  local encoded_signature="${token#*.}"
  local expected_signature

  if [[ "${encoded_body}" == "${token}" || -z "${encoded_signature}" ]]; then
    return 1
  fi

  expected_signature="$(
    base64url_decode "${encoded_body}" \
      | openssl dgst -sha256 -mac HMAC -macopt "key:${secret}" -binary \
      | openssl base64 -A \
      | tr '+/' '-_' \
      | tr -d '='
  )" || return 1

  [[ "${encoded_signature}" == "${expected_signature}" ]]
}

extract_cookie_token_from_headers() {
  local headers_file="$1"

  grep -i "^set-cookie: ${SESSION_COOKIE_NAME}=" "${headers_file}" \
    | tail -n 1 \
    | cut -d= -f2- \
    | cut -d';' -f1 \
    | tr -d '\r'
}

extract_cookie_token_from_jar() {
  local cookie_jar="$1"

  awk -v name="${SESSION_COOKIE_NAME}" '
    BEGIN { FS = "\t" }
    /^#HttpOnly_/ { sub(/^#HttpOnly_/, "", $1) }
    $0 !~ /^#/ && $6 == name { token = $7 }
    END { if (token != "") print token }
  ' "${cookie_jar}"
}

request_gateway_on_port() {
  local port="$1"
  local headers_file="$2"
  local body_file="$3"
  local cookie_jar="$4"

  curl -sS \
    --http1.1 \
    --noproxy '*' \
    --resolve "${TEST_HOST}:${port}:127.0.0.1" \
    --connect-timeout 2 \
    --max-time 10 \
    -D "${headers_file}" \
    -o "${body_file}" \
    -c "${cookie_jar}" \
    -b "${cookie_jar}" \
    -H 'Connection: close' \
    -w '%{http_code}' \
    "http://${TEST_HOST}:${port}${TEST_PATH}"
}

wait_for_initial_response_on_port() {
  local port="$1"
  local headers_file="$2"
  local body_file="$3"
  local cookie_jar="$4"
  local status
  local body

  for _ in $(seq 1 45); do
    : >"${headers_file}"
    : >"${body_file}"
    status="$(request_gateway_on_port "${port}" "${headers_file}" "${body_file}" "${cookie_jar}" || true)"
    body="$(tr -d '\r\n' <"${body_file}")"
    if [[ "${status}" == "200" && ( "${body}" == "backend-a" || "${body}" == "backend-b" ) ]]; then
      printf '%s\n' "${body}"
      return
    fi
    sleep 2
  done

  log "did not receive a successful response from dataplane pod forwarded on port ${port}"
  exit 1
}

capture_initial_cookie_for_pod() {
  local pod="$1"
  local local_port
  local forward_pid
  local cookie_jar="${TMP_DIR}/${pod}.cookies.txt"
  local headers_file="${TMP_DIR}/${pod}.headers.txt"
  local body_file="${TMP_DIR}/${pod}.body.txt"
  local token

  local_port="$(find_free_tcp_port 30080)"
  start_pod_http_port_forward "${pod}" "${local_port}" "${TMP_DIR}/${pod}.http-port-forward.log"
  forward_pid="${POD_HTTP_PORT_FORWARD_PID}"
  wait_for_initial_response_on_port "${local_port}" "${headers_file}" "${body_file}" "${cookie_jar}" >/dev/null
  stop_port_forward_pid "${forward_pid}"

  token="$(extract_cookie_token_from_jar "${cookie_jar}")"
  if [[ -z "${token}" ]]; then
    log "pod ${pod} did not issue ${SESSION_COOKIE_NAME} cookie"
    cat "${headers_file}" >&2 || true
    exit 1
  fi
  if ! token_signature_matches_secret "${token}" "${INITIAL_SESSION_SECRET}"; then
    log "pod ${pod} issued a session token that was not signed by the initial secret"
    exit 1
  fi
  if token_signature_matches_secret "${token}" "${ROTATED_SESSION_SECRET}"; then
    log "pod ${pod} initial session token unexpectedly matched the rotated secret"
    exit 1
  fi

  log "captured initial session cookie from dataplane pod ${pod}"
}

wait_for_rotated_cookie_for_pod() {
  local pod="$1"
  local local_port
  local forward_pid
  local deadline
  local cookie_jar="${TMP_DIR}/${pod}.cookies.txt"
  local headers_file="${TMP_DIR}/${pod}.rotated.headers.txt"
  local body_file="${TMP_DIR}/${pod}.rotated.body.txt"
  local status
  local token

  local_port="$(find_free_tcp_port 30080)"
  start_pod_http_port_forward "${pod}" "${local_port}" "${TMP_DIR}/${pod}.rotated-http-port-forward.log"
  forward_pid="${POD_HTTP_PORT_FORWARD_PID}"
  deadline="$((SECONDS + SESSION_SECRET_UPDATE_TIMEOUT_SEC))"

  while (( SECONDS < deadline )); do
    : >"${headers_file}"
    : >"${body_file}"
    status="$(request_gateway_on_port "${local_port}" "${headers_file}" "${body_file}" "${cookie_jar}" || true)"
    token="$(extract_cookie_token_from_headers "${headers_file}" || true)"
    if [[ -z "${token}" ]]; then
      token="$(extract_cookie_token_from_jar "${cookie_jar}" || true)"
    fi

    if [[ "${status}" == "200" && -n "${token}" ]]; then
      if token_signature_matches_secret "${token}" "${ROTATED_SESSION_SECRET}" \
        && ! token_signature_matches_secret "${token}" "${INITIAL_SESSION_SECRET}"; then
        stop_port_forward_pid "${forward_pid}"
        log "dataplane pod ${pod} started signing session cookies with the rotated secret"
        return
      fi
    fi

    sleep 2
  done

  stop_port_forward_pid "${forward_pid}"
  log "dataplane pod ${pod} did not observe the rotated session secret before timeout"
  cat "${headers_file}" >&2 || true
  exit 1
}

verify_session_secret_rotation() {
  local pods_file="${TMP_DIR}/dataplane-pods.txt"
  local pod

  wait_for_ready_dataplane_pods
  ready_dataplane_pods >"${pods_file}"
  while IFS= read -r pod; do
    [[ -n "${pod}" ]] || continue
    capture_initial_cookie_for_pod "${pod}"
  done <"${pods_file}"

  capture_dataplane_pod_identity "${TMP_DIR}/dataplane-pods-before-rotation.json"

  log "rotating session persistence Secret without restarting dataplane pods"
  apply_session_secret "${ROTATED_SESSION_SECRET}"

  while IFS= read -r pod; do
    [[ -n "${pod}" ]] || continue
    wait_for_rotated_cookie_for_pod "${pod}"
  done <"${pods_file}"

  assert_dataplane_pods_unchanged "${TMP_DIR}/dataplane-pods-before-rotation.json"
  log "session persistence secret rotation was observed without dataplane pod restarts"
}
