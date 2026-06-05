# Metrics Cardinality And Golden Signals

This document is the operator-facing contract for Aether Gateway Prometheus metrics, Grafana usage, and the admin API fields that support SLO review.

The goal is to keep default metrics safe to scrape in production. New metrics may be added additively, but every new metric must define its purpose, label set, cardinality class, unit, and Grafana or alerting consumer before it becomes part of the default surface.

## Scope

This contract covers:

- Controlplane `/metrics`.
- Controlplane pprof debug server when explicitly enabled.
- Dataplane `/metrics`.
- Grafana assets under `deploy/observability/grafana/`.
- Prometheus scrape, recording rule, and alerting assets under `deploy/observability/prometheus/`.
- Admin API summary views that expose the same operational concepts under `/v1/summary`, `/v1/nodes`, `/v1/node`, `/v1/listeners`, `/v1/routes`, and `/v1/backends`.

It does not freeze every internal debug field. Debug-only metrics may exist behind explicit opt-in surfaces, but they must not be required by the default Grafana views or release SLO evidence.

Controlplane observability HTTP surfaces must also have bounded runtime behavior. The default `/metrics` handler limits concurrent scrapes and applies a scrape timeout before Prometheus collection work can pile up under accidental or hostile concurrency. The optional pprof server remains disabled by default and, when enabled, must keep read/header/idle/write timeouts and bounded request headers.

## Cardinality Classes

| Class | Default use | Examples | Contract |
| --- | --- | --- | --- |
| Bounded | Allowed on default high-frequency counters, gauges, and histograms. | `plane`, `runtime`, `protocol`, `listener`, `route_kind`, `method`, `status_class`, `response_flag`, `scope`, `result`, `reason`, `resource`, `controller`. | Values must be drawn from enums, configured listeners, known controllers, or short closed vocabularies. |
| Controlled | Allowed on low-frequency inventory, admin, info, or `topk` Grafana views. Not allowed on default high-QPS histograms unless a review proves the bound is small. | `route_namespace`, `route_name`, `backend`, `node_id`, `snapshot_version`, `current_snapshot_status`. | The metric owner must document the expected upper bound and why the label is required. |
| Forbidden on default high-frequency metrics | Not allowed on default counters, gauges, or histograms emitted for every request, stream, or datagram. | `pod` as an emitted metric label, endpoint address, client IP, raw host, path, method when not normalized, user-provided header value, user ID, token, request_id, raw error. | Use access logs, admin detail endpoints, exemplars, traces, or explicit debug metrics instead. |

Prometheus service discovery may attach Kubernetes target labels such as `pod`, `namespace`, and `service`. That is separate from metric labels emitted by Aether Gateway. New code should not duplicate those labels inside metric families.

## Gateway Golden Signals

