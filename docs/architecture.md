# Architecture Document

## 1. Goals

This project implements a Kubernetes Gateway API gateway based on the Rust proxy framework, meeting the following core goals:

- Separation of control plane and data plane.
- Control plane uses Go, responsible for Kubernetes resource awareness, translation, publishing, status aggregation, and infrastructure maintenance.
- Data plane uses Rust, with HTTP/GRPC traffic prioritized to use Rust proxy native capabilities.
- Unified modeling for the main Gateway API route types: `Gateway`, `GatewayClass`, `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, `TLSRoute`, `ReferenceGrant`, etc.
- Unified logging, metrics, health checks, snapshot queries, topology queries, resource management, and node status interfaces.
- Runtime uses YAML configuration, buildable and testable in mainland China network environments.

The current repository includes a standalone Next.js/React dashboard and Node same-origin proxy, located at [`dashboard/`](../dashboard/). The dashboard works through the control plane and data plane admin APIs and does not define new public management surface contracts; stable contracts remain based on [Management API](user/admin-api.md) and [Admin API Contract](contracts/admin-api-contract.md).

## 2. Overall Architecture

### 2.1 Layering

The system is divided into four layers:

1. Kubernetes Resource Access Layer
   - Uses controller-runtime watch to trigger snapshot rebuilds, with periodic full rebuilds retained as a fallback.
   - Performs unified aggregation of Gateway API resources and dependent objects such as `Service`, `Secret`, `EndpointSlice`, `Pod`, `Namespace`.
   - Can later evolve into finer-grained cache / indexer / partial recomputation models.

2. Control Plane Core Layer
   - Translates Kubernetes resources into internal IR.
   - Performs route binding, backend resolution, certificate and policy assembly.
   - Independently executes Gateway / Route status write-back and some infrastructure maintenance.
   - Distributes snapshots to data planes via gRPC streams.

3. Data Plane Runtime Layer
   - Consumes control plane snapshots and hot-reloads local state.
   - HTTP/GRPC listeners use Rust proxy and host HTTPS TLS termination.
   - TCP/UDP/TLS passthrough listeners are hosted by a separate Stream subsystem.

4. Management and Observability Layer
   - Control plane and data plane uniformly expose health checks, metrics, logs, and current snapshot views.
   - Control plane provides `/v1/*` admin API supporting list filtering, resource-level detail queries, topology views, and resource management.
   - Data plane provides `/v1/*` admin API and `/metrics` Prometheus metrics.
   - Dashboard is deployed as an optional operations console, accessing admin APIs same-origin via Node proxy, without direct Kubernetes API access.
   - Management surface contracts are in [Admin API Contract](contracts/admin-api-contract.md).

### 2.2 Component Diagram

```text
+-------------------+        +--------------------------------------+
| Kubernetes API    |        | Control Plane (Go)                   |
| Gateway API CRDs  | -----> | 1. Watch / Scoped + Full Rebuild     |
| Services/Secrets  |        | 2. Translator -> Published Snapshot  |
+-------------------+        | 3. Snapshot Store / Fanout Cache     |
                             | 4. gRPC Config Discovery Service     |
                             | 5. Status / Infra / Admin / Metrics  |
                             +-------------------+------------------+
                                                 |
                                                 | gRPC stream
                                                 v
                             +-------------------+------------------+
                             | Data Plane (Rust + Rust proxy)          |
                             | 1. xDS Client / Last-Good Snapshot   |
                             | 2. Rust proxy HTTP/GRPC + TLS Engine    |
                             | 3. Stream Engine (TCP/UDP/TLS)       |
                             | 4. Runtime State / Access Log        |
                             | 5. Admin API / Metrics / Health      |
                             +-------------------+------------------+
                                                 |
                                                 v
                                           Backend Services
```

### 2.3 Deployment Topology

The repository currently maintains three main installation forms:

- `deploy/kubernetes/base`
  - Deploys `nantian-controlplane`, `nantian-dataplane`, `nantian-gw-dashboard`, RBAC, GatewayClass, basic Services, and NetworkPolicy.
  - controlplane, dataplane, and dashboard are independent Deployments / Services.
- `deploy/kubernetes/overlays/kind`
  - Overlays images, ports, status addresses, and smoke entry points needed for Kind local debugging.
- `deploy/kubernetes/overlays/production`
  - Overlays production default security boundaries: controlplane gRPC mTLS, dataplane xDS mTLS, admin Bearer Token, session persistence secret, mandatory Secret volumes, dataplane HPA.

Port responsibilities are split by component:

- controlplane gRPC: for dataplane configuration discovery, can enable TLS/mTLS.
- controlplane admin: for operations, automation, dashboard, and future SDK, can enable Bearer Token.
- controlplane metrics: exposed separately via `metricsAddr`, not mixed with admin port.
- controlplane health probe: exposed separately via health address.
- dataplane admin / metrics / probe: data plane local state, Prometheus metrics, and health checks.
- dataplane traffic listeners: dynamically driven by Gateway / Route snapshot.

## 3. Control Plane Design Principles

- Single translation output: all Kubernetes resources are ultimately unified into IR, then output by different publishers.
- Control plane carries no business traffic: only responsible for desired state computation, incremental publishing, and status aggregation.
- Clear resource-to-snapshot boundary: Kubernetes object model does not leak directly to the data plane.
- Extensible: when adding remaining `BackendTLSPolicy` subsets, more policy CRDs, and management interfaces, prioritize extending IR and translator rather than directly intruding into the data plane.
- Control channel is designed by default as "runnable on internal networks, TLS/mTLS enabled on demand," avoiding exposing plaintext config discovery interfaces directly to production environments.

## 4. Data Plane Design Principles

- HTTP/GRPC prioritizes using Rust proxy native proxy capabilities.
- Stream protocols are modeled separately to avoid HTTP logic polluting TCP/UDP/TLS implementations.
- Configuration hot-reload is completed via read-only snapshots + atomic replacement.
- Business access logs are separated from control logs, supporting independent formats and output paths.
- All runtime behavior can be controlled by YAML configuration, including log level, listening addresses, TLS parameters, upstream timeouts, etc.
- Management interfaces and config discovery interfaces are designed with minimal exposure surface by default, supporting Bearer Token and gRPC TLS/mTLS.

## 5. IR Layering

IR is not a single structure but a three-layer model:

1. Control plane input IR: for Kubernetes resource aggregation, Gateway API semantic normalization, status write-back, and infrastructure desired state.
2. Published IR / proto snapshot: for cross-process compatibility contract from control plane to data plane; wire protocol is in `proto/gateway/control/v1/control.proto`.
3. Data plane runtime IR: for matcher, listener plan, endpoint runtime, TLS assets, stream runtime, admin view, and metrics.

For detailed boundaries, mutable state ownership, and new field checklists, see [IR Layering](ir-layering.md).

Core objects of the published IR include:

- `Listener`: protocol, listening address, port, hostname, TLS configuration, address family.
- `HTTPRoute`: Hostname, Path, Header, Query, Method, Filter, BackendRef.
- `GRPCRoute`: Hostname, Service, Method, Header, Filter, BackendRef.
- `StreamRoute`: TCP / UDP / TLS route type, SNI, port matching, BackendRef.
- `BackendCluster`: protocol, timeout, health status, endpoint list.
- `SecretMaterial`: certificate reference, SNI, TLS minimum version, mode.
- `extensions`: currently primarily used to pass workload lists to the data plane, supporting source namespace determination in mesh / Service parent scenarios.

The goal of the published IR is not a one-to-one replica of the Kubernetes API, but a normalized model more suitable for runtime consumption. The data plane builds local runtime indexes, listener plans, TLS asset caches, and endpoint runtime handles on top of it; these request-time mutable states are not part of the published snapshot body.

## 6. Current Implementation and Future Directions

### 6.1 Currently Implemented

- Control plane:
  - Aggregation of Gateway API and dependent resources, IR snapshot publishing, and gRPC configuration distribution.
  - Basic status write-back for `GatewayClass`, `Gateway`, `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, `TLSRoute`.
  - Shared dataplane Service, per-Gateway Service, and mesh frontend / shadow Service / EndpointSlice maintenance.
  - `Gateway.spec.infrastructure` `labels/annotations` passthrough, and per-Gateway Service exposure parameters driven by same-namespace `ConfigMap` `parametersRef`.
  - `/v1/*` management API, node view, topology view, resource management, and metrics.
- Data plane:
  - Rust proxy HTTP/GRPC proxy main path and HTTPS TLS termination.
  - `TCPRoute`, `UDPRoute`, `TLSRoute` stream runtime.
  - Header modifier, redirect, rewrite, mirror, weight selection, healthy endpoint round-robin, route / backend timeout.
  - xDS hot reload, management API, metrics, and access logs.

### 6.2 Near-Term Gaps

- HTTP3 / QUIC listening capability.
- More formal load balancing, retry, and policy extension capabilities.
- More complete managed Kubernetes distribution matrix, 24h/72h soak, node drain, and upgrade/rollback evidence.
- More complete security scanning, SBOM, provenance, and release notes automation.

### 6.3 Longer-Term Directions

- Conformance publication archive automation.
- Performance stress testing, fault injection, and long-duration soak release gates.
- Multi-tenant isolation, policy extension, fine-grained circuit breaking and retry.

## 7. Observability Architecture

### 7.1 Control Plane

- Structured logs: resource version, translation duration, published version, node status.
- Metrics: Reconcile count/duration, current snapshot version, connected data plane node count, publish failure count.

### 7.2 Data Plane

- Control logs: config receipt, version switch, listener reload, backend status.
- Business logs: Host, Method, Path, Route, Backend, Status, Latency, Bytes, supporting custom formats and output paths.
- Metrics: request count, latency, upstream connection errors, route match failures, protection policies, and runtime status.

## 8. Extension Points

- Management surface access: control plane and data plane currently only expose their respective management interfaces; dashboard and any future SDK / UI should read stable surfaces based on [Management API](user/admin-api.md), not depend on page-private aggregate payloads. For cluster-level traffic views, discover nodes via `/v1/dataplanes` and query each node's dataplane `/v1/traffic` on demand, or use `/v1/metrics/query` to query Prometheus aggregate metrics. Dataplane admin endpoints remain valid per-node local debug surfaces but should not be used for cluster-level aggregate views via dataplane Service.
- Policy system: forward-compatible via IR extension fields and `extensions` map.
- Multi-protocol extension: when adding new Route types, prioritize extending Translator and corresponding engine crates.

## 9. Directory Layout

```text
docs/
proto/
controlplane/
dataplane/
configs/
deploy/
tests/
scripts/
```

## 10. Constraints

- Each code file should be kept within approximately 800 lines.
- Control plane and data plane interface changes must first update proto, IR layering documentation, and corresponding contract documents.
- All new features must at minimum include unit tests.
