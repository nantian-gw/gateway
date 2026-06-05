# Metrics Catalog

This document catalogs all Prometheus metrics exposed by Aether Gateway — control plane and data plane.

---

## Control Plane

### Snapshot Build

Snapshot-wide metrics that cover rebuild attempts, outcomes, and resource shape. **Note:** snapshot metrics use the `aether_gateway_snapshot_` prefix (no `controlplane_`), unlike other control-plane families.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_snapshot_builds_total` | Counter | — | Total snapshot rebuild attempts |
| `aether_gateway_snapshot_build_failures_total` | Counter | — | Failed snapshot rebuilds |
| `aether_gateway_snapshot_published_total` | Counter | — | Successfully published snapshots |
| `aether_gateway_snapshot_last_build_success` | Gauge | — | 1 if last build succeeded, 0 otherwise |
| `aether_gateway_snapshot_build_duration_seconds` | Histogram | — | Build duration (DefBuckets) |
| `aether_gateway_snapshot_resource_count` | Histogram | `resource` | Resource counts per snapshot (exp buckets 1..2048) |
| `aether_gateway_snapshot_listener_attached_routes` | Histogram | — | Attached route fanout per listener (exp buckets 1..2048) |

### Admin API

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_controlplane_admin_requests_total` | Counter | `method`, `route`, `status_class` | Admin API request count |
| `aether_gateway_controlplane_admin_request_duration_seconds` | Histogram | `method`, `route`, `status_class` | Admin API request latency (DefBuckets) |

Label values for `route`: `livez`, `readyz`, `summary`, `snapshot_sync`, `snapshot`, `listeners`, `listener_detail`, `routes`, `route_detail`, `nodes`, `node_detail`, `backends`, `backend_detail`, `dataplanes`, `dataplane_detail`, `infrastructure`, `service_catalog`, `resource_kinds`, `resources`, `resource_detail`, `chatbot_config`, `chatbot_query`, `metrics_config`, `metrics_query`, `metrics_query_range`, `topology`, `manage`, `unknown`

### xDS / gRPC

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_controlplane_xds_snapshot_fanout_coalesced_total` | Counter | — | Pending snapshots replaced by newer publishes |
| `aether_gateway_controlplane_xds_stream_terminations_total` | Counter | `reason` | Stream terminations by reason |
| `aether_gateway_controlplane_xds_status_report_rejections_total` | Counter | `reason` | Status report rejections by reason |
| `aether_gateway_controlplane_xds_snapshot_send_duration_seconds` | Histogram | — | Send duration per stream (DefBuckets) |
| `aether_gateway_controlplane_xds_snapshot_send_timeouts_total` | Counter | — | Streams timed out during send |
| `aether_gateway_controlplane_xds_snapshot_ack_timeouts_total` | Counter | — | Streams timed out waiting for ACK/NACK |
| `aether_gateway_controlplane_xds_publish_ack_lag_seconds` | Histogram | — | Publish → ACK latency (DefBuckets) |
| `aether_gateway_controlplane_xds_publish_nack_lag_seconds` | Histogram | — | Publish → NACK latency (DefBuckets) |

### Node Status Persistence

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_controlplane_node_status_persist_queue_depth` | Gauge | — | Pending distinct node status updates |
| `aether_gateway_controlplane_node_status_persist_pending_nodes` | Gauge | — | Nodes in debounce window |
| `aether_gateway_controlplane_node_status_persist_enqueued_total` | Counter | — | Updates accepted into backlog |
| `aether_gateway_controlplane_node_status_persist_dropped_total` | Counter | — | Updates dropped (backlog full) |
| `aether_gateway_controlplane_node_status_persist_immediate_total` | Counter | — | Immediate updates accepted |
| `aether_gateway_controlplane_node_status_persist_debounced_total` | Counter | — | Debounced updates accepted |
| `aether_gateway_controlplane_node_status_persist_flush_duration_seconds` | Histogram | — | Batch flush duration (DefBuckets) |

