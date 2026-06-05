# Design Document

## 1. Control Plane Detailed Design

### 1.1 Components

- `config`
  - Loads YAML runtime configuration.
- `controller`
  - Watches Kubernetes resource changes and triggers snapshot rebuilds.
  - Drives status write-back and infrastructure reconciliation through a standalone runner.
- `translator`
  - Translates Gateway API objects into IR.
- `grpcserver`
  - Provides data plane configuration discovery service.
- `admin`
  - Exposes control plane snapshot, routes, backends, nodes, resources, topology, and health interfaces.
- `ir`
  - Defines control plane input IR, published snapshot, and subscription store; see [IR Layering](ir-layering.md) for the three-layer boundary.

### 1.2 Rebuild Strategy

The current implementation is not a single periodic polling mechanism, but a combination of two paths:

1. `Syncer` watches Gateway API and related dependency resources via controller-runtime watch, immediately rebuilding the snapshot when events occur.
2. The same `Syncer` still performs a full rebuild at `syncPeriod` intervals as a fallback for missed watch events or state drift.
3. `ReconcilerRunner` periodically executes Gateway / Route status write-back and infrastructure maintenance under leader election protection.
4. When the snapshot version changes, it is broadcast to currently connected data plane nodes on the control plane instance.

Future evolution can continue toward event-driven sync based on informer/watch, index-based partial recomputation, partitioned snapshots, or incremental patch publishing.

### 1.3 Admin API

The control plane currently provides `/v1/*` HTTP APIs for operations, scripting, automation, dashboard, and future SDK reuse. The machine-readable surface is in [admin-api-surface.json](contracts/admin-api-surface.json), and the human-readable documentation is in [Management API](user/admin-api.md).

Control plane admin API:

- `GET /livez`
- `GET /readyz`
- `GET /v1/summary`
- `GET /v1/snapshot-sync`
- `GET /v1/snapshot`
- `GET /v1/listeners`
- `GET /v1/listeners/{name}`
- `GET /v1/routes`
- `GET /v1/routes/{kind}/{namespace}/{name}`
- `GET /v1/backends`
- `GET /v1/backends/{namespace}/{name}`
- `GET /v1/nodes`
- `GET /v1/nodes/{nodeId}`
- `GET /v1/infrastructure`
- `GET /v1/service-catalog`
- `GET /v1/resource-kinds`
- `GET /v1/resources`
- `POST /v1/resources`
- `GET /v1/resources/{kind}/{namespace}/{name}`
- `PUT /v1/resources/{kind}/{namespace}/{name}`
- `DELETE /v1/resources/{kind}/{namespace}/{name}`
- `GET /v1/topology`

For cluster-scoped resources, the current admin API uses `_cluster` as the namespace placeholder in paths.

Control plane metrics are not served on the admin port but are exposed separately via `metricsAddr`.

Current list endpoints support lightweight filtering for troubleshooting tools, dashboard, and future SDK direct reuse:

- `listeners`: `name`, `protocol`, `hostname`, `attachedRoute`, `runtimeId`
- `routes`: `kind`, `namespace`, `name`, `hostname`, `runtimeId`, `ruleRuntimeId`
- `backends`: `namespace`, `name`, `protocol`, `runtimeId`, `endpointRuntimeId`, `service`, `all`
- `nodes`: `nodeId`, `cluster`, `connected`, `ready`, `version`
- `resources`: `kind`, `namespace`, `name`
- `topology`: `type`, `kind`, `namespace`, `name`, `status`, `includeRelated`

## 2. Data Plane Detailed Design

### 2.1 Crate Decomposition

- `aeg-config`
  - Data plane YAML configuration parsing.
- `aeg-ir`
  - Runtime snapshot model.
- `aeg-proto`
  - Rust gRPC types generated from proto.
- `aeg-xds`
  - gRPC configuration stream client.
- `aeg-http`
  - HTTP/GRPC engine and route selection based on Rust proxy.
- `aeg-stream`
  - TCP/UDP/TLS route hosting logic.
- `aeg-observability`
  - Tracing, access log formatting, metrics helpers.
- `aeg-app`
  - Startup entry point, assembles all components.

Data plane admin surface:

