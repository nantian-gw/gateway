# Aether Gateway Performance Baseline Execution Template

This document consolidates the `PERF-*` items from `docs/test/plan.md` into an
execution template that follows a “run the minimum baseline first, then expand
gradually” approach. The goal is not to derive all capacity conclusions in one
shot, but to establish a stable set of performance metrics for long-term
maintenance.

If the goal is to close the “multi-environment performance and capacity
baseline” gate, also read
[multi-environment-performance-baseline.md](multi-environment-performance-baseline.md).
A single Kind or local microbenchmark run is not sufficient to close that gate.

## 1. Scope

Primarily intended for the following scenarios:

- Establishing a current performance baseline before a release
- Changes to hot reload, endpoint churn, connection pools, or retry policies
- Preparing for canary rollout and needing a stable/canary comparison
- Answering “has the current version regressed compared to the previous version”

## 2. Environment Tiers

| Tier | Goal | Recommended Environment | Current Repo Status |
| --- | --- | --- | --- |
| P1 Minimum Baseline | Quickly detect obvious regressions | Local Kind / dev machine | Ready to run |
| P2 Extended Baseline | Establish a pre-release comparison baseline | Kind or staging environment | Runnable, but requires load testing tools |
| P3 Long-Running Validation | Soak, capacity inflection, chaos | Staging environment | Depends on dedicated environment |

Current minimum recommendation:

1. First complete the `P1` HTTP and upstream behavior baseline
2. Then complete the `P2` gRPC, hot reload, and metrics collection
3. Before release, supplement with `P3` 24h soak and capacity inflection

## 3. Prerequisites

### 3.1 Minimum Environment

First prepare a reusable Kind environment:

```bash
./tests/e2e/run-kind.sh
```

Entry points directly reusable in the default smoke environment:

- HTTP: `http://127.0.0.1:18080/`, header `Host: example.com`
- gRPC: `127.0.0.1:18080`, `:authority = grpc.example.com`

### 3.2 Recommended Tools

- `vegeta`
- `wrk2`
- `h2load`
- `ghz`
- `jq`
- `./tests/e2e/validate-http-concurrency.sh`
- `./tests/e2e/http_concurrency_client.py`

If any tool is missing, you may keep an alternative command at the same tier, but note the different baseline in the results.

## 4. Evidence Directory

It is recommended to fix a directory before each execution:

```bash
export PERF_EVIDENCE_DIR="tmp/test-evidence/perf-$(date +%Y%m%d%H%M%S)"
mkdir -p "${PERF_EVIDENCE_DIR}"
```

Collect admin snapshots both before and after load testing:

```bash
OUTPUT_DIR="${PERF_EVIDENCE_DIR}/admin-before" \
  ./scripts/collect-admin-snapshots.sh

# Execute load testing and specialized scenarios

OUTPUT_DIR="${PERF_EVIDENCE_DIR}/admin-after" \
  ./scripts/collect-admin-snapshots.sh
```

If this is a Kind environment and the local machine has not set up `port-forward` for the admin Service, you can enable it directly:

```bash
ENABLE_KIND_PORT_FORWARD=true \
OUTPUT_DIR="${PERF_EVIDENCE_DIR}/admin-before" \
  ./scripts/collect-admin-snapshots.sh
```

## 5. Trend Comparison and Regression Thresholds

When both the baseline run and current run are archived under `reports/performance/runs/`, use:

```bash
./scripts/compare-performance-runs.sh \
  reports/performance/runs/<baseline-run-id> \
  reports/performance/runs/<current-run-id>
```

The script writes the following under `reports/performance/comparisons/`:

- `summary.md`: A performance comparison summary for reviewers and operators.
- `index.json`: Machine-readable results for CI, release gates, or subsequent trend aggregation.

Default thresholds:

- `LATENCY_REGRESSION_PCT=20`
- `LATENCY_REGRESSION_MIN_ABS_MS=0.1`
- `RPS_REGRESSION_PCT=20`
- `SUCCESS_RATE_DROP_PCT=1`
- `RESOURCE_REGRESSION_PCT=20`
- `RESOURCE_RSS_REGRESSION_MIN_ABS_KIB=4096`
- `RESOURCE_FD_THREAD_REGRESSION_MIN_ABS=1`
- `RESOURCE_CPU_TICK_REGRESSION_MIN_ABS=5`
- `CPU_REGRESSION_PCT=20`

Percentage thresholds are used to detect trend changes; absolute noise thresholds are used to avoid misclassifying 0.001ms-level microbenchmark jitter or tens-of-KiB RSS sample drift as regression. If a threshold is exceeded along with the corresponding noise threshold, the script exits non-zero; before release, these regressions must be explained in release notes or the validation report as either expected trade-offs, acceptable degradation, or risks that should block the release.

