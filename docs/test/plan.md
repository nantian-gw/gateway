# Nantian Gateway Test Plan

This document is based on four drafts (`docs/archive/test/v1.md` through `v4.md`) and aims to consolidate these “broad-coverage but somewhat generic” materials into a formal test plan applicable to the current repository.

This plan defaults to the boundaries of the currently declared capabilities of the repository, focusing on:

- Control plane
- Data plane
- Control plane and data plane interaction path
- Gateway API semantics
- Management interfaces and observability
- Kind/E2E/Conformance
- Security, performance, stability, canary and rollback

The current repository includes the `dashboard/` management console; dashboard changes are verified separately using `cd dashboard && npm run check`. The main release validation in this document remains focused on controlplane, dataplane, Gateway API, Kind/E2E, conformance, and release evidence, and does not treat dashboard UI-specific testing as a core gateway release gate.

Recommended to use together with the following execution documents:

- [Test Case Matrix](./case-matrix.md)
- [Test Execution Checklist](./checklist.md)
- [Automation Status and Maintenance Rules](./automation-status.md)
- [Latest Automation Baseline Record](./latest-baseline.md)
- [Security Regression Execution Template](./security-regression.md)
- [Performance Baseline Execution Template](./performance-baseline.md)
- [Production Issue Regression Index](./regression-index.md)

To confirm which resources and features the current implementation actually claims to support, first see the [Gateway API Support Matrix](../gateway-api-support.md).
To view control plane and data plane management interfaces, refer to [Management Interfaces](../user/admin-api.md).
To use this plan for a release window, also follow [Production Operations](../user/operations.md) and the [Release, Canary, and Rollback Runbook](../user/release-runbook.md).

## 1. Analysis of `docs/test` Drafts

The four drafts have different value and should be absorbed separately rather than directly copied:

| Draft | Value | Current Limitations | Usage in the Formal Plan |
| --- | --- | --- | --- |
| `v1.md` | Provides the skeleton of a complete testing system, emphasizing that Conformance, controller behavior, data plane protocols, security, performance, and canary/rollback must all be covered | Too generic, not aligned with the repository's existing scripts, management interfaces, and verification costs | Used as the upper-level framework for test layering and coverage |
| `v2.md` | Provides a fairly complete test case catalog with `P0/P1/P2` priority thinking | Many test cases, but not aligned with the repository's currently supported scope and execution entry points | Used as the source for coverage matrix and priority design |
| `v3.md` | Suitable for breaking test cases into tables and execution checklists, convenient for later import into Feishu/Jira/Excel | More of a test management sheet, not a formal technical plan within the repository | Adopt its “split sheets by resource/protocol” organization approach |
| `v4.md` | Very complete on performance, capacity, soak, and canary stage testing | Still somewhat generic, lacking mapping to current Nantian Gateway management plane, Kind workflow, and release scripts | Used as the primary source for performance and release-specific plans |

Final conclusions:

- Materials in `docs/test` are suitable to keep as “brainstorm drafts” and “test checklist material”.
- The formal test plan must return to the current repository reality, clearly defining support boundaries, execution order, script entry points, pass criteria, and evidence retention.
- “Capabilities we want to support in the future” must not be mixed directly into the current release gate.

## 2. Plan Goals

The goal of this plan is not to list all possible testing ideas, but to provide an executable, layered, verifiable, and reusable validation system to answer the following questions:

1. What is the cheapest way to verify the current change.
2. Which capabilities must be covered in daily development.
3. Which capabilities must be covered before release.
4. How the control plane, data plane, and cross-plane interaction should be verified separately.
5. When Conformance or E2E fails, which management interfaces and runtime states should be checked synchronously.

## 3. Scope and Boundaries

### 3.1 Scope Covered by This Plan

- `controlplane/`: resource watching, translation, status write-back, snapshot generation, node ACK, and state persistence
- `dataplane/`: HTTP, gRPC, stream runtime, hot reload, upstream connection pool, backend TLS, management interfaces and metrics
- `proto/`: control plane and data plane shared protocol and compatibility
- `tests/e2e/`: Kind smoke and specialized E2E validation
- `tests/conformance/`: official Gateway API conformance
- `scripts/run-release-validation.sh`: release baseline validation and report archiving

### 3.2 Not Covered by This Plan by Default

- dashboard UI-specific testing; dashboard changes are verified separately per `dashboard/README.md` and `npm run check`
- features not explicitly declared as “implemented” in the [Gateway API Support Matrix](../gateway-api-support.md)
- capabilities that belong only to the future roadmap but currently have no code path or test evidence

### 3.3 Execution Principles

- Run the cheapest verification first, then proceed to more expensive tiers.
- Verify the layer closest to the change first, then run higher tiers.
- Do not default to starting Kind, rebuilding clusters, or running full-suite conformance after every change unless explicitly needed.
- Before release, must explicitly include `ALL_FEATURES=true ./tests/conformance/run.sh`.
- For any cluster-level failure, do not rely only on harness assertions; also check the control plane, data plane, and management interface states.

### 3.4 Current Automation Status

When maintaining this testing system, you must strictly separate content that is “already automated and directly runnable” from content that “still requires a dedicated environment”.

The current repository is recommended to be divided into 4 categories:

| Category | Current Status | Typical Content | Primary Entry Point |
| --- | --- | --- | --- |
| A1 Release-level one-click automation | Implemented | proto, control plane tests, data plane tests, Kind smoke, ReferenceGrant, backend protocol, upstream behavior, session persistence, HTTP security regression, full-suite conformance | `./scripts/run-release-validation.sh` |
| A2 In-repo automation but not merged into release baseline | Implemented | mesh frontend/service parent extended semantics | `./tests/e2e/validate-mesh-frontends.sh` |
| A3 Semi-automated, locally repeatable | Implemented | local integration testing, admin API cross-checking, log/metrics/admin evidence comparison, canary/rollback scripts | `go run`, `cargo run`, `curl`, canary scripts |
| A4 Non-functional tests requiring dedicated environments | Partially implemented | performance, soak, large-scale resource exhaustion, deep security testing, chaos, capacity and formal canary | load testing tools, staging environment, observability systems |

The maintenance approaches for these 4 categories differ:

- `A1` must remain in a “one command to rerun” state and serve as the priority release gate entry point.
- `A2`, although already automated, if not yet merged into the release baseline, must explicitly require supplementary runs in the documentation; the maintainer's memory cannot be relied upon by default.
- `A3` must clearly document which interfaces, logs, and metrics to check, otherwise it will degenerate into “try it manually”.
- `A4` must record environment prerequisites, acceptance thresholds, and evidence formats, not just keep test case titles.

