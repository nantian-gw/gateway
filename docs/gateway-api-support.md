# Gateway API Support Matrix

## English Summary

Nantian Gateway targets Gateway API `v1.5.1` and currently declares `55` `GatewayClass.status.supportedFeatures`.
The latest archived conformance report under `reports/conformance/latest/` is `2026-05-14-162416-90f5126a-full`, which is an `ALL_FEATURES=true` full-suite run for implementation version `90f5126a`.
The latest clean full-suite conformance baseline is `2026-05-14-162416-90f5126a-full`.

The support boundary is intentionally split into four levels:

- `declared`: advertised through `GatewayClass.status.supportedFeatures` and the conformance profile.
- `implemented`: translated by the control plane and served by the data plane.
- `tested`: backed by conformance, e2e, unit, smoke, benchmark, or targeted evidence.
- `production-validated`: backed by long-running production-like evidence. Most current features are still `limited` here.

Current unsupported or intentionally deferred areas include HTTP/3 / QUIC, complete BackendLBPolicy support, full multi-environment production capacity evidence, and all features not explicitly listed as supported below.

---

This inventory is based on the current repository `HEAD` code, the most recent archived conformance report, and the most recent clean full-suite conformance baseline, read at `2026-05-22`.
The control plane currently depends on Gateway API Go module version `sigs.k8s.io/gateway-api v1.5.1`, and uses objects from `apis/v1`, `apis/v1alpha2`, `apis/v1alpha3`, and `apis/v1beta1`. The Gateway API CRD version installed by local `kind` / `conformance` / release verification scripts has also been advanced to `v1.5.1`. For `BackendTLSPolicy`, the control plane interacts with the apiserver via the `gateway.networking.k8s.io/v1` compatibility access layer to avoid hitting the `v1alpha3` deprecation warning. `BackendLBPolicy` is still maintained separately on the `v1.2.1` compatibility track as an additional experimental CRD. Besides the upstream `sessionPersistence`, the repository also maintains a repo-specific `loadBalancing.consistentHash` extension subset. All other backend load balancing policy fields are still considered unsupported unless separately designed, implemented, and externally audited.

This serves two purposes:

- Describe which Kubernetes Gateway API capabilities the current project has already implemented.
- Clarify which capabilities remain partially implemented, unimplemented, or unvalidated.

This inventory is not a statement that "all Gateway API features are fully implemented," but the repository has integrated the official conformance harness and has archived multiple `ALL_FEATURES=true` full-suite validations. The most recent archived conformance report is the `90f5126a` full-suite from `2026-05-14`; the most recent clean full-suite baseline is also `90f5126a` from `2026-05-14`, targeting Gateway API `v1.5.1`. The repository currently declares 55 `supportedFeatures`. Undeclared capabilities such as HTTP/3 are still tracked as gaps. Subsequent runtime code, deployment, or script behavior changes still require a separate full-suite re-archival before serving as a new externally referenced baseline.
<!-- release-evidence:conformance-clean-support:start -->
The most recent clean full-suite baseline is `90f5126a` from `2026-05-14`. If a newer commit is later used as an externally referenced baseline, a fresh full-suite report must still be archived for that commit. See [reports/conformance](../reports/conformance/README.md) for corresponding results.
<!-- release-evidence:conformance-clean-support:end -->

If the goal is not only to describe the technical support scope but also to assess "how far we are from Gateway API official implementation recognition and a more formal community state," also read the [Community Readiness Checklist](community-readiness.md).
If you also need a page better suited for external reference as a "public adopter / case study / compatibility matrix" entry point, also read [adopters-and-compatibility.md](adopters-and-compatibility.md).
If you need to decide whether to continue expanding Gateway API experimental support, read [Gateway API Experimental Feature Audit](backlog/gateway-api-experimental.md).

A key distinction:

- The `support matrix` describes the current code path, test evidence, and most recent archived baseline.
- Whether `current HEAD satisfies this release gate` also depends on whether conformance, E2E, protocol-specific, soak, and canary exercises have been re-run against the current commit.

## State Definitions

This document no longer uses `supported` as a single state, but instead expresses the support scope across four independent levels:

- `declared`: appears in the public declaration of `GatewayClass.status.supportedFeatures` and the `ALL_FEATURES=true` conformance profile. This level only indicates that the repository is willing to include the feature in the official harness validation scope, not that all scenarios are production-ready.
- `implemented`: the control plane translation, status evaluation, gRPC/xDS delivery, and data plane main forwarding path already exist. This level may come from standard Gateway API resources or repo-specific extensions.
- `tested`: verifiable conformance, e2e, unit, smoke, benchmark, or targeted script evidence exists. Official conformance, repository kind smoke, unit tests, and short-duration performance/soak evidence are annotated separately and not mixed.
- `production-validated`: sufficient evidence exists in production overlay, extended soak, fault injection, upgrade/rollback, capacity stress testing, and real or production-like environments. Currently, most capabilities in this repository can only be marked as `limited` or `no`; passing conformance does not automatically elevate to production-grade completion.

Older tables still contain Chinese status labels such as `Implemented`, `Partially Implemented`, and `Not Implemented / Not Validated` to describe the implementation dimension. Whether something is declared, tested, and production-validated should follow the four-level view in this section.

In this document, `tested=conformance` defaults to "verified by the most recent full-suite conformance archive," unless the corresponding capability is separately annotated with targeted conformance or mesh profile evidence. As of the time this document was read, the most recent archive in the repository is the `90f5126a` full-suite from `2026-05-14`, and the most recent clean full-suite baseline is also `90f5126a` from `2026-05-14`. Subsequent commits do not automatically inherit the same level of report (current `HEAD` has had data plane changes since that baseline).

## Four-Level Support View

| Capability | declared | implemented | tested | production-validated | Current Limitations |
| --- | --- | --- | --- | --- | --- |
| `GatewayClass` / `Gateway` basic capability | yes | yes | conformance + e2e + unit | limited | Production overlay, fault injection, and 10m soak pilot exist, but no 24h/72h soak, managed Kubernetes distribution matrix, or formal release canary evidence. |
| Default Gateways / GEP-3793 | no | yes | unit + kind smoke | no | Supports implicit binding of `Gateway.spec.defaultScope=All` with Route `spec.useDefaultGateways=All`, Route parent status, Gateway `DefaultGateway` condition, and data plane IR synthetic parentRef. This experimental field currently has no corresponding upstream `supportedFeatures` declaration, but has Kind smoke coverage. |
| Gateway frontend client certificate validation | yes | yes | targeted conformance + unit | limited | `RequireClientCertificate`, `AllowInsecureFallback`, invalid CA references, and invalid default configurations are covered. Certificate rotation, long-running operation, multi-tenant CA operations, and production fault injection evidence remain insufficient. |
| `HTTPRoute` | yes | yes | conformance + e2e + unit + kind performance | limited | Main path is available. Extended capabilities such as CORS, ExtensionRef, HTTP/3, and frontend client certificates need per-item declaration and evidence review. |
| `GRPCRoute` | yes | yes | conformance + e2e + unit | limited | gRPC is served through HTTP/HTTPS listeners. A standalone `GRPC` listener is not declared as Gateway API Core. |
| `UDPRoute` | yes | yes | official conformance + kind smoke + unit | limited | Gateway API `v1.5.1` official harness covers `UDPRoute`. Production-grade throughput, packet loss, session churn, and long-duration soak are not yet closed. |
| `TCPRoute` | no | yes | supplemental conformance-style + kind smoke + unit | no | Upstream `v1.5.1` does not have a `SupportTCPRoute` feature or TCPRoute conformance tests. The repository uses data plane/control plane supplemental tests and Kind smoke to pin current forwarding, ReferenceGrant, port/protocol isolation, and missing-backend behavior. |
| `TLSRoute` passthrough | yes | yes | targeted conformance + kind smoke + unit | limited | Currently declares core passthrough. The data plane supports mixed mode (`LISTENER_PROTOCOL_TLS`), declared as `TLSRouteModeMixed`. |
| `Service` parent / mesh frontend | yes | yes | targeted conformance + e2e + unit | limited | `Mesh`, `MeshClusterIPMatching`, `MeshConsumerRoute`, and mesh HTTPRoute extended features have been redeclared. Current evidence includes the `2026-05-14` `90f5126a` full-suite, the `2026-05-12` `0355945e` kind mesh profile, and targeted Mesh suite. Production-grade long-running and multi-environment east-west validation remain insufficient. |
| `BackendTLSPolicy` | yes | yes | official conformance + e2e + unit | limited | `v1` compatibility access, system CA, custom CA, SAN validation, basic status, and CA bundle rotation are covered. Long-running and production CA operation evidence remain insufficient. |
| `BackendLBPolicy` | no | partial | e2e + unit + targeted smoke | no | Only covers upstream `sessionPersistence` plus the repo-specific `loadBalancing.consistentHash` subset. This is not a complete BackendLBPolicy support declaration and is not among the current generated 55 `supportedFeatures`. |
| `HTTP3` / `QUIC` | no | no | no | no | Only has internal protocol bit / management plane visibility placeholders. No downstream HTTP/3 capability is declared. |