## 6. Minimum Performance Baseline

### 6.1 HTTP Baseline

Recommended: use `vegeta`:

```bash
echo "GET http://127.0.0.1:18080/" \
  | vegeta attack \
      -duration=5m \
      -rate=200 \
      -header "Host: example.com" \
  > "${PERF_EVIDENCE_DIR}/http-200rps.bin"

vegeta report "${PERF_EVIDENCE_DIR}/http-200rps.bin" \
  > "${PERF_EVIDENCE_DIR}/http-200rps.txt"
```

Recommended: run at least three tiers:

- `50 rps`
- `200 rps`
- `500 rps`

Record metrics:

- success rate
- `p50/p90/p95/p99`
- max latency
- control plane/data plane snapshot version stability
- dataplane `trafficRetryRate`、`trafficUpstreamPoolHitRatio`

If the current environment does not have `vegeta`/`wrk2`, at least run the lightweight concurrency regression in the repository:

```bash
./tests/e2e/validate-http-concurrency.sh
```

This script reuses the Kind smoke entry point by default and executes two tiers of HTTP load:

- steady: medium concurrency, fixed total requests
- burst: higher concurrency, short-duration connection bursts

It does not replace a formal performance baseline, but can cost-effectively discover the following types of issues:

- obvious `5xx`/reset/timeout under high concurrency
- request path slowed by slow logs, slow mirroring, or connection management anomalies
- tail latency suddenly rising to unacceptable levels

`scripts/run-kind-a4-baseline.sh` collects by default HTTP, gRPC, WebSocket, SSE, MCP streamable HTTP, TCPRoute, UDPRoute, backend slow/error, endpoint flapping, and reload-under-load profiles.HTTP profiles use keepalive connection mode by default, aiming to measure data plane latency when regular clients reuse connections;WebSocket/SSE/MCP profiles reuse existing backend-protocols/upstream-behavior specialized resources and write `long-lived-streaming` profile JSON;TCPRoute profile uses short-connection request/response path;UDPRoute profiles cover multi-client, high-churn, multi-upstream, and backend-timeout.The backend-timeout profile deploys a UDP blackhole backend that actually receives datagrams but does not respond, and writes the number of datagrams received by the backend into the profile JSON, avoiding misreading of missing backends as backend timeout.The backend-error, backend-slow-read, backend-slow-write, and endpoint-flapping profiles deploy A4-dedicated HTTP backends to verify expected 503, slow request body reads, slow response body writes, and success rate and p99 during EndpointSlice scale-down/recovery, respectively.The reload-under-load profiles trigger route-only, backend-only, endpoint-only, secret-only, TLS asset rotation, and listener add/remove snapshot mutations respectively while HTTP/gRPC/TCP/UDP traffic is running continuously.
If you need specialized validation of the “new TCP connection per request” connection storm path, you can explicitly switch:

```bash
HTTP_CONNECTION_MODE=close ./scripts/run-kind-a4-baseline.sh
```

When interpreting reports, do not directly attribute connection establishment, NodePort/docker-proxy, and client scheduling costs in close mode to Rust proxy request processing overhead.
For local Kind simple HTTPRoute regular keepalive, the recommended targets are `p95 < 20ms`, `p99 < 50ms`; accounting for local environment jitter, the A4 default gate uses more conservative `steady p99 <= 100ms`, `burst p99 <= 150ms`, `ceiling p99 <= 250ms`. TCPRoute/UDPRoute gates currently serve as same-environment stream/datagram baselines with more conservative default thresholds; they are used to detect obvious regressions and do not represent production capacity limits.
Close mode should be analyzed separately as a connection churn risk.

### 6.2 Upstream Behavior Baseline

This section directly reuses existing repository tests:

```bash
./tests/e2e/validate-upstream-behavior.sh
```

At minimum, record:

- whether keepalive reuse holds
- whether 503 retry failover succeeds
- whether timeout failover succeeds
- whether weighted distribution is close to expected
- `retry_rate`、`failover_success_rate`、`average_latency_ms`

### 6.3 gRPC Low-Cost Regression

The current repository includes a low-cost smoke client suitable for functional concurrency regression, but not as a replacement for full throughput load testing:

```bash
go build -o "${PERF_EVIDENCE_DIR}/grpc-smoke-client" ./controlplane/cmd/grpc-smoke-client

seq 1 200 \
  | xargs -I{} -P16 "${PERF_EVIDENCE_DIR}/grpc-smoke-client" \
      -addr 127.0.0.1:18080 \
      -authority grpc.example.com \
  > "${PERF_EVIDENCE_DIR}/grpc-smoke.txt"
```

