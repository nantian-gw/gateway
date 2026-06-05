# Nantian Gateway Test Case Matrix

This document is the execution matrix version of the [Test Plan](./plan.md), used to turn the strategy document into an executable case catalog.

Scope:

- Control plane
- Data plane
- Control plane and data plane linkage paths
- Gateway API semantics
- Management interfaces and observability
- Kind / E2E / Conformance
- Security, performance, stability, canary and rollback

The current repository includes the `dashboard/` management console. This matrix covers the core gateway paths by default; dashboard UI changes should run `cd dashboard && npm run check` separately per `dashboard/README.md`, and are not implicitly covered by `./scripts/run-release-validation.sh`.

## 1. Field Descriptions

| Field | Meaning |
| --- | --- |
| `ID` | Stable test case ID, for easy mapping to Feishu/Jira/CI |
| `Priority` | `P0` release-blocking; `P1` should be completed before launch; `P2` specialized or periodic validation |
| `Tier` | `L0-L7`, defined in [Test Plan](./plan.md) |
| `Scenario` | The target verified by this test case |
| `Prerequisites` | Environment, objects, scripts, or configuration to prepare |
| `Steps` | Recommended minimum execution steps |
| `Expected Result` | Pass criteria |
| `Automation Entry` | Existing scripts, commands, or suggested tools in the current repository |

## 2. Priority Determination

- `P0`: Release-blocking; must have automation or a stable reproduction method.
- `P1`: Should be covered in pre-release; allowed to be completed in phases.
- `P2`: Specialized enhancement, periodic, or high-cost validation.

## 3. Test Case Matrix

### 3.1 Shared Protocols, Snapshots, and Cross-Plane Linkage

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `XDS-001` | P0 | `L0/L1` | proto generation and dual-end compilation compatibility | Local environment has `make`, `go`, `cargo` | Execute `make proto`, then compile/test controlplane and dataplane separately | Generated code succeeds; no missing fields; dual-end tests pass | `make proto`, `cd controlplane && go test ./...`, `cargo test --manifest-path dataplane/Cargo.toml --workspace` |
| `XDS-002` | P0 | `L0/L1` | snapshot ordering and digest stability | Control plane unit tests runnable | Repeatedly translate same input; swap object input order; repeatedly update same-value spec | digest does not fluctuate; no new snapshots generated for non-semantic changes | Control plane unit tests |
| `XDS-003` | P0 | `L1/L2` | snapshot delivery, ACK, ready advancement | Can start controlplane/dataplane locally | Start local processes; create basic Gateway/Route; check `/v1/snapshot-sync` and `/v1/summary` | Control plane snapshot generation; data plane receives and ACKs; ready advancement consistent | `go run ./cmd/manager`, `cargo run -p aeg-app`, `curl` |
| `XDS-004` | P1 | `L1/L2` | duplicate snapshot deduplication | Data plane supports reload metrics | Deliver same snapshot multiple times; observe reload stats and logs | Same snapshot does not trigger repeated reload; status not incorrectly reset | Data plane unit tests, local integrated debugging, `/metrics` |
| `XDS-005` | P1 | `L1/L2` | runtime state inheritance during snapshot switch | Scenarios with sticky session, weighted routing, or traffic stats | Update Route/weight during sustained traffic; observe behavior before and after snapshot switch | Sessions, weights, and runtime state transition smoothly; no obvious resets | Data plane unit tests, specialized E2E |
| `XDS-006` | P1 | `L2/L6` | multi-node ACK, disconnection, and Lease aggregation | Control plane multi-replica or simulated multi-node | Connect/disconnect/recover multiple data planes; check `/v1/nodes` and Lease | Active node stats, ready/connected/ACK status correct; stale nodes evicted | `/v1/nodes`, `kubectl get lease` |

