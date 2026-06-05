# Kubernetes Gateway API Compliance Analysis

This document makes judgments based on current repository code, test wiring, and archived conformance evidence, not solely on repository documentation declarations.

The analysis corresponds to the current repository commit: `2767574`.

## Conclusion

- The current project is **not** "fully compliant with all Kubernetes Gateway API requirements."
- The current project has implemented most of the important Gateway API capabilities, especially core gateways, mainstream L7/L4 routing, ReferenceGrant, BackendTLSPolicy, and some mesh / service parent capabilities.
- However, from the code, there are clearly partial implementations and unimplemented items, so it should not be described as "complete implementation of all Gateway API requirements."

## Tiered Assessment

### Core

- `GatewayClass / Gateway` basic takeover and status: implemented.
- `Gateway` listener core protocols `HTTP / HTTPS / TLS / TCP / UDP`: implemented.
- Listener conflict detection, basic TLS validity, `ResolvedRefs / Programmed`: implemented.
- `ReferenceGrant`: implemented.
- `HTTPRoute` core body: implemented.
- `GRPCRoute` core body: implemented.
- `TLSRoute` core body: implemented.
- `BackendTLSPolicy` core: implemented.

Assessment: Core is mostly implemented, but this does not warrant claiming "Core 100% fully covered without gaps."

### Extended

- `HTTPRoute` method/query/header, response mods/redirect/rewrite/mirror/timeouts: mostly implemented.
- `GRPCRoute` major extension filter subset: partially implemented.
- `BackendTLSPolicy SAN validation`: implemented.
- `Mesh / Service parent` main path: implemented.

Explicit downgrade points:

- `ExtensionRef`: partially implemented, currently only covers a specific set of implementation-specific filters, not a general extension mechanism.
- `BackendLBPolicy`: partially implemented, currently only `sessionPersistence` is observed.

Assessment: Extended has substantial implementation, but not complete.

### Experimental

- `UDPRoute`: implemented.
- `TCPRoute`: implemented.
- `sessionPersistence`: implemented.
- `HTTP retry`: implemented.
- Partial mesh/profile capabilities: implemented.

Explicit gaps:

- `HTTP/3`: not implemented; the runtime explicitly returns unavailable.
- Overall experimental capability coverage cannot be considered complete.

Assessment: Experimental is only partially implemented.

## Key Evidence

### 1. The repository's declared support surface is wider than actual implementation

The repository once directly declared its supported features as upstream's `AllFeatures + UDPRoute`, but has now tightened to an explicitly enumerated feature subset:

- `controlplane/internal/gatewayapi/supported_features.go`

This shows the repository has started converging external declarations toward actual implementation, but subsequent code checks still reveal that some capabilities are only partially implemented or still missing.

### 2. Official conformance wiring is genuinely present

The repository has indeed integrated with the Gateway API official conformance harness:

- `controlplane/conformance/conformance_test.go`
- `tests/conformance/run.sh`

And the repository preserves `ALL_FEATURES=true` archived passing records:

- `reports/conformance/latest/metadata.yaml`

But this only proves archived results for a specific commit, not automatically proving that current `HEAD` is fully compliant.

### 3. Current HEAD is not the same commit as the most recent archived passing commit

- Current repository `HEAD` at analysis time: `2767574`
- `implementationVersion` in the most recent archived passing record: `a061d62`

Therefore, historical full-suite results cannot be used directly as sufficient evidence that the current code "fully meets all requirements."

## Capability Audit

### GatewayClass / Gateway

Implemented:

- `GatewayClass` basic takeover and status writeback.
- `GatewayClass.status.supportedFeatures` publishing.
- `Gateway` listener basic protocols `HTTP / HTTPS / TLS / TCP / UDP`.
- Listener conflict checking, basic TLS validation, `ResolvedRefs`/`Programmed` conditions.
- `AllowedRoutes`, namespace constraints, route kind constraints.

Not implemented or cannot be assessed as complete:

- `HTTP/3 listener` not implemented.

Related code:

- `controlplane/internal/status/reconciler.go`
- `controlplane/internal/status/evaluator_gateways.go`
- `controlplane/internal/translator/attachments.go`
- `dataplane/crates/aeg-http/src/runtime.rs`

