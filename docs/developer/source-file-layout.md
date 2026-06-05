# Source File Layout and Large File Splitting Guide

This document records a structured split of overly long Go/Rust source files in this repository.
The goal is not to "chop up line counts," but to give each file a more focused responsibility and make it easier to locate entry functions as the codebase evolves.

## Scope and Constraints

- This document covers the source files that were actually split in this pass.
- It also lists source files that currently exceed 800 lines but are considered reasonable exceptions.
- Auto-generated files are not within the scope of this cleanup.
  - `controlplane/internal/gen/**`
  - `dataplane/target/**`
- The "function lists" in this document focus on primary entry points, core helpers, and responsibility boundaries; they do not aim to fully expand every test name or default value function into a directory-level index.

## Control Plane

### `controlplane/internal/grpcserver/server.go`

- Responsibilities:
  - gRPC xDS service main entry point and process-level lifecycle.
  - Defines the `Server` struct and shutdown signals.
- Key types:
  - `Server`
  - `streamRegistration`
- Key functions:
  - `New`
  - `(*Server).Run`
  - `(*Server).Serve`
  - `(*Server).signalShutdown`
- Reason for splitting:
  - The original file simultaneously handled gRPC lifecycle, xDS stream state machine, stream registry, send timeout handling, metrics recording, and `ReportStatus` integration, making the review surface too large.

### `controlplane/internal/grpcserver/server_stream.go`

- Responsibilities:
  - xDS `StreamConfiguration` main state machine.
  - Initial `DiscoveryRequest` handshake, ACK/NACK/heartbeat loop, and snapshot publish.
- Key types:
  - `initialDiscoveryRequestResult`
- Key functions:
  - `(*Server).StreamConfiguration`
  - `(*Server).recvInitialRequest`

### `controlplane/internal/grpcserver/server_registry.go`

- Responsibilities:
  - Active stream registration, superseding, and disconnection management for the same `nodeID`.
- Key functions:
  - `(*Server).registerStream`
  - `(*Server).unregisterStream`
  - `(*Server).isActiveStream`
  - `(*Server).disconnectStreamIfActive`
  - `registrationSuperseded`
  - `(*streamRegistration).supersede`

### `controlplane/internal/grpcserver/server_send.go`

- Responsibilities:
  - Snapshot send path, send timeout, and stream termination error semantics.
- Key functions:
  - `(*Server).sendDiscoveryResponse`
  - `shutdownStreamError`
  - `supersededStreamError`
  - `isSupersededStreamError`
  - `snapshotSendTimeoutError`
  - `snapshotAckTimeoutError`

### `controlplane/internal/grpcserver/server_metrics.go`

- Responsibilities:
  - xDS stream send/timeout/termination metrics recording.
- Key functions:
  - `(*Server).observeSnapshotSendDuration`
  - `(*Server).recordSnapshotSendTimeout`
  - `(*Server).recordSnapshotAckTimeout`
  - `(*Server).recordStreamTermination`

### `controlplane/internal/grpcserver/server_status.go`

- Responsibilities:
  - Dataplane `ReportStatus` unary RPC entry point.
  - Shutdown rejection, report validation, and `observedAt` normalization.
- Key functions:
  - `(*Server).ReportStatus`

### `controlplane/internal/grpcserver/snapshot_proto.go`

- Responsibilities:
  - Encode control plane IR snapshots into gRPC proto responses.
- Key functions:
  - `toProtoSnapshot`
  - `snapshotExtensions`
  - `toProtoTLS`
  - `toProtoRouteTimeouts`
  - `toProtoParents`
  - `toProtoRetryPolicy`
  - `toProtoBackends`
  - `toProtoFilters`
  - `toProtoHeaders`
  - `toProtoQueries`
  - `toListenerProtocol`
  - `toRouteKind`
  - `toEmptyStruct`
  - `durationOrNil`
- Notes:
  - This file is only responsible for proto assembly, not stream lifecycle control.

### `controlplane/internal/admin/query.go`

- Responsibilities:
  - Filtering and lookup main flow for `/v1/listeners`, `/v1/routes`, `/v1/backends`, `/v1/nodes`, etc.
- Key types:
  - `routeListResponse`