### 3.2 `GatewayClass` / `Gateway` Basic Behavior

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `CP-001` | P0 | `L0/L1` | Only take over `GatewayClass` matching `controllerName` | Gateway API CRDs installed | Create `GatewayClass/Gateway` with matching and non-matching `controllerName` | Only take over matching objects; will not mis-handle other controller objects | `cd controlplane && go test ./...` |
| `CP-002` | P0 | `L0/L1` | valid `parametersRef` configuration takes effect | Prepare valid `ConfigMap` | Create `GatewayClass` or `Gateway.infrastructure.parametersRef` with valid `parametersRef` | Configuration correctly read and propagated to infrastructure resources | Control plane unit tests |
| `CP-003` | P0 | `L0/L1` | missing or invalid `parametersRef` rejected | Prepare non-existent object, wrong type, invalid YAML | Create several illegal `parametersRef` scenarios separately | `Accepted=False` or corresponding status accurate; no erroneous infrastructure generated | Control plane unit tests |
| `CP-004` | P0 | `L0/L1/L3` | `Gateway` listener add/delete/modify | Valid `GatewayClass` exists | Create Gateway, then add, modify, delete listeners in sequence | Configuration eventually consistent; old listeners cleaned up; new listeners take effect | Control plane unit tests, `./tests/e2e/run-kind.sh` |
| `CP-005` | P0 | `L0/L1/L3` | listener conflict handling | Prepare port conflicts, protocol conflicts, invalid TLS Secret | Create conflicting listeners and erroneous HTTPS listeners | Conflicting listeners rejected; other valid listeners unaffected | Control plane unit tests, Kind smoke |
| `CP-006` | P0 | `L0/L1` | `spec.addresses` and `statusAddress/statusAddresses` coordination | Configure multiple programmable addresses | Create Gateway with explicit `spec.addresses`; observe status | `Programmed` consistent with address availability; unavailable addresses given clear reason | Control plane unit tests |
| `CP-007` | P0 | `L0/L1/L3` | `spec.infrastructure` propagation | Gateway configured with labels/annotations/parametersRef | Create Gateway and inspect corresponding Service/EndpointSlice | Labels, annotations, Service parameters correctly propagated | Control plane unit tests, `kubectl get svc/endpointslice -o yaml` |
| `CP-008` | P0 | `L0/L1` | `observedGeneration` and conditions correctly advance | Can continuously modify Gateway spec | Continuously modify Gateway; read status each time | `observedGeneration` follows latest spec; conditions match real state | Control plane unit tests |
| `CP-009` | P0 | `L0/L1` | deletion cleanup and finalizer | Gateway and downstream resources created | Delete Gateway/GatewayClass; observe finalizer and resource cleanup | No orphan resources; will not be stuck terminating | Control plane unit tests |
| `CP-010` | P1 | `L1/L6` | multi-replica control plane consistency | Controlplane multi-replica deployment | Observe status and infrastructure during updates and restarts | Consistent results across replicas; no duplicate generation or mutual overwriting | Kind/pre-production environment specialization |