## 4. Layered Testing Strategy

It is recommended to divide the current repository testing into 8 layers:

| Layer | Goal | Applicable Scenario | Primary Entry Point | Execute by Default |
| --- | --- | --- | --- | --- |
| `L0` Unit Tests | Pure logic correctness | Control plane translation, state computation, data plane matching, proto/IR | `go test ./...`, `cargo test --workspace` | Yes |
| `L1` Module-level Integration | Shared protocol and local runtime interaction | `proto/`, IR, snapshot, ACK, management plane aggregation | `make proto` + unit tests | Yes |
| `L2` Local Process Integration | Observe real management interfaces, snapshots, logs, and local port behavior | Management API, xDS, snapshot sync, reload | `go run ./cmd/manager`, `cargo run -p aeg-app` | On demand |
| `L3` Kind Smoke | Verify deployment manifests, Listener exposure, basic traffic paths | Deployment, images, Service, Gateway basic behavior | `./tests/e2e/run-kind.sh` | On demand |
| `L4` Specialized E2E | Verify specific Gateway API extended semantics | ReferenceGrant, BackendTLSPolicy, session persistence, backend protocol, mesh frontend | `tests/e2e/*.sh` | On demand |
| `L5` Conformance | Verify Gateway API specification semantics | Daily quick regression or final release gating | `./tests/conformance/run.sh` | Quick on demand, Full mandatory before release |
| `L6` Security/Performance/Stability | Verify production availability boundaries | Attack surface, capacity, soak, fault injection, configuration scale | Specialized scripts/load testing tools/observability dashboards | No, execute per milestone |
| `L7` Canary/Rollback Drill | Verify rollout process and fault recovery | Version canary, rollback, evidence archiving | `scripts/run-release-validation.sh`, canary scripts | Mandatory before release |

Recommended order:

1. Complete `L0` and `L1` first.
2. If the change involves management interfaces, snapshot sync, logs, or hot reload, then execute `L2`.
3. If the change involves deployment, Gateway Listener exposure, cross-namespace references, or real intra-cluster traffic, then execute `L3` and `L4`.
4. If the change involves Gateway API semantics or is preparing for release, then execute `L5`.
5. Supplement `L6` and `L7` before going live.

## 5. Coverage Matrix

### 5.1 Minimum Coverage Requirements by Module

| Module | P0 Must Test | P1 Pre-release Supplement | P2 Periodic Specialized |
| --- | --- | --- | --- |
| Shared Protocol & proto | Code generation, field compatibility, snapshot digest stability | Version rollback compatibility | Long-cycle compatibility regression |
| Control Plane | Translation, status write-back, Gateway/Route binding, ReferenceGrant, node ACK | Concurrent updates, status storms, Lease aggregation recovery | Large-scale object throughput |
| Data Plane | HTTP/gRPC/stream matching, backend forwarding, hot reload, management interfaces | Long-lived connections, upstream failover, backend TLS rotation | Limit capacity, memory/FD upper bound |
| Cross-Plane Interaction | Snapshot generation, distribution, ACK, ready, drift detection | Snapshot inheritance, duplicate snapshot deduplication | Multi-node convergence limits |
| Cluster-level Semantics | Kind smoke, ReferenceGrant, backend protocol selection, cross-namespace certs | Session persistence, mesh frontend | Larger-scale tenant concurrency |
| Gateway API Specification | Conformance within currently declared support scope | Quick profile high-frequency regression | Periodic full-suite baseline rerun |
| Security | Admin authentication, cross-namespace authorization, TLS/SAN validation | Request smuggling, header injection, slowloris | More systematic fuzz testing |
| Performance & Stability | HTTP/HTTPS/gRPC baseline, configuration hot reload, endpoint churn | 24h soak, connection pools, rolling upgrades | 72h soak, fault storms |
| Release Process | Controlplane/dataplane canary, rollback drill, report archiving | Rollout gate automation | Periodic release exercises |

### 5.2 Coverage Requirements by Resource and Capability

| Capability Domain | Key Resources/Semantics | Coverage Approach |
| --- | --- | --- |
| Basic Takeover | `GatewayClass`, `Gateway` | Control plane unit tests + Kind smoke + management interface checks |
| L7 Routing | `HTTPRoute`, `GRPCRoute` | Data plane unit tests + specialized E2E + conformance |
| Stream Routing | `TCPRoute`, `UDPRoute`, `TLSRoute` | Data plane module tests + Kind smoke + conformance within currently declared scope |
| Cross-namespace Authorization | `ReferenceGrant`, `certificateRefs`, backendRefs | Control plane state tests + specialized E2E |
| Backend Policy | `BackendTLSPolicy`, `BackendLBPolicy.sessionPersistence` | Control plane unit tests + data plane runtime tests + specialized E2E |
| Management Plane | `/livez`, `/readyz`, `/v1/*`, `/metrics` | Local process integration + Kind management interface validation |
| Control Plane State | `Accepted`, `ResolvedRefs`, `Programmed`, `observedGeneration` | Unit tests + cluster state checks |
| Data Plane State | Snapshot ready, reload stats, listener/route/backend lists | Local integration + management interface validation |
| Release & Operations | Kind caching, report archiving, canary GatewayClass, rollback | Release validation + runbook drill |

## 6. Detailed Test Plan

### 6.1 Shared Protocol, Snapshots, and Cross-Plane Foundational Path

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `XDS-001` | P0 | `L0/L1` | Proto generation and dual-end compilation compatibility | `proto/` field additions, enum extensions, optional field compatibility | `make proto`, control plane and data plane tests | Both ends generate code successfully; compilation passes; no missing fields |
| `XDS-002` | P0 | `L0/L1` | Snapshot ordering and digest stability | Repeated translation of same input, resource order changes, non-semantic updates | Control plane unit tests | Digest stable; no jitter; no repeated triggering of meaningless updates |
| `XDS-003` | P0 | `L1/L2` | Snapshot delivery, ACK, and ready path | Control plane generates snapshot, data plane receives, ACK, ready progression | Local process integration, management interfaces | Control plane `/v1/snapshot-sync` consistent with data plane `/v1/summary` |
| `XDS-004` | P1 | `L1/L2` | Snapshot inheritance and duplicate snapshot deduplication | Consecutive delivery of identical snapshots, partial updates, old state inheritance during hot reload | Data plane tests, local integration | Same snapshot does not trigger repeated reload; existing runtime state not incorrectly reset |
| `XDS-005` | P1 | `L2/L6` | Multi-node convergence and Lease aggregation | Multiple data plane nodes ACK, disconnect, recover, stale lease eviction | Control plane `/v1/nodes`, Lease check | `/v1/nodes` aggregate view correct; stale nodes not continuing to participate in ready statistics |