### Reconciler Runner

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_controlplane_reconciler_runner_runs_total` | Counter | — | Total runner executions |
| `aether_gateway_controlplane_reconciler_runner_failures_total` | Counter | — | Failed runner executions |
| `aether_gateway_controlplane_reconciler_runner_last_run_success` | Gauge | — | 1 if last run succeeded |
| `aether_gateway_controlplane_reconciler_runner_duration_seconds` | Histogram | `scope` | Run duration by scope (DefBuckets) |
| `aether_gateway_controlplane_reconciler_runner_queue_depth` | Gauge | — | Current trigger queue depth |
| `aether_gateway_controlplane_reconciler_runner_triggers_enqueued_total` | Counter | — | Triggers accepted into queue |
| `aether_gateway_controlplane_reconciler_runner_triggers_deduplicated_total` | Counter | — | Triggers dropped (queue full) |
| `aether_gateway_controlplane_reconciler_runner_triggers_settled_total` | Counter | — | Triggers through settle window |
| `aether_gateway_controlplane_reconciler_runner_settle_pending` | Gauge | — | 1 if settle trigger pending |
| `aether_gateway_controlplane_reconciler_runner_retries_scheduled_total` | Counter | — | Failure-triggered retries scheduled |
| `aether_gateway_controlplane_reconciler_runner_retry_pending` | Gauge | — | 1 if retry pending |

### Gateway Convergence

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_controlplane_gateway_convergence_generation_lag` | Histogram | `stage` | Gateway spec generation → status observed lag (buckets: 1, 2, 3, 5, 8, 13) |
| `aether_gateway_controlplane_gateway_programmed_pending_total` | Counter | `reason` | Gateways not yet Programmed |
| `aether_gateway_controlplane_gateway_convergence_stage_total` | Gauge | `stage` | Deprecated alias for `gateway_convergence_stage_current` |
| `aether_gateway_controlplane_gateway_convergence_stage_current` | Gauge | `stage` | Current number of managed Gateways at each convergence stage |

Convergence stages: `managed`, `translated`, `infrastructure_converged`, `programmed`

### Go Runtime & Controller-Runtime

The control plane also exposes standard Go and controller-runtime metrics via `collectors.NewGoCollector()` and `collectors.NewProcessCollector()`:

- `go_*` — Go runtime (goroutines, memstats, threads, GC)
- `process_*` — OS process (CPU seconds, resident memory, virtual memory, open FDs, max FDs)
- `controller_runtime_*` — controller-runtime reconcile metrics
- `rest_client_*` — Kubernetes API client metrics

---

## Data Plane

### Snapshot Inventory

Counters for resources in the active configuration snapshot.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_ready` | Gauge | — | 1 if dataplane readiness check passes, 0 otherwise |
| `aether_gateway_dataplane_listener_count` | Gauge | — | Active listeners |
| `aether_gateway_dataplane_http_route_count` | Gauge | — | HTTPRoute resources in active snapshot |
| `aether_gateway_dataplane_grpc_route_count` | Gauge | — | GRPCRoute resources in active snapshot |
| `aether_gateway_dataplane_stream_route_count` | Gauge | — | TCPRoute, UDPRoute, and TLSRoute resources |
| `aether_gateway_dataplane_backend_count` | Gauge | — | Backend clusters in active snapshot |
| `aether_gateway_dataplane_secret_count` | Gauge | — | TLS secrets in active snapshot |

### Node Info

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_node_info` | Gauge | `node_id`, `cluster`, `snapshot_version`, `xds_last_snapshot_version`, `last_good_snapshot_version`, `current_snapshot_status`, `current_snapshot_rejection_version`, `current_snapshot_rejection_runtime`, `runtime_http_required`, `runtime_http_current_status`, `runtime_tls_required`, `runtime_tls_current_status`, `runtime_stream_required`, `runtime_stream_current_status`, …(18+ labels) | Static node identity and runtime state (always 1) |

### xDS Client

Connection, snapshot, and error state for the control-plane xDS stream.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_xds_connect_failures_total` | Counter | — | Failed control-plane connection attempts |
| `aether_gateway_dataplane_xds_stream_failures_total` | Counter | — | xDS stream failures triggering retry |
| `aether_gateway_dataplane_xds_last_connect_failure_unix_seconds` | Gauge | — | Unix timestamp of most recent connect failure |
| `aether_gateway_dataplane_xds_last_stream_failure_unix_seconds` | Gauge | — | Unix timestamp of most recent stream failure |
| `aether_gateway_dataplane_xds_last_connect_error_retained` | Gauge | — | 1 if last connect error detail is retained |
| `aether_gateway_dataplane_xds_last_stream_error_retained` | Gauge | — | 1 if last stream error detail is retained |
| `aether_gateway_dataplane_xds_snapshots_applied_total` | Counter | — | Snapshots successfully applied |
| `aether_gateway_dataplane_xds_snapshots_nacked_total` | Counter | — | Snapshots explicitly rejected |
| `aether_gateway_dataplane_xds_snapshots_skipped_total` | Counter | — | Duplicate snapshots skipped |
| `aether_gateway_dataplane_xds_last_apply_timestamp_seconds` | Gauge | — | Unix timestamp of most recent applied snapshot |
| `aether_gateway_dataplane_xds_last_nack_info` | Gauge | — | 1 if last snapshot was NACK'd |
| `aether_gateway_dataplane_xds_apply_stage_duration_ms` | Histogram | — | Apply stage timing per plane |

### Admin API

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_admin_requests_total` | Counter | `method`, `route`, `status_class` | Admin API request count |
| `aether_gateway_dataplane_admin_request_duration_seconds` | Histogram | `method`, `route`, `status_class` | Admin API request latency |