## Overall Assessment

| Dimension | Current Assessment | Description |
| --- | --- | --- |
| Production Readiness Level | `Partially Implemented` | Has the foundation for long-running operation including multi-replica, PDB, admin authentication, gRPC TLS/mTLS, NetworkPolicy, metrics, and health checks, but the documentation explicitly does not guarantee production-grade certificate rotation, full performance optimization, etc. |
| Gateway API Feature Completeness | `Not Implemented / Not Validated` | Currently only covers core resources and some extended capabilities, not an implementation where "all features are complete." |
| Gateway API Specification Compliance | `Validated (most recent archived full-suite baseline)` | The official conformance harness has completed an `ALL_FEATURES=true` full pass and been archived as the latest baseline. The local default scripts still retain the `GATEWAY-HTTP` quick profile; full-suite must be explicitly enabled. |

### Conclusion Markers

- `Currently should NOT be labeled as "production-grade complete and ready"`.
- `Currently CAN be labeled as "trialable within controlled boundaries / partially production-available"`.
- If the goal is a formal production environment, you must still supplement certificate rotation, stress testing, fault injection, capacity assessment, and target feature validation beyond this repository's declared support scope.

## Conformance Results

As of `2026-05-14`, the repository has two layers of result archives:

- Most recent archived conformance report (current `latest/`, `ALL_FEATURES=true` full-suite):
  - [report.yaml](../reports/conformance/latest/report.yaml)
  - [metadata.yaml](../reports/conformance/latest/metadata.yaml)
  - [run.log](../reports/conformance/latest/run.log)
  - Corresponding immutable archive directory:
    - [report.yaml](../reports/conformance/runs/2026-05-14-162416-90f5126a-full/report.yaml)
    - [metadata.yaml](../reports/conformance/runs/2026-05-14-162416-90f5126a-full/metadata.yaml)
    - [run.log](../reports/conformance/runs/2026-05-14-162416-90f5126a-full/run.log)
<!-- release-evidence:conformance-clean-results:start -->
- Most recent clean full-suite baseline:
  - [report.yaml](../reports/conformance/runs/2026-05-14-162416-90f5126a-full/report.yaml)
  - [metadata.yaml](../reports/conformance/runs/2026-05-14-162416-90f5126a-full/metadata.yaml)
  - [run.log](../reports/conformance/runs/2026-05-14-162416-90f5126a-full/run.log)
<!-- release-evidence:conformance-clean-results:end -->
- The release workflow executes full-suite conformance at release time and publishes the latest report to the repository's `conformance-reports` branch, also preserving it as a release artifact.
- If the current development branch continues to advance, it is still recommended to re-archive an `ALL_FEATURES=true` full-suite report for the commit to be referenced before a release or external claim.

A note:

- The default behavior of `tests/conformance/run.sh` is still the `GATEWAY-HTTP` profile, which is friendly for local quick debugging.
- The full-suite used for release gating in this repository is only executed when `ALL_FEATURES=true` is explicitly set.

## Repository Declared supportedFeatures

<!-- BEGIN GENERATED SUPPORTED FEATURES -->
This section is auto-generated by `scripts/update-gateway-api-support.sh` from `controlplane/internal/gatewayapi/supported_features.go`.
It shares the same feature set with the following two public declarations, rather than being maintained manually:

- `GatewayClass.status.supportedFeatures`
- conformance `SupportedFeatures` when `ALL_FEATURES=true`

Current number of features publicly declared by this repository: `55`

| Feature | GatewayClass.status | Conformance Profile |
| --- | --- | --- |
| `BackendTLSPolicy` | `Yes` | `Yes` |
| `BackendTLSPolicySANValidation` | `Yes` | `Yes` |
| `GRPCRoute` | `Yes` | `Yes` |
| `GRPCRouteNamedRouteRule` | `Yes` | `Yes` |
| `Gateway` | `Yes` | `Yes` |
| `GatewayAddressEmpty` | `Yes` | `Yes` |
| `GatewayBackendClientCertificate` | `Yes` | `Yes` |
| `GatewayFrontendClientCertificateValidation` | `Yes` | `Yes` |
| `GatewayFrontendClientCertificateValidationInsecureFallback` | `Yes` | `Yes` |
| `GatewayHTTPListenerIsolation` | `Yes` | `Yes` |
| `GatewayHTTPSListenerDetectMisdirectedRequests` | `Yes` | `Yes` |
| `GatewayInfrastructurePropagation` | `Yes` | `Yes` |
| `GatewayPort8080` | `Yes` | `Yes` |
| `GatewayStaticAddresses` | `Yes` | `Yes` |
| `HTTPRoute` | `Yes` | `Yes` |
| `HTTPRoute303RedirectStatusCode` | `Yes` | `Yes` |
| `HTTPRoute307RedirectStatusCode` | `Yes` | `Yes` |
| `HTTPRoute308RedirectStatusCode` | `Yes` | `Yes` |
| `HTTPRouteBackendProtocolH2C` | `Yes` | `Yes` |
| `HTTPRouteBackendProtocolWebSocket` | `Yes` | `Yes` |
| `HTTPRouteBackendRequestHeaderModification` | `Yes` | `Yes` |
| `HTTPRouteBackendTimeout` | `Yes` | `Yes` |
| `HTTPRouteCORS` | `Yes` | `Yes` |
| `HTTPRouteDestinationPortMatching` | `Yes` | `Yes` |
| `HTTPRouteHostRewrite` | `Yes` | `Yes` |
| `HTTPRouteMethodMatching` | `Yes` | `Yes` |
| `HTTPRouteNamedRouteRule` | `Yes` | `Yes` |
| `HTTPRouteParentRefPort` | `Yes` | `Yes` |
| `HTTPRoutePathRedirect` | `Yes` | `Yes` |
| `HTTPRoutePathRewrite` | `Yes` | `Yes` |
| `HTTPRoutePortRedirect` | `Yes` | `Yes` |
| `HTTPRouteQueryParamMatching` | `Yes` | `Yes` |
| `HTTPRouteRequestMirror` | `Yes` | `Yes` |
| `HTTPRouteRequestMultipleMirrors` | `Yes` | `Yes` |
| `HTTPRouteRequestPercentageMirror` | `Yes` | `Yes` |
| `HTTPRouteRequestTimeout` | `Yes` | `Yes` |
| `HTTPRouteResponseHeaderModification` | `Yes` | `Yes` |
| `HTTPRouteSchemeRedirect` | `Yes` | `Yes` |
| `Mesh` | `Yes` | `Yes` |
| `MeshClusterIPMatching` | `Yes` | `Yes` |
| `MeshConsumerRoute` | `Yes` | `Yes` |
| `MeshHTTPRouteBackendRequestHeaderModification` | `Yes` | `Yes` |
| `MeshHTTPRouteNamedRouteRule` | `Yes` | `Yes` |
| `MeshHTTPRouteQueryParamMatching` | `Yes` | `Yes` |
| `MeshHTTPRouteRedirectPath` | `Yes` | `Yes` |
| `MeshHTTPRouteRedirectPort` | `Yes` | `Yes` |
| `MeshHTTPRouteRewritePath` | `Yes` | `Yes` |
| `MeshHTTPRouteSchemeRedirect` | `Yes` | `Yes` |
| `ReferenceGrant` | `Yes` | `Yes` |
| `TLSRoute` | `Yes` | `Yes` |
| `TLSRouteModeMixed` | `Yes` | `Yes` |
| `TLSRouteModeTerminate` | `Yes` | `Yes` |
| `UDPRoute` | `Yes` | `Yes` |
<!-- END GENERATED SUPPORTED FEATURES -->