### 6.2 Control Plane Testing

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `CP-001` | P0 | `L0/L1` | `GatewayClass` takeover and ignore logic | Matching/non-matching `controllerName`, invalid parameters, repeated reconcile | `go test ./...` | Only takes over its own objects; status accurate; no unauthorized writes |
| `CP-002` | P0 | `L0/L1/L3` | `Gateway` listener translation and status | Listener add/delete/modify, port/protocol conflicts, invalid TLS secret, address unavailable | Control plane tests + Kind smoke | `Accepted`, `Programmed`, `ResolvedRefs` consistent with actual behavior |
| `CP-003` | P0 | `L0/L1/L3/L4` | Route attachment and cross-namespace handshake | `allowedRoutes`, `parentRefs`, namespace selector, `sectionName` | Control plane tests + Kind specialized E2E | Attachment relationships consistent with security model; recomputed after namespace changes |
| `CP-004` | P0 | `L0/L1/L4` | `ReferenceGrant` resolution and reclamation | Backend/service, certificateRefs, grant add/delete | `validate-reference-grants.sh`, `validate-gateway-cross-namespace-certs.sh`, `validate-grpc-reference-grants.sh` | Fails before authorization, succeeds after, becomes invalid after revocation; status updated synchronously |
| `CP-005` | P0 | `L0/L1` | `BackendTLSPolicy` and `BackendLBPolicy` translation | Target resolution, name conflicts, invalid references, partial valid reference retention | Control plane unit tests | `Accepted/ResolvedRefs` correct; valid portions continue to take effect |
| `CP-006` | P0 | `L0/L1/L2` | Node ACK and state persistence | ACK ordering, ready progression, Lease write failure retry | Control plane unit tests + `/v1/nodes` | Node state not lost; aggregate view stable |
| `CP-007` | P1 | `L0/L1/L6` | Concurrent updates and idempotency | High-frequency Gateway/Route updates, status storms, leader switch | Control plane tests + observability metrics | Eventual state consistent; queue can drain; no orphan resources |
| `CP-008` | P1 | `L2/L3` | Management interface resource view correctness | `/v1/summary`, `/v1/snapshot`, `/v1/listeners`, `/v1/routes`, `/v1/backends`, `/v1/nodes` | `curl` + `jq` | Interface fields, filter parameters, aggregate statistics consistent with cluster state |

### 6.3 Data Plane Testing

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `DP-001` | P0 | `L0/L1` | `HTTPRoute` match correctness | Hostname, wildcard, path exact/prefix, header, query, method | `cargo test --workspace` | Match results consistent with currently declared supported semantics |
| `DP-002` | P0 | `L0/L1/L4` | HTTP filter behavior | `RequestHeaderModifier`, `ResponseHeaderModifier`, `RequestRedirect`, `URLRewrite`, `RequestMirror`, `timeouts` | Data plane tests + HTTP E2E | Filters take effect; invalid combinations rejected; mirror does not affect primary request |
| `DP-003` | P0 | `L0/L1/L4` | `GRPCRoute` matching and forwarding | Service/method, metadata, unary, streaming, deadline, cancel, trailers | Data plane tests + gRPC E2E | Status codes, streams, metadata, cancel propagation correct |
| `DP-004` | P0 | `L0/L1/L3` | `TCPRoute`, `UDPRoute`, `TLSRoute` basic stream runtime | Weighted selection, listener routing, failure fallback | Data plane tests + Kind smoke | Match and forwarding correct; failed listener does not contaminate other paths |
| `DP-005` | P0 | `L1/L2/L4` | Upstream behavior and connection pool | Keepalive, retry, failover, weighted distribution, connect latency | `validate-upstream-behavior.sh` | Retry and failover follow policy; connection pool hit rate and metrics observable |
| `DP-006` | P0 | `L0/L1/L4` | Backend TLS validation | CA bundle, `Hostname`/`URI` SAN, mismatch failure | Data plane tests + specialized E2E | Valid certificates succeed; invalid certificates cannot be bypassed |
| `DP-007` | P1 | `L2/L3/L4` | Backend protocol selection | `appProtocol`, h2c, WebSocket, gRPC backend protocol selection | `validate-backend-protocols.sh` | Backend protocol selection consistent with Service definition |
| `DP-008` | P1 | `L0/L1/L4` | Session persistence | Sticky session token, shared secret, Secret file rotation without restart | `validate-session-persistence.sh` | Same session stably hits; after key rotation, pods begin signing with new key without restarting |
| `DP-009` | P0 | `L2` | Data plane management interfaces and metrics | `/livez`, `/readyz`, `/metrics`, `/v1/summary`, `/v1/node`, `/v1/snapshot` | `curl`, Prometheus scrape | Interface fields complete; metrics scrapable; auth configuration matches expectations |

### 6.4 Control Plane and Data Plane Local Integration

The goal of this layer is to confirm real process behavior in the cheapest way possible, without needing to start Kind immediately.

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `INT-001` | P0 | `L2` | Control plane local startup and basic health check | `go run ./cmd/manager -config ../configs/controlplane/config.yaml` | `curl /livez`, `/readyz`, `/v1/summary` | Process healthy; snapshot state matches current resource input |
| `INT-002` | P0 | `L2` | Data plane local startup and snapshot reception | `cargo run -p aeg-app -- --config ../configs/dataplane/config.yaml` | `curl /readyz`, `/v1/summary`, `/metrics` | Initial warming, ready after receiving snapshot; metrics grow normally |
| `INT-003` | P0 | `L2` | Management interface cross-referencing troubleshooting path | Compare control plane `/v1/snapshot-sync` with data plane `/v1/snapshot` | `curl` + `jq` | Resource counts, versions, listener/route/backend lists alignable between both sides |
| `INT-004` | P1 | `L2` | Hot reload visual verification | Modify Gateway/Route/Secret, observe reload, snapshot, ACK progression | Local process logs + management interfaces | Can locate hot reload and state issues without starting a cluster |

### 6.5 Kind Smoke and Specialized E2E

