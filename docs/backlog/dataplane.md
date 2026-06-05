# Dataplane Backlog

This document tracks data plane forwarding efficiency, resource usage, p99/p999, forwarding success rate, hot reload, and module maintainability work.

## P1: Real Forwarding Performance Baseline

Goal: Establish real data plane forwarding performance, p99, and success rate baselines, not relying solely on reload / selection microbenchmarks.

Scope:

- HTTP/1.1
- H2C gRPC
- WebSocket
- SSE / MCP streamable HTTP
- TCPRoute
- UDPRoute

Scenarios:

- steady
- burst
- ceiling
- long-lived streaming
- backend slow-read / slow-write
- backend error
- endpoint flapping
- reload-under-load

Report must include:

- RPS
- success rate
- p50 / p90 / p95 / p99 / p999 / max
- status class
- response flag
- retry attempts
- upstream pool hit/miss
- upstream connect latency
- CPU / RSS / FD / threads

Acceptance:

- Add or extend `scripts/run-dataplane-throughput-baseline.sh`.
- Output to `reports/performance/runs/<run-id>-dataplane-throughput/`.
- Report distinguishes between "gateway forwarding bottleneck", "runtime snapshot read / string clone", "upstream slow response", "connection pool reuse insufficient", "reload jitter", "observability write overhead".

Current progress:

- `2026-05-06` Added `scripts/run-dataplane-throughput-baseline.sh`, supporting generation of standardized throughput reports from existing kind / staging / production sampled evidence directories, also usable with `RUN_KIND_A4=true` to reuse the existing kind A4 entry point to collect HTTP / gRPC / TCPRoute / UDPRoute evidence.
- Report now consistently outputs `RPS`, success rate, `p50/p90/p95/p99/p999/max`, status class, response flag, retry attempts, upstream pool hit/miss, pool hit ratio, upstream connect latency `p95/p99` and CPU/RSS/FD/threads.
- Report now has a protocol coverage matrix; machine-readable report and summary both list required / observed / missing protocols, preventing HTTP/gRPC evidence from being misinterpreted as WebSocket, SSE/MCP, TCPRoute, UDPRoute already covered.
- Report now has a scenario coverage matrix; machine-readable report and summary both list required / observed / missing status for steady, burst, ceiling, long-lived-streaming, backend-slow-read/write, backend-error, endpoint-flapping, reload-under-load; scenarios can come from the `scenario` / `scenarios` field of evidence JSON or profile filenames.
- Report now has a scenario summary table, aggregating profile count, protocol set, requests, success rate, max p99 / p999, and max RPS per scenario, making it easy to compare tail latency and success rate across slow upstream, backend error, endpoint flapping, and reload-under-load.
- Report now has `bottleneck_classification`, categorizing gateway forwarding, upstream slow response, connection pool reuse, reload jitter, observability overhead, fault injection as indicated / evidence per evidence item; summary outputs a classification table synchronously.
- Report now has a reload / xDS apply section that aggregates observations, average, p95/p99 bucket upper bound, and sum by `stage` from the `nantian_gateway_dataplane_xds_apply_stage_duration_ms` histogram in `metrics.prom`, for locating apply-stage jitter in reload-under-load evidence.
- Report reload summary now includes data plane xDS `snapshots_applied/nacked/skipped`, stream/connect failure counters, and last apply timestamp, making it easier to distinguish reload jitter, NACK, and control plane connection anomalies.
- Report now has a `fault_isolation` section that aggregates from `metrics.prom` gateway-side fast-fail, circuit breaker open, rate limit reject, retry budget exhausted, passive ejection, active unhealthy, recovery latency, and last-good snapshot / current snapshot rejected status.
- Data plane `/metrics` now exposes HTTP retry budget enabled / ratio / burst / available tokens / retryable observed / allowed / rejected counters; throughput reports can directly consume `retry_budget_exhausted_total`.
- Data plane `/metrics` now exposes endpoint runtime tracked / passive ejected current / active unhealthy current gauges; throughput reports can directly consume passive ejection and active unhealthy current values.
- Data plane `/metrics` now exposes endpoint recovery latency histogram `nantian_gateway_dataplane_endpoint_recovery_latency_ms`, and inherits cumulative observations across snapshot runtime state; throughput reports can directly consume recovery latency p95 / p99.
- Report now has a UDPRoute evidence section that aggregates datagrams/packets sent, received, lost, packet loss rate, max p99 from UDP profiles, and aggregates UDP active sessions, queue depth, queue overflow drops, idle evictions, and listener-level session churn from `metrics.prom`.
- `2026-05-16` Extended Kind A4 source evidence to TCPRoute steady, UDPRoute multi-client, UDPRoute high-churn, UDPRoute multi-upstream, and UDPRoute backend-timeout profiles; commit `c3d7b207` run `reports/performance/runs/2026-05-16-190408-c3d7b207-udp-a4-scenarios/` passed source A4 SLO gate, TCPRoute steady `p99=9.68ms`, UDPRoute multi-client / high-churn / multi-upstream `p99=13.79ms` / `16.87ms` / `13.44ms` with packet loss `0`; backend-timeout profile is expected timeout, `64/64` datagrams reached blackhole backend, `p99=203.86ms`.
- `2026-05-16` Extended Kind A4 source evidence further to WebSocket, SSE, and MCP streamable HTTP long-lived connection profiles; commit `279728e3` run `reports/performance/runs/2026-05-16-192109-279728e3-long-lived-a4-profiles/` passed source A4 SLO gate, protocol coverage covers `http`, `grpc`, `websocket`, `sse`, `mcp`, `tcp`, `udp` with `missing_protocols=[]`. This round WebSocket / SSE / MCP long-lived profile `p99=3043.92ms` / `1504.57ms` / `1505.15ms`, all success rates `100%`.
- `2026-05-16` Extended Kind A4 source evidence to real live-traffic `reload-under-load` profiles; commit `c9585cbb` run `reports/performance/runs/2026-05-16-194710-c9585cbb-reload-under-load-a4-profiles/` passed source A4 SLO gate, HTTP/gRPC/TCP/UDP live reload total `4800/4800` successful, all six mutation types (route-only / backend-only / endpoint-only / secret-only / TLS asset rotation / listener add-remove) have evidence, `reload.live_traffic.missing_protocols=[]` and `missing_mutations=[]`, max `p99=12.83ms`.
- `2026-05-16` Extended Kind A4 source evidence to `backend-error`, `backend-slow-read`, `backend-slow-write`, and `endpoint-flapping` scenarios; commit `62145cc9` run `reports/performance/runs/2026-05-16-201743-62145cc9-fault-scenarios-a4-profiles/` passed source A4 SLO gate, four profile types respectively `400/400`, `400/400`, `400/400`, `8000/8000` successful, `p99=45.41ms` / `426.30ms` / `127.01ms` / `98.36ms`, `coverage.missing_scenarios=[]`.
- `2026-05-16` `scripts/run-dataplane-throughput-baseline.sh` now supports `CHAOS_INPUT_DIR` / `SOAK_INPUT_DIR`, can aggregate fault injection `traffic/summary.json`, `conclusions/summary.json` and soak `traffic/resources/observability` summary into the same throughput report; `reports/performance/runs/2026-05-16-204457-46452c26-chaos-soak-throughput-report/` shows Kind A4 coverage complete, chaos release gate `pass`, soak SLO `pass`, but soak is still `3600s` pilot, `is_24h=false`.
- Current Kind A4 covers HTTP/1.1, H2C gRPC, WebSocket, SSE/MCP, TCPRoute, UDPRoute main profiles, backend slow/error, endpoint flapping, and `reload-under-load` live traffic; fault injection / 1h soak can be aggregated at the same scope, remaining work is to rerun in non-Kind production-like environment and complete a real `24h` soak.