### HTTPRoute

Implemented:

- Basic resource translation and dispatch.
- `hostname / path / header / query / method` matching.
- `RequestHeaderModifier`.
- `ResponseHeaderModifier`.
- `RequestRedirect`.
- `URLRewrite`.
- `RequestMirror`.
- `timeouts`.
- `retry`.
- `sessionPersistence`.

Partially implemented:

- `ExtensionRef` still only supports the declared filter subset on the data plane, not a general complete implementation.

Related code:

- `controlplane/internal/translator/translator_routes.go`
- `controlplane/internal/translator/translator_filters.go`
- `controlplane/internal/extensionfilter/resolver.go`
- `dataplane/crates/aeg-http/src/filters.rs`
- `dataplane/crates/aeg-http/src/extensions.rs`
- `dataplane/crates/aeg-http/src/session.rs`

### GRPCRoute

Implemented:

- Basic resource translation and dispatch.
- `service / method / header` matching.
- Carrying gRPC on `HTTP/HTTPS` listeners.
- `RequestHeaderModifier`.
- `ResponseHeaderModifier`.
- `RequestMirror`.
- `sessionPersistence`.

Partially implemented:

- `ExtensionRef` is still limited to the currently declared supported extension filter subset.

Related code:

- `controlplane/internal/translator/translator_routes.go`
- `controlplane/internal/translator/translator_filters.go`
- `dataplane/crates/aeg-http/src/runtime.rs`
- `dataplane/crates/aeg-http/src/extensions.rs`

### TCPRoute / TLSRoute / UDPRoute

Implemented:

- `TCPRoute`.
- `TLSRoute` passthrough.
- `UDPRoute`.

No complete evidence:

- Advanced filter capabilities for stream routes.

Related code:

- `controlplane/internal/translator/translator_routes.go`
- `dataplane/crates/aeg-stream/src/tcp.rs`
- `dataplane/crates/aeg-stream/src/udp.rs`
- `dataplane/crates/aeg-stream/src/sni.rs`

### ReferenceGrant

Implemented:

- Cross-namespace backend reference authorization.
- Cross-namespace certificate, CA, and client cert reference authorization.

Related code:

- `controlplane/internal/translator/backend_refs.go`
- `controlplane/internal/translator/frontend_validation.go`
- `controlplane/internal/status/evaluator_gateways.go`

### BackendTLSPolicy

Implemented:

- Basic `BackendTLSPolicy` translation.
- `hostname` validation.
- System CA and `ConfigMap` CA bundle.
- SAN validation.
- Data plane upstream TLS validation execution.

Related code:

- `controlplane/internal/translator/backend_tls_policy.go`
- `dataplane/crates/aeg-http/src/proxy/backend.rs`

### BackendLBPolicy

Partially implemented:

- Currently only `sessionPersistence` related translation and activation are observed.
- No broader load balancing strategy implementation evidence is observed.

Related code:

- `controlplane/internal/translator/backend_lb_policy.go`

### Mesh / Service Parent

Implemented:

- `Service parent` main path.
- Mesh frontend / source namespace related main paths.

Note:

- These capabilities demonstrate a broad implementation surface, but they are more repo extension capabilities and do not equate to "all Gateway API requirements are met."

Related code:

- `controlplane/internal/translator/mesh.go`
- `dataplane/crates/aeg-ir/src/snapshot.rs`

## Key Points Preventing a "Fully Compliant" Conclusion

- Although the repository has tightened the `supportedFeatures` declaration, actual capabilities still include partially implemented items.
- `BackendLBPolicy` only shows the `sessionPersistence` subset.
- `ExtensionRef` is still not a general complete implementation on the data plane.
- `HTTP/3` is not implemented.
- Historical full-suite passing does not correspond to current `HEAD`.

## Final Conclusion

The most accurate description of the current project should be:

- Has implemented most Gateway API Core capabilities;
- Has implemented a significant number of Extended and Experimental capabilities;
- But still has partial implementations and unimplemented items;
- Therefore should not be described as "fully compliant with all Kubernetes Gateway API requirements" or "complete implementation of all Gateway API requirements."