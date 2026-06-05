# Aether Gateway Test Execution Checklist

Use this document in conjunction with the following documents:

- [Test Plan](./plan.md): Explains test boundaries, tiers, and principles
- [Test Case Matrix](./case-matrix.md): Provides a detailed test case catalog
- [Automation Status and Maintenance Rules](./automation-status.md): Explains which are already automated and which still require specialized environments
- [Latest Automation Baseline Record](./latest-baseline.md): Records the most recent full automation regression results
- [Security Regression Execution Template](./security-regression.md): Collates security specialization steps, commands, and evidence formats
- [Performance Baseline Execution Template](./performance-baseline.md): Collates load test baselines, thresholds, and evidence formats

This document does not repeat all test cases. Instead, it answers two execution questions:

1. What is the minimum to run for the current change.
2. What evidence must be left before release.

The current release validation only covers control plane, data plane, Kind, and specialized E2E; dashboard UI specialized tests are not included as core gateway release gating. Dashboard-related changes require a separate `cd dashboard && npm run check`.

## 1. Usage Principles

- Run the cheapest validation first, then more expensive validation.
- First validate the layer closest to the change, then proceed to Kind, specialized E2E, conformance, performance, and canary.
- Do not default to recreating the Kind cluster without a clear need.
- When conformance or E2E fails, do not just look at harness output; must synchronously check control plane, data plane, and management interfaces.
- Before release, traceable reports, logs, and key interface snapshots must be preserved.

## 2. Environment Preparation Checklist

### 2.1 Local Basic Dependencies

- `make`
- `go`
- `cargo`
- `docker`
- `kubectl`
- `kind`
- `jq`
- `curl`
- `openssl`
- `socat`

### 2.2 Optional Specialized Tools

- `grpcurl`
- `ghz`
- `h2load`
- `vegeta`
- `fortio`
- `wrk2`
- `websocat`
- `tc`
- `osv-scanner`
- `trivy` or `grype`

### 2.3 Kind-Related Prerequisites

- By default, prioritize reusing the existing Kind cluster and local registry.
- Only use `RECREATE_CLUSTER=true` when Kind status, registry patch, or network state is abnormal.
- When reusing existing images, prioritize using `SKIP_BUILD=true`.

## 3. Select Validation by Change Type

### 3.1 Control Plane Only Changes

Minimum requirements:

```bash
cd controlplane && go test ./...
```

If the change involves any of the following, shared protocol linkage validation must also be added:

- `proto/`
- IR structures
- gRPC delivery fields
- `ReferenceGrant`
- Status write-back semantics
- snapshot digest, ACK, node status

Supplementary commands:

```bash
make proto
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

Further supplementary scenarios:

- When changes involve Route attachment, Gateway listener, cross-namespace authorization, add corresponding specialized E2E.
- When changes involve management interfaces or ACK/ready aggregation, add one local integrated debugging session.

### 3.2 Data Plane Only Changes

Minimum requirements:

```bash
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

If the change involves any of the following, protocol or specialized validation must also be added:

- HTTP/GRPC/TCP/UDP/TLS matching paths
- filter, redirect, rewrite, mirror, CORS, timeouts
- connection pool, retry, failover, weighted routing
- backend TLS, session persistence
- hot reload, reload, management interfaces, metrics

Recommended additions:

- One local process integrated debugging session
- One corresponding specialized E2E run

### 3.3 Shared Protocol or Linkage Logic Changes

Must execute:

```bash
make proto
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

If the change affects Gateway API semantics or cross-namespace authorization, also add:

```bash
./tests/conformance/run.sh
```

If the change affects `ReferenceGrant`, backend TLS, backend protocol, session persistence, upstream behavior, also add the corresponding specialized scripts.

### 3.4 Deployment, Script, or Cluster Behavior Changes

Minimum requirements:

```bash
bash -n tests/e2e/run-kind.sh
bash -n scripts/run-release-validation.sh
```

If the change affects images, deployment manifests, Listener exposure, NodePort/LoadBalancer, cluster network behavior, add:

```bash
SKIP_BUILD=true ./tests/e2e/run-kind.sh
```

Only when the Kind environment is inconsistent, add:

```bash
RECREATE_CLUSTER=true ./tests/e2e/run-kind.sh
```

## 4. Daily Development Checklist

Check before each coding session:

- Whether the current feature block is sufficiently independent and can be committed separately.
- Whether a cheaper validation path already exists to avoid defaulting to Kind.
- Whether shared structures between control plane and data plane are touched.
- Whether the support matrix, operations documentation, or test documentation needs to be synchronously updated.

Minimum check after each coding session:

- Control plane changes: control plane tests pass.
- Data plane changes: data plane tests pass.
- Shared protocol changes: `make proto` + dual-end tests pass.
- Management interface changes: at least one local integrated debugging of interface output.
- Cluster-level semantic changes: at least one Kind smoke or specialized E2E run.

Recommended entry point:

```bash
make test-targeted
PLAN_ONLY=true ./scripts/run-targeted-validation.sh
```

## 5. Pre-Merge Checklist

Before merging, at least confirm the following items:

- Corresponding `L0/L1` tests for the change have passed.
- If the change affects runtime behavior, one local integrated debugging or protocol-level specialized validation has been done.
- If the change affects Kubernetes semantics, one Kind smoke or corresponding specialized E2E has been done.
- If the change affects Gateway API semantics, quick conformance has been run.
- At least one form of evidence from management interfaces, logs, or metrics proves correct behavior.
- Command entry points in documentation are consistent with current repository scripts.

## 6. Specialized E2E Checklist

Select the following scripts by capability:

### 6.1 Kind Basic Smoke

```bash
./tests/e2e/run-kind.sh
```

Optional modes:

```bash
SKIP_BUILD=true ./tests/e2e/run-kind.sh
SKIP_SMOKE=true ./tests/e2e/run-kind.sh
RECREATE_CLUSTER=true ./tests/e2e/run-kind.sh
```

### 6.2 Cross-Namespace Authorization

```bash
./tests/e2e/validate-reference-grants.sh
./tests/e2e/validate-grpc-reference-grants.sh
./tests/e2e/validate-gateway-cross-namespace-certs.sh
```

### 6.3 Backend Protocol, Connection Pool, and Sticky Sessions

```bash
./tests/e2e/validate-backend-protocols.sh
./tests/e2e/validate-upstream-behavior.sh
./tests/e2e/validate-session-persistence.sh
```

### 6.4 Mesh Frontend / Service Parent Extension

```bash
./tests/e2e/validate-mesh-frontends.sh
```

### 6.5 HTTP Security Regression

```bash
./tests/e2e/validate-http-security.sh
```

Current script already covers:

- request smuggling: `CL/TE`, `TE/CL`
- malformed chunked
- duplicate header
- oversized headers
- `Host` / `X-Forwarded-*` spoofing
- lightweight slow-header probing

If the current change also involves slow body, idle timeout, or higher-intensity connection exhaustion, supplement A4 specialization per the [Security Regression Execution Template](./security-regression.md).

After executing any specialized script, at minimum check:

- Script return code
- Control plane and data plane management interfaces
- Relevant Route/Gateway conditions
- If the script has metrics validation, also check whether metrics are consistent with request behavior

## 7. Conformance Checklist

### 7.1 Daily Quick Regression

```bash
./tests/conformance/run.sh
```

Applicable scenarios:

- Gateway API semantics daily regression
- HTTPRoute / GRPCRoute / ReferenceGrant related changes

### 7.2 Pre-Release Full-Suite

```bash
ALL_FEATURES=true ./tests/conformance/run.sh
```

Must confirm before release:

- Currently declared supported features have no release-blocking failures
- If externally referenced commits have changed, report corresponds to commit

### 7.3 Report Archiving

```bash
ARCHIVE_REPORT_ID=local-$(date +%Y%m%d%H%M%S) ./scripts/run-release-validation.sh
```

### 7.4 Release Candidate Evidence Freshness Validation

When release notes, README, or the support matrix references a `full-suite`, `A4`, chaos, or soak archive, you cannot just say "the repo has run it before" — you must also clarify the relationship between this evidence and the current candidate commit:

```bash
./scripts/verify-release-evidence.sh \
  --candidate <candidate-commit> \
  --conformance reports/conformance/runs/<full-suite-id>/metadata.yaml \
  --performance reports/performance/runs/<a4-run-id>/metadata.txt \
  --chaos reports/chaos/runs/<fault-run-id>/metadata.txt \
  --soak reports/soak/runs/<soak-run-id>/metadata.txt