### Runtime

Snapshot acceptance status and listener reload outcomes per runtime plane.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_runtime_http_listener_reload_failures_total` | Counter | — | HTTP listener reload failures |
| `aether_gateway_dataplane_runtime_tls_listener_reload_failures_total` | Counter | — | TLS listener reload failures |
| `aether_gateway_dataplane_runtime_stream_listener_reload_failures_total` | Counter | — | Stream listener reload failures |
| `aether_gateway_dataplane_runtime_http_tls_asset_reuses_total` | Counter | — | HTTP TLS cert bundle disk reuse hits |
| `aether_gateway_dataplane_current_snapshot_rejected` | Gauge | — | 1 if current snapshot is rejected |
| `aether_gateway_dataplane_serving_last_good_snapshot` | Gauge | — | 1 if serving a retained last-good snapshot |
| `aether_gateway_dataplane_runtime_http_current_rejected` | Gauge | — | 1 if HTTP runtime rejected current snapshot |
| `aether_gateway_dataplane_runtime_tls_current_rejected` | Gauge | — | 1 if TLS runtime rejected current snapshot |
| `aether_gateway_dataplane_runtime_stream_current_rejected` | Gauge | — | 1 if stream runtime rejected current snapshot |
| `aether_gateway_dataplane_runtime_http_current_failure_count` | Gauge | — | HTTP listeners currently failing |
| `aether_gateway_dataplane_runtime_tls_current_failure_count` | Gauge | — | TLS listeners currently failing |
| `aether_gateway_dataplane_runtime_stream_current_failure_count` | Gauge | — | Stream listeners currently failing |

### Traffic

Request, bytes, latency, status, and upstream connection metrics for observed downstream traffic.

#### Events & Bytes

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_traffic_events_total` | Counter | — | All downstream requests, sessions, and datagrams |
| `aether_gateway_dataplane_traffic_request_events_total` | Counter | — | Request-like traffic events (HTTP, HTTPS, GRPC, GRPCS, H2C, HTTP2, HTTP/2) |
| `aether_gateway_dataplane_traffic_bytes_received_total` | Counter | — | Downstream bytes received |
| `aether_gateway_dataplane_traffic_bytes_sent_total` | Counter | — | Downstream bytes sent |

#### Latency

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_traffic_latency_ms_total` | Counter | — | Total downstream latency in ms |
| `aether_gateway_dataplane_traffic_latency_ms_max` | Gauge | — | Maximum observed downstream latency in ms |
| `aether_gateway_dataplane_traffic_request_latency_ms` | Histogram | — | Bucketed request latency in ms |

#### Status Codes

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_traffic_status_1xx_total` | Counter | — | 1xx responses |
| `aether_gateway_dataplane_traffic_status_2xx_total` | Counter | — | 2xx responses |
| `aether_gateway_dataplane_traffic_status_3xx_total` | Counter | — | 3xx responses |
| `aether_gateway_dataplane_traffic_status_4xx_total` | Counter | — | 4xx responses |
| `aether_gateway_dataplane_traffic_status_5xx_total` | Counter | — | 5xx responses |
| `aether_gateway_dataplane_traffic_status_other_total` | Counter | — | Non-standard status codes |
| `aether_gateway_dataplane_traffic_response_flags_total` | Counter | `flag` | Request completions by response flag |

Response flag values: `none`, `CB`, `DC`, `IB`, `IT`, `MA`, `NR`, `OL`, `RB`, `RH`, `RL`, `UC`, `UF`, `UH`, `UT`

