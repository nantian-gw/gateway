# Admin API

This document is a human-readable description of the control plane and data plane admin APIs. The machine-readable stable surface is defined by the [Admin API Contract](../contracts/admin-api-contract.md) and [admin-api-surface.json](../contracts/admin-api-surface.json).

The current repository includes a Next.js/React admin console and Node proxy in [`dashboard/`](../../dashboard/), and provides a dashboard Deployment / Service / NetworkPolicy in `deploy/kubernetes/base/dashboard.yaml`. The dashboard is a consumer of the admin API, not a new public admin surface contract; do not treat dashboard proxy paths or page data models as stable APIs. The old frontend-specific aggregation API has been removed. Future web consoles or SDKs should be modeled on the controlplane / dataplane admin APIs listed in this document and the machine-readable surface.

## General Rules

- `/livez` and `/readyz` do not require authentication by default, for convenient use with Kubernetes probes.
- Except for probes, controlplane `/v1/*`, dataplane `/metrics`, and dataplane `/v1/*` all require `Authorization: Bearer <token>` when `adminAuth.bearerToken` or `adminAuth.bearerTokenFile` is configured.
- Admin APIs should only be accessed within the cluster, via `port-forward`, controlled proxies, or controlled ops networks by default. Do not expose them directly to the public internet.
- Control plane metrics are exposed on a separate `metricsAddr` and do not share the admin port.
- Data plane `/metrics` shares the admin port with `/v1/*`.
- Filter parameters for JSON list endpoints are all optional; invalid enums, invalid booleans, and invalid pagination values should return 4xx rather than silently expanding the query scope.

## 1. Control Plane API

Access:

- Local process mode: `http://127.0.0.1:18081`
- Kind mode: `kubectl -n aether-gateway port-forward svc/aether-gateway-controlplane-admin 18081:18081`

HTTP server runtime timeouts:

- `adminRuntime.readHeaderTimeout`: default `5s`
- `adminRuntime.readTimeout`: default `30s`
- `adminRuntime.writeTimeout`: default `30s`
- `adminRuntime.idleTimeout`: default `2m`