### 3.3 Route Attachment, Cross-Namespace Binding, and `ReferenceGrant`

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ATT-001` | P0 | `L1/L3` | same-namespace Route successfully binds to Gateway | Gateway and Route in same namespace | Create `HTTPRoute.parentRefs` pointing to Gateway, send requests | Route attach succeeds; traffic hits backend | Control plane unit tests, Kind smoke |
| `ATT-002` | P0 | `L1/L3` | `allowedRoutes` `Same/All/Selector` behavior | Prepare multiple namespaces and labels | Test `Same`, `All`, `Selector` separately, create cross-namespace Routes | Only allowed namespaces can bind | Control plane unit tests, specialized E2E |
| `ATT-003` | P0 | `L1/L3` | `sectionName`, `port`, selector mismatch rejects binding | Gateway has multiple listeners | Route specifies wrong `sectionName/port` or selector does not match | Route does not attach; status gives clear reason | Control plane unit tests, Kind E2E |
| `ATT-004` | P0 | `L1` | `parentRefs` points to non-existent Gateway or listener | Parent object does not exist | Create Route with erroneous `parentRefs` | Does not attach; controller does not panic; does not mis-write other object states | Control plane unit tests |
| `ATT-005` | P0 | `L1/L4` | `HTTPRoute` cross-namespace backendRef requires `ReferenceGrant` | Backend Service in different namespace | First verify failure without grant, then create `ReferenceGrant`, finally delete grant | Fails before authorization, succeeds after, invalidated promptly after deletion | `./tests/e2e/validate-reference-grants.sh` |
| `ATT-006` | P0 | `L1/L4` | `GRPCRoute` cross-namespace backendRef requires `ReferenceGrant` | gRPC backend in different namespace | Repeatedly execute no-grant / with-grant / delete-grant flow | gRPC only succeeds after authorization | `./tests/e2e/validate-grpc-reference-grants.sh` |
| `ATT-007` | P0 | `L1/L4` | `Gateway listener -> Secret` cross-namespace certificate authorization | TLS Secret in different namespace | Repeatedly execute no-grant / with-grant / delete-grant flow | Certificate reference consistent with `ResolvedRefs/Programmed` status | `./tests/e2e/validate-gateway-cross-namespace-certs.sh` |
| `ATT-008` | P1 | `L1/L3` | namespace label change triggers binding recomputation | `allowedRoutes.from=Selector` | Modify namespace label to change Route from allowed to disallowed, then restore | Binding relationships and traffic behavior recomputed on label change | Control plane unit tests, Kind E2E |
| `ATT-009` | P1 | `L1` | status message does not leak existence of unauthorized targets | Construct cross-namespace unauthorized references | Read Route/Gateway status message | Error messages are diagnosable but do not leak object details that should not be exposed | Control plane unit tests |

### 3.4 `HTTPRoute` Detailed Matrix

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `HTTP-001` | P0 | `L0/L4` | hostname exact match | Deploy echo backend | Configure exact hostname rule, access with matching and non-matching domains | Only target domain hits the rule | Data plane unit tests, HTTP E2E |
| `HTTP-002` | P1 | `L0/L4` | wildcard hostname | Listener supports wildcard scenarios | Configure `*.example.com`, access subdomain and root domain separately | Subdomain hits; root domain behavior matches declaration | Data plane unit tests, HTTP E2E |
| `HTTP-003` | P0 | `L0/L4` | path `Exact` match | Echo backend can echo path | Configure exact route, access exact and non-exact paths | Only exact path hits | Data plane unit tests, HTTP E2E |
| `HTTP-004` | P0 | `L0/L4` | path `PathPrefix` match and priority | Configure multiple prefix rules | Access multiple hierarchical paths | More specific rule hits first | Data plane unit tests, HTTP E2E |
| `HTTP-005` | P1 | `L0/L4` | header / query / method matching | Fields declared as supported | Construct requests that satisfy and do not satisfy conditions separately | Only requests meeting conditions hit | Data plane unit tests, HTTP E2E |
| `HTTP-006` | P0 | `L1/L4` | multi-HTTPRoute merge and conflict ordering | Multiple routes under same listener | Create conflicting and non-conflicting rules and issue concurrent requests | Merge stable; conflict ordering predictable | Control plane unit tests, HTTP E2E |
| `HTTP-007` | P0 | `L4` | single backend forwarding | Backend normal | Create single-backend Route and issue requests | 100% hit target backend | HTTP E2E |
| `HTTP-008` | P0 | `L4/L6` | multi-backend weighted distribution | Both backends available | Configure 90/10, 80/20 etc. weights and load test | Distribution ratios close to configuration; deviation within threshold | HTTP E2E, load testing tools |
| `HTTP-009` | P0 | `L4` | `weight=0` behavior | Both backends available | Set one weight to 0 and send requests | `weight=0` backend receives no traffic | HTTP E2E |
| `HTTP-010` | P0 | `L1/L4` | backend Service/port non-existent or no endpoints | Construct erroneous Service, wrong port, empty endpoints | Send requests one by one | Status correct; request fails but no mis-forwarding | Control plane unit tests, HTTP E2E |
| `HTTP-011` | P1 | `L0/L4` | `RequestHeaderModifier` | Backend can echo request headers | Configure add/delete/modify request headers and send requests | Upstream-received request headers match expectations | Data plane unit tests, HTTP E2E |
| `HTTP-012` | P1 | `L0/L4` | `ResponseHeaderModifier` | Backend can set fixed response headers | Configure add/delete/modify response headers and send requests | Client-received response headers match expectations | Data plane unit tests, HTTP E2E |
| `HTTP-013` | P0 | `L0/L4` | `RequestRedirect` | HTTP listener exists | Configure 301/302/308 redirect | Returns correct status code and `Location` | Data plane unit tests, HTTP E2E |
| `HTTP-014` | P0 | `L0/L4` | HTTP -> HTTPS redirect | Both HTTP/HTTPS listeners exist | Access redirect rule via HTTP | Correctly redirects to HTTPS target | Data plane unit tests, HTTP E2E |
| `HTTP-015` | P0 | `L0/L4` | `URLRewrite` | Backend can echo path/host | Configure path/host rewrite | Backend receives rewritten values | Data plane unit tests, HTTP E2E |
| `HTTP-016` | P0 | `L0/L4` | rewrite and redirect conflict | Route contains both filter types | Same rule configured with rewrite + redirect | Invalid configuration rejected or clearly errored | Data plane unit tests, control plane unit tests |
| `HTTP-017` | P1 | `L0/L4` | `RequestMirror` | Primary and mirror backends both observable | Configure mirror and send requests | Primary request normal; mirror receives copy; does not affect primary response | Data plane unit tests, HTTP E2E |
| `HTTP-018` | P1 | `L0/L4` | `CORS` | Client supporting preflight requests | Send preflight and formal requests | Only injects correct CORS response headers when origin matches | Data plane unit tests, HTTP E2E |
| `HTTP-019` | P1 | `L0/L4` | `timeouts` | Backend can simulate slow responses | Configure route/backend timeout, send slow requests | Timeout triggers per configuration; does not affect normal requests | Data plane unit tests, HTTP E2E |
| `HTTP-020` | P1 | `L0/L4` | `ExtensionRef`/`DirectResponse` currently supported subset | `ConfigMap` extensions supported by current repo configured | Verify direct response, header modify, mirror, redirect, rewrite | Only declared supported extensions take effect; unsupported ones rejected | Control plane unit tests, data plane unit tests, HTTP E2E |

### 3.5 `GRPCRoute` Detailed Matrix

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `GRPC-001` | P0 | `L0/L4` | service matching | Deploy gRPC echo backend | Configure service-based matching rules and call | Only target service hits | Data plane unit tests, `grpcurl`/`ghz` |
| `GRPC-002` | P0 | `L0/L4` | method matching | Backend exposes multiple methods | Configure method-based matching and call | Only target method hits | Data plane unit tests, gRPC E2E |
| `GRPC-003` | P1 | `L0/L4` | metadata/header matching | Client can send metadata | Configure metadata-based matching and call | Only requests meeting conditions hit | Data plane unit tests, gRPC E2E |
| `GRPC-004` | P1 | `L0/L4` | hostname / authority matching | Listener configured with hostname | Call with different authorities | Only requests matching hostname hit | Data plane unit tests, gRPC E2E |
| `GRPC-005` | P0 | `L4` | unary call | gRPC backend normal | Send unary request | Response code, body, metadata correct | `grpcurl`, `ghz` |
| `GRPC-006` | P0 | `L4` | server streaming | Backend supports server stream | Send server streaming call | Streaming response complete and stable | gRPC E2E |
| `GRPC-007` | P1 | `L4` | client streaming / bidi streaming | Backend supports streaming | Establish client stream / bidi stream and continuously send/receive messages | Messages complete; no abnormal stream breaks | gRPC E2E |
| `GRPC-008` | P0 | `L4` | deadline and cancel propagation | Backend can simulate slow responses | Set short deadline; actively cancel after establishing request | Returns correct gRPC code; backend can perceive cancellation | gRPC E2E |
| `GRPC-009` | P1 | `L4/L6` | trailers propagation, large messages, and high-concurrency streams | Client can read trailers | Send large messages and run concurrent load tests | Trailers normal; high-concurrency streams stable | `ghz` |
| `GRPC-010` | P0 | `L1/L4` | backend non-existent / no endpoints / cross-namespace authorization | Construct erroneous backend and cross-namespace backend | Test erroneous backend and `ReferenceGrant` scenarios separately | Status correct; fails before authorization, succeeds after | Control plane unit tests, `validate-grpc-reference-grants.sh` |

### 3.6 TLS, `BackendTLSPolicy`, and Stream Route

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TLS-001` | P0 | `L4` | HTTPS terminate | Prepare valid TLS Secret | Configure HTTPS listener and access via domain | TLS handshake succeeds; request forwarded normally | `openssl s_client`, Kind smoke |
| `TLS-002` | P0 | `L4` | SNI certificate selection | Prepare multiple domains and certificates | Configure different certificates for different hostnames and access | Returns certificate matching the domain | HTTPS E2E |
| `TLS-003` | P1 | `L4` | wildcard certificate | Prepare wildcard cert | Access with multiple subdomains | Subdomains match wildcard certificate | HTTPS E2E |
| `TLS-004` | P0 | `L1/L4` | Secret hot update, invalid certificate, expired certificate, cert/key mismatch | TLS Secret can be rotated | Replace Secret content and continuously access | Valid hot update succeeds; invalid certificate does not take effect and status accurate | Control plane unit tests, HTTPS E2E |
| `TLS-005` | P0 | `L1/L4` | cross-namespace `certificateRefs` authorization and multi-certificate fallback order | TLS Secret in different namespace, and listener can simultaneously declare same-namespace / cross-namespace / duplicate / bad-reference groups of `certificateRefs` | Verify no-grant / with-grant / delete-grant three phases, and confirm listener `certificateRefs` only accepts `group=""`, `kind=Secret`; cover the scenario of "retaining multiple valid certificates after filtering out unauthorized, invalid, and duplicate references" | Fails before authorization, succeeds after, invalidated upon revocation; unsupported group/kind exposed via `ResolvedRefs=False (InvalidCertificateRef)`; if the same listener still retains at least one valid certificate, the listener continues `Programmed=True` and infrastructure ports are not removed; remaining valid certificates maintain declaration order, falling back to the first valid certificate after filtering when SNI does not match | `./tests/e2e/validate-gateway-cross-namespace-certs.sh`, control plane/data plane unit tests |
| `TLS-006` | P1 | `L1/L4` | frontend mTLS and `Gateway.spec.tls.frontend` | Prepare client CA `ConfigMap`, client certificates, and both default / per-port configurations | Configure `spec.tls.frontend.default.validation` or `perPort[].tls.validation`, verify `AllowValidOnly` / `AllowInsecureFallback`, valid/invalid clients, bad CA ref, cross-namespace `ReferenceGrant` create/delete, and confirm `caCertificateRefs` only accepts `group=""`, `kind=ConfigMap` | Under `AllowValidOnly`, valid client succeeds, invalid or missing client fails; under `AllowInsecureFallback`, connection continues even without certificate or on validation failure, and `Gateway.status.conditions[InsecureFrontendValidationMode]=True` is written back; bad references exposed via `ResolvedRefs=False`; unsupported CA ref kind/group exposed via `InvalidCACertificateKind`; as long as at least one valid CA still exists, listener continues `Programmed=True` and infrastructure ports are retained; when all CA refs are invalid or unauthorized, listener enters `Accepted=False (NoValidCACertificate)` | Control plane unit tests, Kind/pre-production specialization |
| `TLS-007` | P0 | `L0/L4` | `BackendTLSPolicy` system CA and explicit CA bundle | HTTPS backend available | Call backend using system CA and explicit CA bundle separately | Valid TLS handshake succeeds | Data plane unit tests, specialized E2E |
| `TLS-008` | P0 | `L0/L4` | `BackendTLSPolicy` SAN validation | Backend certificate supports multiple SAN types | Verify `Hostname`, `URI` SAN match and mismatch separately | Match succeeds; mismatch fails and cannot be bypassed | Data plane unit tests, specialized E2E |
| `TLS-009` | P1 | `L0/L4` | backend mTLS client cert and cross-namespace authorization | `Gateway.spec.backendTLS.clientCertificateRef` configured, covering same-namespace and cross-namespace Secret | Verify valid TLS Secret, invalid Secret, no-`ReferenceGrant` / with-`ReferenceGrant` / after-deleting-grant three phases, and confirm `clientCertificateRef` only accepts `group=""`, `kind=Secret` | Valid and authorized client cert takes effect; invalid or unauthorized references do not enter snapshot and expose issues via status; unsupported group/kind exposed via `ResolvedRefs=False (InvalidCertificateRef)` | Control plane unit tests, specialized E2E |
| `STRM-001` | P0 | `L0/L3` | `TCPRoute` basic forwarding and port isolation | TCP backend exists | Configure multiple TCP listeners and routes and establish connections | Corresponding ports hit corresponding backends; long connections stable | Kind smoke, stream tests |
| `STRM-002` | P1 | `L0/L3` | `TCPRoute` backend failure fallback and half-close | Backend can simulate connection refusal and abnormal disconnect | Simulate backend failure after establishing connection | Failure diagnosable; does not pollute other ports/connections | Stream E2E |
| `STRM-003` | P0 | `L0/L3` | `UDPRoute` basic forwarding | UDP backend exists, e.g. DNS | Configure `UDPRoute` and send UDP requests | Request/response reachable; erroneous backend path failure observable | Kind smoke, `udp_dns_smoke.py` |
| `STRM-004` | P1 | `L0/L3` | `TLSRoute` passthrough and SNI routing | TLS backend exists | Configure `TLSRoute` and access via different SNIs | Requests routed to correct backend by SNI | Kind smoke |