## By Gateway API Level View

In this section, `Core / Extended / Experimental` follow the upstream Gateway API object annotations in the control plane's current dependency version; the `BackendTLSPolicy` apiserver interaction has been handled via `gateway.networking.k8s.io/v1`.

Two important distinctions:

- `Core / Extended` mostly refer to the support level of a field, filter, or specific semantic, not necessarily that the entire resource object has only a single level.
- `Experimental` refers to experimental channel resources or fields marked with `<gateway:experimental>`, not equal to `Implementation-specific`.

Additionally, the current repository has implemented some behaviors that the specification marks as `Implementation-specific`, such as certain regex matching capabilities. These capabilities are available but should not be misunderstood as standard `Core` or `Extended` guarantees.

### 1. Core View

| Upstream Capability | Repository Status | Description |
| --- | --- | --- |
| `GatewayClass.spec.controllerName` and basic adoption semantics | `Implemented` | Identifies by controller name and writes back `GatewayClass` basic status. |
| `Gateway` Listener core fields: `hostname`, `port`, `protocol`, `tls.mode`, `AllowedRoutes.namespaces`, `AllowedRoutes.kinds` | `Implemented` | Standard core listener protocol semantics handled as `HTTP / HTTPS / TLS / TCP / UDP`. |
| Current supported subset of one or more `certificateRefs -> Secret` | `Implemented (currently declared subset)` | Currently supports parsing multiple `Secret` type certificates in declaration order. The control plane first filters unauthorized, missing, or invalid references, then deduplicates remaining valid references in declaration order. The data plane prioritizes SNI exact/wildcard matching and falls back to the first valid certificate in the filtered list on miss. Other resource types remain out of scope. |
| `GRPCRouteRule.RequestHeaderModifier` core filter | `Implemented` | Although `GRPCRoute` as a whole belongs to Extended, its rule-level `RequestHeaderModifier` core semantics have been implemented. |
| `ReferenceGrant` core authorization model | `Implemented` | Covers the two core scenarios actually used by the repository: Route -> Service and Gateway -> Secret. |
| `HTTPRoute` core subset | `Implemented` | Core support for `hostname`, `Path Exact/PathPrefix`, `Header Exact`, `RequestHeaderModifier`, `RequestRedirect`, `Service backendRef`, and weight has been implemented. |
| `GRPCRoute` core field subset | `Implemented` | Although `GRPCRoute` as a whole belongs to Extended, its core subset such as `Exact(service+method)`, `Header Exact`, and `Service backendRef` have been implemented. |
| Basic `Accepted / ResolvedRefs / Programmed` status | `Implemented` | Core mainline status is written back, but this does not equal full coverage of the complete status matrix. |

### 2. Extended View

| Upstream Capability | Repository Status | Description |
| --- | --- | --- |
| `GRPCRoute` resource type | `Implemented` | The repository has actual translation, matching, and forwarding paths, and has been included in the most recent archived full-suite conformance passing results. |
| `HTTPRoute` Extended subset: `method`, `queryParams(Exact)`, `ResponseHeaderModifier`, `URLRewrite`, `RequestMirror`, `timeouts` | `Implemented` | These capabilities have been implemented in both control plane translation and data plane runtime. |
| Extended filter subset on `HTTPBackendRef` | `Implemented` | The repository sinks supported HTTP filters to `HTTPBackendRef` and merges them at runtime. |
| `GRPCRoute` Extended filter subset | `Implemented` | Rule-level and `GRPCBackendRef.filters` for the repository's supported gRPC filter subset have been implemented and will be merged at the data plane. |
| Extended weight semantics in `TCPRoute` / `UDPRoute` / `TLSRoute` | `Implemented` | The stream runtime implements weighted backend selection. |
| `Gateway.spec.addresses` | `Implemented (currently declared subset)` | Supports multi-value validation, normalized deduplication, stable status publication, and delivery of multiple listener bind addresses to the data plane for `IPAddress` / `Hostname` type addresses. When `spec.addresses[].value` is empty, the control plane automatically selects a programmable address based on the request type and writes `Programmed=False, Reason=AddressNotAssigned` when no matching address is found. When the per-Gateway Service exposes `LoadBalancer ingress` or `externalIPs` and its metadata has converged with the current Gateway, status addresses are preferentially derived from the Service using canonical semantics. If the Service is still undergoing ownership/metadata drift, it falls back to the global `statusAddress` / `statusAddresses`, and empty automatic allocation only selects from this set of "publishable" addresses to avoid reassigning drifting old addresses into the Gateway status. |
| `Gateway.spec.infrastructure` | `Partially Implemented` | Currently supports `labels` and `annotations`, propagating these metadata to the control-plane-managed per-Gateway Service and corresponding frontend EndpointSlice. Also supports `GatewayClass.spec.parametersRef` as default parameters for per-Gateway Service, with `Gateway.spec.infrastructure.parametersRef` overriding `type`, traffic policy, and load balancer exposure parameters per field. The control plane also writes stable ownership / `gatewayClassName` / `parametersRef` / effective service-parameters hash annotations to these derived resources; per-Gateway Service gets controller `ownerReference -> Gateway`, derived frontend EndpointSlice gets controller `ownerReference -> Service`, for auditing current source and effective configuration. When the parameter reference does not exist, the kind is unsupported, or the content is invalid, `Gateway` writes `Accepted=False, Reason=InvalidParameters` to expose the error, and continues converging with the current default fallback model. Broader infrastructure orchestration capabilities are not declared as supported. |
| `ServiceImport` as backend | `Implemented` | Currently supports `multicluster.x-k8s.io/ServiceImport` backend references, status validation, and data plane selection. |

### 3. Experimental View

| Upstream Capability | Repository Status | Description |
| --- | --- | --- |
| `TCPRoute`, `UDPRoute`, `TLSRoute` resource channel | `Implemented` | These resources currently belong to the `v1alpha2` experimental channel upstream. The repository has runtime support but does not claim full conformance. |
| `HTTPRoute.retry` | `Implemented` | Supports control plane translation, gRPC delivery, and data plane retry triggered by response code / connection failure with minimum backoff. |
| `HTTPRoute.sessionPersistence` / `GRPCRoute.sessionPersistence` | `Implemented` | The control plane can translate and deliver to the data plane. The data plane currently uses stateless signed tokens for session persistence; production environments should explicitly configure a stable shared secret. |
| `BackendLBPolicy` | `Implemented (currently covers sessionPersistence + repo-specific loadBalancing.consistentHash subset)` | Currently supports `Service` / `ServiceImport` target, delivering backend-level `sessionPersistence` with `loadBalancing.type=ConsistentHash` to the data plane, and writing back `Accepted` / `ResolvedRefs` basic status. Consistent hashing currently supports `Header` / `SourceIP` / `Hostname` three hash key types. When multiple policies target the same backend, the control plane selects the effective policy by creation time first, then name lexicographic order, with remaining policies entering `Conflicted`. If multiple backends referenced by the same route do not resolve to the same load balancing policy, the data plane falls back to existing weighted round-robin. |
| `Gateway.spec.tls.frontend` | `Implemented (HTTPS listener scope)` | The control plane parses `spec.tls.frontend.default.validation` and `spec.tls.frontend.perPort[].tls.validation`, validates the `ca.crt` in `ConfigMap`, and enables frontend mTLS validation only on HTTPS listeners. Currently supports `AllowValidOnly` and `AllowInsecureFallback` modes. When any HTTPS listener uses `AllowInsecureFallback`, the Gateway additionally writes back `InsecureFrontendValidationMode=True`. |
| `Gateway.spec.backendTLS.clientCertificateRef` | `Implemented` | The control plane validates and translates `Secret` references, and the data plane uses client certificates to establish mTLS on `HTTPS` / `GRPCS` upstream connections. |
| `BackendTLSPolicy` | `Implemented (converged with upstream Rust proxy/OpenSSL current capabilities)` | Currently supports `Service` / `ServiceImport` target, `validation.hostname`, `wellKnownCACertificates=System`, `ConfigMap/ca.crt` in the same namespace for `caCertificateRefs`, and one or more `Hostname` / `URI` `subjectAltNames` combinations. The data plane continues to use `validation.hostname` for upstream SNI. If `subjectAltNames` is explicitly configured, certificate identity verification performs an additional "any match passes" check against the configured SAN set after handshake, rather than reusing `validation.hostname` for hostname matching. The repo-specific `spec.options` `gateway.nantian.dev/backend-tls-min-version` / `gateway.nantian.dev/backend-tls-max-version` are now explicitly rejected by the control plane rather than relying on a vendored Rust proxy patch. When multiple policies target the same backend, the control plane still selects the effective policy by creation time first, then name lexicographic order, with remaining policies entering `Conflicted`; if `caCertificateRefs` or `targetRefs` contain at least one entry that causes a conflict, all conflicting policies enter `Conflicted`.  | Currently supports `Service` / `ServiceImport` target, system CA / same-namespace `ConfigMap/ca.crt` custom CA, one or more `Hostname` / `URI` `subjectAltNames` combinations, and repo-declared support conditions with `Accepted` / `ResolvedRefs` reasons. When at least one of `caCertificateRefs` or `targetRefs` still has a valid reference, the control plane retains valid CA refs and exposes broken refs via `ResolvedRefs=False`. When all CA refs are invalid or unauthorized, enters `Accepted=False, Reason=InvalidCertificateRef`. When all target refs are invalid or unauthorized for all matching backends, enters `Accepted=False, Reason=InvalidTargetRef`.