## P1: HTTP / gRPC Hot Path Allocation

Goal: Optimize HTTP / gRPC hot path allocation and temporary objects, prioritizing p99 jitter reduction under high concurrency.

Status: Completed per current P1 acceptance; runtime handle ID-ification continues as a separate long-term evaluation item.

Background:

- `dataplane/crates/aeg-http/src/proxy/request.rs` constructs owned header maps per request, and lowercases / clones header name/value.
- Selection microbenchmarks are currently low; candidate `Vec` optimizations must wait for full proxy flamegraph evidence showing they enter the hot path.

Acceptance:

- Introduce `RequestView` or equivalent lazy accessor.
- Normal requests only directly read `Host`, path, method, `content-length`, request id, trace headers.
- Only materialize header / query map when route matcher, CORS, response filter, mirror, access log, or tracing actually needs it.
- header-heavy benchmark compares alloc, RSS delta, CPU, and p99.
- Preserve Gateway API header/query match semantics unchanged.

Current progress:

- `2026-05-06` Added `request_meta_header_heavy` as a formal `aeg-bench` microbenchmark scenario, pinning the baseline for constructing owned `RequestMeta` with heavy header/query requests; benchmark report now includes `p99_ms`, RSS delta, user/system CPU tick delta, and allocations/deallocations/reallocations with allocated/deallocated/reallocated byte delta under system allocator profile. Scenario details output header value count/bytes and query param count. Real full proxy throughput/p99 evidence is still being completed in the throughput baseline TODO.
- `2026-05-06` Moved header/query name normalization for HTTP / gRPC matchers forward to the runtime index rebuild phase; compiled snapshot match paths use borrowed lowercase key lookup, avoiding per-request allocation of lowercase strings for matcher names.
- `2026-05-06` Made `SelectedBackendConfig` only store precomputed fields actually needed for upstream peer construction; config build phase borrows from `BackendPolicy` / protocol instead of cloning the full `BackendPolicy` into per-request cache; Debug output fixed to precomputed fields like timeout / TLS / client-cert.
- `2026-05-06` Removed the independent owned vector for `RequestContext.filters`; upstream request / response filter stage directly reuses cached `SelectedBackend.filters`; CORS response headers now determined by passing filters at call site, avoiding cloning filters into request context after backend selection.
- `2026-05-06` Removed the independent policy clone for `RequestContext.session_persistence`; response session cookie writing directly reuses cached `SelectedBackend.session_persistence`; request context only retains the resolved `ResolvedSession`.
- `2026-05-06` Removed the independent policy clone for `RequestContext.retry_policy`; retry code / attempt limit determination directly reuses cached `SelectedBackend.retry`; request context only retains retry attempt count and next retry backoff.
- `2026-05-06` Removed the second clone of route annotations on the selected backend access log path; `RequestContext` no longer copies `SelectedBackend.route_annotations`, log rendering directly borrows annotations from the selected backend, paths without a selected backend (direct response / redirect) still save route annotations via context.
- `2026-05-07` Introduced `RequestView` in the HTTP proxy request path; the front path directly reads host, path, method, content-length, request id, trace headers, and header bytes from `RequestHeader`; subrequest, header/body limit, listener admission / rate-limit no longer construct owned `RequestMeta` / header map beforehand.
- `2026-05-07` Added `request_view_header_heavy` as a formal `aeg-bench` microbenchmark scenario using the same header-heavy fixture as `request_meta_header_heavy`; the new scenario only times view-only context capture, trace header extraction, and header byte statistics, for comparing lazy view vs owned materialization p99, RSS, CPU tick, and allocator delta in the same `bench.json`.
- `2026-05-07` `aeg-bench` report now has a `request_view_vs_meta_header_heavy` comparison, directly outputting timing delta, p99 reduction ratio, and RSS/FD/thread/CPU/allocator delta of lazy view relative to owned materialization.
- `2026-05-07` HTTP/gRPC listener candidate collection changed to single-pass best-host-score scan, no longer constructs a candidate listener Vec and matched listener Vec separately; wildcard hostname suffix helper changed to iterator. With the same default `aeg-bench --iterations 1`, `http_route_selection` allocations `115 -> 110`, `grpc_route_selection` allocations `104 -> 102`, while preserving HTTPRoute listener/hostname and mesh service frontend integration tests semantic coverage.
- `2026-05-07` This P1 sub-item is now closed per current acceptance: lazy `RequestView`, on-demand materialization, header-heavy benchmark / comparison, selected backend clone consolidation, and listener candidate Vec optimization are all implemented; the larger runtime handle ID-ification remains as a separate long-term item for continued evaluation.

## P1: Upstream Peer And Connection Reuse

Goal: Optimize upstream peer construction, connection reuse, and connect latency observability, reducing forwarding p99.

To do:

- Add Prometheus counter / histogram for upstream pool hit/miss, connect latency, TLS handshake failure, peer build failure.
- Throughput report outputs pool hit ratio, connect latency p95/p99, success rate after retry.
- If caching peer templates, must add isolation tests ensuring different routes' protocol hint, timeout, TLS validation, client cert do not leak across configurations.

Current progress:

- `2026-05-06` Throughput report now outputs pool hit ratio, connect latency `p95/p99` bucket upper bound, and success rate after retry, establishing the reporting baseline that must be referenced before further optimizing keepalive / HTTP/2 upstream max streams.
- `2026-05-06` Per-request `SelectedBackendConfig` now precomputes IP/host address form, port, TLS / HTTP/2 upstream flags derived from backend protocol, SNI, connect timeout, and backend request timeout; `HttpPeer` construction path reuses these fields, without caching `HttpPeer` or peer template yet, to avoid prematurely changing connection pool isolation semantics.
- `2026-05-06` `SelectedBackendConfig` now continues to pre-resolve BackendTLSPolicy validation cache and backend client cert key; route selection pins TLS validation group key / CA / SAN hook and client cert handle; `HttpPeer` construction path no longer re-reads snapshot for these resolutions.

## P1: MCP / SSE / Streaming HTTP

Goal: Establish long-lived connection specific tests for MCP / SSE / streaming HTTP, preventing default timeouts and retry from incorrectly affecting streaming forwarding.

To cover:

- MCP streamable HTTP
- SSE
- chunked response
- Long connection lifetime with no body data
- Client cancel
- Backend cancel

