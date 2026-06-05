# Aether Gateway Latest Automation Baseline Record

This document records a combined view of the current repository's "most recent full `A1` sample" and "most recent archived sub-baseline evidence".
It is not a replacement for the long-term archive directory, but provides maintainers with a recently reusable execution sample and aligns the `latest` pointer and long-term archives with current repository facts.

## 1. Current Baseline Composition

<!-- release-evidence:baseline-current:start -->
- Most recent full `A1` release-level one-click automation sample: `2026-03-27`
- Most recent archived conformance report: `2026-05-14-162416-90f5126a-full`, commit `90f5126a`, covering `ALL_FEATURES=true` full-suite
- Most recent archived full-suite conformance: `2026-05-14`, commit `90f5126a`
- Most recent clean-commit full-suite conformance baseline: `2026-05-14`, commit `90f5126a`
- Most recent `A4` kind performance evidence: `2026-05-16`, commit `62145cc9`
- Most recent soak pilot evidence: `2026-05-14`, commit `1b48c5a6`
- Most recent full chaos release gate evidence: `2026-05-14`, commit `bb72c8f7`
- Current document state: the latest full-suite archive and clean full-suite baseline remain aligned to `90f5126a`; the most recent A4 kind performance evidence has advanced to `62145cc9`. However, the current candidate has not yet formed a "complete `A1 + A4 + 24h soak + fault injection + performance` unified refresh on the same candidate commit"
<!-- release-evidence:baseline-current:end -->

## 2. Execution Commands

The most recent full `A1` sample command remains:

```bash
./scripts/run-release-validation.sh
```

The minimum validation entry point for routine changes is:

```bash
PLAN_ONLY=true ./scripts/run-targeted-validation.sh
```

Currently `run-targeted-validation.sh` outputs three categories of information:

- `validation plan`: the minimum validation commands selected by diff for this run and the selection rationale.
- `skipped validations`: high-cost validations such as Kind smoke and metrics surface that are not run by default, and how to enable them.
- `notes`: supplementary notes, such as documentation-only changes, configuration changes, or validation tiers requiring manual judgment.

To include cluster-level validation in the plan, use:

```bash
INCLUDE_KIND=true SKIP_BUILD=true PLAN_ONLY=true ./scripts/run-targeted-validation.sh
```

## 3. Results Summary

### 3.1 Most Recent Full `A1` Sample (`2026-03-27`)

All of the following steps passed:

- `make proto`
- `cd controlplane && go test ./...`
- `cargo test --manifest-path dataplane/Cargo.toml --workspace`
- `./tests/e2e/run-kind.sh`
- `./scripts/update-gateway-api-support.sh --check`
- `./tests/e2e/validate-status-surfaces.sh`
- `./tests/e2e/validate-backend-protocols.sh`
- `./tests/e2e/validate-gateway-cross-namespace-certs.sh`
- `./tests/e2e/validate-grpc-reference-grants.sh`
- `./tests/e2e/validate-reference-grants.sh`
- `./tests/e2e/validate-upstream-behavior.sh`
- `./tests/e2e/validate-session-persistence.sh`
- `./tests/e2e/validate-http-security.sh`
- `./tests/e2e/validate-mesh-frontends.sh`
- `ALL_FEATURES=true ./tests/conformance/run.sh`

Key conclusions:

- Kind smoke passed.
- Support matrix generation results are consistent with documentation; no longer reliant on manual comparison of `supportedFeatures` before release.
- `GatewayClass` / `Gateway` / `Route` conditions are consistent with the dataplane admin view.
- Backend protocol selection passed.
- HTTP/gRPC `ReferenceGrant` and Gateway cross-namespace `certificateRef` passed.
- Upstream keepalive, retry, failover, weighted routing, and metrics validation passed.
- Session persistence passed, using a stable secret rather than a temporary key.
- `Host` / `X-Forwarded-*` spoofing cannot bypass host-based routing.
- Duplicate `Host`, conflicting `Content-Length`, `CL/TE`, `TE/CL`, malformed chunked, and oversized headers did not hit the inspect backend.
- Under lightweight slow-header probing with 8 concurrent partial headers, clean requests still completed within `0.001002s`.
- Full-suite conformance final result: `PASS`.

### 3.2 Subsequently Refreshed Archived Evidence

<!-- release-evidence:baseline-refreshes:start -->
- Conformance `latest/` has been refreshed to `2026-05-14-162416-90f5126a-full/`, corresponding to [report.yaml](../../reports/conformance/latest/report.yaml), [metadata.yaml](../../reports/conformance/latest/metadata.yaml), and [run.log](../../reports/conformance/latest/run.log). This is an `ALL_FEATURES=true` full-suite archive with implementationVersion `90f5126a`
- The most recent clean-commit full-suite conformance baseline is `2026-05-14-162416-90f5126a-full/`
- The most recent `A4` performance baseline has been refreshed to `reports/performance/runs/2026-05-16-201743-62145cc9-fault-scenarios-a4-profiles/`; the full chaos release gate has been refreshed to `reports/chaos/runs/2026-05-14-123117-bb72c8f7-kind-faults/`; the soak pilot still uses `reports/soak/runs/2026-05-14-125005-1b48c5a6-kind-soak-1h/`
- There are currently no additional `A2` scripts; mesh frontend has been merged into `A1`
<!-- release-evidence:baseline-refreshes:end -->