### 3.7 Backend Protocol Selection

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `BPS-001` | P0 | `L4` | default behavior when `appProtocol` is not configured | Service does not have `appProtocol` configured | Deploy backend and access via HTTP/WS/h2c/gRPC | Uses default protocol path, behavior consistent with current declaration | `./tests/e2e/validate-backend-protocols.sh` |
| `BPS-002` | P0 | `L4` | `kubernetes.io/h2c` | ServicePort set `appProtocol: kubernetes.io/h2c` | Access via h2c prior knowledge and normal paths | Data plane communicates with backend via h2c | `validate-backend-protocols.sh` |
| `BPS-003` | P0 | `L4` | `kubernetes.io/ws` | ServicePort set `appProtocol: kubernetes.io/ws` | Initiate WebSocket upgrade | Upgrade succeeds; correct failure path for erroneous backend | `validate-backend-protocols.sh` |
| `BPS-004` | P1 | `L4` | h1/h2 coexistence | Different appProtocol backends exist in same environment | Concurrent requests in mixed protocol environment | Different backends forwarded per their respective protocols; no interference | `validate-backend-protocols.sh` |
| `BPS-005` | P1 | `L2/L4` | admin API reports backend protocol mapping | Data plane admin API accessible | Run specialized script then check admin API | Admin API reported protocol mapping consistent with actual traffic | `validate-backend-protocols.sh`, `curl /v1/backends` |