### 1.1 Endpoint List

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/livez` | Control plane process liveness check. |
| `GET` | `/readyz` | Control plane snapshot readiness check; returns `503` when no snapshot has been generated. |
| `GET` | `/v1/summary` | Control plane summary view, including snapshot version, listener/route/backend/Secret statistics, node connections, and ACK/ready statistics. |
| `GET` | `/v1/snapshot-sync` | Current snapshot alignment with data plane ACK/ready status, including readiness mode, drift, rejection, not-ready, and disconnected node breakdown. |
| `GET` | `/v1/snapshot` | Current full published snapshot. |
| `GET` | `/v1/listeners` | List of listeners in the published snapshot. |
| `GET` | `/v1/listeners/{name}` | Individual listener details. |
| `GET` | `/v1/routes` | Grouped object `{http, grpc, stream}`, containing HTTPRoute, GRPCRoute, and stream route lists respectively. |
| `GET` | `/v1/routes/{kind}/{namespace}/{name}` | Individual route details; `kind` supports `http`, `grpc`, `tcp`, `udp`, `tls` and corresponding Route kind aliases. |
| `GET` | `/v1/backends` | Returns backends currently referenced by routes by default; use `all=true` to view the full discovery set. |
| `GET` | `/v1/backends/{namespace}/{name}` | Individual backend details; `name` uses the backend name from the snapshot, e.g. `api:80`. |
| `GET` | `/v1/nodes` | Data plane node status list, aggregating local connection state and shared Lease state. |
| `GET` | `/v1/nodes/{nodeId}` | Individual data plane node details. |
| `GET` | `/v1/infrastructure` | Lifecycle and drift view of control-plane-derived infrastructure resources. |
| `GET` | `/v1/service-catalog` | Service catalog and port summary usable as backend references. |
| `GET` | `/v1/namespaces` | List of namespaces accessible to the management plane. |
| `GET` | `/v1/resource-kinds` | Catalog of Kubernetes resource types supported by the management plane. |
| `GET` | `/v1/resources` | List of Kubernetes resources supported by the management plane. |
| `POST` | `/v1/resources` | Create or update resources via YAML/JSON in the request body. |
| `GET` | `/v1/resources/{kind}/{namespace}/{name}` | Individual supported resource details; cluster-scoped resources use `_cluster` as the namespace placeholder. |
| `PUT` | `/v1/resources/{kind}/{namespace}/{name}` | Create or update a resource identified by path. |
| `DELETE` | `/v1/resources/{kind}/{namespace}/{name}` | Delete an individual supported resource. |
| `GET` | `/v1/topology` | Topology view among listeners, routes, backends, workloads, and nodes. |
| `GET` | `/v1/dataplanes` | List of discovered data plane admin endpoints, including node ID, address, and ready status. |
| `GET` | `/v1/dataplanes/{nodeId}/summary` | Summary data for an individual data plane node. |
| `GET` | `/v1/chatbot/config` | Current Chatbot configuration (LLM provider, model, system prompt, etc.). |
| `PUT` | `/v1/chatbot/config` | Update Chatbot configuration. |
| `POST` | `/v1/chatbot/chat` | Chatbot streaming conversation (`text/event-stream`). |
| `GET` | `/v1/metrics/config` | Current Prometheus metrics proxy configuration. |
| `PUT` | `/v1/metrics/config` | Update Prometheus metrics proxy configuration. |
| `POST` | `/v1/metrics/query` | Prometheus instant query (passthrough). |
| `POST` | `/v1/metrics/query_range` | Prometheus range query (passthrough). |
| `GET` | `/v1/ai/overview` | AI global view, including total_requests, total_tokens, active_services, active_policies statistics. |
| `GET` | `/v1/ai/services` | AI service list (from AIService CRD). Supports `namespace`, `name`, `model` filtering and pagination. |
| `GET` | `/v1/ai/token-usage` | AI token usage statistics, supporting filtering by service, model, and time range. |
| `GET` | `/v1/ai/traces` | AI request call chain tracing, including latency, token count, and status code. Supports pagination. |
| `GET` | `/v1/ai/cost` | AI service cost statistics, supporting aggregation by service and model. |
| `GET` | `/v1/resources?kind=WasmPlugin` | Wasm plugin list query; the following CRUD paths also apply to experimental resources such as `AIService`, `TokenPolicy`, `WasmPlugin`. |
| — | — | Traffic data is not aggregated on the control plane. Use `/v1/dataplanes` to get the node list, then query each node's dataplane `/v1/traffic` as needed; for aggregated metrics, use `/v1/metrics/query` to query Prometheus. |

### 1.2 Query Parameters

`/v1/listeners`

- `name`
- `protocol`: `http`, `https`, `grpc`, `http3`, `tcp`, `udp`, `tls`
- `hostname`
- `attachedRoute`
- `sort`: `name`, `protocol`, default `name`
- `order`: `asc`, `desc`, default `asc`
- `limit`: positive integer
- `offset`: non-negative integer

`/v1/routes`

- `kind`: `http`, `grpc`, `tcp`, `udp`, `tls`
- `namespace`
- `name`
- `hostname`
- `sort`: `namespace`, `name`, default `namespace`
- `order`: `asc`, `desc`, default `asc`
- `limit`: positive integer; must also specify `kind`
- `offset`: non-negative integer; must also specify `kind`

`/v1/backends`

- `namespace`
- `name`
- `protocol`
- `service`
- `all`: when `true`, returns the full discovery set; defaults to only backends referenced by routes
- `sort`: `namespace`, `name`, `protocol`, default `namespace`
- `order`: `asc`, `desc`, default `asc`
- `limit`: positive integer
- `offset`: non-negative integer

`/v1/nodes`

- `nodeId`
- `cluster`
- `connected`: `true`, `false`
- `ready`: `true`, `false`
- `version`
- `sort`: `nodeId`, `cluster`, `version`, default `nodeId`
- `order`: `asc`, `desc`, default `asc`
- `limit`: positive integer
- `offset`: non-negative integer

`/v1/infrastructure`

- `state`: `ready`, `missing`, `drifted`, `orphan`
- `role`: `shared-service`, `gateway-service`, `mesh-frontend-service`, `mesh-shadow-service`, `shared-endpointslice`, `gateway-endpointslice`, `mesh-endpointslice`
- `kind`
- `namespace`
- `name`
- `sort`: `state`, `role`, `kind`, `namespace`, `name`
- `order`: `asc`, `desc`
- `limit`: positive integer
- `offset`: non-negative integer

`/v1/service-catalog`

- `namespace`
- `name`
- `port`
- `protocol`
- `sort`: `namespace`, `name`
- `order`: `asc`, `desc`
- `limit`: positive integer
- `offset`: non-negative integer

`/v1/namespaces`

- `name`
- `sort`: `name`
- `order`: `asc`, `desc`
- `limit`: positive integer
- `offset`: non-negative integer

`/v1/resources`

- `kind`
- `namespace`
- `name`
- `limit`: positive integer
- `offset`: non-negative integer

Currently supported resource types:

- `Gateway`
- `GatewayClass`
- `HTTPRoute`
- `GRPCRoute`
- `TCPRoute`
- `UDPRoute`
- `TLSRoute`
- `BackendLBPolicy`
- `BackendTLSPolicy`
- `ReferenceGrant`
- `ServiceImport`

`/v1/topology`

- `type`: `plane`, `listener`, `route`, `backend`, `endpoint-set`
- `kind`: `http`, `grpc`, `tcp`, `udp`, `tls`
- `namespace`
- `name`
- `status`
- `includeRelated`: `true`, `false`, default `false`

### 1.3 Examples

```bash
curl -fsS http://127.0.0.1:18081/livez
curl -fsS http://127.0.0.1:18081/readyz
curl -fsS http://127.0.0.1:18081/v1/summary | jq
curl -fsS http://127.0.0.1:18081/v1/snapshot-sync | jq
curl -fsS 'http://127.0.0.1:18081/v1/listeners?protocol=http&hostname=example.com' | jq
curl -fsS 'http://127.0.0.1:18081/v1/routes?kind=grpc&sort=name&offset=0&limit=20' | jq
curl -fsS 'http://127.0.0.1:18081/v1/backends?all=true&sort=protocol&order=asc' | jq
curl -fsS 'http://127.0.0.1:18081/v1/nodes?connected=true&ready=true' | jq
curl -fsS 'http://127.0.0.1:18081/v1/infrastructure?state=drifted&sort=name' | jq
curl -fsS 'http://127.0.0.1:18081/v1/service-catalog?namespace=default&limit=20' | jq
curl -fsS 'http://127.0.0.1:18081/v1/resources?kind=Gateway&limit=20&offset=0' | jq
curl -fsS 'http://127.0.0.1:18081/v1/topology?type=route&kind=http&namespace=default&name=web&includeRelated=true' | jq
curl -fsS http://127.0.0.1:18081/v1/namespaces | jq
curl -fsS http://127.0.0.1:18081/v1/chatbot/config | jq
curl -fsS -X PUT http://127.0.0.1:18081/v1/chatbot/config -H 'Content-Type: application/json' -d '{"provider":"openai","model":"gpt-4o"}'
curl -fsS -X POST http://127.0.0.1:18081/v1/chatbot/chat -H 'Content-Type: application/json' -d '{"message":"create a Gateway named web-gw"}'
curl -fsS -X POST http://127.0.0.1:18081/v1/metrics/query -H 'Content-Type: application/json' -d '{"query":"up"}'
curl -fsS -X POST http://127.0.0.1:18081/v1/metrics/query_range -H 'Content-Type: application/json' -d '{"query":"rate(http_requests_total[5m])","start":"2025-01-01T00:00:00Z","end":"2025-01-01T01:00:00Z","step":"30s"}'
curl -fsS -H "Authorization: Bearer ${PGW_ADMIN_TOKEN}" http://127.0.0.1:18081/v1/summary | jq
```

## 2. Data Plane API

Access:

- Local process mode: `http://127.0.0.1:19080`
- Kind mode: `kubectl -n aether-gateway port-forward svc/aether-gateway-dataplane-admin 19080:19080`