- Key functions:
  - `newRouteListResponse`
  - `filterListeners`
  - `filterRoutes`
  - `findRoute`
  - `filterBackends`
  - `filterNodes`
  - `findListener`
  - `findBackend`
  - `findNode`
  - `httpRouteMatches`
  - `grpcRouteMatches`
  - `streamRouteMatches`
- Reason for splitting:
  - Kept the "resource filtering main flow" in one file, with parsing, sorting, and canonicalization helpers pushed down.

### `controlplane/internal/admin/query_support.go`

- Responsibilities:
  - Admin query parameter parsing, pagination, sorting, protocol canonicalization, and generic slice utilities.
- Key types:
  - `sortOrder`
  - `listenerSortField`
  - `backendSortField`
  - `nodeSortField`
  - `routeSortField`
  - `listPagination`
- Key functions:
  - `parseOptionalBool`
  - `parseOptionalPositiveInt`
  - `parseOptionalNonNegativeInt`
  - `parseOptionalInt`
  - `parseIncludeAllBackends`
  - `parseListPagination`
  - `parseRoutePagination`
  - `parseSortOrder`
  - `parseListenerSortField`
  - `parseBackendSortField`
  - `parseNodeSortField`
  - `parseRouteSortField`
  - `sortListeners`
  - `sortBackends`
  - `sortNodes`
  - `sortHTTPRoutes`
  - `sortGRPCRoutes`
  - `sortStreamRoutes`
  - `paginateSlice`
  - `canonicalProtocol`
  - `canonicalBackendProtocol`
  - `canonicalRouteKind`

### `controlplane/internal/admin/server.go`

- Responsibilities:
  - Admin HTTP service construction, route registration, and lifecycle entry point.
  - Assembles controlplane admin routes with authentication/metrics middleware.
- Key types:
  - `Summary`
  - `Server`
- Key functions:
  - `NewServer`
  - `(*Server).registerRoutes`
  - `(*Server).ListenAndServe`
  - `(*Server).Serve`
  - `(*Server).Shutdown`
  - `(*Server).Close`
  - `(*Server).SetInfrastructureInspector`
- Reason for splitting:
  - The original file simultaneously handled service construction, all admin handlers, resource mutation auditing, response encoding, and error mapping, making the review and regression surface too large.

### `controlplane/internal/admin/server_overview.go`

- Responsibilities:
  - Health check, snapshot summary, and snapshot-sync related handlers.
  - Aggregates the `Summary` view.
- Key functions:
  - `(*Server).handleLiveness`
  - `(*Server).handleReadiness`
  - `(*Server).handleSnapshot`
  - `(*Server).handleSummary`
  - `(*Server).handleSnapshotSync`
  - `buildSummary`

### `controlplane/internal/admin/server_queries.go`

- Responsibilities:
  - Read-only query handlers for listener / route / backend / node / topology.
- Key functions:
  - `(*Server).handleListeners`
  - `(*Server).handleListenerDetail`
  - `(*Server).handleRoutes`
  - `(*Server).handleRouteDetail`
  - `(*Server).handleBackends`
  - `(*Server).handleBackendDetail`
  - `(*Server).handleNodes`
  - `(*Server).handleNodeDetail`
  - `(*Server).handleTopology`

### `controlplane/internal/admin/server_management.go`

- Responsibilities:
  - Infrastructure / service catalog / managed resources management handlers.
  - Resource mutation audit logging and resource identity extraction.
- Key functions:
  - `(*Server).handleInfrastructure`
  - `(*Server).handleServiceCatalog`
  - `(*Server).handleResourceKinds`
  - `(*Server).handleResources`
  - `(*Server).handleResourceDetail`
  - `(*Server).handleResourceApply`
  - `(*Server).handleResourceDelete`
  - `resourceMutationOperation`
  - `(*Server).logResourceMutationSuccess`
  - `(*Server).logResourceMutationFailure`
  - `resourceMutationIdentity`

### `controlplane/internal/admin/server_response.go`

- Responsibilities:
  - Admin JSON response encoding, size limits, and unified error status code mapping.
- Key types:
  - `limitedBuffer`