- `GET /livez`
- `GET /readyz`
- `GET /metrics`
- `GET /v1/summary`
- `GET /v1/node`
- `GET /v1/snapshot`
- `GET /v1/overload`
- `GET /v1/circuit-breakers`
- `GET /v1/rate-limits`
- `GET /v1/listeners`
- `GET /v1/listeners/{name}`
- `GET /v1/listener-statuses`
- `GET /v1/listener-statuses/{name}`
- `GET /v1/routes`
- `GET /v1/routes/{kind}/{namespace}/{name}`
- `GET /v1/backends`
- `GET /v1/backends/{namespace}/{name}`
- `GET /v1/traffic`

`/v1/summary` currently provides `summarySurface=dataplane-summary` and `summarySchemaVersion=1` for consumer structure handshake.

### 2.2 HTTP/GRPC Routing

HTTP and GRPC share the Listener and Backend selection flow:

1. Determine candidate Listeners based on listening port, Host/SNI, and protocol.
2. Perform matching based on Route type:
   - HTTPRoute: Path/Header/Query/Method
   - GRPCRoute: Service/Method/Header
3. Select BackendCluster and Endpoint.
4. Create upstream connections via Rust proxy.

Current implementation notes:

- HTTP listener and HTTPS listener dynamically generate listening plans based on snapshot via `aeg-http`.
- HTTPS termination uses Rust proxy native TLS listener capabilities as much as possible.
- TLS certificates come from Secret snapshots distributed by the control plane and are materialized as temporary certificate files on the data plane for Rust proxy loading.
- Plaintext listeners enable h2c by default; TLS listeners enable ALPN h2 by default to support GRPCRoute downstream access.
- Implemented L7 filters include Header modifier, RequestRedirect, URLRewrite, RequestMirror.
- `BackendRef.weight` participates in runtime selection; healthy endpoints within backend clusters are also round-robined.
- Route timeout and backend timeout are merged at the data plane and applied to the upstream peer.

### 2.3 Stream Routing

`TCPRoute`, `UDPRoute`, `TLSRoute` are handled by `aeg-stream`:

- TCP: Layer 4 passthrough, binds backends by port and parent listener.
- UDP: Maps ports to target backends.
- TLSRoute: TLS passthrough routing by SNI.

TLS passthrough and HTTPS TLS termination are two independent data paths:

- TLSRoute is still handled by `aeg-stream` for Layer 4 passthrough.
- HTTPS listener is handled by `aeg-http` for TLS termination before entering the HTTP/GRPC routing flow.

Stream runtime current semantics:

- TCP / TLS passthrough listeners accept connections, read the current shared snapshot, and select backends by listener name, port, and optional SNI.
- TLS passthrough only reads the ClientHello preface to resolve SNI; no TLS termination is performed.
- UDP listeners perform datagram forwarding by listener / route / backend; access log events are `udp_datagram`.
- Stream runtime records `tcp_session`, `tls_session`, `udp_datagram` access logs and aggregates traffic observations into data plane metrics/admin view.
- Reload is coordinated via listener plan and runtime state; existing connections continue forwarding to the backend selected at accept time; new connections read the latest available snapshot.

### 2.4 Service Parent / Mesh Frontend

The current implementation already supports scenarios where `Route.parentRefs` points to a `Service`:

1. The control plane assigns deterministic frontend ports to referenced Services and translates them into mesh listeners.
2. The control plane maintains mesh frontend Services, shadow Services, and self-managed EndpointSlices, mapping traffic entry points to dataplane Pods.
3. After receiving workload extensions, the data plane can infer source namespace from source IP and apply additional constraints to cross-namespace mesh routes.
4. When a mesh listener has no matching explicit route, the data plane can still fall back to the corresponding Service's default backend.

## 3. gRPC Protocol Design

Protocol goals:

- Allow data planes to establish long-lived connections for snapshot subscription.
- Support version ACK and node status reporting.
- Keep the protocol simple without introducing complex xDS type systems.

Core RPCs:

- `StreamConfiguration`
  - Bidirectional stream; data plane sends node info and ACK, control plane distributes configuration snapshots.
- `ReportStatus`
  - One-shot report, transmitting health and version information.