### 3.8 Rust Proxy Lifecycle, Upstream Behavior, and Hot Reload

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `PGW-001` | P0 | `L0/L4` | `request_filter` route selection, redirect, direct response | Redirect/direct response/filter scenarios exist | Construct normal requests, redirect requests, short-circuit response requests | Rule selection correct; short-circuit return does not trigger erroneous upstream requests | Data plane unit tests, HTTP E2E |
| `PGW-002` | P1 | `L0/L4` | `request_body_filter` and mirror/body forwarding | Mirror backend and large body upload exist | Upload body and observe primary and mirror backends | Body complete; mirror receives expected copy; no cross-contamination | Data plane unit tests, HTTP E2E |
| `PGW-003` | P0 | `L0/L2/L4` | `upstream_peer` constructing backend peer | Backend supports HTTP/HTTPS/GRPCS | Trigger different backend protocol and TLS policy combinations | Correct backend selected; TLS parameters, SNI, CA, client cert correct | Data plane unit tests, specialized E2E |
| `PGW-004` | P0 | `L2/L4/L6` | `connected_to_upstream` and connection reuse metrics | Traffic enabled and metrics scrapable | Continuously send requests and compare first vs subsequent requests | Keepalive effective; pool hit ratio and connect latency metrics grow reasonably | `validate-upstream-behavior.sh`, `/metrics` |
| `PGW-005` | P0 | `L0/L4` | `upstream_request_filter` header/path/host rewrite | Backend can echo upstream requests | Configure header modifier and rewrite then access | Upstream request matches filter order and expectations | Data plane unit tests, HTTP E2E |
| `PGW-006` | P0 | `L0/L4` | `response_filter` header/CORS/sticky session write-back | Response filters and session persistence enabled | Send requests and observe response headers, cookie/token, CORS headers | Response headers correct; sticky token stable; retryable status handled correctly | Data plane unit tests, HTTP E2E |
| `PGW-007` | P0 | `L2/L4` | `logging` and traffic stats | Access log and traffic metrics enabled | Send success, failure, retry, direct response requests | Log fields complete; summary/metrics consistent with actual behavior | `/metrics`, access log, admin API |
| `PGW-008` | P0 | `L4` | `fail_to_proxy` error path | Backend returns error or route unreachable | Construct no-route, no-backend, upstream failure scenarios | Returns correct error code and response flag; client can diagnose | Data plane unit tests, E2E |
| `PGW-009` | P0 | `L4/L6` | `fail_to_connect`, retry, and failover | At least two backends, one can inject faults | Create connection refusal/timeout/handshake failure | Retry or switch per policy after failure; stats accurate | `validate-upstream-behavior.sh` |
| `PGW-010` | P1 | `L4/L6` | HTTP/1.1, HTTP/2, WebSocket, gRPC boundary behavior | Backend supporting corresponding protocols | Verify keepalive, chunked, trailers, GOAWAY, RST, upgrade, cancel, etc. | Protocol boundary behavior stable, no stream mixing or abnormal reuse | Protocol-specific tools |
| `PGW-011` | P0 | `L4/L6` | hot reload and zero-downtime | Sustained HTTP/WS/gRPC traffic | Modify Route/Secret or rolling update dataplane under sustained traffic | Downtime within threshold; no widespread 502/503; reload observable | Kind/pre-production environment specialization |