| Signal | Primary Prometheus source | Admin API / Grafana view | Cardinality policy | Current state |
| --- | --- | --- | --- | --- |
| request success rate | `1 - sum(rate(aether_gateway_dataplane_traffic_status_5xx_total[$__rate_interval])) / clamp_min(sum(rate(aether_gateway_dataplane_traffic_request_events_total[$__rate_interval])), 1)` with `aether_gateway_dataplane_traffic_response_flags_total` as gateway-error context. Do not use `aether_gateway_dataplane_traffic_events_total` as the denominator for HTTP ratios because it also counts TCP sessions and UDP datagrams. Status-class, response-flag, and retry counters are request-only; TCP sessions and UDP datagrams must not increment `aether_gateway_dataplane_traffic_status_other_total`, `aether_gateway_dataplane_traffic_response_flags_total{flag="none"}`, `aether_gateway_dataplane_traffic_retried_events_total`, or `aether_gateway_dataplane_traffic_retry_attempts_total`. | Dataplane `/v1/summary`; Grafana `Executive Overview` and traffic SLO panels. | Aggregate by `job` / `instance`; only use `status_class` and `response_flag` for breakdowns. | Covered by current counters; route/backend success-rate breakdowns need explicit cardinality review. |
| p99 / p999 latency | `aether_gateway_dataplane_traffic_request_latency_ms_bucket` is the default bounded request latency histogram for HTTP, HTTPS, GRPC, GRPCS, H2C, HTTP2, and HTTP/2 request-like traffic. Query p99 / p999 with `histogram_quantile()` over `rate(..._bucket{protocol=~"HTTP\|HTTPS\|GRPC\|GRPCS\|H2C\|HTTP2\|HTTP/2"}[window])`. `aether_gateway_dataplane_traffic_latency_ms_total` and `_max` remain coarse all-traffic summary helpers. | Dataplane `/v1/summary`; Grafana traffic deep dive. | Histogram labels are `listener`, `protocol`, `route_kind`, `status_class`, and `response_flag`; do not add `route_name`, `backend`, raw host, or path to this default high-QPS histogram. | Covered for observed request-like HTTP/gRPC events; TCP session and UDP datagram completion latency intentionally stay out of request SLO histograms. |
| upstream connect latency | `aether_gateway_dataplane_traffic_upstream_connect_latency_ms_bucket`, `_sum`, and `_count` provide p95/p99 for request-like HTTP/gRPC new upstream connections; `aether_gateway_dataplane_traffic_upstream_connect_latency_ms_average`, `_total`, and `_max` remain coarse helpers. TCP sessions, TLS passthrough sessions, and UDP datagrams must not increment upstream pool hit/miss, peer-build failure, or upstream connect latency counters because those metrics describe the HTTP/gRPC upstream acquisition path. | Dataplane `/v1/summary`; Grafana `Retry / Failover / Pool Hit Ratio and Connect Latency` and `Traffic Latency / Upstream Pool / TLS Asset Reuse`. | Default aggregate only. Backend-level `topk` views are acceptable when bounded and reviewed. | Covered for observed request-like HTTP/gRPC new upstream connections. |
| dataplane admin API health | `aether_gateway_dataplane_admin_requests_total` and `aether_gateway_dataplane_admin_request_duration_seconds_bucket`, `_sum`, and `_count` expose request rate, status class mix, and p95/p99 admin request latency. | Dataplane `/metrics`; Grafana `Dataplane Admin API Health`. | Labels are `method`, normalized admin `route`, and `status_class`. The `route` label is the admin route template such as `summary`, `metrics`, or `listener_detail`, not a Gateway API Route object name; unknown paths must collapse to `unknown`. Do not add raw path, token, client IP, user, or request ID labels. | Covered for probes, `/metrics`, and dataplane `/v1/*` admin requests. |
| snapshot apply latency | `aether_gateway_dataplane_xds_apply_stage_duration_ms_bucket`, `_sum`, and `_count` expose decode, inherit runtime state, index rebuild, snapshot swap, listener apply, and ACK wait stage durations. | Dataplane `/v1/node`; Grafana dataplane xDS panels. | Aggregate by dataplane scrape target; `stage` must remain a bounded enum. | Covered for xDS apply stages. |
| snapshot ACK latency | `aether_gateway_controlplane_xds_publish_ack_lag_seconds` and `aether_gateway_controlplane_xds_publish_nack_lag_seconds`. | Controlplane `/v1/nodes`; Grafana `Controlplane Deep Dive`. | Use `node_id` only for low-frequency node state or `topk` views. Default Grafana views should aggregate. | Covered by current histograms. |
| Gateway convergence | `aether_gateway_controlplane_gateway_convergence_stage_current`, `aether_gateway_controlplane_gateway_convergence_generation_lag`, and `aether_gateway_controlplane_gateway_programmed_pending_total`. | Controlplane status conditions; Grafana `Controlplane Gateway Convergence`. | `stage` and `reason` must remain bounded enums, Gateway API condition reasons, or the normalized fallback `Other`; do not add Gateway name/namespace labels to default metrics. | Covered for current managed Gateway stage counts, observed generation lag, and Programmed pending reasons. `aether_gateway_controlplane_gateway_convergence_stage_total` remains a deprecated compatibility alias only. |
| controlplane status update pressure | `aether_gateway_controlplane_status_update_conflicts_total`, `aether_gateway_controlplane_status_update_retries_total`, and `aether_gateway_controlplane_status_update_errors_total`. | Controlplane status writer; Grafana `Controlplane Deep Dive`. | `resource` and `reason` must remain bounded enums with unknown resources normalized to `other`; do not add object name/namespace labels. | Covered for status conflict, retry, and terminal error pressure. |
| dataplane ready replicas | Recording rule `aether_gateway_dataplane_ready_replicas` from per-pod `aether_gateway_dataplane_ready`. | Controlplane `/v1/nodes`, dataplane `/v1/node`; Grafana `Operator Overview`. | Ready is per scrape target. Replica-level views must scrape every pod or endpoint and aggregate. | Covered when Prometheus uses PodMonitor, ServiceMonitor endpoint discovery, or native endpoint slice discovery. |
| error flag / 5xx rate | `aether_gateway_dataplane_traffic_response_flags_total` and `aether_gateway_dataplane_traffic_status_5xx_total`. | Dataplane `/v1/summary`; Grafana traffic and alert panels. | `response_flag` must remain a short enum, not raw upstream error text. Response flag counters are request-like HTTP/gRPC only; TCP and UDP transport events must use transport metrics rather than `flag="none"`. | Covered by current counters. |
| xDS reconnect rate / rejection rate | `aether_gateway_dataplane_xds_connect_failures_total`, `aether_gateway_dataplane_xds_stream_failures_total`, `aether_gateway_controlplane_xds_stream_terminations_total`, and `aether_gateway_controlplane_xds_status_report_rejections_total`. | Controlplane `/v1/nodes`, dataplane `/v1/node`; Grafana controlplane and dataplane xDS panels. | Termination and rejection `reason` values must stay bounded enums with unknown values normalized to `other`. | Covered by current counters. |
| RSS / FD / thread slope | Standard process metrics `process_cpu_seconds_total`, `process_resident_memory_bytes`, `process_open_fds`, `process_max_fds`, `process_threads`, Go `go_threads`, and dataplane runtime resource gauges. | Release and soak reports; Grafana `Dataplane Process Resources`. | Resource metrics must not include request, route, backend, or error-detail labels. | Covered for controlplane through Go/process collectors and for dataplane through the Rust admin `/metrics` process sampler. |
| container CPU / memory pressure | Recording rules `aether_gateway_dataplane_container_cpu_cores`, `aether_gateway_dataplane_container_cpu_request_cores`, `aether_gateway_dataplane_container_cpu_throttle_ratio`, `aether_gateway_dataplane_container_memory_working_set_bytes`, `aether_gateway_dataplane_container_memory_limit_bytes`, and `aether_gateway_dataplane_container_memory_request_bytes` derive container resource pressure from kubelet cAdvisor and kube-state-metrics. Grafana resource panels should prefer these recording rules and include raw cAdvisor / kube-state-metrics fallback expressions so dashboards still render before the optional recording rules are installed. | Grafana `Dataplane Container CPU / Throttle` and `Dataplane Container Memory`. | Recording rule and fallback labels are `namespace`, `pod`, and `container`; do not add route, backend, request, or error labels. | Covered when Prometheus scrapes kubelet cAdvisor metrics and kube-state-metrics. |