The Kind layer is only executed when real Kubernetes resources, Service exposure, and port paths are needed. Default preference is to reuse existing Kind clusters and local registry.

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `E2E-001` | P0 | `L3` | Basic Kind smoke | Gateway, HTTPRoute, GRPCRoute, TCPRoute, UDPRoute, TLSRoute smoke | `./tests/e2e/run-kind.sh` | Cluster deployment successful; basic HTTP/gRPC/TCP/UDP/TLS traffic reachable |
| `E2E-002` | P0 | `L4` | HTTP cross-namespace backend authorization | Fail without grant, succeed with grant, invalid after grant deletion | `./tests/e2e/validate-reference-grants.sh` | `ResolvedRefs` changes synchronously with actual forwarding |
| `E2E-003` | P0 | `L4` | `GRPCRoute` cross-namespace backend authorization | gRPC backendRef + `ReferenceGrant` | `./tests/e2e/validate-grpc-reference-grants.sh` | gRPC only succeeds after authorization |
| `E2E-004` | P0 | `L4` | `Gateway` cross-namespace certificate authorization | `certificateRefs` pointing to Secret in another namespace | `./tests/e2e/validate-gateway-cross-namespace-certs.sh` | Certificate reference only takes effect after authorization |
| `E2E-005` | P1 | `L4` | Backend protocol selection | `appProtocol` as h2c, WebSocket, or gRPC | `./tests/e2e/validate-backend-protocols.sh` | Actual forwarding protocol consistent with declaration |
| `E2E-006` | P1 | `L4` | Upstream behavior | Keepalive, retry, failover, weighted routing, management metrics | `./tests/e2e/validate-upstream-behavior.sh` | Request results and metrics simultaneously meet expectations |
| `E2E-007` | P1 | `L4` | Session persistence | HTTP/GRPC sticky behavior, shared secret, Secret file rotation without restart | `./tests/e2e/validate-session-persistence.sh` | Session stickiness stable; after rotation, pods sign cookies with new key without pod UID/restartCount changes |
| `E2E-008` | P1 | `L4` | Mesh frontend/service parent extended behavior | Frontend binding and exposure scope when mesh listener exists | `./tests/e2e/validate-mesh-frontends.sh` | Nodes that should not be exposed are not mistakenly added to frontend |
| `E2E-009` | P1 | `L4` | Admin token rotation without restart | Controlplane/dataplane `adminAuth.bearerTokenFile` Secret rotation | `./tests/e2e/validate-admin-token-rotation.sh` | Old token invalidated, new token active; pod UID/restartCount unchanged during rotation |
| `E2E-010` | P1 | `L4` | xDS mTLS rotation without restart | Controlplane gRPC TLS Secret, dataplane xDS TLS Secret, failure reconnect last-good | `./tests/e2e/validate-xds-mtls-rotation.sh` | Projected files updated after rotation; bad xDS TLS config triggers reconnect failure but preserves last-good; reconnects with new mTLS material after config recovery, pod UID/restartCount unchanged |
| `E2E-011` | P1 | `L4` | Gateway/backend TLS asset rotation without restart | Gateway TLS Secret, BackendTLSPolicy CA ConfigMap, Gateway backend client cert Secret | `./tests/e2e/validate-tls-asset-rotation.sh` | New downstream handshake presents rotated Gateway cert after rotation, upstream TLS uses rotated CA and rotated client cert, controlplane/dataplane pod UID and restartCount unchanged |

Kind execution recommendations:

- Basic smoke: `./tests/e2e/run-kind.sh`
- Reuse images and cluster: `SKIP_BUILD=true ./tests/e2e/run-kind.sh`
- Skip smoke, only prepare environment: `SKIP_SMOKE=true ./tests/e2e/run-kind.sh`
- Only execute when kind state is abnormal: `RECREATE_CLUSTER=true ./tests/e2e/run-kind.sh`

### 6.6 Gateway API Conformance

Conformance is used to prove whether “Gateway API semantics within the currently declared support scope” remain continuously correct.

| ID | Priority | Layer | Verification Target | Execution Entry Point | Pass Criteria |
| --- | --- | --- | --- | --- | --- |
| `CONF-001` | P0 | `L5` | Daily quick regression | `./tests/conformance/run.sh` | Current quick profile passes; suitable for daily semantic regression |
| `CONF-002` | P0 | `L5` | Pre-release full-suite | `ALL_FEATURES=true ./tests/conformance/run.sh` | Full-suite passes; no release blockers |
| `CONF-003` | P1 | `L5/L7` | Report archiving and version tracking | `ARCHIVE_REPORT_ID=<id> ./scripts/run-release-validation.sh` or `scripts/archive-conformance-report.sh` | Generate and archive `report.yaml`, `metadata.yaml`, `run.log` |

Minimum synchronous checks on Conformance failure:

1. `kubectl describe gateway` and related Route conditions.
2. Control plane logs.
3. Data plane logs.
4. Control plane `/v1/summary`, `/v1/snapshot-sync`, `/v1/nodes`.
5. Data plane `/v1/summary`, `/v1/snapshot`, `/v1/routes`.

### 6.7 Management Interfaces and Observability

Management interfaces are not ancillary features, but the foundation for all troubleshooting, integration, release, and rollback workflows.

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `OBS-001` | P0 | `L2/L3` | Health check interfaces | Control plane and data plane `/livez`, `/readyz` | `curl` | Returns reasonable state when not ready; returns to normal after ready |
| `OBS-002` | P0 | `L2/L3` | Summary interfaces | `/v1/summary`, `/v1/node`, `/v1/snapshot-sync` | `curl` + `jq` | Statistics, version numbers, ready state consistent with real runtime |
| `OBS-003` | P0 | `L2/L3` | Resource detail interfaces | `/v1/listeners`, `/v1/routes`, `/v1/backends`, `/v1/nodes` | `curl` + filter parameters | Filtering takes effect; objects consistent with control/data plane perspective |
| `OBS-004` | P0 | `L2/L3/L6` | Metrics exposure | Data plane `/metrics`, control plane independent metrics port | Prometheus scrape | Key metrics present and consistent with behavior |
| `OBS-005` | P1 | `L2/L6` | Log and metric alignment | Failed retries, connection pools, state anomalies, reload | Logs + metrics + admin API | Same issue can be cross-aligned across all three evidence types |

Key metrics recommended for ongoing attention:

- Control plane: reconcile latency, queue depth, error rate, status update rate, snapshot propagation latency
- Data plane: request rate, error rate, p95/p99, retry rate, failover success rate, upstream pool hit ratio, upstream connect latency, reload count, FD, memory, active connections

### 6.8 Security Testing