- Key functions:
  - `(*Server).respondJSON`
  - `(*Server).respondQueryError`
  - `(*Server).respondRequestError`
  - `newLimitedBuffer`
  - `(*limitedBuffer).Write`
  - `(*limitedBuffer).Bytes`
  - `(*Server).respondNotFound`
  - `statusCodeForAdminError`

### `controlplane/internal/status/reconciler.go`

- Responsibilities:
  - Status reconciler main entry point, construction, and top-level orchestration.
  - Chains state loading, evaluation, and status write-back for various resource types.
- Key types:
  - `Reconciler`
  - `noopEventRecorder`
- Key functions:
  - `New`
  - `NewWithAddresses`
  - `NewWithAddressesAndReader`
  - `(*Reconciler).SetEventRecorder`
  - `(*Reconciler).Reconcile`
  - `(*Reconciler).ReconcileGatewayClassObject`
- Reason for splitting:
  - The original file simultaneously carried the entry point, cluster state loading, resource dispatch, Gateway/GatewayClass status updates, Route status updates, and Policy status updates, with overly broad responsibility boundaries.

### `controlplane/internal/status/reconciler_state.go`

- Responsibilities:
  - Loads the cluster state required for status reconciliation.
  - Unified read ordering for Gateway/GatewayClass/Route/Policy/ReferenceGrant and supporting resources.
- Key functions:
  - `(*Reconciler).loadState`

### `controlplane/internal/status/reconciler_collections.go`

- Responsibilities:
  - Dispatches status write-back by resource collection.
  - Bridges evaluation results with per-object status update functions.
- Key functions:
  - `(*Reconciler).reconcileGatewayClasses`
  - `(*Reconciler).reconcileGateways`
  - `(*Reconciler).reconcileHTTPRoutes`
  - `(*Reconciler).reconcileGRPCRoutes`
  - `(*Reconciler).reconcileTCPRoutes`
  - `(*Reconciler).reconcileUDPRoutes`
  - `(*Reconciler).reconcileTLSRoutes`
  - `(*Reconciler).reconcileBackendTLSPolicies`
  - `(*Reconciler).reconcileBackendLBPolicies`

### `controlplane/internal/status/reconciler_gateway_status.go`

- Responsibilities:
  - GatewayClass / Gateway per-object status updates.
  - Infrastructure parameters related event emission and accepted message extraction.
- Key functions:
  - `(*Reconciler).reconcileGatewayClassStatus`
  - `(*Reconciler).reconcileGatewayClassStatusWithSupportResolver`
  - `(*Reconciler).reconcileGatewayStatus`
  - `gatewayInfrastructureParametersMessage`
  - `(*Reconciler).emitGatewayInfrastructureParameterEvent`

### `controlplane/internal/status/reconciler_policy_status.go`

- Responsibilities:
  - `BackendTLSPolicy` / `BackendLBPolicy` per-object status updates.
- Key functions:
  - `(*Reconciler).reconcileBackendTLSPolicyStatus`
  - `(*Reconciler).reconcileBackendLBPolicyStatus`

### `controlplane/internal/status/reconciler_route_status.go`