### 4. Implementation-specific / Repository Extension View

| Capability or Behavior | Repository Status | Description |
| --- | --- | --- |
| `HTTPRoute` `RegularExpression` path/header/query matching | `Implemented` | Upstream marks these as `Implementation-specific`; supported by the repository runtime. |
| `GRPCRoute` `RegularExpression` header matching | `Implemented` | Uses the unified header matcher; current implementation is available. |
| `GRPCRoute` `RegularExpression` method/service matching | `Implemented` | The control plane preserves `GRPCMethodMatch.type=RegularExpression`, and the data plane uses Rust `regex` semantics for service / method matching. |
| Direct `GRPC` Listener protocol | `Implemented as internal capability, not as standard Gateway API Core declaration` | The upstream `Listener.protocol` core values do not include `GRPC`; the repository actually serves `GRPCRoute` through `HTTP` / `HTTPS` listeners. |
| `HTTP3` protocol bit and management plane visibility | `Implemented as internal placeholder, not as current standard capability declaration` | The repository IR and management plane contain `HTTP3` identifiers, but the current build does not actually enable downstream HTTP/3. |
| `Service` parent / mesh frontend / source namespace constraints | `Implemented, but not declared as passing standard Gateway profile` | The repository has additional implementation for mesh / Service parent scenarios, which is closer to repo extension and mesh profile capabilities. |

## Resource & Capability Matrix

### 1. GatewayClass / Gateway