Security testing is organized into three layers: permissions and authorization, protocol security, resource exhaustion and operational surface exposure.

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `SEC-001` | P0 | `L2/L3` | Admin authentication | `/livez`, `/readyz` anonymously accessible, `/v1/*` and `/metrics` protected by Bearer Token | `curl` | Unauthorized access rejected; probe interfaces available |
| `SEC-002` | P0 | `L1/L4` | Cross-namespace authorization boundaries | BackendRef, certificateRef, frontendValidation CA, client cert ref | Control plane tests + ReferenceGrant specialized E2E | Never takes effect without grant; authorization scope does not spill over |
| `SEC-003` | P0 | `L1/L4/L6` | TLS and mTLS validation | Invalid CA, SAN mismatch, expired certificates, frontend mTLS | Data plane tests + `openssl` + specialized E2E | Handshake failure cannot be bypassed; legitimate traffic unaffected |
| `SEC-004` | P1 | `L6` | Request smuggling and message boundaries | `CL/TE`, `TE/CL`, malformed chunked, duplicate headers, CRLF injection | `./tests/e2e/validate-http-security.sh` + raw socket/fuzz scripts | No request mixing, no downstream contamination, no abnormal reuse triggered |
| `SEC-005` | P1 | `L6` | Header spoofing and tenant isolation | `Host`, `X-Forwarded-*`, hostname hijack, log sanitization | `./tests/e2e/validate-http-security.sh` + log checks | No unauthorized routing due to spoofed headers; logs do not leak sensitive info |
| `SEC-006` | P1 | `L6` | Connection exhaustion attacks | Slowloris, idle timeout, connection flood | `./tests/e2e/validate-http-security.sh` + rate-limited clients/load scripts | Normal traffic remains serviceable; FD and connection counts do not spiral |

### 6.9 Performance, Capacity, and Stability

The goal of performance testing is not to produce just a set of QPS numbers, but to determine the operable range, inflection points, and fault recovery behavior.

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `PERF-001` | P0 | `L6` | HTTP/HTTPS baseline | Steady-state QPS, p95/p99, TLS handshake latency | `fortio`, `wrk2`, `vegeta`, `h2load` | Error rate and tail latency meet SLA at target QPS |
| `PERF-002` | P0 | `L6` | gRPC baseline | Unary, streaming, deadline/cancel scenarios | `ghz` | Concurrent streams stable; error rate and p99 controllable |
| `PERF-003` | P0 | `L6` | Connection pool and upstream behavior | Keepalive, retry, failover, weight accuracy | Upstream behavior scripts + metrics | Pool hit ratio, connect latency, and failover success rate meet expectations |
| `PERF-004` | P0 | `L6` | Configuration hot reload and endpoint churn | Update Route/Secret/weight under sustained traffic, rolling restart backend Pods | Kind + load testing + admin API | Configuration propagation and recovery time within threshold; 5xx does not exceed gate |
| `PERF-005` | P0 | `L6` | Configuration scale testing | Large numbers of listeners, routes, backends, endpoints | Generation scripts + observability dashboards | Convergence time, memory, and CPU remain in acceptable ranges |
| `PERF-006` | P0 | `L6` | Soak and resource leaks | 24h steady-state traffic, long-lived connections, continuous log and metrics collection | Load testing tools + Prometheus | No linear memory/FD leaks; error rate does not continuously rise |
| `PERF-007` | P1 | `L6` | Limit capacity and saturation point | CPU, memory, FD, TLS handshake, connection limits | Stepped load increase | Provide recommended operating waterline and limit capacity |
| `PERF-008` | P1 | `L6` | Mixed workload | HTTP short connections + gRPC/WS long connections + large body | Multiple load testing tools combined | Core business traffic not significantly degraded by other types |
| `PERF-009` | P1 | `L6` | Control plane throughput | Reconcile latency, queue depth, status update storms | Batch resource changes + metrics | Queue can drain; status updates do not form sustained storms |

Recommended acceptance threshold template:

- Error rate `< 0.1%`
- `p99` `< 100ms / 200ms / 500ms`, set per business targets
- Configuration propagation time `< 5s`
- Rollback completion time `< 5min`
- No sustained linear memory or FD growth within 24h
- FD usage rate `< 70%` or within a clearly defined safe waterline
- Canary version performance degradation relative to stable version `< 10%`

### 6.10 Release, Canary, and Rollback

Release testing must clearly distinguish between the control plane and data plane; it is not recommended to treat both sides as a single unified entry point for a “one-step replacement release”.

| ID | Priority | Layer | Verification Target | Key Scenarios | Entry Point/Tool | Pass Criteria |
| --- | --- | --- | --- | --- | --- | --- |
| `REL-001` | P0 | `L7` | Pre-release baseline validation | Proto, control plane tests, data plane tests, Kind smoke, specialized E2E, conformance | `./scripts/run-release-validation.sh` | Baseline passes; reports and logs complete |
| `REL-002` | P0 | `L7` | Control plane canary | Canary `GatewayClass`, small number of Gateway switches, ACK and snapshot observation | `prepare-canary-gatewayclass.sh`, admin API | Canary control plane takes over correctly; state and snapshot versions progress stably |
| `REL-003` | P0 | `L7` | Data plane canary | Independent canary `Gateway`/Service/exposed address, first handle synthetic/shadow | Runbook + load testing/observability dashboards | Does not mix with stable version's same entry backend pool; metrics meet rollout gate |
| `REL-004` | P0 | `L7` | Rollback drill | Control plane switch back `GatewayClass`, data plane switch back to stable address/DNS/traffic weight | `rollback-canary-gatewayclass.sh` | Rollback completed within 5 minutes; business restored to baseline |
| `REL-005` | P0 | `L7` | Evidence archiving | Conformance, smoke, specialized E2E, performance charts, rollback records | `archive-conformance-report.sh`, release records | All key evidence traceable to version and time window |

### 6.11 Detailed Test Case Catalog

The previous sections provided a layered strategy and specialized themes, but to achieve “comprehensive coverage”, a finer-grained test case catalog is also needed. The following section absorbs most of the details from `docs/test/v1-v4` and remaps them to the current repository scope.

#### 6.11.1 `GatewayClass` / `Gateway` Basic Behavior

This set of test cases primarily verifies whether the control plane correctly takes over resources, generates status, and maintains infrastructure.

Must cover:

- Taken over when `controllerName` matches, completely ignored when it does not.
- Rejection behavior for `GatewayClass` with invalid `parametersRef`, missing objects, wrong `kind/group`, or invalid content.
- `Gateway` creation, update, deletion, repeated apply, and rapid consecutive updates.
- Listener addition, modification, deletion.
- Listener `hostname` conflicts, `port/protocol` conflicts, TLS listener invalid configuration.
- Coordination between `spec.addresses` and global `statusAddress/statusAddresses`.
- `spec.infrastructure.labels/annotations/parametersRef` propagation to downstream Service, EndpointSlice.
- Residual cleanup when Service falls back from `LoadBalancer/NodePort` to `ClusterIP`.
- Accuracy of `Accepted`, `ResolvedRefs`, `Programmed`, `observedGeneration`.
- Finalizer, deletion order, orphan resource cleanup, idempotency of repeated deletion.