Acceptance:

- Default configuration must not use short-request upstream read timeout to incorrectly kill legitimate long-lived connections.
- Long-lived connection scenarios must not incorrectly retry non-idempotent requests.
- Normal client cancel must not be recorded as upstream failure.
- Upstream close after response is already written must not retry; access log retains real downstream status, and no longer outputs Rust proxy `Fail to proxy` ERROR noise.
- Access log / metrics must distinguish `UT`, client cancel, idle timeout, max connection age, normal long-lived connection end; currently covers upstream timeout `UT`, client cancel `DC`, upstream close `UC`, HTTP downstream read timeout `IT`.
- `2026-05-16` Kind A4 has archived WebSocket, SSE, and MCP streamable HTTP as archivable profile JSON, and included them in `long-lived-streaming` scenario coverage; all three long-lived SLO gates in `reports/performance/runs/2026-05-16-201743-62145cc9-fault-scenarios-a4-profiles/` are `pass`, with p99 `3052.43ms` / `1505.75ms` / `1504.38ms` respectively.

## P1: Fault Isolation And Health Evidence

Goal: Strengthen success rate verification for fault isolation, active/passive health checking, and retry budget, proving that success rate improvements do not come at the cost of p99 jitter or incorrect retries.

Current progress:

- `2026-05-06` Throughput report now has `fault_isolation` JSON and Markdown summary, pinning the reporting scope for fast-fail, circuit open, retry budget exhausted, passive ejection, active unhealthy, recovery latency, last-good snapshot / current rejected.
- `2026-05-06` Data plane `/metrics` now formally exposes HTTP retry budget status and allowed/rejected counters; admin, HTTP runtime, shared TLS HTTP runtime use the same reloadable retry budget controller.
- `2026-05-06` Data plane `/metrics` now formally exposes endpoint runtime current status, including tracked endpoints, passive ejected current, and active unhealthy current.
- `2026-05-06` Data plane `/metrics` now formally exposes endpoint recovery latency histogram, preserving cumulative observations across snapshot runtime state inheritance.
- `2026-05-06` HTTP retry determination converged to a dual condition of "config allows + request is automatically replayable": `GET/HEAD/OPTIONS/TRACE/PUT/DELETE` can retry when retry buffer is not truncated; `POST/PATCH` or unknown methods no longer auto-retry; regression cases cover `POST` body with configured retry status preserving original upstream `503` without accessing next backend.

To do:

- Cover endpoint flapping, partial endpoint slow response, all endpoints transient failure, recovery traffic return, active probe jitter.
- Verify that under last-good snapshot, during bad snapshot publication, old connections remain, new requests continue forwarding, readyz / metrics / ACK-NACK status remains consistent.

## P1: UDP Runtime

Goal: Optimize UDP data path, reducing per-datagram spawn / copy / lock contention resource usage and p99 jitter.

To do:

- Fixed worker dispatcher already connected: UDP listener receive loop dispatches by `listener + client + upstream` key to a fixed number of workers, no longer creating a dispatch task per datagram.
- Session registry changed to sharded map.
- Session count, queue depth, queue overflow, drop, idle eviction metrics already connected.
- Response buffer reused per upstream session.
- Throughput report already has UDPRoute packet-loss / p99 / session churn artifact contract, outputting packet loss, max p99, active sessions, queue depth, queue overflow drops, and idle evictions.
- UDPRoute throughput / packet-loss / p99 / session churn report covers multi-client, multi-upstream, backend timeout, and high-churn.
- `2026-05-16` Kind A4 archived real multi-client, high-churn, multi-upstream, and backend-timeout UDPRoute profiles: normal UDP profiles total `1500/1500` datagrams successful, packet loss `0`, max p99 `16.87ms`, queue overflow drops `0`; backend-timeout profile uses a UDP blackhole backend that receives but does not respond, confirming `64/64` datagrams reached the backend and timed out as expected, p99 `203.86ms`. This round report `udp.coverage.missing_scenarios=[]`.
- `2026-05-16` Latest reload-under-load A4 rerun maintains UDPRoute coverage complete: multi-client / high-churn / multi-upstream `p99=8.51ms` / `11.54ms` / `8.77ms`, normal UDP profiles packet loss `0`, backend-timeout `p99=203.46ms`, `udp.coverage.missing_scenarios=[]`; additionally added UDP live reload secret-only profile `400/400` successful, `p99=12.83ms`.
- `2026-05-16` Latest fault-scenario A4 rerun maintains UDPRoute coverage complete: multi-client / high-churn / multi-upstream `p99=28.13ms` / `14.01ms` / `6.92ms`, normal UDP profiles packet loss `0`, backend-timeout `p99=204.39ms`, `udp.coverage.missing_scenarios=[]`.

## P1: TCP / TLS Passthrough Runtime

Goal: Optimize TCP / TLS passthrough stream resource model, reducing RSS and tail latency under high-concurrency long-lived connections.

Current progress:

- `2026-05-06` Added TLS passthrough preface slow-client timeout regression; `read_preface()` returns a clear `timed out reading client preface` with a fixed timeout when client connects but does not send preface, preventing infinite wait in the SNI parsing path.
- `2026-05-06` Added TCP/TLS passthrough backend connect failure regression; `handle_connection()` records a `UF` traffic event with connect latency when upstream `TcpStream::connect()` fails, preventing connect failures from only having task logs without observability.
- `2026-05-13` Added TCP client reset classification regression; downstream reset / broken pipe classified as `DC`, upstream reset / broken pipe classified as `UC`, common TCP connection termination no longer bubbles up as `stream tcp connection failed` WARN.
- `2026-05-16` Kind A4 archived TCPRoute steady short-lived connection request/response profile: `1000/1000` successful, concurrency `64`, `p99=12.46ms`, `p999=14.10ms`, `RPS=4784.69`. This validates the current short-lived TCPRoute forwarding path, but is not yet a high-concurrency long-lived connection RSS / FD / thread resource curve.
- `2026-05-16` Latest reload-under-load A4 rerun TCPRoute steady profile: `1000/1000` successful, concurrency `64`, `p99=12.81ms`, `p999=16.21ms`, `RPS=5780.35`; TCP live reload endpoint-only and listener add/remove profiles both `800/800` successful, `p99=7.94ms` / `10.12ms`. Conclusion remains limited to short-lived connection request/response path, does not replace high-concurrency long-lived connection resource curve.
- `2026-05-16` Latest fault-scenario A4 rerun TCPRoute steady profile: `1000/1000` successful, concurrency `64`, `p99=6.79ms`, `p999=7.91ms`, `RPS=5617.97`; TCP live reload endpoint-only and listener add/remove profiles both `800/800` successful, `p99=10.05ms` / `11.47ms`. Conclusion remains limited to short-lived connection request/response path, does not replace high-concurrency long-lived connection resource curve.

Acceptance:

- Establish a curve report of concurrent connections to RSS / FD / threads / p99 / throughput.
- Evaluate connection-level buffer pool or per-listener profile default buffer size adjustment.
- Cover slow client, half-close, idle timeout, max connection age, SNI parse timeout, backend connect failure, client reset, and upstream reset classification.
- Verify that during TCPRoute / TLSRoute passthrough reload, existing connections are not incorrectly killed, and new connections correctly select backend per last-good or current snapshot.

## P1: Reload Under Live Traffic

Goal: Establish phased metrics and live-traffic reload benchmark for hot reload, reducing reload time and p99 jitter during reload.

Current progress:

- `2026-05-06` Added low-cardinality duration histogram for core xDS apply stages, recording duration for `decode`, `inherit_runtime_state`, `snapshot_swap`, `listener_apply`, `ack_wait` stages, and outputting `nantian_gateway_dataplane_xds_apply_stage_duration_ms{stage=...}` bucket/sum/count at `/metrics`.
- `2026-05-06` xDS apply path separated proto decode and `rebuild_runtime_indexes()` into independent stages: `aeg-ir` added an unindexed decode entry point, data plane explicitly records `rebuild_indexes` duration after inheriting runtime state; finer `listener_plan`, `tls_assets` split still needs to be supplemented from the runtime layer.
- `2026-05-06` `scripts/run-dataplane-throughput-baseline.sh` can now incorporate xDS apply stage histogram from `metrics.prom` into `throughput-report.json` and `summary.md`, allowing reload-under-load reports to directly display apply stage series, observations, average, and p95/p99 bucket upper bound.
- `2026-05-06` The same throughput report also outputs data plane xDS applied/NACK/skipped, stream/connect failure counters, and last apply timestamp; when NACK or xDS connection anomaly occurs, bottleneck notes prompt to first correlate control plane availability and listener apply failure.
- `2026-05-07` Throughput report added `reload.live_traffic` coverage contract, identifying live reload scenarios from `reload_mutation` / `snapshot_mutation` in profile JSON, outputting HTTP, gRPC, TCP, UDP protocol coverage, and route-only, backend-only, endpoint-only, secret-only, TLS asset rotation, listener add/remove snapshot mutation coverage and mutation-level p99 / ACK / NACK / last-good fallback summary.
- `2026-05-07` `aeg-bench` added `runtime_index_rebuild_route_only`, `runtime_index_rebuild_endpoint_only`, `runtime_index_rebuild_secret_only` three full-rebuild baseline scenarios. With default `aeg-bench --iterations 1`, local samples show all three mutation types' full rebuild p99 approximately `331-333ms`, allocations approximately `80k-81k`, providing a local baseline for future assessment of whether incremental indexing is worth designing.

Subsequent trigger conditions:

- `2026-05-16` Real live-traffic reload benchmark has entered Kind A4 source evidence: in the latest `reports/performance/runs/2026-05-16-201743-62145cc9-fault-scenarios-a4-profiles/`, HTTP/gRPC/TCP/UDP reload profiles total `4800/4800` successful, required protocols / mutations have no gaps, source SLO gate is `pass`, max `p99=11.47ms`. Future release candidates still need to rerun on the same candidate commit per release evidence process and archive the report.
- Currently only maintain full-rebuild baseline and `rebuild_indexes` stage metrics for route-only, endpoint-only, secret-only; only evaluate production incremental index or general diff engine when throughput / p999 report explicitly flags `rebuild_runtime_indexes()` as bottleneck.

## P2: Observability Cost

Goal: Optimize metrics / traffic graph / access log overhead under high QPS, preventing the observability system from inversely increasing p99.

To do:

- Request latency histogram uses only low-cardinality labels by default. `/metrics` already outputs `nantian_gateway_dataplane_traffic_request_latency_ms` with labels limited to `listener`, `protocol`, `route_kind`, `status_class`, `response_flag`.
- Route/backend level high-cardinality metrics are fixed in `/v1/traffic` admin summary; `nantian_gateway_dataplane_traffic_*` metrics at `/metrics` do not output route/backend/pod/endpoint level labels.
- Access log writer adds queue depth, drop count, flush latency, sink error metrics, and supplements slow sink stress testing. Currently covers caller non-blocking when slow sink stalls, and cumulative drop semantics after queue full.
- Traffic graph evaluates downsampling or async aggregation. Current throughput report outputs traffic graph node/edge count, route/backend/endpoint topology existence, and Prometheus traffic label cardinality check, making it easy to prove necessary troubleshooting information remains visible before further optimization.

## P2: Long-Term Runtime Model

Candidate directions:

- Lock-free snapshot read, e.g., `ArcSwap<Snapshot>` or equivalent model.
- Introduce stable runtime ID for route / rule / backend / endpoint / listener.
- Separate endpoint runtime mutable state from the snapshot body.
- Only advance to formal implementation when throughput / p999 report proves RwLock or string clone is the bottleneck; current throughput report explicitly outputs `runtime_snapshot_read_or_string_clone` classification, keeping `indicated=false` when no direct evidence is captured.

Current progress:

- `2026-05-07` Added `snapshot_read_rwlock` / `snapshot_read_arc_swap` prototype benchmark and `arc_swap_vs_rwlock_snapshot_read` comparison report in `aeg-bench`; production shared snapshot path remains `Arc<RwLock<Snapshot>>`.
- `2026-05-07` `EndpointRuntimeStore` now carries endpoint success, failure, and active probe state via interior mutable handle/store; `Snapshot::record_endpoint_failure/success/active_probe_*` now only requires `&self`, new regression confirms runtime recording does not require writing to the snapshot body.
- `2026-05-07` `aeg-ir` added stable `RuntimeId` / `RuntimeIdIndex`; `rebuild_runtime_indexes()` generates order-independent deterministic runtime IDs for listener, HTTP/gRPC/stream route, rule, backend, endpoint, and provides snapshot accessors; regression confirms resource order changes in snapshot do not change the same resource ID.
- `2026-05-07` HTTP proxy's `SelectedBackendConfig` / `RequestContext` now caches listener / route / backend / endpoint runtime IDs for the selected path.
- `2026-05-07` Route selection result now carries rule index; the rule runtime ID for selected backend of HTTPRoute / GRPCRoute / TCPRoute / UDPRoute / TLSRoute is precomputed with `SelectedBackendConfig`; non-rule paths like mesh/default fallback remain `None`.
- `2026-05-07` HTTP access log now outputs `listenerRuntimeId` / `routeRuntimeId` / `ruleRuntimeId` / `backendRuntimeId` / `endpointRuntimeId` while preserving original string fields; text mode simultaneously supports `%*_RUNTIME_ID%` placeholders; paths without IDs do not output corresponding JSON fields.
- `2026-05-07` TCP / TLS passthrough / UDP stream access log outputs the same set of runtime ID fields; stream proxy path precomputes IDs when backend is selected; log layer retains original string fields for compatibility.
- `2026-05-07` Dataplane admin's listener / route / backend detail views now output `runtimeId`; route detail additionally outputs `ruleRuntimeIds`; backend detail endpoints also include `runtimeId`, enabling direct cross-reference with runtime IDs in logs.
- `2026-05-07` Dataplane admin's listener / route / backend list views reuse the detail's runtime ID display semantics; list responses also carry `runtimeId` / `ruleRuntimeIds` / endpoint `runtimeId`, facilitating batch log ID investigation.
- `2026-05-07` Dataplane admin's `/v1/listener-statuses` list and detail responses output listener `runtimeId`; listener runtime state, reload failure, and access log can be directly correlated by the same ID.
- `2026-05-07` Dataplane admin's `/v1/summary` listener signal fields now output parallel `*RuntimeIds` alongside existing `*Names`; pending / rejected / stale / recovery / attention category summaries can directly jump to listener status and log ID.
- `2026-05-07` Dataplane admin's `/v1/traffic` topology nodes output `runtimeId` on listener / route / backend nodes; HTTP / TCP / UDP hot paths only pass numeric IDs, which are rendered as hex strings at the JSON boundary.
- `2026-05-07` `aeg-ir` runtime index now provides the ability to resolve runtime IDs back to resource references, providing a unified entry point for "resolve strings at display time" for future admin / log usage.
- `2026-05-07` Dataplane admin's `/v1/listeners`, `/v1/routes`, `/v1/backends` now all support runtime ID filtering; routes additionally support `ruleRuntimeId`; backends additionally support `endpointRuntimeId`, enabling direct navigation from log ID back to resources.
- `2026-05-09` `scripts/run-dataplane-throughput-baseline.sh` added `runtime_snapshot_read_or_string_clone` bottleneck classification; current reports explicitly state there is no direct RwLock or string clone p999 evidence; production lock-free snapshot read and deeper ID-first migration continue to be evidence-triggered.
- `2026-05-09` Dataplane admin's listener / route / backend list and detail views now append `runtimeRef` alongside `runtimeId`; route rules append `ruleRuntimeRefs`; backend endpoints append endpoint `runtimeRef`; these display fields are resolved from `RuntimeIdIndex`, reducing admin view dependency on string-first fields, and allowing access log / traffic graph / admin views to correlate by ID first and display resource references later.
- `2026-05-09` Dataplane admin's `/v1/traffic` topology nodes append `runtimeRef` from current snapshot's `RuntimeIdIndex` when `runtimeId` exists; traffic graph hot paths still only pass numeric IDs, which are resolved to structured resource references at the JSON admin boundary.
- `2026-05-09` Access log deterministic sampling now preferentially uses listener / route / backend runtime ID as sampling key; fallback paths without IDs continue using old display strings, preventing sampling decisions from changing due to resource display name changes when runtime ID already exists.
- `2026-05-12` HTTP proxy selected backend cache consolidated to ID-first: `cache_selected_backend_state()` / fast path no longer copies listener / route / backend display strings into `RequestContext` when both access log and tracing are disabled, only retaining runtime ID, `SelectedBackend` / compiled fast-path selected, and precomputed `TrafficTopology`; access log, sample key, and traffic graph read display fields from `SelectedBackend` or fast-path selected at output boundary. Verified with `cargo test --manifest-path dataplane/Cargo.toml -p aeg-http -- --nocapture`.

## P2: Module Split

Current targets:

- `dataplane/crates/aeg-shared-tls/src/runtime.rs` (converged to 555 lines, continue monitoring by responsibility)
- `dataplane/crates/aeg-http/src/proxy/backend.rs`
- `dataplane/crates/aeg-http/src/runtime/listener_plan.rs`

Current progress:

- `2026-05-07` Split `dataplane/crates/aeg-bench/src/scenarios.rs` into `scenarios/{route,request,snapshot,reload}.rs` with 20-line orchestrator, separating benchmark implementation by route selection / request path / snapshot model / reload apply scenarios, keeping all scenario names and report contract unchanged.
- `2026-05-02` Split response writing helpers from `dataplane/crates/aeg-http/src/proxy.rs` into `proxy/responses.rs`, moved selected backend runtime config assembly to `proxy/selection.rs`, and changed implicit parent module imports in `proxy/request.rs` to explicit imports; `proxy.rs` is now `795` lines.
- `2026-05-02` Split warning category/message aggregation from `dataplane/crates/aeg-app/src/admin/summary/overview_sections.rs` into `overview_sections/warnings.rs`, letting the main overview sections file focus on JSON overview assembly; `overview_sections.rs` is now `503` lines.
- `2026-05-02` Split test module from `dataplane/crates/aeg-shared-tls/src/runtime.rs` into `runtime/tests.rs`, separating shared TLS runtime implementation from handshake/listener regression assertions; `runtime.rs` is now `736` lines.
- `2026-05-02` Split TLS termination / dynamic certificate callback from `dataplane/crates/aeg-shared-tls/src/runtime.rs` into `runtime/handshake.rs`, letting shared TLS runtime main file focus on reload loop, bind lifecycle, and connection dispatch; `runtime.rs` is now `555` lines.
- `2026-05-02` Split shared TLS listener plan unit tests from `dataplane/crates/aeg-shared-tls/src/tests.rs` into `tests/listener_plan.rs`, letting crate-level test entry only keep shared fixture/helper and dispatch/runtime submodule mounts; `tests.rs` is now `273` lines.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_h2c/grpc_control/weighted.rs` into `weighted/{sequential,concurrent,support}.rs` with 3-line orchestrator, separating H2C gRPC weighted backend assertions by sequential stream, concurrent stream, and shared fixture.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/streaming.rs` into `streaming/{chunks,idle,cancel,timeout}.rs` with 4-line orchestrator, separating streaming HTTP/1 assertions by chunk helper, idle stream, client/backend cancel, and explicit backend timeout scenarios.
- `2026-05-02` Split retry / retry-budget scenarios from `dataplane/crates/aeg-http/src/runtime/tests_http1/retries_and_limits.rs` into `retries_and_limits/retry.rs`, letting the main file focus on rate-limit and circuit-breaker fast-fail scenarios; `retries_and_limits.rs` is now `188` lines.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_support_snapshots.rs` into `tests_support_snapshots/{tls,http_basic,http_multi,http_direct,grpc}.rs` with 5-line orchestrator, separating runtime snapshot test support code by TLS material, basic HTTP, multi-backend HTTP, direct response, gRPC H2C fixture.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_core/listener_signals.rs` into `listener_signals/{state,convergence,failure_recovery,attention,overviews}.rs` with orchestrator, separating summary signal assertions by listener state, convergence, failure recovery, attention, overview summary.
- `2026-05-02` Split `dataplane/crates/aeg-stream/src/udp/tests.rs` into `udp/tests/{proxy,budget,sessions,routing,support}.rs` with orchestrator, separating UDP runtime assertions by UDP proxy, datagram budget, session registry, routing error, shared fixture.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_h2c/grpc_control/weighted/sequential.rs` into `sequential/{gateway,mesh_single_connection,mesh_same_port,mesh_multiple_connections}.rs` with 4-line orchestrator, separating weighted sequential gRPC assertions by normal Gateway, mesh single connection, mesh same port, mesh multiple connections.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_h2c/grpc_control/weighted/concurrent.rs` into `concurrent/{gateway,mesh_single_connection,mesh_same_port}.rs` with 3-line orchestrator, separating weighted concurrent gRPC assertions by normal Gateway, mesh single connection, mesh same port.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_recovery_states/recovery_counts.rs` into `recovery_counts/{counts,failure_recovery,convergence,attention,overviews}.rs` with orchestrator, separating assertions by listener recovery counts, failure recovery, convergence, attention, overview/warnings.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/admin_views/metrics.rs` into `metrics/{surface,state_match,inventory,traffic,overload,protection}.rs` with orchestrator, separating assertions by `/metrics` surface, fixture flow, inventory, traffic, overload, protection metric consistency.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_current_states.rs` into `summary_current_states/{current,convergence,failure_recovery,attention,serving}.rs` with orchestrator, separating listener current state summary assertions by current status, convergence, failure recovery, attention, serving/warnings.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/http_route_selection/hostnames_and_listeners.rs` into `hostnames_and_listeners/{attachment_intersection,hostname_specificity,listener_ports,conformance_intersection}.rs` with orchestrator, separating HTTPRoute selection assertions by listener hostname attach, hostname specificity, request port, conformance regression.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_weighted/weighted_selection.rs` into `weighted_selection/{regex_indexes,http_round_robin,http_rule_filters,grpc_round_robin,grpc_large_weights}.rs` with orchestrator, separating weighted selection assertions by regex index, HTTP weighted, filter inheritance, gRPC weighted, large weight batch.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_listener_plan_updates.rs` into `tests_listener_plan_updates/{restart_diff,certificate_rotation,identity_order}.rs` with orchestrator, separating listener plan update assertions by listener restart diff, certificate/frontend-validation rotation, TLS identity order/wildcard.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_websocket.rs` into `tests_websocket/{upgrade,rejection,large_payload,close_propagation}.rs` with orchestrator, separating WebSocket runtime assertions by upgrade tunnel, backend rejection, large payload, client/backend close propagation.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/listener_runtime.rs` into `listener_runtime/{runtime_planes,recovery_history,current_serving_state}.rs` with orchestrator, separating listener runtime status assertions by runtime plane, recent recovery history, current/serving state.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/listener_status_endpoints_basic.rs` into `listener_status_endpoints_basic/{surface_detail,invalid_filters,serving_snapshot,serving_recovery}.rs` with orchestrator, separating listener status endpoint assertions by endpoint surface/detail, invalid filter, serving snapshot/version, serving/recovery state.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_runtime.rs` into `summary_runtime/{pending_runtime,current_failures}.rs` with orchestrator, separating summary runtime assertions by pending runtime summary, multi-runtime current failure/recovery/attention overview.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_core/transport_resources.rs` into `transport_resources/{resources_features,xds,runtime_reload,traffic,composed_overviews}.rs` with orchestrator, separating assertions by resources/features, xDS, runtime reload, traffic, composed overview/warnings.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_h2c/grpc_payloads.rs` into `grpc_payloads/{server_streaming,unary_request,backend_disconnect}.rs` with orchestrator, separating H2C gRPC payload assertions by server streaming, unary request body, backend disconnect error propagation.
- `2026-05-02` Split `dataplane/crates/aeg-stream/src/tcp/tests/proxy.rs` into `proxy/{plain_tcp,tls_passthrough,sni_priority,half_close}.rs` with orchestrator, separating TCP proxy assertions by plain TCP proxy, TLS passthrough, SNI exact priority, client half-close preservation.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_stream.rs` into `tests_stream/{http_backend,http_endpoints,stream_weighted,tcp_port_isolation}.rs` with orchestrator, separating IR selection assertions by HTTP backend timeout/weight, endpoint rotation, stream weighted round-robin, TCP listener port isolation.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/context/state.rs` into `state/{lifecycle,selected_backend,connection_protocol,annotations}.rs` with orchestrator, separating proxy context state assertions by RequestContext lifecycle, selected backend cache, connection/protocol fields, route annotation cache policy.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/mesh_service_selection/cross_namespace.rs` into `cross_namespace/{consumer_scope,fallback,live_like}.rs` with orchestrator, separating mesh selection assertions by consumer workload scope, mesh service backend fallback, live-like cross-namespace service frontend regression.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/mesh_service_selection/excluded_ports.rs` into `excluded_ports/{same_service_fallback,non_mesh_listener_guard}.rs` with include orchestrator, separating mesh selection assertions by excluded mesh port same Service fallback and non-mesh listener attached route guard.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/backend_tls_validation/cache_and_errors.rs` into `cache_and_errors/{rotation,cache_reuse,errors}.rs` with orchestrator, separating backend TLS validation assertions by BackendTLSPolicy rotation, cache reuse/interleaved snapshots, missing CA error.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_selection/indexes_and_grpc/route_selection.rs` into `route_selection/{http_wildcard,grpc_exact,grpc_guards,grpc_listener_regex,grpc_catchall}.rs` with orchestrator, separating route selection assertions by HTTP wildcard, gRPC exact/header, non-gRPC guard, HTTP listener/regex, catch-all/header-only.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/context/request.rs` into `request/{capture,meta,request_id,tracing,header_cache}.rs` with orchestrator, separating proxy request context assertions by request context capture, request meta construction, request ID priority, traceparent propagation, CORS header cache.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_core/surface_status.rs` into `surface_status/{meta_health,snapshot_runtime}.rs` with orchestrator, separating admin summary surface assertions by meta/instance/health/warnings and snapshot/runtime overview.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/backend_client_cert.rs` into `backend_client_cert/{basic,rotation,cache_reuse,errors}.rs` with orchestrator, separating backend client cert assertions by backend client cert usage, snapshot rotation, cache reuse/interleaved snapshots, missing secret error.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/retries_and_limits/retry.rs` into `retry/{connect_failure,response_status,budget}.rs` with orchestrator, separating HTTP/1 retry assertions by connect failure retry, retryable response status, retry budget exhaustion.
- `2026-05-02` Split `dataplane/crates/aeg-shared-tls/src/tests/listener_plan.rs` into `listener_plan/{shared_bind,frontend_validation,addresses_and_protocols}.rs` with orchestrator, separating shared TLS listener plan assertions by shared bind, frontend validation, address expansion/non-TLS listener filtering.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_selection/endpoint_health/passive_ejection.rs` into `passive_ejection/{eject,expiry,inheritance,success}.rs` with orchestrator, separating endpoint health assertions by passive ejection, cooldown expiry, runtime state inheritance, success clearing.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_selection/core/runtime_inheritance.rs` into `runtime_inheritance/{weighted_progress,recovered_backend,updated_weights}.rs` with orchestrator, separating runtime inheritance assertions by weighted selection progress inheritance, recovered backend return, new weights taking effect.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_load_balancing/consistent_hash_backends.rs` into `consistent_hash_backends/{same_key,stable_hash,fallback}.rs` with orchestrator, separating backend consistent hash assertions by header hash stickiness, stable backend hash, missing hash key fallback to weighted RR.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/request_body/content_length.rs` into `content_length/{forwarding,large_forwarding,body_limit,header_limit}.rs` with orchestrator, separating HTTP/1 request body assertions by content-length forwarding, large request body forwarding, body limit, header limit.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_runtime/current_failures.rs` into `current_failures/{runtime_failures,listener_blocking,failure_recovery_overview,attention_overview}.rs` with fixture orchestrator, separating summary current failure assertions by runtime failure array, listener blocking/risk, failure recovery overview, attention overview.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/summary_core/session_persistence.rs` into `session_persistence/{surface,warnings,features}.rs` with fixture orchestrator, separating summary session persistence assertions by snapshot/runtime/health surface, ephemeral secret warning overview, session persistence feature counts.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/context/runtime.rs` into `runtime/{failure_ejection,read_guard,success_clears}.rs` with orchestrator, separating proxy runtime context assertions by failure passive ejection, snapshot read guard completion, success clearing failure streak.
- `2026-05-02` Split `dataplane/crates/aeg-observability/src/runtime/tests.rs` into `tests/{reload_snapshot,listener_history,apply_events,runtime_state,tls_plane,supervisor}.rs` with orchestrator, separating observability runtime assertions by reload snapshot/progress, listener history, apply event, runtime lifecycle, TLS plane, supervisor state.
- `2026-05-02` Split `dataplane/crates/aeg-shared-tls/src/runtime/tests.rs` into `tests/{binds,desired_plan,frontend_validation}.rs` with orchestrator, separating shared TLS runtime assertions by dual-stack bind, invalid HTTPS identity planning, frontend client certificate validation.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/filters/tests/header_and_request.rs` into `header_and_request/{request_modifier,response_modifier,request_filters,response_filters,property_chain}.rs` with orchestrator, separating filter assertions by request header modifier, response header modifier, request filter chain, response filter chain, property-based request filter chain.
- `2026-05-02` Split `dataplane/crates/aeg-xds/src/tests/status_reports.rs` into `status_reports/{heartbeat,snapshot_version,client_stats,discovery_and_requirements}.rs` with orchestrator, separating xDS status assertions by status heartbeat, snapshot version/apply, client stats, discovery ACK/NACK and runtime requirements.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/metrics/protection.rs` into `protection/{overload,circuit_breaker,rate_limit}.rs` with orchestrator, separating admin metrics assertions by three protection categories: overload, circuit breaker, rate limit.
- `2026-05-02` Split `dataplane/crates/aeg-observability/src/traffic/tests.rs` into `tests/{topology,shard_merge,capacity,eviction}.rs` with orchestrator, separating traffic graph assertions by traffic topology, shard merge, per-shard capacity, stale entry eviction.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/backend_protocol.rs` into `backend_protocol/{policy,protocol,cached_config,keepalive}.rs` with orchestrator, separating upstream peer assertions by backend timeout policy, backend protocol/SNI, cached peer config, TCP keepalive.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_proto/route_filters.rs` into `route_filters/{header_modifier,redirect_rewrite,request_mirror,cors}.rs` with orchestrator, separating proto route filter decode assertions by header modifier, redirect/rewrite, request mirror, CORS.
- `2026-05-02` Split `dataplane/crates/aeg-stream/src/tcp/tests/limits.rs` into `limits/{connection_budget,idle_timeout,max_connection_age}.rs` with orchestrator, separating stream TCP limit assertions by TCP listener connection budget, session idle timeout, max connection age.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_tls_assets/asset_cleanup.rs` into `asset_cleanup/{rotation_prune,write_contents,stale_temp,cleanup_unused}.rs` with orchestrator, separating TLS asset cleanup assertions by TLS asset rotation prune, asset content write, stale temp cleanup, unused cleanup.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/config_mapping/tests.rs` into `tests/{runtime_tuning,tracing,xds_transport}.rs` with shared fixture orchestrator, separating config mapping assertions by runtime tuning fan-out, OpenTelemetry tracing identity, xDS transport/TLS config.
- `2026-05-02` Split `dataplane/crates/aeg-xds/src/tests/runtime_apply.rs` into `runtime_apply/{apply_result,async_events,stream_message}.rs` with orchestrator, separating xDS runtime apply assertions by runtime apply success/failure/timeout, async apply event wakeup, xDS stream stale/message wait.
- `2026-05-02` Split `dataplane/crates/aeg-observability/src/access/tests.rs` into `tests/{rendering,route_overrides,sampling,writer}.rs` with shared fixture orchestrator, separating access log assertions by access log rendering/template, route annotation override, sampling, background writer/drop.
- `2026-05-02` Split `dataplane/crates/aeg-xds/src/tests/logging.rs` into `logging/{duplicate_snapshot,stream_reconnect,heartbeat}.rs` with orchestrator, separating xDS logging assertions by duplicate snapshot debug log, expected/unexpected stream reconnect log, heartbeat cancellation log.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/extension_filters.rs` into `extension_filters/{proto_nested,proto_flattened,selection}.rs` with orchestrator, separating ExtensionRef direct response assertions by nested proto direct response, flat proto direct response, no backend route selection.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/invalid_backend_refs.rs` into `invalid_backend_refs/{invalid_ref,mixed_refs,unhealthy,serviceimport}.rs` with orchestrator, separating error and route assertions by invalid backend ref, mixed backend refs, no healthy endpoint, ServiceImport backend ref.
- `2026-05-02` Split `dataplane/crates/aeg-config/src/tests/runtime_tuning.rs` into `runtime_tuning/{defaults,overrides}.rs` with orchestrator, separating dataplane runtime tuning assertions by default runtime tuning and explicit override configuration.
- `2026-05-02` Split `dataplane/crates/aeg-observability/src/overload/tests.rs` into `tests/{http_admission,snapshot_nonblocking,stream_admission}.rs` with orchestrator, separating overload assertions by HTTP overload admission, snapshot read non-blocking, TCP/UDP admission fast-fail.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/connection_and_direct.rs` into `connection_and_direct/{direct_response,keepalive,upstream_pool}.rs` with include orchestrator, separating HTTP/1 runtime assertions by direct response observability, downstream/upstream keepalive, cross-client upstream pool reuse.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/protocol_admission.rs` into `protocol_admission/{cors_preflight,listener_budget,route_budget}.rs` with include orchestrator, separating HTTP/1 protocol admission assertions by CORS preflight short-circuit, listener inflight budget, route inflight budget.
- `2026-05-02` Split `dataplane/crates/aeg-config/src/tests/runtime_protection.rs` into `runtime_protection/{defaults,overrides}.rs` with orchestrator, separating dataplane protection config assertions by default runtime protection and explicit override configuration.
- `2026-05-02` Split `dataplane/crates/aeg-stream/src/tests.rs` into `tests/{runtime_options,listener_replace,unchanged_plan_apply}.rs` with orchestrator, separating stream runtime assertions by TCP proxy buffer normalization, listener replace startup failure, unchanged stream plan version apply.
- `2026-05-02` Split `dataplane/crates/aeg-stream/src/listener_plan/tests.rs` into `tests/{build_plan,updates}.rs` with orchestrator, separating stream listener planning assertions by stream listener plan construction and listener update diff/reload.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_property.rs` into `tests_property/{request_meta,grpc_selection,proto_snapshot}.rs` with include orchestrator, separating IR property tests by RequestMeta query/method property, gRPC selection property, proto snapshot decode property.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/streaming/idle.rs` into `idle/{after_first_chunk,before_first_body}.rs` with include orchestrator, separating streaming HTTP idle assertions by idle after first chunk and idle before first body chunk.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/streaming/cancel.rs` into `cancel/{backend_cancel,client_cancel}.rs` with include orchestrator, separating streaming HTTP cancel/retry assertions by backend cancel and client cancel.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_h2c/grpc_control/timeout_and_metadata.rs` into `timeout_and_metadata/{response_metadata,timeout_header}.rs` with include orchestrator, separating H2C control flow assertions by gRPC timeout header and response metadata/status.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/runtime/tests_http1/request_body/chunked.rs` into `chunked/{body_limit,forwarding,trailers}.rs` with include orchestrator, separating HTTP/1 request body assertions by chunked normal forwarding, body limit, H1 trailer compatibility.
- `2026-05-02` Split `dataplane/crates/aeg-shared-tls/src/tests/runtime.rs` into `runtime/{routes,missing_bind}.rs` with module orchestrator, separating runtime assertions by shared TLS same-port passthrough/terminate forwarding and missing bind plan reload state.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_selection/endpoint_health/active_probe.rs` into `active_probe/{failure_threshold,inheritance,success_recovery}.rs` with include orchestrator, separating endpoint health assertions by active probe failure threshold, cross-snapshot inheritance, success recovery.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/session_persistence/http_routes/backend_policy.rs` into `backend_policy/{missing_token,policy_match,route_override}.rs` with include orchestrator, separating HTTP session persistence assertions by BackendPolicy match, no existing token, route policy override backend policy.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/listener_status_endpoints_filters/recovery_and_attention.rs` into `recovery_and_attention/{attention_reason,attention_required,has_ever_failed,recovered}.rs` with module orchestrator, separating listener status endpoint filter assertions by recovery, historical failure, requires attention, attention reason.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/listener_status_endpoints_filters/runtime_and_progress.rs` into `runtime_and_progress/{attempt_progress,runtime_plane}.rs` with module orchestrator, separating listener status endpoint filter assertions by runtime plane/current failure and attempt progress/failure age.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_selection/stream_and_fallback/http_fallback.rs` into `http_fallback/{basic,unmatched_route,mesh_grpc_no_route,mesh_http_no_route,mesh_short_host}.rs` with include orchestrator, separating fallback selection assertions by normal fallback, route miss, mesh HTTP/gRPC/short host no-fallback.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_selection/stream_and_fallback/stream_routes.rs` into `stream_routes/{tls_exact_priority,tls_wildcard,udp}.rs` with include orchestrator, separating stream route assertions by TLS wildcard, TLS exact SNI priority, UDP listener selection.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_selection/indexes_and_grpc/runtime_indexes.rs` into `runtime_indexes/{backend_secret_workload,listener_hostname_routes,stream_listener_routes}.rs` with include orchestrator, separating runtime index assertions by backend/secret/workload index, HTTP/gRPC hostname route index, stream listener route index.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/filters/tests/cors.rs` into `cors/{response_headers,non_matching_origin,preflight_apply,preflight_response}.rs` with module orchestrator, separating CORS filter assertions by matching origin, non-matching origin, preflight response filter, preflight short-circuit response.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/filters/tests/redirect_and_support.rs` into `redirect_and_support/{path_rewrite,authority_and_port,redirect_response,supported_filters}.rs` with module orchestrator, separating filter support assertions by URL rewrite, redirect authority/port, redirect response, supported filter validation.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/backend_tls_validation/peer_tls.rs` into `peer_tls/{hostname,policy_enables_tls,custom_ca,version_bounds}.rs` with module orchestrator, separating upstream peer TLS assertions by BackendTLSPolicy hostname, policy-enforced TLS, custom CA, version bound rejection.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/proxy/tests/backend_tls_validation/subject_alt_names.rs` into `subject_alt_names/{post_handshake,uri_validation,any_san_match,hostname_match}.rs` with module orchestrator, separating backend TLS subjectAltName assertions by post-handshake SAN validation, URI SAN, any SAN match, hostname SAN match.
- `2026-05-02` Split `dataplane/crates/aeg-http/src/session/tests.rs` into `tests/{options,cookie_transport,secret_file,generated_tokens}.rs` with fixture orchestrator, separating session persistence assertions by secret source, cookie token, file reload/cache, generated backend/transport property.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/session_persistence/http_routes.rs` into `http_routes/{route_policy,backend_policy}.rs` with orchestrator, separating HTTPRoute session persistence assertions by route-level sticky backend, unavailable session fallback, BackendPolicy inheritance/override.
- `2026-05-02` Split `dataplane/crates/aeg-ir/tests/session_persistence/grpc_routes.rs` into `grpc_routes/{route_policy,backend_policy}.rs` with include orchestrator, separating GRPCRoute session persistence assertions by gRPC route-level sticky backend and BackendPolicy session persistence.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_load_balancing.rs` into `tests_load_balancing/{proto_decode,consistent_hash_backends,consistent_hash_endpoints}.rs` with orchestrator, separating load balancing assertions by proto decode, backend consistent hash/fallback, endpoint consistent hash.
- `2026-05-02` Split `dataplane/crates/aeg-app/src/admin/tests/auth_health.rs` into `auth_health/{auth,livez,readyz}.rs` with orchestrator, separating health check assertions by admin auth bypass/protection, livez runtime health, readyz readiness gate.
- `2026-05-02` Split `dataplane/crates/aeg-ir/src/tests_weighted/request_mirror.rs` into `request_mirror/{single_mirror,primary_selection,multiple_mirrors,fraction_sampling}.rs` with orchestrator, separating request mirror assertions by single mirror backend, primary selection unaffected, multiple mirror expansion, fraction sampling window.
- `2026-05-02` Split backend client certificate lookup and BackendTLSPolicy validation from `dataplane/crates/aeg-http/src/proxy/backend.rs` into `backend/{client_cert,tls_validation}.rs`, main file only retains upstream peer construction, backend protocol / timeout policy, and selection error mapping; `backend.rs` is now `197` lines.
- `2026-05-02` Split TLS asset materialization and listener bind address planning from `dataplane/crates/aeg-http/src/runtime/listener_plan.rs` into `listener_plan/{assets,binds}.rs`, main file retains runtime plan types, TLS identity resolution, and SNI candidate sorting; `listener_plan.rs` is now `275` lines.
- `2026-05-02` Split completion observability from `dataplane/crates/aeg-http/src/proxy.rs` into `proxy/logging.rs`, centralizing traffic graph, route-scoped access log override, access log record, and post-completion context cleanup; `proxy.rs` is now `729` lines.

Acceptance:

- Split by responsibility, no behavioral changes mixed in.
- Corresponding crate unit tests pass.
- Prioritize splitting when approaching 800 lines.