| Object or Capability | Status | Current Scope | Primary Evidence |
| --- | --- | --- | --- |
| `GatewayClass` identification and adoption | `Implemented` | Only adopts `GatewayClass` whose `spec.controllerName` matches the current controller name. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [controlplane/internal/status/reconciler.go](../controlplane/internal/status/reconciler.go) |
| `GatewayClass` basic status write-back | `Implemented` | Currently writes back `Accepted`, `SupportedVersion`, and publishes `status.supportedFeatures`. | [controlplane/internal/status/reconciler.go](../controlplane/internal/status/reconciler.go), [controlplane/internal/status/reconciler_core_test.go](../controlplane/internal/status/reconciler_core_test.go), [status-matrix.md](status-matrix.md) |
| `Gateway` HTTP / HTTPS / TCP / UDP / TLS listeners, and GRPC over HTTP/HTTPS | `Implemented` | Control plane translates; data plane has HTTP/GRPC and stream runtime paths. | [docs/architecture.md](architecture.md), [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-http/src/runtime.rs](../dataplane/crates/aeg-http/src/runtime.rs), [dataplane/crates/aeg-stream/src/tcp.rs](../dataplane/crates/aeg-stream/src/tcp.rs), [dataplane/crates/aeg-stream/src/udp.rs](../dataplane/crates/aeg-stream/src/udp.rs) |
| `Gateway` HTTP3 listener | `Not Implemented / Not Validated` | IR and management plane have `HTTP3` protocol bits, but current build does not actually enable downstream HTTP/3. | [docs/design.md](design.md), [dataplane/crates/aeg-http/src/runtime.rs](../dataplane/crates/aeg-http/src/runtime.rs) |
| `Gateway.spec.addresses` and `Gateway.status.addresses` | `Implemented (currently declared subset)` | The control plane validates and publishes available addresses via `statusAddress` / `statusAddresses`, normalizes case and trailing `.` for Hostname before deduplication, and performs canonical deduplication and stable sorting for Service-derived addresses. If `spec.addresses[].value` is empty, it automatically assigns an available address based on the request type and writes `Programmed=False, Reason=AddressNotAssigned` when no matching address is found. For explicit IP addresses, the control-plane-managed Gateway Service syncs them to `externalIPs` and additionally sets the first `loadBalancerIP` under the `LoadBalancer` type. If explicit static IPv4/IPv6 addresses are used without explicitly configuring `ipFamilies/ipFamilyPolicy`, the control plane also automatically derives the corresponding single-stack/dual-stack family hints, and cleans up these derived fields when static addresses are revoked to prevent `Service` address family drift from `Gateway.spec.addresses`. The control plane now only publishes metadata-converged derived Gateway Service addresses to `Gateway.status.addresses`. If the Service is still undergoing ownership/metadata drift, it falls back to the global `statusAddress` / `statusAddresses`, and empty automatic allocation only selects from this published address set. Explicit static address availability validation still references the Service’s currently exposed addresses to avoid premature downgrades to `AddressNotUsable` during resource convergence. Additionally, `Gateway` only writes `Programmed=True` after the derived frontend `EndpointSlice` has been actually created and converged with the current lifecycle. The data plane consumes multiple `spec.addresses` as listener bind addresses. External allocators or non-`Service` full address orchestration models are currently not declared.
| `Gateway.spec.infrastructure` | `Partially Implemented` | Currently propagates `labels/annotations` to the control-plane-managed Gateway Service and corresponding frontend EndpointSlice. Supports `GatewayClass.spec.parametersRef` for default Service parameters, with `Gateway.spec.infrastructure.parametersRef` in the same namespace overriding `type`, `externalTrafficPolicy`, `internalTrafficPolicy`, `ipFamilyPolicy`, `ipFamilies`, `sessionAffinity`, `sessionAffinityConfig.clientIP.timeoutSeconds`, `publishNotReadyAddresses`, `externalIPs`, `loadBalancerIP`, `healthCheckNodePort`, `loadBalancerClass`, `loadBalancerSourceRanges`, and `allocateLoadBalancerNodePorts`. Derived Gateway Services and frontend EndpointSlices also receive ownership / `gatewayClassName` / `parametersRef` / effective service-parameters hash annotations; per-Gateway Service additionally gets controller `ownerReference -> Gateway`, frontend EndpointSlice gets controller `ownerReference -> Service`. When parameters are deleted or rolled back to defaults, these annotations are sync-refreshed or cleaned up. If `GatewayClass.spec.parametersRef` or `Gateway.spec.infrastructure.parametersRef` is missing, the kind is unsupported, or the content is invalid, the control plane surfaces the error in the `Gateway` status rather than keeping it only in logs. Broader infrastructure orchestration capabilities are not declared as supported.
| Listener `AllowedRoutes.kinds` | `Implemented` | Filters attachable Routes by listener protocol and explicit kinds. | [controlplane/internal/translator/attachments.go](../controlplane/internal/translator/attachments.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| Listener `AllowedRoutes.namespaces` | `Implemented` | Supports `Same`, `All`, `Selector`. | [controlplane/internal/translator/attachments.go](../controlplane/internal/translator/attachments.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| Listener hostname and Route hostname intersection | `Implemented` | Handles exact and wildcard hostname intersection and optimal listener selection. | [controlplane/internal/translator/attachments.go](../controlplane/internal/translator/attachments.go), [dataplane/crates/aeg-ir/src/http_selection.rs](../dataplane/crates/aeg-ir/src/http_selection.rs) |
| HTTPS TLS termination | `Implemented` | HTTPS listeners are served by `aeg-http`, with certificates from control-plane-delivered Secret snapshots. | [docs/design.md](design.md), [docs/architecture.md](architecture.md) |
| HTTPS SNI / Host mismatch detection | `Implemented (declared)` | HTTPS handshake captures downstream SNI. Before route selection, the data plane compares the best HTTPS listener matched by SNI and by Host. If they are disjoint, it returns `421` without connecting to upstream. Unknown SNI or unmatched Host preserves normal route / `404` semantics. | [dataplane/crates/aeg-ir/src/http_selection/candidates.rs](../dataplane/crates/aeg-ir/src/http_selection/candidates.rs), [dataplane/crates/aeg-http/src/proxy.rs](../dataplane/crates/aeg-http/src/proxy.rs), [dataplane/crates/aeg-http/src/runtime/server.rs](../dataplane/crates/aeg-http/src/runtime/server.rs) |
| `certificateRefs` pointing to Secret | `Implemented (currently declared subset)` | Only supports `group=""` and `kind=Secret`. Validates that each Secret exists and its certificate is parseable. HTTPS listener supports multiple Secret certificates under the same listener. The control plane filters unauthorized, missing, or invalid references first, then deduplicates valid references in declaration order. The data plane prioritizes SNI exact/wildcard matching and falls back to the first valid certificate in the filtered list on miss. If both valid and invalid `certificateRefs` exist in the same listener, the control plane exposes bad references via `ResolvedRefs=False`, but as long as at least one valid certificate remains, the listener stays `Programmed=True` and the corresponding infrastructure port is retained.
| Cross-namespace `certificateRefs` + `ReferenceGrant` | `Implemented` | Cross-namespace Secret references require `ReferenceGrant`. | [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| `Gateway.spec.backendTLS.clientCertificateRef` | `Implemented` | Currently supports standard `Secret` references. Same-namespace references are directly available; cross-namespace still requires `ReferenceGrant`. The data plane mounts client certificates on `HTTPS` / `GRPCS` backends. | [controlplane/internal/translator/backend_tls.go](../controlplane/internal/translator/backend_tls.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go), [dataplane/crates/aeg-http/src/proxy.rs](../dataplane/crates/aeg-http/src/proxy.rs) |
| `Gateway.spec.tls.frontend` | `Implemented (HTTPS listener scope)` | Currently reads the client CA bundle from `spec.tls.frontend.default.validation` and `spec.tls.frontend.perPort[].tls.validation` via `ConfigMap/ca.crt`, and enables frontend mTLS only on `HTTPS` listeners. `TLS` passthrough or other non-`HTTPS` listeners do not consume this configuration. Cross-namespace references still require `ReferenceGrant`. `AllowValidOnly` requires a client certificate that must pass CA validation. `AllowInsecureFallback` enables optional certificate validation, allowing connections without a certificate or with validation failure to proceed, and additionally writes back `Gateway.status.conditions[InsecureFrontendValidationMode]=True`. When at least one CA ref is still valid, the control plane retains the valid CA and exposes bad references via `ResolvedRefs=False`, while keeping the listener `Programmed=True` with infrastructure ports exposed. When all CA refs are invalid or unauthorized, the listener enters `Accepted=False, Reason=NoValidCACertificate`. | [controlplane/internal/translator/frontend_validation.go](../controlplane/internal/translator/frontend_validation.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go), [dataplane/crates/aeg-http/src/runtime.rs](../dataplane/crates/aeg-http/src/runtime.rs), [docs/user/operations.md](user/operations.md) |
| `Gateway` / Listener basic status write-back | `Implemented` | Implements basic status including `Accepted`, `Programmed`, `ResolvedRefs`, and attached routes statistics. | [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go), [controlplane/internal/status/reconciler.go](../controlplane/internal/status/reconciler.go) |
| Full status write-back matrix | `Implemented (currently declared scope)` | The currently declared supported status matrix for `GatewayClass / Gateway / Listener / Route / BackendTLSPolicy / BackendLBPolicy`, reason/message semantics, and test anchors are in the [Gateway API Status Matrix](status-matrix.md). New fields added in higher Gateway API versions still require separate auditing. | [status-matrix.md](status-matrix.md) |

### 2. HTTPRoute

| Capability | Status | Current Scope | Primary Evidence |
| --- | --- | --- | --- |
| `HTTPRoute` resource translation and delivery | `Implemented` | The control plane translates to IR and delivers to the data plane via gRPC/xDS. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [proto/gateway/control/v1/control.proto](../proto/gateway/control/v1/control.proto) |
| `parentRefs` pointing to `Gateway` | `Implemented` | Supports binding to parent listeners by `name`, `namespace`, `sectionName`, `port`. | [controlplane/internal/translator/attachments.go](../controlplane/internal/translator/attachments.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| `parentRefs` pointing to `Service` | `Implemented` | The repository supports Service parent / mesh frontend scenarios as an implemented extension capability. | [docs/design.md](design.md), [controlplane/internal/translator/mesh.go](../controlplane/internal/translator/mesh.go), [controlplane/internal/infrastructure/mesh_services.go](../controlplane/internal/infrastructure/mesh_services.go) |
| hostname matching | `Implemented` | Supports exact hostname and `*.example.com` wildcard matching. | [dataplane/crates/aeg-ir/src/http_selection.rs](../dataplane/crates/aeg-ir/src/http_selection.rs), [dataplane/crates/aeg-ir/src/tests.rs](../dataplane/crates/aeg-ir/src/tests.rs) |
| path matching | `Implemented` | `Core` `Exact` / `PathPrefix` are implemented, also supports `Implementation-specific` `RegularExpression`. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs) |
| method matching | `Implemented` | Supports HTTP method matching. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs) |
| header matching | `Implemented` | `Core` `Exact` is implemented, also supports `Implementation-specific` `RegularExpression`. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs) |
| query param matching | `Implemented` | `Extended` `Exact` is implemented, also supports `Implementation-specific` `RegularExpression`. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs) |
| Rule priority selection | `Implemented` | Selects more specific rules by combining listener hostname, route hostname, path precision, header/query count, etc. | [dataplane/crates/aeg-ir/src/http_selection.rs](../dataplane/crates/aeg-ir/src/http_selection.rs), [dataplane/crates/aeg-ir/tests/http_route_selection.rs](../dataplane/crates/aeg-ir/tests/http_route_selection.rs) |
| `RequestHeaderModifier` | `Implemented` | Control plane translates; data plane actually modifies request headers. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-http/src/filters.rs](../dataplane/crates/aeg-http/src/filters.rs) |
| `ResponseHeaderModifier` | `Implemented` | Control plane translates; data plane actually modifies response headers. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-http/src/filters.rs](../dataplane/crates/aeg-http/src/filters.rs) |
| `CORS` | `Implemented` | Currently supports `allowOrigins`, `allowMethods`, `allowHeaders`, `exposeHeaders`, `allowCredentials`, `maxAge`, and handles preflight short-circuit and formal response header write-back at the data plane. | [controlplane/internal/translator/translator_filters.go](../controlplane/internal/translator/translator_filters.go), [dataplane/crates/aeg-http/src/filters.rs](../dataplane/crates/aeg-http/src/filters.rs), [dataplane/crates/aeg-http/src/proxy.rs](../dataplane/crates/aeg-http/src/proxy.rs) |
| `RequestRedirect` | `Implemented` | Data plane can directly generate redirect responses. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-http/src/proxy.rs](../dataplane/crates/aeg-http/src/proxy.rs), [dataplane/crates/aeg-http/src/filters.rs](../dataplane/crates/aeg-http/src/filters.rs) |
| `URLRewrite` | `Implemented` | Supports host/path rewrite. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-http/src/filters.rs](../dataplane/crates/aeg-http/src/filters.rs) |
| `RequestMirror` | `Implemented` | Supports mirror backends, percentage, and fraction. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [dataplane/crates/aeg-http/src/mirror.rs](../dataplane/crates/aeg-http/src/mirror.rs) |
| `ExtensionRef` | `Partially Implemented (long-term positioned as repo-specific extension)` | Currently supports same-namespace `ConfigMap` carrier with configuration key `filter.yaml`. Implements `CORS`, `RequestHeaderModifier`, `ResponseHeaderModifier`, `RequestRedirect`, `URLRewrite`, `RequestMirror`, and HTTP-specific `DirectResponse`. If the reference is missing or configuration is invalid, the control plane exposes it via `ResolvedRefs`, and the data plane does not silently skip. The repository currently positions this capability as a long-term repo-specific extension rather than a general portable `ExtensionRef` extension model. | [controlplane/internal/extensionfilter/resolver.go](../controlplane/internal/extensionfilter/resolver.go), [controlplane/internal/status/extension_filters.go](../controlplane/internal/status/extension_filters.go), [dataplane/crates/aeg-ir/tests/extension_filters.rs](../dataplane/crates/aeg-ir/tests/extension_filters.rs), [dataplane/crates/aeg-http/src/extensions.rs](../dataplane/crates/aeg-http/src/extensions.rs) |
| `backendRefs` pointing to Service | `Implemented` | Only supports `group=""` and `kind=Service`.
| `ServicePort.appProtocol` backend protocol hint | `Implemented (currently declared subset)` | Currently falls back to `ServicePort.protocol` (default `TCP`) for backends without `appProtocol` set. Recognizes `kubernetes.io/h2c`, `kubernetes.io/grpc`, `HTTP`, `HTTP2`, `HTTPS`, `TLS`, `PROXY`, `MULTIPLEX` protocol hints used by the repository. The data plane covers `H2C`, `GRPC` success/failure, and `HTTP`/`HTTPS` coexistence on the same backend.
| `HTTPBackendRef.filters` | `Implemented (ExtensionRef is partial implementation)` | Currently preserves and executes supported HTTP filters. `ExtensionRef` inherits the same same-namespace `ConfigMap` carrier mechanism, implementing `CORS`, `RequestHeaderModifier`, `ResponseHeaderModifier`, `RequestRedirect`, `URLRewrite`, `RequestMirror`, and `DirectResponse`.
| Other `backendRef` target types | `Not Implemented / Not Validated` | Other group/kind are treated as `InvalidKind`. | [controlplane/internal/translator/backend_refs.go](../controlplane/internal/translator/backend_refs.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| Cross-namespace `backendRefs` + `ReferenceGrant` | `Implemented` | Cross-namespace Service references require `ReferenceGrant`. | [controlplane/internal/translator/backend_refs.go](../controlplane/internal/translator/backend_refs.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| `BackendRef.weight` | `Implemented` | Data plane selects backends by weighted round-robin; `weight=0` backends are skipped. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [docs/design.md](design.md) |
| healthy endpoint polling | `Implemented` | Only polls among healthy endpoints. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [docs/design.md](design.md) |
| `HTTPRouteTimeouts` | `Implemented` | Supports `request` and `backendRequest`, merged into upstream peer timeout at the data plane. | [controlplane/internal/translator/timeouts.go](../controlplane/internal/translator/timeouts.go), [dataplane/crates/aeg-http/src/proxy.rs](../dataplane/crates/aeg-http/src/proxy.rs) |
| `HTTPRoute.retry` | `Implemented` | Supports field translation, status code retry, connection failure retry, and minimum backoff. In the current implementation, if `attempts` is not explicitly set, it defaults to 1 retry. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [controlplane/internal/translator/translator_test.go](../controlplane/internal/translator/translator_test.go), [dataplane/crates/aeg-http/src/proxy.rs](../dataplane/crates/aeg-http/src/proxy.rs) |
| `HTTPRoute.sessionPersistence` | `Implemented` | Currently supports `Cookie` / `Header` transports, using stateless signed tokens to maintain sessions among backends matched by the rule. If the target backend is no longer available, it falls back to normal weight-based selection and reissues a new token. Production environments should configure a stable shared secret for the data plane. | [controlplane/internal/translator/session_persistence.go](../controlplane/internal/translator/session_persistence.go), [controlplane/internal/grpcserver/session_persistence.go](../controlplane/internal/grpcserver/session_persistence.go), [dataplane/crates/aeg-http/src/session.rs](../dataplane/crates/aeg-http/src/session.rs), [dataplane/crates/aeg-ir/tests/session_persistence.rs](../dataplane/crates/aeg-ir/tests/session_persistence.rs) |
| HTTPRoute basic status write-back | `Implemented` | Supports `Accepted`, `ResolvedRefs` and other basic parent status. | [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go), [controlplane/internal/status/reconciler.go](../controlplane/internal/status/reconciler.go) |

### 3. GRPCRoute

| Capability | Status | Current Scope | Primary Evidence |
| --- | --- | --- | --- |
| `GRPCRoute` resource translation and delivery | `Implemented` | Control plane translates to IR; data plane matches by gRPC routing rules. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [proto/gateway/control/v1/control.proto](../proto/gateway/control/v1/control.proto) |
| `parentRefs` pointing to `Gateway` | `Implemented` | Supports binding to parent listeners by `name`, `namespace`, `sectionName`, `port`, and serving on HTTP/HTTPS listeners. | [controlplane/internal/translator/attachments.go](../controlplane/internal/translator/attachments.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go), [dataplane/crates/aeg-ir/src/http_selection.rs](../dataplane/crates/aeg-ir/src/http_selection.rs) |
| `parentRefs` pointing to `Service` | `Implemented` | Like HTTPRoute, the repository supports Service parent / mesh frontend scenarios and generates corresponding mesh frontend Service for this type of parent reference. | [controlplane/internal/translator/mesh.go](../controlplane/internal/translator/mesh.go), [controlplane/internal/infrastructure/mesh_services.go](../controlplane/internal/infrastructure/mesh_services.go), [controlplane/internal/mesh/routes.go](../controlplane/internal/mesh/routes.go) |
| hostname matching | `Implemented` | Supports exact and wildcard hostname in candidate listener / route selection. | [dataplane/crates/aeg-ir/src/http_selection.rs](../dataplane/crates/aeg-ir/src/http_selection.rs) |
| gRPC `service` / `method` matching | `Implemented` | `Core` `Exact(service+method)` is implemented. The current implementation also preserves and executes `Implementation-specific` `RegularExpression` semantics. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [dataplane/crates/aeg-ir/src/tests.rs](../dataplane/crates/aeg-ir/src/tests.rs), [dataplane/crates/aeg-ir/tests/grpc_route_matching.rs](../dataplane/crates/aeg-ir/tests/grpc_route_matching.rs) |
| header matching | `Implemented` | `Core` `Exact` is implemented, also uses the unified matcher for `Implementation-specific` `RegularExpression`. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs) |
| Serving gRPC on HTTP / HTTPS listeners | `Implemented` | Cleartext listener defaults to h2c, TLS listener defaults to ALPN h2. GRPCRoute can attach to both HTTP and HTTPS listeners. Current repository-level automation covers h2/gRPC, h2c, h2/gRPC+h2c passthrough, h2/gRPC+h2c propagation, and downstream error detection on mid-stream disconnection.
| `RequestHeaderModifier` | `Implemented` | Translation and runtime support exist. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-http/src/filters.rs](../dataplane/crates/aeg-http/src/filters.rs) |
| `ResponseHeaderModifier` | `Implemented` | Translation and runtime support exist. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-http/src/filters.rs](../dataplane/crates/aeg-http/src/filters.rs) |
| `RequestMirror` | `Implemented` | Translation and data plane mirroring logic exist. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [dataplane/crates/aeg-http/src/mirror.rs](../dataplane/crates/aeg-http/src/mirror.rs) |
| `ExtensionRef` | `Partially Implemented (long-term positioned as repo-specific extension)` | Currently supports same-namespace `ConfigMap` carrier with configuration key `filter.yaml`. Implements `CORS`, `RequestHeaderModifier`, `ResponseHeaderModifier`, `RequestRedirect`, `URLRewrite`, `RequestMirror`, and `DirectResponse`.
| `backendRefs` Service / `ReferenceGrant` / weight | `Implemented` | Like HTTPRoute, only supports Service, with cross-namespace authorization and weight support. | [controlplane/internal/translator/backend_refs.go](../controlplane/internal/translator/backend_refs.go), [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs) |
| `GRPCBackendRef.filters` | `Implemented (ExtensionRef is partial implementation)` | Control plane extracts filters and merges them with rule-level filters during backend selection. `ExtensionRef` currently supports same-namespace `ConfigMap` carrier with `CORS`, `RequestHeaderModifier`, `ResponseHeaderModifier`, `RequestRedirect`, `URLRewrite`, `RequestMirror`, and `DirectResponse`.
| `GRPCRoute.sessionPersistence` | `Implemented` | Uses the same stateless signed token mechanism as `HTTPRoute.sessionPersistence`, and prefers backends that are still healthy and still referenced by the rule. Currently supports `Cookie` / `Header` transports. Production environments should configure a stable shared secret for the data plane. | [controlplane/internal/translator/session_persistence.go](../controlplane/internal/translator/session_persistence.go), [controlplane/internal/grpcserver/session_persistence.go](../controlplane/internal/grpcserver/session_persistence.go), [dataplane/crates/aeg-http/src/session.rs](../dataplane/crates/aeg-http/src/session.rs), [dataplane/crates/aeg-ir/tests/session_persistence.rs](../dataplane/crates/aeg-ir/tests/session_persistence.rs) |
| GRPCRoute basic status write-back | `Implemented` | Supports basic `Accepted` / `ResolvedRefs` results. | [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go), [controlplane/internal/status/reconciler.go](../controlplane/internal/status/reconciler.go) |