Replica-wide Grafana ratio panels must derive ratios from `rate()` over counters after aggregation. Do not use `avg()` over dataplane-local ratio gauges such as `aether_gateway_dataplane_traffic_retry_rate`, `aether_gateway_dataplane_traffic_failover_success_rate`, `aether_gateway_dataplane_traffic_upstream_pool_hit_ratio`, `aether_gateway_dataplane_traffic_upstream_connect_latency_ms_average`, or `aether_gateway_dataplane_container_cpu_throttle_ratio`; those gauges are useful per target after their denominator has at least one observation, but they are not weighted correctly across replicas. For fleet-wide CPU throttling, use `max(aether_gateway_dataplane_container_cpu_throttle_ratio)` to expose the hottest pod or derive a weighted ratio from `container_cpu_cfs_throttled_periods_total` and `container_cpu_cfs_periods_total` after aggregation.

Replica-wide retry and failover panels must derive HTTP/gRPC retry ratios from request-only counters. TCP and UDP transport observations must not contribute synthetic retries to `aether_gateway_dataplane_traffic_retried_events_total`, `aether_gateway_dataplane_traffic_retry_attempts_total`, or `aether_gateway_dataplane_traffic_retried_success_events_total`; otherwise retry rate uses a request denominator with a transport-inflated numerator.

Grafana panels labeled as HTTP request rate or HTTP request volume must use `aether_gateway_dataplane_traffic_request_events_total`. `aether_gateway_dataplane_traffic_events_total` is the all-traffic event counter for HTTP/gRPC requests, TCP sessions, TLS passthrough sessions, and UDP datagrams; dashboards may show it only when the panel title, description, and legend make the all-traffic scope explicit.