- Responsibilities:
  - `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, `TLSRoute` per-object status updates.
- Key functions:
  - `(*Reconciler).reconcileHTTPRouteStatus`
  - `(*Reconciler).reconcileGRPCRouteStatus`
  - `(*Reconciler).reconcileTCPRouteStatus`
  - `(*Reconciler).reconcileUDPRouteStatus`
  - `(*Reconciler).reconcileTLSRouteStatus`

### `controlplane/internal/infrastructure/inspector.go`

- Responsibilities:
  - Infrastructure inspection main entry point.
  - Defines inspection report model and status enum.
- Key types:
  - `InfrastructureReport`
  - `InfrastructureSummary`
  - `InfrastructureResource`
- Key functions:
  - `(*Reconciler).Inspect`
- Reason for splitting:
  - Retains the external inspection entry point and report data structure, while pushing "expected resource derivation / observed reading / diff / sorting aggregation" down to supporting files.

### `controlplane/internal/infrastructure/inspector_support.go`

- Responsibilities:
  - Internal inspection support logic.
  - Deriving expected Service/EndpointSlice, reading observed resources, and determining drift/orphan/missing status.
- Key types:
  - `infrastructureExpectation`
- Key functions:
  - `(*Reconciler).expectedInfrastructure`
  - `(*Reconciler).loadMeshServiceParents`
  - `(*Reconciler).loadObservedServices`
  - `(*Reconciler).loadObservedEndpointSlices`
  - `addServiceExpectation`
  - `addEndpointSliceExpectation`
  - `classifyObservedService`
  - `classifyObservedEndpointSlice`
  - `serviceDiffReasons`
  - `endpointSliceDiffReasons`
  - `finalizeInfrastructureReport`
  - `sortedExpectationKeys`
  - `infrastructureStateRank`

### `controlplane/internal/infrastructure/parameters.go`

- Responsibilities:
  - Gateway Service parameter model and resolver entry point.
  - GatewayClass-level parameter caching and Gateway-level override orchestration.
- Key types:
  - `gatewayServiceParameters`
  - `sessionAffinityConfigParameters`
  - `clientIPConfigParameters`
  - `gatewayClassServiceParametersResult`
  - `gatewayServiceParameterResolver`
- Key functions:
  - `newGatewayServiceParameterResolver`
  - `(*Reconciler).resolveGatewayServiceParameters`
  - `(*gatewayServiceParameterResolver).resolve`
  - `(*gatewayServiceParameterResolver).loadGatewayClassServiceParameters`
  - `(*Reconciler).loadGatewayClassServiceParametersUncached`

### `controlplane/internal/infrastructure/parameters_load.go`

- Responsibilities:
  - Read and decode Gateway / GatewayClass Service parameter documents from `ConfigMap`.
- Key functions:
  - `loadGatewayServiceParameters`
  - `loadGatewayClassServiceParameters`
  - `decodeGatewayServiceParameters`
  - `serviceParametersDocument`

### `controlplane/internal/infrastructure/parameters_service.go`

- Responsibilities:
  - Gateway Service parameter normalization, validation, application, and merge/clone helpers.
- Key functions:
  - `(*gatewayServiceParameters).normalize`
  - `(gatewayServiceParameters).validate`
  - `applyGatewayServiceParameters`
  - `mergeGatewayServiceParameters`
  - `cloneGatewayServiceParameters`
  - `cloneSessionAffinityConfigParameters`

### `controlplane/internal/admin/server_test.go`

- Responsibilities:
  - Admin server integration test shared fixtures, test data, and assertion helpers.
- Key contents:
  - `newTestServer*` / `newInfrastructureTestServer`
  - `performRequest`
  - `listenerNames`, `backendKeys`, `nodeIDs`, `*_RouteKeys`
  - `histogramVecSampleCount`
- Notes:
  - The large number of endpoint tests originally present have been split into multiple test files by topic.
  - This file now only retains construction and utility functions reusable across test scenarios.

### `controlplane/internal/admin/server_summary_test.go`

- Responsibilities:
  - Summary view and snapshot-sync status classification tests.
- Key test topics:
  - `buildSummary` aggregation
  - Persistent version drift alerts
  - Snapshot-sync node status classification
  - Current snapshot NACK / rejected status

### `controlplane/internal/admin/server_listener_route_test.go`

- Responsibilities:
  - Listener, route, snapshot-sync, and snapshot endpoint behavior tests.
- Key test topics:
  - Listener filtering, sorting, pagination, and detail
  - Display address priority
  - Route filtering, pagination by kind, and detail
  - `/v1/snapshot` and `/v1/snapshot-sync` response content

### `controlplane/internal/admin/server_backend_node_test.go`

- Responsibilities:
  - Backend and node endpoint behavior tests.
- Key test topics:
  - Backend filtering, sorting, pagination, and protocol canonicalization
  - Node filtering, sorting, pagination, and detail
  - Shared repository / lease fallback node status sources

### `controlplane/internal/admin/server_infrastructure_test.go`

- Responsibilities:
  - `/v1/infrastructure` inspection endpoint tests.
- Key test topics:
  - 501 semantics when inspector is not configured
  - Drift / missing / orphan resource reports
  - Kind / state / role filtering
  - Sorting and pagination constraints

### `controlplane/internal/admin/server_auth_metrics_test.go`

- Responsibilities:
  - Admin authentication, probe pass-through, request metrics, and readiness semantic tests.
- Key test topics:
  - Bearer token protection and hot-reload
  - `/livez`, `/readyz` bypass authentication
  - Admin request metrics statistics
  - Current-snapshot-any / all readiness modes

### `controlplane/internal/admin/resource_api_test.go`

- Responsibilities:
  - Resource admin API integration test shared fixtures, fake client wrappers, and request helpers.
- Key contents:
  - `newTestServerWithResourceManager*`
  - `resourceManagerForTest*`
  - `topologyNodeIDs`, `topologyEdgeIDs`
  - `noMatchListClient`, `resourceErrorClient`, `countingResourceClient`
  - `metav1TypeMeta`, `metav1ObjectMeta`
  - `createServiceForTest`
  - `serviceCatalogKeys`
  - `performRequestWithBody`
- Notes:
  - Resource, service catalog, topology, and special resource kind tests that were originally aggregated in one file have been split out by topic.

### `controlplane/internal/admin/resource_api_resources_test.go`

- Responsibilities:
  - `/v1/resources` and `/v1/resource-kinds` CRUD, pagination, audit logging, and error mapping tests.
- Key test topics:
  - Basic CRUD flow
  - Request body size limit
  - Exact-match direct get optimization
  - Graceful degradation when optional kinds are missing
  - Kubernetes forbidden / conflict error mapping

### `controlplane/internal/admin/resource_api_service_catalog_test.go`

- Responsibilities:
  - `/v1/service-catalog` query, filter, sort, pagination, and direct get tests.
- Key test topics:
  - Service port display
  - Namespace / protocol filtering
  - Sort / order / pagination constraints
  - Exact-match direct get path

### `controlplane/internal/admin/resource_api_topology_test.go`

- Responsibilities:
  - `/v1/topology` topology view and drilldown filter tests.
- Key test topics:
  - Full topology graph response
  - Route / listener drilldown
  - Related node/edge expansion
  - Invalid filter parameter validation

### `controlplane/internal/admin/resource_api_special_kinds_test.go`

- Responsibilities:
  - Special Gateway API / MCS resource kind admin CRUD regression tests.
- Key test topics:
  - `BackendLBPolicy`
  - `BackendTLSPolicy`
  - `ReferenceGrant`
  - `GatewayClass`
  - `ServiceImport`

## Data Plane

### `dataplane/crates/aeg-config/src/lib.rs`

- Responsibilities:
  - Data plane configuration structure, defaults, environment variable overrides, and config file parsing.
- Key contents:
  - `DataPlaneConfig` and related configuration struct `impl`
  - Default value helpers
  - Millisecond duration conversion helpers
- Split result:
  - Only production code retained.
  - Tests migrated to `src/tests.rs`.

### `dataplane/crates/aeg-config/src/tests.rs`

- Responsibilities:
  - Configuration parsing and default value regression tests.
- Key test topics:
  - YAML parsing
  - Environment variable overrides
  - Admin token / session secret file loading
  - xDS transport parameter defaults
  - Runtime protection and tuning defaults

### `dataplane/crates/aeg-observability/src/runtime.rs`

- Responsibilities:
  - Runtime reload statistics, listener progress tracking, apply event publishing.
- Key types:
  - `RuntimePlane`
  - `RuntimeApplyOutcome`
  - `RuntimeApplyEvent`
  - `RuntimeListenerFailure`
  - `RuntimeListenerEvent`
  - `RuntimeListenerProgress`
  - `RuntimeStats`
  - `RuntimeStatsSnapshot`
- Key functions:
  - `RuntimeStats` related observation functions
  - `epoch_seconds`
  - `record_listener_event`
- Split result:
  - Tests migrated to `src/runtime/tests.rs`.

### `dataplane/crates/aeg-observability/src/runtime/tests.rs`

- Responsibilities:
  - Runtime observability state regression tests.
- Key test topics:
  - Listener failure and recovery accumulation
  - Retained version tracking
  - Recent event history pruning
  - HTTP/stream apply event
  - Supervisor / runtime exit status

### `dataplane/crates/aeg-stream/src/udp.rs`

- Responsibilities:
  - UDP listener runtime.
  - UDP session registry, upstream socket reuse, datagram-level access log recording.
- Key types:
  - `UdpSessionKey`
  - `UdpSessionRegistry`
  - `UdpSessionTask`
- Key functions:
  - `UdpSessionRegistry::dispatch`
  - `UdpSessionRegistry::ensure_sender`
  - `build_udp_session_task`
  - `run_udp_session`
  - `proxy_session_datagram`
  - `record_udp_datagram`
  - `udp_session_idle_timeout`
- Split result:
  - Tests migrated to `src/udp/tests.rs`.

### `dataplane/crates/aeg-stream/src/udp/tests.rs`

- Responsibilities:
  - UDP forwarding and budget control regression tests.
- Key test topics:
  - Single datagram proxying
  - Second datagram dropped under overload
  - Multi-response forwarding
  - Upstream socket reuse
  - Idle timeout
  - Error semantics when no matching route

### `dataplane/crates/aeg-xds/src/lib.rs`

- Responsibilities:
  - xDS client main flow.
  - gRPC connection, reconnect backoff, snapshot reception, runtime apply wait, heartbeat reporting.
- Key types:
  - `RuntimeApplyRequirements`
  - `ControlPlaneClient`
  - `ReconnectBackoff`
- Key functions:
  - `ControlPlaneClient::connect`
  - `ControlPlaneClient::stream_configuration`
  - `ReconnectBackoff::new`
  - `ReconnectBackoff::next_delay`
  - `build_status_report`
  - `snapshot_runtime_apply_requirements`
  - `wait_for_runtime_apply_result`
  - `log_stream_failure_retry`
  - `log_heartbeat_report_failure`
  - `discovery_ack`
  - `discovery_nack`
- Split result:
  - Connection stats split to `src/stats.rs`.
  - Unit tests split to `src/tests.rs`.

### `dataplane/crates/aeg-xds/src/stats.rs`

- Responsibilities:
  - xDS client connection status, snapshot apply status, and transport error statistics.
- Key types:
  - `SharedClientStats`
  - `ClientStats`
  - `ClientStatsSnapshot`
- Key functions:
  - `ClientStats::shared`
  - `observe_connect_failure[_with_error]`
  - `observe_stream_connected`
  - `observe_stream_failure[_with_error]`
  - `observe_snapshot_applied`
  - `observe_snapshot_skipped`
  - `observe_snapshot_nacked`
  - `snapshot`

### `dataplane/crates/aeg-xds/src/tests.rs`

- Responsibilities:
  - xDS client status, log level, and async apply wait regression tests.
- Key test topics:
  - ACK/NACK encoding
  - Readiness / waiting-for-snapshot semantics
  - Duplicate snapshot log level
  - Expected vs unexpected reconnect/heartbeat error logs
  - Apply timeout and async event wakeup

### `dataplane/crates/aeg-app/src/admin/metrics/listeners.rs`

- Responsibilities:
  - Renders listener current status, attention, convergence, and recovery information as Prometheus metrics.
- Key functions:
  - `append_listener_metrics`
- Reason for splitting:
  - The original file was simultaneously responsible for "status summary computation" and "metrics output," with many metric names and an overly long main function.

### `dataplane/crates/aeg-app/src/admin/metrics/listeners/counts.rs`

- Responsibilities:
  - Precomputes intermediate counts and severity levels required for listener metrics.
- Key types:
  - `ListenerMetricCounts`
- Key functions:
  - `collect_listener_metric_counts`
- Notes:
  - `listeners.rs` now only retains the metrics output main flow.
  - This file handles listener runtime state collection and aggregation.

### `dataplane/crates/aeg-ir/src/snapshot.rs`

- Responsibilities:
  - `Snapshot` core entry point capabilities.
  - Route selection, listener indexing, hostname indexing, and consistent hash helpers.
- Key function / method clusters:
  - `Snapshot::shared`
  - `Snapshot::rebuild_runtime_indexes`
  - `Snapshot::inherit_runtime_state_from`
  - `Snapshot::select_backend`
  - `Snapshot::select_http_backend`
  - `Snapshot::select_http_route`
  - `Snapshot::select_grpc_backend`
  - `Snapshot::select_stream_backend`
  - `Snapshot::select_request_mirror`
  - `Snapshot::select_request_mirrors`
  - `Snapshot::select_first_healthy_backend`
  - `Snapshot::select_listener_default_backend`
  - `Snapshot::default_stream_backend`
  - `Snapshot::default_service_backend`
  - `Snapshot::source_namespace`
- Split result:
  - Backend resolution, endpoint selection, and runtime endpoint state inheritance migrated to `src/snapshot/backend_resolution.rs`.

### `dataplane/crates/aeg-ir/src/snapshot/backend_resolution.rs`

- Responsibilities:
  - Backend resolution and endpoint-level selection logic.
  - Session persistence, consistent hash, and endpoint availability determination after passive circuit breaking.
  - Endpoint runtime state inheritance during snapshot switching.
- Key function / method clusters:
  - `Snapshot::resolve_backend_refs`
  - `Snapshot::resolve_http_backend_refs`
  - `Snapshot::resolve_http_backend_refs_with_session`
  - `Snapshot::resolve_persistent_http_backend`
  - `Snapshot::resolve_backend_policy_persistent_http_backend`
  - `Snapshot::collect_http_backend_candidates`
  - `Snapshot::resolve_consistent_hash_http_backend`
  - `Snapshot::select_backend_ref`
  - `Snapshot::backend_cluster_by_name`
  - `Snapshot::inherit_endpoint_runtime`
  - `Snapshot::update_endpoint_runtime`
  - `Snapshot::endpoint_is_available_at`
  - `backend_ref_is_routable`

### `dataplane/crates/aeg-ir/src/tests_selection.rs`

- Responsibilities:
  - Include entry point for `aeg-ir` route / backend selection tests.
- Notes:
  - Split into multiple sub-files by scenario cluster, retaining unified helpers and fixtures in `tests.rs`.

### `dataplane/crates/aeg-ir/src/tests_selection/core.rs`

- Responsibilities:
  - Basic HTTP / weighted runtime state regression tests.
- Key test topics:
  - HTTP condition matching route selection
  - Weighted round-robin progress inheritance
  - Backend re-entering rotation after recovery
  - Runtime state inheritance after backend weight changes

### `dataplane/crates/aeg-ir/src/tests_selection/endpoint_health.rs`

- Responsibilities:
  - Endpoint passive circuit breaking / active probe health status tests.
- Key test topics:
  - Passive ejection and cooldown recovery
  - Success clearing failure streak
  - Active probe unhealthy / healthy state transitions
  - Snapshot inheriting endpoint runtime state

### `dataplane/crates/aeg-ir/src/tests_selection/indexes_and_grpc.rs`

- Responsibilities:
  - Runtime index, snapshot signal, and gRPC route selection tests.
- Key test topics:
  - Backend / secret / workload index
  - Listener / hostname / stream route precomputed indexes
  - Async + blocking snapshot signal wakeup
  - Wildcard hostname and gRPC exact / regex matching

### `dataplane/crates/aeg-ir/src/tests_selection/stream_and_fallback.rs`

- Responsibilities:
  - Stream route selection and fallback semantic tests.
- Key test topics:
  - TLS passthrough SNI matching
  - Exact SNI preferred over wildcard
  - UDP listener backend selection
  - First healthy backend fallback
  - "Route exists but request does not match" returns empty selection

### `dataplane/crates/aeg-http/src/runtime/tests_http1.rs`

- Responsibilities:
  - HTTP/1 runtime integration test include entry point.
- Notes:
  - The original matrix has been split into sub-files by "protocol admission / request body / connection reuse / retries and rate limiting."

### `dataplane/crates/aeg-http/src/runtime/tests_http1/protocol_admission.rs`

- Responsibilities:
  - Protocol admission tests after HTTP/1 requests enter the listener.
- Key test topics:
  - CORS preflight local handling
  - Route inflight limit
  - Listener inflight limit

### `dataplane/crates/aeg-http/src/runtime/tests_http1/request_body.rs`

- Responsibilities:
  - HTTP/1 request body forwarding, size limits, trailers, and downstream read timeout tests.
- Key test topics:
  - Content-length / chunked body forwarding
  - Request body / header limits
  - Chunked trailers compatibility
  - `Expect: 100-continue`
  - Slow upload and downstream read timeout

### `dataplane/crates/aeg-http/src/runtime/tests_http1/connection_and_direct.rs`

- Responsibilities:
  - HTTP/1 keepalive, upstream connection pool, and direct response tests.
- Key test topics:
  - Downstream/upstream keepalive reuse
  - Idle upstream connection pool reuse
  - Direct response local short-circuit and observability output

### `dataplane/crates/aeg-http/src/runtime/tests_http1/retries_and_limits.rs`

- Responsibilities:
  - HTTP/1 retry, retry budget, rate limit, and circuit breaker tests.
- Key test topics:
  - Connect failure retry
  - Retryable response reselect
  - Retry budget exhaustion
  - Route rate limit 429
  - Backend circuit breaker 503

### `dataplane/crates/aeg-http/src/runtime/tests_h2c.rs`

- Responsibilities:
  - H2C / gRPC runtime test entry file.
- Key contents:
  - Aggregates H2C sub-scenario tests via `include!`, retaining shared helper scope.

### `dataplane/crates/aeg-http/src/runtime/tests_h2c/connection_reuse.rs`

- Responsibilities:
  - H2C upstream connection reuse tests.
- Key test topics:
  - Single upstream HTTP/2 connection multiplexing multiple request streams

### `dataplane/crates/aeg-http/src/runtime/tests_h2c/grpc_payloads.rs`

- Responsibilities:
  - gRPC data payload and streaming body forwarding tests.
- Key test topics:
  - Server streaming over H2C
  - Unary request body forwarding
  - Backend mid-stream disconnect downstream error propagation

### `dataplane/crates/aeg-http/src/runtime/tests_h2c/grpc_control.rs`

- Responsibilities:
  - gRPC control signals, metadata, and cancellation semantic tests.
- Key test topics:
  - `grpc-timeout` passthrough
  - Client cancel to upstream reset propagation
  - Response metadata / trailers passthrough

### `dataplane/crates/aeg-http/src/runtime/tests_h2c/protocol_mix.rs`

- Responsibilities:
  - H2C prior-knowledge and HTTP/1 + H2C mixed backend tests.
- Key test topics:
  - Prior knowledge preface
  - HTTP/1 and H2C backends coexisting on the same listener

## Current Exceptions Over 800 Lines

These files were not split further in this pass because they are primarily scenario-matrix tests or benchmark scaffolding, where the benefit of splitting is outweighed by the loss of reading continuity. If further cleanup is done in the future, it is recommended to split by "scenario family" rather than arbitrarily by line count.

### Control Plane Test Exceptions

- `controlplane/internal/grpcserver/server_test.go`
- `controlplane/internal/infrastructure/reconciler_core_test.go`
- `controlplane/internal/infrastructure/reconciler_mesh_test.go`
- `controlplane/internal/status/reconciler_core_test.go`
- `controlplane/internal/status/reconciler_route_misc_test.go`
- `controlplane/internal/status/reconciler_acceptance_cross_namespace_test.go`
- `controlplane/internal/nodestatus/registry_test.go`
- `controlplane/internal/translator/translator_test.go`
- `controlplane/internal/controller/syncer_test.go`

Notes:
- Most of these files are status matrix, cross-resource priority, or regression scenario table-driven tests.
- If further splitting is desired, it is recommended to split by `happy-path / conflict / cross-namespace / invalid / performance fixture` dimensions.

### Data Plane Test and Benchmark Exceptions

- `dataplane/crates/aeg-http/src/proxy/tests/backend.rs`
- `dataplane/crates/aeg-http/src/proxy/tests/context.rs`
- `dataplane/crates/aeg-app/src/admin/tests/summary_core.rs`
- `dataplane/crates/aeg-ir/src/bench.rs`

Notes:
- These files are high-density protocol/behavior matrices with significant shared fixtures internally.
- It is recommended to first extract common fixtures, then split into multiple test files by "protocol family," "status family," and "failure family."

## Future Maintenance Recommendations

- When adding new logic, prefer appending to the sub-file closest to the current responsibility; do not pile helpers back into the main entry file.
- When a file approaches 800 lines again, prefer splitting by "single responsibility + call boundary" rather than evenly cutting by line count.
- When splitting test files, prefer using scenario semantics as the boundary; do not arbitrarily split the same table-driven test.
