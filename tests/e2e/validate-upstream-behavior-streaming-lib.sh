traffic_response_flag_count() {
  local traffic="$1"
  local flag="$2"

  jq -r --arg flag "${flag}" '.response_flags[$flag] // 0' <<<"${traffic}"
}

traffic_retry_attempts() {
  local traffic="$1"

  jq -r '.total_retry_attempts // 0' <<<"${traffic}"
}

streaming_request() {
  local headers_file="$1"
  local body_file="$2"
  local curl_error_file="$3"

  curl \
    --silent \
    --show-error \
    --http1.1 \
    --no-buffer \
    --noproxy '*' \
    --max-time "${STREAM_CURL_MAX_TIME_SEC}" \
    --resolve "${STREAM_HOST}:${GATEWAY_HOST_PORT}:127.0.0.1" \
    -H 'Accept: text/event-stream' \
    -H 'Connection: close' \
    -D "${headers_file}" \
    -o "${body_file}" \
    -w '%{http_code} %{time_total}' \
    "http://${STREAM_HOST}:${GATEWAY_HOST_PORT}/stream" \
    2>"${curl_error_file}"
}

assert_streaming_response() {
  local status="$1"
  local elapsed="$2"
  local headers_file="$3"
  local body_file="$4"
  local content_type
  local minimum_elapsed

  if [[ "${status}" != "200" ]]; then
    log "expected streaming route to return HTTP 200"
    printf 'status=%s elapsed=%s\n' "${status}" "${elapsed}" >&2
    cat "${headers_file}" >&2
    cat "${body_file}" >&2
    exit 1
  fi

  content_type="$(header_value "${headers_file}" "Content-Type")"
  if [[ "${content_type}" != text/event-stream* ]]; then
    log "expected streaming route to preserve text/event-stream content type"
    printf 'content_type=%s\n' "${content_type}" >&2
    cat "${headers_file}" >&2
    exit 1
  fi

  grep -Fq 'data: first' "${body_file}" || {
    log "streaming response did not include the first event"
    cat "${body_file}" >&2
    exit 1
  }
  grep -Fq 'data: second' "${body_file}" || {
    log "streaming response did not include the second event after idle gap"
    cat "${body_file}" >&2
    exit 1
  }

  minimum_elapsed="$(
    awk -v gap_ms="${STREAM_IDLE_GAP_MS}" 'BEGIN {
      value = (gap_ms / 1000.0) - 0.25
      if (value < 0) {
        value = 0
      }
      printf "%.3f", value
    }'
  )"
  awk -v value="${elapsed}" -v minimum="${minimum_elapsed}" -v maximum="${STREAM_CURL_MAX_TIME_SEC}" \
    'BEGIN { exit !(value >= minimum && value < maximum) }' || {
    log "streaming response elapsed time did not reflect the configured idle gap"
    printf 'elapsed=%s expected_range=[%s,%s)\n' \
      "${elapsed}" "${minimum_elapsed}" "${STREAM_CURL_MAX_TIME_SEC}" >&2
    exit 1
  }
}

wait_for_streaming_route_ready() {
  local curl_info
  local status
  local elapsed

  for attempt in $(seq 1 30); do
    if curl_info="$(
      streaming_request \
        "${TMP_DIR}/stream-ready.headers" \
        "${TMP_DIR}/stream-ready.body" \
        "${TMP_DIR}/stream-ready.curl.err"
    )"; then
      status="${curl_info%% *}"
      elapsed="${curl_info##* }"
      if [[ "${status}" == "200" ]] \
        && grep -Fq 'data: first' "${TMP_DIR}/stream-ready.body" \
        && grep -Fq 'data: second' "${TMP_DIR}/stream-ready.body"
      then
        return
      fi
    fi

    if [[ "${attempt}" -eq 30 ]]; then
      break
    fi
    sleep 2
  done

  log "streaming route did not become ready"
  cat "${TMP_DIR}/stream-ready.curl.err" >&2 || true
  cat "${TMP_DIR}/stream-ready.headers" >&2 || true
  cat "${TMP_DIR}/stream-ready.body" >&2 || true
  exit 1
}