#### Retries & Failover

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_traffic_retried_events_total` | Counter | — | Requests requiring ≥1 retry |
| `aether_gateway_dataplane_traffic_retry_attempts_total` | Counter | — | Total retry attempts |
| `aether_gateway_dataplane_traffic_retried_success_events_total` | Counter | — | Retried requests that succeeded |
| `aether_gateway_dataplane_traffic_retry_rate` | Gauge | — | Ratio of requests requiring retry |
| `aether_gateway_dataplane_traffic_failover_success_rate` | Gauge | — | Ratio of retried events that succeeded |

#### Upstream Pool & Connection

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_traffic_upstream_pool_hits_total` | Counter | — | Upstream acquisitions reusing pooled connection |
| `aether_gateway_dataplane_traffic_upstream_pool_misses_total` | Counter | — | Upstream acquisitions requiring new connection |
| `aether_gateway_dataplane_traffic_upstream_pool_hit_ratio` | Gauge | — | Ratio of upstream hits to total acquisitions |
| `aether_gateway_dataplane_traffic_upstream_peer_build_failures_total` | Counter | — | Peer build failures before connection |
| `aether_gateway_dataplane_traffic_upstream_tls_handshake_failures_total` | Counter | — | Upstream TLS handshake failures |
| `aether_gateway_dataplane_traffic_upstream_connect_latency_ms_total` | Counter | — | Total upstream connect latency in ms |
| `aether_gateway_dataplane_traffic_upstream_connect_latency_ms_max` | Gauge | — | Maximum upstream connect latency in ms |
| `aether_gateway_dataplane_traffic_upstream_connect_latency_ms_average` | Gauge | — | Average upstream connect latency in ms |
| `aether_gateway_dataplane_traffic_upstream_connect_latency_ms` | Histogram | — | Bucketed upstream connect latency in ms |
| `aether_gateway_dataplane_traffic_upstream_tls_handshake_failure_latency_ms` | Histogram | — | Bucketed latency before upstream TLS handshake failure |

### Protection

#### Overload Admission Control

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_http_overload_rejected_total` | Counter | `scope` | HTTP requests rejected by overload (scopes: `total`, `global`, `listener`, `route`) |
| `aether_gateway_dataplane_http_overload_rejected_listener_total` | Counter | `listener` | HTTP rejections per listener inflight budget |
| `aether_gateway_dataplane_http_overload_rejected_route_total` | Counter | `route` | HTTP rejections per route inflight budget |
| `aether_gateway_dataplane_tcp_overload_rejected_total` | Counter | `scope` | TCP sessions rejected by overload (scopes: `total`, `global`, `listener`) |
| `aether_gateway_dataplane_tcp_overload_rejected_listener_total` | Counter | `listener` | TCP rejections per listener connection budget |
| `aether_gateway_dataplane_udp_overload_rejected_total` | Counter | `scope` | UDP datagrams rejected by overload (scopes: `total`, `global`, `listener`) |
| `aether_gateway_dataplane_udp_overload_rejected_listener_total` | Counter | `listener` | UDP rejections per listener datagram budget |
| `aether_gateway_dataplane_http_global_inflight_current` | Gauge | — | Current HTTP global inflight requests |
| `aether_gateway_dataplane_http_listener_inflight_current` | Gauge | `listener` | Current HTTP inflight per listener |
| `aether_gateway_dataplane_http_route_inflight_current` | Gauge | `route` | Current HTTP inflight per route |
| `aether_gateway_dataplane_tcp_global_connections_current` | Gauge | — | Current TCP global connections |
| `aether_gateway_dataplane_tcp_listener_connections_current` | Gauge | `listener` | Current TCP connections per listener |
| `aether_gateway_dataplane_udp_global_datagrams_current` | Gauge | — | Current UDP global datagrams |
| `aether_gateway_dataplane_udp_listener_datagrams_current` | Gauge | `listener` | Current UDP datagrams per listener |

#### Circuit Breaker

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_http_circuit_breaker_backend_max_inflight_requests` | Gauge | — | Configured max inflight per backend |
| `aether_gateway_dataplane_http_circuit_breaker_backend_inflight_current` | Gauge | `backend` | Current inflight per backend |
| `aether_gateway_dataplane_http_circuit_breaker_rejected_total` | Counter | `scope` | Requests rejected by circuit breaker (scopes: `total`, `backend`) |
| `aether_gateway_dataplane_http_circuit_breaker_rejected_backend_total` | Counter | `backend` | Rejections per backend |

#### Rate Limiting