Current implementation notes:

- Control plane gRPC server supports TLS per configuration.
- When `clientCAPath` and `requireClientCert` are configured, the control channel can switch to mTLS.
- Data plane `aeg-xds` supports CA verification, SNI/domain override, and optional client certificates.
- `DiscoveryRequest.result_status` currently carries ACK / NACK semantics; `error_detail` carries the reason the data plane rejected the current snapshot. On application failure, the data plane should continue serving the last-good snapshot, and the control plane retains error info in the node view.
- The control plane caches published proto objects by snapshot ID, reducing duplicate conversion cost during fanout to multiple data plane replicas.
- After decoding `ConfigSnapshot`, the data plane first builds runtime IR, listener plan, TLS asset plan, and indexes; only after successful application does it switch to the current snapshot.
- If a new snapshot fails to decode or apply at runtime, the data plane reports NACK and continues serving the last-good snapshot; `/readyz`, `/v1/summary`, listener statuses, and metrics expose current / last-good / recovery state.
- Old data planes must ignore new proto fields; new data planes must provide safe defaults for missing fields. Field evolution rules are in [Contract Versioning And Compatibility](contracts/versioning.md).

### 3.1 Runtime Apply

Data plane runtime apply is divided into four steps:

1. `aeg-xds` receives `DiscoveryResponse` and decodes the proto snapshot.
2. `aeg-ir` builds a runtime snapshot from the published IR and rebuilds local indexes such as matcher, backend, mesh, policy, and endpoint runtime handles.
3. `aeg-http` / `aeg-stream` generate listener plans based on the snapshot; HTTP/HTTPS, TCP, UDP, and TLS passthrough are each hosted by their respective runtimes.
4. On successful runtime switch, send ACK; on failure, retain last-good and expose the error via NACK, admin summary, and metrics.

This means the snapshot payload is the desired configuration; listener bind state, TLS asset materialization, endpoint ejection, selection cursor, connection pool, and traffic stats all belong to data plane runtime state and are not written back to the proto snapshot.

## 4. Configuration Design

### 4.1 Control Plane Configuration

Control plane YAML includes at minimum:

- gRPC listening address
- Admin API address
- Metrics address
- Health probe address
- Log level and format
- Default sync period
- GatewayClass controllerName
- Leader election parameters
- Optional admin Bearer Token
- Optional gRPC TLS/mTLS certificate paths

### 4.2 Data Plane Configuration

Data plane YAML includes at minimum:

- Node ID
- Control plane gRPC address
- Admin API address
- Listener default parameters
- Access log format/output path
- HTTP3, IPv6, TLS1.3 toggles
- Optional admin Bearer Token
- Optional xDS TLS/mTLS certificate paths

## 5. Logging Design

### 5.1 Control Logs

Control plane and data plane control logs uniformly use structured logging:

- `ts`
- `level`
- `component`
- `controller_name` or `node_id`
- `snapshot_version`
- `message`

Current implementation notes:

- Control plane `log.addSource` controls whether source location is output.
- Data plane `log.addSource`, `log.includeTarget`, `log.includeThreadIds`, `log.includeThreadNames` respectively control source location, target, thread IDs, and thread names.
- The control plane main process always attaches `component=controlplane` and `controller_name=<controllerName>`.
- Data plane tracing is uniformly initialized by `aeg-observability`, avoiding individual crate format assembly.

### 5.2 Access Logs

Data plane access logs are used for business traffic:

- HTTP/GRPC: records method, host, path, route, backend, status, latency, bytes.
- TCP/TLS/UDP: records session or datagram-level events, listener, backend, bytes, and duration.
- Log format and output location are controlled by data plane configuration.

## 6. Test Boundaries

- Control plane admin API changes must at least run `cd controlplane && go test ./internal/admin`.
- Data plane admin API changes must at least run `cargo test --manifest-path dataplane/Cargo.toml -p aeg-app`.
- Route and backend logic changes must run `./tests/e2e/run-kind.sh` for smoke validation.
- Gateway API semantics changes must run `ALL_FEATURES=true ./tests/conformance/run.sh`.
- When adding new proto fields, run `./scripts/run-skew-validation.sh` to verify forward/backward compatibility.