Key observations:

- Whether status conditions are consistent with actual behavior, not just errors in logs.
- Whether invalid listeners fail locally rather than bringing down the entire Gateway.
- Out-of-scope objects are not incorrectly status-written.

#### 6.11.2 Route Attachment, Cross-Namespace Binding, and `ReferenceGrant`

This set of test cases is the core of the Gateway API security model and must cover the “handshake model” rather than just verifying whether traffic can pass through.

Must cover:

- Route and Gateway same-namespace binding.
- Route cross-namespace binding.
- `allowedRoutes.namespaces.from` with `Same`, `All`, `Selector`.
- Namespace selector match and non-match.
- `parentRefs.name`, `namespace`, `sectionName`, `port`.
- Pointing to a non-existent `Gateway` or non-existent listener.
- Whether binding relationships are recomputed after namespace label changes.
- Backend/certificate reference recomputation after `ReferenceGrant` addition, deletion, or change.
- Whether status messages leak the existence of cross-namespace resources that should not be exposed.

The current repository must focus on verifying three types of authorization:

- `HTTPRoute -> Service` cross-namespace `backendRef`
- `GRPCRoute -> Service` cross-namespace `backendRef`
- `Gateway listener -> Secret` cross-namespace `certificateRef`

Recommended tools and entry points:

- `./tests/e2e/validate-reference-grants.sh`
- `./tests/e2e/validate-grpc-reference-grants.sh`
- `./tests/e2e/validate-gateway-cross-namespace-certs.sh`

#### 6.11.3 `HTTPRoute` Detailed Coverage

The earlier `DP-001/002` only provided themes; here we complete a finer-grained HTTPRoute checklist.

Match semantics:

- Hostname exact match.
- Wildcard hostname.
- Empty hostnames behavior.
- Path `Exact`.
- Path `PathPrefix`.
- Header exact match.
- `queryParam` exact match.
- `method` matching.
- Multi-condition combination matching.
- Default matching and fallback path for “no rule matches”.

Priority and conflicts:

- More specific path takes precedence.
- Multiple HTTPRoute merging.
- Whether hostname/path conflict ordering is stable.
- Whether older route/name ordering is stable.
- Listener semantics when HTTPRoute coexists with other L7 Routes.

Backend behavior:

- Single backend.
- Multiple backend weighted distribution.
- `weight=0`.
- Backend Service does not exist.
- Backend port does not exist.
- Service has no endpoints.
- Cross-namespace backend with/without `ReferenceGrant`.

Filter behavior:

- `RequestHeaderModifier`
- `ResponseHeaderModifier`
- `RequestRedirect`
- `URLRewrite`
- `RequestMirror`
- `CORS`
- `timeouts`
- backend request header modification
- `ExtensionRef` currently declared supported `ConfigMap` carrier and implementation-specific filters
- `DirectResponse` if exposed via extension filter, supplement short-circuit return path

Special semantics:

- Redirect and rewrite cannot coexist in the same rule.
- HTTP -> HTTPS redirect.
- Order of host rewrite and path rewrite.
- Mirror does not affect primary request result.
- Behavior when timeout is zero or disabled.
- Execution order when filter and weighted backend appear together.

Observation points:

- Whether data plane `/v1/routes` correctly reflects routing and filter results.
- Whether access log, retry metrics, and traffic statistics in summary are consistent with request results.

#### 6.11.4 `GRPCRoute` Detailed Coverage

`GRPCRoute` must not be tested with just a single `grpcurl` request; long streams and cancel propagation must be covered separately.

Match semantics:

- Service matching.
- Method matching.
- Metadata/header matching.
- Hostname/authority matching.
- Multiple rule combination and priority.

Request types:

- Unary.
- Server streaming.
- Client streaming.
- Bidi streaming.

Protocol details:

- Metadata pass-through.
- Trailers pass-through.
- gRPC status code pass-through.
- Deadline timeout.
- Client cancel propagation.
- Backend disconnection.
- h2/h2c behavior.
- Large message bodies and high-concurrency streams.

Backend and authorization:

- Backend does not exist.
- Backend has no endpoints.
- Cross-namespace backend with/without `ReferenceGrant`.
- HTTPS/GRPCS upstream behavior when combined with `BackendTLSPolicy`.

#### 6.11.5 TLS / HTTPS / `BackendTLSPolicy` / `TLSRoute` / `TCPRoute` / `UDPRoute`

This section was broadly covered in the drafts, but previously only themes were kept without pulling out the details. Here we re-land it in three groups: frontend TLS, backend TLS, and stream routes.

Frontend TLS:

- HTTPS terminate.
- SNI certificate selection.
- Wildcard cert.
- Secret hot reload.
- Invalid certificate.
- Expired certificate.
- Cert/key mismatch.
- Listener boundaries when multiple certificates coexist.
- `frontendValidation` client CA, cross-namespace `ConfigMap` references, and `ReferenceGrant`.
- Frontend mTLS success and failure paths.

Backend TLS:

- System CA.
- Explicit CA bundle.
- `Hostname` SAN validation.
- `URI` SAN validation.
- SAN mismatch failure.
- Backend certificate invalid.
- Backend TLS version upper/lower bounds.
- Client certificate to backend reference missing, content missing, authorization missing.

`TLSRoute`:

- Passthrough.
- SNI routing.
- Coexistence with regular HTTPS listener.
- Binding listener and `sectionName` correspondence.
- Backend weighted selection.

`TCPRoute`:

- Multiple TCP listeners.
- Port isolation.
- Backend switch and failure fallback.
- Long-lived connection stability.
- Half-close, abnormal disconnect, backend rejection.

`UDPRoute`:

- Basic forwarding.
- Multiple listeners/port isolation.
- Empty backend or failed backend scenarios.
- DNS-type scenario request/response reachability.

#### 6.11.6 Backend Protocol Selection

This is a common Gateway API official test area and a capability already supported by existing repository scripts.

Must cover:

- `ServicePort.appProtocol` not configured.
- `appProtocol: kubernetes.io/h2c`
- `appProtocol: kubernetes.io/ws`
- h2c prior knowledge.
- h1/h2 coexistence.
- WebSocket upgrade success path.
- WebSocket upgrade failure path.
- gRPC backend protocol selection.

Recommended entry point:

```bash
./tests/e2e/validate-backend-protocols.sh
```

Simultaneously verify:

- Actual request results
- Backend protocol mapping reported by data plane admin API

#### 6.11.7 Rust Proxy Request Lifecycle Testing

This is the most easily overlooked part of the four drafts, but also the most valuable for the current project.