Replica-wide upstream pool and connect-latency panels must derive HTTP/gRPC upstream acquisition ratios from request-only counters. TCP, TLS passthrough, and UDP transport observations must not contribute synthetic pool hits, misses, peer-build failures, or connect-latency buckets to `aether_gateway_dataplane_traffic_upstream_pool_hits_total`, `aether_gateway_dataplane_traffic_upstream_pool_misses_total`, `aether_gateway_dataplane_traffic_upstream_peer_build_failures_total`, or `aether_gateway_dataplane_traffic_upstream_connect_latency_ms_*`; otherwise pool hit ratio and connect latency mix unrelated transport semantics.

Replica-wide protection panels that show total rejection rates for scoped counter families must query `scope="total"`, not `scope!="total"`. Families such as `aether_gateway_dataplane_http_overload_rejected_total`, `aether_gateway_dataplane_tcp_overload_rejected_total`, `aether_gateway_dataplane_udp_overload_rejected_total`, `aether_gateway_dataplane_http_circuit_breaker_rejected_total`, and `aether_gateway_dataplane_http_rate_limit_rejected_total` publish an authoritative total sample plus child scope samples; summing child scopes can double-count if scope semantics expand or overlap.

Replica-wide Grafana inventory panels for snapshot-derived dataplane gauges must use `max()`, not `sum()`. Metrics such as `aether_gateway_dataplane_listener_count`, route counts, backend counts, secret counts, and session persistence policy counts are emitted once per dataplane target for the same applied snapshot; summing them multiplies configured resource counts by the number of scraped replicas.

Replica-wide Grafana listener-state panels for `aether_gateway_dataplane_listener_*_count` gauges must use `max()`, not `sum()`. These gauges count current listener status, attention, recovery, convergence, and serving-state classifications inside each dataplane target over the same listener set; summing them multiplies the same configured listeners by replica count. Node-count questions should use node-level gauges such as runtime rejection booleans or `aether_gateway_dataplane_node_info` labels instead.

Replica-wide Grafana runtime current failure count panels for `aether_gateway_dataplane_runtime_{http,stream,tls}_current_failure_count` must use `max()`, not `sum()`. These gauges count currently failing listeners for the active snapshot inside each dataplane target; summing them multiplies listener failures by dataplane replica count. Use `sum()` only for node-level runtime rejection booleans such as `aether_gateway_dataplane_runtime_http_current_rejected` when the panel intentionally counts affected nodes.

Replica-wide freshness panels that render "seconds since last apply" from `aether_gateway_dataplane_xds_last_apply_timestamp_seconds` must use `time() - min(... > 0)`. Since age is inverted from the timestamp, `time() - max(...)` shows the freshest dataplane node and hides a stale replica, and a raw zero timestamp means the replica has not applied any snapshot yet rather than "Unix epoch freshness."

## Metric Addition Checklist

Before adding a metric to the default surface:

- Define the metric name, type, unit, and owner.
- List every label and classify it as bounded or controlled.
- State the expected upper bound for each controlled label.
- State whether the metric is high-frequency request-path, low-frequency inventory, or debug-only.
- Add or update Grafana, Prometheus rules, admin API mapping, or docs that consume the metric.
- Run `scripts/check-metrics-cardinality-contract.sh`.

Default request-path histograms must prefer aggregate SLO usefulness over high-cardinality drill-downs. If route or backend labels are needed, add a separate low-cardinality `topk` Grafana query or an opt-in debug metric rather than changing the default histogram.

## Current Gaps

The current observability surface is usable for health, readiness, error rate, xDS freshness, ACK lag, request-like HTTP/gRPC latency histograms, upstream connect latency histograms, and xDS apply stage histograms. Percentile queries must use Prometheus histogram windows, for example `histogram_quantile(0.99, sum by (le) (rate(<metric>_bucket[5m])))`; direct cumulative buckets are not a time-windowed p99.

Recommended follow-up work:

- Consider adding seconds-named aliases for the current millisecond histogram families if Prometheus naming convention alignment becomes a release requirement.
- Extend downstream latency histograms to TCP, UDP, and long-lived streaming surfaces where request-style completion latency is meaningful.
- Add bounded runtime reload duration histograms.
- Add node or CNI network drop/error recording rules once the supported production scrape source is documented.
- Keep route, backend, raw host, path, request_id, and raw error detail out of default high-QPS histograms.