Debug scripts, the dashboard, and future SDKs should access the data plane admin port `19080` and must not mistakenly use the traffic ports `10080`, `80`, or `443`.

### 2.1 Endpoint List

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/livez` | Process-level liveness check. Returns `200 alive` during normal operation; returns `503` when the supervisor requests shutdown or the required runtime has exited. |
| `GET` | `/readyz` | Snapshot and runtime readiness check. Distinguishes between `warming`, `serving-current`, `serving-last-good`, and `not-ready`. |
| `GET` | `/metrics` | Prometheus text-format metrics. |
| `GET` | `/v1/summary` | Data plane summary view, including node, snapshot, xDS, runtime, listener status, protection policies, and summary diagnostics. |
| `GET` | `/v1/node` | Node view, currently reusing the same summary information as `/v1/summary`. |
| `GET` | `/v1/snapshot` | Current full runtime snapshot. |
| `GET` | `/v1/overload` | Overload manager current configuration and observed state. |
| `GET` | `/v1/circuit-breakers` | Circuit breaker configuration, state, and metrics view. |
| `GET` | `/v1/rate-limits` | Rate limit configuration, state, and metrics view. |
| `GET` | `/v1/listeners` | List of listeners in the current snapshot. |
| `GET` | `/v1/listeners/{name}` | Individual listener details. |
| `GET` | `/v1/listener-statuses` | List of listener runtime current/carrying/recovery states. |
| `GET` | `/v1/listener-statuses/{name}` | Individual listener runtime state details. |
| `GET` | `/v1/routes` | Grouped object `{http, grpc, stream}`. |
| `GET` | `/v1/routes/{kind}/{namespace}/{name}` | Individual route details; `kind` supports `http`, `grpc`, `tcp`, `udp`, `tls`. |
| `GET` | `/v1/backends` | Current backend cluster list. |
| `GET` | `/v1/backends/{namespace}/{name}` | Individual backend cluster details. |
| `GET` | `/v1/traffic` | Data plane traffic observation summary. |

### 2.2 Query Parameters

`/v1/listeners`

- `name`
- `protocol`
- `hostname`
- `attachedRoute`
- `runtimeId`: 16-character hex runtime ID, usable to trace back from access logs, `/v1/summary`, `/v1/traffic`, or list responses to the listener.

`/v1/listener-statuses`

- `name`
- `protocol`
- `hostname`
- `attachedRoute`
- `runtimePlane`: `http`, `stream`, `tls`, `none`
- `currentStatus`: `idle`, `warming`, `pending`, `accepted`, `retained`, `rejected`, `stale`
- `currentFailure`: `true`, `false`
- `hasEverFailed`: `true`, `false`
- `attentionRequired`: `true`, `false`
- `attentionReason`: `pending`, `rejected`, `stale`, `unrecovered_failure`
- `recoveredFromFailure`: `true`, `false`
- `servingSnapshot`: `current`, `last-good`
- `servingVersion`
- `servingState`: `none`, `current-accepted`, `current-retained`, `last-good-rejected`, `last-good-stale`
- `attemptProgress`: `awaiting-current`, `blocked-current`, `other`
- `recoveryState`: `idle`, `warming`, `steady`, `recovered`, `awaiting-current`, `blocked-current`, `unrecovered-current`, `unrecovered-historical`, `drifted-last-good`
- `unrecoveredFailureAge`: `current`, `historical`, `none`

`/v1/routes`

- `kind`
- `namespace`
- `name`
- `hostname`
- `runtimeId`: 16-character hex route runtime ID.
- `ruleRuntimeId`: 16-character hex rule runtime ID, used to trace from rule IDs in logs back to the owning route.

`/v1/backends`

- `namespace`
- `name`
- `protocol`
- `runtimeId`: 16-character hex backend runtime ID.
- `endpointRuntimeId`: 16-character hex endpoint runtime ID, used to trace from logs or traffic topology back to the backend endpoint.

### 2.3 Key Fields

`/v1/summary` currently provides the following stable handshake fields:

- `summarySurface=dataplane-summary`
- `summarySchemaVersion=1`

Common runtime diagnostic fields include:

- `currentSnapshotStatus`
- `currentSnapshotFallbackState`
- `servingLastGoodSnapshot`
- `runtimeHttpCurrentStatus`
- `runtimeStreamCurrentStatus`
- Listener current state count, carrying state count, recovery state count, and convergence blocking count
- xDS ACK/NACK, most recent failure version, and error reason

`/v1/listener-statuses` is the preferred list endpoint for troubleshooting runtime reload, last-good, stale listener, and failure recovery. Warnings or recommended paths in `/v1/summary` typically point to filter combinations on this endpoint.

`/v1/listeners`, `/v1/routes`, and `/v1/backends` list and detail responses will, when resolvable, also output:

- `runtimeId`: stable hex runtime ID.
- `runtimeRef`: structured resource reference reverse-resolved from the runtime ID, e.g. listener name, route namespace/name, backend key, or endpoint address/port.
- `ruleRuntimeIds` / `ruleRuntimeRefs`: route rule-level IDs and structured reference arrays.

`/v1/traffic` topology nodes will also output the same `runtimeRef` when a `runtimeId` is present and the current snapshot is resolvable, enabling direct jumps from traffic graph nodes back to listener, route, or backend resources.

The old name, namespace, and backend string fields remain for compatibility; the new `runtimeRef` field is used to correlate logs, traffic topology, and admin views by the same ID before displaying resource references.

### 2.4 Examples

```bash
curl -fsS http://127.0.0.1:19080/livez
curl -fsS http://127.0.0.1:19080/readyz
curl -fsS http://127.0.0.1:19080/metrics
curl -fsS http://127.0.0.1:19080/v1/summary | jq
curl -fsS http://127.0.0.1:19080/v1/snapshot | jq
curl -fsS 'http://127.0.0.1:19080/v1/listeners?protocol=http' | jq
curl -fsS 'http://127.0.0.1:19080/v1/listener-statuses?attentionRequired=true' | jq
curl -fsS 'http://127.0.0.1:19080/v1/routes?kind=tls&hostname=secure.example.com' | jq
curl -fsS 'http://127.0.0.1:19080/v1/backends?namespace=default&protocol=http' | jq
curl -fsS http://127.0.0.1:19080/v1/overload | jq
curl -fsS http://127.0.0.1:19080/v1/circuit-breakers | jq
curl -fsS http://127.0.0.1:19080/v1/rate-limits | jq
curl -fsS http://127.0.0.1:19080/v1/traffic | jq
curl -fsS -H "Authorization: Bearer ${PGW_ADMIN_TOKEN}" http://127.0.0.1:19080/v1/summary | jq
```

## 3. Dashboard / SDK API Boundary

- Do not assume the repository has a stable frontend-specific aggregation layer; the dashboard proxy is only a same-origin forwarding and page implementation detail, and old paths are no longer part of the current contract.
- Prefer composing views from controlplane `/v1/summary`, `/v1/snapshot-sync`, `/v1/topology`, `/v1/resources`, `/v1/dataplanes` and dataplane `/v1/summary`, `/v1/listener-statuses`, `/v1/traffic`. For aggregated traffic metrics, use `/v1/metrics/query` to query Prometheus, or discover nodes via `/v1/dataplanes` and query each node's dataplane `/v1/traffic` as needed.
- When writing Kubernetes resources, only enter through controlplane `/v1/resources` and adhere to the supported resource type list.
- Do not store admin bearer tokens in the browser; production entry points should handle identity, session, and token injection through controlled proxies, internal control planes, or a future dedicated auth layer.
- If a new aggregation API is needed, update [admin-api-surface.json](../contracts/admin-api-surface.json), the route contract test, and this document before implementing server-side code.

## 4. Traffic Data Retrieval

The control plane does not provide an aggregated traffic endpoint. Use the following methods to retrieve traffic data:

- Use `/v1/dataplanes` to get the list of all data plane nodes (including `nodeId`, address, and ready status).
- Directly query each specific node's dataplane `/v1/traffic` endpoint as needed for that node's real-time traffic statistics.
- Use `/v1/dataplanes/{nodeId}/summary` to get summary information for an individual node.
- For cluster-level aggregated metrics (request count, latency distribution, error rate, etc.), query Prometheus via `/v1/metrics/query`.