### 4. TCPRoute / UDPRoute / TLSRoute

| Capability | Status | Current Scope | Primary Evidence |
| --- | --- | --- | --- |
| `TCPRoute` resource translation and stream runtime | `Implemented` | Uses independent `aeg-stream` runtime, bound to backends by listener. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-stream/src/tcp.rs](../dataplane/crates/aeg-stream/src/tcp.rs) |
| `UDPRoute` resource translation and stream runtime | `Implemented` | Uses independent `aeg-stream` runtime, forwards datagrams by listener / port. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-stream/src/udp.rs](../dataplane/crates/aeg-stream/src/udp.rs) |
| `TLSRoute` resource translation and passthrough runtime | `Implemented` | Data plane reads ClientHello SNI and performs L4 passthrough selection according to `TLSRoute`. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-stream/src/tcp.rs](../dataplane/crates/aeg-stream/src/tcp.rs), [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs) |
| stream route `parentRefs` pointing to `Gateway` | `Implemented` | `TCPRoute`, `UDPRoute`, `TLSRoute` all support binding to parent listeners by `name`, `namespace`, `sectionName`, `port`.o parent listeners by `name`, `namespace`, `sectionName`, `port`. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [controlplane/internal/translator/attachments.go](../controlplane/internal/translator/attachments.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| stream route `parentRefs` pointing to `Service` | `Implemented` | Like L7 Routes, these three stream Route types also support Service parent / mesh frontend scenarios and participate in mesh frontend route generation.
| `TCPRoute` / `UDPRoute` listener-level matching | `Implemented` | Currently selects by attached listener. Individual rules do not add extra L4 matching conditions. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [dataplane/crates/aeg-ir/src/tests.rs](../dataplane/crates/aeg-ir/src/tests.rs) |
| `TLSRoute` SNI hostname matching | `Implemented` | Uses ClientHello SNI for selection, supports exact hostname and `*.example.com` wildcard matching. | [controlplane/internal/translator/translator.go](../controlplane/internal/translator/translator.go), [dataplane/crates/aeg-stream/src/sni.rs](../dataplane/crates/aeg-stream/src/sni.rs), [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [dataplane/crates/aeg-ir/src/tests.rs](../dataplane/crates/aeg-ir/src/tests.rs) |
| `TLSRoute` and HTTPS termination boundary | `Implemented` | `TLSRoute` handles only passthrough. HTTPS listener handles HTTP/GRPC routing after TLS termination. | [docs/design.md](design.md), [docs/architecture.md](architecture.md) |
| `TLSRoute` mixed mode (`LISTENER_PROTOCOL_TLS`) | `Implemented (data plane)` | When the TLS listener has `mode: Terminate`, the control plane maps it to `LISTENER_PROTOCOL_TLS`. The data plane provides both passthrough and terminate surfaces on the same port (shared TLS listener plan). `StreamMatch.mode` has a reserved field but is not yet used for route selection.
| stream route `backendRefs` Service only | `Implemented` | Only supports Service backends. | [controlplane/internal/translator/backend_refs.go](../controlplane/internal/translator/backend_refs.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| stream route `ReferenceGrant` | `Implemented` | Cross-namespace Service backends still require `ReferenceGrant`. | [controlplane/internal/translator/backend_refs.go](../controlplane/internal/translator/backend_refs.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| stream route `BackendRef.weight` | `Implemented` | Stream backend selection also uses weighted logic. | [dataplane/crates/aeg-ir/src/lib.rs](../dataplane/crates/aeg-ir/src/lib.rs), [dataplane/crates/aeg-ir/src/tests.rs](../dataplane/crates/aeg-ir/src/tests.rs) |
| TCPRoute / UDPRoute / TLSRoute basic status write-back | `Implemented` | Control plane writes back basic Route status. | [docs/architecture.md](architecture.md), [controlplane/internal/status/reconciler.go](../controlplane/internal/status/reconciler.go) |
| `UDPRoute` official conformance result | `Validated (currently declared support scope)` | Gateway API `v1.5.1` upstream harness includes `UDPRoute` feature and `UDPRoute` test cases. In the most recent `latest` archive, `TestGatewayAPIConformance/UDPRoute` has `PASS`. | [tests/conformance/run.sh](../tests/conformance/run.sh), [reports/conformance/README.md](../reports/conformance/README.md) |
| `TCPRoute` official conformance result | `No upstream tests, repository supplements validation` | Gateway API `v1.5.1` upstream `pkg/features` does not have `SupportTCPRoute`, and `conformance/tests` has no TCPRoute test cases, so official reports will not include `TCPRoute`. The repository uses supplemental conformance-style tests to cover data plane listener attachment, protocol isolation, listener port matching, missing-backend, healthy endpoint selection, and control plane cross-namespace Service backend `ReferenceGrant` deny/allow semantics. Kind smoke continues to cover the `tcp-echo` success path and the `tcp-missing` missing-backend failure path. `scripts/check-stream-route-test-coverage.sh` pins these evidence boundaries. | [dataplane/crates/aeg-ir/src/tests_selection/stream_and_fallback/stream_routes/tcp.rs](../dataplane/crates/aeg-ir/src/tests_selection/stream_and_fallback/stream_routes/tcp.rs), [controlplane/internal/translator/tcproute_conformance_test.go](../controlplane/internal/translator/tcproute_conformance_test.go), [controlplane/internal/status/tcproute_conformance_test.go](../controlplane/internal/status/tcproute_conformance_test.go), [tests/e2e/smoke.yaml](../tests/e2e/smoke.yaml), [tests/e2e/run-kind.sh](../tests/e2e/run-kind.sh), [scripts/check-stream-route-test-coverage.sh](../scripts/check-stream-route-test-coverage.sh) |
| `TLSRoute` official conformance result | `Validated (currently declared support scope)` | Currently declares TLSRoute core passthrough, `TLSRouteModeMixed` declared, data plane mixed mode, SNI wildcard and exact matching, and control plane listener attachment and status semantics.

### 5. ReferenceGrant

| Capability | Status | Current Scope | Primary Evidence |
| --- | --- | --- | --- |
| Route cross-namespace reference to Service backend | `Implemented` | `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, `TLSRoute` all check `ReferenceGrant`. | [controlplane/internal/translator/backend_refs.go](../controlplane/internal/translator/backend_refs.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| Gateway cross-namespace reference to TLS Secret | `Implemented` | `certificateRefs` pointing to Secrets in other namespaces require `ReferenceGrant`. | [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |
| General cross-resource authorization matrix | `Implemented (covers all cross-namespace references currently declared supported)` | In addition to Route -> Service backend and Gateway -> Secret `certificateRefs`, `Gateway.spec.tls.frontend` CA `ConfigMap` references and `Gateway.spec.backendTLS.clientCertificateRef` cross-namespace `Secret` references also check `ReferenceGrant`. | [controlplane/internal/translator/backend_refs.go](../controlplane/internal/translator/backend_refs.go), [controlplane/internal/translator/backend_tls.go](../controlplane/internal/translator/backend_tls.go), [controlplane/internal/translator/frontend_validation.go](../controlplane/internal/translator/frontend_validation.go), [controlplane/internal/status/evaluator.go](../controlplane/internal/status/evaluator.go) |

### 6. Backend Policy

| Capability | Status | Current Scope | Primary Evidence |
| --- | --- | --- | --- |
| `BackendLBPolicy.sessionPersistence` | `Implemented` | Currently supports `Service` / `ServiceImport` targets, delivering backend-level sticky session to the data plane. The control plane writes back `Accepted` / `ResolvedRefs`, and resolves conflicts on the same backend by creation time first, then name lexicographic order. | [controlplane/internal/translator/backend_lb_policy.go](../controlplane/internal/translator/backend_lb_policy.go), [controlplane/internal/status/backend_lb_policy.go](../controlplane/internal/status/backend_lb_policy.go), [dataplane/crates/aeg-ir/tests/session_persistence.rs](../dataplane/crates/aeg-ir/tests/session_persistence.rs), [docs/user/operations.md](user/operations.md) |
| `BackendLBPolicy.loadBalancing.consistentHash` | `Implemented (repo-specific extension)` | Currently supports `Service` / `ServiceImport` targets, delivering backend-level consistent hashing to the data plane. Supports `Header` / `SourceIP` / `Hostname` three hash key types, and performs stable selection by the same hash key at both backend and endpoint layers. When hash key is missing or backend load balancing policies referenced by the same route are inconsistent, falls back to existing weighted round-robin. | [controlplane/internal/translator/backend_lb_policy.go](../controlplane/internal/translator/backend_lb_policy.go), [controlplane/internal/status/backend_lb_policy.go](../controlplane/internal/status/backend_lb_policy.go), [controlplane/internal/grpcserver/load_balancing.go](../controlplane/internal/grpcserver/load_balancing.go), [dataplane/crates/aeg-ir/src/tests_load_balancing.rs](../dataplane/crates/aeg-ir/src/tests_load_balancing.rs), [docs/user/operations.md](user/operations.md) |
| `BackendTLSPolicy` | `Implemented (converged with upstream Rust proxy/OpenSSL current capabilities)` | Currently supports `Service` / `ServiceImport` target, system CA / same-namespace `ConfigMap/ca.crt` custom CA, one or more `Hostname` / `URI` `subjectAltNames` combinations, and repository-declared support conditions with `Accepted` / `ResolvedRefs` reasons. When at least one of `caCertificateRefs` or `targetRefs` still has a valid reference, the control plane retains valid CA refs and exposes broken refs via `ResolvedRefs=False`. When all CA refs are invalid or unauthorized, enters `Accepted=False, Reason=InvalidCertificateRef`. When all target refs are invalid or unauthorized for all matching backends, enters `Accepted=False, Reason=InvalidTargetRef`.

### 7. Admin API Management Plane

| Capability | Status | Current Scope | Primary Evidence |
| --- | --- | --- | --- |
| Control plane admin API for Gateway API resource management | `Implemented` | Currently `GET/POST/PUT/DELETE /v1/resources` and `GET /v1/resource-kinds` are implemented, supporting read and operation of Gateway API core resource types.

## Current Known Gaps

The following points have clear evidence showing they cannot yet be considered "fully supported" by the current repository:

### Gateway API v1.5.1 upstream features not yet declared

The following upstream features are still not declared as supported:

- `HTTP3 / QUIC` downstream listener capability is not implemented; currently only protocol bits and status exposure are retained.
- `BackendLBPolicy` currently only declares support for upstream `sessionPersistence` plus the repository-custom `loadBalancing.consistentHash` subset. Do not treat other backend load balancing policy fields as supported.
- `BackendTLSPolicy.spec.options` is an implementation-specific extension point. The repository no longer declares support for `gateway.nantian.dev/backend-tls-min-version` / `gateway.nantian.dev/backend-tls-max-version`. Any other custom options should also not be considered supported.
- `HTTPRoute` experimental `ExternalAuth` filter / GEP-1494 currently supports only the `protocol: HTTP` subset (Phase 1): usable at both rule-level and backendRef-level. The control plane translates auth `backendRef`, HTTP `path` / `allowedHeaders` / `allowedResponseHeaders`. The data plane calls the HTTP auth backend before forwarding; `2xx` passes, non-`2xx` rejects, unreachable/timeout/protocol errors fail-closed (returns 5xx). `protocol: GRPC` and `forwardBody.maxSize > 0` are currently explicitly rejected. BackendTLSPolicy combination validation and Kind/conformance evidence are not yet complete, so it is temporarily not added to `GatewayClass.status.supportedFeatures`.
- `Default Gateways` / GEP-3793 has control plane and translator unit test coverage, and Kind smoke manifest has been added. However, it remains an experimental field subset, not among the current 55 `supportedFeatures` declarations, and has no official conformance or production-grade evidence.
- `ExtensionRef` currently only supports same-namespace `ConfigMap` carrier and a subset of implementation-specific filters: `CORS`, `RequestHeaderModifier`, `ResponseHeaderModifier`, `RequestMirror`, and HTTP-specific `RequestRedirect`, `URLRewrite`, `DirectResponse`. The repository currently positions this long-term as a repo-specific extension, not a general extension framework.
- Further Gateway API experimental expansion has completed phase assessment. The current decision is not to add new supportedFeatures declarations. See [Gateway API Experimental Feature Audit](backlog/gateway-api-experimental.md).
- `Gateway.spec.addresses` and `Gateway.spec.infrastructure` only have partial capabilities implemented and should not be considered full Extended support.
- The repository does not guarantee "complete Extended feature support."
- The repository does not guarantee "Full status write-back matrix."
- The repository does not guarantee "production-grade certificate rotation."
- The repository does not guarantee "full performance optimization."
- Without a configured stable shared secret, `sessionPersistence` degrades to a temporary key generated at data plane process startup, suitable only for local debugging, not for multi-replica or restart-stable production deployments.
- Local `tests/conformance/run.sh` still defaults to the `GATEWAY-HTTP` profile for low-cost debugging rather than full-suite. Explicitly set `ALL_FEATURES=true` for releases or final validation.
- The repository has archived the `2026-05-14` `90f5126a` full-suite result as the most recent clean full-suite baseline. Future externally referenced commits will still need to re-run the official Gateway API conformance on the corresponding commit.
- Gateway API `v1.5.1` official conformance does not cover `TCPRoute`: upstream has no `SupportTCPRoute` feature or TCPRoute test files.The repository’s current validation for `TCPRoute` comes from supplemental conformance-style data plane/control plane tests, and Kind smoke success and missing-backend failure paths, rather than the official conformance report.
- `24h soak + failure injection + performance` on the current commit have not been fully consolidated as repository evidence for the same candidate commit. HTTP/3 / QUIC and TLSRoute mixed mode are still treated as unimplemented or undeclared gaps.
- Canary `GatewayClass` and rollback scripts now have repository entry points, but still need to integrate namespace, rollout scope, and observation thresholds into the release process based on the actual environment.

Primary evidence:

- [docs/architecture.md](architecture.md)
- [docs/design.md](design.md)
- [controlplane/conformance/conformance_test.go](../controlplane/conformance/conformance_test.go)
- [tests/conformance/run.sh](../tests/conformance/run.sh)
- [scripts/prepare-canary-gatewayclass.sh](../scripts/prepare-canary-gatewayclass.sh)
- [scripts/rollback-canary-gatewayclass.sh](../scripts/rollback-canary-gatewayclass.sh)

## Production Usage Recommendations

If the goal is a controlled production or long-running environment, it is recommended to treat the current project as an implementation that "can be used within clear boundaries" rather than a "complete Gateway API implementation":

- Trustworthy components: HTTP / GRPC basic L7 forwarding, TCP / UDP / TLS passthrough, basic Gateway / Route status, management interface, xDS hot-reload, admin authentication, control channel TLS/mTLS.
- Components to supplement before deployment: full conformance report, stress testing and fault injection, certificate rotation plan, missing policy capabilities, and validation of the Gateway API feature subset actually needed by your environment.

Recommended wording for documentation, reviews, or external communication:

- `It is NOT recommended to describe the current project as "production-ready."`
- `A more accurate description: the project has the foundation for long-running operation in controlled environments, but overall remains partially implemented, suitable for trial within clear boundaries or gradual production adoption.`

## Maintenance Recommendations

If new Gateway API capabilities are added later, it is recommended to update this document in at least three places:

1. Resource and capability matrix.
2. Current known gaps.
3. Boundary descriptions in production usage recommendations.
