# IR Layering

This document defines the IR layering boundaries for Aether Gateway. The goal is to prevent Kubernetes input models, cross-process transport models, and data plane runtime indices from continuing to intrude on each other.

In this document, IR is not a single structure, but a three-layer model:

1. Control plane input IR
2. Publish IR / proto snapshot
3. Data plane runtime IR

These three layers may share conceptual names such as `Listener`, `Route`, `Backend`, but their responsibilities and mutable state ownership differ.

## 1. Control Plane Input IR

The control plane input IR is oriented toward Kubernetes resource aggregation, Gateway API semantic normalization, and status write-back.

Primary code locations:

- `controlplane/internal/translator/`
- `controlplane/internal/ir/`
- `controlplane/internal/status/`
- `controlplane/internal/infrastructure/`

Input sources:

- Gateway API resources: `GatewayClass`, `Gateway`, `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, `TLSRoute`, `ReferenceGrant`
- Policy resources: `BackendTLSPolicy`, `BackendLBPolicy`
- Kubernetes supporting resources: `Service`, `ServiceImport`, `EndpointSlice`, `Secret`, `ConfigMap`, `Namespace`, `Pod`
- Control plane local state: dataplane node ACK / readiness, infrastructure ownership, mesh frontend derived resource status

Responsibilities:

- Resolve `parentRefs`, `backendRefs`, `ReferenceGrant`, and policy target refs.
- Normalize Gateway API defaults and implementation boundaries.
- Retain Kubernetes object-level context for status write-back, such as namespace, generation, condition reason, and bad ref location.
- Provide desired state for infrastructure convergence, such as shared / per-Gateway / mesh frontend Service and EndpointSlice.
- Construct stable object sequences for the snapshot to be published.

Prohibited:

- Do not put Kubernetes runtime objects, client cache handles, or controller-runtime requests directly into the published snapshot.
- Do not write data-plane runtime mutable state such as request counts, endpoint passive failures, or selection cursors back to the control plane input IR.
- Do not let status or infrastructure logic directly depend on data-plane runtime-only indices.

Mutable state ownership:

- Kubernetes resource version, generation, condition, and managed resource ownership belong to the control plane input IR / status / infrastructure.
- Dataplane ACK, NACK, ready, and last applied snapshot version belong to the control plane node status view, not the published snapshot itself.

## 2. Publish IR / Proto Snapshot

The publish IR is the cross-process contract between the control plane and data plane, with the wire protocol at `proto/gateway/control/v1/control.proto`.

Primary code locations:

- `proto/gateway/control/v1/control.proto`
- `controlplane/internal/grpcserver/`
- `controlplane/internal/ir/`
- `dataplane/crates/aeg-proto/`
- `dataplane/crates/aeg-ir/src/proto.rs`

Responsibilities:

- Express the complete desired state required for data plane application configuration.
- Maintain stable field order, object ordering, and snapshot digest.
- Support old data planes ignoring new fields, and new data planes using safe defaults for missing fields.
- Pass the control-plane-normalized listener, route, backend, secret, workload hint, and backend policy output to the data plane.
- Carry ACK / NACK semantics through `DiscoveryRequest.result_status`, `version`, `nonce`, and `error_detail`.

The publish IR should remain nearly immutable:

- One `ConfigSnapshot.id` corresponds to one logically immutable configuration.
- Do not append request-time runtime state inside objects after publishing.
- Fanout caches may reuse the same proto object, but must not allow a single data plane connection to modify a shared object.
- New fields must follow the proto checklist in [Contract Versioning And Compatibility](contracts/versioning.md).

Prohibited:

- Do not carry control-plane internal execution details such as controller-runtime cache keys, watch requests, or status patch attempts in the proto snapshot.
- Do not carry data-plane runtime state such as endpoint passive ejection, active probe failure, connection pool state, or load-balancing cursor in the proto snapshot.
- Do not disguise repo-specific temporary implementation details as Gateway API standard fields. If transport is necessary, prefer explicitly named fields or `extensions` sub-structures, and document compatibility semantics.

Mutable state ownership:

- `version`, `nonce`, ACK/NACK, and node status are xDS session state.
- Snapshot payload is desired configuration.
- Last-good fallback is data-plane runtime behavior and should not be written back into the snapshot payload.

## 3. Data Plane Runtime IR

The data plane runtime IR is oriented toward request matching, listener plan, TLS assets, endpoint runtime, and observability. It is decoded from the publish IR but can build local indices and runtime handles.

Primary code locations:

- `dataplane/crates/aeg-ir/`
- `dataplane/crates/aeg-http/src/runtime/`
- `dataplane/crates/aeg-http/src/proxy/`
- `dataplane/crates/aeg-stream/`
- `dataplane/crates/aeg-app/src/admin/`

Responsibilities:

- Decode the proto snapshot and build a request-time read-only view.
- Precompute indices for route selection, hostname, listener, mesh frontend, backend policy, and workload.
- Generate HTTP / HTTPS listener plan, stream listener plan, and TLS asset plan.
- Maintain endpoint runtime state such as passive ejection, active probe health, selection cursor, and connection / retry observations.
- Expose the runtime view needed for data plane admin API, Prometheus metrics, and access logs.

Allowed local mutable state:

- `selection_state`
- `endpoint_runtime`
- listener bind / handoff runtime state
- TLS asset materialization cache
- connection pool, traffic stats, circuit breaker, rate limit, overload state
- xDS connection state, last-good snapshot handle

Prohibited:

- Do not serialize data plane runtime indices back into the control plane published snapshot.
- Do not let the request path directly read Kubernetes object semantics. The request path should only consume runtime IR and lightweight request views.
- Do not fix Gateway API status in the runtime IR; status is the responsibility of the control plane.

## 4. Placement Rules for New Gateway API Fields

When adding a new Gateway API field, the design or commit message must answer these three questions:

- Which layer normalizes: the control plane input IR is responsible for parsing defaults, reference authorization, conflicts, and status reasons.
- Which layer transports: the publish IR / proto snapshot only transmits stable results that the data plane must consume, and documents safe defaults for old data planes.
- Which layer precomputes or consumes at runtime: the data plane runtime IR determines whether a matcher index, listener plan, TLS asset, endpoint handle, or request-time filter is needed.

Recommended workflow:

1. Add semantic tests at the control plane translator / status layer to pin the conversion from Kubernetes input to status and publish IR.
2. Append fields in proto; do not delete, reuse field numbers, or change wire types.
3. Define missing field defaults and unknown enum fallbacks in `aeg-ir/src/proto.rs` or sub-modules.
4. Add consumption tests at the data plane matcher / runtime / admin layer.
5. Update the declared / implemented / tested / production-validated boundaries in `docs/gateway-api-support.md`.

## 5. Endpoint Runtime State Separation Direction

The current data plane `aeg-ir::Snapshot` already carries both read-only configuration and local runtime auxiliary state, such as `selection_state` and `endpoint_runtime`. This is safer than writing state back to the proto snapshot, but it is not yet the final form for future lock-free reads.

Future direction:

- The snapshot payload continues to remain immutable, shareable, and atomically replaceable.
- Endpoint success / failure, passive ejection, active probe, and selection cursor gradually migrate to independent runtime handles.
- When the request path reads the snapshot, it only takes immutable configuration references and runtime handles, without copying or modifying the snapshot payload.
- Admin summary can combine snapshot view and runtime handle view, but must distinguish desired config, current runtime, and historical recovery state at the field level.

This is also the prerequisite boundary for the hot path to transition from an owned model to a view / handle model.

## 6. Review Checklist

Changes involving IR, proto, translator, or runtime indices should at minimum check:

- Whether Kubernetes object semantics are limited to the control plane input IR.
- Whether cross-process fields are limited to the publish IR / proto snapshot.
- Whether request-time mutable state is limited to the data plane runtime IR.
- Whether safe defaults for new fields and old data plane behavior are documented.
- Whether endpoint runtime success/failure is prevented from being written into the snapshot body.
- Whether related admin / support / compatibility documentation is updated.
- Whether the lowest-cost verification commands are chosen, such as `cd controlplane && go test ./...`, `cargo test --manifest-path dataplane/Cargo.toml --workspace`, or `./scripts/run-skew-validation.sh`.