If `ghz` is already installed in the environment, it is recommended to switch the full gRPC baseline to `ghz` and additionally record in the evidence:

- concurrency
- connections
- duration
- error rate
- `p95/p99`

## 7. Extended Performance Baseline

### 7.1 Hot Reload and Endpoint Churn

At minimum, execute once under sustained traffic:

- modify `HTTPRoute`
- modify weight
- modify Secret
- rolling restart backend Pods

Pass criteria:

- configuration propagation time `< 5s`
- no sustained 5xx spikes
- `p99` does not remain above baseline for an extended period after reload

Recommended to continuously collect during execution:

- `./scripts/collect-admin-snapshots.sh`
- data plane `/metrics`
- access log fragments

### 7.2 HTTPS Baseline

The current default smoke environment primarily covers HTTP, gRPC, TCP, UDP, and TLS passthrough.
If you need an HTTPS terminate baseline, first prepare a clearly usable HTTPS listener, then execute with `h2load` or an equivalent tool.

At minimum, record:

- handshake latency
- success rate
- `p95/p99`
- performance overhead relative to HTTP

### 7.3 Soak

Recommended: run at least one 24h soak in the staging environment:

- target load: `30% - 50%` of rated peak
- retain continuous metrics scraping
- sample admin snapshot once per hour
- when using `REQUIRE_24H=true ./scripts/run-kind-soak.sh` or an equivalent staging entry point, do not substitute short `SOAK_DURATION_SECONDS` for release evidence; archived metadata must record `duration_seconds>=86400`
- when a release candidate references soak evidence, `scripts/verify-release-evidence.sh` also requires `code_tree_state=clean` in metadata and `traffic/summary.json.slo_gate.status=pass` by default; reusing dirty evidence or evidence from old scripts missing this field requires explicit `--allow-dirty-soak-code-tree` risk acceptance, and traffic SLO failure evidence cannot serve as default release evidence
- `SUMMARY_ONLY=true` can regenerate summary and SLO gate from existing soak artifacts, and will backfill `run_id`, tree state, duration, sampling interval, and SLO threshold metadata for release evidence auditing

Minimum pass criteria:

- no sustained linear memory growth
- no sustained linear FD growth
- error rate does not continuously rise
- snapshot version does not exhibit abnormal jitter

### 7.4 Control Plane Status Storm Local Baseline

If the current change primarily affects the controlplane status/infrastructure convergence path rather than the data plane traffic path, it is recommended to first run local control plane benchmarks instead of directly starting kind:

```bash
./scripts/run-controlplane-status-bench.sh
```

If this round of results needs to be archived as long-term citable evidence, directly enable archive mode:

```bash
ARCHIVE_REPORTS=true ./scripts/run-controlplane-status-bench.sh
```

This script will consistently execute two benchmark groups:

- `internal/status`
- `BenchmarkReconcileFullStatusRouteFanout`
  for observing the route fanout cost of full status reconcile under `50`/`200` HTTPRoutes
- `BenchmarkReconcileFullStatusAttachDetachStorm`
  for observing the status churn cost when the same batch of HTTPRoutes repeatedly attach/detach

- `internal/controller`
- `BenchmarkPublishSnapshotRouteFanout`
  for observing the route fanout cost of the full snapshot publish path under `50`/`200` HTTPRoutes
- `BenchmarkPublishSnapshotAttachDetachStorm`
  for observing the snapshot rebuild cost when the same batch of HTTPRoutes repeatedly attach/detach
- `BenchmarkSnapshotInputStatusStorm`
  for observing the cost of status-only Route update storms being filtered at the watch predicate layer, rather than triggering snapshot rebuild every time

Default artifacts are written to:

```bash
tmp/controlplane-status-bench/<run-id>/
```

At minimum, retain:

- `metadata.txt`
- `status-bench.txt`
- `controller-bench.txt`
- `bench.txt`
- `summary.md`

This set of results can serve as local preliminary evidence for `reconcile latency / queue depth / status storm`, but it does not replace conclusions from real Kind or staging environment API server pressure, node drain, and long-duration soak.

### 7.5 Data Plane Hot Reload Local Baseline

If the current change primarily affects the dataplane reload/xDS apply/TLS asset rotation path rather than the north-south load testing path, it is recommended to first run local reload benchmarks:

```bash
./scripts/run-dataplane-reload-bench.sh
```

If this round of results needs to be archived as long-term citable evidence, directly enable archive mode:

```bash
ARCHIVE_REPORTS=true ./scripts/run-dataplane-reload-bench.sh
```

This script will consistently generate four categories of local evidence:

- `large_snapshot_switch`
  observe the cost of large snapshot clone, runtime state inheritance, and a single backend selection probe
- `request_meta_header_heavy` / `request_view_header_heavy`
  compare eager metadata materialization vs. lazy request view capture cost under header-heavy requests
- `snapshot_read_rwlock` / `snapshot_read_arc_swap`
  compare `RwLock` vs. `ArcSwap` baselines for the shared snapshot read path, preserving evidence for future lock-free read model evaluation
- `runtime_index_rebuild_route_only` / `runtime_index_rebuild_endpoint_only` / `runtime_index_rebuild_secret_only`
  observe the cost of runtime index rebuild when route, endpoint, or secret input changes individually
- `access_log_disabled_path` / `access_log_sampled_out_path` / `access_log_write_path`
  observe the hot path cost of access log disabled, sample miss, and full write paths respectively
- `traffic_observe_reused_topology` / `traffic_observe_no_route` / `traffic_observe_backend_topology_4_shards` / `traffic_observe_backend_topology_64_shards`
  observe the hot path cost of traffic stats under high label reuse, no-route fallback, backend topology, and different shard counts respectively
- `http_capacity_matrix`
  a capacity derivation matrix with fixed HTTP runtime worker threads, accept concurrency, upstream keepalive pool size, and reuse_port
- `stream_tcp_buffer_matrix`
  a matrix with fixed TCP proxy buffer defaults, upper/lower clamp limits, and tuning candidate baselines
- `stream_udp_dispatcher_distribution`
  a baseline with fixed UDP dispatcher worker, queue capacity, and session shard key distribution
- `stream_udp_payload_copy`
  observe the hot path cost of representative UDP datagram payload copy
- `tls_asset_rotation`
  observe the cost of repeated TLS asset materialization, CA bundle rotation, and stale asset cleanup
- `high_frequency_apply`
  observe the cost of high-frequency xDS apply success path and status report generation
- `last_good_fallback`
  observe whether `last-good` ready semantics and error panel output remain stable when the current version is rejected

Default artifacts are written to:

```bash
tmp/dataplane-reload-bench/<run-id>/
```

By default, `cargo run --release` is used to collect local baselines per the dataplane workspace official `release profile`; to preserve a debug build baseline, you can explicitly override:

```bash
CARGO_PROFILE=dev ./scripts/run-dataplane-reload-bench.sh
```

If the local default Rust toolchain is not the repository's current supported version, you can explicitly specify the cargo toolchain, and the script will write that baseline into `metadata.txt`:

```bash
CARGO_TOOLCHAIN=1.88.0 ./scripts/run-dataplane-reload-bench.sh
```

The same variable is also passed through by `run-dataplane-perf-baseline.sh` to the replay benchmark, `perf record`, and `perf stat` stages:

```bash
CARGO_TOOLCHAIN=1.88.0 ./scripts/run-dataplane-perf-baseline.sh
```

To compare different allocators, you can explicitly switch:

```bash
ALLOCATOR=mimalloc ./scripts/run-dataplane-reload-bench.sh
ALLOCATOR=jemalloc ./scripts/run-dataplane-reload-bench.sh
```

The script writes `allocator_requested`/`allocator_observed` into `metadata.txt` and validates that the `bench.json` top-level `allocator` field matches the script's requested value, preventing report baseline drift.

With `ARCHIVE_REPORTS=true` enabled, additional archiving goes to:

```bash
reports/performance/runs/<run-id>/
```

At minimum, retain:

- `metadata.txt`
- `bench.json`
- `summary.md`

The `2026-04-26` clean-tree allocator comparison baselines have been archived to:

- `reports/performance/runs/2026-04-26-195218-b3b2b85-dataplane-reload-bench-system/`
- `reports/performance/runs/2026-04-26-195400-b3b2b85-dataplane-reload-bench-mimalloc/`
- `reports/performance/runs/2026-04-26-195457-b3b2b85-dataplane-reload-bench-jemalloc/`

In this round of results, `jemalloc` was faster on average/`p95` latency in most reload microbenchmarks, but had higher RSS delta in `large_snapshot_switch` compared to `system` and `mimalloc`; `mimalloc` improved some local paths but did not form a stable overall advantage. Therefore the current default remains `system allocator`, with a decision on switching to be made later based on `A4` throughput and long-running evidence.