The current repository's HTTP data plane implementation is based on the Rust proxy `ProxyHttp`, with the main logic located in:

- `dataplane/crates/aeg-http/src/proxy.rs`
- `dataplane/crates/aeg-http/src/filters.rs`
- `dataplane/crates/aeg-http/src/proxy/backend.rs`

Therefore testing must not be organized only by “Gateway API resources”, but also by request lifecycle.

Stages that the current repository has actually overridden or explicitly participates in:

- `request_filter`
- `request_body_filter`
- `upstream_peer`
- `connected_to_upstream`
- `upstream_request_filter`
- `response_filter`
- `logging`
- `fail_to_proxy`
- `fail_to_connect`

Although the current `GatewayProxy` does not implement custom `early_request_filter`, `upstream_response_filter`, `response_body_filter`, these stages are still affected by the Rust proxy stack and protocol path, so their external behavior must still be verified through protocol-level E2E.

Recommended to design tests by stage:

| Stage | Must-Test Path | Key Risk |
| --- | --- | --- |
| `request_filter` | Route selection, direct response, redirect, mirror, session persistence parsing | Rule selection errors, invalid filters not rejected, short-circuit return errors |
| `request_body_filter` | Body forwarding to mirror, slow upload, large body | Body loss, mirror corruption, incorrect backpressure |
| `upstream_peer` | Backend selection, reselection before retry, TLS peer construction | Wrong backend, TLS parameter errors, connection pool failure |
| `connected_to_upstream` | Connection reuse, connect latency statistics | Pool hit stats inaccurate, reuse anomalies |
| `upstream_request_filter` | Header/path/host rewrite | Rewrite order errors, header contamination |
| `response_filter` | Response header modify, CORS, sticky session token write-back, retryable status handling | Response header errors, retry misjudgment, token instability |
| `logging` | Access logs, traffic metrics, latency statistics, response flag | Missing log fields, metrics inconsistent with reality |
| `fail_to_proxy` | 5xx error return, response flag | Wrong error codes, no diagnosable error returned downstream |
| `fail_to_connect` | Retry, failover, connect failure statistics | No retry after failure, missing connect latency stats |

Protocol boundaries should also be verified specifically against Rust proxy runtime capabilities:

- HTTP/1.1: keepalive, chunked body, `Content-Length`, trailers, `Expect: 100-continue`
- HTTP/2: concurrent streams, GOAWAY, RST, header size boundaries, flow control, backpressure
- WebSocket: upgrade, large messages, idle timeout, backend/client close
- gRPC: long streams, cancel propagation, deadline, backend disconnection

#### 6.11.8 Connection Pool, Retry, Load Balancing, and Recovery

The earlier `DP-005` and `PERF-003` only provided themes; here we complete the specific check items.

Must cover:

- Upstream keepalive reuse.
- Connection pool upper limit and idle reclamation.
- Backend removal and recovery.
- Retry count, conditions, and backoff.
- Whether peer is reselected on failover.
- Impact scope when a backend is slow, times out, or refuses connections.
- Weighted distribution accuracy.
- Convergence time after weight changes.
- Connection re-establishment after DNS or EndpointSlice changes.

Key metrics:

- upstream connect latency
- upstream pool hit ratio
- retry rate
- failover success rate
- backend imbalance
- Dataplane reload time

Recommended entry point:

```bash
./tests/e2e/validate-upstream-behavior.sh
```

This type of testing must simultaneously verify three types of evidence:

- Request results
- Metrics
- Admin API summary/traffic statistics

#### 6.11.9 Hot Reload, Graceful Restart, and Zero Downtime

Must cover:

- Listener configuration changes.
- Route batch updates.
- Secret updates.
- Deployment rolling update.
- Gateway dataplane pod restart.
- Controller restart.
- Updates while long-lived connections, WebSocket, gRPC streams exist.

Pass criteria:

- No connection loss or only within agreed thresholds.
- No widespread `502/503`.
- Configuration switch latency quantifiable.
- No prolonged `p99` spikes during reload.

#### 6.11.10 Controller / Reconciler Testing

Many control plane issues do not manifest in the traffic path, but in resource lifecycle and state convergence.

Must cover:

- Behavior when informer/cache is not synced.
- Reconcile idempotency.
- Rapid consecutive updates.
- Large-scale Route churn.
- Finalizer.
- Deletion order.
- Orphan cleanup.
- Leader election switch.
- Multi-replica controller consistency.
- Status update storms.
- Out-of-scope objects should not have status written.

Key metrics:

- Reconcile latency
- Queue depth
- Error rate
- Requeue count
- Status update success rate

#### 6.11.11 Security Testing Supplement

In addition to the runtime security tests listed above, supply chain and configuration security must also be explicitly supplemented.

Supply chain and dependencies:

- Rust dependency vulnerability scanning.
- Go dependency vulnerability scanning.
- Container image scanning.
- SBOM generation and retention.
- Manifest security scanning.

Recommended tools:

- `cargo audit`
- `osv-scanner`
- `trivy` or `grype`
- `kubescape` or `kubeaudit`

Protocol and cache regression:

- Request smuggling: `CL/TE`, `TE/CL`
- Malformed chunked body
- Duplicate header
- Header injection / CRLF
- Oversized headers
- `Host`/`X-Forwarded-*` spoofing
- Smuggling regression on cache hit path
- Cache poisoning

Notes:

- The current repository does not assume “caching capability is online” by default, but if Rust proxy cache or a custom cache layer is enabled, this set of tests should immediately be elevated to P0.

#### 6.11.12 Performance, Capacity, and Fault Injection Supplement

Performance testing must not only look at HTTP QPS, but must simultaneously cover “scale + configuration propagation + tail latency + stability”.

North-south traffic dimensions:

- HTTP QPS
- HTTPS QPS
- gRPC QPS / streams
- WebSocket concurrent connections

Latency dimensions:

- `p50/p90/p99/p999`
- Latency jitter during configuration changes
- Tail latency when backend slows down

Resource dimensions:

- CPU
- Memory
- FD
- Socket
- Goroutine / thread
- Rust heap growth
- Kube-controller resource usage

Control plane scale dimensions:

- GatewayClass count
- Gateway count
- Listener count
- HTTPRoute count
- Rule count
- BackendRefs count
- Namespace count
- Cross-namespace refs count

Data plane scale dimensions:

- Concurrent connections
- Concurrent h2 streams
- Long-lived connections
- TLS handshake rate
- Backend count
- Endpoint change frequency

Fault injection dimensions:

- Backend timeout
- Backend reset
- Backend 5xx
- Backend connection refused
- Backend TLS handshake failure
- All backend pods down
- Only partial endpoint anomalies
- Gateway pod kill
- Node drain
- Pod eviction
- OOM restart
- Readiness fail
- Rolling update
- Controller restart
- Leader switch
- Apiserver temporarily unavailable
- Watch disconnect and reconnect
- Etcd latency
- Packet loss, latency, jitter, DNS failure, NetworkPolicy impact

Recommended tools:

- `fortio`
- `vegeta`
- `wrk2`
- `k6`
- `h2load`
- `ghz`
- `websocat`
- `tc/netem`
- `Chaos Mesh`

#### 6.11.13 Test Environments and Tool Stack

To make execution practical, it is recommended to fix the test environments into four categories:

| Environment | Goal | Typical Content |
| --- | --- | --- |
| Local dev environment | Lowest cost verification | Unit tests, module tests, local process integration |
| Local Kind environment | Kubernetes semantics and specialized E2E | `run-kind.sh`, specialized E2E, quick conformance |
| Staging environment | Near-production domains, certificates, network policies | Soak, fault injection, canary drill |
| Release window environment | Final gating | Full-suite conformance, canary, rollback |

Recommended tool stack:

- Gateway API: official conformance harness
- Cluster: `kind`
- HTTP: `curl`, `fortio`, `vegeta`, `wrk2`, `k6`
- HTTP/2 / TLS: `h2load`, `nghttp`, `openssl s_client`
- gRPC: `grpcurl`, `ghz`
- WebSocket: `websocat`
- Rust: `cargo test`, `cargo nextest`, `cargo llvm-cov`, `proptest`, `cargo fuzz`
- Debugging: `tcpdump`, `wireshark`, `kubectl describe`

#### 6.11.14 Deliverables and Execution Evidence

To make this plan truly become “pre-launch testing assets”, the following artifacts should at minimum be produced:

1. Support matrix.
2. Feature checklist.
3. Conformance execution scripts and report archiving.
4. E2E test case catalog.
5. Performance load testing scripts and result summaries.
6. Fault injection scripts or execution records.
7. Canary release Runbook.
8. Rollback Runbook.
9. Monitoring dashboards and alerting rules.
10. Final launch acceptance report.

## 7. Execution Cadence and Release Gates

### 7.1 Daily Development

Applicable scenarios:

- Pure control plane logic changes
- Pure data plane logic changes
- Proto/IR interaction changes

Recommended execution:

```bash
make test-targeted
```

Or execute by module:

```bash
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

If the change involves shared protocol or interaction:

```bash
make proto
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

### 7.2 Before Merge

Supplement on top of daily development:

- Local process integration, at least cross-check management interfaces once
- If involving cluster semantics, supplement with one Kind smoke or corresponding specialized E2E

Recommended entry point:

```bash
PLAN_ONLY=true ./scripts/run-targeted-validation.sh
```

Output requirements:

- Each selected verification command must include the reason for selection, so reviewers can judge whether the verification level is sufficient.
- High-cost verifications skipped by default such as Kind smoke and metrics surface must appear in the `skipped validations` block with instructions on how to enable them.
- If the reviewer believes a skipped item is relevant to the current change, they should rerun the plan with `INCLUDE_KIND=true` or directly execute the corresponding specialized script.

### 7.3 Before Release

Must execute:

```bash
./scripts/run-release-validation.sh
```

If report archiving is needed:

```bash
ARCHIVE_REPORT_ID=local-$(date +%Y%m%d%H%M%S) ./scripts/run-release-validation.sh
```

The default release workflow executes serially:

- Proto generation
- Control plane tests
- Data plane tests
- Kind smoke
- Backend protocol selection testing
- Gateway cross-namespace certs testing
- gRPC ReferenceGrant testing
- HTTP ReferenceGrant testing
- Upstream behavior testing
- Session persistence testing
- HTTP security regression testing
- `ALL_FEATURES=true` conformance

Only if the current change is unrelated to a specific item can that step be explicitly skipped.

### 7.4 Periodic Specialized Testing

Recommended to supplement weekly or per release milestone:

- 24h soak
- Configuration scale testing
- Endpoint churn
- `./tests/e2e/validate-http-security.sh`
- Slow body / large-scale slowloris / header injection deep dive
- Canary rollout and rollback drill

### 7.5 Recommended Advancement Order

To advance from “currently having a bunch of test ideas” to truly “forming a launch closed loop”, execute in the following order:

1. Clarify the support matrix and current release boundaries, breaking features into `P0/P1/P2`.
2. First supplement unit/module tests for control plane translation, status conditions, snapshots, and ACK.
3. Then supplement specialized E2E for HTTPRoute, GRPCRoute, TLS/stream, ReferenceGrant, BackendTLSPolicy.
4. Then run quick conformance to ensure daily Gateway API semantic regression is reproducible.
5. Supplement performance, fault injection, soak, and hot reload testing in Kind or staging environment.
6. Before release, run full-suite conformance, canary, rollback drill, and evidence archiving.

Not recommended order:

- Running full conformance before features are finalized.
- Defaulting to starting Kind every time without unit tests and local integration.
- Doing large-scale load testing or production canary without specialized E2E.

## 8. Troubleshooting and Evidence Retention Requirements

### 8.1 Fixed Troubleshooting Order for Cluster-Level Failures

1. `kubectl describe gateway` and Route conditions.
2. Control plane logs.
3. Data plane logs.
4. Control plane `/v1/summary`, `/v1/snapshot-sync`, `/v1/listeners`, `/v1/routes`, `/v1/backends`, `/v1/nodes`.
5. Data plane `/v1/summary`, `/v1/snapshot`, `/v1/listeners`, `/v1/routes`, `/v1/backends`, `/metrics`.
6. If multi-replica control plane, also check `Lease` aggregation state.

### 8.2 Minimum Retention per Formal Release

- Conformance report for the current commit
- Kind smoke and specialized E2E results
- Key performance charts or load test summaries
- Rollback steps and actual rollback duration
- Control plane and data plane key management interface snapshots
- If anomalies exist, retain post-mortem conclusions and fix commits

## 9. Minimum Release Blocking Checklist

If time is limited, the following items must still be covered:

- `XDS-001` through `XDS-003`
- `CP-001` through `CP-006`
- `DP-001` through `DP-006`
- `INT-001` through `INT-003`
- `E2E-001` through `E2E-004`
- `CONF-002`
- `OBS-001` through `OBS-004`
- `SEC-001` through `SEC-003`
- `PERF-001` through `PERF-006`
- `REL-001` through `REL-005`

## 10. One-Sentence Execution Principle

Use the support matrix as the boundary, prioritize by lowest verification cost, and use management interfaces and report archiving as release evidence.