### 3.9 Controller / Reconciler, Management Interfaces, and Observability

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `CTRL-001` | P0 | `L1` | behavior when informer/cache not synced | Can control cache sync in tests | Trigger reconcile when cache not ready | Does not panic; returns retryable behavior; does not write erroneous status | Control plane unit tests |
| `CTRL-002` | P0 | `L1` | reconcile idempotency | Same object reconciled repeatedly | Execute multiple reconciles on same resource consecutively | Results consistent; no duplicate resource creation | Control plane unit tests |
| `CTRL-003` | P1 | `L1/L6` | rapid successive updates and Route churn | Can batch update resources | High-frequency updates to Gateway/Route/ReferenceGrant | Eventually consistent state; queue can drain | Control plane unit tests, scale specialization |
| `CTRL-004` | P0 | `L1` | finalizer, deletion order, and orphan cleanup | Downstream resources managed by controller exist | Delete Gateway, Route, GatewayClass in different orders | Downstream resources correctly cleaned up; no orphans | Control plane unit tests |
| `CTRL-005` | P1 | `L6` | leader election switch and multi-replica consistency | Controlplane multi-replica deployment | Rolling restart leader or force leader switch | Continues to converge after switch; status does not oscillate | Kind/pre-production specialization |
| `CTRL-006` | P1 | `L6` | status update storm | Can batch construct attach/detach scenarios | Trigger large volume of status changes and observe apiserver/controller metrics | Does not form sustained storm; system can recover to steady state | Scale specialization |
| `OBS-001` | P0 | `L2/L3` | `/livez`, `/readyz` | Control plane and data plane admin accessible | Access in not-ready and ready phases separately | Status code and response text match documentation conventions | `curl` |
| `OBS-002` | P0 | `L2/L3` | `/v1/summary`, `/v1/snapshot-sync` | Valid snapshot exists | Read control plane and data plane summary/sync interfaces | Version, ready, listener/route/backend stats consistent | `curl`, `jq` |
| `OBS-003` | P0 | `L2/L3` | `/v1/listeners`, `/v1/routes`, `/v1/backends`, `/v1/nodes` filtering behavior | Resources created | Filter using name/kind/namespace/hostname/protocol parameters | Filter results accurate; object details traceable | `curl`, `jq` |
| `OBS-004` | P0 | `L2/L3/L6` | metrics exposure and scraping | Prometheus or curl can access metrics | Scrape control plane metrics port and data plane `/metrics` | Key metrics present and continuously scrapable | Prometheus, `curl` |
| `OBS-005` | P1 | `L2/L6` | log, metrics, admin API tri-party alignment | Access log and metrics enabled | Trigger success, failure, timeout, retry scenarios | Three evidence types correspond to same request result | Logs, metrics, admin API |

### 3.10 Mesh Frontend / Service Parent Extension

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `MESH-001` | P1 | `L4` | consumer namespace isolation | Mesh/service parent scenarios enabled | Create producer/consumer namespaces and corresponding Routes | Only allowed consumer paths hit; no unauthorized access to producer | `./tests/e2e/validate-mesh-frontends.sh` |
| `MESH-002` | P1 | `L4` | `parentRef.port` scope | Service exposes multiple ports | Create Route bound by `parentRef.port` | Only frontend of specified port exposes corresponding route | `validate-mesh-frontends.sh` |
| `MESH-003` | P1 | `L4` | admin API reports mesh listeners/routes | Data plane admin API accessible | Run mesh specialization then read admin API | Mesh listeners/routes visible in admin API and consistent with behavior | `validate-mesh-frontends.sh`, `curl /v1/routes` |