This set of results is primarily used to establish a local preliminary baseline for `reload latency / xDS apply overhead / TLS asset churn / RSS/FD delta`. It does not replace kind `A4` real traffic performance, 24h soak, node drain, and apiserver jitter conclusions, but can help determine whether the reload path is already broken locally before engaging more expensive environments.

### 7.6 Data Plane Throughput Report

If you already have a kind, staging, or production sampling evidence directory, you can first standardize it into a data plane throughput report:

```bash
INPUT_DIR=reports/performance/runs/2026-04-30-1bc4aea-comprehensive-kind-a4 \
  ./scripts/run-dataplane-throughput-baseline.sh
```

Default output goes to:

```bash
reports/performance/runs/<run-id>-dataplane-throughput/
```

If you need to first reuse the existing kind A4 entry point to collect HTTP/gRPC/TCPRoute/UDPRoute evidence, then generate the same throughput report, you can explicitly enable:

```bash
RUN_KIND_A4=true ./scripts/run-dataplane-throughput-baseline.sh
```

If you already have separately archived fault injection or soak evidence, you can aggregate them as external inputs into the same throughput report:

```bash
INPUT_DIR=reports/performance/runs/2026-05-16-201743-62145cc9-fault-scenarios-a4-profiles/source-kind-a4 \
CHAOS_INPUT_DIR=reports/chaos/runs/2026-05-14-123117-bb72c8f7-kind-faults \
SOAK_INPUT_DIR=reports/soak/runs/2026-05-14-125005-1b48c5a6-kind-soak-1h \
  ./scripts/run-dataplane-throughput-baseline.sh
```

The script reads `http/*.json`, `grpc/*.json`, `tcp/*.json`, `udp/*.json`, `admin-after/dataplane/traffic.json`, `metrics.prom`, and `resources/after.tsv` from the evidence directory, and generates:

- `metadata.txt`
- `throughput-report.json`
- `summary.md`

The report places each profile's `RPS`, success rate, `p50/p90/p95/p99/p999/max` alongside global status class, response flag, retry attempts, upstream pool hit/miss, pool hit ratio, upstream connect latency `p95/p99`, traffic graph node/edge counts, traffic graph node kind distribution, Prometheus traffic label cardinality check, request latency histogram series, CPU/RSS/FD/threads into a single artifact, and outputs required/observed/missing protocol coverage. The current A4 entry point already collects HTTP, gRPC, WebSocket, SSE/MCP, TCPRoute, UDPRoute multi-client/high-churn/multi-upstream/backend-timeout, backend-error, backend-slow-read, backend-slow-write, endpoint-flapping, and reload-under-load live traffic profiles; the latest `2026-05-16` A4 archive `reports/performance/runs/2026-05-16-201743-62145cc9-fault-scenarios-a4-profiles/` has `missing_protocols=[]`, `missing_scenarios=[]`, `reload.live_traffic.missing_protocols=[]`, `reload.live_traffic.missing_mutations=[]`. `reports/performance/runs/2026-05-16-204457-46452c26-chaos-soak-throughput-report/` further aggregates the full fault injection release gate and `1h` soak pilot: chaos `release_gate_status=pass`, traffic SLO `pass`, soak traffic SLO `pass`, `duration_seconds=3600`, `is_24h=false`. This aggregated report cannot substitute for a real `24h` soak or non-Kind production-like rerun on the same candidate commit.

## 8. Metric Baselines

Each round of performance baselines should at minimum uniformly record the following:

- QPS / RPS
- success rate / error rate
- `p50/p90/p95/p99/p999`
- TLS handshake latency
- upstream connect latency
- retry rate
- failover success rate
- upstream pool hit ratio
- CPU / RSS / FD / active connections
- controlplane reconcile latency / queue depth

## 9. Recommended Threshold Template

If there is no formal business SLA yet, start with the following template:

- error rate `< 0.1%`
- `p99` no more than `10%` above the previous stable baseline
- configuration propagation `< 5s`
- no sustained linear memory or FD growth during 24h soak
- canary version performance degradation relative to stable version `< 10%`

For an actual release, replace these templates with the business's own acceptance thresholds.

## 10. Required Evidence

Each round of performance baselines must at minimum retain:

- load test commands
- load test parameters
- raw result files
- `admin-before` and `admin-after`
- `metrics.prom`
- if regression occurs, retain the corresponding version, logs, and fix commit

## 11. Current Boundaries

This document has already established the most reusable performance entry points in the current repository, but the following items still need to be supplemented later:

- standardized `ghz` gRPC baseline commands
- dedicated HTTPS terminate baseline environment
- unified 24h/72h soak automation
- configuration scale batch generation script
- canary/stable automatic comparison template