Rate limit metrics use three scopes: `global`, `listener`, and `route`.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_http_rate_limit_global_enabled` | Gauge | — | 1 if global scope enabled |
| `aether_gateway_dataplane_http_rate_limit_global_requests_per_second` | Gauge | — | Global requests/second limit |
| `aether_gateway_dataplane_http_rate_limit_global_burst` | Gauge | — | Global burst limit |
| `aether_gateway_dataplane_http_rate_limit_global_available_tokens` | Gauge | — | Global available tokens |
| `aether_gateway_dataplane_http_rate_limit_listener_enabled` | Gauge | — | 1 if per-listener scope enabled |
| `aether_gateway_dataplane_http_rate_limit_listener_requests_per_second` | Gauge | — | Per-listener requests/second limit |
| `aether_gateway_dataplane_http_rate_limit_listener_burst` | Gauge | — | Per-listener burst limit |
| `aether_gateway_dataplane_http_rate_limit_listener_available_tokens` | Gauge | `listener` | Available tokens per listener |
| `aether_gateway_dataplane_http_rate_limit_route_enabled` | Gauge | — | 1 if per-route scope enabled |
| `aether_gateway_dataplane_http_rate_limit_route_requests_per_second` | Gauge | — | Per-route requests/second limit |
| `aether_gateway_dataplane_http_rate_limit_route_burst` | Gauge | — | Per-route burst limit |
| `aether_gateway_dataplane_http_rate_limit_route_available_tokens` | Gauge | `route` | Available tokens per route |
| `aether_gateway_dataplane_http_rate_limit_rejected_total` | Counter | `scope` | Requests rejected by rate limit (scopes: `total`, `listener`, `route`) |
| `aether_gateway_dataplane_http_rate_limit_rejected_listener_total` | Counter | `listener` | Rejections per listener |
| `aether_gateway_dataplane_http_rate_limit_rejected_route_total` | Counter | `route` | Rejections per route |
| `aether_gateway_dataplane_http_rate_limit_allowed_total` | Counter | — | Requests allowed by rate limit |

#### Retry Budget

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_http_retry_budget_enabled` | Gauge | — | 1 if retry budget enabled |
| `aether_gateway_dataplane_http_retry_budget_burst` | Gauge | — | Retry budget burst |
| `aether_gateway_dataplane_http_retry_budget_ratio_percent` | Gauge | — | Retry budget ratio in percent |
| `aether_gateway_dataplane_http_retry_budget_available_tokens` | Gauge | — | Available retry budget tokens |
| `aether_gateway_dataplane_http_retry_budget_available_milli_tokens` | Gauge | — | Available retry budget milli-tokens (sub-token resolution) |
| `aether_gateway_dataplane_http_retry_budget_rejected_total` | Counter | — | Retries rejected by budget |
| `aether_gateway_dataplane_http_retry_budget_allowed_total` | Counter | — | Retries allowed by budget |
| `aether_gateway_dataplane_http_retry_budget_retryable_requests_observed_total` | Counter | — | Retryable requests observed |

#### Endpoint Health

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_endpoint_runtime_tracked_current` | Gauge | — | Current tracked endpoints |
| `aether_gateway_dataplane_endpoint_active_unhealthy_current` | Gauge | — | Currently unhealthy endpoints |
| `aether_gateway_dataplane_endpoint_passive_ejected_current` | Gauge | — | Passively ejected endpoints |
| `aether_gateway_dataplane_endpoint_recovery_latency_ms` | Gauge | `recovery_type` | Endpoint recovery latency in ms |

### Listener State Categories

Listener state is partitioned into eight category families. Each family exposes total, per-plane (`http`, `stream`, `tls`), and `none` (listeners not assigned to a plane) counts.

#### Current Status

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_current_idle_count` | Gauge | — | Listeners classified as idle |
| `aether_gateway_dataplane_listener_current_warming_count` | Gauge | — | Listeners classified as warming |
| `aether_gateway_dataplane_listener_current_pending_count` | Gauge | — | Listeners classified as pending |
| `aether_gateway_dataplane_listener_current_accepted_count` | Gauge | — | Listeners classified as accepted |
| `aether_gateway_dataplane_listener_current_retained_count` | Gauge | — | Listeners classified as retained |
| `aether_gateway_dataplane_listener_current_rejected_count` | Gauge | — | Listeners classified as rejected |
| `aether_gateway_dataplane_listener_current_stale_count` | Gauge | — | Listeners classified as stale |