### 3.11 Security Matrix

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `SEC-001` | P0 | `L2/L3` | admin authentication boundary | Controlplane/dataplane configured with Bearer Token | Access `/livez`, `/readyz`, `/v1/*`, `/metrics` anonymously | Probe endpoints allow anonymous access; other endpoints return `401/403` when unauthorized | `curl` |
| `SEC-002` | P0 | `L1/L4` | cross-namespace authorization boundary | BackendRef, certificateRef, CA ConfigMap cross-namespace scenarios exist | Verify no-grant / with-grant / delete-grant separately | Authorization scope strictly controlled; no spillover | Control plane unit tests, specialized E2E |
| `SEC-003` | P0 | `L0/L4` | TLS/SAN/mTLS validation cannot be bypassed | Can construct valid and invalid certificates | Initiate valid and invalid handshakes separately | Valid succeeds; invalid fails; cannot bypass SAN/CA validation | Data plane unit tests, `openssl s_client` |
| `SEC-004` | P1 | `L6` | request smuggling: `CL/TE`, `TE/CL` | Can send raw HTTP messages | Construct multiple smuggling request sets covering keepalive/reuse scenarios | No request mixing; no downstream pollution; no abnormal reuse triggered | `./tests/e2e/validate-http-security.sh` |
| `SEC-005` | P1 | `L6` | malformed chunked, duplicate header, CRLF injection | Custom raw requests supported | Construct abnormal header/body messages | Illegal requests rejected or safely handled | `./tests/e2e/validate-http-security.sh` + raw socket/fuzz scripts |
| `SEC-006` | P1 | `L6` | oversized headers | Can control header size | Gradually increase header size near limit | Over-limit requests safely rejected; resource usage controllable | `./tests/e2e/validate-http-security.sh` |
| `SEC-007` | P1 | `L6` | `Host` / `X-Forwarded-*` spoofing | Trusted proxy and direct paths exist | Spoof Host/XFF/XFP separately and access | Spoofed headers do not cause unauthorized routing or authentication errors | `./tests/e2e/validate-http-security.sh` |
| `SEC-008` | P1 | `L6` | slowloris / connection flood / idle timeout | Can establish many slow connections | Establish slow connections, half-open connections, idle connections | Normal traffic still servable; FD/connection count not out of control | `./tests/e2e/validate-http-security.sh` + load testing tools |
| `SEC-009` | P1 | `L6` | log sanitization and multi-tenant isolation | Access log enabled | Trigger authentication errors, cross-namespace errors, and abnormal requests | Logs do not leak secret/token; tenant boundaries not breached | Log inspection |
| `SEC-010` | P1 | `L6` | supply chain and image security | Scanning tools runnable | Scan Rust/Go dependencies, images, Manifests | No high-severity blockers; report archivable | `cargo audit`, `osv-scanner`, `trivy`/`grype`, `kubescape`/`kubeaudit` |

### 3.12 Performance, Capacity, Stability, and Chaos

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `PERF-001` | P0 | `L6` | HTTP baseline throughput | Load test environment available | Steady-state load test on HTTP listener for 10-30 minutes | Error rate, p99, CPU meet SLA | `fortio`, `wrk2`, `vegeta` |
| `PERF-002` | P0 | `L6` | HTTPS baseline throughput and handshake capacity | HTTPS listener available | Load test HTTPS forwarding and high-concurrency short-connection handshakes | Explainable overhead vs HTTP; handshake failure rate within threshold | `h2load`, `openssl` |
| `PERF-003` | P0 | `L6` | gRPC baseline and streaming stability | gRPC backend available | Load test unary, server/client/bidi streaming separately | gRPC error rate controllable; long streams stable | `ghz` |
| `PERF-004` | P0 | `L6` | WebSocket concurrent connections | WebSocket backend available | Establish many sustained WS connections | High connection success rate; FD/memory controllable | `websocat` or custom load test |
| `PERF-005` | P0 | `L6` | large request body / slow upload / backpressure | Backend supports large body | Upload large body and simulate slow upload | No memory spike; normal traffic not dragged down | Load testing tools, custom client |
| `PERF-006` | P0 | `L6` | upstream connection pool and weighted distribution | At least two backends | Test pool hit ratio, connect latency, weighted distribution | Connection pool effective; distribution deviation acceptable | `validate-upstream-behavior.sh` |
| `PERF-007` | P0 | `L6` | config hot reload and endpoint churn | Sustained traffic present | Update Route/Secret/weight or rolling restart backend Pods | Config propagation fast; 5xx within threshold | Kind/pre-production environment specialization |
| `PERF-008` | P0 | `L6` | 24h soak | Stable load test and monitoring environment | Sustain 30%-50% target traffic for 24h | No linear memory/FD leak; error rate does not continuously rise | Load testing tools + Prometheus |
| `PERF-009` | P1 | `L6` | 72h soak | Long-running pre-production environment | Run at moderate load continuously for 72h | System stable; resource curves smooth | Load testing tools + Prometheus |
| `PERF-010` | P0 | `L6` | configuration scale test | Can batch generate resources | Gradually increase Gateway/Route/rule/backend count | Convergence time, memory, CPU within acceptable range | Generation scripts + metrics |
| `PERF-011` | P1 | `L6` | extreme capacity and CPU/memory/FD saturation points | Environment resource quotas fixed | Stepwise pressure increase until near saturation | Obtain safe operating watermark and inflection point | Load testing tools |
| `CHAOS-001` | P0 | `L6` | backend timeout/reset/5xx/connection refusal | Backend supports fault injection | Inject different types of backend faults under sustained traffic | Failure isolation correct; retry/switch matches design | `tc/netem`, fault injection scripts |
| `CHAOS-002` | P0 | `L6` | all backend pods down and partial endpoint anomalies | Backend multi-replica | Take down all or partial endpoints | Overall behavior matches design; can re-converge after recovery | Chaos tools |
| `CHAOS-003` | P1 | `L6` | gateway pod kill / node drain / pod eviction / OOM | Cluster environment can simulate faults | Trigger gateway-side faults separately | Service recovers within expected time; long-connection impact acceptable | Chaos tools |
| `CHAOS-004` | P1 | `L6` | controller restart / leader switch / watch disconnect | Controlplane multi-replica or restartable | Restart controller under sustained traffic and resource changes | Convergence recovery; status not chaotic; no sustained deadlock | Chaos tools |
| `CHAOS-005` | P1 | `L6` | network faults: packet loss, latency, jitter, DNS failure | Can inject network faults | Inject network issues on dataplane/backend/controlplane | System behavior explainable during faults; can return to steady state after recovery | `tc/netem`, Chaos Mesh |