## 4. Artifact Locations

<!-- release-evidence:baseline-artifacts:start -->
- Current latest conformance report: `reports/conformance/latest/report.yaml`
- Current latest conformance metadata: `reports/conformance/latest/metadata.yaml`
- Current latest conformance log: `reports/conformance/latest/run.log`
- Current latest immutable archive: `reports/conformance/runs/2026-05-14-162416-90f5126a-full/`
- Most recent clean-commit full-suite baseline: `reports/conformance/runs/2026-05-14-162416-90f5126a-full/`

### 4.1 Most Recent A4 Kind Evidence

Additionally ran and archived the following two sets of kind evidence requiring long-term retention:

- Performance and capacity baseline: `reports/performance/runs/2026-05-16-201743-62145cc9-fault-scenarios-a4-profiles/`
- Fault injection: `reports/chaos/runs/2026-05-14-123117-bb72c8f7-kind-faults/`

Key results of this round:

- HTTP concurrency baseline:
- steady `p99=12.90ms`
- burst `p99=7.95ms`
- ceiling `p99=13.82ms`
- gRPC unary baseline: `p99=8.43ms`
- WebSocket / SSE / MCP long-lived profile: `p99=3052.43ms` / `1505.75ms` / `1504.38ms`, all three SLO gates are `pass`
- TCPRoute steady baseline: `p99=6.79ms`, `p999=7.91ms`, `RPS=5617.97`
- UDPRoute multi-client / high-churn / multi-upstream baseline: `p99=28.13ms` / `14.01ms` / `6.92ms`, ordinary UDP profiles packet loss is `0`
- UDPRoute backend-timeout baseline: expected timeout, `64/64` datagrams to black-hole backend, `p99=204.39ms`
- Backend-error / backend-slow-read / backend-slow-write / endpoint-flapping scenario profiles: `400/400`, `400/400`, `400/400`, `8000/8000` success respectively, `p99=45.41ms` / `426.30ms` / `127.01ms` / `98.36ms`
- Live reload route-only / backend-only / endpoint-only / secret-only / TLS asset rotation / listener add-remove — six profile types total `4800/4800` success, max `p99=11.47ms`, required protocols / mutations all accounted for
- This round's sampled dataplane upstream pool miss is `457`, pool hit ratio is `99.21%`
- This round's A4 protocol coverage covers `http`, `grpc`, `websocket`, `sse`, `mcp`, `tcp`, `udp`, `missing_protocols=[]`; scenario coverage covers `steady`, `burst`, `ceiling`, `long-lived-streaming`, `backend-error`, `backend-slow-read`, `backend-slow-write`, `endpoint-flapping`, `reload-under-load`, `missing_scenarios=[]`
- Upstream keepalive, retry, failover, weighted distribution, weight convergence, and backend recovery have all left raw logs
- Plain/h2c/ws success and failure paths in backend protocol selection have all left raw logs
- During the fault injection release gate, sustained HTTP traffic completed `32000` total requests, with `32000` successes, average success rate `100%`, error count `0`, batch max `p99≈1063.08ms`; the four required scenarios `controlplane-leader-switch`, `dataplane-pod-restart`, `node-drain`, and `apiserver-watch-disruption` all `pass`
- `scripts/run-kind-soak.sh` has verified a short-term pilot baseline; the most recent sample is at `reports/soak/runs/2026-05-14-125005-1b48c5a6-kind-soak-1h/`. This sample is of type `1h soak pilot`, completing `1374400` total requests with `1374400` successes, average success rate ~`100.00%`, batch max `p99≈1067.11ms`, max latency `3105.25ms`, xDS NACK delta `0`, ready replicas `2/2` throughout. This sample is not a `24h` complete conclusion and is only used to stabilize the soak automation entry point
<!-- release-evidence:baseline-artifacts:end -->

## 5. Content Not Covered in This Round

This round executed "the core regression tests already automated in the current repository." The following content is still not included in this round's automation results:

- Manual judgment items for local integrated debugging
- Header injection / CRLF deep analysis
- Slow body, prolonged idle, large-scale slowloris / connection flood
- `24h/72h` soak
- Re-running node drain, apiserver / watch latency jitter on subsequent candidate commits
- Production environment canary rollout and rollback drills

Maintenance entry points for these items:

- [Automation Status and Maintenance Rules](./automation-status.md)
- [Test Execution Checklist](./checklist.md)