#### Attention

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_attention_severity_level` | Gauge | — | 0=ok, 1=warning, 2=critical |
| `aether_gateway_dataplane_listener_attention_required_count` | Gauge | — | Listeners requiring operator attention |
| `aether_gateway_dataplane_listener_attention_pending_count` | Gauge | — | Pending listeners needing attention |
| `aether_gateway_dataplane_listener_attention_rejected_count` | Gauge | — | Rejected listeners needing attention |
| `aether_gateway_dataplane_listener_attention_stale_count` | Gauge | — | Stale listeners needing attention |
| `aether_gateway_dataplane_listener_attention_http_count` | Gauge | — | HTTP-plane listeners needing attention |
| `aether_gateway_dataplane_listener_attention_stream_count` | Gauge | — | Stream-plane listeners needing attention |
| `aether_gateway_dataplane_listener_attention_tls_count` | Gauge | — | TLS-plane listeners needing attention |
| `aether_gateway_dataplane_listener_attention_none_count` | Gauge | — | Plane-less listeners needing attention |
| `aether_gateway_dataplane_listener_attention_unrecovered_failure_count` | Gauge | — | Attention-worthy listeners with unrecovered failures |
| `aether_gateway_dataplane_listener_risk_pending_unrecovered_count` | Gauge | — | Pending + unrecovered failure |
| `aether_gateway_dataplane_listener_risk_rejected_unrecovered_count` | Gauge | — | Rejected + unrecovered failure |
| `aether_gateway_dataplane_listener_risk_stale_unrecovered_count` | Gauge | — | Stale + unrecovered failure |

#### Serving

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_serving_current_snapshot_count` | Gauge | — | Serving current snapshot version |
| `aether_gateway_dataplane_listener_serving_last_good_snapshot_count` | Gauge | — | Serving last-good snapshot |
| `aether_gateway_dataplane_listener_serving_state_none_count` | Gauge | — | No serving version exposed |
| `aether_gateway_dataplane_listener_serving_state_current_accepted_count` | Gauge | — | Serving via normal accepted apply |
| `aether_gateway_dataplane_listener_serving_state_current_retained_count` | Gauge | — | Serving via retained in-place state |
| `aether_gateway_dataplane_listener_serving_state_last_good_rejected_count` | Gauge | — | Serving last-good after explicit rejection |
| `aether_gateway_dataplane_listener_serving_state_last_good_stale_count` | Gauge | — | Serving last-good after drift |
| `aether_gateway_dataplane_listener_serving_drift_count` | Gauge | — | Listeners drifted from current snapshot |
| `aether_gateway_dataplane_listener_serving_drift_http_count` | Gauge | — | HTTP drifted |
| `aether_gateway_dataplane_listener_serving_drift_stream_count` | Gauge | — | Stream drifted |
| `aether_gateway_dataplane_listener_serving_drift_tls_count` | Gauge | — | TLS drifted |
| `aether_gateway_dataplane_listener_serving_drift_none_count` | Gauge | — | Plane-less drifted |

#### Convergence

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_convergence_severity_level` | Gauge | — | 0=converged, 1=warning, 2=critical |
| `aether_gateway_dataplane_listener_convergence_blocked_count` | Gauge | — | Not yet converged (pending/rejected/stale) |
| `aether_gateway_dataplane_listener_convergence_blocked_http_count` | Gauge | — | HTTP not converged |
| `aether_gateway_dataplane_listener_convergence_blocked_stream_count` | Gauge | — | Stream not converged |
| `aether_gateway_dataplane_listener_convergence_blocked_tls_count` | Gauge | — | TLS not converged |
| `aether_gateway_dataplane_listener_convergence_blocked_none_count` | Gauge | — | Plane-less not converged |

#### Awaiting Current Attempt

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_awaiting_current_attempt_count` | Gauge | — | Pending + current attempt in flight |
| `aether_gateway_dataplane_listener_awaiting_current_attempt_http_count` | Gauge | — | HTTP awaiting |
| `aether_gateway_dataplane_listener_awaiting_current_attempt_stream_count` | Gauge | — | Stream awaiting |
| `aether_gateway_dataplane_listener_awaiting_current_attempt_tls_count` | Gauge | — | TLS awaiting |
| `aether_gateway_dataplane_listener_awaiting_current_attempt_none_count` | Gauge | — | Plane-less awaiting |
| `aether_gateway_dataplane_listener_current_attempt_blocked_count` | Gauge | — | Attempt blocked (version drift) |
| `aether_gateway_dataplane_listener_current_attempt_blocked_http_count` | Gauge | — | HTTP blocked |
| `aether_gateway_dataplane_listener_current_attempt_blocked_stream_count` | Gauge | — | Stream blocked |
| `aether_gateway_dataplane_listener_current_attempt_blocked_tls_count` | Gauge | — | TLS blocked |
| `aether_gateway_dataplane_listener_current_attempt_blocked_none_count` | Gauge | — | Plane-less blocked |