```

If an "adjacent and reviewed" commit window is permitted rather than an exact same candidate commit, the allowed values must be explicitly listed:

```bash
./scripts/verify-release-evidence.sh \
  --candidate <candidate-commit> \
  --allow-commit <approved-window-commit> \
  --conformance reports/conformance/runs/<full-suite-id>/metadata.yaml \
  --performance reports/performance/runs/<a4-run-id>/metadata.txt \
  --chaos reports/chaos/runs/<fault-run-id>/metadata.txt \
  --soak reports/soak/runs/<soak-run-id>/metadata.txt
```

If a set of archived evidence already exists and only the summary document needs to be uniformly refreshed to this set of evidence before the candidate release, the following wrapper entry point can be used directly; it only consumes existing archives and metadata, and will not rerun kind or conformance:

```bash
./scripts/refresh-release-evidence.sh \
  --candidate 66c7bef \
  --allow-commit af4a7d3
```

If the release summary entry point `CHANGELOG.md` also needs to be aligned with this set of evidence, run the refresh above first, then confirm:

```bash
./scripts/check-evidence-reference-alignment.sh
```

Additionally, `scripts/prepare-release-assets.sh` now enforces this check before packaging release assets, first re-validating the candidate commit and evidence window via `scripts/refresh-release-evidence.sh --check-only`; packaging will fail outright if `CHANGELOG.md` still references old evidence, or if `performance` / `chaos` / `soak` still point to unapproved old commits. The `release-evidence.txt` in the packaging artifacts will record the candidate commit, allowed window, and the actually selected conformance / performance / chaos / soak metadata, facilitating offline auditing of the release package.

If the same set of release evidence gates needs to be incorporated into the release candidate main entry point, enable it when executing release validation:

```bash
REQUIRE_RELEASE_EVIDENCE=true \
RELEASE_EVIDENCE_ALLOW_COMMITS="<approved-window-commit>" \
./scripts/run-release-validation.sh
```

In default mode, after all regular release validation passes, `scripts/refresh-release-evidence.sh --check-only` and `scripts/check-evidence-reference-alignment.sh` are run. Use `RELEASE_EVIDENCE_CANDIDATE` to override the candidate commit, or use `RELEASE_EVIDENCE_CONFORMANCE_RUN`, `RELEASE_EVIDENCE_PERFORMANCE_RUN`, `RELEASE_EVIDENCE_CHAOS_RUN`, `RELEASE_EVIDENCE_SOAK_RUN` to explicitly specify archives; if you only want to validate the evidence window first without requiring the current summary document to be refreshed, temporarily set `SKIP_RELEASE_EVIDENCE_ALIGNMENT=true`.

Default constraints:

- conformance `implementationVersion` must not be `-dirty`
- conformance, performance, chaos, and soak — all four evidence types must exist
- Each evidence type must point to the candidate commit or be explicitly listed in the allowed window
- Performance evidence must include `throughput-report.json`, and the missing lists for protocols, scenarios, and reload live-traffic must all be empty
- Performance evidence must include `source-kind-a4/slo-gate.json` or direct kind A4 `slo-gate.json`, and both the overall status and all profile statuses must be `pass`
- Performance, chaos, and soak metadata must record `code_tree_state=clean`
- Chaos evidence must include `conclusions/summary.json.release_gate_status=pass` and `traffic/summary.json.slo_gate.status=pass`
- Soak evidence must include `metadata.txt.duration_seconds>=86400` and `traffic/summary.json.slo_gate.status=pass`

If this archiving also refreshes `reports/conformance/latest/`, `reports/conformance/README.md`, `docs/test/latest-baseline.md`, `docs/gateway-api-support.md`, or `docs/community-readiness.md`, add one more summary consistency check:

```bash
./scripts/check-evidence-reference-alignment.sh
```

If this conclusion involves `TCPRoute` / `UDPRoute` coverage scope, also add a stream Route coverage boundary check. Gateway API `v1.5.1` official conformance only provides `UDPRoute` features / test cases; `TCPRoute` requires supplementary proof from the repository's kind smoke success and missing-backend failure paths — the two cannot be conflated into the same conformance conclusion:

```bash
./scripts/check-stream-route-test-coverage.sh
```

### 7.5 Full Automated Regression Entry Point for the Current Repository

To run a complete pass of the core tests that are "already automated and directly executable" in the current repository, the recommended direct execution is:

```bash
./scripts/run-release-validation.sh
./tests/e2e/validate-mesh-frontends.sh
```

Explanation:

- The first command covers the current release-level one-click automation baseline.
- The second command supplements the mesh frontend automation script not yet incorporated into the release baseline.
- This set of commands cannot substitute for performance, security, soak, chaos, and production canary specializations.

## 8. Local Integrated Debugging Checklist

### 8.1 Start Control Plane

```bash
cd controlplane
go run ./cmd/manager -config ../configs/controlplane/config.yaml
```

At minimum, check:

- `/livez`
- `/readyz`
- `/v1/summary`
- `/v1/snapshot-sync`
- `/v1/listeners`
- `/v1/routes`
- `/v1/backends`
- `/v1/nodes`

### 8.2 Start Data Plane

```bash
cargo run --manifest-path dataplane/Cargo.toml -p aeg-app -- \
  --config ../configs/dataplane/config.yaml