validate_streaming_http() {
  local before_traffic
  local after_traffic
  local before_ut
  local after_ut
  local before_retries
  local after_retries
  local curl_info
  local status
  local elapsed

  wait_for_streaming_route_ready

  before_traffic="$(traffic_json)"
  before_ut="$(traffic_response_flag_count "${before_traffic}" UT)"
  before_retries="$(traffic_retry_attempts "${before_traffic}")"

  if ! curl_info="$(
    streaming_request \
      "${TMP_DIR}/stream.headers" \
      "${TMP_DIR}/stream.body" \
      "${TMP_DIR}/stream.curl.err"
  )"; then
    log "streaming request failed before receiving a complete response"
    cat "${TMP_DIR}/stream.curl.err" >&2
    cat "${TMP_DIR}/stream.headers" >&2 || true
    cat "${TMP_DIR}/stream.body" >&2 || true
    exit 1
  fi

  status="${curl_info%% *}"
  elapsed="${curl_info##* }"
  assert_streaming_response "${status}" "${elapsed}" "${TMP_DIR}/stream.headers" "${TMP_DIR}/stream.body"

  after_traffic="$(traffic_json)"
  after_ut="$(traffic_response_flag_count "${after_traffic}" UT)"
  after_retries="$(traffic_retry_attempts "${after_traffic}")"

  if [[ "${after_ut}" -ne "${before_ut}" ]]; then
    log "streaming request unexpectedly increased upstream timeout response flags"
    printf 'before_ut=%s after_ut=%s\n' "${before_ut}" "${after_ut}" >&2
    exit 1
  fi
  if [[ "${after_retries}" -ne "${before_retries}" ]]; then
    log "streaming request unexpectedly triggered retries"
    printf 'before_retries=%s after_retries=%s\n' "${before_retries}" "${after_retries}" >&2
    exit 1
  fi

  wait_for_admin_route_edges streaming-route 1
  log "streaming HTTP/SSE route preserved both chunks across ${STREAM_IDLE_GAP_MS}ms idle gap without UT or retry"
}

write_streaming_profile() {
  local protocol="$1"
  local output="$2"
  local accept_header="$3"
  local tmp

  mkdir -p "$(dirname "${output}")"
  python3 "${ROOT_DIR}/tests/e2e/http_concurrency_client.py" \
    --url "http://127.0.0.1:${GATEWAY_HOST_PORT}/stream" \
    --host-header "${STREAM_HOST}" \
    --requests "${STREAM_PROFILE_REQUESTS}" \
    --concurrency "${STREAM_PROFILE_CONCURRENCY}" \
    --connect-timeout 3 \
    --request-timeout "${STREAM_PROFILE_REQUEST_TIMEOUT}" \
    --connection-mode close \
    --header "Accept: ${accept_header}" \
    --expect-status 200 \
    --expect-body-substring "data: second" \
    --output "${output}" >/dev/null

  tmp="$(mktemp)"
  jq \
    --arg protocol "${protocol}" \
    --argjson stream_idle_gap_ms "${STREAM_IDLE_GAP_MS}" \
    '. + {
      protocol: $protocol,
      scenario: "long-lived-streaming",
      stream_idle_gap_ms: $stream_idle_gap_ms,
      expected_body_substring: "data: second"
    }' "${output}" >"${tmp}"
  mv "${tmp}" "${output}"
}

write_streaming_profiles() {
  if [[ -z "${PROFILE_OUTPUT_DIR}" ]]; then
    return
  fi

  log "writing streaming HTTP/SSE/MCP profile evidence to ${PROFILE_OUTPUT_DIR}"
  write_streaming_profile \
    sse \
    "${PROFILE_OUTPUT_DIR}/sse/long-lived-streaming.json" \
    "text/event-stream"
  write_streaming_profile \
    mcp \
    "${PROFILE_OUTPUT_DIR}/mcp/streamable-http.json" \
    "application/json, text/event-stream"
}