### 3.13 Release, Canary, Rollback, and Report Archiving

| ID | Priority | Tier | Scenario | Prerequisites | Steps | Expected Result | Automation Entry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `REL-001` | P0 | `L7` | pre-release baseline validation | Local or CI has required dependencies | Execute release validation | Control plane, data plane, Kind, specialized E2E, conformance all pass | `./scripts/run-release-validation.sh` |
| `REL-002` | P0 | `L7` | core path release validation | Local or CI has required dependencies | Execute release validation | Core gateway path fully validated; dashboard UI changes require separate validation | `./scripts/run-release-validation.sh` |
| `REL-003` | P0 | `L7` | conformance report archiving | Release validation success or failure | Set `ARCHIVE_REPORT_ID` and execute release validation | `report.yaml`, `metadata.yaml`, `run.log` archived | `ARCHIVE_REPORT_ID=<id> ./scripts/run-release-validation.sh` |
| `REL-004` | P0 | `L7` | control plane canary `GatewayClass` preparation | Stable and canary control plane images available | Create canary `GatewayClass` and switch a few Gateways to canary | Canary control plane only takes over target Gateways; status normal | `./scripts/prepare-canary-gatewayclass.sh` |
| `REL-005` | P0 | `L7` | data plane synthetic/shadow validation | Independent canary dataplane entry available | First synthetic, then shadow traffic | Canary and stable behavior largely consistent; does not affect user traffic | Runbook, load testing/observation tools |
| `REL-006` | P0 | `L7` | phased rollout 1%/5%/10%/25%/50%/100% | Can control ingress traffic weight | Phase rollout and observe for fixed duration at each stage | Business, performance, stability metrics meet thresholds at each stage | Runbook, observation dashboards |
| `REL-007` | P0 | `L7` | rollback drill | Canary traffic exists | Execute GatewayClass switchback, traffic weight callback, or image rollback | Rollback completed within 5 minutes; business restored to baseline | `./scripts/rollback-canary-gatewayclass.sh`, Runbook |
| `REL-008` | P1 | `L7` | release evidence retention | Release window completed | Save conformance, E2E, performance charts, management interface snapshots, rollback records | All key evidence traceable to version and time window | Repository report directory, release records |

## 4. Minimum Release-Blocking Subset

If the current phase must converge to a minimum blocking set, at least cover the following test cases:

- `XDS-001`、`XDS-002`、`XDS-003`
- `CP-001`、`CP-004`、`CP-005`、`CP-008`、`CP-009`
- `ATT-001`、`ATT-002`、`ATT-005`、`ATT-006`、`ATT-007`
- `HTTP-001`、`HTTP-004`、`HTTP-008`、`HTTP-010`、`HTTP-013`、`HTTP-015`
- `GRPC-001`、`GRPC-005`、`GRPC-008`、`GRPC-010`
- `TLS-001`、`TLS-004`、`TLS-005`、`TLS-007`、`TLS-008`
- `BPS-001`、`BPS-002`、`BPS-003`
- `PGW-003`、`PGW-004`、`PGW-006`、`PGW-007`、`PGW-009`、`PGW-011`
- `OBS-001`、`OBS-002`、`OBS-003`、`OBS-004`
- `SEC-001`、`SEC-002`、`SEC-003`
- `PERF-001`、`PERF-002`、`PERF-003`、`PERF-006`、`PERF-007`、`PERF-008`
- `REL-001`、`REL-003`、`REL-004`、`REL-006`、`REL-007`

## 5. Follow-Up Maintenance Recommendations

- When adding new features, first supplement coverage boundaries in the [Test Plan](./plan.md), then add specific test cases in this document.
- When adding specialized scripts, backfill the script path into the "Automation Entry" column.
- Before each external release, at least review once whether the `P0` set and the current support matrix are consistent.