```

At minimum, check:

- `/livez`
- `/readyz`
- `/metrics`
- `/v1/summary`
- `/v1/node`
- `/v1/snapshot`
- `/v1/listeners`
- `/v1/routes`
- `/v1/backends`

### 8.3 Integrated Debugging Conclusions

At minimum, answer the following questions:

- Whether the control plane has generated a snapshot.
- Whether the data plane has received and applied the current snapshot.
- Whether ACK/ready are advancing.
- Whether the listener, route, and backend views are consistent.
- Whether metrics and logs can explain the current behavior.

## 9. Performance and Stability Checklist

Before release, at minimum the `P0` set from the following items should be completed:

- HTTP baseline throughput
- HTTPS baseline throughput
- gRPC baseline and streaming stability
- Upstream connection pool / retry / failover / weight validation
- Config hot reload and endpoint churn
- 24h soak

Recommended unified metrics to record:

- QPS / RPS
- Success rate / error rate
- `p50/p90/p95/p99/p999`
- TLS handshake latency
- upstream connect latency
- retry rate
- failover success rate
- upstream pool hit ratio
- CPU / memory / FD / active connections
- reconcile latency / queue depth

Recommended minimum threshold template:

- Error rate `< 0.1%`
- `p99` within business SLA
- Configuration propagation `< 5s`
- No sustained linear leak within 24h
- FD usage at safe watermark

## 10. Security Checklist

Before release, at minimum complete the following checks:

- admin authentication boundary
- cross-namespace `ReferenceGrant` authorization boundary
- TLS / SAN / mTLS cannot be bypassed
- request smuggling regression
- `Host` / `X-Forwarded-*` spoofing regression
- slowloris / connection flood basic protection
- dependency and image scanning

In-repo automated HTTP security regression entry point:

```bash
./tests/e2e/validate-http-security.sh
```

Recommended tools:

```bash
cargo audit
osv-scanner -r .
trivy fs .
```

If scanning images and Manifests, supplement with corresponding image and deployment file scanning.

## 11. Pre-Release Gate Checklist

### 11.1 Must All Be Satisfied

- [ ] Current support boundary aligned with [Gateway API Support Matrix](../gateway-api-support.md)
- [ ] All `P0` test cases passed
- [ ] `P1` has no blockers; if there are unexecuted items, risk acceptance scope is explicitly defined
- [ ] No `Sev1/Sev2` unclosed defects
- [ ] Quick conformance passed
- [ ] `ALL_FEATURES=true` full-suite conformance passed
- [ ] Kind smoke passed
- [ ] Required specialized E2E passed:
  - [ ] `ReferenceGrant`
  - [ ] backend protocol selection
  - [ ] upstream behavior
  - [ ] session persistence
  - [ ] gateway cross-namespace certs
  - [ ] mesh frontend (if within this scope)
- [ ] At least one round of performance baseline completed
- [ ] At least one round of 24h soak completed
- [ ] Canary and rollback Runbook reviewed
- [ ] Control plane and data plane management interfaces accessible in target environment
- [ ] Monitoring, logging, and alerting rules ready

### 11.2 Standard Release Entry Point for the Current Repository

Default complete entry point:

```bash
./scripts/run-release-validation.sh
```

To supplement the mesh frontend automated validation not yet incorporated into the release baseline in the current repository, additionally run:

```bash
./tests/e2e/validate-mesh-frontends.sh
```

Archive report:

```bash
ARCHIVE_REPORT_ID=release-$(date +%Y%m%d%H%M%S) ./scripts/run-release-validation.sh
```

## 12. Canary Execution Checklist

### 12.1 Control Plane Canary

- [ ] canary `GatewayClass` created
- [ ] Only switch a few Gateways to canary class
- [ ] Observe `Accepted`, `Programmed`, `ResolvedRefs`
- [ ] Observe `/v1/summary`, `/v1/nodes`, `/v1/snapshot-sync`
- [ ] Confirm snapshot version advances stably

### 12.2 Data Plane Canary

- [ ] Use independent canary Gateway / Service / exposure address
- [ ] First accept synthetic traffic
- [ ] Then accept shadow traffic
- [ ] Then roll out at 1%/5%/10%/25%/50%/100%
- [ ] Retain a fixed observation window at each stage
- [ ] Record at each stage: 2xx/4xx/5xx, gRPC error, WS connection success rate, p95/p99, retry rate, FD, memory

### 12.3 Rollback Threshold

Upon reaching any of the following conditions, immediately stop rollout and trigger rollback:

- `5xx` significantly higher than baseline
- `p99` significantly higher than baseline
- `UNAVAILABLE` / `DEADLINE_EXCEEDED` significantly increased
- WebSocket connection success rate decreased
- `Gateway` / `Route` abnormal status increased
- reconcile error continuously rising
- memory or FD continuously leaking

### 12.4 Rollback Actions

- [ ] Switch ingress traffic weight back to stable
- [ ] Switch back canary `GatewayClass`
- [ ] Rollback canary dataplane image or scale down canary
- [ ] Preserve the scene; do not immediately delete the cluster

## 13. Failure Troubleshooting Checklist

When a cluster-level failure occurs, follow this fixed order:

1. `kubectl describe gateway` and related Route conditions
2. Control plane logs
3. Data plane logs
4. Control plane `/v1/summary`
5. Control plane `/v1/snapshot-sync`
6. Control plane `/v1/nodes`
7. Data plane `/v1/summary`
8. Data plane `/v1/snapshot`
9. Data plane `/v1/routes`
10. Data plane `/v1/backends`
11. Data plane `/metrics`
12. If multi-replica control plane, also check Lease

During troubleshooting, at minimum preserve:

- Failed command
- Control plane logs
- Data plane logs
- Management interface output
- Relevant Gateway/Route YAML
- If conformance failed, preserve `report.yaml` and `run.log`

## 14. Release Evidence Checklist

For each formal release, at minimum archive the following:

- conformance report
- Kind smoke and specialized E2E results
- Key performance charts or load test summary
- 24h soak results
- canary and rollback records
- Key management interface snapshots for control plane and data plane
- If anomalies occurred, preserve post-mortem conclusions and fix commits

After archiving is complete, additionally execute once:

```bash
./scripts/verify-release-evidence.sh \
  --candidate <candidate-commit> \
  --conformance reports/conformance/runs/<full-suite-id>/metadata.yaml \
  --performance reports/performance/runs/<a4-run-id>/metadata.txt \
  --chaos reports/chaos/runs/<fault-run-id>/metadata.txt \
  --soak reports/soak/runs/<soak-run-id>/metadata.txt
```

Only when this check passes can these archives be used as external evidence references for "current release candidate verified".
Among these, the performance archive must include full `throughput-report.json` coverage and source SLO `pass` conclusion; the chaos archive must include release gate and traffic SLO `pass` conclusions; the soak archive must be genuine long-term evidence with `duration_seconds>=86400` and traffic SLO `pass`.

## 15. Recommended Weekly Cadence

To truly operationalize this system, the following cadence is recommended:

### Week 1

- Complete support matrix and `P0/P1/P2` classification
- Fill in control plane and data plane unit tests up to the current change scope
- Establish the minimum closed loop for local integrated debugging

### Week 2

- Pass Kind smoke
- Pass `ReferenceGrant`, backend protocol, upstream behavior, session persistence specializations
- Run quick conformance

### Week 3

- Supplement performance baseline
- Supplement config hot reload and endpoint churn
- Supplement 24h soak
- Run one round of fault injection

### Week 4

- Prepare canary `GatewayClass`
- Walk through the release Runbook once
- Walk through the rollback Runbook once
- Do full-suite conformance and report archiving