#### Apply Blocked

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_apply_blocked_count` | Gauge | — | Apply blocked (overload guardian) |
| `aether_gateway_dataplane_listener_apply_blocked_http_count` | Gauge | — | HTTP apply blocked |
| `aether_gateway_dataplane_listener_apply_blocked_stream_count` | Gauge | — | Stream apply blocked |
| `aether_gateway_dataplane_listener_apply_blocked_tls_count` | Gauge | — | TLS apply blocked |
| `aether_gateway_dataplane_listener_apply_blocked_none_count` | Gauge | — | Plane-less apply blocked |

#### Recovery

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_failure_recovery_severity_level` | Gauge | — | 0=healthy, 1=warning, 2=critical |
| `aether_gateway_dataplane_listener_has_ever_failed_count` | Gauge | — | Listeners with ≥1 failure in current process |
| `aether_gateway_dataplane_listener_recovered_from_failure_count` | Gauge | — | Listeners recovered from failure |
| `aether_gateway_dataplane_listener_recovered_from_failure_http_count` | Gauge | — | HTTP recovered |
| `aether_gateway_dataplane_listener_recovered_from_failure_stream_count` | Gauge | — | Stream recovered |
| `aether_gateway_dataplane_listener_recovered_from_failure_tls_count` | Gauge | — | TLS recovered |
| `aether_gateway_dataplane_listener_recovered_from_failure_none_count` | Gauge | — | Plane-less recovered |

#### Unrecovered Failures

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_listener_unrecovered_failure_count` | Gauge | — | Failed and not yet recovered |
| `aether_gateway_dataplane_listener_unrecovered_failure_http_count` | Gauge | — | HTTP unrecovered |
| `aether_gateway_dataplane_listener_unrecovered_failure_stream_count` | Gauge | — | Stream unrecovered |
| `aether_gateway_dataplane_listener_unrecovered_failure_tls_count` | Gauge | — | TLS unrecovered |
| `aether_gateway_dataplane_listener_unrecovered_failure_none_count` | Gauge | — | Plane-less unrecovered |
| `aether_gateway_dataplane_listener_unrecovered_current_snapshot_failure_count` | Gauge | — | Unrecovered + active snapshot failures |
| `aether_gateway_dataplane_listener_unrecovered_current_snapshot_failure_http_count` | Gauge | — | HTTP unrecovered + active |
| `aether_gateway_dataplane_listener_unrecovered_current_snapshot_failure_stream_count` | Gauge | — | Stream unrecovered + active |
| `aether_gateway_dataplane_listener_unrecovered_current_snapshot_failure_tls_count` | Gauge | — | TLS unrecovered + active |
| `aether_gateway_dataplane_listener_unrecovered_current_snapshot_failure_none_count` | Gauge | — | Plane-less unrecovered + active |
| `aether_gateway_dataplane_listener_unrecovered_historical_failure_count` | Gauge | — | Failed in past, now stable |
| `aether_gateway_dataplane_listener_unrecovered_historical_failure_http_count` | Gauge | — | HTTP historical |
| `aether_gateway_dataplane_listener_unrecovered_historical_failure_stream_count` | Gauge | — | Stream historical |
| `aether_gateway_dataplane_listener_unrecovered_historical_failure_tls_count` | Gauge | — | TLS historical |
| `aether_gateway_dataplane_listener_unrecovered_historical_failure_none_count` | Gauge | — | Plane-less historical |

### UDP Sessions

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_udp_sessions_active_current` | Gauge | — | Active UDP upstream sessions |
| `aether_gateway_dataplane_udp_sessions_active_listener_current` | Gauge | `listener` | Active UDP sessions per listener |
| `aether_gateway_dataplane_udp_session_queue_depth_current` | Gauge | — | Queued UDP datagrams awaiting processing |
| `aether_gateway_dataplane_udp_session_queue_depth_listener_current` | Gauge | `listener` | UDP queue depth per listener |
| `aether_gateway_dataplane_udp_session_queue_overflow_dropped_total` | Counter | — | Datagrams dropped (queue full) |
| `aether_gateway_dataplane_udp_session_queue_overflow_dropped_listener_total` | Counter | `listener` | Datagrams dropped per listener |
| `aether_gateway_dataplane_udp_session_idle_evictions_total` | Counter | — | Sessions evicted after idle timeout |
| `aether_gateway_dataplane_udp_session_idle_evictions_listener_total` | Counter | `listener` | Evictions per listener |

