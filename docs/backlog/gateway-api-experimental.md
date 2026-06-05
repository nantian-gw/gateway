# Gateway API Experimental Feature Audit

This page documents the evaluation results of the current repository against Gateway API `v1.5.1` experimental capabilities.
Its goal is not to continue expanding support claims, but to separate “supported experimental capabilities,” “capabilities that only implement a subset of the repo,” and “explicitly deferred capabilities,” avoiding unbounded scope expansion before `P0/P1` items are cleared.

## Evaluation Source

- Current control plane dependency: `sigs.k8s.io/gateway-api v1.5.1`
- Upstream feature metadata: `pkg/features`
- Repo declaration source: `controlplane/internal/gatewayapi/supported_features.go`
- Current support matrix: `docs/gateway-api-support.md`
- Recent evidence boundaries:
  - latest conformance archive: `reports/conformance/runs/2026-05-12-0355945e-kind-mesh-profile-current/`
  - Most recent clean full-suite baseline: `reports/conformance/runs/2026-05-08-3af22b42-full/`

Features currently marked as `FeatureChannelExperimental` in `pkg/features`:

| Feature | Current Conclusion | Action |
| --- | --- | --- |
| `HTTPRouteDestinationPortMatching` | Declared, implemented, included in supportedFeatures | Maintain current declaration; only regression verification going forward, no new expansion. |
| `UDPRoute` | Declared, implemented, with official conformance and kind smoke evidence | Maintain current declaration; production-grade throughput, packet loss, and long-stability evidence still tracked via performance/soak backlog. |
| `ListenerSet` | Partially implemented, currently not declared | Retain CRD/RBAC/watch and translator listener merge foundation; do not add to `supportedFeatures` yet; must complete status, conflict, attachment, and conformance evidence before declaring. |
| `TLSRouteModeTerminate` | Declared, translator implemented | Already added to `supportedFeatures`; TLSRoute + mode=Terminate → translated to L7 HTTPRoute |

## Existing Experimental Channel or Experimental Field Subsets

The following capabilities already exist in the repository, but this does not mean they can automatically extend to the full upstream experimental surface:

| Capability | Current State | Boundary |
| --- | --- | --- |
| `TCPRoute` | Implemented, covered by kind smoke + unit, not declared as official conformance feature | Gateway API `v1.5.1` has no upstream `SupportTCPRoute` feature or TCPRoute conformance test cases; continue to handle as repo supplementary validation. |
| `TLSRoute` passthrough | Currently declared as core passthrough + `TLSRouteModeMixed` + `TLSRouteModeTerminate` subset | `TLSRouteModeTerminate` is declared; TLSRoute on terminate listener is translated to L7 HTTPRoute. |
| `BackendLBPolicy` | Implemented `sessionPersistence` + repo-specific `loadBalancing.consistentHash` subset | Do not declare full BackendLBPolicy; other load balancing fields require separate design, status semantics, and test evidence. |
| `HTTPRoute.retry` | Implemented current request retry subset | Continue maintaining as dataplane policy capability; do not automatically extend to unaudited policy fields. |
| `HTTPRoute.sessionPersistence` / `GRPCRoute.sessionPersistence` | Implemented current sticky session subset | Multi-replica production usage requires configuring a stable shared secret; do not promote to production-validated without long-term restart/rotation evidence. |
| `Gateway.spec.tls.frontend` | Implemented frontend mTLS subset scoped to HTTPS listener | Only applies to HTTPS listener; TLS passthrough does not consume this configuration. |
| `Gateway.spec.backendTLS.clientCertificateRef` | Implemented current Secret reference subset | Cross-namespace still requires `ReferenceGrant`; certificate sources beyond Secret are not within current scope. |
| `Service` parent / mesh frontend | Currently declared as mesh profile subset | Evidence from kind mesh profile and targeted conformance; still missing multi-environment east-west long-stability evidence. |
| Default Gateways / GEP-3793 | Implemented implicit Gateway parent subset for `defaultScope=All` and `useDefaultGateways=All` | Control plane synthesizes Route parent status, Gateway `DefaultGateway` condition, and translator IR parentRef; currently only covered by unit tests, not declared in supportedFeatures, and not promoted to conformance or production-validated. |

## Deferred Items

These areas include experimental features, experimental channel resources, and related capabilities that would expand the Gateway API support surface; they should not directly enter `GatewayClass.status.supportedFeatures` or release gates at the current stage:

| Deferred Item | Reason | Re-evaluation Condition |
| --- | --- | --- |
| `HTTPRoute` `ExternalAuth` / GEP-1494 HTTP Auth | Implemented rule-level + backendRef-level `protocol: HTTP` and `protocol: GRPC` subset, including control plane translation, dataplane HTTP auth backend calls, Envoy ext_authz-compatible gRPC `Authorization/Check`, allow/deny/error mapping, HTTP mandatory `Authorization` forwarding, HTTP allowed response header injection, GRPC `allowedHeaders` filtering, bounded body forwarding with `forwardBody.maxSize > 0`, backend replay after allow, and overflow `413`; BackendTLSPolicy combination and Kind/conformance evidence not yet complete. | Continue implementing BackendTLS combination, status reference refinement, and targeted e2e/conformance; declare supportedFeatures only after evidence is complete. |
| More complete `BackendLBPolicy` fields | Currently only validated session persistence and consistent hash subset | Design upstream fields one by one, clarifying conflict, inheritance, status, and dataplane behavior. |
| General `ExtensionRef` framework | Currently only supports same-namespace `ConfigMap` carrier and a few repo-specific filters | Requires security model, schema, version compatibility, and failure isolation design. |
| HTTP/3 / QUIC | Not just a Gateway API feature declaration issue; also requires dataplane protocol stack, listeners, TLS/QUIC certificates, and stress test evidence | Separate proposal after production performance, soak, and release gates are stable. |

## Current Decision

As of `2026-05-12`, no new Gateway API experimental declarations will be added.

Maintenance and regression verification of already implemented subsets are permitted:

- `HTTPRouteDestinationPortMatching`
- `HTTPRoute ExternalAuth` HTTP / GRPC auth backend subset
- `UDPRoute`
- `TCPRoute` repo supplementary implementation
- `TLSRoute` passthrough
- `BackendLBPolicy` current subset
- retry / session persistence / frontend mTLS / backend client cert / mesh profile current subset
- Default Gateways / GEP-3793 current subset

Other experimental or related Gateway API support surface expansions must first satisfy:

1. Have an independent design or backlog entry, clearly specifying the four-layer goal of `declared / implemented / tested / production-validated`.
2. Have a minimum unit/e2e/targeted conformance validation plan.
3. Does not conflict with `24h` soak, release evidence, Gateway API support matrix, and current production boundaries.
4. Support matrix, backlog, and conformance evidence updated synchronously.

This means “Evaluate more Gateway API Experimental features” in `ROADMAP.md` can be closed as “Evaluated and not expanding for now”; truly new capabilities should still enter as separate backlog items or proposals, rather than reusing this umbrella entry.