### Access Log Writer

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_access_log_writer_count` | Gauge | — | Number of access-log writers |
| `aether_gateway_dataplane_access_log_writer_queue_depth` | Gauge | — | Pending log lines in buffer |
| `aether_gateway_dataplane_access_log_writer_flushes_total` | Counter | — | Flush operations |
| `aether_gateway_dataplane_access_log_writer_flush_latency_ms_total` | Counter | — | Total flush latency in ms |
| `aether_gateway_dataplane_access_log_writer_flush_latency_ms_max` | Gauge | — | Maximum flush latency in ms |
| `aether_gateway_dataplane_access_log_writer_dropped_lines_total` | Counter | — | Log lines dropped |
| `aether_gateway_dataplane_access_log_writer_sink_errors_total` | Counter | — | Sink write errors |

### Session Persistence

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_session_persistence_active` | Gauge | — | 1 if any route/backend policy uses session persistence |
| `aether_gateway_dataplane_session_persistence_route_rule_count` | Gauge | — | Routes with session persistence rules |
| `aether_gateway_dataplane_session_persistence_backend_policy_count` | Gauge | — | Backend policies with session persistence |

### HTTP/3

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aether_gateway_dataplane_http3_configured` | Gauge | — | 1 if HTTP/3 is configured |
| `aether_gateway_dataplane_http3_available` | Gauge | — | 1 if Rust proxy build supports HTTP/3 |
| `aether_gateway_dataplane_http3_enabled` | Gauge | — | 1 if HTTP/3 is configured and available |

### Process

Data-plane process-level metrics mirror the standard `process_*` namespace.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `process_cpu_seconds_total` | Counter | — | Total CPU time in seconds |
| `process_resident_memory_bytes` | Gauge | — | Resident set size in bytes |
| `process_virtual_memory_bytes` | Gauge | — | Virtual memory size in bytes |
| `process_open_fds` | Gauge | — | Open file descriptors |
| `process_max_fds` | Gauge | — | Maximum file descriptors |
| `process_threads` | Gauge | — | OS thread count |

---

## Recording Rules

These Prometheus recording rules are defined in `deploy/observability/prometheus/native/prometheus-dataplane-rules.yaml` and derive aggregated metrics from raw cAdvisor and kube-state-metrics data.

### Runtime Replicas

| Metric | Labels | Derivation |
|---|---|---|
| `aether_gateway_dataplane_ready_replicas` | `namespace`, `job` | `sum by (namespace, job) (aether_gateway_dataplane_runtime_supervisor_http_states)` |
| `aether_gateway_dataplane_targets` | `namespace`, `job` | `count by (namespace, job) (aether_gateway_dataplane_runtime_supervisor_http_states)` |
| `aether_gateway_dataplane_not_ready_replicas` | `namespace`, `job` | `count by (namespace, job) (aether_gateway_dataplane_runtime_supervisor_http_states == 0)` |

### Container Resources

These rules require cAdvisor and kube-state-metrics. They are **optional** — Grafana dashboards must fall back to raw container metrics when they are absent.

| Metric | Labels | Type | Derivation |
|---|---|---|---|
| `aether_gateway_dataplane_container_cpu_cores` | `namespace`, `pod`, `container` | Gauge | `sum by (ns,pod,container) (rate(container_cpu_usage_seconds_total{...}[5m]))` |
| `aether_gateway_dataplane_container_cpu_request_cores` | `namespace`, `pod`, `container` | Gauge | `max by (ns,pod,container) (kube_pod_container_resource_requests{...,resource="cpu"})` |
| `aether_gateway_dataplane_container_cpu_throttle_ratio` | `namespace`, `pod`, `container` | Gauge | `sum(rate(throttled[5m])) / clamp_min(sum(rate(periods[5m])), 1e-9)` |
| `aether_gateway_dataplane_container_memory_working_set_bytes` | `namespace`, `pod`, `container` | Gauge | `sum by (ns,pod,container) (container_memory_working_set_bytes{...})` |
| `aether_gateway_dataplane_container_memory_limit_bytes` | `namespace`, `pod`, `container` | Gauge | `max by (ns,pod,container) (kube_pod_container_resource_limits{...,resource="memory"})` |
| `aether_gateway_dataplane_container_memory_request_bytes` | `namespace`, `pod`, `container` | Gauge | `max by (ns,pod,container) (kube_pod_container_resource_requests{...,resource="memory"})` |

---

## Summary

| Plane | Metric Groups | Approx. Metric Count |
|---|---|---|
| Control Plane | Snapshot Build, Admin API, xDS/gRPC, Node Status, Reconciler Runner, Gateway Convergence, Go/Process Runtime | ~38 custom + Go std |
| Data Plane | Snapshot Inventory, Node Info, xDS Client, Admin API, Runtime, Traffic, Protection (Overload/CB/RL/RB/Endpoints), Listener State (8 categories), UDP Sessions, Access Log, Session Persistence, HTTP/3, Process | ~190 custom + process std |
| Recording Rules | Runtime Replicas, Container Resources | 9 |
| **Total** | | **~237 custom + runtime std** |